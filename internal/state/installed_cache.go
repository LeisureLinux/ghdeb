// Package state 已装版本缓存：按 deb 包名缓存 dpkg 实查结果，24h TTL
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// 已装版本缓存默认有效期
const installedCacheTTL = 24 * time.Hour

// installedCacheEntry 单条已装版本缓存记录
type installedCacheEntry struct {
	Version   string `json:"version"`
	FetchedAt string `json:"fetched_at"` // RFC3339
}

// installedCacheData 缓存文件结构
type installedCacheData struct {
	Versions map[string]*installedCacheEntry `json:"versions"` // key = deb 包名
}

// installedCachePath 返回已装版本缓存文件路径 ~/.cache/ghdeb/installed_versions.json
func installedCachePath() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "ghdeb", "installed_versions.json")
}

func loadInstalledCache() *installedCacheData {
	cd := &installedCacheData{Versions: make(map[string]*installedCacheEntry)}
	data, err := os.ReadFile(installedCachePath())
	if err != nil {
		return cd
	}
	_ = json.Unmarshal(data, cd)
	if cd.Versions == nil {
		cd.Versions = make(map[string]*installedCacheEntry)
	}
	return cd
}

func saveInstalledCache(cd *installedCacheData) error {
	path := installedCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetCachedInstalled 从缓存获取已装版本号，未命中或过期返回空
func GetCachedInstalled(pkgName string) string {
	if pkgName == "" {
		return ""
	}
	cd := loadInstalledCache()
	entry, ok := cd.Versions[pkgName]
	if !ok {
		return ""
	}
	fetchedAt, err := time.Parse(time.RFC3339, entry.FetchedAt)
	if err != nil || time.Since(fetchedAt) > installedCacheTTL {
		return ""
	}
	return entry.Version
}

// SetCachedInstalled 将已装版本号写入缓存
func SetCachedInstalled(pkgName, version string) {
	if pkgName == "" {
		return
	}
	cd := loadInstalledCache()
	cd.Versions[pkgName] = &installedCacheEntry{
		Version:   version,
		FetchedAt: time.Now().Format(time.RFC3339),
	}
	_ = saveInstalledCache(cd)
}
