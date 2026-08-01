package state

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// FindGitHubFromHomepage 从 Homepage 页面中查找 GitHub 链接
func FindGitHubFromHomepage(homepage string) (owner, repo string, err error) {
	// 创建带超时的 HTTP 客户端
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 发送 HEAD 请求检查是否可访问
	resp, err := client.Head(homepage)
	if err != nil {
		return "", "", fmt.Errorf("HEAD 请求失败: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	// 发送 GET 请求获取页面内容
	resp, err = client.Get(homepage)
	if err != nil {
		return "", "", fmt.Errorf("GET 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 限制读取大小，避免下载过大页面
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB
	if err != nil {
		return "", "", fmt.Errorf("读取页面失败: %w", err)
	}

	// 提取所有 GitHub 链接
	githubRepoRegex := regexp.MustCompile(`https?://github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)
	matches := githubRepoRegex.FindAllStringSubmatch(string(body), -1)

	if len(matches) == 0 {
		return "", "", fmt.Errorf("页面中未找到 GitHub 链接")
	}

	// 去重并过滤
	seen := make(map[string]bool)
	var candidates []struct{ owner, repo string }

	for _, match := range matches {
		owner, repo := match[1], match[2]
		key := owner + "/" + repo

		// 过滤掉一些明显不相关的
		if strings.Contains(repo, "actions") ||
			strings.Contains(repo, "sponsors") ||
			strings.Contains(owner, "features") {
			continue
		}

		if !seen[key] {
			seen[key] = true
			candidates = append(candidates, struct{ owner, repo string }{owner, repo})
		}
	}

	if len(candidates) == 0 {
		return "", "", fmt.Errorf("未找到有效的 GitHub 仓库链接")
	}

	// 简单启发式：优先选择与包名相关的
	// 这里暂时返回第一个，后续可以优化
	return candidates[0].owner, candidates[0].repo, nil
}
