// Package github - release 信息本地缓存
package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// 缓存默认有效期
const cacheTTL = 24 * time.Hour

// cacheEntry 单条缓存记录
type cacheEntry struct {
	TagName   string `json:"tag_name"`
	FetchedAt string `json:"fetched_at"` // RFC3339
}

// cacheData 缓存文件结构
type cacheData struct {
	Releases map[string]*cacheEntry `json:"releases"` // key = "owner/repo"
}

// cachePath 返回缓存文件路径 ~/.cache/ghdeb/releases.json
func cachePath() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "ghdeb", "releases.json")
}

// loadCache 从磁盘加载缓存
func loadCache() *cacheData {
	cd := &cacheData{Releases: make(map[string]*cacheEntry)}
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return cd
	}
	_ = json.Unmarshal(data, cd)
	if cd.Releases == nil {
		cd.Releases = make(map[string]*cacheEntry)
	}
	return cd
}

// saveCache 保存缓存到磁盘
func saveCache(cd *cacheData) error {
	path := cachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetCachedRelease 从缓存获取最新版本号，过期返回空
func (c *Client) GetCachedRelease(owner, repo string) string {
	cd := loadCache()
	key := owner + "/" + repo
	entry, ok := cd.Releases[key]
	if !ok {
		return ""
	}
	// 检查是否过期
	fetchedAt, err := time.Parse(time.RFC3339, entry.FetchedAt)
	if err != nil || time.Since(fetchedAt) > cacheTTL {
		return ""
	}
	return entry.TagName
}

// SetCachedRelease 将版本号写入缓存
func (c *Client) SetCachedRelease(owner, repo, tagName string) {
	cd := loadCache()
	key := owner + "/" + repo
	cd.Releases[key] = &cacheEntry{
		TagName:   tagName,
		FetchedAt: time.Now().Format(time.RFC3339),
	}
	_ = saveCache(cd)
}

// InvalidateCache 清除指定仓库的缓存，空字符串表示清除全部
func InvalidateCache(owner, repo string) {
	cd := loadCache()
	if owner == "" {
		// 清除全部
		cd.Releases = make(map[string]*cacheEntry)
	} else {
		delete(cd.Releases, owner+"/"+repo)
	}
	_ = saveCache(cd)
}
