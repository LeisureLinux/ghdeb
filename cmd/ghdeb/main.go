// ghdeb - 从 GitHub Releases 安装/升级 .deb 包
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"sort"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/leisurelinux/ghdeb/internal/catalog"
	"github.com/leisurelinux/ghdeb/internal/deb"
	gh "github.com/leisurelinux/ghdeb/internal/github"
	"github.com/leisurelinux/ghdeb/internal/state"
)

const version = "0.7.3"

func main() {
	// 检查是否使用 --json，如果是则不打印 banner
	jsonMode := false
	if len(os.Args) >= 2 && (os.Args[1] == "list" || os.Args[1] == "ls" || os.Args[1] == "show") {
		for _, arg := range os.Args[2:] {
			if arg == "--json" {
				jsonMode = true
				break
			}
		}
	}
	
	if !jsonMode {
		fmt.Printf(T("ghdeb v%s - 管理从 GitHub Releases 下载的 .deb 包 © LeisureLinux\n", "ghdeb v%s - manage .deb packages downloaded from GitHub Releases © LeisureLinux\n"), version)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "install":
		err = cmdInstall(args)
	case "upgrade":
		err = cmdUpgrade(args)
	case "reinstall":
		err = cmdReinstall(args)
	case "scan":
		err = cmdScan(args)
	case "search":
		err = cmdSearch(args)
	case "list", "ls":
		err = cmdList(args)
	case "history":
		err = cmdHistory(args)
	case "remove", "rm":
		err = cmdRemove(args)
	case "purge":
		err = cmdPurge(args)
	case "show":
		err = cmdShow(args)
	case "info":
		err = cmdShow(args) // info 作为 show 的别名
	case "clean":
		err = cmdClean(args)
	case "catalog":
		err = cmdCatalog(args)
	case "set-repo":
		err = cmdSetRepo(args)
	case "test-homepage":
		err = cmdTestHomepage(args)
	case "version", "--version", "-v":
		fmt.Printf("ghdeb %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", T("错误", "Error"), err)
		os.Exit(1)
	}
}

func printUsage() {
	if isChinese() {
		fmt.Print(`ghdeb - 管理从 GitHub Releases 下载的 .deb 包

用法:
  ghdeb install <pkg|owner/repo>[@tag]  安装（支持短名称或 owner/repo）
  ghdeb upgrade [pkg]                   升级包（不指定则升级所有）
  ghdeb reinstall <pkg>                 重新安装指定包
  ghdeb scan [--deep]                   扫描系统中的 GitHub 孤立包并纳入管理
  ghdeb search <pattern>                在包目录中搜索
  ghdeb list [--refresh]                列出所有已管理的包
  ghdeb catalog list                    列出包目录中所有条目
  ghdeb catalog show <name>             显示目录条目详情
  ghdeb catalog add <name> --repo <owner/repo>  添加条目到用户目录
  ghdeb catalog delete <name>           从用户目录删除条目
  ghdeb catalog cleanup                 清理用户目录中与系统目录重复的条目
  ghdeb show <pkg>                      显示包的完整信息
  ghdeb history <pkg>                   查看某包的完整操作历史
  ghdeb remove <pkg>                    标记移除（保留历史记录，不卸载）
  ghdeb purge <pkg>                     卸载软件并清除配置文件
  ghdeb clean [--dry-run]               清理下载的 .deb 缓存
  ghdeb set-repo <pkg> <owner/repo>     为包设置 GitHub 仓库
  ghdeb info <pkg>                      show 的别名
  ghdeb version                         显示版本

包目录:
  系统目录: /usr/share/ghdeb/catalog.toml
  用户目录: ~/.config/ghdeb/catalog.toml（同名覆盖系统条目）

环境变量:
  GITHUB_TOKEN / GH_TOKEN               GitHub 个人访问令牌（提高 API 限额）

示例:
  ghdeb install bat                     通过短名称安装 bat
  ghdeb install sharkdp/bat             通过 owner/repo 安装
  ghdeb install LeisureLinux/ghdeb@v0.6.0  安装指定版本
  ghdeb search monitor                  搜索包含 monitor 的包
  ghdeb catalog list                    列出所有目录条目
  ghdeb catalog add myapp --repo user/myapp --summary "我的应用"
  ghdeb show rustdesk                   显示包信息
  ghdeb clean                           清理缓存
  ghdeb purge rustdesk                  卸载 rustdesk
`)
	} else {
		fmt.Print(`ghdeb - Manage .deb packages from GitHub Releases

Usage:
  ghdeb install <pkg|owner/repo>[@tag]  Install (short name or owner/repo)
  ghdeb upgrade [pkg]                   Upgrade packages (all if unspecified)
  ghdeb reinstall <pkg>                 Reinstall a package
  ghdeb scan [--deep]                   Scan system for GitHub orphan packages
  ghdeb search <pattern>                Search in package catalog
  ghdeb list [--refresh]                List managed packages
  ghdeb catalog list                    List all catalog entries
  ghdeb catalog show <name>             Show catalog entry details
  ghdeb catalog add <name> --repo <owner/repo>  Add entry to user catalog
  ghdeb catalog delete <name>           Remove entry from user catalog
  ghdeb catalog cleanup                 Clean up duplicate entries from user catalog
  ghdeb show <pkg>                      Show package details
  ghdeb history <pkg>                   View operation history
  ghdeb remove <pkg>                    Mark as removed (keep history)
  ghdeb purge <pkg>                     Uninstall and purge config
  ghdeb clean [--dry-run]               Clean .deb cache
  ghdeb set-repo <pkg> <owner/repo>     Set GitHub repo for a package
  ghdeb info <pkg>                      Alias for show
  ghdeb version                         Show version

Catalog:
  System: /usr/share/ghdeb/catalog.toml
  User:   ~/.config/ghdeb/catalog.toml (overrides system entries)

Environment Variables:
  GITHUB_TOKEN / GH_TOKEN               GitHub personal access token

Examples:
  ghdeb install bat                     Install via short name
  ghdeb install sharkdp/bat             Install via owner/repo
  ghdeb install LeisureLinux/ghdeb@v0.6.0  Install specific version
  ghdeb search monitor                  Search catalog
  ghdeb catalog list                    List catalog entries
  ghdeb catalog add myapp --repo user/myapp --summary "My app"
  ghdeb show rustdesk                   Show package info
  ghdeb clean                           Clean cache
  ghdeb purge rustdesk                  Uninstall rustdesk
`)
	}
}

// resolvePkgArg 解析包参数：支持 owner/repo 和短名称（通过 catalog）
func resolvePkgArg(arg string) (owner, repo string, err error) {
	// 先尝试 owner/repo 格式
	owner, repo, parseErr := gh.ParseRepo(arg)
	if parseErr == nil {
		return owner, repo, nil
	}
	// 尝试 catalog 短名称
	cat, catErr := catalog.Load()
	if catErr != nil {
		return "", "", fmt.Errorf("无法解析 %s 且加载目录失败: %w", arg, catErr)
	}
	entry := cat.Lookup(arg)
	if entry == nil {
		return "", "", fmt.Errorf("未找到包 %s（既不是有效的 owner/repo，也不在目录中）", arg)
	}
	// 处理直接 URL 来源（非 GitHub）
	if entry.IsDirectURL() {
		return "", "", fmt.Errorf("包 %s 使用直接 URL 来源，暂不支持自动安装，请手动下载: %s", arg, entry.URL)
	}
	owner, repo, err = gh.ParseRepo(entry.Repo)
	if err != nil {
		return "", "", fmt.Errorf("目录中 %s 的仓库 %s 格式无效: %w", arg, entry.Repo, err)
	}
	fmt.Printf(T("📖 目录匹配: %s → %s/%s\n", "📖 Catalog match: %s → %s/%s\n"), arg, owner, repo)
	return owner, repo, nil
}

// --- install ---

func cmdInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名或仓库，如: ghdeb install bat 或 ghdeb install LeisureLinux/ghdeb")
	}

	repoStr, tag := parseRepoSpec(args[0])
	
	// 检查是否是 catalog 短名称
	var catalogName string
	cat, _ := catalog.Load()
	if cat != nil {
		if entry := cat.Lookup(repoStr); entry != nil {
			catalogName = repoStr
		}
	}
	
	owner, repo, err := resolvePkgArg(repoStr)
	if err != nil {
		return err
	}

	arch, err := deb.DetectArch()
	if err != nil {
		return err
	}
	fmt.Printf(T("🔍 系统架构: %s\n", "🔍 System arch: %s\n"), arch.DpkgArch)

	client := gh.NewClient()
	release, err := fetchRelease(client, owner, repo, tag)
	if err != nil {
		return err
	}
	fmt.Printf(T("📌 版本: %s\n", "📌 Version: %s\n"), release.TagName)

	st, err := state.Load()
	if err != nil {
		return err
	}
	repoKey := owner + "/" + repo
	if existing := st.Get(repoKey); existing != nil && existing.CurrentVersion == release.TagName && !existing.Removed {
		fmt.Printf(T("✅ %s 已安装版本 %s，无需重复安装\n", "✅ %s version %s already installed, no need to reinstall\n"), repo, release.TagName)
		return nil
	}

	asset, err := gh.FindDebAsset(release, arch)
	if err != nil {
		if fbErr, ok := err.(*gh.FallbackError); ok {
			fmt.Fprintf(os.Stderr, "\n%s\n", fbErr.Error())
			return fmt.Errorf("无法自动安装，请手动下载上述文件")
		}
		return err
	}
	fmt.Printf(T("📥 匹配文件: %s (%s)\n", "📥 Matched file: %s (%s)\n"), asset.Name, formatSize(asset.Size))

	destPath, err := downloadAsset(client, *asset)
	if err != nil {
		return err
	}

	pkgName := deb.ExtractPkgName(destPath)

	if err := installDeb(destPath); err != nil {
		return err
	}

	releaseURL := release.HTMLURL
	if releaseURL == "" {
		releaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, release.TagName)
	}
	st.SetInstall(repoKey, owner, repo, release.TagName, asset.Name, destPath, releaseURL, arch.DpkgArch, pkgName)
	if err := st.Save(); err != nil {
		return fmt.Errorf("保存状态失败: %w", err)
	}

	// 显示安装结果
	slug := repoKey
	if catalogName != "" {
		slug = catalogName + " (" + repoKey + ")"
	}
	fmt.Printf(T("🎉 安装完成: %s\n", "🎉 Install complete: %s\n"), slug)
	fmt.Printf(T("   OS package name: %s\n", "   OS package name: %s\n"), pkgName)
	return nil
}

// --- upgrade ---

func cmdUpgrade(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}

	// 升级前自动扫描孤立包
	if len(args) == 0 {
		fmt.Println(T("🔍 扫描系统中的 GitHub 孤立包...", "🔍 Scanning GitHub orphan packages..."))
		orphans, scanErr := state.ScanOrphans(false, nil)
		if scanErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("扫描失败", "Scan failed"), scanErr)
		} else if len(orphans) > 0 {
			added := state.MergeOrphansToState(st, orphans)
			if added > 0 {
				fmt.Printf(T("📦 发现 %d 个新的 GitHub 孤立包，已纳入管理\n", "📦 Found %d new GitHub orphan packages, added to management\n"), added)
				for _, o := range orphans {
					var repoKey string
					if o.HasGitHub {
						repoKey = o.Owner + "/" + o.Repo
					} else {
						repoKey = o.PkgName
					}
					if st.Get(repoKey) != nil && st.Get(repoKey).UpdatedAt == "auto-discovered" {
						if o.HasGitHub {
							fmt.Printf("   + %s/%s (%s)\n", o.Owner, o.Repo, o.Version)
						} else {
							fmt.Printf("   + %s (%s)\n", o.PkgName, o.Version)
						}
					}
				}
				if saveErr := st.Save(); saveErr != nil {
					fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("保存状态失败", "Save state failed"), saveErr)
				}
			}
		}
		fmt.Println()
	}

	type upgradeTarget struct {
		owner string
		repo  string
		pkg   *state.PackageRecord
	}
	var targets []upgradeTarget

	if len(args) > 0 {
		for _, arg := range args {
			owner, repo, resolveErr := resolvePkgArg(arg)
			if resolveErr != nil {
				// 尝试作为 pkg name 查找 state
				rec := st.GetByPkgName(arg)
				if rec != nil && rec.Owner != "" && rec.Repo != "" {
					if !deb.IsPackageInstalled(rec.PkgName) {
						if !rec.Removed {
							rec.Removed = true
							fmt.Printf(T("⚠️  %s/%s 未安装，将重新安装\n", "⚠️  %s/%s not installed, will reinstall\n"), rec.Owner, rec.Repo)
						}
					}
					targets = append(targets, upgradeTarget{owner: rec.Owner, repo: rec.Repo, pkg: rec})
				} else {
					fmt.Fprintf(os.Stderr, "⚠️  %s %s: %s\n", T("跳过", "Skip"), arg, T("未找到该包", "package not found"))
				}
				continue
			}
			repoKey := owner + "/" + repo
			rec := st.Get(repoKey)
			if rec != nil {
				if rec.PkgName != "" && !deb.IsPackageInstalled(rec.PkgName) {
					if !rec.Removed {
						rec.Removed = true
						fmt.Printf(T("⚠️  %s 未安装，将重新安装\n", "⚠️  %s not installed, will reinstall\n"), repoKey)
					}
				}
				targets = append(targets, upgradeTarget{owner: owner, repo: repo, pkg: rec})
			} else {
				// 未管理，但 catalog 能解析，直接安装
				targets = append(targets, upgradeTarget{owner: owner, repo: repo, pkg: nil})
			}
		}
	} else {
		for _, r := range st.List() {
			if r.Owner == "" || r.Repo == "" {
				continue
			}
			installed := false
			if r.PkgName != "" {
				installed = deb.IsPackageInstalled(r.PkgName)
			}
			if r.Removed && installed {
				r.Removed = false
				fmt.Printf(T("📦 %s/%s 仍在系统上，恢复管理\n", "📦 %s/%s still installed, restoring management\n"), r.Owner, r.Repo)
			}
			targets = append(targets, upgradeTarget{owner: r.Owner, repo: r.Repo, pkg: r})
		}
	}

	if len(targets) == 0 {
		fmt.Println(T("没有已管理的包需要升级", "No managed packages to upgrade"))
		return nil
	}

	client := gh.NewClient()
	arch, err := deb.DetectArch()
	if err != nil {
		return err
	}

	upgraded := 0
	for _, t := range targets {
		repoKey := t.owner + "/" + t.repo

		fmt.Printf(T("\n🔍 检查 %s...\n", "\n🔍 Checking %s...\n"), repoKey)
		release, getErr := client.GetLatestRelease(t.owner, t.repo)
		if getErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("获取 release 失败", "Get release failed"), getErr)
			continue
		}
		client.SetCachedRelease(t.owner, t.repo, release.TagName)

		if t.pkg != nil && t.pkg.CurrentVersion == release.TagName && !t.pkg.Removed {
			fmt.Printf(T("✅ 已是最新版本 %s\n", "✅ Already latest version %s\n"), release.TagName)
			continue
		}
		if t.pkg == nil || t.pkg.Removed {
			fmt.Printf(T("📦 安装: %s\n", "📦 Installing: %s\n"), release.TagName)
		} else {
			fmt.Printf(T("📦 发现新版本: %s → %s\n", "📦 New version found: %s → %s\n"), t.pkg.CurrentVersion, release.TagName)
		}

		asset, findErr := gh.FindDebAsset(release, arch)
		if findErr != nil {
			if fbErr, ok := findErr.(*gh.FallbackError); ok {
				fmt.Fprintf(os.Stderr, "%s\n", fbErr.Error())
			} else {
				fmt.Fprintf(os.Stderr, "⚠️  %v\n", findErr)
			}
			continue
		}

		destPath, dlErr := downloadAsset(client, *asset)
		if dlErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("下载失败", "Download failed"), dlErr)
			continue
		}

		pkgName := deb.ExtractPkgName(destPath)

		if instErr := installDeb(destPath); instErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("安装失败", "Install failed"), instErr)
			continue
		}

		releaseURL := release.HTMLURL
		if releaseURL == "" {
			releaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", t.owner, t.repo, release.TagName)
		}

		if t.pkg != nil && !t.pkg.Removed && t.pkg.CurrentVersion != "" {
			st.SetUpgrade(repoKey, release.TagName, asset.Name, destPath, releaseURL)
		} else {
			st.SetInstall(repoKey, t.owner, t.repo, release.TagName, asset.Name, destPath, releaseURL, arch.DpkgArch, pkgName)
		}
		upgraded++
		if t.pkg != nil && t.pkg.Removed {
			fmt.Printf(T("✅ 重新安装完成: %s %s\n", "✅ Reinstall complete: %s %s\n"), repoKey, release.TagName)
		} else if t.pkg == nil {
			fmt.Printf(T("✅ 安装完成: %s %s\n", "✅ Install complete: %s %s\n"), repoKey, release.TagName)
		} else {
			fmt.Printf(T("✅ 升级完成: %s %s\n", "✅ Upgrade complete: %s %s\n"), repoKey, release.TagName)
		}
	}

	if saveErr := st.Save(); saveErr != nil {
		return fmt.Errorf("保存状态失败: %w", saveErr)
	}

	if upgraded == 0 {
		fmt.Println(T("\n所有包已是最新", "\nAll packages are up to date"))
	} else {
		fmt.Printf(T("\n🎉 共处理 %d 个包\n", "\n🎉 Processed %d packages\n"), upgraded)
	}
	return nil
}

// --- reinstall ---

func cmdReinstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名，如: ghdeb reinstall bat")
	}

	owner, repo, err := resolvePkgArg(args[0])
	if err != nil {
		return err
	}

	repoKey := owner + "/" + repo
	arch, err := deb.DetectArch()
	if err != nil {
		return err
	}

	client := gh.NewClient()
	fmt.Printf(T("📦 获取最新 release...\n", "📦 Fetching latest release...\n"))
	release, err := client.GetLatestRelease(owner, repo)
	if err != nil {
		return err
	}
	fmt.Printf(T("📌 版本: %s\n", "📌 Version: %s\n"), release.TagName)

	asset, err := gh.FindDebAsset(release, arch)
	if err != nil {
		return err
	}
	fmt.Printf(T("📥 匹配文件: %s (%s)\n", "📥 Matched file: %s (%s)\n"), asset.Name, formatSize(asset.Size))

	destPath, err := downloadAsset(client, *asset)
	if err != nil {
		return err
	}

	pkgName := deb.ExtractPkgName(destPath)

	if err := installDeb(destPath); err != nil {
		return err
	}

	releaseURL := release.HTMLURL
	if releaseURL == "" {
		releaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, release.TagName)
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	// 记录为 reinstall
	st.SetInstall(repoKey, owner, repo, release.TagName, asset.Name, destPath, releaseURL, arch.DpkgArch, pkgName)
	// 标记最后一条历史为 reinstall
	if rec := st.Get(repoKey); rec != nil && len(rec.History) > 0 {
		rec.History[len(rec.History)-1].Reinstall = true
	}
	if err := st.Save(); err != nil {
		return fmt.Errorf("保存状态失败: %w", err)
	}

	fmt.Printf(T("🎉 重装完成: %s %s\n", "🎉 Reinstall complete: %s %s\n"), repoKey, release.TagName)
	return nil
}

// --- search ---

func cmdSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定搜索关键词，如: ghdeb search monitor")
	}
	pattern := strings.Join(args, " ")

	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}

	results, err := cat.Search(pattern)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Printf(T("未找到匹配 %q 的包\n", "No packages matching %q\n"), pattern)
		return nil
	}

	// 按名称排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	// 加载 state 检查安装状态
	st, _ := state.Load()

	fmt.Printf(T("找到 %d 个匹配的包:\n", "Found %d matching packages:\n"), len(results))
	fmt.Printf("%-20s %-30s %s\n", T("名称", "Name"), T("仓库", "Repo"), T("简介", "Summary"))
	fmt.Println(strings.Repeat("-", 80))

	for _, r := range results {
		entry := r.Entry
		repoKey := entry.Repo
		installed := false
		if st != nil {
			rec := st.Get(repoKey)
			if rec != nil && !rec.Removed {
				installed = true
			}
		}
		status := ""
		if installed {
			status = " ✅"
		}
		summary := truncate(entry.Summary, 40)
		fmt.Printf("%-20s %-30s %s%s\n", r.Name, repoKey, summary, status)
	}
	return nil
}

// --- show ---

func cmdShow(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名或仓库，如: ghdeb show bat")
	}

	// 解析 --json 参数
	jsonOutput := false
	var pkgArgs []string
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		} else {
			pkgArgs = append(pkgArgs, a)
		}
	}

	if len(pkgArgs) == 0 {
		return fmt.Errorf("请指定包名或仓库，如: ghdeb show bat")
	}
	arg := pkgArgs[0]
	cat, _ := catalog.Load()

	// 尝试解析为 owner/repo
	owner, repo, parseErr := gh.ParseRepo(arg)
	var catName string
	var catEntry *catalog.CatalogEntry

	if parseErr != nil {
		// 尝试 catalog 查找
		if cat != nil {
			catEntry = cat.Lookup(arg)
			if catEntry != nil {
				owner, repo, _ = gh.ParseRepo(catEntry.Repo)
				catName = arg
			}
		}
		if catEntry == nil {
			// 尝试 state 查找
			st, _ := state.Load()
			rec := st.GetByPkgName(arg)
			if rec != nil && rec.Owner != "" && rec.Repo != "" {
				owner = rec.Owner
				repo = rec.Repo
			} else {
				return fmt.Errorf("未找到包 %s", arg)
			}
		}
	}

	repoKey := owner + "/" + repo

	// 加载 state
	st, _ := state.Load()
	rec := st.Get(repoKey)

	// 从 catalog 查找（如果还没找到）
	if catEntry == nil && cat != nil {
		// 尝试通过 repo 反查
		for name, entry := range cat.AllEntries() {
			if entry.Repo == repoKey {
				catEntry = entry
				catName = name
				break
			}
		}
	}

	// JSON 输出结构
	type showInfo struct {
		ShortName      string `json:"short_name,omitempty"`
		Repo           string `json:"repo"`
		PrettyName     string `json:"pretty_name,omitempty"`
		Website        string `json:"website,omitempty"`
		Summary        string `json:"summary,omitempty"`
		Status         string `json:"status,omitempty"`
		RecordedVer    string `json:"recorded_version,omitempty"`
		SystemVer      string `json:"system_version,omitempty"`
		Arch           string `json:"arch,omitempty"`
		DebName        string `json:"deb_name,omitempty"`
		LatestVer      string `json:"latest_version,omitempty"`
		ReleaseURL     string `json:"release_url,omitempty"`
		MatchFile      string `json:"match_file,omitempty"`
		MatchSize      string `json:"match_size,omitempty"`
	}
	var info showInfo
	info.Repo = repoKey

	if !jsonOutput {
		// 显示信息
		fmt.Println(strings.Repeat("─", 50))
	}
	if catName != "" {
		info.ShortName = catName
		if !jsonOutput {
			fmt.Printf(T("短名称:   %s\n", "Short name:  %s\n"), catName)
		}
	}
	if !jsonOutput {
		fmt.Printf(T("仓库:     %s\n", "Repo:        %s\n"), repoKey)
	}
	if catEntry != nil {
		if catEntry.PrettyName != "" {
			info.PrettyName = catEntry.PrettyName
			if !jsonOutput {
				fmt.Printf(T("显示名:   %s\n", "Pretty name: %s\n"), catEntry.PrettyName)
			}
		}
		if catEntry.Website != "" {
			info.Website = catEntry.Website
			if !jsonOutput {
				fmt.Printf(T("网站:     %s\n", "Website:     %s\n"), catEntry.Website)
			}
		}
		if catEntry.Summary != "" {
			info.Summary = catEntry.Summary
			if !jsonOutput {
				fmt.Printf(T("简介:     %s\n", "Summary:     %s\n"), catEntry.Summary)
			}
		}
	}

	// 安装状态
	if rec != nil {
		if !jsonOutput {
			fmt.Println(strings.Repeat("─", 50))
		}
		if rec.Removed {
			info.Status = "removed"
			if !jsonOutput {
				fmt.Printf(T("状态:     ❌ 已移除\n", "Status:      ❌ removed\n"))
			}
		} else {
			info.Status = "managed"
			if !jsonOutput {
				fmt.Printf(T("状态:     ✅ 已管理\n", "Status:      ✅ managed\n"))
			}
		}
		info.RecordedVer = rec.CurrentVersion
		if !jsonOutput {
			fmt.Printf(T("记录版本: %s\n", "Recorded:    %s\n"), rec.CurrentVersion)
		}
		rec.RefreshSystemInfo(rec.PkgName)
		if rec.SystemVersion != "" {
			info.SystemVer = rec.SystemVersion
			if !jsonOutput {
				fmt.Printf(T("系统版本: %s\n", "System:      %s\n"), rec.SystemVersion)
			}
		}
		if rec.Arch != "" {
			info.Arch = rec.Arch
			if !jsonOutput {
				fmt.Printf(T("架构:     %s\n", "Arch:        %s\n"), rec.Arch)
			}
		}
		if rec.PkgName != "" {
			info.DebName = rec.PkgName
			if !jsonOutput {
				fmt.Printf(T("deb 包名: %s\n", "Deb name:    %s\n"), rec.PkgName)
			}
		}
	}

	// GitHub release 信息
	if !jsonOutput {
		fmt.Println(strings.Repeat("─", 50))
	}
	client := gh.NewClient()
	release, relErr := client.GetLatestRelease(owner, repo)
	if relErr == nil {
		info.LatestVer = release.TagName
		if !jsonOutput {
			fmt.Printf(T("最新版本: %s\n", "Latest:      %s\n"), release.TagName)
		}
		if release.HTMLURL != "" {
			info.ReleaseURL = release.HTMLURL
			if !jsonOutput {
				fmt.Printf("Release:     %s\n", release.HTMLURL)
			}
		}
		arch, _ := deb.DetectArch()
		if arch != nil {
			result, findErr := gh.FindAssetWithFallback(release, arch)
			if result != nil && result.Asset != nil {
				info.MatchFile = result.Asset.Name
				info.MatchSize = formatSize(result.Asset.Size)
				if !jsonOutput {
					fmt.Printf(T("匹配文件: %s (%s)\n", "Match:       %s (%s)\n"), result.Asset.Name, formatSize(result.Asset.Size))
				}
			} else if findErr != nil {
				if !jsonOutput {
					fmt.Printf(T("匹配文件: ⚠️ %v\n", "Match:       ⚠️ %v\n"), findErr)
				}
			}
		}
	} else {
		if !jsonOutput {
			fmt.Printf(T("最新版本: ⚠️ 获取失败: %v\n", "Latest:      ⚠️ failed: %v\n"), relErr)
		}
	}

	if jsonOutput {
		jsonData, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println(strings.Repeat("─", 50))
	}
	return nil
}



// --- catalog ---

func cmdCatalog(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定子命令: list, show, search, add, delete")
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list":
		return cmdCatalogList(subargs)
	case "show":
		return cmdCatalogShow(subargs)
	case "search":
		return cmdSearch(subargs) // 复用 search 逻辑
	case "add":
		return cmdCatalogAdd(subargs)
	case "delete", "del", "rm":
		return cmdCatalogDelete(subargs)
	default:
		return fmt.Errorf("未知子命令: %s（可用: list, show, search, add, delete）", subcmd)
	}
}

func cmdCatalogList(args []string) error {
	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}

	entries := cat.AllEntries()
	if len(entries) == 0 {
		fmt.Println(T("目录为空", "Catalog is empty"))
		return nil
	}

	st, _ := state.Load()

	names := cat.SortedNames()
	fmt.Printf("%-20s %-30s %s\n", T("名称", "Name"), T("仓库/URL", "Repo/URL"), T("简介", "Summary"))
	fmt.Println(strings.Repeat("-", 70))

	for _, name := range names {
		entry := entries[name]

		repoOrURL := entry.Repo
		if repoOrURL == "" {
			repoOrURL = truncate(entry.URL, 28)
		}

		// 检查安装状态
		installed := ""
		if st != nil && entry.Repo != "" {
			rec := st.Get(entry.Repo)
			if rec != nil && !rec.Removed {
				installed = " ✅"
			}
		}

		summary := truncate(entry.Summary, 40)
		fmt.Printf("%-20s %-30s %s%s\n", name, repoOrURL, summary, installed)
	}

	fmt.Printf("\n共 %d 个条目\n", len(entries))
	return nil
}

func cmdCatalogShow(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名，如: ghdeb catalog show bat")
	}

	name := args[0]
	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}

	entry := cat.Lookup(name)
	if entry == nil {
		return fmt.Errorf("目录中未找到 %s", name)
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("配置路径: %s\n", catalog.SystemCatalogPath())
	fmt.Println(strings.Repeat("─", 50))
	fmt.Print(catalog.FormatEntry(name, entry))
	return nil
}

func cmdCatalogAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: ghdeb catalog add <name> --repo <owner/repo> [选项]")
	}

	name := args[0]
	var entry catalog.CatalogEntry

	// 解析参数
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			if i+1 >= len(args) {
				return fmt.Errorf("--repo 需要参数")
			}
			i++
			entry.Repo = args[i]
		case "--url":
			if i+1 >= len(args) {
				return fmt.Errorf("--url 需要参数")
			}
			i++
			entry.URL = args[i]
		case "--pretty-name":
			if i+1 >= len(args) {
				return fmt.Errorf("--pretty-name 需要参数")
			}
			i++
			entry.PrettyName = args[i]
		case "--website":
			if i+1 >= len(args) {
				return fmt.Errorf("--website 需要参数")
			}
			i++
			entry.Website = args[i]
		case "--summary":
			if i+1 >= len(args) {
				return fmt.Errorf("--summary 需要参数")
			}
			i++
			entry.Summary = args[i]
		case "--gpg-key":
			if i+1 >= len(args) {
				return fmt.Errorf("--gpg-key 需要参数")
			}
			i++
			entry.GPGKey = args[i]
		default:
			return fmt.Errorf("未知选项: %s", args[i])
		}
	}

	if err := addToSystemCatalog(name, &entry); err != nil {
		return err
	}

	fmt.Printf(T("✅ 已添加 %s 到系统目录 (%s)\n", "✅ Added %s to system catalog (%s)\n"), name, catalog.SystemCatalogPath())
	if entry.Repo != "" {
		fmt.Printf("   repo: %s\n", entry.Repo)
	}
	if entry.URL != "" {
		fmt.Printf("   url: %s\n", entry.URL)
	}
	return nil
}

func cmdCatalogDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名，如: ghdeb catalog delete myapp")
	}

	name := args[0]
	if err := removeFromSystemCatalog(name); err != nil {
		return err
	}

	fmt.Printf(T("✅ 已从系统目录删除 %s\n", "✅ Deleted %s from system catalog\n"), name)
	return nil
}

// addToSystemCatalog 将条目添加到系统级目录
func addToSystemCatalog(name string, entry *catalog.CatalogEntry) error {
	if err := catalog.ValidateCatalogName(name); err != nil {
		return err
	}
	if err := catalog.ValidateCatalogEntry(entry); err != nil {
		return err
	}

	path := catalog.SystemCatalogPath()
	
	// 加载现有内容
	entries := make(map[string]catalog.CatalogEntry)
	if data, err := os.ReadFile(path); err == nil {
		toml.Decode(string(data), &entries)
	}
	
	// 检查是否已存在
	if _, ok := entries[name]; ok {
		return fmt.Errorf("条目 %s 已存在于 %s", name, path)
	}
	
	entries[name] = *entry
	
	// 写入（需要 root 权限）
	return writeSystemCatalog(path, entries)
}

// removeFromSystemCatalog 从系统级目录删除条目
func removeFromSystemCatalog(name string) error {
	path := catalog.SystemCatalogPath()
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("系统目录不存在: %s", path)
		}
		return err
	}
	
	entries := make(map[string]catalog.CatalogEntry)
	toml.Decode(string(data), &entries)
	
	if _, ok := entries[name]; !ok {
		return fmt.Errorf("系统目录中未找到 %s", name)
	}
	
	delete(entries, name)
	return writeSystemCatalog(path, entries)
}

// writeSystemCatalog 写入系统级目录文件
func writeSystemCatalog(path string, entries map[string]catalog.CatalogEntry) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	
	var sb strings.Builder
	sb.WriteString("# ghdeb 包目录 (Known Packages Catalog)\n")
	sb.WriteString("# 路径: " + path + "\n#\n")
	sb.WriteString("# 编辑此文件需 root 权限: sudo vim /etc/ghdeb/catalog.toml\n\n")
	
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
	
	data := []byte(sb.String())
	if err := os.WriteFile(path, data, 0644); err != nil {
		if errors.Is(err, os.ErrPermission) {
			// 权限不足，使用 sudo tee 写入
			cmd := exec.Command("sudo", "tee", path)
			cmd.Stdin = strings.NewReader(string(data))
			cmd.Stdout = nil
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("写入失败（sudo 被拒绝或失败）: %w", err)
			}
			return nil
		}
		return fmt.Errorf("写入失败: %w", err)
	}
	
	return nil
}

// --- clean ---

func cmdClean(args []string) error {
	dryRun := false
	for _, a := range args {
		if a == "--dry-run" || a == "-n" {
			dryRun = true
		}
	}

	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println(T("缓存目录不存在，无需清理", "Cache directory does not exist, nothing to clean"))
			return nil
		}
		return err
	}

	var totalSize int64
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".deb") || strings.HasSuffix(e.Name(), ".json_extract") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, e.Name())
			totalSize += info.Size()
		}
	}

	if len(files) == 0 {
		fmt.Println(T("缓存为空，无需清理", "Cache is empty, nothing to clean"))
		return nil
	}

	if dryRun {
		fmt.Printf(T("将清理 %d 个文件，释放 %s:\n", "Would clean %d files, freeing %s:\n"), len(files), formatSize(totalSize))
		for _, f := range files {
			fmt.Printf("  %s\n", f)
		}
		return nil
	}

	removed := 0
	for _, f := range files {
		path := filepath.Join(cacheDir, f)
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  删除失败 %s: %v\n", f, err)
		} else {
			removed++
		}
	}

	fmt.Printf(T("✅ 已清理 %d 个文件，释放 %s\n", "✅ Cleaned %d files, freed %s\n"), removed, formatSize(totalSize))
	return nil
}

// --- purge ---

func cmdPurge(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名，如: ghdeb purge bat")
	}

	owner, repo, err := resolvePkgArg(args[0])
	if err != nil {
		return err
	}
	repoKey := owner + "/" + repo

	st, err := state.Load()
	if err != nil {
		return err
	}
	rec := st.Get(repoKey)
	if rec == nil {
		return fmt.Errorf("未找到 %s 的管理记录", repoKey)
	}

	// 获取 deb 包名
	pkgName := rec.PkgName
	if pkgName == "" {
		pkgName = repo
	}

	// 检查是否已安装
	if !deb.IsPackageInstalled(pkgName) {
		fmt.Printf(T("⚠️  %s 未安装在系统上\n", "⚠️  %s is not installed on system\n"), pkgName)
	} else {
		// 执行 apt-get purge
		fmt.Printf(T("🗑️  卸载 %s (apt-get purge)...\n", "🗑️  Purging %s (apt-get purge)...\n"), pkgName)
		cmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "purge", "-y", pkgName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("purge 失败: %w", err)
		}

		// autoremove
		fmt.Printf(T("🧹 清理依赖 (apt-get autoremove)...\n", "🧹 Cleaning dependencies (apt-get autoremove)...\n"))
		autoCmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "autoremove", "-y")
		autoCmd.Stdout = os.Stdout
		autoCmd.Stderr = os.Stderr
		_ = autoCmd.Run() // autoremove 失败不阻塞
	}

	// 标记移除
	st.MarkRemoved(repoKey)
	if err := st.Save(); err != nil {
		return fmt.Errorf("保存状态失败: %w", err)
	}

	fmt.Printf(T("✅ 已卸载并清除 %s\n", "✅ Purged %s\n"), repoKey)
	return nil
}

// --- scan ---

func cmdScan(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}

	deepScan := false
	for _, arg := range args {
		if arg == "--deep" {
			deepScan = true
		}
	}

	if deepScan {
		fmt.Println(T("🔍 深度扫描系统中的孤立包（抓取 Homepage 查找 GitHub 链接）...", "🔍 Deep scanning orphan packages (fetching Homepage for GitHub links)..."))
	} else {
		fmt.Println(T("🔍 扫描系统中的孤立包（无 apt 源）...", "🔍 Scanning orphan packages (no apt source)..."))
		fmt.Println(T("   提示: 使用 --deep 参数可尝试从 Homepage 页面中查找 GitHub 链接", "   Hint: Use --deep to fetch Homepage for GitHub links"))
	}

	progress := func(msg string) {
		fmt.Println(msg)
	}

	pkgs, scanErr := state.ScanOrphans(deepScan, progress)
	if scanErr != nil {
		return fmt.Errorf("扫描失败: %w", scanErr)
	}

	if len(pkgs) == 0 {
		fmt.Println(T("未发现 GitHub 来源的孤立包", "No GitHub-sourced orphan packages found"))
		return nil
	}

	fmt.Printf(T("\n发现 %d 个孤立包:\n", "\nFound %d orphan packages:\n"), len(pkgs))
	fmt.Printf("%-20s %-30s %-12s %-10s %s\n", T("包名", "Package"), T("仓库", "Repo"), T("版本", "Version"), T("状态", "Status"), "Homepage")
	fmt.Println(strings.Repeat("-", 100))

	newCount := 0
	for _, p := range pkgs {
		var repoSlug string
		if p.HasGitHub {
			repoSlug = p.Owner + "/" + p.Repo
		} else {
			repoSlug = T("(待补充)", "(pending)")
		}

		existing := st.GetByPkgName(p.PkgName)

		status := "🆕 " + T("未管理", "unmanaged")
		if existing != nil {
			if existing.Removed {
				status = "❌ " + T("removed", "removed")
			} else {
				status = "✅ " + T("已管理", "managed")
			}
		}

		fmt.Printf("%-20s %-30s %-12s %-10s %s\n",
			p.PkgName,
			repoSlug,
			p.Version,
			status,
			truncate(p.Homepage, 40),
		)

		if existing == nil {
			newCount++
		}
	}

	if newCount > 0 {
		fmt.Printf(T("\n📦 其中 %d 个尚未管理\n", "\n📦 %d of them are unmanaged\n"), newCount)
		fmt.Print(T("是否纳入 ghdeb 管理？[y/N] ", "Add them to ghdeb management? [y/N] "))
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) == "y" {
			added := state.MergeOrphansToState(st, pkgs)
			if err := st.Save(); err != nil {
				return fmt.Errorf("保存状态失败: %w", err)
			}
			fmt.Printf(T("✅ 已将 %d 个包纳入管理\n", "✅ Added %d packages to management\n"), added)
		}
	}

	return nil
}

// --- list ---

func cmdList(args []string) error {
	refresh := false
	jsonOutput := false
	for _, a := range args {
		if a == "--refresh" || a == "-r" {
			refresh = true
		}
		if a == "--json" {
			jsonOutput = true
		}
	}

	// 加载 catalog
	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}

	// 加载 state
	st, err := state.Load()
	if err != nil {
		return err
	}

	client := gh.NewClient()
	if refresh {
		gh.InvalidateCache("", "")
	}

	// JSON 输出结构
	type listPkg struct {
		Name           string `json:"name"`
		Repo           string `json:"repo"`
		InstalledVer   string `json:"installed_version,omitempty"`
		LatestVer      string `json:"latest_version,omitempty"`
		Status         string `json:"status"`
	}
	var jsonPkgs []listPkg

	if !jsonOutput {
		// 表头
		fmt.Printf("%s %s %s %s %s\n",
			padRight(T("包名", "Name"), 20),
			padRight(T("仓库", "Repo"), 25),
			padRight(T("已装版本", "Installed"), 12),
			padRight(T("最新版本", "Latest"), 12),
			T("状态", "Status"))
		fmt.Println(strings.Repeat("-", 90))
	}

	// 遍历 catalog 所有条目
	names := cat.SortedNames()
	installedCount := 0
	upgradableCount := 0
	orphanCount := 0

	for _, name := range names {
		entry := cat.Packages[name]
		repo := entry.Repo

		// 从 state 查找（通过 repo key）
		rec := st.Get(repo)

		// 检查系统是否实际安装
		var sysVer string
		if rec != nil && rec.PkgName != "" {
			rec.RefreshSystemInfo(rec.PkgName)
			sysVer = rec.SystemVersion
		}

		// 获取最新版本
		latestVer := "-"
		if repo != "" {
			parts := strings.SplitN(repo, "/", 2)
			if len(parts) == 2 {
				cached := client.GetCachedRelease(parts[0], parts[1])
				if cached != "" {
					latestVer = cached
				} else {
					rel, apiErr := client.GetLatestRelease(parts[0], parts[1])
					if apiErr == nil {
						latestVer = rel.TagName
						client.SetCachedRelease(parts[0], parts[1], rel.TagName)
					}
				}
			}
		}

		// 确定状态
		var status string
		var installedVer string

		if rec != nil && !rec.Removed && sysVer != "" {
			// 已安装
			installedVer = sysVer
			installedCount++

			// 比较版本
			if latestVer != "-" && latestVer != rec.CurrentVersion {
				status = "🔄 " + T("可升级", "upgradable")
				upgradableCount++
			} else {
				status = "✅ " + T("已安装", "installed")
			}
		} else {
			// 未安装
			installedVer = "-"
			status = "📦 " + T("可安装", "available")
		}

		if jsonOutput {
			pkg := listPkg{Name: name, Repo: repo, Status: status}
			if installedVer != "-" {
				pkg.InstalledVer = installedVer
			}
			if latestVer != "-" {
				pkg.LatestVer = latestVer
			}
			jsonPkgs = append(jsonPkgs, pkg)
		} else {
			fmt.Printf("%s %s %s %s %s\n",
				padRight(name, 20),
				padRight(truncate(repo, 23), 25),
				padRight(installedVer, 12),
				padRight(latestVer, 12),
				status)
		}
	}

	// 检查 state 中不在 catalog 的包（orphan，仅显示有 GitHub repo 的）
	for _, rec := range st.List() {
		if rec.Removed {
			continue
		}
		// 跳过没有 GitHub repo 的包
		if rec.Owner == "" || rec.Repo == "" {
			continue
		}
		repoKey := rec.Owner + "/" + rec.Repo
		// 检查是否已在 catalog 中处理
		found := false
		for _, entry := range cat.Packages {
			if entry.Repo == repoKey {
				found = true
				break
			}
		}
		if found {
			continue
		}

		// 不在 catalog 中的 orphan 包（有 GitHub repo）
		rec.RefreshSystemInfo(rec.PkgName)
		sysVer := rec.SystemVersion
		if sysVer == "" {
			// 未实际安装，跳过
			continue
		}
		status := "✅ " + T("已安装", "installed")
		installedCount++
		orphanCount++
		if jsonOutput {
			pkg := listPkg{Name: rec.PkgName, Repo: repoKey, InstalledVer: sysVer, Status: status}
			jsonPkgs = append(jsonPkgs, pkg)
		} else {
			fmt.Printf("%s %s %s %s %s\n",
				padRight(rec.PkgName, 20),
				padRight(repoKey, 25),
				padRight(sysVer, 12),
				padRight("-", 12),
				status)
		}
	}

	if jsonOutput {
		jsonData, _ := json.MarshalIndent(jsonPkgs, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println(strings.Repeat("-", 90))
		totalCount := len(names) + orphanCount
		fmt.Printf(T("共 %d 个包，%d 个已安装", "Total %d packages, %d installed"), totalCount, installedCount)
		if upgradableCount > 0 {
			fmt.Printf(T("，%d 个可升级", ", %d upgradable"), upgradableCount)
		}
		fmt.Println()
	}

	return nil
}

// --- history ---

func cmdHistory(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定仓库，如: ghdeb history LeisureLinux/ghdeb")
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	rec := st.Get(args[0])
	if rec == nil {
		// 尝试短名称
		owner, repo, resolveErr := resolvePkgArg(args[0])
		if resolveErr != nil {
			return fmt.Errorf("未找到 %s 的记录", args[0])
		}
		rec = st.Get(owner + "/" + repo)
		if rec == nil {
			return fmt.Errorf("未找到 %s 的记录", args[0])
		}
	}

	fmt.Printf("仓库: %s/%s\n", rec.Owner, rec.Repo)
	fmt.Printf("当前版本: %s\n", rec.CurrentVersion)
	fmt.Printf("架构: %s\n", rec.Arch)
	if rec.Removed {
		fmt.Printf("状态: ❌ 已移除\n")
	}

	rec.RefreshSystemInfo(rec.PkgName)
	if rec.SystemVersion != "" {
		fmt.Printf("系统版本: %s\n", rec.SystemVersion)
	}
	if rec.InstalledPath != "" {
		fmt.Printf("安装路径: %s\n", rec.InstalledPath)
	}

	fmt.Printf("\n操作历史:\n")
	fmt.Println(strings.Repeat("-", 80))
	for i, e := range rec.History {
		ts := e.Timestamp
		if ts != "auto-discovered" {
			ts = formatTime(ts)
		}
		switch e.Action {
		case state.ActionInstall:
			if e.Reinstall {
				fmt.Printf("  %d. [%s] REINSTALL %s\n", i+1, ts, e.Version)
			} else {
				fmt.Printf("  %d. [%s] INSTALL   %s\n", i+1, ts, e.Version)
			}
		case state.ActionUpgrade:
			fmt.Printf("  %d. [%s] UPGRADE   %s → %s\n", i+1, ts, e.FromVersion, e.Version)
		case state.ActionRemove:
			fmt.Printf("  %d. [%s] REMOVE    (was %s)\n", i+1, ts, e.Version)
		}
		if e.DebFile != "" {
			fmt.Printf("     文件: %s\n", e.DebFile)
		}
		if e.DebPath != "" {
			fmt.Printf("     路径: %s\n", e.DebPath)
		}
		if e.ReleaseURL != "" {
			fmt.Printf("     来源: %s\n", e.ReleaseURL)
		}
	}
	return nil
}

// --- remove ---

func cmdRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定包名，如: ghdeb remove bat")
	}

	owner, repo, err := resolvePkgArg(args[0])
	if err != nil {
		return err
	}
	repoKey := owner + "/" + repo

	st, err := state.Load()
	if err != nil {
		return err
	}
	if st.Get(repoKey) == nil {
		return fmt.Errorf("未找到 %s 的安装记录", repoKey)
	}
	st.MarkRemoved(repoKey)
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf(T("✅ 已标记移除 %s（历史记录已保留，软件本身未被卸载）\n", "✅ Marked %s as removed (history preserved, software not uninstalled)\n"), repoKey)
	return nil
}

// --- set-repo ---

func cmdSetRepo(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: ghdeb set-repo <pkg> <owner/repo>")
	}
	pkgName := args[0]
	owner, repo, err := gh.ParseRepo(args[1])
	if err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	if !st.SetRepo(pkgName, owner, repo) {
		return fmt.Errorf("未找到包 %s", pkgName)
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("保存状态失败: %w", err)
	}

	fmt.Printf("✅ 已设置 %s 的仓库为 %s/%s\n", pkgName, owner, repo)
	return nil
}

// --- test-homepage ---

func cmdTestHomepage(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: ghdeb test-homepage <url>")
	}
	url := args[0]
	fmt.Printf("测试: %s\n", url)
	owner, repo, err := state.FindGitHubFromHomepage(url)
	if err != nil {
		return fmt.Errorf("错误: %w", err)
	}
	fmt.Printf("找到: %s/%s\n", owner, repo)
	return nil
}

// --- 辅助函数 ---

func parseRepoSpec(s string) (repo, tag string) {
	if idx := strings.Index(s, "@"); idx >= 0 {
		return s[:idx], s[idx+1:]
	}
	return s, ""
}

func fetchRelease(client *gh.Client, owner, repo, tag string) (*gh.Release, error) {
	if tag != "" {
		fmt.Printf(T("📦 获取 release %s...\n", "📦 Fetching release %s...\n"), tag)
		return client.GetReleaseByTag(owner, repo, tag)
	}
	fmt.Printf(T("📦 获取最新 release...\n", "📦 Fetching latest release...\n"))
	return client.GetLatestRelease(owner, repo)
}

func downloadAsset(client *gh.Client, asset gh.Asset) (string, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", err
	}
	destPath := filepath.Join(cacheDir, asset.Name)
	fmt.Printf(T("⬇️  多线程下载中 (%d 线程): %s\n", "⬇️  Parallel downloading (%d threads): %s\n"), 4, asset.BrowserDownloadURL)
	err = client.DownloadAssetWithFallback(asset, destPath, func(downloaded, total int64) {
		printProgress(downloaded, total)
	})
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	fmt.Printf(T("✅ 下载完成: %s\n", "✅ Download complete: %s\n"), destPath)
	return destPath, nil
}

func installDeb(path string) error {
	fmt.Printf(T("📦 安装中 (dpkg -i)...\n", "📦 Installing (dpkg -i)...\n"))
	cmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "dpkg", "-i", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "debconf") || strings.Contains(errMsg, "configuration") {
			fmt.Printf(T("\n⚠️  此包需要交互式配置，请手动运行:\n", "\n⚠️  This package requires interactive configuration, please run manually:\n"))
			fmt.Printf("  sudo dpkg -i %s\n\n", path)
			return fmt.Errorf(T("需要交互式配置", "requires interactive configuration"))
		}

		fmt.Printf(T("🔧 尝试修复依赖 (apt-get install -f)...\n", "🔧 Trying to fix dependencies (apt-get install -f)...\n"))
		fixCmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-f", "-y")
		fixCmd.Stdout = os.Stdout
		fixCmd.Stderr = os.Stderr
		if fixErr := fixCmd.Run(); fixErr != nil {
			return fmt.Errorf(T("安装失败且依赖修复失败: %w", "install failed and dependency fix failed: %w"), fixErr)
		}
	}
	return nil
}

func getCacheDir() (string, error) {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".cache")
	}
	cacheDir := filepath.Join(dir, "ghdeb")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	return cacheDir, nil
}

func printProgress(downloaded, total int64) {
	if total <= 0 {
		fmt.Printf("\r⬇️  %s        ", formatSize(downloaded))
		return
	}
	pct := float64(downloaded) / float64(total) * 100
	fmt.Printf("\r⬇️  %s / %s (%.0f%%)", formatSize(downloaded), formatSize(total), pct)
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02 15:04:05")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
// displayWidth 计算字符串的显示宽度（中文字符占 2 个宽度）
func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF || // CJK 统一汉字
		   r >= 0x3000 && r <= 0x303F || // CJK 符号和标点
		   r >= 0xFF00 && r <= 0xFFEF || // 全角 ASCII 和半角形式
		   r >= 0x3400 && r <= 0x4DBF || // CJK 扩展 A
		   r >= 0x20000 && r <= 0x2A6DF { // CJK 扩展 B
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

// padRight 右填充空格到指定显示宽度
func padRight(s string, width int) string {
	currentWidth := displayWidth(s)
	if currentWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-currentWidth)
}



var _ = syscall.Geteuid

