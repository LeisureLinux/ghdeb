// Package catalog 提供包目录管理，仅支持系统级配置 /etc/ghdeb/catalog.toml
package catalog

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// CatalogEntry 包目录条目
type CatalogEntry struct {
	Repo       string `toml:"repo,omitempty"`
	PrettyName string `toml:"pretty_name,omitempty"`
	Website    string `toml:"website,omitempty"`
	Summary    string `toml:"summary,omitempty"`
	URL        string `toml:"url,omitempty"`        // 非 GitHub 来源的直接下载 URL 模板
	GPGKey     string `toml:"gpg_key,omitempty"`     // GPG 公钥 URL
}

// Catalog 包目录
type Catalog struct {
	Packages map[string]*CatalogEntry // key = 短名称
}

const defaultSystemCatalogPath = "/etc/ghdeb/catalog.toml"

// Load 加载系统目录 /etc/ghdeb/catalog.toml
func Load() (*Catalog, error) {
	cat := &Catalog{Packages: make(map[string]*CatalogEntry)}
	path := SystemCatalogPath()
	if err := cat.loadFile(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("加载目录失败: %w", err)
	}
	return cat, nil
}

// loadFile 从单个 TOML 文件加载
func (c *Catalog) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]CatalogEntry
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("解析 TOML 失败: %w", err)
	}

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

// SortedNames 返回排序后的名称列表
func (c *Catalog) SortedNames() []string {
	names := make([]string, 0, len(c.Packages))
	for name := range c.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// --- 校验函数 ---

var catalogNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]*$`)

// ValidateCatalogName 校验目录短名称
func ValidateCatalogName(name string) error {
	if name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if len(name) > 64 {
		return fmt.Errorf("名称过长（最多 64 字符）")
	}
	if !catalogNameRe.MatchString(name) {
		return fmt.Errorf("名称 %q 不合法，仅允许小写字母、数字、连字符、下划线、点号", name)
	}
	return nil
}

// ValidateCatalogEntry 校验目录条目
func ValidateCatalogEntry(entry *CatalogEntry) error {
	if entry.Repo == "" && entry.URL == "" {
		return fmt.Errorf("至少需要指定 --repo 或 --url 之一")
	}
	if entry.Repo != "" {
		parts := strings.SplitN(entry.Repo, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("repo 格式错误，应为 owner/repo")
		}
	}
	if entry.URL != "" {
		if !strings.HasPrefix(entry.URL, "https://") && !strings.HasPrefix(entry.URL, "http://") {
			return fmt.Errorf("url 必须以 https:// 或 http:// 开头")
		}
	}
	return nil
}

// --- 路径函数 ---

// SystemCatalogPath 返回系统目录路径
func SystemCatalogPath() string {
	if path := os.Getenv("GHCETALOG_PATH"); path != "" {
		return path
	}
	return defaultSystemCatalogPath
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
	if entry.Repo != "" {
		sb.WriteString(fmt.Sprintf("仓库: %s\n", entry.Repo))
	}
	if entry.URL != "" {
		sb.WriteString(fmt.Sprintf("URL: %s\n", entry.URL))
	}
	if entry.PrettyName != "" {
		sb.WriteString(fmt.Sprintf("显示名: %s\n", entry.PrettyName))
	}
	if entry.Website != "" {
		sb.WriteString(fmt.Sprintf("网站: %s\n", entry.Website))
	}
	if entry.Summary != "" {
		sb.WriteString(fmt.Sprintf("简介: %s\n", entry.Summary))
	}
	if entry.GPGKey != "" {
		sb.WriteString(fmt.Sprintf("GPG 密钥: %s\n", entry.GPGKey))
	}
	return sb.String()
}

// IsDirectURL 判断条目是否为直接 URL 来源（非 GitHub）
func (e *CatalogEntry) IsDirectURL() bool {
	return e.URL != "" && e.Repo == ""
}

// RepoSet 返回所有 repo 的集合（小写 owner/repo）
func (c *Catalog) RepoSet() map[string]bool {
	set := make(map[string]bool)
	for _, entry := range c.Packages {
		if entry.Repo != "" {
			set[strings.ToLower(entry.Repo)] = true
		}
	}
	return set
}

// HasRepo 检查是否有某个 repo
func (c *Catalog) HasRepo(repo string) bool {
	repo = strings.ToLower(repo)
	for _, entry := range c.Packages {
		if strings.ToLower(entry.Repo) == repo {
			return true
		}
	}
	return false
}

// HasName 检查是否有某个短名称
func (c *Catalog) HasName(name string) bool {
	_, ok := c.Packages[name]
	return ok
}
