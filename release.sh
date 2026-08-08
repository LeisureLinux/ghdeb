#!/bin/bash
# 发版脚本：构建 5 架构 .deb，生成校验和，并创建 GitHub release
set -e

# --push: 构建后自动调用 gh release create
PUSH=0
[[ "$1" == "--push" ]] && PUSH=1

# 版本号取自 build-deb.sh，避免两处维护
# 版本号：CI 里由 VERSION 环境变量传入（从 tag 提取）；本地手动时从 build-deb.sh 解析
VERSION="${VERSION:-$(sed -n 's/^VERSION="\${VERSION:-\([^}]*\)}"/\1/p; t; s/^VERSION="\([^"]*\)"/\1/p' build-deb.sh | head -1)}"
TAG="v${VERSION}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "🚀 ghdeb 发版 v${VERSION}（tag: ${TAG}）"
echo ""

# 1. 构建全部架构
echo "🔨 构建 5 个架构..."
bash build-all-deb.sh

# 2. 收集产物
DEBS=(dist/ghdeb_${VERSION}_*.deb)
echo ""
echo "📦 产物:"
for d in "${DEBS[@]}"; do
    echo "  - $d"
done

# 3. 生成 SHA256SUMS（内容路径去掉 dist/ 前缀）
echo ""
echo "🔐 生成 SHA256SUMS..."
( cd dist && sha256sum ghdeb_${VERSION}_*.deb ) | tee dist/SHA256SUMS

# 4. 生成 release notes（自上个 tag 以来的提交）
echo ""
echo "📝 生成 release notes..."
PREV_TAG="$(git describe --abbrev=0 --tags "${TAG}" 2>/dev/null || git tag --sort=-version:refname | head -1 | sed "s/^/x/;s/^x//")"
NOTES="dist/release-notes-${VERSION}.md"

{
    echo "## ghdeb v${VERSION}"
    echo ""
    echo "支持 5 种架构的 Debian/Ubuntu 安装包："
    echo ""
    echo "- amd64 (x86_64)"
    echo "- arm64 (aarch64)"
    echo "- armhf (armv7)"
    echo "- loong64 (loongarch64)"
    echo "- riscv64"
    echo ""
    echo "### 更新内容"
    echo ""
    if [[ -n "$PREV_TAG" && "$PREV_TAG" != "x" ]]; then
        git log --oneline --no-merges "${PREV_TAG}..HEAD"
    else
        git log --oneline --no-merges -15
    fi
    echo ""
    echo "### 校验和（SHA256）"
    echo '```'
    cat dist/SHA256SUMS
    echo '```'
} > "$NOTES"
echo "  - 已写入 $NOTES"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
CMD=(gh release create "${TAG}" "${DEBS[@]}" dist/SHA256SUMS \
    --title "ghdeb v${VERSION}" \
    --notes-file "$NOTES")

if [[ "$PUSH" == "1" ]]; then
    echo "🚀 执行 gh release create ..."
    "${CMD[@]}"
    echo "✅ 发布完成: https://github.com/LeisureLinux/ghdeb/releases/tag/${TAG}"
else
    echo "预览模式（未推送）。执行以下命令即可发布："
    echo ""
    echo "${CMD[@]}"
    echo ""
    echo "或直接运行: bash release.sh --push"
fi
