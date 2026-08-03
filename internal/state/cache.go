// Package state 统一缓存：单文件 /var/cache/ghdeb/cache.json
// 同时承载 releases（GitHub 最新版本）、installed（已装版本）、
// packages（ghdeb list 展示快照），避免散落多个 json。
// ghdeb update 只读写这一个文件；apt 钩子也直接更新它。
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// 缓存默认有效期
const cacheTTL = 24 * time.Hour

// CacheDir 返回系统级缓存目录（root 写，其他只读）
// 可用环境变量 GHDEB_CACHE_DIR 覆盖（主要用于隔离测试）
func CacheDir() string {
	if d := os.Getenv("GHDEB_CACHE_DIR"); d != "" {
		return d
	}
	return "/var/cache/ghdeb"
}

// CachePath 返回统一缓存文件路径
func CachePath() string {
	return filepath.Join(CacheDir(), "cache.json")
}

// ReleaseEntry 单条 GitHub release 缓存记录
type ReleaseEntry struct {
	TagName   string `json:"tag_name"`
	FetchedAt string `json:"fetched_at"` // RFC3339
}

// InstalledEntry 单条已装版本缓存记录
type InstalledEntry struct {
	Version   string `json:"version"`
	FetchedAt string `json:"fetched_at"` // RFC3339
}

// SnapshotPkg 单个包的 list 展示信息
type SnapshotPkg struct {
	Repo             string `json:"repo,omitempty"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	Upgradeable      bool   `json:"upgradeable"`
}

// Cache 统一缓存结构
type Cache struct {
	UpdatedAt string                     `json:"updated_at"` // RFC3339，快照时间
	Releases  map[string]*ReleaseEntry   `json:"releases"`   // key = "owner/repo"
	Installed map[string]*InstalledEntry `json:"installed"`  // key = deb 包名
	Packages  map[string]*SnapshotPkg    `json:"packages"`   // key = catalog 短名称
}

// LoadCache 从磁盘加载统一缓存
func LoadCache() *Cache {
	c := &Cache{
		Releases:  make(map[string]*ReleaseEntry),
		Installed: make(map[string]*InstalledEntry),
		Packages:  make(map[string]*SnapshotPkg),
	}
	data, err := os.ReadFile(CachePath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, c)
	if c.Releases == nil {
		c.Releases = make(map[string]*ReleaseEntry)
	}
	if c.Installed == nil {
		c.Installed = make(map[string]*InstalledEntry)
	}
	if c.Packages == nil {
		c.Packages = make(map[string]*SnapshotPkg)
	}
	return c
}

// SaveCache 保存统一缓存（权限不足时自动 sudo 写入）
func SaveCache(c *Cache) error {
	dir := filepath.Dir(CachePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		if err := sudoMkdirAll(dir); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(CachePath(), data, 0644); err != nil {
		return sudoWriteFile(CachePath(), data)
	}
	return nil
}

// --- GitHub release 缓存 ---

// GetCachedRelease 从缓存获取最新版本号，未命中或过期返回空
func GetCachedRelease(owner, repo string) string {
	if owner == "" || repo == "" {
		return ""
	}
	c := LoadCache()
	entry, ok := c.Releases[owner+"/"+repo]
	if !ok {
		return ""
	}
	fetchedAt, err := time.Parse(time.RFC3339, entry.FetchedAt)
	if err != nil || time.Since(fetchedAt) > cacheTTL {
		return ""
	}
	return entry.TagName
}

// SetCachedRelease 将版本号写入缓存
func SetCachedRelease(owner, repo, tagName string) {
	if owner == "" || repo == "" {
		return
	}
	c := LoadCache()
	c.Releases[owner+"/"+repo] = &ReleaseEntry{
		TagName:   tagName,
		FetchedAt: time.Now().Format(time.RFC3339),
	}
	_ = SaveCache(c)
}

// InvalidateReleaseCache 清除指定仓库的缓存，空字符串表示清除全部
func InvalidateReleaseCache(owner, repo string) {
	c := LoadCache()
	if owner == "" {
		c.Releases = make(map[string]*ReleaseEntry)
	} else {
		delete(c.Releases, owner+"/"+repo)
	}
	_ = SaveCache(c)
}

// --- 已装版本缓存 ---

// GetCachedInstalled 从缓存获取已装版本号，未命中或过期返回空
func GetCachedInstalled(pkgName string) string {
	if pkgName == "" {
		return ""
	}
	c := LoadCache()
	entry, ok := c.Installed[pkgName]
	if !ok {
		return ""
	}
	fetchedAt, err := time.Parse(time.RFC3339, entry.FetchedAt)
	if err != nil || time.Since(fetchedAt) > cacheTTL {
		return ""
	}
	return entry.Version
}

// SetCachedInstalled 将已装版本号写入缓存
func SetCachedInstalled(pkgName, version string) {
	if pkgName == "" {
		return
	}
	c := LoadCache()
	c.Installed[pkgName] = &InstalledEntry{
		Version:   version,
		FetchedAt: time.Now().Format(time.RFC3339),
	}
	_ = SaveCache(c)
}

// ClearCachedInstalled 清空全部已装版本缓存（apt 钩子在包变更后调用）
func ClearCachedInstalled() {
	c := LoadCache()
	c.Installed = make(map[string]*InstalledEntry)
	_ = SaveCache(c)
}

// --- list 快照 ---

// SortedNames 返回快照中的排序名称列表
func (c *Cache) SortedNames() []string {
	names := make([]string, 0, len(c.Packages))
	for n := range c.Packages {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Get 获取某个包的快照信息
func (c *Cache) Get(name string) *SnapshotPkg {
	return c.Packages[name]
}

// Set 写入某个包的快照信息
func (c *Cache) Set(name string, pkg *SnapshotPkg) {
	c.Packages[name] = pkg
}

// Remove 从快照删除某个包
func (c *Cache) Remove(name string) {
	delete(c.Packages, name)
}

// SudoMkdirAll 用 sudo 创建系统级缓存目录
func SudoMkdirAll(path string) error {
	return sudoMkdirAll(path)
}

// SudoRemove 用 sudo 删除文件（清理 root 拥有的缓存）
func SudoRemove(path string) error {
	return sudoRemove(path)
}
