// ghdeb - 从 GitHub Releases 安装/升级 .deb 包
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/leisurelinux/ghdeb/internal/deb"
	gh "github.com/leisurelinux/ghdeb/internal/github"
	"github.com/leisurelinux/ghdeb/internal/state"
)

const version = "0.2.0"

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
	case "list", "ls":
		err = cmdList()
	case "history":
		err = cmdHistory(args)
	case "remove", "rm":
		err = cmdRemove(args)
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
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`ghdeb - 从 GitHub Releases 安装 .deb 包

用法:
  ghdeb install <owner/repo>[@tag]   安装（或升级到）指定版本
  ghdeb upgrade [owner/repo]         升级包（不指定则升级所有已安装包）
  ghdeb list                         列出所有包（含已移除）
  ghdeb history <owner/repo>         查看某包的完整操作历史
  ghdeb remove <owner/repo>          标记移除（保留历史记录）
  ghdeb info <owner/repo>            查看最新 release 信息
  ghdeb version                      显示版本

环境变量:
  GITHUB_TOKEN / GH_TOKEN            GitHub 个人访问令牌（提高 API 限额）

示例:
  ghdeb install sharkdp/bat          安装 bat 最新版
  ghdeb install casey/just@v1.25.0   安装 just 指定版本
  ghdeb upgrade                      升级所有已安装的包
  ghdeb upgrade sharkdp/bat          只升级 bat
  ghdeb history sharkdp/bat          查看 bat 的安装/升级/移除历史
`)
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
	fmt.Printf("🔍 系统架构: %s\n", arch.DpkgArch)

	client := gh.NewClient()
	release, err := fetchRelease(client, owner, repo, tag)
	if err != nil {
		return err
	}
	fmt.Printf("📌 版本: %s\n", release.TagName)

	// 检查是否已安装相同版本
	st, err := state.Load()
	if err != nil {
		return err
	}
	repoKey := owner + "/" + repo
	if existing := st.Get(repoKey); existing != nil && existing.CurrentVersion == release.TagName && !existing.Removed {
		fmt.Printf("✅ %s 已安装版本 %s，无需重复安装\n", repo, release.TagName)
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
	fmt.Printf("📥 匹配文件: %s (%s)\n", asset.Name, formatSize(asset.Size))

	// 下载
	destPath, err := downloadAsset(client, *asset)
	if err != nil {
		return err
	}

	// 安装
	if err := installDeb(destPath); err != nil {
		return err
	}

	// 记录状态
	releaseURL := release.HTMLURL
	if releaseURL == "" {
		releaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, release.TagName)
	}
	st.SetInstall(repoKey, owner, repo, release.TagName, asset.Name, destPath, releaseURL, arch.DpkgArch)
	if err := st.Save(); err != nil {
		return fmt.Errorf("保存状态失败: %w", err)
	}

	fmt.Printf("🎉 安装完成: %s %s\n", repoKey, release.TagName)
	return nil
}

// --- upgrade ---

func cmdUpgrade(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}

	var repos []string
	if len(args) > 0 {
		repos = args
	} else {
		for _, r := range st.ListActive() {
			repos = append(repos, r.Owner+"/"+r.Repo)
		}
		if len(repos) == 0 {
			fmt.Println("没有已安装的包需要升级")
			return nil
		}
	}

	client := gh.NewClient()
	arch, err := deb.DetectArch()
	if err != nil {
		return err
	}

	upgraded := 0
	for _, repoStr := range repos {
		owner, repo, parseErr := gh.ParseRepo(repoStr)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  跳过 %s: %v\n", repoStr, parseErr)
			continue
		}
		repoKey := owner + "/" + repo

		fmt.Printf("\n🔍 检查 %s...\n", repoKey)
		release, getErr := client.GetLatestRelease(owner, repo)
		if getErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  获取 release 失败: %v\n", getErr)
			continue
		}

		existing := st.Get(repoKey)
		if existing != nil && existing.CurrentVersion == release.TagName && !existing.Removed {
			fmt.Printf("✅ 已是最新版本 %s\n", release.TagName)
			continue
		}
		if existing != nil && !existing.Removed {
			fmt.Printf("📦 发现新版本: %s → %s\n", existing.CurrentVersion, release.TagName)
		} else {
			fmt.Printf("📦 新版本: %s\n", release.TagName)
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
			fmt.Fprintf(os.Stderr, "⚠️  下载失败: %v\n", dlErr)
			continue
		}

		if instErr := installDeb(destPath); instErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  安装失败: %v\n", instErr)
			continue
		}

		releaseURL := release.HTMLURL
		if releaseURL == "" {
			releaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, release.TagName)
		}

		// 根据是否已有记录，决定是 install 还是 upgrade
		if existing != nil && !existing.Removed && existing.CurrentVersion != "" {
			st.SetUpgrade(repoKey, release.TagName, asset.Name, destPath, releaseURL)
		} else {
			st.SetInstall(repoKey, owner, repo, release.TagName, asset.Name, destPath, releaseURL, arch.DpkgArch)
		}
		upgraded++
		fmt.Printf("✅ 升级完成: %s %s\n", repoKey, release.TagName)
	}

	if saveErr := st.Save(); saveErr != nil {
		return fmt.Errorf("保存状态失败: %w", saveErr)
	}

	if upgraded == 0 {
		fmt.Println("\n所有包已是最新")
	} else {
		fmt.Printf("\n🎉 共升级 %d 个包\n", upgraded)
	}
	return nil
}

// --- list ---

func cmdList() error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	records := st.List()
	if len(records) == 0 {
		fmt.Println("没有已安装的包")
		return nil
	}

	fmt.Printf("%-30s %-12s %-12s %-10s %-19s\n", "仓库", "安装版本", "系统版本", "状态", "最后操作")
	fmt.Println(strings.Repeat("-", 90))
	for _, r := range records {
		// 运行时查询系统版本
		r.RefreshSystemInfo(r.Repo)

		status := "✅ installed"
		if r.Removed {
			status = "❌ removed"
		}

		sysVer := r.SystemVersion
		if sysVer == "" {
			sysVer = "-"
		}

		updatedAt := formatTime(r.UpdatedAt)
		fmt.Printf("%-30s %-12s %-12s %-10s %-19s\n",
			r.Owner+"/"+r.Repo,
			r.CurrentVersion,
			sysVer,
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

	// 查询系统版本
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
		ts := formatTime(e.Timestamp)
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

// --- info ---

func cmdInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定仓库，如: ghdeb info sharkdp/bat")
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

	fmt.Printf("仓库: %s/%s\n", owner, repo)
	fmt.Printf("最新版本: %s\n", release.TagName)
	if release.Name != "" && release.Name != release.TagName {
		fmt.Printf("名称: %s\n", release.Name)
	}
	if release.HTMLURL != "" {
		fmt.Printf("Release: %s\n", release.HTMLURL)
	}

	if result != nil && result.Asset != nil {
		fmt.Printf("\n✅ 匹配当前架构 %s 的 .deb 文件:\n", arch.DpkgArch)
		fmt.Printf("  → %s (%s)\n", result.Asset.Name, formatSize(result.Asset.Size))
	} else {
		fmt.Printf("\n⚠️  没有匹配当前架构 %s 的 .deb 文件\n", arch.DpkgArch)
		if findErr != nil {
			fmt.Printf("   %v\n", findErr)
		}
	}

	if result != nil && len(result.Fallbacks) > 0 {
		fmt.Printf("\n📦 其他可用的安装包（需手动下载安装）:\n")
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
		fmt.Printf("📦 获取 release %s...\n", tag)
		return client.GetReleaseByTag(owner, repo, tag)
	}
	fmt.Printf("📦 获取最新 release...\n")
	return client.GetLatestRelease(owner, repo)
}

func downloadAsset(client *gh.Client, asset gh.Asset) (string, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", err
	}
	destPath := filepath.Join(cacheDir, asset.Name)
	fmt.Printf("⬇️  下载中...\n")
	err = client.DownloadAsset(asset, destPath, func(downloaded, total int64) {
		printProgress(downloaded, total)
	})
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	fmt.Printf("✅ 下载完成: %s\n", destPath)
	return destPath, nil
}

func installDeb(path string) error {
	fmt.Printf("📦 安装中 (dpkg -i)...\n")
	cmd := exec.Command("sudo", "dpkg", "-i", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("🔧 尝试修复依赖 (apt-get install -f)...\n")
		fixCmd := exec.Command("sudo", "apt-get", "install", "-f", "-y")
		fixCmd.Stdout = os.Stdout
		fixCmd.Stderr = os.Stderr
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

// formatTime 将 RFC3339 时间格式化为更易读的格式
func formatTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02 15:04:05")
}

// 确保 syscall 被使用
var _ = syscall.Geteuid
