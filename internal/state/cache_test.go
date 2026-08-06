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

// TestSaveCacheSkipsUnchanged 验证内容未变时 SaveCache 跳过写入（不触发 sudo），
// 仅 updated_at 变化不应重写文件。
func TestSaveCacheSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GHDEB_CACHE_DIR", dir)

	c := LoadCache()
	c.UpdatedAt = time.Now().Format(time.RFC3339)
	c.Set("bat", &PkgState{Name: "bat", Repo: "sharkdp/bat", Installed: true, InstalledVersion: "0.24.0", GitHubVersion: "0.24.0"})
	if err := SaveCache(c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	fi1, _ := os.Stat(CachePath())

	// 重新构造内容完全相同（仅 updated_at 不同）的快照并保存
	c2 := LoadCache()
	c2.UpdatedAt = time.Now().Add(time.Hour).Format(time.RFC3339) // 仅时间戳变化
	if err := SaveCache(c2); err != nil {
		t.Fatalf("SaveCache unchanged: %v", err)
	}
	fi2, _ := os.Stat(CachePath())
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatalf("expected write to be skipped when packages unchanged (mtime changed: %v -> %v)", fi1.ModTime(), fi2.ModTime())
	}

	// 内容真正变化时必须写入
	c3 := LoadCache()
	p := c3.Get("bat")
	p.Installed = false
	p.InstalledVersion = ""
	if err := SaveCache(c3); err != nil {
		t.Fatalf("SaveCache changed: %v", err)
	}
	fi3, _ := os.Stat(CachePath())
	if fi3.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("expected write to happen when packages changed")
	}
}
