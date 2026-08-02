# Fish shell completion for ghdeb
# 安装: cp ghdeb.fish ~/.config/fish/completions/
# 或: sudo cp ghdeb.fish /usr/share/fish/vendor_completions.d/

# 子命令列表
set -l subcommands install upgrade reinstall scan search list ls catalog show info history purge clean test-homepage version help

# 子命令补全
complete -c ghdeb -f -n "not __fish_seen_subcommand_from $subcommands" \
    -a "$subcommands"

# 获取已管理的包列表（从 /var/cache/ghdeb/installed.json）
function __ghdeb_installed_packages
    set -l state_file /var/cache/ghdeb/installed.json
    if test -f "$state_file"
        python3 -c "
import json, sys
try:
    with open('$state_file') as f:
        data = json.load(f)
    for pkg in data.get('packages', {}).keys():
        print(pkg)
except:
    pass
" 2>/dev/null
    end
end

# 获取 catalog 中的包名（从 /etc/ghdeb/catalog.toml）
function __ghdeb_catalog_names
    set -l catalog_file /etc/ghdeb/catalog.toml
    if test -f "$catalog_file"
        grep '^\[' "$catalog_file" | sed 's/^\[//;s/\]$//'
    end
end

# install: 补全 catalog 包名 + 已管理的 owner/repo
complete -c ghdeb -f -n "__fish_seen_subcommand_from install" \
    -a "(__ghdeb_catalog_names) (__ghdeb_installed_packages)"

# upgrade: 补全已管理的包
complete -c ghdeb -f -n "__fish_seen_subcommand_from upgrade" \
    -a "(__ghdeb_installed_packages)"

# reinstall: 补全已管理的包
complete -c ghdeb -f -n "__fish_seen_subcommand_from reinstall" \
    -a "(__ghdeb_installed_packages)"

# show/info: 补全 catalog 包名 + 已管理的包
complete -c ghdeb -f -n "__fish_seen_subcommand_from show info" \
    -a "(__ghdeb_catalog_names) (__ghdeb_installed_packages)"

# purge: 补全已管理的包
complete -c ghdeb -f -n "__fish_seen_subcommand_from purge" \
    -a "(__ghdeb_installed_packages)"

# history: 补全已管理的包
complete -c ghdeb -f -n "__fish_seen_subcommand_from history" \
    -a "(__ghdeb_installed_packages)"

# scan: --deep 选项
complete -c ghdeb -f -n "__fish_seen_subcommand_from scan" \
    -a "--deep" -d "深度扫描（抓取 Homepage 查找 GitHub 链接）"

# list: --refresh 和 --json 选项
complete -c ghdeb -f -n "__fish_seen_subcommand_from list ls" \
    -a "--refresh --json" -d "强制刷新版本缓存 / JSON 输出"

# clean: --dry-run 选项
complete -c ghdeb -f -n "__fish_seen_subcommand_from clean" \
    -a "--dry-run" -d "仅显示将清理的内容"

# search: 无补全（用户输入搜索词）

# catalog: 子命令补全
complete -c ghdeb -f -n "__fish_seen_subcommand_from catalog; and not __fish_seen_subcommand_from list show search add modify delete validate" \
    -a "list show search add modify delete validate"

# catalog show/delete/validate/modify: 补全 catalog 包名
complete -c ghdeb -f -n "__fish_seen_subcommand_from catalog; and __fish_seen_subcommand_from show delete validate modify" \
    -a "(__ghdeb_catalog_names)"

# catalog modify: --repo 选项
complete -c ghdeb -f -n "__fish_seen_subcommand_from catalog; and __fish_seen_subcommand_from modify" \
    -a "--repo" -d "GitHub 仓库 (owner/repo)"

# catalog validate: --all 选项
complete -c ghdeb -f -n "__fish_seen_subcommand_from catalog; and __fish_seen_subcommand_from validate" \
    -a "--all" -d "校验并清洗全部条目"

# catalog add: 选项补全
complete -c ghdeb -f -n "__fish_seen_subcommand_from catalog; and __fish_seen_subcommand_from add" \
    -a "--repo --url --pretty-name --website --summary --gpg-key"
