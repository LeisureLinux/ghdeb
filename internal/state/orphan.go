package state

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// DpkgPackage dpkg status 文件中的包信息
type DpkgPackage struct {
	Name         string
	Status       string
	Version      string
	Homepage     string
	Architecture string
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
func ScanOrphans(deepScan bool, progress func(string)) ([]OrphanPackage, error) {
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
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current != nil && current.Name != "" {
				pkgs = append(pkgs, *current)
			}
			current = nil
			continue
		}

		if strings.HasPrefix(line, "Package: ") {
			current = &DpkgPackage{
				Name: strings.TrimPrefix(line, "Package: "),
			}
			continue
		}

		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "Status: ") {
			current.Status = strings.TrimPrefix(line, "Status: ")
		} else if strings.HasPrefix(line, "Version: ") {
			current.Version = strings.TrimPrefix(line, "Version: ")
		} else if strings.HasPrefix(line, "Homepage: ") {
			current.Homepage = strings.TrimPrefix(line, "Homepage: ")
		} else if strings.HasPrefix(line, "Architecture: ") {
			current.Architecture = strings.TrimPrefix(line, "Architecture: ")
		}
	}

	if current != nil && current.Name != "" {
		pkgs = append(pkgs, *current)
	}

	return pkgs, scanner.Err()
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

// SetRepo 为包设置仓库信息
func (s *State) SetRepo(pkgName, owner, repo string) bool {
	// 先找到这个包
	rec := s.GetByPkgName(pkgName)
	if rec == nil {
		return false
	}

	// 找到旧的 key
	var oldKey string
	for k, v := range s.Packages {
		if v == rec {
			oldKey = k
			break
		}
	}

	// 更新记录
	rec.Owner = owner
	rec.Repo = repo

	// 如果 key 需要改变（从 pkgName 改为 owner/repo）
	newKey := owner + "/" + repo
	if oldKey != newKey {
		// 删除旧 key
		delete(s.Packages, oldKey)
		// 添加新 key
		s.Packages[newKey] = rec
	}

	return true
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

// AptGitHubPackage apt 安装的 GitHub 包信息
type AptGitHubPackage struct {
	PkgName  string // deb 包名
	Version  string
	Owner    string // GitHub owner
	Repo     string // GitHub repo
	Homepage string
	InCatalog bool  // 是否已在 catalog 中
}

// ScanAptGitHubPackages 扫描所有 apt 已安装包，找出 Homepage 指向 github.com 的
func ScanAptGitHubPackages(catalogRepos map[string]bool) ([]AptGitHubPackage, error) {
	// 解析所有已安装的包
	allPkgs, err := parseDpkgStatus()
	if err != nil {
		return nil, err
	}

	githubRepoRegex := regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)

	var ghPkgs []AptGitHubPackage
	seen := make(map[string]bool) // 避免重复

	for _, pkg := range allPkgs {
		// 只处理已安装的包
		if !strings.Contains(pkg.Status, "installed") || strings.Contains(pkg.Status, "not-installed") {
			continue
		}

		if pkg.Homepage == "" {
			continue
		}

		matches := githubRepoRegex.FindStringSubmatch(pkg.Homepage)
		if matches == nil {
			continue
		}

		owner := matches[1]
		repo := matches[2]
		repoKey := strings.ToLower(owner + "/" + repo)

		// 跳过重复的 repo（同一个 repo 可能被多个 deb 包安装）
		if seen[repoKey] {
			continue
		}
		seen[repoKey] = true

		// 跳过 github.com 自身的元链接（如 gh-pages 等）
		if owner == "github" || repo == "github" {
			continue
		}

		// 清理 repo 名称（去除 .git 后缀、大小写规范化）
		repo = strings.TrimSuffix(repo, ".git")

		inCatalog := catalogRepos[strings.ToLower(owner+"/"+repo)]

		ghPkgs = append(ghPkgs, AptGitHubPackage{
			PkgName:   pkg.Name,
			Version:   pkg.Version,
			Owner:     owner,
			Repo:      repo,
			Homepage:  pkg.Homepage,
			InCatalog: inCatalog,
		})
	}

	// 按包名排序
	sort.Slice(ghPkgs, func(i, j int) bool {
		return ghPkgs[i].PkgName < ghPkgs[j].PkgName
	})

	return ghPkgs, nil
}
