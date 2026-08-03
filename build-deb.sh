#!/bin/bash
set -e

VERSION="0.7.49"

# 架构规格表: 别名(参数) -> "GOARCH|GOARM|dpkg架构"
#   GOOS 固定为 linux。
#   armhf 由 Go 的 GOARCH=arm + GOARM=7 交叉编译而来。
#   可同时接受 go 架构名与 dpkg/发行版常用名（如 loongarch64 别名）。
declare -A ARCH_SPEC=(
    [amd64]="amd64||amd64"
    [x86_64]="amd64||amd64"
    [arm64]="arm64||arm64"
    [aarch64]="arm64||arm64"
    [armhf]="arm|7|armhf"
    [armv7]="arm|7|armhf"
    [arm]="arm|7|armhf"
    [loong64]="loong64||loong64"
    [loongarch64]="loong64||loong64"
    [riscv64]="riscv64||riscv64"
)

# 默认构建当前架构（go env GOARCH → 规范化到 ARCH_SPEC 的 key）
DEFAULT_GOARCH="$(go env GOARCH)"
case "$DEFAULT_GOARCH" in
    arm) DEFAULT_KEY="armhf" ;;
    *)   DEFAULT_KEY="$DEFAULT_GOARCH" ;;
esac

TARGET_KEY="${1:-$DEFAULT_KEY}"
SPEC="${ARCH_SPEC[$TARGET_KEY]}"

if [[ -z "$SPEC" ]]; then
    echo "❌ 不支持的架构: $TARGET_KEY"
    echo "   支持的架构: ${!ARCH_SPEC[*]}"
    exit 1
fi

# 解析 GOARCH|GOARM|dpkg 三段
GOARCH="${SPEC%%|*}"
REST="${SPEC#*|}"
GOARM="${REST%%|*}"
TARGET_ARCH="${REST#*|}"

PKG_NAME="ghdeb_${VERSION}_${TARGET_ARCH}"
PKG_DIR="dist/${PKG_NAME}"

echo "🔨 构建 ghdeb .deb 包 [${TARGET_ARCH}] (GOARCH=${GOARCH}${GOARM:+ GOARM=${GOARM}})..."

# 清理旧的构建
rm -rf "dist/${PKG_NAME}"

# 交叉编译二进制（GOARM 仅 arm 需要）
echo "📦 编译二进制文件 (GOOS=linux GOARCH=${GOARCH}${GOARM:+ GOARM=${GOARM}})..."
ENV_GOARM=()
if [[ -n "$GOARM" ]]; then
    ENV_GOARM=(GOARM="$GOARM")
fi
env GOOS=linux GOARCH="$GOARCH" "${ENV_GOARM[@]}" \
    go build -ldflags="-s -w -X main.version=${VERSION}" -o dist/ghdeb ./cmd/ghdeb/

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

# 生成控制文件（替换 Version 与 Architecture 字段）
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
