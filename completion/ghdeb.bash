# ghdeb bash completion
# 安装: source /usr/share/bash-completion/completions/ghdeb
# 或添加到 ~/.bashrc: source /usr/share/bash-completion/completions/ghdeb

_ghdeb_get_packages() {
    # 获取已管理的包列表
    local state_file="${XDG_STATE_HOME:-$HOME/.local/state}/ghdeb/installed.json"
    if [ -f "$state_file" ]; then
        # 从 JSON 提取包名（owner/repo 格式）
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
    fi
}

_ghdeb_get_catalog_names() {
    # 获取 catalog 中的短名称
    local catalog_file="/usr/share/ghdeb/catalog.toml"
    local user_catalog="${XDG_CONFIG_HOME:-$HOME/.config}/ghdeb/catalog.toml"
    
    # 读取系统 catalog
    if [ -f "$catalog_file" ]; then
        grep '^\[' "$catalog_file" | sed 's/^\[//;s/\]$//'
    fi
    # 读取用户 catalog
    if [ -f "$user_catalog" ]; then
        grep '^\[' "$user_catalog" | sed 's/^\[//;s/\]$//'
    fi
}

_ghdeb() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    commands="install upgrade reinstall scan search list ls catalog show info history remove rm purge clean set-repo test-homepage version help"
    
    # 如果是第一个参数，补全子命令
    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return 0
    fi
    
    # 根据子命令补全参数
    case "${COMP_WORDS[1]}" in
        install)
            # 补全 catalog 包名 + 已管理包
            local names=$(_ghdeb_get_catalog_names)
            local pkgs=$(_ghdeb_get_packages)
            COMPREPLY=( $(compgen -W "$names $pkgs" -- "$cur") )
            ;;
        upgrade|reinstall)
            # 补全已管理的包
            COMPREPLY=( $(compgen -W "$(_ghdeb_get_packages)" -- "$cur") )
            ;;
        show|info)
            # 补全 catalog 包名 + 已管理包
            local names=$(_ghdeb_get_catalog_names)
            local pkgs=$(_ghdeb_get_packages)
            COMPREPLY=( $(compgen -W "$names $pkgs" -- "$cur") )
            ;;
        remove|rm|purge|history)
            # 补全已管理的包
            COMPREPLY=( $(compgen -W "$(_ghdeb_get_packages)" -- "$cur") )
            ;;
        scan)
            COMPREPLY=( $(compgen -W "--deep" -- "$cur") )
            ;;
        list|ls)
            COMPREPLY=( $(compgen -W "--refresh" -- "$cur") )
            ;;
        clean)
            COMPREPLY=( $(compgen -W "--dry-run" -- "$cur") )
            ;;
        search)
            # 不补全，用户输入搜索词
            ;;
        catalog)
            # catalog 子命令补全
            if [ $COMP_CWORD -eq 2 ]; then
                COMPREPLY=( $(compgen -W "list show search add delete" -- "$cur") )
            elif [ "${COMP_WORDS[2]}" = "show" ] || [ "${COMP_WORDS[2]}" = "delete" ]; then
                local names=$(_ghdeb_get_catalog_names)
                COMPREPLY=( $(compgen -W "$names" -- "$cur") )
            elif [ "${COMP_WORDS[2]}" = "add" ]; then
                if [ $COMP_CWORD -eq 3 ]; then
                    : # 用户输入新名称
                else
                    COMPREPLY=( $(compgen -W "--repo --url --pretty-name --website --summary --gpg-key" -- "$cur") )
                fi
            fi
            ;;
    esac
    
    return 0
}

complete -F _ghdeb ghdeb
