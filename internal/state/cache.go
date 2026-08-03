// Package state 统一缓存：单文件 /var/cache/ghdeb/cache.json
// 扁平化结构：仅一个 packages 段，从 catalog.toml 枚举，
// 每条记录聚合该包的全部展示/查询信息（已装、版本、可升级等）。
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

// PkgState 单个目录包的完整展示信息（key = catalog 短名，从 catalog.toml 枚举）
type PkgState struct {
	Name             string `json:"name"`                        // catalog 短名
	Installed        bool   `json:"installed"`                   // 系统是否已装
	InstallTime      string `json:"install_time,omitempty"`      // 最近安装/升级时间
	InstalledVersion string `json:"installed_version,omitempty"` // dpkg 实际已装版本
	Repo             string `json:"repo"`                        // owner/reponame
	GitHubVersion    string `json:"github_version,omitempty"`    // GitHub 最新 tag/版本
	Upgradable       bool   `json:"upgradable"`                  // 是否可升级
	Arch             string `json:"arch,omitempty"`              // 目标架构
	PkgFile          string `json:"pkg_file,omitempty"`          // 已下载的 .deb 文件名
}

// Cache 统一缓存结构
type Cache struct {
	UpdatedAt string               `json:"updated_at"` // RFC3339，快照生成时间
	Packages  map[string]*PkgState `json:"packages"`   // key = catalog 短名
}

// LoadCache 从磁盘加载统一缓存
func LoadCache() *Cache {
	c := &Cache{Packages: make(map[string]*PkgState)}
	data, err := os.ReadFile(CachePath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, c)
	if c.Packages == nil {
		c.Packages = make(map[string]*PkgState)
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
func (c *Cache) Get(name string) *PkgState {
	return c.Packages[name]
}

// Set 写入某个包的快照信息
func (c *Cache) Set(name string, pkg *PkgState) {
	c.Packages[name] = pkg
}

// Remove 从快照删除某个包
func (c *Cache) Remove(name string) {
	delete(c.Packages, name)
}

// --- GitHub 最新版本缓存（按 owner/repo 反查 package） ---

// findByName 从 owner/repo 反查对应的 PkgState
func (c *Cache) findByName(owner, repo string) *PkgState {
	key := owner + "/" + repo
	for _, p := range c.Packages {
		if p.Repo == key {
			return p
		}
	}
	return nil
}

// cacheFresh 判断快照是否在 24h TTL 内
func (c *Cache) cacheFresh() bool {
	if c.UpdatedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, c.UpdatedAt)
	if err != nil {
		return false
	}
	return time.Since(t) <= cacheTTL
}

// GetCachedRelease 从缓存获取最新版本号，未命中或过期返回空
func GetCachedRelease(owner, repo string) string {
	if owner == "" || repo == "" {
		return ""
	}
	c := LoadCache()
	if !c.cacheFresh() {
		return ""
	}
	if p := c.findByName(owner, repo); p != nil {
		return p.GitHubVersion
	}
	return ""
}

// SetCachedRelease 将版本号写入缓存
func SetCachedRelease(owner, repo, tagName string) {
	if owner == "" || repo == "" {
		return
	}
	c := LoadCache()
	if p := c.findByName(owner, repo); p != nil {
		p.GitHubVersion = tagName
		_ = SaveCache(c)
	}
}

// InvalidateReleaseCache 清除指定仓库的缓存，空字符串表示清除全部
func InvalidateReleaseCache(owner, repo string) {
	c := LoadCache()
	if owner == "" {
		for _, p := range c.Packages {
			p.GitHubVersion = ""
		}
	} else {
		if p := c.findByName(owner, repo); p != nil {
			p.GitHubVersion = ""
		}
	}
	_ = SaveCache(c)
}

// ClearInstalled 清空各 package 的已装相关字段（apt 钩子在包变更后调用）
// 保留 repo / github_version / arch / pkg_file，只清已装状态
func ClearInstalled() {
	c := LoadCache()
	for _, p := range c.Packages {
		p.Installed = false
		p.InstallTime = ""
		p.InstalledVersion = ""
		p.Upgradable = false
	}
	_ = SaveCache(c)
}

// SudoMkdirAll 用 sudo 创建系统级缓存目录
func SudoMkdirAll(path string) error {
	return sudoMkdirAll(path)
}

// SudoRemove 用 sudo 删除文件（清理 root 拥有的缓存）
func SudoRemove(path string) error {
	return sudoRemove(path)
}
