#compdef ghdeb
# ghdeb zsh completion
# 安装: 复制到 /usr/share/zsh/site-functions/_ghdeb 或 ~/.zsh/completions/_ghdeb

_ghdeb_get_packages() {
    local state_file="${XDG_STATE_HOME:-$HOME/.local/state}/ghdeb/installed.json"
    if [[ -f "$state_file" ]]; then
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
    local -a commands
    commands=(
        'install:Install or upgrade a package from GitHub Releases'
        'upgrade:Upgrade managed packages'
        'scan:Scan system for GitHub orphan packages'
        'list:List all managed packages'
        'history:Show operation history for a package'
        'remove:Mark a package as removed'
        'set-repo:Set GitHub repository for a package'
        'info:Show latest release information'
        'version:Show ghdeb version'
        'help:Show help message'
    )
    
    _arguments -C \
        '1:command:->command' \
        '*::arg:->args'
    
    case $state in
        command)
            _describe -t commands 'ghdeb command' commands
            ;;
        args)
            case $words[1] in
                install|info)
                    _message 'owner/repo[@tag]'
                    ;;
                upgrade|history|remove)
                    local -a packages
                    packages=(${(f)"$(_ghdeb_get_packages)"})
                    _describe -t packages 'package' packages
                    ;;
                set-repo)
                    if (( CURRENT == 2 )); then
                        local -a packages
                        packages=(${(f)"$(_ghdeb_get_packages)"})
                        _describe -t packages 'package' packages
                    else
                        _message 'owner/repo'
                    fi
                    ;;
                scan)
                    _arguments \
                        '--deep[Fetch Homepage to find GitHub links]'
                    ;;
                list)
                    _arguments \
                        '--refresh[Force refresh version cache]' \
                        '-r[Force refresh version cache]'
                    ;;
            esac
            ;;
    esac
}

_ghdeb "$@"
