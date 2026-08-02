# ghdeb bash completion
# 安装: source /usr/share/bash-completion/completions/ghdeb
# 或添加到 ~/.bashrc: source /usr/share/bash-completion/completions/ghdeb

_ghdeb_get_packages() {
    # 获取已管理的包列表（从 /var/cache/ghdeb/installed.json）
    local state_file="/var/cache/ghdeb/installed.json"
    if [ -f "$state_file" ]; then
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
    # 获取 catalog 中的短名称（从 /etc/ghdeb/catalog.toml）
    local catalog_file="/etc/ghdeb/catalog.toml"
    [ -f "$catalog_file" ] && grep '^\[' "$catalog_file" | sed 's/^\[//;s/\]$//' | sort -u
}

_ghdeb() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    commands="install update upgrade reinstall scan search list ls catalog show info history purge clean test-homepage version help"
    
    # 第一个参数：补全顶级子命令
    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return 0
    fi
    
    # 根据顶级子命令补全参数
    case "${COMP_WORDS[1]}" in
        install|show|info)
            local names pkgs
            names=$(_ghdeb_get_catalog_names)
            pkgs=$(_ghdeb_get_packages)
            COMPREPLY=( $(compgen -W "$names $pkgs" -- "$cur") )
            ;;
        upgrade|reinstall|history|purge)
            COMPREPLY=( $(compgen -W "$(_ghdeb_get_packages)" -- "$cur") )
            ;;
        scan)
            COMPREPLY=( $(compgen -W "--deep" -- "$cur") )
            ;;
        list|ls)
            COMPREPLY=( $(compgen -W "--refresh -r --json" -- "$cur") )
            ;;
        clean)
            COMPREPLY=( $(compgen -W "--dry-run -n" -- "$cur") )
            ;;
        search)
            # 不补全，用户输入搜索词
            ;;
        catalog)
            # catalog 子命令补全
            local catalog_subcmds="list show search add modify delete validate"
            
            # 第二个参数：补全 catalog 子命令
            if [ $COMP_CWORD -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$catalog_subcmds" -- "$cur") )
                return 0
            fi
            
            # 第三个及以后参数：根据 catalog 子命令补全
            case "${COMP_WORDS[2]}" in
                show|delete|validate|modify)
                    local names
                    names=$(_ghdeb_get_catalog_names)
                    COMPREPLY=( $(compgen -W "$names" -- "$cur") )
                    ;;
                modify)
                    if [ $COMP_CWORD -ge 4 ]; then
                        COMPREPLY=( $(compgen -W "--repo" -- "$cur") )
                    fi
                    ;;
                validate)
                    if [ $COMP_CWORD -eq 3 ]; then
                        COMPREPLY=( $(compgen -W "--all" -- "$cur") )
                    fi
                    ;;
                add)
                    # 第四个参数开始补全选项
                    if [ $COMP_CWORD -ge 4 ]; then
                        COMPREPLY=( $(compgen -W "--repo --url --pretty-name --website --summary --gpg-key" -- "$cur") )
                    fi
                    ;;
                search)
                    # 不补全，用户输入搜索词
                    ;;
            esac
            ;;
    esac
    
    return 0
}

complete -F _ghdeb ghdeb
