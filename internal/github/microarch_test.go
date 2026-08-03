package github

import (
	"testing"

	"github.com/leisurelinux/ghdeb/internal/deb"
)

// TestDaeMicroArchSelection 验证 dae 多指令集版本能按 CPU 微架构选中最合适的 .deb
func TestDaeMicroArchSelection(t *testing.T) {
	names := []string{
		"dae-linux-x86_64.deb",
		"dae-linux-x86_64_v2_sse.deb",
		"dae-linux-x86_64_v3_avx2.deb",
	}
	assets := make([]Asset, len(names))
	for i, n := range names {
		assets[i] = Asset{Name: n, BrowserDownloadURL: "https://x/" + n, Size: 100}
	}
	rel := &Release{TagName: "v2.0.0", Assets: assets}
	arch, err := deb.DetectArch()
	if err != nil {
		t.Fatal(err)
	}
	got, err := FindDebAsset(rel, arch)
	if err != nil {
		t.Fatal(err)
	}
	level := deb.DetectX86MicroArch()
	if level < 1 {
		t.Skip("非 x86_64 平台，跳过")
	}
	want := "dae-linux-x86_64.deb"
	switch level {
	case 2:
		want = "dae-linux-x86_64_v2_sse.deb"
	case 3, 4:
		want = "dae-linux-x86_64_v3_avx2.deb"
	}
	if got.Name != want {
		t.Errorf("CPU 微架构级别 %d，选中 %s，期望 %s", level, got.Name, want)
	}
}
