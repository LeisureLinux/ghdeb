// ghdeb - 从 GitHub Releases 安装/升级 .deb 包
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/leisurelinux/ghdeb/internal/catalog"
	"github.com/leisurelinux/ghdeb/internal/deb"
	gh "github.com/leisurelinux/ghdeb/internal/github"
	"github.com/leisurelinux/ghdeb/internal/state"
)

var version = "dev" // 构建时通过 -ldflags -X 注入，避免多版本号漏改

func main() {
	// 检查是否使用 --json，如果是则不打印 banner
	jsonMode := false
	if len(os.Args) >= 2 && (os.Args[1] == "list" || os.Args[1] == "ls" || os.Args[1] == "show" || os.Args[1] == "history") {
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
	case "update":
		err = cmdUpdate(args)
	case "reinstall":
		err = cmdReinstall(args)
	case "search":
		err = cmdSearch(args)
	case "list", "ls":
		err = cmdList(args)
	case "history":
		err = cmdHistory(args)
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
  ghdeb update [--verbose]              刷新各包最新/已装版本信息到本地快照（类 apt update）
  ghdeb update --verbose              显示后台各步骤与每个包检查明细
  ghdeb upgrade [pkg]                   升级包（不指定则升级所有）
  ghdeb reinstall <pkg>                 重新安装指定包
  ghdeb search <pattern>                在包目录中搜索
  ghdeb list [--json]                 列出目录所有条目（读取 update 生成的本地快照）
  ghdeb catalog show <name>             显示目录条目详情
  ghdeb catalog add <name> --repo <owner/repo>  添加条目到系统目录
  ghdeb catalog delete <name>           从系统目录删除条目
  ghdeb catalog modify <name> --repo <owner/repo>  修改目录条目的仓库
  ghdeb catalog validate <name>        校验目录条目（移除无 .deb 的条目）
  ghdeb catalog validate --all         校验清洗全部条目（移除无 .deb 的条目）
  ghdeb show <pkg>                      显示包的完整信息
  ghdeb history <pkg>                   查看某包的完整操作历史
  ghdeb purge <pkg>                     卸载软件并清除配置文件
  ghdeb clean [--dry-run]               清理下载的 .deb 缓存
  ghdeb info <pkg>                      show 的别名
  ghdeb version                         显示版本

包目录:
  系统目录: /etc/ghdeb/catalog.toml

环境变量:
  GITHUB_TOKEN / GH_TOKEN               GitHub 个人访问令牌（提高 API 限额）

示例:
  ghdeb install bat                     通过短名称安装 bat
  ghdeb install sharkdp/bat             通过 owner/repo 安装
  ghdeb install LeisureLinux/ghdeb@v0.6.0  安装指定版本
  ghdeb update                          刷新版本信息后再 list
  ghdeb search monitor                  搜索包含 monitor 的包
  ghdeb catalog add myapp --repo user/myapp --summary "我的应用"
  ghdeb show rustdesk                   显示包信息
  ghdeb clean                           清理缓存
  ghdeb purge rustdesk                  卸载 rustdesk
`)
	} else {
		fmt.Print(`ghdeb - Manage .deb packages from GitHub Releases

Usage:
  ghdeb install <pkg|owner/repo>[@tag]  Install (short name or owner/repo)
  ghdeb update [--verbose]              Refresh latest/installed version info into local snapshot (like apt update)
  ghdeb update --verbose              Show background steps & per-package check details
  ghdeb upgrade [pkg]                   Upgrade packages (all if unspecified)
  ghdeb reinstall <pkg>                 Reinstall a package
  ghdeb search <pattern>                Search in package catalog
  ghdeb list [--json]                  List all catalog entries (reads local update snapshot)
  ghdeb catalog show <name>             Show catalog entry details
  ghdeb catalog add <name> --repo <owner/repo>  Add entry to system catalog
  ghdeb catalog delete <name>           Remove entry from system catalog
  ghdeb catalog modify <name> --repo <owner/repo>  Modify catalog entry repo
  ghdeb catalog validate <name>         Validate entry (remove those with no .deb)
  ghdeb catalog validate --all          Validate all entries (remove no-.deb)
  ghdeb show <pkg>                      Show package details
  ghdeb history <pkg>                   View operation history
  ghdeb purge <pkg>                     Uninstall and purge config
  ghdeb clean [--dry-run]               Clean .deb cache
  ghdeb info <pkg>                      Alias for show
  ghdeb version                         Show version

Catalog:
  System: /etc/ghdeb/catalog.toml

Environment Variables:
  GITHUB_TOKEN / GH_TOKEN               GitHub personal access token

Examples:
  ghdeb install bat                     Install via short name
  ghdeb install sharkdp/bat             Install via owner/repo
  ghdeb update                          Refresh version info before list
  ghdeb install LeisureLinux/ghdeb@v0.6.0  Install specific version
  ghdeb search monitor                  Search catalog
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
	if existing := st.Get(repoKey); existing != nil && !existing.Removed {
		// 以系统实际安装版本为准，避免 state 里陈旧的 current_version 误判
		pkgName := existing.PkgName
		if pkgName == "" {
			pkgName = existing.Repo
		}
		sysVer := state.QuerySystemVersion(pkgName)
		if sysVer != "" && versionEqual(sysVer, release.TagName) {
			fmt.Printf(T("✅ %s 已安装版本 %s，无需重复安装\n", "✅ %s version %s already installed, no need to reinstall\n"), repo, release.TagName)
			return nil
		}
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

	// 命令行直接安装（非目录短名称）时，若该仓库尚未收录且存在合理 .deb，
	// 自动将其加入系统目录 catalog.toml（安装校验已确认有当前架构的 .deb 资产）
	catalogShortName := catalogName
	if catalogShortName == "" {
		catalogShortName = ensureCatalogAfterInstall(owner, repo, repoKey)
	}

	// 安装成功后更新统一缓存快照（list/upgrade 依赖 cache.json）
	if err := updateCacheAfterInstall(catalogShortName, repoKey, release, asset, arch.DpkgArch); err != nil {
		return fmt.Errorf("更新缓存失败: %w", err)
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

// ensureCatalogAfterInstall 命令行直接安装 owner/repo 成功后，将仓库自动加入系统目录。
// 返回最终使用的 catalog 短名称；若无法加入（重名冲突等）则返回空字符串。
func ensureCatalogAfterInstall(owner, repo, repoKey string) string {
	path := catalog.SystemCatalogPath()
	entries := make(map[string]catalog.CatalogEntry)
	if data, err := os.ReadFile(path); err == nil {
		toml.Decode(string(data), &entries)
	}

	// 仓库已在目录中（无论短名是否与 repo 同名）→ 无需重复加入
	for name, e := range entries {
		if e.Repo == repoKey {
			return name
		}
	}

	// 候选短名：优先 repo 名，冲突（被其他仓库占用）时回退为 owner-repo
	candidates := []string{strings.ToLower(repo), strings.ToLower(owner) + "-" + strings.ToLower(repo)}
	for _, cand := range candidates {
		if catalog.ValidateCatalogName(cand) != nil {
			continue
		}
		if _, exists := entries[cand]; exists {
			continue // 该短名已被其他仓库占用，尝试下一候选
		}
		entry := &catalog.CatalogEntry{
			Repo:       repoKey,
			PrettyName: repo,
			Website:    fmt.Sprintf("https://github.com/%s", repoKey),
			Summary:    T("从 GitHub Releases 安装的软件包", "Package installed from GitHub Releases"),
		}
		if err := addToSystemCatalog(cand, entry); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s %s: %v\n", T("自动加入目录失败", "Failed to auto-add to catalog"), cand, err)
			return ""
		}
		fmt.Printf(T("📝 已自动加入目录: %s → %s\n", "📝 Auto-added to catalog: %s → %s\n"), cand, repoKey)
		return cand
	}
	return ""
}

// updateCacheAfterInstall 安装成功后更新统一缓存快照，使 list/upgrade 立即反映最新状态。
func updateCacheAfterInstall(shortName, repoKey string, release *gh.Release, asset *gh.Asset, arch string) error {
	if shortName == "" {
		return nil
	}
	snap := state.LoadCache()
	snap.Set(shortName, &state.PkgState{
		Name:             shortName,
		Installed:        true,
		InstallTime:      time.Now().Format(time.RFC3339),
		InstalledVersion: release.TagName,
		Repo:             repoKey,
		GitHubVersion:    release.TagName,
		Upgradable:       false,
		Arch:             arch,
		PkgFile:          asset.Name,
	})
	snap.UpdatedAt = time.Now().Format(time.RFC3339)
	return state.SaveCache(snap)
}

// --- upgrade ---

func cmdUpgrade(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
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
		// 无参数：按 cache.json 快照驱动，仅收集「已安装且可升级」的条目
		// （update 已扫描 OS 已装状态与 GitHub 最新版本，不必再依赖 history 管理记录）
		snap := state.LoadCache()
		cat, catErr := catalog.Load()
		if catErr != nil {
			return fmt.Errorf("加载目录失败: %w", catErr)
		}
		for _, name := range snap.SortedNames() {
			sp := snap.Get(name)
			if sp == nil || !sp.Installed || !sp.Upgradable {
				continue
			}
			entry := cat.Lookup(name)
			if entry == nil || entry.Repo == "" {
				continue
			}
			owner, repo, perr := gh.ParseRepo(entry.Repo)
			if perr != nil {
				continue
			}
			repoKey := owner + "/" + repo
			rec := st.Get(repoKey)
			if rec != nil && rec.Removed {
				continue
			}
			targets = append(targets, upgradeTarget{owner: owner, repo: repo, pkg: rec})
		}
	}

	if len(targets) == 0 {
		fmt.Println(T("没有可升级的包", "No packages to upgrade"))
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

		// 判断是否已是最新：以系统实际安装版本为准，避免 state 里陈旧的 current_version 误判
		alreadyLatest := false
		if t.pkg != nil && !t.pkg.Removed {
			pkgName := t.pkg.PkgName
			if pkgName == "" {
				pkgName = t.pkg.Repo
			}
			sysVer := state.QuerySystemVersion(pkgName)
			if sysVer != "" && versionEqual(sysVer, release.TagName) {
				alreadyLatest = true
			}
		}
		if alreadyLatest {
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

		// 升级/安装成功后同样更新统一 cache.json 快照，使 list/upgrade 立即反映最新状态
		shortName := ensureCatalogAfterInstall(t.owner, t.repo, repoKey)
		if err := updateCacheAfterInstall(shortName, repoKey, release, asset, arch.DpkgArch); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", T("更新缓存失败", "Update cache failed"), err)
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

	// 加载 cache 快照检查安装状态（不依赖 history 管理记录）
	snap := state.LoadCache()

	fmt.Printf(T("找到 %d 个匹配的包:\n", "Found %d matching packages:\n"), len(results))
	fmt.Printf("%-20s %-30s %s\n", T("名称", "Name"), T("仓库", "Repo"), T("简介", "Summary"))
	fmt.Println(strings.Repeat("-", 80))

	for _, r := range results {
		entry := r.Entry
		repoKey := entry.Repo
		installed := false
		if sp := snap.Get(r.Name); sp != nil {
			installed = sp.Installed
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
		ShortName   string `json:"short_name,omitempty"`
		Repo        string `json:"repo"`
		PrettyName  string `json:"pretty_name,omitempty"`
		Website     string `json:"website,omitempty"`
		Summary     string `json:"summary,omitempty"`
		Status      string `json:"status,omitempty"`
		RecordedVer string `json:"recorded_version,omitempty"`
		SystemVer   string `json:"system_version,omitempty"`
		Arch        string `json:"arch,omitempty"`
		DebName     string `json:"deb_name,omitempty"`
		LatestVer   string `json:"latest_version,omitempty"`
		ReleaseURL  string `json:"release_url,omitempty"`
		MatchFile   string `json:"match_file,omitempty"`
		MatchSize   string `json:"match_size,omitempty"`
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

	// 安装状态：优先 history 管理记录；无管理记录（如 apt 安装的包）则回退 cache.json 快照
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
	} else {
		// 无管理记录：回退 cache.json 快照（ghdeb update 已扫描 OS 已装状态）
		if sp := state.LoadCache().Get(catName); sp != nil {
			if !jsonOutput {
				fmt.Println(strings.Repeat("─", 50))
			}
			if sp.Installed {
				info.Status = "installed"
				if !jsonOutput {
					fmt.Printf(T("状态:     ✅ 已安装\n", "Status:      ✅ installed\n"))
				}
			} else {
				info.Status = "not-installed"
				if !jsonOutput {
					fmt.Printf(T("状态:     ⬜ 未安装\n", "Status:      ⬜ not installed\n"))
				}
			}
			if sp.InstalledVersion != "" {
				info.SystemVer = sp.InstalledVersion
				if !jsonOutput {
					fmt.Printf(T("系统版本: %s\n", "System:      %s\n"), sp.InstalledVersion)
				}
			}
			if sp.Arch != "" {
				info.Arch = sp.Arch
				if !jsonOutput {
					fmt.Printf(T("架构:     %s\n", "Arch:        %s\n"), sp.Arch)
				}
			}
			if sp.PkgFile != "" {
				info.DebName = sp.PkgFile
				if !jsonOutput {
					fmt.Printf(T("deb 包名: %s\n", "Deb name:    %s\n"), sp.PkgFile)
				}
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
		return fmt.Errorf("请指定子命令: show, search, add, modify, delete, validate")
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "show":
		return cmdCatalogShow(subargs)
	case "search":
		return cmdSearch(subargs) // 复用 search 逻辑
	case "add":
		return cmdCatalogAdd(subargs)
	case "delete", "del", "rm":
		return cmdCatalogDelete(subargs)
	case "modify":
		return cmdCatalogModify(subargs)
	case "validate":
		return cmdCatalogValidate(subargs)
	default:
		return fmt.Errorf("未知子命令: %s（可用: show, search, add, modify, delete, validate）", subcmd)
	}
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

	// 添加前校验：GitHub 条目必须提供当前架构的 .deb，否则不允许添加
	if entry.Repo != "" {
		owner, repo, perr := gh.ParseRepo(entry.Repo)
		if perr != nil {
			return fmt.Errorf("添加失败: 无效的仓库格式 %s", entry.Repo)
		}
		has, vErr := checkRepoDeb(owner, repo)
		if vErr != nil {
			return fmt.Errorf("添加失败: 校验 %s 失败: %w", entry.Repo, vErr)
		}
		if !has {
			archName := T("当前架构", "current arch")
			if arch, aErr := deb.DetectArch(); aErr == nil {
				archName = arch.DpkgArch
			}
			return fmt.Errorf("添加失败: %s 最新 Release 未提供 %s 架构的 .deb 包", entry.Repo, archName)
		}
	}

	if err := addToSystemCatalog(name, &entry); err != nil {
		return fmt.Errorf("添加失败: %w", err)
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

// cmdCatalogValidate 校验目录：移除未提供当前架构 .deb 的 GitHub 条目。
// 支持单包（ghdeb catalog validate <name|owner/repo>）与全量（ghdeb catalog validate --all）。
func cmdCatalogValidate(args []string) error {
	all := false
	var name string
	for _, arg := range args {
		switch arg {
		case "--all", "-a":
			all = true
		default:
			if strings.HasPrefix(arg, "-") {
				// 忽略未知选项
			} else if name == "" {
				name = arg
			}
		}
	}

	if !all && name == "" {
		return fmt.Errorf("请指定包名，或用 --all 清理目录中所有无 .deb 的条目")
	}

	// 加载系统目录
	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}
	entries := cat.AllEntries()
	if len(entries) == 0 {
		fmt.Println(T("目录为空", "Catalog is empty"))
		return nil
	}

	// 确定待检测的条目集合（短名称 -> 条目）
	candidates := make(map[string]*catalog.CatalogEntry)
	if all {
		for k, e := range entries {
			candidates[k] = e
		}
	} else {
		// 支持短名称或 owner/repo 两种写法
		entry := entries[name]
		if entry == nil {
			for k, e := range entries {
				if e.Repo == name {
					entry = e
					name = k
					break
				}
			}
		}
		if entry == nil {
			return fmt.Errorf("系统目录中未找到 %s", name)
		}
		candidates[name] = entry
	}

	if all {
		fmt.Println(T("🔍 清洗目录：检查所有条目是否提供当前架构的 .deb ...",
			"🔍 Cleaning catalog: checking all entries provide a current-arch .deb ..."))
	} else {
		fmt.Printf(T("🔍 清洗目录：检查 %s 是否提供当前架构的 .deb ...\n",
			"🔍 Cleaning catalog: checking %s provides a current-arch .deb ...\n"), name)
	}

	arch, aErr := deb.DetectArch()
	if aErr != nil {
		return fmt.Errorf("检测系统架构失败: %w", aErr)
	}

	names := make([]string, 0, len(candidates))
	for k := range candidates {
		names = append(names, k)
	}
	sort.Strings(names)

	// 操作前先备份目录文件：中途出错或文件损坏可回退到操作前完整状态
	path := catalog.SystemCatalogPath()
	backupPath := path + ".cleanbak"
	if err := backupCatalogFile(path, backupPath); err != nil {
		return fmt.Errorf("创建目录备份失败: %w", err)
	}
	fmt.Printf(T("📦 已备份目录到 %s（出错时将自动回退）\n",
		"📦 Backed up catalog to %s (will auto-restore on error)\n"), backupPath)

	// 加载当前目录内容，用于分批删除与写回
	curData, rErr := os.ReadFile(path)
	if rErr != nil {
		return fmt.Errorf("读取目录失败: %w", rErr)
	}
	current := make(map[string]catalog.CatalogEntry)
	toml.Decode(string(curData), &current)

	toRemove := make(map[string]bool)
	skippedNet := 0
	total := len(names)

	// 成功标志：正常走完则删除备份，出错则回退备份
	success := false
	defer func() {
		if success {
			os.Remove(backupPath)
			fmt.Println(T("✅ 已删除临时备份文件", "✅ Temporary backup removed"))
		} else if restoreCatalogFile(backupPath, path) == nil {
			fmt.Printf(T("⚠️  操作未完成，已从备份回退目录文件 %s\n",
				"⚠️  Operation incomplete, catalog %s restored from backup\n"), path)
		}
	}()

	// 分批处理：每处理一批（20 个）就写一次盘，降低中断损失
	const batchSize = 20
	processed := 0
	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		batch := names[start:end]

		for _, k := range batch {
			e := candidates[k]
			// ghdeb 自身永远保留
			if k == "ghdeb" {
				continue
			}
			// 非 GitHub 直接 URL 条目无法校验 .deb，保留不动
			if e.IsDirectURL() || e.Repo == "" {
				continue
			}
			owner, repo, perr := gh.ParseRepo(e.Repo)
			if perr != nil {
				continue
			}
			fmt.Printf("检查 %s（%s）...\n", k, e.Repo)
			has, cErr := checkRepoDeb(owner, repo)
			if cErr != nil {
				// 网络/API 错误：保留该条目，跳过，避免误删
				fmt.Printf("  🌐 网络/API 错误，跳过保留：%v\n", cErr)
				skippedNet++
				continue
			}
			if has {
				fmt.Printf("  ✅ 最新 Release 提供 %s 架构的 .deb，保留\n", arch.DpkgArch)
			} else {
				fmt.Printf("  ⚠️  最新 Release 无 %s 架构的 .deb，将从目录删除\n", arch.DpkgArch)
				toRemove[k] = true
				delete(current, k)
			}
		}
		processed = end

		// 每批写盘
		if err := writeSystemCatalog(path, current, "ghdeb"); err != nil {
			return fmt.Errorf("写入目录失败: %w", err)
		}
		fmt.Printf(T("  已处理 %d/%d 个条目并写盘（累计删除 %d 个）\n",
			"  Processed %d/%d entries and flushed (%d removed so far)\n"),
			processed, total, len(toRemove))
	}

	if skippedNet > 0 {
		fmt.Printf(T("🌐 因网络/API 错误跳过 %d 个条目（已保留，可重跑 validate --all）\n",
			"🌐 Skipped %d entries due to network/API errors (kept, re-run validate --all)\n"), skippedNet)
	}

	if len(toRemove) == 0 {
		if all {
			fmt.Println(T("✅ 目录中所有条目均提供 .deb，无需清理", "✅ All catalog entries provide .deb, nothing to clean"))
		} else {
			fmt.Println(T("✅ 该条目提供 .deb，无需清理", "✅ This entry provides .deb, nothing to clean"))
		}
	} else {
		fmt.Printf(T("✅ 已从目录清理 %d 个无 .deb 的条目\n", "✅ Removed %d entries without .deb from catalog\n"), len(toRemove))
		for k := range toRemove {
			fmt.Printf("  - %s\n", k)
		}
	}

	success = true
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

	// 支持短名称和 owner/repo 两种写法，与 show 的 repo 反查保持一致
	key := name
	if _, ok := entries[key]; !ok {
		found := ""
		for k, e := range entries {
			if e.Repo == name {
				found = k
				break
			}
		}
		if found == "" {
			return fmt.Errorf("系统目录中未找到 %s", name)
		}
		key = found
	}

	delete(entries, key)
	return writeSystemCatalog(path, entries)
}

// writeSystemCatalog 写入系统级目录文件
func writeSystemCatalog(path string, entries map[string]catalog.CatalogEntry, firstKeys ...string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# ghdeb 包目录 (Known Packages Catalog)\n")
	sb.WriteString("# 路径: " + path + "\n#\n")
	sb.WriteString("# 维护本目录可使用命令：ghdeb catalog show, search, add, modify, delete, validate\n")
	// 仅维护首次初始化时间：文件已记录则保留原值，否则写当前时间
	initAt := time.Now().Format("2006-01-02 15:04:05")
	if data, rErr := os.ReadFile(path); rErr == nil {
		marker := "# Catalog initialized at "
		if i := strings.Index(string(data), marker); i >= 0 {
			rest := string(data)[i+len(marker):]
			if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
				initAt = rest[:nl]
			}
		}
	}
	sb.WriteString("# Catalog initialized at " + initAt + "\n\n")

	// 输出顺序：firstKeys 强制排最前，其余按名称排序
	ordered := make([]string, 0, len(firstKeys)+len(entries))
	seen := make(map[string]bool)
	for _, k := range firstKeys {
		if _, ok := entries[k]; ok && !seen[k] {
			ordered = append(ordered, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(entries))
	for name := range entries {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)

	for _, name := range ordered {
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
		if err := state.SudoRemove(path); err != nil {
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

	arg := args[0]

	// 解析：优先 owner/repo，其次 catalog 短名
	cat, _ := catalog.Load()
	shortName := ""
	if cat != nil && cat.Lookup(arg) != nil {
		shortName = arg
	}
	owner, repo, err := resolvePkgArg(arg)
	if err != nil {
		return err
	}
	repoKey := owner + "/" + repo
	// 若传入的是 owner/repo 而非短名，反查 catalog 短名作为 cache 键
	if shortName == "" && cat != nil {
		for cn, ce := range cat.AllEntries() {
			if ce.Repo == repoKey {
				shortName = cn
				break
			}
		}
	}
	if shortName == "" {
		shortName = repo // 以 repo 尾段作为候选短名
	}

	// 从 dpkg 系统状态反查实际已装包名（不依赖 history.json 管理记录）
	installed := deb.ScanInstalledDpkg()
	var pkgName string
	var pkgVersion string
	if installed != nil {
		for _, cand := range deb.CandidatePkgNames(shortName, repoKey) {
			if installed[cand] != nil {
				pkgName = cand
				pkgVersion = installed[cand].Version
				break
			}
		}
	}

	if pkgName == "" {
		// 回退：history 管理记录里记录的包名
		if st, lerr := state.Load(); lerr == nil {
			if rec := st.Get(repoKey); rec != nil {
				if rec.PkgName != "" {
					pkgName = rec.PkgName
				}
			}
		}
	}

	if pkgName == "" {
		// 未知包名：尝试用 repo 尾段
		pkgName = repo
	}

	// 检查是否已安装
	purged := false
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
		purged = true

		// autoremove
		fmt.Printf(T("🧹 清理依赖 (apt-get autoremove)...\n", "🧹 Cleaning dependencies (apt-get autoremove)...\n"))
		autoCmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "autoremove", "-y")
		autoCmd.Stdout = os.Stdout
		autoCmd.Stderr = os.Stderr
		_ = autoCmd.Run() // autoremove 失败不阻塞
	}

	// 同步 history.json：存在管理记录则标记移除；
	// 若包从未被 ghdeb 管理（如 apt 安装）但确实被 purge，则补建一条 remove 历史，便于 ghdeb history 查询
	if st, lerr := state.Load(); lerr == nil {
		if rec := st.Get(repoKey); rec != nil {
			st.MarkRemoved(repoKey)
			_ = st.Save()
		} else if purged {
			st.RecordPurge(repoKey, owner, repo, pkgName, pkgVersion)
			_ = st.Save()
		}
	}

	// 同步 cache.json：将已装状态置为未装
	snap := state.LoadCache()
	if sp := snap.Get(shortName); sp != nil {
		sp.Installed = false
		sp.InstallTime = ""
		sp.InstalledVersion = ""
		sp.Upgradable = false
		_ = state.SaveCache(snap)
	}

	fmt.Printf(T("✅ 已卸载并清除 %s\n", "✅ Purged %s\n"), repoKey)
	return nil
}

// checkRepoDeb 查询 GitHub 仓库最新 Release 是否提供当前架构的 .deb。
// 返回 hasDeb 表示是否存在匹配的 .deb；err 非 nil 表示网络/API 错误（并非"无 .deb"）。
func checkRepoDeb(owner, repo string) (bool, error) {
	arch, err := deb.DetectArch()
	if err != nil {
		return false, err
	}
	client := gh.NewClient()
	release, relErr := client.GetLatestRelease(owner, repo)
	if relErr != nil {
		if errors.Is(relErr, gh.ErrNoStableRelease) {
			// 无稳定 release（全为 draft/prerelease）→ 视为无匹配 .deb，由调用方删除该条目
			return false, nil
		}
		return false, relErr
	}
	result, _ := gh.FindAssetWithFallback(release, arch)
	return result != nil && result.Asset != nil, nil
}

// writeFileMaybeSudo 写文件；权限不足时降级用 sudo tee。
func writeFileMaybeSudo(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0644); err != nil {
		if errors.Is(err, os.ErrPermission) {
			cmd := exec.Command("sudo", "tee", path)
			cmd.Stdin = strings.NewReader(string(data))
			cmd.Stdout = nil
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("写入失败（sudo 被拒绝或失败）: %w", err)
			}
			return nil
		}
		return err
	}
	return nil
}

// backupCatalogFile 将目录文件复制为临时备份（用于出错回退）。
func backupCatalogFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFileMaybeSudo(dst, data)
}

// restoreCatalogFile 从备份恢复目录文件。
func restoreCatalogFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFileMaybeSudo(dst, data)
}

// compareVersion 比较两个软件版本，返回 -1(a<b) / 0(a==b) / 1(a>b)。
// 归一化：去掉开头 v/V、忽略 epoch(1:xxx) 前缀；按 [.-_] 分段，
// 数字段按数值比较，非数字段按字典序比较，缺省段视为更小。
// 由此保证「已装版本 >= GitHub 最新版本」被判定为正常（不提示可升级）。
func compareVersion(a, b string) int {
	norm := func(s string) []string {
		s = strings.TrimPrefix(s, "v")
		s = strings.TrimPrefix(s, "V")
		if i := strings.IndexByte(s, ':'); i >= 0 {
			s = s[i+1:] // 忽略 epoch
		}
		return strings.FieldsFunc(s, func(r rune) bool {
			return r == '.' || r == '-' || r == '_'
		})
	}
	as, bs := norm(a), norm(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ap, bp string
		if i < len(as) {
			ap = as[i]
		}
		if i < len(bs) {
			bp = bs[i]
		}
		an, aErr := strconv.Atoi(ap)
		bn, bErr := strconv.Atoi(bp)
		switch {
		case aErr == nil && bErr == nil:
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		case aErr == nil && bErr != nil:
			// 数字段 > 空/字母段（如 1.2.4-1 的 -1 版本号高于 1.2.4）
			return 1
		case aErr != nil && bErr == nil:
			return -1
		default:
			if ap < bp {
				return -1
			}
			if ap > bp {
				return 1
			}
		}
	}
	return 0
}

// --- list ---

func cmdList(args []string) error {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	// 加载 catalog（纯本地，仅用于名称/仓库/URL 展示）
	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}
	entries := cat.AllEntries()

	// 只读快照缓冲（由 ghdeb update 生成）
	snap := state.LoadCache()

	names := cat.SortedNames()
	if len(names) == 0 {
		fmt.Println(T("目录为空", "Catalog is empty"))
		return nil
	}

	// JSON 输出结构
	type listPkg struct {
		Name             string `json:"name"`
		Repo             string `json:"repo"`
		URL              string `json:"url,omitempty"`
		Summary          string `json:"summary,omitempty"`
		Installed        bool   `json:"installed"`
		InstalledVersion string `json:"installed_version,omitempty"`
		LatestVersion    string `json:"latest_version,omitempty"`
		Upgradable       bool   `json:"upgradeable"`
	}
	var jsonPkgs []listPkg
	installedCount := 0
	upgradeableCount := 0

	if !jsonOutput {
		fmt.Printf("%s %s %s %s %s\n",
			padRight(T("名称", "Name"), 20), padRight(T("仓库/URL", "Repo/URL"), 25),
			padRight(T("已装版本", "Installed"), 14), padRight(T("最新版本", "Latest"), 12),
			T("状态", "Status"))
		fmt.Println(strings.Repeat("-", 80))
	}

	for _, name := range names {
		entry := entries[name]

		repoOrURL := entry.Repo
		if repoOrURL == "" {
			repoOrURL = truncate(entry.URL, 23)
		}

		// 从快照读取（未在快照中视为无数据）
		sn := snap.Get(name)
		installed := false
		installedVer := ""
		latest := ""
		upgradeable := false
		if sn != nil {
			installed = sn.Installed
			installedVer = sn.InstalledVersion
			latest = sn.GitHubVersion
			upgradeable = sn.Upgradable
		}

		if installed {
			installedCount++
		}
		if upgradeable {
			upgradeableCount++
		}

		status := ""
		switch {
		case !installed:
			status = T("未装", "not installed")
		case upgradeable:
			status = "⬆️ " + T("可升级", "upgradeable")
		default:
			status = "✅"
		}

		if jsonOutput {
			jsonPkgs = append(jsonPkgs, listPkg{
				Name: name, Repo: entry.Repo, URL: entry.URL,
				Summary: truncate(entry.Summary, 35), Installed: installed,
				InstalledVersion: installedVer, LatestVersion: latest,
				Upgradable: upgradeable,
			})
		} else {
			iv, lv := installedVer, latest
			if iv == "" {
				iv = "-"
			}
			if lv == "" {
				lv = "-"
			}
			fmt.Printf("%s %s %s %s %s\n",
				padRight(name, 20), padRight(truncate(repoOrURL, 23), 25),
				padRight(iv, 14), padRight(lv, 12), status)
		}
	}

	if jsonOutput {
		jsonData, _ := json.MarshalIndent(jsonPkgs, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println(strings.Repeat("-", 80))
		notInstalled := len(names) - installedCount
		fmt.Printf(T("目录共 %d 个软件包，未安装 %d 个，已安装 %d 个，其中可升级 %d 个\n",
			"Catalog has %d packages: %d not installed, %d installed, %d upgradeable\n"),
			len(names), notInstalled, installedCount, upgradeableCount)
	}
	return nil
}

// cmdUpdate 类似 apt update：查询各包最新版本、本地已装版本，判定可升级性，
// 校验并移除无 .deb 的目录条目，把结果写入 list 快照缓冲文件。
func cmdUpdate(args []string) error {
	// --verbose 开关：显示后台各步骤与每个包的检查明细
	verbose := false
	cmdArgs := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--verbose" || a == "-verbose" {
			verbose = true
		} else {
			cmdArgs = append(cmdArgs, a)
		}
	}
	vlog := func(format string, v ...interface{}) {
		if verbose {
			fmt.Fprintf(os.Stderr, "  "+format+"\n", v...)
		}
	}

	cat, err := catalog.Load()
	if err != nil {
		return fmt.Errorf("加载目录失败: %w", err)
	}
	entries := cat.AllEntries()
	if len(entries) == 0 {
		fmt.Println(T("目录为空，无需更新", "Catalog is empty, nothing to update"))
		return nil
	}

	client := gh.NewClient()
	names := cat.SortedNames()
	vlog("目录共 %d 个软件包", len(names))

	// 检测当前架构（一次即可）
	sysArch := ""
	var archInfo *deb.ArchInfo
	if ai, aErr := deb.DetectArch(); aErr == nil && ai != nil {
		sysArch = ai.DpkgArch
		archInfo = ai
	}
	vlog("目标架构: %s", sysArch)

	// 1) 收集所有需检查的 GitHub 仓库条目
	type task struct{ name, owner, repo string }
	var tasks []task
	for _, name := range names {
		entry := entries[name]
		if entry.Repo == "" {
			continue
		}
		owner, repo, perr := gh.ParseRepo(entry.Repo)
		if perr != nil {
			continue
		}
		tasks = append(tasks, task{name, owner, repo})
	}
	total := len(tasks)
	if total == 0 {
		fmt.Println(T("目录中没有可检查的 GitHub 软件包", "No GitHub packages to check in catalog"))
		return nil
	}

	// 进展提示：需检查的包数与预计耗时
	mm := estimateMinutes(total)
	fmt.Printf(T("需要去 github 网站检查 catalog 内 %d 个软件包的最新版本，预计需要 %d 分钟\n",
		"Need to check latest versions of %d packages on GitHub, estimated ~%d minutes\n"),
		total, mm)

	// 2) 并发检查（8 路限流），单行进度条实时刷新。
	//    每取到一个 release，同时校验当前架构是否有对应 .deb 资产，
	//    把"版本号 + 是否有 .deb"一并拿到，避免二次网络往返。
	latestVer := make(map[string]string) // key = "owner/repo"
	hasDeb := make(map[string]bool)      // key = "owner/repo"，是否含当前架构 .deb
	var (
		mu         sync.Mutex // 保护 latestVer / hasDeb / 缓存写入
		progressMu sync.Mutex // 保护进度条输出与完成计数
		done       int
	)
	// printProgress 刷新进度条（\r 单行覆盖）；verbose 时改为逐行明细
	printProgress := func(name, owner, repo, note string) {
		progressMu.Lock()
		done++
		if verbose {
			fmt.Fprintf(os.Stderr, "  [%d/%d] %-20s <%s/%s> %s\n",
				done, total, name, owner, repo, note)
		} else {
			fmt.Fprintf(os.Stderr, "\r  %s %s，<%s/%s> ...  [%d/%d]  ",
				T("正在检查", "Checking"), name, owner, repo, done, total)
		}
		progressMu.Unlock()
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			note := ""
			// 缓存命中直接取缓存版本（快速推进进度条），否则请求 GitHub
			ver := client.GetCachedRelease(t.owner, t.repo)
			if ver == "" {
				if rel, relErr := client.GetLatestRelease(t.owner, t.repo); relErr == nil {
					ver = rel.TagName
					mu.Lock()
					client.SetCachedRelease(t.owner, t.repo, ver)
					result, _ := gh.FindAssetWithFallback(rel, archInfo)
					hasDeb[t.owner+"/"+t.repo] = archInfo != nil && result != nil && result.Asset != nil
					mu.Unlock()
					note = fmt.Sprintf("version=%s hasDeb=%v", ver, hasDeb[t.owner+"/"+t.repo])
				} else if errors.Is(relErr, gh.ErrNoStableRelease) {
					// 无稳定 release（全为 draft/prerelease）→ 视为无匹配 .deb
					mu.Lock()
					hasDeb[t.owner+"/"+t.repo] = false
					mu.Unlock()
					note = "no-stable-release → 无 .deb"
				} else {
					note = "network/api error（保留条目）"
				}
			} else {
				// 缓存命中：无 release 对象，假定有效（避免额外网络往返）
				mu.Lock()
				hasDeb[t.owner+"/"+t.repo] = true
				mu.Unlock()
				note = fmt.Sprintf("cached version=%s", ver)
			}
			if ver != "" {
				mu.Lock()
				latestVer[t.owner+"/"+t.repo] = ver
				mu.Unlock()
			}
			printProgress(t.name, t.owner, t.repo, note)
		}(t)
	}
	wg.Wait()
	if !verbose {
		fmt.Fprintln(os.Stderr)
	}
	vlog("并发检查完成：%d 个仓库", total)

	// 3) 校验并移除无当前架构 .deb 的条目（写入系统目录）。
	//    直接复用并发阶段拿到的 hasDeb 结果，不再二次访问网络。
	path := catalog.SystemCatalogPath()
	curData, rErr := os.ReadFile(path)
	if rErr != nil {
		return fmt.Errorf("读取目录失败: %w", rErr)
	}
	current := make(map[string]catalog.CatalogEntry)
	toml.Decode(string(curData), &current)

	removed := 0
	removedNames := make([]string, 0)
	for _, name := range names {
		e := entries[name]
		// ghdeb 自身与直接 URL 条目保留
		if name == "ghdeb" || e.IsDirectURL() || e.Repo == "" {
			continue
		}
		owner, repo, perr := gh.ParseRepo(e.Repo)
		if perr != nil {
			continue
		}
		valid, known := hasDeb[owner+"/"+repo]
		if !known {
			// 无版本信息或检查失败：保留，避免误删
			continue
		}
		if !valid {
			vlog("移除 %s（当前架构无 .deb 资产）", name)
			removed++
			removedNames = append(removedNames, name)
			delete(current, name)
		}
	}
	if removed > 0 {
		vlog("写入系统目录 %s（移除 %d 条）", path, removed)
		if err := writeSystemCatalog(path, current, "ghdeb"); err != nil {
			return fmt.Errorf("写入目录失败: %w", err)
		}
	}

	// 4) 扫描系统已装包（一次读 /var/lib/dpkg/status），写入扁平化快照。
	//    已装判定与版本号完全取自 dpkg 系统状态，不再依赖 history.json。
	vlog("扫描系统已装包（/var/lib/dpkg/status）...")
	installed := deb.ScanInstalledDpkg()
	if installed == nil {
		vlog("（读取 dpkg 状态库失败，视为无已装包）")
		installed = make(map[string]*deb.DpkgPkg)
	}
	vlog("系统已装 dpkg 包 %d 个", len(installed))

	vlog("组装并写回扁平化快照 %s ...", state.CachePath())
	snap := state.LoadCache()
	snap.UpdatedAt = time.Now().Format(time.RFC3339)

	for _, name := range names {
		// 已因无 .deb 被移除的条目不进快照
		if containsStr(removedNames, name) {
			snap.Remove(name)
			continue
		}
		entry := entries[name]

		sp := &state.PkgState{Name: name, Repo: entry.Repo, Arch: sysArch}

		// 已装信息：从 dpkg 系统状态反查目录条目（名字优先 + repo 尾段兜底）
		if entry.Repo != "" {
			for _, cand := range deb.CandidatePkgNames(name, entry.Repo) {
				if dp := installed[cand]; dp != nil {
					sp.Installed = true
					sp.InstalledVersion = dp.Version
					sp.InstallTime = deb.InstallTimeOf(cand)
					vlog("已装 %s (%s): dpkg 版本 %q，安装于 %s", name, cand, dp.Version, sp.InstallTime)
					break
				}
			}
		}

		// 最新版本：本次并发查询结果（GitHub 最新 tag）
		if entry.Repo != "" {
			if owner, repo, perr := gh.ParseRepo(entry.Repo); perr == nil {
				sp.GitHubVersion = latestVer[owner+"/"+repo]
			}
		}

		// 可升级性
		if sp.Installed && sp.InstalledVersion != "" && sp.GitHubVersion != "" {
			sp.Upgradable = compareVersion(sp.InstalledVersion, sp.GitHubVersion) < 0
		}

		snap.Set(name, sp)
	}

	if err := state.SaveCache(snap); err != nil {
		return fmt.Errorf("保存 list 快照失败: %w", err)
	}
	vlog("快照已保存到 %s", state.CachePath())

	fmt.Printf(T("更新 %d 个软件包信息，移除 %d 条不满足要求（无 .deb 包）的记录\n",
		"Updated info for %d packages, removed %d records without a .deb package\n"),
		len(snap.Packages), removed)
	return nil
}
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// estimateMinutes 估算并发检查 n 个仓库需要的分钟数：
// 平均每个请求约 1.5s，8 路并发，结果向上取整，至少 1 分钟。
func estimateMinutes(n int) int {
	if n <= 0 {
		return 0
	}
	sec := float64(n) * 1.5 / 8.0
	mm := int(sec / 60.0)
	if sec/60.0-float64(mm) > 0 {
		mm++
	}
	if mm < 1 {
		mm = 1
	}
	return mm
}

// --- history ---

type flatHistory struct {
	Repo        string       `json:"repo"`
	PkgName     string       `json:"pkg_name,omitempty"`
	Action      state.Action `json:"action"`
	Version     string       `json:"version"`
	FromVersion string       `json:"from_version,omitempty"`
	DebFile     string       `json:"deb_file,omitempty"`
	DebPath     string       `json:"deb_path,omitempty"`
	ReleaseURL  string       `json:"release_url,omitempty"`
	Reinstall   bool         `json:"reinstall,omitempty"`
	Timestamp   string       `json:"timestamp"`
}

// historySortKey 解析 RFC3339 时间用于排序；解析失败按零值（最早）处理
func historySortKey(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

// collectFlatHistory 汇总所有包的历史记录为扁平列表
func collectFlatHistory(st *state.State) []flatHistory {
	var out []flatHistory
	for repoKey, rec := range st.Packages {
		for _, e := range rec.History {
			out = append(out, flatHistory{
				Repo:        repoKey,
				PkgName:     rec.PkgName,
				Action:      e.Action,
				Version:     e.Version,
				FromVersion: e.FromVersion,
				DebFile:     e.DebFile,
				DebPath:     e.DebPath,
				ReleaseURL:  e.ReleaseURL,
				Reinstall:   e.Reinstall,
				Timestamp:   e.Timestamp,
			})
		}
	}
	// 按时间由旧到新
	sort.SliceStable(out, func(i, j int) bool {
		return historySortKey(out[i].Timestamp).Before(historySortKey(out[j].Timestamp))
	})
	return out
}

// formatHistoryLine 单条历史的一行文本
func formatHistoryLine(e flatHistory) string {
	ts := e.Timestamp
	if ts != "auto-discovered" {
		ts = formatTime(ts)
	}
	verb := strings.ToUpper(string(e.Action))
	if e.Reinstall {
		verb = "REINSTALL"
	}
	line := fmt.Sprintf("%s  %s  %s", ts, e.Repo, verb)
	if e.Action == state.ActionUpgrade && e.FromVersion != "" {
		line += fmt.Sprintf("  %s → %s", e.FromVersion, e.Version)
	} else {
		line += fmt.Sprintf("  %s", e.Version)
	}
	return line
}

func cmdHistory(args []string) error {
	// 解析 --json
	jsonOutput := false
	var rest []string
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		} else {
			rest = append(rest, a)
		}
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	// 无参数：列出全部历史，按时间由旧到新，每行一条（--json 输出数组）
	if len(rest) == 0 {
		entries := collectFlatHistory(st)
		if jsonOutput {
			jsonData, _ := json.MarshalIndent(entries, "", "  ")
			fmt.Println(string(jsonData))
			return nil
		}
		for _, e := range entries {
			fmt.Println(formatHistoryLine(e))
		}
		return nil
	}

	// 带参数：某包的详细历史
	rec := st.Get(rest[0])
	if rec == nil {
		// 尝试短名称
		owner, repo, resolveErr := resolvePkgArg(rest[0])
		if resolveErr != nil {
			return fmt.Errorf("未找到 %s 的记录", rest[0])
		}
		rec = st.Get(owner + "/" + repo)
		if rec == nil {
			return fmt.Errorf("未找到 %s 的记录", rest[0])
		}
	}
	repoKey := rec.Owner + "/" + rec.Repo

	// --json：输出该包历史数组
	if jsonOutput {
		var entries []flatHistory
		for _, e := range rec.History {
			entries = append(entries, flatHistory{
				Repo:        repoKey,
				PkgName:     rec.PkgName,
				Action:      e.Action,
				Version:     e.Version,
				FromVersion: e.FromVersion,
				DebFile:     e.DebFile,
				DebPath:     e.DebPath,
				ReleaseURL:  e.ReleaseURL,
				Reinstall:   e.Reinstall,
				Timestamp:   e.Timestamp,
			})
		}
		sort.SliceStable(entries, func(i, j int) bool {
			return historySortKey(entries[i].Timestamp).Before(historySortKey(entries[j].Timestamp))
		})
		jsonData, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(jsonData))
		return nil
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

// cmdCatalogModify 修改目录条目：ghdeb catalog modify <pkgname> --repo <owner/repo>
func cmdCatalogModify(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: ghdeb catalog modify <pkgname> --repo <owner/repo>")
	}
	name := args[0]
	var repo string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			if i+1 >= len(args) {
				return fmt.Errorf("--repo 需要参数")
			}
			i++
			repo = args[i]
		default:
			return fmt.Errorf("未知选项: %s", args[i])
		}
	}
	if repo == "" {
		return fmt.Errorf("请指定 --repo <owner/repo>")
	}
	if _, _, err := gh.ParseRepo(repo); err != nil {
		return err
	}
	if err := modifySystemCatalogRepo(name, repo); err != nil {
		return err
	}
	fmt.Printf(T("✅ 已更新目录条目 %s 的仓库为 %s\n", "✅ Updated catalog entry %s repo to %s\n"), name, repo)
	return nil
}

// modifySystemCatalogRepo 修改系统目录中某条目的 repo 字段
func modifySystemCatalogRepo(name, repo string) error {
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

	// 支持短名称和 owner/repo 两种写法
	key := name
	entry, ok := entries[key]
	if !ok {
		found := ""
		for k, e := range entries {
			if e.Repo == name {
				found = k
				break
			}
		}
		if found == "" {
			return fmt.Errorf("目录中未找到 %s", name)
		}
		key = found
		entry = entries[key]
	}
	entry.Repo = repo
	entries[key] = entry
	return writeSystemCatalog(path, entries)
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
	// 先下载到用户可写的临时文件，再 sudo 移动到系统缓存目录
	tmp, err := os.CreateTemp("", "ghdeb-*.deb")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	fmt.Printf(T("⬇️  多线程下载中 (%d 线程): %s\n", "⬇️  Parallel downloading (%d threads): %s\n"), 4, asset.BrowserDownloadURL)
	err = client.DownloadAssetWithFallback(asset, tmpPath, func(downloaded, total int64) {
		printProgress(downloaded, total)
	})
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}

	cacheDir, err := getCacheDir()
	if err != nil {
		return "", err
	}
	destPath := filepath.Join(cacheDir, asset.Name)
	if err := state.SudoMove(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("移动到缓存目录失败: %w", err)
	}
	fmt.Printf(T("✅ 下载完成: %s\n", "✅ Download complete: %s\n"), destPath)
	return destPath, nil
}

func installDeb(path string) error {
	fmt.Printf(T("📦 安装中 (dpkg -i)...\n", "📦 Installing (dpkg -i)...\n"))
	cmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "dpkg", "--force-confold", "-i", path)
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
		fixCmd := exec.Command("sudo", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-o", "Dpkg::Options::=--force-confold", "install", "-f", "-y")
		fixCmd.Stdout = os.Stdout
		fixCmd.Stderr = os.Stderr
		if fixErr := fixCmd.Run(); fixErr != nil {
			return fmt.Errorf(T("安装失败且依赖修复失败: %w", "install failed and dependency fix failed: %w"), fixErr)
		}
	}
	return nil
}

func getCacheDir() (string, error) {
	// 系统级缓存目录：root 写，其他只读，便于所有用户共享监测
	cacheDir := state.CacheDir()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		if err := state.SudoMkdirAll(cacheDir); err != nil {
			return "", err
		}
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

// normalizeVersion 规范化版本号用于比较：
//  1. 去掉 v/V 前缀
//  2. 剥离 Debian 数字修订号后缀（如 1.2.4-1 → 1.2.4），
//     因为 GitHub release tag 通常不带修订号，且修订号只会让 dpkg 版本更高
func normalizeVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.LastIndex(v, "-"); i >= 0 {
		allDigits := true
		for _, c := range v[i+1:] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && v[i+1:] != "" {
			v = v[:i]
		}
	}
	return v
}

// versionEqual 比较两个版本是否相等（忽略 v 前缀）
func versionEqual(a, b string) bool {
	return normalizeVersion(a) == normalizeVersion(b)
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
