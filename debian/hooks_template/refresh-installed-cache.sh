#!/bin/bash
# ghdeb apt hook: 新软件包经 apt/dpkg 安装后，自动重跑 `ghdeb update`，
# 立即刷新 /var/cache/ghdeb/cache.json 的已装状态/版本（而非等到下次手工 update）。
# 由 Dpkg::Post-Invoke 触发，配置见 /etc/apt/apt.conf.d/99ghdeb-hook。
#
# 说明：
#  - 本脚本在 apt 运行期间以 root 执行，ghdeb update 可直接写缓存，无需 sudo。
#  - ghdeb update 优先命中 24h 内 GitHub 版本缓存（通常无网络请求），主要工作是
#    重扫 /var/lib/dpkg/status 回填已装信息，开销很小。
#  - 用 setsid 后台异步执行，避免拖慢 apt 收尾；输出丢弃以免刷屏。

# 容错：ghdeb 二进制缺失时静默退出
[ -x /usr/bin/ghdeb ] || exit 0

# 后台异步重跑 update（脱离控制终端与进程组，apt 退出后不会被回收）
setsid /usr/bin/ghdeb update >/dev/null 2>&1 &
disown 2>/dev/null || true

exit 0
