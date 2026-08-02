#!/bin/bash
set -e

VERSION="0.7.40"

# 支持的架构映射: go arch -> dpkg arch
declare -A ARCH_MAP=(
    ["amd64"]="amd64"
    # 以下架构暂未稳定，先注释，待后续稳定后再加回
    # ["arm64"]="arm64"
    # ["loong64"]="loong64"
    # ["riscv64"]="riscv64"
)

# 默认构建当前架构
TARGET_GOARCH="${1:-$(go env GOARCH)}"
TARGET_ARCH="${ARCH_MAP[$TARGET_GOARCH]}"

if [[ -z "$TARGET_ARCH" ]]; then
    echo "❌ 不支持的架构: $TARGET_GOARCH"
    echo "   支持的架构: ${!ARCH_MAP[*]}"
    exit 1
fi

PKG_NAME="ghdeb_${VERSION}_${TARGET_ARCH}"
PKG_DIR="dist/${PKG_NAME}"

echo "🔨 构建 ghdeb .deb 包 [${TARGET_ARCH}]..."

# 清理旧的构建
rm -rf "dist/${PKG_NAME}"*

# 交叉编译二进制
echo "📦 编译二进制文件 (GOOS=linux GOARCH=${TARGET_GOARCH})..."
GOOS=linux GOARCH="${TARGET_GOARCH}" go build -ldflags="-s -w -X main.version=${VERSION}" -o dist/ghdeb ./cmd/ghdeb/

# 创建包目录结构
echo "📁 创建包目录结构..."
mkdir -p ${PKG_DIR}/DEBIAN
mkdir -p ${PKG_DIR}/usr/bin
mkdir -p ${PKG_DIR}/etc/ghdeb
mkdir -p ${PKG_DIR}/usr/share/man/man1
mkdir -p ${PKG_DIR}/usr/share/man/zh_CN/man1
mkdir -p ${PKG_DIR}/usr/share/bash-completion/completions
mkdir -p ${PKG_DIR}/usr/share/zsh/site-functions
mkdir -p ${PKG_DIR}/usr/share/fish/vendor_completions.d
mkdir -p ${PKG_DIR}/usr/share/ghdeb/hooks

# 复制二进制
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

# zsh 补全
cp completion/ghdeb.zsh ${PKG_DIR}/usr/share/zsh/site-functions/_ghdeb
chmod 644 ${PKG_DIR}/usr/share/zsh/site-functions/_ghdeb

# fish 补全
cp completion/ghdeb.fish ${PKG_DIR}/usr/share/fish/vendor_completions.d/_ghdeb
chmod 644 ${PKG_DIR}/usr/share/fish/vendor_completions.d/_ghdeb

# 安装 curated catalog.toml（直接随包分发，不再扫描 OS 已装包）
cp catalog/catalog.toml ${PKG_DIR}/etc/ghdeb/catalog.toml
chmod 644 ${PKG_DIR}/etc/ghdeb/catalog.toml

# 安装 dpkg hook 脚本
cp debian/hooks_template/remove-monitor.sh ${PKG_DIR}/usr/share/ghdeb/hooks/remove-monitor.sh
chmod 755 ${PKG_DIR}/usr/share/ghdeb/hooks/remove-monitor.sh
cp debian/hooks_template/refresh-installed-cache.sh ${PKG_DIR}/usr/share/ghdeb/hooks/refresh-installed-cache.sh
chmod 755 ${PKG_DIR}/usr/share/ghdeb/hooks/refresh-installed-cache.sh

# 生成控制文件（替换 Architecture 字段）
sed -e "s/^Version:.*/Version: ${VERSION}/" -e "s/^Architecture:.*/Architecture: ${TARGET_ARCH}/" debian/control > ${PKG_DIR}/DEBIAN/control

# 复制 postinst/prerm/postrm 并设置权限
cp debian/postinst ${PKG_DIR}/DEBIAN/postinst
chmod 755 ${PKG_DIR}/DEBIAN/postinst
cp debian/prerm ${PKG_DIR}/DEBIAN/prerm
chmod 755 ${PKG_DIR}/DEBIAN/prerm
cp debian/postrm ${PKG_DIR}/DEBIAN/postrm
chmod 755 ${PKG_DIR}/DEBIAN/postrm

# catalog.toml 随包安装，作为 conffile 登记
printf '/etc/ghdeb/catalog.toml\n' > ${PKG_DIR}/DEBIAN/conffiles

# 统一修正所有目录权限为 755（避免 umask 导致 775）
find ${PKG_DIR} -type d -exec chmod 755 {} +

# 构建 .deb 包
echo "📦 打包 .deb [${TARGET_ARCH}]..."
dpkg-deb --root-owner-group --build ${PKG_DIR} dist/${PKG_NAME}.deb

echo "✅ 构建完成: dist/${PKG_NAME}.deb"
ls -lh dist/${PKG_NAME}.deb
