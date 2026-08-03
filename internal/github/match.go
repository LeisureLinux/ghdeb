package github

import (
	"fmt"
	"strings"

	"github.com/leisurelinux/ghdeb/internal/i18n"

	"github.com/leisurelinux/ghdeb/internal/deb"
)

// 特殊变体关键词，优先级较低
var variantKeywords = []string{"musl", "static", "portable", "legacy"}

// 真正的架构标识
var archPatterns = []string{
	"amd64", "x86_64", "x86-64", "x64",
	"arm64", "aarch64",
	"armhf", "armv7l", "armv7", "armv6l", "armv6",
	"i386", "i686", "386",
	"ppc64le", "s390x", "riscv64", "loongarch64",
}

// MatchResult 匹配结果
type MatchResult struct {
	Asset     *Asset  // 精确匹配的 .deb
	Fallbacks []Asset // 备选方案（无架构 .deb、tar.gz、AppImage）
}

// FindDebAsset 在 release assets 中找到匹配当前架构的最佳 .deb 文件
func FindDebAsset(release *Release, arch *deb.ArchInfo) (*Asset, error) {
	result, err := FindAssetWithFallback(release, arch)
	if err != nil {
		return nil, err
	}
	if result.Asset != nil {
		return result.Asset, nil
	}
	// 没有精确匹配，返回错误但包含备选方案
	if len(result.Fallbacks) > 0 {
		return nil, &FallbackError{
			Release:   release.TagName,
			Arch:      arch.DpkgArch,
			Fallbacks: result.Fallbacks,
		}
	}
	return nil, fmt.Errorf(i18n.T("release %s 中没有可用的安装包", "release %s has no available packages"), release.TagName)
}

// FallbackError 包含备选下载方案的错误
type FallbackError struct {
	Release   string
	Arch      string
	Fallbacks []Asset
}

func (e *FallbackError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(i18n.T("release %s 中没有匹配架构 %s 的 .deb 文件\n", "release %s has no .deb file matching arch %s\n"), e.Release, e.Arch))
	sb.WriteString(i18n.T("\n其他可用的安装包:\n", "\nOther available packages:\n"))
	for _, a := range e.Fallbacks {
		sb.WriteString(fmt.Sprintf("  %s (%s)\n", a.Name, formatSize(a.Size)))
		sb.WriteString(fmt.Sprintf("    %s\n", a.BrowserDownloadURL))
	}
	return sb.String()
}

// FindAssetWithFallback 查找匹配资产，并收集备选方案
func FindAssetWithFallback(release *Release, arch *deb.ArchInfo) (*MatchResult, error) {
	result := &MatchResult{}

	var allAssets []Asset
	for i := range release.Assets {
		name := strings.ToLower(release.Assets[i].Name)
		if strings.HasSuffix(name, ".deb") ||
			strings.HasSuffix(name, ".tar.gz") ||
			strings.HasSuffix(name, ".tgz") ||
			strings.HasSuffix(name, ".appimage") {
			allAssets = append(allAssets, release.Assets[i])
		}
	}

	if len(allAssets) == 0 {
		return nil, fmt.Errorf(i18n.T("release %s 中没有可用的安装包（.deb/.tar.gz/.AppImage）", "release %s has no available packages (.deb/.tar.gz/.AppImage)"), release.TagName)
	}

	// 1. 优先找精确匹配的 .deb（排除变体）
	var matchedDebs []Asset
	for _, a := range allAssets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".deb") && arch.MatchAsset(a.Name) {
			matchedDebs = append(matchedDebs, a)
		}
	}

	if len(matchedDebs) > 0 {
		// 从匹配的 .deb 中选择最佳（优先标准包，排除变体）
		best := selectBestAsset(matchedDebs, arch)
		result.Asset = best
		return result, nil
	}

	// 2. 收集备选方案
	for _, a := range allAssets {
		lower := strings.ToLower(a.Name)

		// .deb 文件：只收集无架构标识的
		if strings.HasSuffix(lower, ".deb") {
			if !hasArchIdentifier(a.Name) {
				result.Fallbacks = append(result.Fallbacks, a)
			}
			continue
		}

		// .tar.gz / .tgz 文件：只收集 Linux 平台的
		if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
			if isLinuxPackage(a.Name) && arch.MatchAsset(a.Name) {
				result.Fallbacks = append(result.Fallbacks, a)
			}
			continue
		}

		// AppImage：只收集 Linux 平台的（AppImage 本身就是 Linux 格式）
		if strings.HasSuffix(lower, ".appimage") {
			if !hasArchIdentifier(a.Name) || arch.MatchAsset(a.Name) {
				result.Fallbacks = append(result.Fallbacks, a)
			}
			continue
		}
	}

	return result, nil
}

// selectBestAsset 从多个匹配的 assets 中选择最佳的一个
// 优先标准包；x86_64 时优先选择与当前 CPU 指令集匹配的微架构变体
func selectBestAsset(assets []Asset, arch *deb.ArchInfo) *Asset {
	// x86_64：优先选微架构变体（v2/v3/v4），取当前 CPU 支持的最高级别
	if arch.DpkgArch == "amd64" {
		if cpuLevel := deb.DetectX86MicroArch(); cpuLevel >= 2 {
			bestLevel := 0
			var best *Asset
			for i := range assets {
				lvl := deb.AssetMicroArch(assets[i].Name)
				if lvl > 0 && lvl <= cpuLevel && lvl > bestLevel {
					bestLevel = lvl
					best = &assets[i]
				}
			}
			if best != nil {
				return best
			}
		}
	}

	var standard, variant []Asset
	for _, a := range assets {
		if isVariant(a.Name) {
			variant = append(variant, a)
		} else {
			standard = append(standard, a)
		}
	}

	// 优先从标准包中选
	candidates := standard
	if len(candidates) == 0 {
		candidates = variant
	}

	// 在候选中，优先选 dpkg 架构名精确匹配的
	for _, a := range candidates {
		if strings.Contains(a.Name, arch.DpkgArch) {
			return &a
		}
	}
	return &candidates[0]
}

// hasArchIdentifier 检查文件名是否包含架构标识
func hasArchIdentifier(filename string) bool {
	lower := strings.ToLower(filename)
	for _, p := range archPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isLinuxPackage 检查文件名是否表示 Linux 平台的包
func isLinuxPackage(filename string) bool {
	lower := strings.ToLower(filename)
	// 检查是否包含 linux 标识
	if strings.Contains(lower, "linux") {
		return true
	}
	// 检查是否包含其他平台的标识
	otherPlatforms := []string{"darwin", "macos", "osx", "windows", "win"}
	for _, platform := range otherPlatforms {
		if strings.Contains(lower, platform) {
			return false
		}
	}
	// 如果既没有 linux 也没有其他平台标识，假设是通用的（可能是 Linux）
	return true
}

// isVariant 判断文件名是否为特殊变体
func isVariant(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range variantKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
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
