// ghdeb - 从 GitHub Releases 安装/升级 .deb 包
package main

import (
	"fmt"
	"os"
	"sort"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/leisurelinux/ghdeb/internal/deb"
	gh "github.com/leisurelinux/ghdeb/internal/github"
	"github.com/leisurelinux/ghdeb/internal/state"
)

const version = "0.3.2"

func main() {
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
	case "scan":
		err = cmdScan(args)
	case "list", "ls":
		err = cmdList(args)
	case "history":
		err = cmdHistory(args)
	case "remove", "rm":
		err = cmdRemove(args)
	case "set-repo":
		err = cmdSetRepo(args)
	case "test-homepage":
		err = cmdTestHomepage(args)
	case "info":
		err = cmdInfo(args)
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
		fmt.Print(`ghdeb - 从 GitHub Releases 安装 .deb 包

用法:
  ghdeb install <owner/repo>[@tag]   安装（或升级到）指定版本
  ghdeb upgrade [owner/repo]         升级包（自动扫描 orphan，不指定则升级所有）
  ghdeb scan [--deep]                扫描系统中的 GitHub orphan 包并纳入管理
                                     --deep: 抓取 Homepage 页面查找 GitHub 链接
  ghdeb list [--refresh]             列出所有包（含已移除），--refresh 强制刷新版本缓存
  ghdeb history <owner/repo>         查看某包的完整操作历史
  ghdeb remove <owner/repo>          标记移除（保留历史记录）
  ghdeb set-repo <pkg> <owner/repo>  为包设置 GitHub 仓库
  ghdeb info <owner/repo>            查看最新 release 信息
  ghdeb version                      显示版本

环境变量:
  GITHUB_TOKEN / GH_TOKEN            GitHub 个人访问令牌（提高 API 限额）

示例:
  ghdeb install sharkdp/bat          安装 bat 最新版
  ghdeb scan [--deep]                扫描系统中的 GitHub orphan 包并纳入管理
                                     --deep: 抓取 Homepage 页面查找 GitHub 链接
  ghdeb upgrade                      升级所有已管理的包
  ghdeb history sharkdp/bat          查看 bat 的安装/升级/移除历史
  ghdeb set-repo draw.io jgraph/drawio  设置 draw.io 的仓库
`)
	} else {
		fmt.Print(`ghdeb - Install .deb packages from GitHub Releases

Usage:
  ghdeb install <owner/repo>[@tag]   Install (or upgrade to) specified version
  ghdeb upgrade [owner/repo]         Upgrade packages (auto-scan orphans, upgrade all if unspecified)
  ghdeb scan [--deep]                Scan system for GitHub orphan packages and add to management
                                     --deep: Fetch Homepage to find GitHub links
  ghdeb list [--refresh]             List all packages (including removed), --refresh to force refresh cache
  ghdeb history <owner/repo>         View complete operation history for a package
  ghdeb remove <owner/repo>          Mark as removed (preserve history)
  ghdeb set-repo <pkg> <owner/repo>  Set GitHub repository for a package
  ghdeb info <owner/repo>            View latest release info
  ghdeb version                      Show version

Environment Variables:
  GITHUB_TOKEN / GH_TOKEN            GitHub personal access token (increases API rate limit)

Examples:
  ghdeb install sharkdp/bat          Install latest bat
  ghdeb scan [--deep]                Scan system for GitHub orphan packages
                                     --deep: Fetch Homepage to find GitHub links
  ghdeb upgrade                      Upgrade all managed packages
  ghdeb history sharkdp/bat          View bat's install/upgrade/remove history
  ghdeb set-repo draw.io jgraph/drawio  Set draw.io's repository
`)
	}
}

// --- install ---

func cmdInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定仓库，如: ghdeb install sharkdp/bat")
	}

	repoStr, tag := parseRepoSpec(args[0])
	owner, repo, err := gh.ParseRepo(repoStr)
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

	// 提取 deb 包名
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

	fmt.Printf(T("🎉 安装完成: %s %s\n", "🎉 Install complete: %s %s\n"), repoKey, release.TagName)
	return nil
}

// --- upgrade ---

func cmdUpgrade(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}

	// 升级前自动扫描 orphan 包
	if len(args) == 0 {
		fmt.Println(T("🔍 扫描系统中的 GitHub orphan 包...", "🔍 Scanning GitHub orphan packages..."))
		orphans, scanErr := state.ScanOrphans(false, nil)
		if scanErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("扫描失败", "Scan failed"), scanErr)
		} else if len(orphans) > 0 {
			added := state.MergeOrphansToState(st, orphans)
			if added > 0 {
				fmt.Printf(T("📦 发现 %d 个新的 GitHub orphan 包，已纳入管理\n", "📦 Found %d new GitHub orphan packages, added to management\n"), added)
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

	// 解析参数：支持包名或 owner/repo
	type upgradeTarget struct {
		owner string
		repo  string
		pkg   *state.PackageRecord
	}
	var targets []upgradeTarget

	if len(args) > 0 {
		for _, arg := range args {
			// 先尝试作为 owner/repo 解析
			owner, repo, parseErr := gh.ParseRepo(arg)
			if parseErr == nil {
				repoKey := owner + "/" + repo
				rec := st.Get(repoKey)
				if rec != nil {
					targets = append(targets, upgradeTarget{owner: owner, repo: repo, pkg: rec})
					continue
				}
			}
			// 尝试作为包名查找
			rec := st.GetByPkgName(arg)
			if rec != nil && rec.Owner != "" && rec.Repo != "" {
				targets = append(targets, upgradeTarget{owner: rec.Owner, repo: rec.Repo, pkg: rec})
			} else {
				fmt.Fprintf(os.Stderr, "⚠️  %s %s: %s\n", T("跳过", "Skip"), arg, T("未找到该包或无仓库信息", "package not found or no repo info"))
			}
		}
	} else {
		for _, r := range st.ListActive() {
			if r.Owner != "" && r.Repo != "" {
				targets = append(targets, upgradeTarget{owner: r.Owner, repo: r.Repo, pkg: r})
			}
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
		// 升级时获取到的版本写入缓存
		client.SetCachedRelease(t.owner, t.repo, release.TagName)

		if t.pkg.CurrentVersion == release.TagName && !t.pkg.Removed {
			fmt.Printf(T("✅ 已是最新版本 %s\n", "✅ Already latest version %s\n"), release.TagName)
			continue
		}
		if !t.pkg.Removed {
			fmt.Printf(T("📦 发现新版本: %s → %s\n", "📦 New version found: %s → %s\n"), t.pkg.CurrentVersion, release.TagName)
		} else {
			fmt.Printf(T("📦 新版本: %s\n", "📦 New version: %s\n"), release.TagName)
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

		// 提取 deb 包名
		pkgName := deb.ExtractPkgName(destPath)

		if instErr := installDeb(destPath); instErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("安装失败", "Install failed"), instErr)
			continue
		}

		releaseURL := release.HTMLURL
		if releaseURL == "" {
			releaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", t.owner, t.repo, release.TagName)
		}

		if !t.pkg.Removed && t.pkg.CurrentVersion != "" {
			st.SetUpgrade(repoKey, release.TagName, asset.Name, destPath, releaseURL)
		} else {
			st.SetInstall(repoKey, t.owner, t.repo, release.TagName, asset.Name, destPath, releaseURL, arch.DpkgArch, pkgName)
		}
		upgraded++
		fmt.Printf(T("✅ 升级完成: %s %s\n", "✅ Upgrade complete: %s %s\n"), repoKey, release.TagName)
	}

	if saveErr := st.Save(); saveErr != nil {
		return fmt.Errorf("保存状态失败: %w", saveErr)
	}

	if upgraded == 0 {
		fmt.Println(T("\n所有包已是最新", "\nAll packages are up to date"))
	} else {
		fmt.Printf(T("\n🎉 共升级 %d 个包\n", "\n🎉 Upgraded %d packages\n"), upgraded)
	}
	return nil
}

// --- scan ---

func cmdScan(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}

	// 解析参数
	deepScan := false
	for _, arg := range args {
		if arg == "--deep" {
			deepScan = true
		}
	}

	if deepScan {
		fmt.Println(T("🔍 深度扫描系统中的 orphan 包（抓取 Homepage 查找 GitHub 链接）...", "🔍 Deep scanning orphan packages (fetching Homepage for GitHub links)..."))
	} else {
		fmt.Println(T("🔍 扫描系统中的 orphan 包（无 apt 源）...", "🔍 Scanning orphan packages (no apt source)..."))
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
		fmt.Println(T("未发现 GitHub 来源的 orphan 包", "No GitHub-sourced orphan packages found"))
		return nil
	}

	// 显示发现的包
	fmt.Printf(T("\n发现 %d 个 orphan 包:\n", "\nFound %d orphan packages:\n"), len(pkgs))
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

	// 询问是否纳入管理
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
	// 检查是否强制刷新缓存
	refresh := false
	for _, a := range args {
		if a == "--refresh" || a == "-r" {
			refresh = true
		}
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	records := st.List()
	if len(records) == 0 {
		fmt.Println(T("没有已管理的包", "No managed packages"))
		fmt.Println(T("提示: 运行 'ghdeb scan' 扫描系统中的 GitHub 来源包", "Hint: Run 'ghdeb scan' to discover GitHub-sourced packages"))
		return nil
	}

	fmt.Printf("%-35s %-12s %-12s %-12s %-10s %-19s\n", T("包名:仓库slug", "Pkg:Repo"), T("记录版本", "Recorded"), T("实际版本", "System"), T("最新版本", "Latest"), T("状态", "Status"), T("最后操作", "Updated"))
	fmt.Println(strings.Repeat("-", 110))

	client := gh.NewClient()

	// 强制刷新时清除全部缓存
	if refresh {
		gh.InvalidateCache("", "")
	}

	// 按包名排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].PkgName < records[j].PkgName
	})

	for _, r := range records {
		r.RefreshSystemInfo(r.PkgName)

		status := "✅ " + T("installed", "installed")
		if r.Removed {
			status = "❌ " + T("removed", "removed")
		}

		sysVer := r.SystemVersion
		if sysVer == "" {
			sysVer = "-"
		}

		// 获取最新版本：优先读缓存，缓存未命中才请求 API
		latestVer := "-"
		if !r.Removed && r.Owner != "" && r.Repo != "" {
			// 先查缓存
			cached := client.GetCachedRelease(r.Owner, r.Repo)
			if cached != "" {
				latestVer = cached
			} else {
				// 缓存未命中，请求 API
				release, apiErr := client.GetLatestRelease(r.Owner, r.Repo)
				if apiErr == nil {
					latestVer = release.TagName
					// 写入缓存
					client.SetCachedRelease(r.Owner, r.Repo, release.TagName)
				}
			}
			if latestVer != "-" && latestVer != r.CurrentVersion {
				status = "🔄 " + T("可升级", "upgradable")
			}
		}

		updatedAt := r.UpdatedAt
		if updatedAt != "auto-discovered" {
			updatedAt = formatTime(updatedAt)
		}

		// 拼接包名:仓库
		var repoPart string
		if r.Owner != "" && r.Repo != "" {
			repoPart = r.Owner + "/" + r.Repo
		} else {
			repoPart = T("无", "None")
		}
		pkgSlug := r.PkgName + ":" + repoPart

		fmt.Printf("%-35s %-12s %-12s %-12s %-10s %-19s\n",
			pkgSlug,
			r.CurrentVersion,
			sysVer,
			latestVer,
			status,
			updatedAt,
		)
	}
	return nil
}

// --- history ---

func cmdHistory(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定仓库，如: ghdeb history sharkdp/bat")
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	rec := st.Get(args[0])
	if rec == nil {
		return fmt.Errorf("未找到 %s 的记录", args[0])
	}

	fmt.Printf("仓库: %s/%s\n", rec.Owner, rec.Repo)
	fmt.Printf("当前版本: %s\n", rec.CurrentVersion)
	fmt.Printf("架构: %s\n", rec.Arch)
	if rec.Removed {
		fmt.Printf("状态: ❌ 已移除\n")
	}

	rec.RefreshSystemInfo(rec.Repo)
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
			fmt.Printf("  %d. [%s] INSTALL  %s\n", i+1, ts, e.Version)
		case state.ActionUpgrade:
			fmt.Printf("  %d. [%s] UPGRADE  %s → %s\n", i+1, ts, e.FromVersion, e.Version)
		case state.ActionRemove:
			fmt.Printf("  %d. [%s] REMOVE   (was %s)\n", i+1, ts, e.Version)
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
		return fmt.Errorf("请指定仓库，如: ghdeb remove sharkdp/bat")
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	repoKey := args[0]
	if st.Get(repoKey) == nil {
		return fmt.Errorf("未找到 %s 的安装记录", repoKey)
	}
	st.MarkRemoved(repoKey)
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("✅ 已标记移除 %s（历史记录已保留，软件本身未被卸载）\n", repoKey)
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

// --- info ---

func cmdInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(T("请指定仓库，如: ghdeb info sharkdp/bat", "Please specify a repo, e.g.: ghdeb info sharkdp/bat"))
	}
	owner, repo, err := gh.ParseRepo(args[0])
	if err != nil {
		return err
	}
	client := gh.NewClient()
	release, err := client.GetLatestRelease(owner, repo)
	if err != nil {
		return err
	}
	arch, _ := deb.DetectArch()

	result, findErr := gh.FindAssetWithFallback(release, arch)

	fmt.Printf(T("仓库: %s/%s\n", "Repo: %s/%s\n"), owner, repo)
	fmt.Printf(T("最新版本: %s\n", "Latest version: %s\n"), release.TagName)
	if release.Name != "" && release.Name != release.TagName {
		fmt.Printf(T("名称: %s\n", "Name: %s\n"), release.Name)
	}
	if release.HTMLURL != "" {
		fmt.Printf("Release: %s\n", release.HTMLURL)
	}

	if result != nil && result.Asset != nil {
		fmt.Printf(T("\n✅ 匹配当前架构 %s 的 .deb 文件:\n", "\n✅ Matched .deb file for arch %s:\n"), arch.DpkgArch)
		fmt.Printf("  → %s (%s)\n", result.Asset.Name, formatSize(result.Asset.Size))
	} else {
		fmt.Printf(T("\n⚠️  没有匹配当前架构 %s 的 .deb 文件\n", "\n⚠️  No .deb file matched for arch %s\n"), arch.DpkgArch)
		if findErr != nil {
			fmt.Printf("   %v\n", findErr)
		}
	}

	if result != nil && len(result.Fallbacks) > 0 {
		fmt.Printf(T("\n📦 其他可用的安装包（需手动下载安装）:\n", "\n📦 Other available packages (manual download required):\n"))
		for _, a := range result.Fallbacks {
			fmt.Printf("  %s (%s)\n", a.Name, formatSize(a.Size))
			fmt.Printf("    %s\n", a.BrowserDownloadURL)
		}
	}

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
	fmt.Printf(T("⬇️  下载中...\n", "⬇️  Downloading...\n"))
	err = client.DownloadAsset(asset, destPath, func(downloaded, total int64) {
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
	cmd := exec.Command("sudo", "dpkg", "-i", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 设置非交互式前端，避免 debconf 告警
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if err := cmd.Run(); err != nil {
		fmt.Printf(T("🔧 尝试修复依赖 (apt-get install -f)...\n", "🔧 Trying to fix dependencies (apt-get install -f)...\n"))
		fixCmd := exec.Command("sudo", "apt-get", "install", "-f", "-y")
		fixCmd.Stdout = os.Stdout
		fixCmd.Stderr = os.Stderr
		fixCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		if fixErr := fixCmd.Run(); fixErr != nil {
			return fmt.Errorf("安装失败且依赖修复失败: %w", fixErr)
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

var _ = syscall.Geteuid
