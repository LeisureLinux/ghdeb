package state

import (
	"regexp"
	"sort"
	"strings"
)

// InstalledGitHubPkg 已安装且 Homepage 指向 GitHub 的包
type InstalledGitHubPkg struct {
	PkgName  string // dpkg 包名
	Version  string // 已装版本
	Owner    string // GitHub owner
	Repo     string // GitHub repo
	Homepage string
	Arch     string
}

// ScanInstalledGitHubRepos 枚举所有已安装包（含孤立包）中 Homepage 指向 GitHub 的仓库。
// 对 Homepage 不含 github.com 的孤立包，尝试深度抓取其主页以查找 GitHub 链接。
// pkgFilter 非空时只处理该包。
func ScanInstalledGitHubRepos(pkgFilter string) ([]InstalledGitHubPkg, error) {
	// 解析所有已安装包
	allPkgs, err := parseDpkgStatus()
	if err != nil {
		return nil, err
	}

	// 构建孤立包集合（无 apt 源的本地包），用于深挖非 GitHub 主页
	orphanSet := make(map[string]bool)
	if orphans, err := getOrphanPackages(); err == nil {
		for _, o := range orphans {
			orphanSet[o] = true
		}
	}

	githubRepoRegex := regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)

	var result []InstalledGitHubPkg
	seen := make(map[string]bool)

	for _, pkg := range allPkgs {
		if pkgFilter != "" && pkg.Name != pkgFilter {
			continue
		}
		if !strings.Contains(pkg.Status, "installed") || strings.Contains(pkg.Status, "not-installed") {
			continue
		}
		if pkg.Homepage == "" {
			continue
		}

		owner, repo := "", ""
		if m := githubRepoRegex.FindStringSubmatch(pkg.Homepage); m != nil {
			owner, repo = m[1], m[2]
		} else if orphanSet[pkg.Name] {
			// 孤立包且主页不含 github.com：深度抓页查找
			o, r, derr := FindGitHubFromHomepage(pkg.Homepage)
			if derr == nil {
				owner, repo = o, r
			}
		}

		if owner == "" || repo == "" {
			continue
		}
		// 跳过 github 自身的元链接
		if owner == "github" || repo == "github" {
			continue
		}
		repo = strings.TrimSuffix(repo, ".git")

		repoKey := strings.ToLower(owner + "/" + repo)
		if seen[repoKey] {
			continue
		}
		seen[repoKey] = true

		result = append(result, InstalledGitHubPkg{
			PkgName:  pkg.Name,
			Version:  pkg.Version,
			Owner:    owner,
			Repo:     repo,
			Homepage: pkg.Homepage,
			Arch:     pkg.Architecture,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PkgName < result[j].PkgName
	})

	return result, nil
}

// MergeInstalledToState 将已装 GitHub 包合并到 state（避免重复）。
// 返回实际新增的数量。
func MergeInstalledToState(st *State, pkgs []InstalledGitHubPkg) int {
	added := 0
	for _, p := range pkgs {
		repoKey := strings.ToLower(p.Owner + "/" + p.Repo)
		if st.Get(repoKey) != nil {
			continue
		}
		if st.GetByPkgName(p.PkgName) != nil {
			continue
		}

		arch := p.Arch
		if arch == "" {
			arch = "unknown"
		}

		st.Packages[repoKey] = &PackageRecord{
			PkgName:        p.PkgName,
			Owner:          p.Owner,
			Repo:           p.Repo,
			CurrentVersion: p.Version,
			Arch:           arch,
			Removed:        false,
			History: []HistoryEntry{
				{
					Action:    ActionInstall,
					Version:   p.Version,
					Timestamp: "auto-discovered",
				},
			},
			UpdatedAt: "auto-discovered",
		}
		added++
	}
	return added
}

// ScanInstalledGitHubReposQuick 纯本地快路径：仅解析 /var/lib/dpkg/status，
// 提取 Homepage 直接包含 github.com 的已装包。不调用 apt、不做网络抓取，
// 专供 catalog init 快速建立目录使用。
func ScanInstalledGitHubReposQuick(pkgFilter string) ([]InstalledGitHubPkg, error) {
	allPkgs, err := parseDpkgStatus()
	if err != nil {
		return nil, err
	}

	githubRepoRegex := regexp.MustCompile(`github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)`)

	var result []InstalledGitHubPkg
	seen := make(map[string]bool)

	for _, pkg := range allPkgs {
		if pkgFilter != "" && pkg.Name != pkgFilter {
			continue
		}
		if !strings.Contains(pkg.Status, "installed") || strings.Contains(pkg.Status, "not-installed") {
			continue
		}
		if pkg.Homepage == "" {
			continue
		}

		m := githubRepoRegex.FindStringSubmatch(pkg.Homepage)
		if m == nil {
			continue // 仅收录 Homepage 直接含 github.com 的包
		}
		owner, repo := m[1], m[2]
		if owner == "github" || repo == "github" {
			continue // 跳过 github 自身元链接
		}
		repo = strings.TrimSuffix(repo, ".git")

		repoKey := strings.ToLower(owner + "/" + repo)
		if seen[repoKey] {
			continue
		}
		seen[repoKey] = true

		result = append(result, InstalledGitHubPkg{
			PkgName:  pkg.Name,
			Version:  pkg.Version,
			Owner:    owner,
			Repo:     repo,
			Homepage: pkg.Homepage,
			Arch:     pkg.Architecture,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PkgName < result[j].PkgName
	})

	return result, nil
}
