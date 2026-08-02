#!/bin/bash
# ghdeb dpkg hook: monitor package removal and update status automatically
# This script is triggered by Dpkg::Post-Invoke in apt config

STATE_FILE="/var/cache/ghdeb/installed.json"
CATALOG_FILE="/etc/ghdeb/catalog.toml"

# 仅当有包被移除时才处理
if [ -z "$1" ]; then
    exit 0
fi

REMOVED_PKG="$1"

# 检查状态文件是否存在
if [ ! -f "$STATE_FILE" ]; then
    exit 0
fi

# 检查 catalog 文件是否存在
if [ ! -f "$CATALOG_FILE" ]; then
    exit 0
fi

# 调用 ghdeb daemon 或 Python 脚本来更新状态
# 这里使用一个简单的方案：通过 python3 解析 JSON 并更新
/usr/bin/env python3 << PYEOF &>/dev/null || true
import json
import os
import re

state_file = "$STATE_FILE"
catalog_file = "$CATALOG_FILE"
removed_pkg = "$REMOVED_PKG"

try:
    # 加载状态
    with open(state_file, 'r') as f:
        state = json.load(f)
    
    packages = state.get('packages', {})
    updated = False
    
    for repo_key, record in packages.items():
        # 检查包名是否匹配
        if record.get('pkg_name') == removed_pkg or record.get('repo') == removed_pkg:
            if not record.get('removed', False):
                record['removed'] = True
                import datetime
                record['updated_at'] = datetime.datetime.now().isoformat()
                record.setdefault('history', []).append({
                    'action': 'remove',
                    'version': record.get('current_version', ''),
                    'timestamp': datetime.datetime.now().isoformat()
                })
                updated = True
    
    if updated:
        with open(state_file, 'w') as f:
            json.dump(state, f, indent=2)
        
except Exception as e:
    pass
PYEOF
