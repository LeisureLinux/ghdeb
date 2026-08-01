// Package deb 提供系统架构检测和 .deb 相关工具函数
package deb

import (
	"fmt"
	"os/exec"
	"strings"
)

// ArchInfo 包含 dpkg 架构名和 GitHub release 中常见的架构别名
type ArchInfo struct {
	DpkgArch string   // dpkg --print-architecture 输出, 如 amd64, arm64
	Aliases  []string // GitHub release 文件名中常见的架构名
}

// DetectArch 检测当前系统架构
func DetectArch() (*ArchInfo, error) {
	out, err := exec.Command("dpkg", "--print-architecture").Output()
	if err != nil {
		return nil, fmt.Errorf("无法运行 dpkg --print-architecture: %w", err)
	}
	arch := strings.TrimSpace(string(out))
	return archInfoFor(arch), nil
}

func archInfoFor(dpkgArch string) *ArchInfo {
	switch dpkgArch {
	case "amd64":
		return &ArchInfo{
			DpkgArch: "amd64",
			Aliases:  []string{"amd64", "x86_64", "x86-64", "x64"},
		}
	case "arm64":
		return &ArchInfo{
			DpkgArch: "arm64",
			Aliases:  []string{"arm64", "aarch64"},
		}
	case "armhf":
		return &ArchInfo{
			DpkgArch: "armhf",
			Aliases:  []string{"armhf", "armv7l", "armv7", "arm"},
		}
	case "i386":
		return &ArchInfo{
			DpkgArch: "i386",
			Aliases:  []string{"i386", "x86", "i686", "386"},
		}
	default:
		return &ArchInfo{
			DpkgArch: dpkgArch,
			Aliases:  []string{dpkgArch},
		}
	}
}

// MatchAsset 判断一个 asset 文件名是否匹配当前架构
func (a *ArchInfo) MatchAsset(filename string) bool {
	lower := strings.ToLower(filename)
	for _, alias := range a.Aliases {
		// 检查架构名是否出现在文件名中（前后用 - 或 _ 或 . 分隔）
		for _, sep := range []string{"_", "-", "."} {
			if strings.Contains(lower, sep+alias) || strings.Contains(lower, alias+sep) {
				return true
			}
		}
		// 也匹配文件名直接以架构名结尾的情况 (如 foo_amd64.deb)
		if strings.Contains(lower, alias) {
			return true
		}
	}
	return false
}
