package deb

import "testing"

func TestAssetMicroArch(t *testing.T) {
	cases := []struct {
		name string
		want int
	}{
		{"dae-linux-x86_64_v3_avx2.deb", 3},
		{"dae-linux-x86_64_v2_sse.deb", 2},
		{"foo-x86_64_v4_avx512.deb", 4},
		{"dae-linux-x86_64.deb", 0},
		{"x86_64v3.deb", 3},
		{"foo-amd64.deb", 0},
		{"foo_arm64.deb", 0},
	}
	for _, c := range cases {
		if got := AssetMicroArch(c.name); got != c.want {
			t.Errorf("AssetMicroArch(%q) = %d, want %d", c.name, got, c.want)
		}
	}
}
