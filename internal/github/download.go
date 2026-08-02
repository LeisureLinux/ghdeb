// Package github 多线程下载实现
package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"github.com/leisurelinux/ghdeb/internal/i18n"
)

// 下载配置
const (
	defaultConcurrency = 4    // 默认并发数
	minChunkSize       = 1 << 20 // 最小块大小 1MB
	chunkSize          = 5 << 20 // 每块大小 5MB
)

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

// downloadChunk 下载单个块
func (c *Client) downloadChunk(ctx context.Context, downloadURL string, start, end int64, destPath string) error {
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

	_, err = io.Copy(f, resp.Body)
	return err
}

// DownloadAssetParallel 多线程下载 asset
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

	// 计算块
	var chunks []struct{ start, end int64 }
	for start := int64(0); start < fileSize; start += chunkSize {
		end := start + chunkSize - 1
		if end >= fileSize {
			end = fileSize - 1
		}
		chunks = append(chunks, struct{ start, end int64 }{start, end})
	}

	// 并发下载
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var downloaded int64
	var wg sync.WaitGroup
	errCh := make(chan error, len(chunks))

	// 限制并发数
	sem := make(chan struct{}, concurrency)

	for _, chunk := range chunks {
		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			if err := c.downloadChunk(ctx, asset.BrowserDownloadURL, start, end, destPath); err != nil {
				errCh <- err
				cancel()
				return
			}

			chunkSize := end - start + 1
			atomic.AddInt64(&downloaded, chunkSize)
			if progress != nil {
				progress(atomic.LoadInt64(&downloaded), fileSize)
			}
		}(chunk.start, chunk.end)
	}

	wg.Wait()
	close(errCh)

	// 检查错误
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

// DownloadAssetWithFallback 带多线程回退的下载
func (c *Client) DownloadAssetWithFallback(asset Asset, destPath string, progress func(downloaded, total int64)) error {
	// 尝试多线程下载
	err := c.DownloadAssetParallel(asset, destPath, defaultConcurrency, progress)
	if err != nil {
		// 如果多线程失败，回退到单线程
		fmt.Fprintf(os.Stderr,
			i18n.T("⚠️  多线程下载失败，回退到单线程: %v\n",
				"⚠️  Parallel download failed, falling back to single thread: %v\n"), err)
		return c.DownloadAsset(asset, destPath, progress)
	}
	return nil
}
