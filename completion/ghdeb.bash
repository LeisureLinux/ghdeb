# ghdeb bash completion
# 安装: source /usr/share/bash-completion/completions/ghdeb
# 或添加到 ~/.bashrc: source /usr/share/bash-completion/completions/ghdeb

_ghdeb_get_packages() {
    # 获取已管理的包列表
    local state_file="${XDG_STATE_HOME:-$HOME/.local/state}/ghdeb/installed.json"
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
    # 获取 catalog 中的短名称
    local catalog_file="/usr/share/ghdeb/catalog.toml"
    local user_catalog="${XDG_CONFIG_HOME:-$HOME/.config}/ghdeb/catalog.toml"
    
    {
        [ -f "$catalog_file" ] && grep '^\[' "$catalog_file"
        [ -f "$user_catalog" ] && grep '^\[' "$user_catalog"
    } 2>/dev/null | sed 's/^\[//;s/\]$//' | sort -u
}

_ghdeb() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    commands="install upgrade reinstall scan search list ls catalog show info history remove rm purge clean set-repo test-homepage version help"
    
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
        upgrade|reinstall|history|remove|rm|purge)
            COMPREPLY=( $(compgen -W "$(_ghdeb_get_packages)" -- "$cur") )
            ;;
        scan)
            COMPREPLY=( $(compgen -W "--deep" -- "$cur") )
            ;;
        list|ls)
            COMPREPLY=( $(compgen -W "--refresh -r" -- "$cur") )
            ;;
        clean)
            COMPREPLY=( $(compgen -W "--dry-run -n" -- "$cur") )
            ;;
        search)
            # 不补全，用户输入搜索词
            ;;
        catalog)
            # catalog 子命令补全
            local catalog_subcmds="list show search add delete"
            
            # 第二个参数：补全 catalog 子命令
            if [ $COMP_CWORD -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$catalog_subcmds" -- "$cur") )
                return 0
            fi
            
            # 第三个及以后参数：根据 catalog 子命令补全
            case "${COMP_WORDS[2]}" in
                show|delete)
                    local names
                    names=$(_ghdeb_get_catalog_names)
                    COMPREPLY=( $(compgen -W "$names" -- "$cur") )
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
