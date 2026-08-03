package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestUnifiedCache 验证统一缓存的读写与合并行为
func TestUnifiedCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GHDEB_CACHE_DIR", dir)
	if got := CacheDir(); got != dir {
		t.Fatalf("CacheDir=%q want %q", got, dir)
	}

	// 写入 release 与 installed
	SetCachedRelease("owner", "repo", "v1.0.0")
	SetCachedInstalled("mypkg", "1.2.3")

	// 写入快照
	c := LoadCache()
	c.UpdatedAt = time.Now().Format(time.RFC3339)
	c.Set("mypkg", &SnapshotPkg{
		Repo: "owner/repo", Installed: true,
		InstalledVersion: "1.2.3", LatestVersion: "v1.0.0", Upgradeable: true,
	})
	if err := SaveCache(c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	// 验证单文件确实只落一个 json
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 json file, got %d: %v", len(files), files)
	}

	// 重新加载校验三段数据都在同一文件
	c2 := LoadCache()
	if v := c2.Get("mypkg"); v == nil || !v.Upgradeable || v.LatestVersion != "v1.0.0" {
		t.Fatalf("snapshot mismatch: %+v", v)
	}
	if v := GetCachedRelease("owner", "repo"); v != "v1.0.0" {
		t.Fatalf("release mismatch: %q", v)
	}
	if v := GetCachedInstalled("mypkg"); v != "1.2.3" {
		t.Fatalf("installed mismatch: %q", v)
	}

	// 清空已装版本
	ClearCachedInstalled()
	if v := GetCachedInstalled("mypkg"); v != "" {
		t.Fatalf("installed should be cleared, got %q", v)
	}
	// release 与快照不受影响（同一文件内部分段清理）
	if v := GetCachedRelease("owner", "repo"); v != "v1.0.0" {
		t.Fatalf("release should survive, got %q", v)
	}
	if c2 = LoadCache(); c2.Get("mypkg") == nil {
		t.Fatalf("snapshot should survive")
	}

	_ = filepath.Join
}
