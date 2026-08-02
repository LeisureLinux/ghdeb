package state

import "testing"

func TestSplitProvides(t *testing.T) {
	cases := map[string][]string{
		"p7zip, p7zip-full": {"p7zip", "p7zip-full"},
		"adwaita-icon-theme-full (= 48.1-1), gnome-icon-theme-symbolic": {"adwaita-icon-theme-full", "gnome-icon-theme-symbolic"},
		"mail-reader, mailx": {"mail-reader", "mailx"},
		"httpd":              {"httpd"},
	}
	for in, want := range cases {
		got := splitProvides(in)
		if len(got) != len(want) {
			t.Fatalf("splitProvides(%q) len=%d want %d", in, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitProvides(%q)[%d]=%q want %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestResolveVirtualPkg(t *testing.T) {
	all := []DpkgPackage{
		{Name: "fd", Status: "install ok not-installed"}, // 虚包/残留
		{Name: "fd-find", Provides: nil},                 // 实包（不 Provide fd）
		{Name: "bind9-dnsutils", Provides: []string{"dnsutils"}},
		{Name: "real-dns", Provides: []string{"dnsutils"}},
	}

	// 未声明 Provides 的虚包走内建别名表
	if got := resolveVirtualPkg("fd", all); got != "fd-find" {
		t.Errorf("fd -> %q, want fd-find", got)
	}
	// 实包自身不变
	if got := resolveVirtualPkg("fd-find", all); got != "fd-find" {
		t.Errorf("fd-find -> %q, want fd-find", got)
	}
	// 走 Provides 声明（取第一个提供者）
	if got := resolveVirtualPkg("dnsutils", all); got != "bind9-dnsutils" {
		t.Errorf("dnsutils -> %q, want bind9-dnsutils", got)
	}
	// 未知名原样返回
	if got := resolveVirtualPkg("nope-xyz", all); got != "nope-xyz" {
		t.Errorf("nope-xyz -> %q, want unchanged", got)
	}
}
