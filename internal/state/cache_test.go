package state

import (
	"os"
	"testing"
	"time"
)

// TestFlatCache 验证扁平化统一缓存的读写与分段清理行为
func TestFlatCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GHDEB_CACHE_DIR", dir)
	if got := CacheDir(); got != dir {
		t.Fatalf("CacheDir=%q want %q", got, dir)
	}

	// 写入一个扁平 package 记录
	c := LoadCache()
	c.UpdatedAt = time.Now().Format(time.RFC3339)
	c.Set("bat", &PkgState{
		Name: "bat", Repo: "sharkdp/bat", Installed: true,
		InstallTime: time.Now().Format(time.RFC3339),
		InstalledVersion: "0.24.0", GitHubVersion: "0.24.0",
		Upgradable: false, Arch: "amd64", PkgFile: "bat_0.24.0_amd64.deb",
	})
	if err := SaveCache(c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	// 验证单文件确实只落一个 json
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 json file, got %d: %v", len(files), files)
	}

	// 重新加载校验字段都在同一文件
	c2 := LoadCache()
	if v := c2.Get("bat"); v == nil || !v.Installed || v.GitHubVersion != "0.24.0" || v.Arch != "amd64" {
		t.Fatalf("package mismatch: %+v", v)
	}
	// 按 owner/repo 反查 github 版本
	if v := GetCachedRelease("sharkdp", "bat"); v != "0.24.0" {
		t.Fatalf("release mismatch: %q", v)
	}

	// 更新 github 版本并验证
	SetCachedRelease("sharkdp", "bat", "0.25.0")
	if v := GetCachedRelease("sharkdp", "bat"); v != "0.25.0" {
		t.Fatalf("release after set mismatch: %q", v)
	}

	// 清空已装状态：只清 installed/install_time/installed_version/upgradable
	ClearInstalled()
	if v := c2.Get("bat"); v == nil {
		t.Fatalf("package should survive")
	}
	c3 := LoadCache()
	v := c3.Get("bat")
	if v.Installed || v.InstalledVersion != "" || v.Upgradable {
		t.Fatalf("installed fields should be cleared: %+v", v)
	}
	// repo/github_version/arch/pkg_file 保留
	if v.Repo != "sharkdp/bat" || v.GitHubVersion != "0.25.0" || v.Arch != "amd64" || v.PkgFile == "" {
		t.Fatalf("non-installed fields should survive: %+v", v)
	}

	// 清除指定仓库的 github 版本
	InvalidateReleaseCache("sharkdp", "bat")
	if v := GetCachedRelease("sharkdp", "bat"); v != "" {
		t.Fatalf("github version should be cleared, got %q", v)
	}
	_ = os.Remove
}
