// Package i18n 提供双语支持
package i18n

import (
	"os"
	"strings"
)

// IsChinese 判断当前 locale 是否为中文
func IsChinese() bool {
	lang := os.Getenv("LANG")
	return strings.HasPrefix(lang, "zh")
}

// T 双语翻译函数
func T(zh, en string) string {
	if IsChinese() {
		return zh
	}
	return en
}
