#!/bin/bash
set -e

echo "🔨 构建所有架构的 ghdeb .deb 包..."
echo ""

# 目标架构：amd64 + 新增的 arm64 / armhf / loong64 / riscv64
ARCHS=("amd64" "arm64" "armhf" "loong64" "riscv64")
FAILED=()

for arch in "${ARCHS[@]}"; do
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    if bash build-deb.sh "$arch"; then
        echo ""
    else
        echo "❌ 构建 ${arch} 失败"
        FAILED+=("$arch")
        echo ""
    fi
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📦 构建结果:"
ls -lh dist/ghdeb_*_*.deb 2>/dev/null || echo "   (无)"
echo ""

if [[ ${#FAILED[@]} -gt 0 ]]; then
    echo "⚠️  失败的架构: ${FAILED[*]}"
    exit 1
else
    echo "✅ 所有架构构建完成！"
fi
