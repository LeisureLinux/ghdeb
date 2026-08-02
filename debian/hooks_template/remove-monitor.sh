#!/bin/bash
# ghdeb dpkg hook: monitor package removal and update status automatically
# This script is triggered by Dpkg::Post-Invoke in apt config

STATE_FILE="/var/cache/ghdeb/installed.json"
CATALOG_FILE="/etc/ghdeb/catalog.toml"

# 检查必要文件是否存在
if [ ! -f "$STATE_FILE" ] || [ ! -f "$CATALOG_FILE" ]; then
    exit 0
fi

# 从 dpkg.log 获取最近被移除的软件包
# dpkg.log 格式: YYYY-MM-DD HH:MM:SS status removed <pkgname> <version>
REMOVED_PKGS=$(grep -E "^[0-9]{4}-[0-9]{2}-[0-9]{2}.*status removed" /var/log/dpkg.log 2>/dev/null | \
    awk '{print $5}' | tail -20)

if [ -z "$REMOVED_PKGS" ]; then
    exit 0
fi

# 调用 Python 脚本更新状态
echo "$REMOVED_PKGS" | /usr/bin/env python3 << PYEOF &>/dev/null || true
import json
import sys
from datetime import datetime

state_file = "$STATE_FILE"

try:
    # 加载状态
    with open(state_file, 'r') as f:
        state = json.load(f)
    
    packages = state.get('packages', {})
    updated = False
    
    # 从 stdin 读取被移除的包名
    for line in sys.stdin:
        pkg_name = line.strip()
        if not pkg_name:
            continue
        
        # 在 catalog 中查找匹配的条目
        for repo_key, record in packages.items():
            if record.get('pkg_name') == pkg_name or record.get('repo') == pkg_name:
                if not record.get('removed', False):
                    record['removed'] = True
                    record['updated_at'] = datetime.now().isoformat()
                    record.setdefault('history', []).append({
                        'action': 'remove',
                        'version': record.get('current_version', ''),
                        'timestamp': datetime.now().isoformat()
                    })
                    updated = True
    
    if updated:
        with open(state_file, 'w') as f:
            json.dump(state, f, indent=2)
        
except Exception as e:
    pass
PYEOF
