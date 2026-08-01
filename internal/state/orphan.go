package state

import (
	"bufio"
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
}

// GitHubOrphan 来自 GitHub 的 orphan 包
type GitHubOrphan struct {
	PkgName  string // deb 包名（如 "bat"）
	Version  string
	Owner    string // GitHub owner（从 Homepage 解析）
	Repo     string // GitHub repo（从 Homepage 解析）
	Homepage string
}

// ScanGitHubOrphans 扫描系统中的 GitHub 来源 orphan 包
func ScanGitHubOrphans() ([]GitHubOrphan, error) {
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

	// 3. 对 orphan 包检查 Homepage 是否包含 GitHub
	var orphans []GitHubOrphan
	githubRepoRegex := regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)

	for _, pkgName := range orphanPkgs {
		pkg, ok := pkgMap[pkgName]
		if !ok || pkg.Homepage == "" {
			continue
		}
		matches := githubRepoRegex.FindStringSubmatch(pkg.Homepage)
		if matches == nil {
			continue
		}

		orphans = append(orphans, GitHubOrphan{
			PkgName:  pkgName,
			Version:  pkg.Version,
			Owner:    matches[1],
			Repo:     matches[2],
			Homepage: pkg.Homepage,
		})
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
func MergeOrphansToState(st *State, orphans []GitHubOrphan) int {
	added := 0
	for _, o := range orphans {
		repoKey := o.Owner + "/" + o.Repo
		if st.Get(repoKey) != nil {
			continue
		}
		st.Packages[repoKey] = &PackageRecord{
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

// DiscoverGitHubPackages 发现系统中的 GitHub 来源包（不一定是 orphan）
func DiscoverGitHubPackages() ([]GitHubOrphan, error) {
	pkgs, err := parseDpkgStatus()
	if err != nil {
		return nil, err
	}

	var results []GitHubOrphan
	githubRepoRegex := regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)

	for _, pkg := range pkgs {
		if !strings.Contains(pkg.Status, "install ok installed") {
			continue
		}
		if pkg.Homepage == "" {
			continue
		}
		matches := githubRepoRegex.FindStringSubmatch(pkg.Homepage)
		if matches == nil {
			continue
		}

		results = append(results, GitHubOrphan{
			PkgName:  pkg.Name,
			Version:  pkg.Version,
			Owner:    matches[1],
			Repo:     matches[2],
			Homepage: pkg.Homepage,
		})
	}

	return results, nil
}
