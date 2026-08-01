// Package state 追踪已安装的包版本信息
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Record 记录一个已安装的包
type Record struct {
	Repo       string `json:"repo"`        // owner/repo
	Version    string `json:"version"`     // release tag
	DebFile    string `json:"deb_file"`    // 下载的 .deb 文件名
	InstalledAt string `json:"installed_at"` // 安装时间
}

// State 管理已安装包的状态
type State struct {
	path    string
	Records map[string]*Record `json:"records"` // key = "owner/repo"
}

// Load 从磁盘加载状态
func Load() (*State, error) {
	path := statePath()
	s := &State{path: path, Records: make(map[string]*Record)}

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
	if s.Records == nil {
		s.Records = make(map[string]*Record)
	}
	return s, nil
}

// Save 保存状态到磁盘
func (s *State) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Get 获取某个仓库的安装记录
func (s *State) Get(repo string) *Record {
	return s.Records[repo]
}

// Set 更新安装记录
func (s *State) Set(repo, version, debFile string) {
	s.Records[repo] = &Record{
		Repo:        repo,
		Version:     version,
		DebFile:     debFile,
		InstalledAt: time.Now().Format(time.RFC3339),
	}
}

// Remove 删除安装记录
func (s *State) Remove(repo string) {
	delete(s.Records, repo)
}

// List 返回所有已安装记录
func (s *State) List() []*Record {
	var records []*Record
	for _, r := range s.Records {
		records = append(records, r)
	}
	return records
}

func statePath() string {
	// 优先使用 XDG_STATE_HOME
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "ghdeb", "installed.json")
}
