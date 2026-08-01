// Package catalog 提供包目录管理，支持短名称查找和搜索
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// CatalogEntry 包目录条目
type CatalogEntry struct {
	Repo       string `toml:"repo"`
	PrettyName string `toml:"pretty_name,omitempty"`
	Website    string `toml:"website,omitempty"`
	Summary    string `toml:"summary,omitempty"`
}

// Catalog 包目录
type Catalog struct {
	Packages map[string]*CatalogEntry // key = 短名称
}

// Load 加载包目录（合并系统和用户目录）
func Load() (*Catalog, error) {
	cat := &Catalog{Packages: make(map[string]*CatalogEntry)}

	// 加载系统目录
	sysPath := systemCatalogPath()
	if err := cat.loadFile(sysPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("加载系统目录失败: %w", err)
	}

	// 加载用户目录（覆盖同名条目）
	userPath := userCatalogPath()
	if err := cat.loadFile(userPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("加载用户目录失败: %w", err)
	}

	return cat, nil
}

// loadFile 从单个文件加载
func (c *Catalog) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// 解析 TOML
	var raw map[string]CatalogEntry
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("解析 TOML 失败: %w", err)
	}

	// 合并到 catalog
	for name, entry := range raw {
		c.Packages[name] = &entry
	}

	return nil
}

// Lookup 按短名称查找
func (c *Catalog) Lookup(name string) *CatalogEntry {
	return c.Packages[name]
}

// Search 搜索包（支持正则）
func (c *Catalog) Search(pattern string) ([]*SearchResult, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("无效的正则表达式: %w", err)
	}

	var results []*SearchResult
	for name, entry := range c.Packages {
		// 搜索 name、pretty_name、summary
		if re.MatchString(name) ||
			re.MatchString(entry.PrettyName) ||
			re.MatchString(entry.Summary) {
			results = append(results, &SearchResult{
				Name:  name,
				Entry: entry,
			})
		}
	}

	return results, nil
}

// SearchResult 搜索结果
type SearchResult struct {
	Name  string
	Entry *CatalogEntry
}

// AllEntries 返回所有条目
func (c *Catalog) AllEntries() map[string]*CatalogEntry {
	return c.Packages
}

// systemCatalogPath 返回系统目录路径
func systemCatalogPath() string {
	return "/usr/share/ghdeb/catalog.toml"
}

// userCatalogPath 返回用户目录路径
func userCatalogPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ghdeb", "catalog.toml")
}

// FormatSearchResult 格式化搜索结果
func FormatSearchResult(r *SearchResult, installed bool) string {
	status := ""
	if installed {
		status = " [已安装]"
	}
	return fmt.Sprintf("%s (%s)%s — %s", r.Name, r.Entry.Repo, status, r.Entry.Summary)
}

// FormatEntry 格式化条目信息
func FormatEntry(name string, entry *CatalogEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("名称: %s\n", name))
	sb.WriteString(fmt.Sprintf("仓库: %s\n", entry.Repo))
	if entry.PrettyName != "" {
		sb.WriteString(fmt.Sprintf("显示名: %s\n", entry.PrettyName))
	}
	if entry.Website != "" {
		sb.WriteString(fmt.Sprintf("网站: %s\n", entry.Website))
	}
	if entry.Summary != "" {
		sb.WriteString(fmt.Sprintf("简介: %s\n", entry.Summary))
	}
	return sb.String()
}
