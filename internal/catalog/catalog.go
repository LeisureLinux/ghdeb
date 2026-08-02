// Package catalog 提供包目录管理，支持短名称查找、搜索和写入
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
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

// Load 加载包目录（合并系统和用户目录）
func Load() (*Catalog, error) {
	cat := &Catalog{Packages: make(map[string]*CatalogEntry)}

	// 加载系统目录
	sysPath := SystemCatalogPath()
	if err := cat.loadFile(sysPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("加载系统目录失败: %w", err)
	}

	// 加载用户目录（覆盖同名条目）
	userPath := UserCatalogPath()
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

// --- 用户目录写入支持 ---

// AddToUserCatalog 向用户目录添加/覆盖条目
func AddToUserCatalog(name string, entry *CatalogEntry) error {
	if err := ValidateCatalogName(name); err != nil {
		return err
	}
	if err := ValidateCatalogEntry(entry); err != nil {
		return err
	}

	userPath := UserCatalogPath()

	// 加载现有用户目录
	userEntries := make(map[string]CatalogEntry)
	if data, err := os.ReadFile(userPath); err == nil {
		toml.Decode(string(data), &userEntries)
	}

	// 添加/覆盖
	userEntries[name] = *entry

	// 写回
	return writeUserCatalog(userPath, userEntries)
}

// DeleteFromUserCatalog 从用户目录删除条目
func DeleteFromUserCatalog(name string) error {
	userPath := UserCatalogPath()

	userEntries := make(map[string]CatalogEntry)
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("用户目录不存在，无法删除 %s", name)
		}
		return err
	}
	toml.Decode(string(data), &userEntries)

	if _, ok := userEntries[name]; !ok {
		return fmt.Errorf("用户目录中未找到 %s", name)
	}

	delete(userEntries, name)
	return writeUserCatalog(userPath, userEntries)
}

// LoadUserCatalog 仅加载用户目录
func LoadUserCatalog() (map[string]CatalogEntry, error) {
	userPath := UserCatalogPath()
	entries := make(map[string]CatalogEntry)
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	if _, err := toml.Decode(string(data), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// writeUserCatalog 写入用户目录文件
func writeUserCatalog(path string, entries map[string]CatalogEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# ghdeb 用户自定义包目录\n")
	sb.WriteString("# 由 ghdeb catalog add/delete 命令管理\n")
	sb.WriteString("# 同名条目覆盖系统目录 (/usr/share/ghdeb/catalog.toml)\n\n")

	// 按名称排序输出
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := entries[name]
		sb.WriteString(fmt.Sprintf("[%s]\n", name))
		if entry.Repo != "" {
			sb.WriteString(fmt.Sprintf("repo = %q\n", entry.Repo))
		}
		if entry.PrettyName != "" {
			sb.WriteString(fmt.Sprintf("pretty_name = %q\n", entry.PrettyName))
		}
		if entry.Website != "" {
			sb.WriteString(fmt.Sprintf("website = %q\n", entry.Website))
		}
		if entry.Summary != "" {
			sb.WriteString(fmt.Sprintf("summary = %q\n", entry.Summary))
		}
		if entry.URL != "" {
			sb.WriteString(fmt.Sprintf("url = %q\n", entry.URL))
		}
		if entry.GPGKey != "" {
			sb.WriteString(fmt.Sprintf("gpg_key = %q\n", entry.GPGKey))
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// --- 校验函数 ---

// catalogNameRe 合法的目录短名称：小写字母、数字、连字符、下划线
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
	// 如果指定了 repo，校验格式
	if entry.Repo != "" {
		parts := strings.SplitN(entry.Repo, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("repo 格式错误，应为 owner/repo")
		}
	}
	// 如果指定了 URL，校验基本格式
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
	return "/usr/share/ghdeb/catalog.toml"
}

// UserCatalogPath 返回用户目录路径
func UserCatalogPath() string {
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
