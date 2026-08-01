// ghdeb - 双语支持（封装 internal/i18n）
package main

import (
	"github.com/leisurelinux/ghdeb/internal/i18n"
)

// isChinese 判断当前 locale 是否为中文
func isChinese() bool {
	return i18n.IsChinese()
}

// T 双语翻译函数
func T(zh, en string) string {
	return i18n.T(zh, en)
}
