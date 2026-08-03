// Package state 追踪已安装包的状态与完整操作历史
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Action 表示一次操作的类型
type Action string

const (
	ActionInstall Action = "install"
	ActionUpgrade Action = "upgrade"
	ActionRemove  Action = "remove"
)

// HistoryEntry 记录一次操作
type HistoryEntry struct {
	Action      Action `json:"action"`                 // install / upgrade / remove
	Version     string `json:"version"`                // 操作涉及的版本
	FromVersion string `json:"from_version,omitempty"` // upgrade 时的旧版本
	DebFile     string `json:"deb_file,omitempty"`     // .deb 文件名
	DebPath     string `json:"deb_path,omitempty"`     // .deb 完整路径
	ReleaseURL  string `json:"release_url,omitempty"`  // GitHub release URL
	Reinstall   bool   `json:"reinstall,omitempty"`    // 是否为重装操作
	Timestamp   string `json:"timestamp"`              // 操作时间 RFC3339
}

// PackageRecord 一个仓库的完整状态
type PackageRecord struct {
	Owner          string         `json:"owner"`
	PkgName        string         `json:"pkg_name,omitempty"` // deb 包名（如 "bat"）
	Repo           string         `json:"repo"`
	CurrentVersion string         `json:"current_version"`          // 最后一次 install/upgrade 的版本
	SystemVersion  string         `json:"system_version,omitempty"` // dpkg 实际版本（运行时查询）
	InstalledPath  string         `json:"installed_path,omitempty"` // dpkg 安装的关键路径
	Arch           string         `json:"arch,omitempty"`
	Removed        bool           `json:"removed"` // 是否已标记移除
	History        []HistoryEntry `json:"history"`
	UpdatedAt      string         `json:"updated_at"` // 最后更新时间
}

// State 管理所有包的状态
type State struct {
	path     string
	Packages map[string]*PackageRecord `json:"packages"` // key = "owner/repo"
}

// defaultStatePath 默认状态文件路径
func defaultStatePath() string {
	return "/var/cache/ghdeb/history.json"
}

// Load 从磁盘加载状态
func Load() (*State, error) {
	path := defaultStatePath()
	s := &State{path: path, Packages: make(map[string]*PackageRecord)}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("状态文件损坏: %w", err)
	}
	if s.Packages == nil {
		s.Packages = make(map[string]*PackageRecord)
	}
	return s, nil
}

// Save 保存状态到磁盘（权限不足时自动 sudo 写入）
func (s *State) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// MkdirAll 失败，尝试 sudo
		if err := sudoMkdirAll(dir); err != nil {
			return fmt.Errorf("创建目录失败（可能需要 sudo ghdeb）: %w", err)
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		if errors.Is(err, os.ErrPermission) {
			// 权限不足，使用 sudo tee 写入
			return sudoWriteFile(s.path, data)
		}
		return err
	}
	return nil
}

// sudoWriteFile 用 sudo tee 写入文件（需要用户输入密码）
func sudoWriteFile(path string, data []byte) error {
	cmd := exec.Command("sudo", "tee", path)
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stdout = nil // 抑制输出
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("写入失败（sudo 被拒绝或失败）: %w", err)
	}
	return nil
}

// sudoMkdirAll 用 sudo 创建目录
func sudoMkdirAll(path string) error {
	cmd := exec.Command("sudo", "mkdir", "-p", path)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("sudo", "chmod", "755", path)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Get 获取某个仓库的记录
func (s *State) Get(repo string) *PackageRecord {
	return s.Packages[repo]
}

// SetInstall 记录一次安装操作
func (s *State) SetInstall(repo, owner, repoName, version, debFile, debPath, releaseURL, arch, pkgName string) {
	now := time.Now().Format(time.RFC3339)
	rec := s.Packages[repo]
	if rec == nil {
		rec = &PackageRecord{Owner: owner, Repo: repoName, Arch: arch, PkgName: pkgName}
		s.Packages[repo] = rec
	}
	rec.CurrentVersion = version
	rec.Arch = arch
	rec.Removed = false
	rec.UpdatedAt = now
	rec.History = append(rec.History, HistoryEntry{
		Action:     ActionInstall,
		Version:    version,
		DebFile:    debFile,
		DebPath:    debPath,
		ReleaseURL: releaseURL,
		Timestamp:  now,
	})
}

// SetUpgrade 记录一次升级操作
func (s *State) SetUpgrade(repo, version, debFile, debPath, releaseURL string) {
	now := time.Now().Format(time.RFC3339)
	rec := s.Packages[repo]
	if rec == nil {
		return
	}
	fromVersion := rec.CurrentVersion
	rec.CurrentVersion = version
	rec.Removed = false
	rec.UpdatedAt = now
	rec.History = append(rec.History, HistoryEntry{
		Action:      ActionUpgrade,
		Version:     version,
		FromVersion: fromVersion,
		DebFile:     debFile,
		DebPath:     debPath,
		ReleaseURL:  releaseURL,
		Timestamp:   now,
	})
}

// MarkRemoved 标记为已移除（保留历史记录）
func (s *State) MarkRemoved(repo string) {
	now := time.Now().Format(time.RFC3339)
	rec := s.Packages[repo]
	if rec == nil {
		return
	}
	rec.Removed = true
	rec.UpdatedAt = now
	rec.History = append(rec.History, HistoryEntry{
		Action:    ActionRemove,
		Version:   rec.CurrentVersion,
		Timestamp: now,
	})
}

// RecordPurge 当包从未被 ghdeb 管理（如 apt 直接安装）但被 ghdeb purge 时，
// 创建一条仅含 remove 历史的管理记录，便于 ghdeb history 查询到「已删除」。
func (s *State) RecordPurge(repo, owner, repoName, pkgName, version string) {
	now := time.Now().Format(time.RFC3339)
	rec := s.Packages[repo]
	if rec == nil {
		rec = &PackageRecord{Owner: owner, Repo: repoName, PkgName: pkgName}
		s.Packages[repo] = rec
	}
	rec.CurrentVersion = version
	rec.Removed = true
	rec.UpdatedAt = now
	rec.History = append(rec.History, HistoryEntry{
		Action:    ActionRemove,
		Version:   version,
		Timestamp: now,
	})
}

// List 返回所有记录（含已移除），按更新时间倒序
func (s *State) List() []*PackageRecord {
	var records []*PackageRecord
	for _, r := range s.Packages {
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].UpdatedAt > records[j].UpdatedAt
	})
	return records
}

// ListActive 返回未移除的记录
func (s *State) ListActive() []*PackageRecord {
	var records []*PackageRecord
	for _, r := range s.Packages {
		if !r.Removed {
			records = append(records, r)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].UpdatedAt > records[j].UpdatedAt
	})
	return records
}

// LatestEntry 返回最近一条历史记录
func (r *PackageRecord) LatestEntry() *HistoryEntry {
	if len(r.History) == 0 {
		return nil
	}
	return &r.History[len(r.History)-1]
}

// --- 系统版本查询 ---

// QuerySystemVersion 通过 dpkg-query 查询包在系统中的实际版本
func QuerySystemVersion(pkgName string) string {
	cmd := exec.Command("dpkg-query", "-W", "-f", "${Version}", pkgName)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// QueryInstalledPath 查询包安装的关键二进制路径
func QueryInstalledPath(pkgName string) string {
	cmd := exec.Command("dpkg-query", "-L", pkgName)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/usr/bin/") || strings.HasPrefix(line, "/usr/sbin/") {
			return line
		}
	}
	return ""
}

// RefreshSystemInfo 从 dpkg 刷新系统版本和安装路径
func (r *PackageRecord) RefreshSystemInfo(debPkgName string) {
	if debPkgName == "" {
		debPkgName = r.Repo
	}
	r.SystemVersion = QuerySystemVersion(debPkgName)
	if path := QueryInstalledPath(debPkgName); path != "" {
		r.InstalledPath = path
	}
}

// sudoRemove 用 sudo 删除文件
func sudoRemove(path string) error {
	cmd := exec.Command("sudo", "rm", "-f", path)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// sudoMove 用 sudo mv 移动文件到系统目录
func sudoMove(src, dst string) error {
	cmd := exec.Command("sudo", "mv", src, dst)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SudoMove 导出 sudo mv
func SudoMove(src, dst string) error {
	return sudoMove(src, dst)
}
