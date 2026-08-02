package state

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// DpkgPackage dpkg status 文件中的包信息
type DpkgPackage struct {
	Name         string
	Status       string
	Version      string
	Homepage     string
	Architecture string
	Provides     []string
}

// OrphanPackage orphan 包信息
type OrphanPackage struct {
	PkgName       string // deb 包名（如 "bat"）
	Version       string
	Owner         string // GitHub owner（从 Homepage 解析，可能为空）
	Repo          string // GitHub repo（从 Homepage 解析，可能为空）
	Homepage      string
	HasGitHub     bool   // 是否能从 Homepage 识别出 GitHub 仓库
	DeepScanError string // 深度扫描的错误信息（如果有）
}

// ScanOrphans 扫描系统中所有 orphan 包
// deepScan: 是否对非 GitHub Homepage 进行深度扫描（抓取页面查找 GitHub 链接）
func ScanOrphans(deepScan bool, progress func(string), pkgFilter string) ([]OrphanPackage, error) {
	// 1. 快速获取所有 orphan 包名
	orphanPkgs, err := getOrphanPackages()
	if err != nil {
		return nil, err
	}
	if len(orphanPkgs) == 0 {
		return nil, nil
	}

	// 2. 解析 dpkg status，建立包名到信息的映射
	allPkgs, err := parseDpkgStatus()
	if err != nil {
		return nil, err
	}
	pkgMap := make(map[string]*DpkgPackage)
	for i := range allPkgs {
		pkgMap[allPkgs[i].Name] = &allPkgs[i]
	}

	// 3. 收集所有 orphan 包信息
	var orphans []OrphanPackage
	githubRepoRegex := regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)

	for _, pkgName := range orphanPkgs {
		// 若指定目标包，仅处理该包
		if pkgFilter != "" && pkgName != pkgFilter {
			continue
		}
		pkg, ok := pkgMap[pkgName]
		if !ok {
			continue
		}

		orphan := OrphanPackage{
			PkgName:  pkgName,
			Version:  pkg.Version,
			Homepage: pkg.Homepage,
		}

		// 尝试从 Homepage 解析 GitHub 仓库
		if pkg.Homepage != "" {
			matches := githubRepoRegex.FindStringSubmatch(pkg.Homepage)
			if matches != nil {
				orphan.Owner = matches[1]
				orphan.Repo = matches[2]
				orphan.HasGitHub = true
			} else if deepScan {
				// 深度扫描：尝试从页面中查找 GitHub 链接
				if progress != nil {
					progress(fmt.Sprintf("🔍 深度扫描 %s (%s)...", pkgName, pkg.Homepage))
				}
				owner, repo, err := FindGitHubFromHomepage(pkg.Homepage)
				if err == nil {
					orphan.Owner = owner
					orphan.Repo = repo
					orphan.HasGitHub = true
				} else {
					orphan.DeepScanError = err.Error()
				}
			}
		}

		orphans = append(orphans, orphan)
	}

	return orphans, nil
}

// getOrphanPackages 快速获取所有 orphan 包名（无 apt 源）
func getOrphanPackages() ([]string, error) {
	cmd := exec.Command("apt", "list", "--installed")
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8") // 确保英文输出
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var orphans []string
	// 匹配 [installed,local] 或中文 [已安装，本地]
	localRegex := regexp.MustCompile(`\[.*local.*\]|\[.*本地.*\]`)

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if !localRegex.MatchString(line) {
			continue
		}
		// 解析包名：格式为 "pkgname/source version arch [status]"
		parts := strings.SplitN(line, "/", 2)
		if len(parts) < 2 {
			continue
		}
		pkgName := strings.TrimSpace(parts[0])
		if pkgName != "" && pkgName != "Listing" {
			orphans = append(orphans, pkgName)
		}
	}

	return orphans, scanner.Err()
}

// parseDpkgStatus 解析 /var/lib/dpkg/status 文件
func parseDpkgStatus() ([]DpkgPackage, error) {
	f, err := os.Open("/var/lib/dpkg/status")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var pkgs []DpkgPackage
	var current *DpkgPackage
	lastField := ""
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current != nil && current.Name != "" {
				pkgs = append(pkgs, *current)
			}
			current = nil
			lastField = ""
			continue
		}

		if strings.HasPrefix(line, "Package: ") {
			current = &DpkgPackage{
				Name: strings.TrimPrefix(line, "Package: "),
			}
			lastField = "pkg"
			continue
		}

		if current == nil {
			continue
		}

		// 续行：把上一字段的换行内容拼接回去（Provides 可能跨行）
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if lastField == "provides" && len(current.Provides) > 0 {
				current.Provides[len(current.Provides)-1] += strings.TrimSpace(line)
			}
			continue
		}

		if strings.HasPrefix(line, "Status: ") {
			current.Status = strings.TrimPrefix(line, "Status: ")
			lastField = "status"
		} else if strings.HasPrefix(line, "Version: ") {
			current.Version = strings.TrimPrefix(line, "Version: ")
			lastField = "version"
		} else if strings.HasPrefix(line, "Homepage: ") {
			current.Homepage = strings.TrimPrefix(line, "Homepage: ")
			lastField = "homepage"
		} else if strings.HasPrefix(line, "Architecture: ") {
			current.Architecture = strings.TrimPrefix(line, "Architecture: ")
			lastField = "arch"
		} else if strings.HasPrefix(line, "Provides: ") {
			current.Provides = splitProvides(strings.TrimPrefix(line, "Provides: "))
			lastField = "provides"
		}
	}

	if current != nil && current.Name != "" {
		pkgs = append(pkgs, *current)
	}

	return pkgs, scanner.Err()
}

// splitProvides 解析 Provides 字段值，支持逗号分隔及 "名 (= 版本)" 形式。
func splitProvides(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		if idx := strings.Index(name, "("); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// virtualToRealPkg 内建虚包→实包映射。
// 用于 dpkg status 未声明 Provides 的已知别名场景，如 fd（命令名）→ fd-find（deb 包名）。
var virtualToRealPkg = map[string]string{
	"fd": "fd-find",
}

// resolveVirtualPkg 将虚包包名解析为实际安装的实包包名。
// 优先级：dpkg status 中 Provides 声明 > 内建别名表；找不到则原样返回。
func resolveVirtualPkg(pkgName string, allPkgs []DpkgPackage) string {
	for i := range allPkgs {
		p := &allPkgs[i]
		if p.Name == pkgName {
			continue
		}
		for _, v := range p.Provides {
			if v == pkgName {
				return p.Name
			}
		}
	}
	if real := virtualToRealPkg[pkgName]; real != "" {
		return real
	}
	return pkgName
}

// MergeOrphansToState 将发现的 orphan 包合并到 state 中
func MergeOrphansToState(st *State, orphans []OrphanPackage) int {
	added := 0
	for _, o := range orphans {
		// 对于有 GitHub 信息的，用 owner/repo 作为 key
		// 对于没有的，用 pkgName 作为 key
		var repoKey string
		if o.HasGitHub {
			repoKey = o.Owner + "/" + o.Repo
		} else {
			repoKey = o.PkgName
		}

		// 检查是否已存在
		if st.Get(repoKey) != nil {
			continue
		}
		// 也检查是否已用 pkgName 存在
		if st.GetByPkgName(o.PkgName) != nil {
			continue
		}

		st.Packages[repoKey] = &PackageRecord{
			PkgName:        o.PkgName,
			Owner:          o.Owner,
			Repo:           o.Repo,
			CurrentVersion: o.Version,
			Arch:           "amd64",
			Removed:        false,
			History: []HistoryEntry{
				{
					Action:    ActionInstall,
					Version:   o.Version,
					Timestamp: "auto-discovered",
				},
			},
			UpdatedAt: "auto-discovered",
		}
		added++
	}
	return added
}

// GetByPkgName 根据包名查找记录
func (s *State) GetByPkgName(pkgName string) *PackageRecord {
	for _, rec := range s.Packages {
		if rec.PkgName == pkgName {
			return rec
		}
	}
	return nil
}
