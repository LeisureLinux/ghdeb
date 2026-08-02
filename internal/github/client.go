// Package github 提供 GitHub Releases API 客户端
package github

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/leisurelinux/ghdeb/internal/config"
	"github.com/leisurelinux/ghdeb/internal/i18n"
)

// 下载重试配置
const (
	maxRetries    = 3               // 最大重试次数
	retryBaseWait = 2 * time.Second // 重试基础等待时间
	apiTimeout    = 30 * time.Second // API 请求超时
)

// Release 表示一个 GitHub Release
type Release struct {
	HTMLURL    string  `json:"html_url"`
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Prerelease bool    `json:"prerelease"`
	Draft      bool    `json:"draft"`
	Assets     []Asset `json:"assets"`
}

// Asset 表示 Release 中的一个文件
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// Client GitHub API 客户端
type Client struct {
	apiClient      *http.Client // API 请求（短超时）
	downloadClient *http.Client // 下载请求（仅连接超时，无全局超时）
	token          string
}

// getProxyFunc 返回代理函数，优先使用环境变量，其次使用配置文件
func getProxyFunc() func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		proxyStr := config.GetProxy()
		if proxyStr == "" {
			return nil, nil
		}
		return url.Parse(proxyStr)
	}
}

// getGhCliToken 尝试从 gh CLI 获取 token
func getGhCliToken() string {
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// newTransport 构建下载/API 传输层。
// 关键点：显式禁用 HTTP/2（TLSNextProto 置为非 nil 空 map）。
// 在走代理 + GitHub release-assets 场景下，HTTP/2 over CONNECT 隧道
// 容易卡住响应头（http2: timeout awaiting response headers），
// 强制 HTTP/1.1 后下载稳定得多。
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: getProxyFunc(), // 统一代理：环境变量 + 配置文件
		// TCP 拨号超时：弱代理建连本身就可能极慢或挂起，必须设上限。
		// 否则 Transport 的 DialContext 默认无超时，会无限期卡在 connect。
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// 仅设置连接/响应头超时，不设全局超时，避免大文件被固定超时卡死。
		// 慢代理（如 wpad.lan:8888）首次建连可达 8~15s，超时值需放宽避免误杀。
		ResponseHeaderTimeout: 60 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   16,
		// 禁用 HTTP/2，强制 HTTP/1.1
		ForceAttemptHTTP2: false,
		TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
}

// NewClient 创建客户端，自动从参数或环境变量获取 token
func NewClient() *Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		// 尝试从 gh CLI 获取 token
		token = getGhCliToken()
	}
	// API 与下载共用一套禁用了 HTTP/2 的传输层，保证代理环境下连接稳定
	transport := newTransport()
	return &Client{
		// API 客户端：30s 全局超时（JSON 响应很小）
		apiClient: &http.Client{
			Timeout:   apiTimeout,
			Transport: transport,
		},
		// 下载客户端：仅设置连接/响应头超时，不设全局超时
		// 这样大文件下载不会被固定超时卡死
		downloadClient: &http.Client{
			Transport: transport,
		},
		token: token,
	}
}

// GetLatestRelease 获取最新 release（排除 draft 和 prerelease）
func (c *Client) GetLatestRelease(owner, repo string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.apiClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w",
			i18n.T("请求 GitHub API 失败", "GitHub API request failed"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf(
			i18n.T("仓库 %s/%s 不存在或无权限访问", "repo %s/%s does not exist or is inaccessible"),
			owner, repo)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf(
			i18n.T("GitHub API 返回 %d: %s", "GitHub API returned %d: %s"),
			resp.StatusCode, string(body))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("%s: %w",
			i18n.T("解析响应失败", "failed to parse response"), err)
	}

	// 找到第一个非 draft、非 prerelease 的版本
	for _, r := range releases {
		if !r.Draft && !r.Prerelease {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("%s",
		i18n.T("未找到稳定版本（所有 release 均为 draft 或 prerelease）",
			"no stable version found (all releases are draft or prerelease)"))
}

// GetReleaseByTag 获取指定 tag 的 release
func (c *Client) GetReleaseByTag(owner, repo, tag string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.apiClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w",
			i18n.T("请求 GitHub API 失败", "GitHub API request failed"), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf(
			i18n.T("tag %s 不存在", "tag %s does not exist"), tag)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf(
			i18n.T("GitHub API 返回 %d: %s", "GitHub API returned %d: %s"),
			resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("%s: %w",
			i18n.T("解析响应失败", "failed to parse response"), err)
	}
	return &release, nil
}

// DownloadAsset 下载 asset 到指定路径，带重试、断点续传和进度回调
func (c *Client) DownloadAsset(asset Asset, destPath string, progress func(downloaded, total int64)) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			wait := retryBaseWait * time.Duration(1<<(attempt-1)) // 指数退避: 2s, 4s, 8s
			fmt.Fprintf(os.Stderr,
				i18n.T("⏳ 下载失败，%v 后重试 (%d/%d)...\n",
					"⏳ Download failed, retrying in %v (%d/%d)...\n"),
				wait, attempt, maxRetries)
			time.Sleep(wait)
		}

		err := c.downloadOnce(asset, destPath, progress)
		if err == nil {
			return nil
		}
		lastErr = err

		// 不可重试的错误直接返回
		if !isRetryable(err) {
			return err
		}
	}

	return fmt.Errorf(
		i18n.T("下载失败（重试 %d 次）: %w",
			"download failed after %d retries: %w"),
		maxRetries, lastErr)
}

// downloadOnce 单次下载尝试，支持断点续传
func (c *Client) downloadOnce(asset Asset, destPath string, progress func(downloaded, total int64)) error {
	req, err := http.NewRequest("GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	// 断点续传：检查已下载的部分
	var existingSize int64
	if fi, statErr := os.Stat(destPath); statErr == nil {
		existingSize = fi.Size()
		if existingSize > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
		}
	}

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w",
			i18n.T("下载失败", "download failed"), err)
	}
	defer resp.Body.Close()

	// 206 = 服务器支持断点续传，从已有位置继续
	// 200 = 服务器不支持 Range，需要重新下载
	// 416 = Range Not Satisfiable，本地文件可能已完整或损坏，需要删除后重新下载
	if resp.StatusCode == 416 {
		// 删除本地文件，重新下载
		os.Remove(destPath)
		return fmt.Errorf("%s",
			i18n.T("断点续传失败，重新下载", "range not satisfiable, restarting download"))
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf(
			i18n.T("下载返回 HTTP %d", "download returned HTTP %d"),
			resp.StatusCode)
	}

	// 确定写入模式
	resuming := resp.StatusCode == 206 && existingSize > 0
	var f *os.File
	if resuming {
		f, err = os.OpenFile(destPath, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		// 服务器不支持续传或没有已有文件，重新下载
		existingSize = 0
		f, err = os.Create(destPath)
	}
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	if resuming {
		total += existingSize
	}

	var downloaded int64 = existingSize
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := f.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

// isRetryable 判断错误是否可重试
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// 超时、连接重置、EOF 等网络错误可重试
	retryablePatterns := []string{
		"timeout", "deadline exceeded", "cancellation",
		"connection reset", "connection refused",
		"EOF", "broken pipe", "no such host",
		"TLS handshake", "i/o timeout", "range not satisfiable", "断点续传失败",
	}
	for _, p := range retryablePatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	// HTTP 5xx 也可重试
	if strings.Contains(msg, "HTTP 5") {
		return true
	}
	return false
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	// 避免 API 限流的友好标识
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// ParseRepo 解析 "owner/repo" 格式
func ParseRepo(s string) (owner, repo string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf(
			i18n.T("仓库格式应为 owner/repo，当前输入: %q",
				"repo format should be owner/repo, got: %q"), s)
	}
	return parts[0], parts[1], nil
}
