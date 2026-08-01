# ghdeb

从 GitHub Releases 一键安装/升级 `.deb` 包。

```
ghdeb install sharkdp/bat      # 安装 bat 最新版
ghdeb upgrade                  # 升级所有已安装的包
ghdeb list                     # 查看已安装列表
```

## 为什么需要这个工具？

很多优秀的 CLI 工具（bat、fd、ripgrep、gh 等）在 GitHub Releases 提供 `.deb` 包，但安装流程繁琐：

1. 打开 GitHub releases 页面
2. 找到最新的 `.deb` 文件
3. 判断哪个文件匹配自己的架构（amd64? arm64?）
4. 复制下载链接，`wget` 下载
5. `sudo dpkg -i` 安装
6. 升级时重复以上步骤

**ghdeb 把这些步骤简化为一行命令。**

## 安装

```bash
# 从源码编译
make build
sudo make install

# 或直接 go install
go install github.com/leisurelinux/ghdeb/cmd/ghdeb@latest
```

## 使用方法

### 安装

```bash
# 安装最新版
ghdeb install sharkdp/bat

# 安装指定版本
ghdeb install sharkdp/bat@v0.25.0

# 安装 GitHub CLI
ghdeb install cli/cli
```

### 升级

```bash
# 升级所有已安装的包
ghdeb upgrade

# 只升级某个包
ghdeb upgrade sharkdp/bat
```

### 查看信息

```bash
# 查看远程最新 release 信息（不安装）
ghdeb info BurntSushi/ripgrep

# 列出已安装的包
ghdeb list
```

### 移除记录

```bash
# 移除安装记录（不卸载软件本身）
ghdeb remove sharkdp/bat
```

## 工作原理

1. **架构检测**：通过 `dpkg --print-architecture` 检测系统架构
2. **版本查询**：调用 GitHub API 获取最新 release
3. **智能匹配**：从 release assets 中找到匹配当前架构的 `.deb` 文件
   - 优先选择标准包，跳过 musl/static/portable 变体
   - 支持多种架构命名（amd64/x86_64/x64、arm64/aarch64 等）
4. **下载安装**：下载 `.deb` 并用 `dpkg -i` 安装，自动处理依赖
5. **状态追踪**：记录已安装版本，避免重复安装，支持增量升级

## 架构匹配

| 系统架构 | 匹配的文件名关键词 |
|---------|------------------|
| amd64   | amd64, x86_64, x86-64, x64 |
| arm64   | arm64, aarch64 |
| armhf   | armhf, armv7l, armv7 |
| i386    | i386, x86, i686, 386 |

## 环境变量

| 变量 | 说明 |
|-----|------|
| `GITHUB_TOKEN` / `GH_TOKEN` | GitHub 个人访问令牌，提高 API 速率限制（未认证 60 次/小时 → 认证 5000 次/小时） |

## 数据存储

| 路径 | 说明 |
|-----|------|
| `~/.cache/ghdeb/` | 下载的 .deb 文件缓存 |
| `~/.local/state/ghdeb/installed.json` | 已安装包的状态记录 |

遵循 XDG 规范，可通过 `XDG_CACHE_HOME` 和 `XDG_STATE_HOME` 自定义。

## 已有类似工具？

调研发现现有工具要么不活跃、要么过于复杂、要么是服务端方案：

| 工具 | 问题 |
|-----|------|
| gitdeb | Shell 脚本，0 stars，无社区验证 |
| debian-package-installer | Python，需要 JSON 配置文件 |
| github-apt-repos (★110) | 服务端工具，构建 APT 仓库 |
| inapt (★8) | 服务端 APT 仓库代理 |
| apt-transport-github (★13) | 标注 "NOT YET READY"，多年未更新 |

**ghdeb 填补了轻量级客户端 CLI 工具的空白。**

## License

MIT
