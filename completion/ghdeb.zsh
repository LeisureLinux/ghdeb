#compdef ghdeb
# ghdeb zsh completion
# 安装: 复制到 /usr/share/zsh/site-functions/_ghdeb 或 ~/.zsh/completions/_ghdeb

_ghdeb_get_packages() {
    local state_file="/var/cache/ghdeb/installed.json"
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

_ghdeb_get_catalog_names() {
    local catalog_file="/etc/ghdeb/catalog.toml"
    if [[ -f "$catalog_file" ]]; then
        grep '^\[' "$catalog_file" | sed 's/^\[//;s/\]$//'
    fi
}

_ghdeb() {
    local -a commands
    commands=(
        'install:Install or upgrade a package'
        'update:Refresh version info to local snapshot'
        'upgrade:Upgrade managed packages'
        'reinstall:Force reinstall a package'
        'scan:Scan system for GitHub orphan packages'
        'search:Search in package catalog'
        'list:List managed packages'
        'catalog:Manage package catalog'
        'show:Show package details'
        'info:Alias for show'
        'history:Show operation history'
        'purge:Uninstall and purge config'
        'clean:Clean .deb cache'
        'test-homepage:Test homepage for GitHub links'
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
                install|show|info)
                    local -a names packages
                    names=(${(f)"$(_ghdeb_get_catalog_names)"})
                    packages=(${(f)"$(_ghdeb_get_packages)"})
                    _describe -t names 'package' names packages
                    ;;
                upgrade|reinstall|history|purge)
                    local -a packages
                    packages=(${(f)"$(_ghdeb_get_packages)"})
                    _describe -t packages 'package' packages
                    ;;
                scan)
                    _arguments '--deep[Fetch Homepage to find GitHub links]'
                    ;;
                list)
                    _arguments \
                        '--refresh[Force refresh version cache]' \
                        '-r[Force refresh version cache]' \
                        '--json[Output as JSON]'
                    ;;
                clean)
                    _arguments '--dry-run[Preview without deleting]'
                    ;;
                catalog)
                    if (( CURRENT == 2 )); then
                        local -a subcmds
                        subcmds=('list:List all entries' 'show:Show entry details' 'search:Search catalog' 'add:Add entry' 'modify:Modify entry repo' 'delete:Delete entry' 'validate:Validate entries')
                        _describe -t subcmds 'catalog subcommand' subcmds
                    else
                        case $words[2] in
                            show|delete|validate|modify)
                                local -a names
                                names=(${(f)"$(_ghdeb_get_catalog_names)"})
                                _describe -t names 'name' names
                                ;;
                            modify)
                                if (( CURRENT > 3 )); then
                                    _arguments '--repo[GitHub repository (owner/repo)]:'
                                fi
                                ;;
                            validate)
                                if (( CURRENT == 3 )); then
                                    _arguments '--all[Validate all entries]'
                                fi
                                ;;
                            add)
                                if (( CURRENT == 3 )); then
                                    _message 'name'
                                else
                                    _arguments \
                                        '--repo[GitHub repository (owner/repo)]:' \
                                        '--url[Direct .deb URL template]:' \
                                        '--pretty-name[Display name]:' \
                                        '--website[Website URL]:' \
                                        '--summary[Package summary]:' \
                                        '--gpg-key[GPG public key URL]:'
                                fi
                                ;;
                        esac
                    fi
                    ;;
            esac
            ;;
    esac
}

_ghdeb "$@"
