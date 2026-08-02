package github

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestAsset 构造一个指向本地 httptest 服务器的 Asset
func newTestAsset(url string) Asset {
	return Asset{Name: "test.bin", BrowserDownloadURL: url}
}

// serveTestData 启动一个支持 Range 的模拟下载服务器，返回 url 与内容
func serveTestData(t *testing.T, data []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHdr := r.Header.Get("Range")
		if rangeHdr == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(200)
			w.Write(data)
			return
		}
		// 解析 "bytes=start-end"
		var start, end int64
		if _, err := fmt.Sscanf(strings.TrimPrefix(rangeHdr, "bytes="), "%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", 400)
			return
		}
		if end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(206)
		w.Write(data[start : end+1])
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func expectedSHA(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// 测试动态段分配 worker 池：并行下载完整文件并校验一致性
func TestDynamicSegmentParallel(t *testing.T) {
	// 12MB 数据，段大小压到 1MB，并发 4
	data := make([]byte, 12<<20)
	for i := range data {
		data[i] = byte(i % 251)
	}
	url := serveTestData(t, data)

	segSize = 1 << 20
	stallTimeout = 5 * time.Second
	defer func() { segSize = 4 << 20; stallTimeout = 20 * time.Second }()

	dest := t.TempDir() + "/out.bin"
	asset := newTestAsset(url)

	start := time.Now()
	if err := NewClient().DownloadAssetParallel(asset, dest, 4, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	t.Logf("parallel 12MB in %v", time.Since(start).Round(time.Millisecond))

	if got := fileSHA(t, dest); got != expectedSHA(data) {
		t.Fatalf("content mismatch:\n got %s\nwant %s", got, expectedSHA(data))
	}
}

// 测试僵死连接检测 + 逐段重试：某一段首次下载被掐断，重试后成功，整体文件完整
func TestStallDetectionAndRetry(t *testing.T) {
	data := make([]byte, 8<<20)
	for i := range data {
		data[i] = byte(i % 199)
	}

	// 记录某段(1MB段大小下，段2即 [1MB,2MB))的首次请求数，首次故意僵死
	var mu sync.Mutex
	stalledOnce := false

	handler := func(w http.ResponseWriter, r *http.Request) {
		rangeHdr := r.Header.Get("Range")
		var start, end int64
		fmt.Sscanf(strings.TrimPrefix(rangeHdr, "bytes="), "%d-%d", &start, &end)
		if end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		// 命中目标段且是首次：声明整段长度，但只写 100 字节后长时间挂起，
		// 让客户端 Read 阻塞等待剩余数据 -> 触发僵死检测 -> 中断重试
		if start == 1<<20 && func() bool {
			mu.Lock()
			defer mu.Unlock()
			if stalledOnce {
				return false
			}
			stalledOnce = true
			return true
		}() {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
			w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			w.WriteHeader(206)
			w.Write(data[start : start+100]) // 只写 100 字节然后僵死
			time.Sleep(5 * time.Second)      // 远超 stallTimeout，期间无新字节
			return
		}
		// 正常返回完整段
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(206)
		w.Write(data[start : end+1])
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	segSize = 1 << 20
	stallTimeout = 200 * time.Millisecond
	defer func() { segSize = 4 << 20; stallTimeout = 20 * time.Second }()

	dest := t.TempDir() + "/out.bin"
	start := time.Now()
	if err := NewClient().DownloadAssetParallel(newTestAsset(srv.URL), dest, 4, nil); err != nil {
		t.Fatalf("download with stall should succeed: %v", err)
	}
	t.Logf("stall+retry download in %v", time.Since(start).Round(time.Millisecond))

	if got := fileSHA(t, dest); got != expectedSHA(data) {
		t.Fatalf("content mismatch:\n got %s\nwant %s", got, expectedSHA(data))
	}
}
