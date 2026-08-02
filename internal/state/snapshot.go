// Package state - ghdeb list 使用的快照缓冲文件
// ghdeb update 会查询 GitHub 最新版本、本地已装版本并判定可升级性，
// 将结果写入此快照；ghdeb list 只读此快照，不再实时查询网络。
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// SnapshotPkg 单个包的 list 展示信息
type SnapshotPkg struct {
	Repo             string `json:"repo,omitempty"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	Upgradeable      bool   `json:"upgradeable"`
}

// Snapshot 快照缓冲文件结构
type Snapshot struct {
	UpdatedAt string                  `json:"updated_at"` // RFC3339
	Packages  map[string]*SnapshotPkg `json:"packages"`   // key = catalog 短名称
}

// snapshotPath 返回 list 快照文件路径 ~/.cache/ghdeb/snapshot.json
func snapshotPath() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "ghdeb", "snapshot.json")
}

// LoadSnapshot 从磁盘加载快照
func LoadSnapshot() *Snapshot {
	s := &Snapshot{Packages: make(map[string]*SnapshotPkg)}
	data, err := os.ReadFile(snapshotPath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, s)
	if s.Packages == nil {
		s.Packages = make(map[string]*SnapshotPkg)
	}
	return s
}

// SaveSnapshot 将快照写入磁盘
func SaveSnapshot(s *Snapshot) error {
	path := snapshotPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SnapshotNames 返回快照中的排序名称列表
func (s *Snapshot) SortedNames() []string {
	names := make([]string, 0, len(s.Packages))
	for n := range s.Packages {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Get 获取某个包的快照信息
func (s *Snapshot) Get(name string) *SnapshotPkg {
	return s.Packages[name]
}

// Set 写入某个包的快照信息
func (s *Snapshot) Set(name string, pkg *SnapshotPkg) {
	s.Packages[name] = pkg
}

// Remove 从快照删除某个包
func (s *Snapshot) Remove(name string) {
	delete(s.Packages, name)
}
