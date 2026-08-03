#!/bin/bash
# ghdeb apt hook: 新软件包经 apt/dpkg 安装后，清空统一缓存中各 package 的已装状态
# 下次 ghdeb update 将重新实查 dpkg，回填最新已装版本（避免 24h TTL 内读到旧版本）
# 统一缓存为单文件 /var/cache/ghdeb/cache.json（扁平化：仅 packages 段），由 Dpkg::Post-Invoke 触发
# 配置见 /etc/apt/apt.conf.d/99ghdeb-hook

# 容错：脚本或自身缺失时静默退出
[ -x /usr/share/ghdeb/hooks/refresh-installed-cache.sh ] || exit 0

CACHE_FILE="/var/cache/ghdeb/cache.json"

# 直接更新单 json：清空各 package 的已装相关字段，其余字段（repo/github_version/arch/pkg_file）保留
/usr/bin/env python3 << PYEOF &>/dev/null || true
import json
import os
from datetime import datetime

cache_file = "$CACHE_FILE"

try:
    if os.path.exists(cache_file):
        with open(cache_file, 'r') as f:
            data = json.load(f)
    else:
        data = {}

    packages = data.get('packages', {})
    for p in packages.values():
        p['installed'] = False
        p['install_time'] = ''
        p['installed_version'] = ''
        p['upgradable'] = False
    data['packages'] = packages
    data['updated_at'] = datetime.now().isoformat()

    with open(cache_file, 'w') as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
except Exception:
    pass
PYEOF

exit 0
