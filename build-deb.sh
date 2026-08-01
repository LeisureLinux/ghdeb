#!/bin/bash
set -e

VERSION="0.3.5"
ARCH="amd64"
PKG_NAME="ghdeb_${VERSION}_${ARCH}"
PKG_DIR="dist/${PKG_NAME}"

echo "🔨 构建 ghdeb .deb 包..."

# 清理旧的构建
rm -rf dist/${PKG_NAME}*

# 编译二进制
echo "📦 编译二进制文件..."
go build -ldflags="-s -w" -o dist/ghdeb ./cmd/ghdeb/

# 创建包目录结构
echo "📁 创建包目录结构..."
mkdir -p ${PKG_DIR}/DEBIAN
mkdir -p ${PKG_DIR}/usr/bin
# 默认 man（英文）
mkdir -p ${PKG_DIR}/usr/share/man/man1
# 中文 man
mkdir -p ${PKG_DIR}/usr/share/man/zh_CN/man1
# bash 补全
mkdir -p ${PKG_DIR}/usr/share/bash-completion/completions
# zsh 补全
mkdir -p ${PKG_DIR}/usr/share/zsh/site-functions

# 复制文件
cp dist/ghdeb ${PKG_DIR}/usr/bin/ghdeb
chmod 755 ${PKG_DIR}/usr/bin/ghdeb

# 英文版 man（默认）
cp man/en_US/ghdeb.1 ${PKG_DIR}/usr/share/man/man1/ghdeb.1
chmod 644 ${PKG_DIR}/usr/share/man/man1/ghdeb.1

# 中文版 man
cp man/zh_CN/ghdeb.1 ${PKG_DIR}/usr/share/man/zh_CN/man1/ghdeb.1
chmod 644 ${PKG_DIR}/usr/share/man/zh_CN/man1/ghdeb.1

# bash 补全
cp completion/ghdeb.bash ${PKG_DIR}/usr/share/bash-completion/completions/ghdeb
chmod 644 ${PKG_DIR}/usr/share/bash-completion/completions/ghdeb

# zsh 补全（注意：zsh 补全文件名需要加下划线前缀）
cp completion/ghdeb.zsh ${PKG_DIR}/usr/share/zsh/site-functions/_ghdeb
chmod 644 ${PKG_DIR}/usr/share/zsh/site-functions/_ghdeb

# 复制控制文件
cp debian/control ${PKG_DIR}/DEBIAN/control

# 构建 .deb 包
echo "📦 打包 .deb..."
dpkg-deb --root-owner-group --build ${PKG_DIR} dist/${PKG_NAME}.deb

echo "✅ 构建完成: dist/${PKG_NAME}.deb"
ls -lh dist/${PKG_NAME}.deb
