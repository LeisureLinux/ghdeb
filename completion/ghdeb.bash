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

_ghdeb() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    commands="install upgrade scan list history remove set-repo info version help"
    
    # 补全命令
    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return 0
    fi
    
    # 补全包名
    case "${COMP_WORDS[1]}" in
        install|info)
            # 这些命令需要 owner/repo 格式，不提供补全
            ;;
        upgrade|history|remove)
            # 补全已管理的包
            local packages=$(_ghdeb_get_packages)
            COMPREPLY=( $(compgen -W "$packages" -- "$cur") )
            ;;
        set-repo)
            # 第一个参数是包名，第二个是 owner/repo
            if [ $COMP_CWORD -eq 2 ]; then
                local packages=$(_ghdeb_get_packages)
                COMPREPLY=( $(compgen -W "$packages" -- "$cur") )
            fi
            ;;
        scan)
            # 补全 --deep 选项
            if [[ "$cur" == -* ]]; then
                COMPREPLY=( $(compgen -W "--deep" -- "$cur") )
            fi
            ;;
        list)
            # 补全 --refresh 选项
            if [[ "$cur" == -* ]]; then
                COMPREPLY=( $(compgen -W "--refresh" -- "$cur") )
            fi
            ;;
    esac
    
    return 0
}

complete -F _ghdeb ghdeb
