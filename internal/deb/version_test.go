package deb

import "testing"

func TestCompareVersions(t *testing.T) {
	// (a, b, want)  want: -1 = a<b, 0 = a==b, 1 = a>b
	cases := []struct {
		a, b string
		want int
	}{
		// 用户报告的原始场景：+58 是 debian revision
		{"1.17.0+58", "1.17.0", 1},
		{"1.17.0", "1.17.0+58", -1},

		// GitHub tag 带 v 前缀应等价于不带
		{"1.17.0+58", "v1.17.0", 1},
		{"1.2.0", "v1.2.0", 0},
		{"v3.13.3", "3.13.3", 0},
		{"1.4.9", "1.4.9", 0},
		{"15.2.0", "15.2.0", 0},

		// epoch
		{"1:1.0", "2.0", 1},     // epoch 1 > 无 epoch(0)
		{"2:1.0", "1:9.9", 1},   // 大 epoch 直接获胜
		{"0:1.0", "1.0", 0},     // epoch 0 == 无 epoch

		// revision 规则：无 revision 视为比有 revision 小
		{"1.0", "1.0-1", -1},
		{"1.0-2", "1.0-1", 1},
		{"1.0-1.1", "1.0-1", 1},

		// '~' 排序最前（用于预发布）
		{"1.0~rc1", "1.0", -1},
		{"1.0~rc1", "1.0-1", -1},
		{"1.0", "1.0~rc1", 1},

		// 字符序：'~' < 空 < 字母 < '+' < '.'
		{"1.0a", "1.0", 1},     // 末尾附加字母更大
		{"1.0", "1.0.", -1},    // '.' 比空大 => 1.0. > 1.0
		{"1.0+git", "1.0.1", -1}, // '+' < '.'
		{"1.0.1", "1.0+git", 1},

		// 前导零与数值比较
		{"1.007", "1.7", 0},
		{"1.01.1", "1.1.1", 0},
		{"1.1", "1.10", -1},
		{"1.9", "1.10", -1},

		// 相等
		{"1.2.0", "1.2.0", 0},
		{"1.2.0+1", "1.2.0+1", 0},
	}

	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestCompareVersionsSymmetric 校验结果反号对称。
func TestCompareVersionsSymmetric(t *testing.T) {
	pairs := [][2]string{
		{"1.17.0+58", "1.17.0"},
		{"2.0", "1:1.0"},
		{"1.0~rc1", "1.0"},
		{"15.2.0", "v15.2.0"},
		{"1.0-2", "1.0-10"},
	}
	for _, p := range pairs {
		a, b := CompareVersions(p[0], p[1]), CompareVersions(p[1], p[0])
		if a != -b {
			t.Errorf("symmetric mismatch for %q,%q: got %d vs %d", p[0], p[1], a, b)
		}
	}
}
