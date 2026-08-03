package deb

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DpkgPkg 记录一个系统已安装的 dpkg 包的信息
type DpkgPkg struct {
	Version     string // dpkg 实际版本
	InstallTime string // 安装时间 RFC3339（来自 /var/lib/dpkg/info/<pkg>.list 的 mtime）
}

// defaultDpkgStatusFile 系统 dpkg 状态数据库
const defaultDpkgStatusFile = "/var/lib/dpkg/status"

// infoListDir dpkg 已装包信息目录（其 *.list 文件 mtime 近似安装时间）
const infoListDir = "/var/lib/dpkg/info"

// ScanInstalledDpkg 一次性扫描 /var/lib/dpkg/status，返回系统所有已安装包（name -> 信息）。
// 相比逐个 dpkg-query，只读一次状态库，更快速；status 必须为 "installed" 才计入
// （避免 fd 这类 "install ok not-installed" 的残留条目被误判为已装）。
func ScanInstalledDpkg() map[string]*DpkgPkg {
	path := os.Getenv("GHDEB_DPKG_STATUS")
	if path == "" {
		path = defaultDpkgStatusFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	installed := make(map[string]*DpkgPkg)
	var (
		curName  string
		curVer   string
		curState string
		have     bool
	)
	flush := func() {
		if have && strings.Contains(curState, " installed") {
			installed[curName] = &DpkgPkg{Version: curVer}
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			flush()
			curName, curVer, curState, have = "", "", "", false
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue // 多行字段（如长 Description）跳过
		}
		switch {
		case strings.HasPrefix(line, "Package: "):
			curName = strings.TrimSpace(line[len("Package: "):])
		case strings.HasPrefix(line, "Version: "):
			curVer = strings.TrimSpace(line[len("Version: "):])
		case strings.HasPrefix(line, "Status: "):
			curState = strings.TrimSpace(line[len("Status: "):])
			have = true
		}
	}
	flush()
	return installed
}

// CandidatePkgNames 返回一个目录条目可能的 dpkg 包名候选列表（去重）：
// 优先用 catalog 短名（如 du-dust），其次用仓库最后一段（如 dust），
// 逐个匹配系统已装包即可覆盖绝大多数 ghdeb 包。
func CandidatePkgNames(name, repo string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, 2)
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(name)
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		add(repo[i+1:])
	} else {
		add(repo)
	}
	return out
}

// InstallTimeOf 返回某 dpkg 包的大致安装时间（RFC3339），
// 取自 /var/lib/dpkg/info/<pkg>.list 文件的 mtime；文件缺失返回空串。
func InstallTimeOf(pkgName string) string {
	infoDir := os.Getenv("GHDEB_DPKG_INFO")
	if infoDir == "" {
		infoDir = infoListDir
	}
	fi, err := os.Stat(filepath.Join(infoDir, pkgName+".list"))
	if err != nil {
		return ""
	}
	return fi.ModTime().UTC().Format(time.RFC3339)
}
