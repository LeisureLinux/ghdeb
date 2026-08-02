#!/bin/bash
# ghdeb apt hook: 新软件包经 apt/dpkg 安装后，清除已装版本缓存
# 下次 ghdeb list 将重新实查 dpkg，回填最新已装版本（避免 24h TTL 内读到旧版本）
# 由 Dpkg::Post-Invoke 触发，见 /etc/apt/apt.conf.d/99ghdeb-hook

# 容错：脚本或自身缺失时静默退出
[ -x /usr/share/ghdeb/hooks/refresh-installed-cache.sh ] || exit 0

CACHE_FILE="installed_versions.json"

# 遍历各用户主目录，删除其已装版本缓存
for home in /home/* /root; do
    [ -d "$home/.cache/ghdeb" ] || continue
    rm -f "$home/.cache/ghdeb/$CACHE_FILE"
done

# 兼容 XDG_CACHE_HOME 指向别处的用户
for envfile in /home/*/.config/environment.d/*; do
    [ -f "$envfile" ] || continue
    val=$(grep -E '^XDG_CACHE_HOME=' "$envfile" 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"')
    [ -n "$val" ] && rm -f "$val/ghdeb/$CACHE_FILE"
done

exit 0
