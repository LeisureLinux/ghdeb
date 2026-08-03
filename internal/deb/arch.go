// Package deb 提供系统架构检测和 .deb 相关工具函数
package deb

import (
	"fmt"
	"os"
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
	case "loong64":
		return &ArchInfo{
			DpkgArch: "loong64",
			Aliases:  []string{"loong64", "loongarch64", "loongarch"},
		}
	case "riscv64":
		return &ArchInfo{
			DpkgArch: "riscv64",
			Aliases:  []string{"riscv64", "riscv"},
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

// ExtractPkgName 从 deb 文件提取包名
func ExtractPkgName(debPath string) string {
	cmd := exec.Command("dpkg-deb", "-f", debPath, "Package")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// IsPackageInstalled 检查 deb 包是否已安装
func IsPackageInstalled(pkgName string) bool {
	cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", pkgName)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// dpkg-query 输出 "install ok installed" 表示已安装
	return strings.Contains(string(output), "install ok installed")
}

// --- x86_64 微架构（x86-64 v1/v2/v3/v4）检测 ---

// DetectX86MicroArch 检测当前 x86_64 CPU 支持的微架构级别。
// 返回 1（v1 基线）~ 4（v4），非 x86_64 返回 0。
// 依据 x86-psABI microarchitecture levels 判定。
func DetectX86MicroArch() int {
	ai, err := DetectArch()
	if err != nil || ai.DpkgArch != "amd64" {
		return 0
	}
	flags := cpuinfoFlags()
	// v2：在 v1 基础上要求 SSE4.2/POPCNT/CX16/LAHF-LM 等
	if !hasAllFlags(flags, []string{"sse4_2", "popcnt", "cx16", "lahf_lm"}) {
		return 1
	}
	// v3：在 v2 基础上要求 AVX2/BMI1/BMI2/FMA/F16C/MOVBE 等核心指令。
	// 不要求 lzcnt/osxsave 等，因部分虚拟机 /proc/cpuinfo 不会透传这些位，
	// 而 AVX2 是 v3 最具标志性的指令，可据此可靠判定。
	if hasAllFlags(flags, []string{"avx2", "bmi1", "bmi2", "f16c", "fma", "movbe"}) {
		// v4：在 v3 基础上要求 AVX-512 扩展
		if hasAllFlags(flags, []string{"avx512f", "avx512bw", "avx512cd", "avx512dq", "avx512vl"}) {
			return 4
		}
		return 3
	}
	return 2
}

// AssetMicroArch 从资产文件名解析其所携带的 x86 微架构级别。
// 识别形如 x86_64_v2_sse、x86_64_v3_avx2、_v4_ 等命名，返回 2/3/4；未标注返回 0。
func AssetMicroArch(name string) int {
	lower := strings.ToLower(name)
	for lvl := 4; lvl >= 2; lvl-- {
		// 匹配 _v2_、_v3_、_v4_、-v2-、.v2 等分隔形式
		if strings.Contains(lower, fmt.Sprintf("_v%d_", lvl)) ||
			strings.Contains(lower, fmt.Sprintf("-v%d_", lvl)) ||
			strings.Contains(lower, fmt.Sprintf("v%d_", lvl)) ||
			strings.Contains(lower, fmt.Sprintf("x86_64v%d", lvl)) {
			return lvl
		}
	}
	return 0
}

// cpuinfoFlags 读取 /proc/cpuinfo 的 flags 字段
func cpuinfoFlags() map[string]bool {
	flags := map[string]bool{}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return flags
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "flags") && !strings.HasPrefix(line, "Features") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if f == ":" || f == "flags" {
				continue
			}
			flags[f] = true
		}
	}
	return flags
}

// hasAllFlags 检查 flags 中是否包含全部所需特征位
func hasAllFlags(flags map[string]bool, need []string) bool {
	for _, f := range need {
		if !flags[f] {
			return false
		}
	}
	return true
}
