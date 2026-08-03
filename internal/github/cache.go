// Package github - release 信息本地缓存
// 统一缓存已迁移至 /var/cache/ghdeb/cache.json（见 internal/state/cache.go），
// 此处仅保留 Client 方法签名以兼容调用方。
package github

import "github.com/leisurelinux/ghdeb/internal/state"

// GetCachedRelease 从统一缓存获取最新版本号，未命中或过期返回空
func (c *Client) GetCachedRelease(owner, repo string) string {
	return state.GetCachedRelease(owner, repo)
}

// SetCachedRelease 将版本号写入统一缓存
func (c *Client) SetCachedRelease(owner, repo, tagName string) {
	state.SetCachedRelease(owner, repo, tagName)
}

// InvalidateCache 清除指定仓库的缓存，空字符串表示清除全部
func InvalidateCache(owner, repo string) {
	state.InvalidateReleaseCache(owner, repo)
}
