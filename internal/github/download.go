// Package github 多线程下载实现。
//
// 下载核心借鉴 aria2 / axel 的"抢占式段表"算法，针对弱网 + 代理环境做四层强化：
//  1. 段队列 + 抢占接管：所有段放入共享队列，worker 按需领取。
//     某段下载中断/僵死时，把"未下载的剩余部分"重新入队，空闲 worker
//     可立即抢占接管——绝不因为一段僵死就整体干等。
//  2. 逐段重试（无退避）：段失败只把剩余部分交回队列，由空闲 worker 接手，
//     避免指数退避造成的长时间假死。
//  3. 僵死连接检测：watchdog 从请求发出前就开始盯住"最后读到字节的时间"，
//     覆盖"等响应头"与"读响应体"两个阶段，超过阈值即主动 cancel 中断，
//     交回剩余段。绝不无限期卡在 Do() 或 Read() 上。
//  4. 实时字节进度：读循环内随写盘实时上报已下载字节，界面连续跳动，
//     不再"整段完成才报"造成卡死观感。
//
// 相比 aria2 的二进制/库依赖，这里仅用 Go 标准库即可复刻其核心抢占算法。
package github

import (
	"context"
	"errors"
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
	defaultConcurrency = 4                      // 默认并发数
	minChunkSize       = 1 << 20                // 小于该大小不值得多线程
	maxSegmentRetries  = 5                      // 单个剩余段最大接管/重试次数
	writeBufSize       = 64 * 1024              // 写缓冲
	progressInterval   = 200 * time.Millisecond // 实时进度上报的最小间隔
)

// 以下几项用变量而非常量，便于测试时注入小值做快速验证。
// 生产环境保持默认即可：4MB 段 + 15s 僵死阈值。
var (
	segSize      = 4 << 20          // 动态段大小 4MB
	stallTimeout = 15 * time.Second // 段内超过该时长无新字节判定僵死
)

// segment 一个待下载的段（或剩余子段）
type segment struct {
	start, end int64
	attempts   int // 已被接管/重试的次数
}

// downloadState 共享的下载进度状态（worker 池共享，用条件变量协调抢占）
type downloadState struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pending  []segment // 待下载段队列
	active   int       // 正在下载（含阻塞读）的 worker 数
	fileSize int64
	failed   error // 不可恢复错误
	done     bool  // 全部字节已落盘

	downloaded int64 // 已完成的字节数（atomic 访问）
	progress   func(downloaded, total int64)
	lastTick   time.Time // 上次进度回调时刻（用于节流）
}

// newDownloadState 初始化下载状态，fileSize 决定何时判定完成
func newDownloadState(fileSize int64, progress func(downloaded, total int64)) *downloadState {
	s := &downloadState{
		fileSize: fileSize,
		progress: progress,
		lastTick: time.Now(),
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// enqueueInitial 按段大小预填充整份文件的初始段队列
func (s *downloadState) enqueueInitial(segSize int64) {
	s.mu.Lock()
	for off := int64(0); off < s.fileSize; off += segSize {
		end := off + segSize - 1
		if end >= s.fileSize {
			end = s.fileSize - 1
		}
		s.pending = append(s.pending, segment{start: off, end: end})
	}
	s.cond.Broadcast()
	s.mu.Unlock()
}

// take 领取一个段。无段可领时阻塞等待，直到有段、出错或全部完成。
// 领取成功即计入 active，防止队列变空时 worker 误判完成退出。
func (s *downloadState) take() (segment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.failed != nil || s.done {
			return segment{}, false
		}
		if len(s.pending) > 0 {
			seg := s.pending[0]
			s.pending = s.pending[1:]
			s.active++
			return seg, true
		}
		s.cond.Wait()
	}
}

// requeue 把剩余段交回队列尾部，唤醒所有等待的 worker（抢占接管）
func (s *downloadState) requeue(seg segment) {
	s.mu.Lock()
	s.pending = append(s.pending, seg)
	s.cond.Broadcast()
	s.mu.Unlock()
}

// finish 标记一个 worker 处理完当前段（无论成败）。当无活跃 worker 且
// 队列为空时即全部完成；若字节数对不上则视为内部错误。
func (s *downloadState) finish() {
	s.mu.Lock()
	s.active--
	if s.failed == nil && s.active == 0 && len(s.pending) == 0 {
		if atomic.LoadInt64(&s.downloaded) >= s.fileSize {
			s.done = true
		} else {
			s.failed = errors.New("internal: downloaded bytes mismatch")
		}
	}
	s.cond.Broadcast()
	s.mu.Unlock()
}

// addDownloaded 累加已下载字节并节流地触发实时进度回调
func (s *downloadState) addDownloaded(n int64) {
	d := atomic.AddInt64(&s.downloaded, n)
	if s.progress == nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	if now.Sub(s.lastTick) >= progressInterval {
		s.lastTick = now
		s.mu.Unlock()
		s.progress(d, s.fileSize)
		return
	}
	s.mu.Unlock()
}

func (s *downloadState) setFailed(err error) {
	s.mu.Lock()
	if s.failed == nil {
		s.failed = err
	}
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *downloadState) getFailed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}

// checkRangeSupport 检查服务器是否支持 Range 请求
func (c *Client) checkRangeSupport(downloadURL string) (bool, int64, error) {
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
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("range check returned HTTP %d", resp.StatusCode)
	}

	supportsRange := resp.StatusCode == http.StatusPartialContent

	var contentLength int64
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		var total int64
		if _, err := fmt.Sscanf(contentRange, "bytes 0-0/%d", &total); err == nil {
			contentLength = total
		}
	}

	if !supportsRange && resp.ContentLength > 0 {
		contentLength = resp.ContentLength
	}

	return supportsRange, contentLength, nil
}

// downloadSegment 下载单个段 [start,end]，带僵死检测与实时进度。
//
// watchdog 在发起请求前即启动，统一盯住两个阶段：
//   - 等响应头（http.Client.Do 阻塞）：lastRead 一直停留在段启动时刻，
//     若超过 stallTimeout 无响应头，cancel 中断 Do，交回整段重试；
//   - 读响应体（resp.Body.Read 阻塞）：每读到 n>0 字节更新 lastRead，
//     若超过 stallTimeout 无新字节，cancel 中断阻塞读，交回剩余段抢占。
//
// 返回已写盘的字节数与错误；调用方据此计算"剩余部分"交回队列抢占。
func (c *Client) downloadSegment(ctx context.Context, downloadURL string, start, end int64, destPath string, st *downloadState) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ---- 僵死检测 watchdog（覆盖 Do 与 Body 读两个阶段）----
	// lastRead 初始化为段启动时刻：若连响应头都迟迟不来，同样视为僵死。
	var lastRead int64
	atomic.StoreInt64(&lastRead, time.Now().UnixNano())

	done := make(chan struct{})
	var watchdogOnce sync.Once
	stopWatchdog := func() { watchdogOnce.Do(func() { close(done) }) }
	defer stopWatchdog()

	// ticker 间隔跟随 stallTimeout 自适应，测试注入小阈值时也能秒级触发
	tick := stallTimeout / 4
	if tick < 20*time.Millisecond {
		tick = 20 * time.Millisecond
	}
	if tick > 2*time.Second {
		tick = 2 * time.Second
	}

	go func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				last := atomic.LoadInt64(&lastRead)
				if time.Since(time.Unix(0, last)) >= stallTimeout {
					cancel() // 中断当前阻塞的 Do 或 Body.Read
					return
				}
			}
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("chunk download returned HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0, err
	}

	// ---- 带实时进度的段拷贝 ----
	buf := make([]byte, writeBufSize)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			atomic.StoreInt64(&lastRead, time.Now().UnixNano())
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				stopWatchdog()
				return written, writeErr
			}
			written += int64(n)
			st.addDownloaded(int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			stopWatchdog()
			return written, readErr
		}
	}
	stopWatchdog()

	expected := end - start + 1
	if written != expected {
		return written, fmt.Errorf("segment truncated: got %d, want %d", written, expected)
	}
	return written, nil
}

// worker 单个下载 worker 的主循环：
// 领段 -> 下载（失败把剩余部分交回队列抢占）-> 实时报进度 -> 领下一段。
func (c *Client) worker(ctx context.Context, downloadURL, destPath string, st *downloadState) {
	for {
		seg, ok := st.take()
		if !ok {
			return // 已完成或已出错
		}

		for {
			written, err := c.downloadSegment(ctx, downloadURL, seg.start, seg.end, destPath, st)
			if err == nil {
				break // 本段完成
			}
			if ctx.Err() != nil {
				st.setFailed(ctx.Err())
				break
			}

			remStart := seg.start + written
			if remStart > seg.end {
				break // 字节已写满，仅末尾读报错，视为成功
			}

			seg.start = remStart
			seg.attempts++
			if seg.attempts > maxSegmentRetries {
				st.setFailed(err)
				break
			}
			// 交回剩余段，空闲 worker 可抢占接管
			st.requeue(seg)
			break
		}
		st.finish()
	}
}

// DownloadAssetParallel 多线程下载 asset（抢占式段队列 worker 池）。
func (c *Client) DownloadAssetParallel(asset Asset, destPath string, concurrency int, progress func(downloaded, total int64)) error {
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	supportsRange, fileSize, err := c.checkRangeSupport(asset.BrowserDownloadURL)
	if err != nil {
		return c.DownloadAsset(asset, destPath, progress)
	}

	if !supportsRange || fileSize <= 0 {
		return c.DownloadAsset(asset, destPath, progress)
	}

	if fileSize < minChunkSize*2 {
		return c.DownloadAsset(asset, destPath, progress)
	}

	// 预分配目标文件，各 worker 定位写入
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	if err := f.Truncate(fileSize); err != nil {
		f.Close()
		return err
	}
	f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := newDownloadState(fileSize, progress)
	st.enqueueInitial(int64(segSize))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.worker(ctx, asset.BrowserDownloadURL, destPath, st)
		}()
	}
	wg.Wait()

	if err := st.getFailed(); err != nil {
		return err
	}
	return nil
}

// DownloadAssetWithFallback 带多线程回退的下载。
// 多线程本身已内建"抢占接管 + 僵死检测"，只有不可恢复错误或
// 文件不支持 Range 时才回退到单线程下载。
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
