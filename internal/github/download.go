// Package github 多线程下载实现。
//
// 下载核心借鉴 aria2 / axel 的"段分配器"算法，针对弱网 + 代理环境做三层强化：
//  1. 动态段分配（而非静态分块）：workers 用一个互斥游标按需领取小段，
//     快 worker 会自然领到更多段，慢 worker 不会拖累其它 worker——自平衡。
//  2. 逐段重试（而非整体回退）：某一段失败只重新入队该段并指数退避，
//     绝不再因为一段失败就全盘放弃多线程回退单线程。
//  3. 僵死连接检测：每个段配一个 watchdog，盯住"最后读到字节的时间"，
//     超过阈值即判定连接僵死，主动掐断该段请求交由其它 worker 重试。
//
// 相比 aria2 的二进制/库依赖，这里仅用 Go 标准库即可复刻其核心思路。
package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leisurelinux/ghdeb/internal/i18n"
)

// 下载配置
const (
	defaultConcurrency = 4               // 默认并发数
	minChunkSize       = 1 << 20         // 小于该大小不值得多线程
	maxSegmentRetries  = 5               // 单段最大重试次数
	writeBufSize       = 64 * 1024       // 写缓冲
)

// 以下两项用变量而非常量，便于测试时注入小值做快速验证。
// 生产环境保持默认即可：4MB 段 + 20s 僵死阈值。
var (
	segSize      = 4 << 20          // 动态段大小 4MB
	stallTimeout = 20 * time.Second // 段内超过该时长无新字节判定僵死
)

// downloadState 共享的下载进度状态（worker 池共享）
type downloadState struct {
	mu         sync.Mutex
	nextOffset int64 // 下一个待分配的段起点（动态游标）
	fileSize   int64
	failed     error // 是否出现不可恢复错误

	downloaded int64 // 已完成的字节数（atomic 访问）
}

// nextSegment 动态领取下一段 [start,end]，文件领完返回 ok=false
func (s *downloadState) nextSegment() (start, end int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nextOffset >= s.fileSize {
		return 0, 0, false
	}
	start = s.nextOffset
	end = start + int64(segSize) - 1
	if end >= s.fileSize {
		end = s.fileSize - 1
	}
	s.nextOffset = end + 1
	return start, end, true
}

func (s *downloadState) addDownloaded(n int64) {
	atomic.AddInt64(&s.downloaded, n)
}

func (s *downloadState) currentDownloaded() int64 {
	return atomic.LoadInt64(&s.downloaded)
}

func (s *downloadState) setFailed(err error) {
	s.mu.Lock()
	if s.failed == nil {
		s.failed = err
	}
	s.mu.Unlock()
}

func (s *downloadState) getFailed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}

// checkRangeSupport 检查服务器是否支持 Range 请求
func (c *Client) checkRangeSupport(downloadURL string) (bool, int64, error) {
	// 使用 GET 请求的 Range: bytes=0-0 来检查支持情况并获取文件大小
	// 这比 HEAD 更可靠，因为某些服务器对 HEAD 和 GET 的响应不同
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Range", "bytes=0-0")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	// 读完 body 使连接能被复用，避免连接泄漏
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	// 206 表示支持 Range，200 表示不支持
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("range check returned HTTP %d", resp.StatusCode)
	}

	supportsRange := resp.StatusCode == http.StatusPartialContent

	// 从 Content-Range 获取文件大小: "bytes 0-0/12345"
	var contentLength int64
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		var total int64
		if _, err := fmt.Sscanf(contentRange, "bytes 0-0/%d", &total); err == nil {
			contentLength = total
		}
	}

	// 如果不支持 Range，尝试从 Content-Length 获取
	if !supportsRange && resp.ContentLength > 0 {
		contentLength = resp.ContentLength
	}

	return supportsRange, contentLength, nil
}

// downloadSegment 下载单个动态段 [start,end]，带僵死连接检测。
// 通过 watchdog goroutine 监控最后读到字节的时间，若超过 stallTimeout
// 判定连接僵死，主动 cancel + 关闭 body 中断当前阻塞读，返回错误交由上层重试。
func (c *Client) downloadSegment(ctx context.Context, downloadURL string, start, end int64, destPath string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chunk download returned HTTP %d", resp.StatusCode)
	}

	// 打开文件并定位到正确位置
	f, err := os.OpenFile(destPath, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}

	// ---- 僵死连接检测 watchdog ----
	// lastRead 记录最后读到字节的时刻（nanotime，atomic 访问）
	var lastRead int64
	atomic.StoreInt64(&lastRead, time.Now().UnixNano())

	done := make(chan struct{})
	var watchdogOnce sync.Once
	stopWatchdog := func() { watchdogOnce.Do(func() { close(done) }) }
	defer stopWatchdog()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				last := atomic.LoadInt64(&lastRead)
				if time.Since(time.Unix(0, last)) >= stallTimeout {
					// 僵死：中断当前阻塞读，交由上层重试
					cancel()
					resp.Body.Close()
					return
				}
			}
		}
	}()

	// ---- 带进度的段拷贝 ----
	buf := make([]byte, writeBufSize)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			atomic.StoreInt64(&lastRead, time.Now().UnixNano())
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				stopWatchdog()
				return writeErr
			}
			written += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			stopWatchdog()
			return readErr
		}
	}
	stopWatchdog()

	// 校验段长度，防止截断/被 watchdog 中断后仍误判成功
	expected := end - start + 1
	if written != expected {
		return fmt.Errorf("segment truncated: got %d, want %d", written, expected)
	}
	return nil
}

// worker 单个下载 worker 的主循环：
// 动态领段 -> 下载（失败则逐段重试+指数退避）-> 报告进度 -> 领下一段。
func (c *Client) worker(ctx context.Context, downloadURL, destPath string, st *downloadState, progress func(downloaded, total int64)) {
	for {
		start, end, ok := st.nextSegment()
		if !ok {
			return // 所有段已分配完
		}

		// 逐段重试（指数退避），失败不殃及其它 worker
		var segErr error
		for attempt := 1; ; attempt++ {
			if ctx.Err() != nil {
				st.setFailed(ctx.Err())
				return
			}
			segErr = c.downloadSegment(ctx, downloadURL, start, end, destPath)
			if segErr == nil {
				break
			}
			if attempt >= maxSegmentRetries {
				st.setFailed(segErr)
				return
			}
			wait := retryBaseWait * time.Duration(1<<(attempt-1)) // 2s,4s,8s...
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				st.setFailed(ctx.Err())
				return
			}
		}

		st.addDownloaded(end - start + 1)
		if progress != nil {
			progress(st.currentDownloaded(), st.fileSize)
		}
	}
}

// DownloadAssetParallel 多线程下载 asset（动态段分配 worker 池）。
func (c *Client) DownloadAssetParallel(asset Asset, destPath string, concurrency int, progress func(downloaded, total int64)) error {
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	// 检查是否支持 Range
	supportsRange, fileSize, err := c.checkRangeSupport(asset.BrowserDownloadURL)
	if err != nil {
		// 回退到单线程
		return c.DownloadAsset(asset, destPath, progress)
	}

	if !supportsRange || fileSize <= 0 {
		// 不支持 Range 或无法获取大小，回退到单线程
		return c.DownloadAsset(asset, destPath, progress)
	}

	// 小文件不需要多线程
	if fileSize < minChunkSize*2 {
		return c.DownloadAsset(asset, destPath, progress)
	}

	// 创建目标文件并预分配空间
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	if err := f.Truncate(fileSize); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// 启动 worker 池，动态领段
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := &downloadState{
		fileSize: fileSize,
	}

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.worker(ctx, asset.BrowserDownloadURL, destPath, st, progress)
		}()
	}
	wg.Wait()

	if err := st.getFailed(); err != nil {
		return err
	}
	return nil
}

// DownloadAssetWithFallback 带多线程回退的下载。
// 多线程下载本身已内建"逐段重试 + 僵死检测"，只有出现不可恢复错误
// 或文件不支持 Range 时才回退到单线程下载。
func (c *Client) DownloadAssetWithFallback(asset Asset, destPath string, progress func(downloaded, total int64)) error {
	err := c.DownloadAssetParallel(asset, destPath, defaultConcurrency, progress)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			i18n.T("⚠️  多线程下载失败，回退到单线程: %v\n",
				"⚠️  Parallel download failed, falling back to single thread: %v\n"), err)
		return c.DownloadAsset(asset, destPath, progress)
	}
	return nil
}
