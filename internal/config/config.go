// Package config 提供配置管理
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 配置结构
type Config struct {
	Proxy string `json:"proxy,omitempty"` // 代理地址，如 http://wpad.lan:8888
}

// configPath 返回配置文件路径
func configPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ghdeb", "config.json")
}

// Load 加载配置
func Load() *Config {
	cfg := &Config{}
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, cfg)
	return cfg
}

// GetProxy 获取代理地址（优先环境变量，其次配置文件）
func GetProxy() string {
	// 优先使用环境变量
	for _, env := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if proxy := os.Getenv(env); proxy != "" {
			return proxy
		}
	}
	// 其次使用配置文件
	cfg := Load()
	return cfg.Proxy
}
