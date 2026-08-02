# ghdeb — Install & Upgrade .deb Packages from GitHub Releases

[![Release](https://img.shields.io/github/v/release/LeisureLinux/ghdeb)](https://github.com/LeisureLinux/ghdeb/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**ghdeb** is a lightweight CLI tool that manages `.deb` packages downloaded from GitHub Releases — one command, no PPA, no manual download. It features a built-in catalog of 50+ popular packages with short-name installs.

```bash
ghdeb install LeisureLinux/ghdeb      # install latest ghdeb
ghdeb upgrade                         # upgrade all managed packages
ghdeb list                            # list installed packages
```

## What problem does ghdeb solve?

Many excellent CLI tools — **bat**, **fd**, **ripgrep**, **gh**, **rustdesk**, **localsend** — distribute `.deb` packages via GitHub Releases, but the install workflow is painful:

1. Open the GitHub releases page
2. Find the correct `.deb` file for your architecture
3. Copy the download URL, `wget` it
4. `sudo dpkg -i` to install
5. Repeat all of the above for every upgrade

**ghdeb reduces this to a single command.**

## How to install ghdeb?

```bash
# Download and install the latest release
ghdeb install LeisureLinux/ghdeb

# Or install a specific version
ghdeb install LeisureLinux/ghdeb@v0.6.0

# Or build from source
git clone https://github.com/LeisureLinux/ghdeb.git
cd ghdeb && make build && sudo make install
```

## How to use the package catalog?

ghdeb includes a curated catalog of 50+ popular packages. You can install them by short name:

```bash
# Install by short name (mapped via catalog)
ghdeb install bat                     # → sharkdp/bat
ghdeb install fd                      # → sharkdp/fd
ghdeb install ripgrep                 # → BurntSushi/ripgrep
ghdeb install gh                      # → cli/cli

# Search the catalog
ghdeb search monitor                  # regex search by name/summary
ghdeb catalog list                    # list all catalog entries
ghdeb catalog show bat                # show entry details
```

### Adding custom packages to the catalog

```bash
# Add a GitHub-hosted package
ghdeb catalog add myapp --repo user/myapp --summary "My awesome app"

# Add a non-GitHub source (direct .deb URL)
ghdeb catalog add myapp --url "https://example.com/myapp_{version}_{arch}.deb"

# Remove from user catalog
ghdeb catalog delete myapp
```

User catalog entries are stored in `~/.config/ghdeb/catalog.toml` and override system entries.

## How to manage packages?

```bash
# Reinstall a package
ghdeb reinstall bat

# Uninstall and purge config
ghdeb purge rustdesk

# Clean downloaded .deb cache
ghdeb clean
ghdeb clean --dry-run                 # preview only
```

## How to install any .deb package from GitHub?

```bash
# Install the latest release of any GitHub project that provides .deb assets
ghdeb install cli/cli               # GitHub CLI
ghdeb install rustdesk/rustdesk     # RustDesk remote desktop
ghdeb install localsend/localsend   # LocalSend file sharing
ghdeb install jgraph/drawio         # draw.io diagrams

# Install a specific version
ghdeb install LeisureLinux/ghdeb@v0.6.0
```

ghdeb automatically detects your system architecture (`dpkg --print-architecture`) and selects the matching `.deb` asset from the release.

## How to upgrade all GitHub-sourced packages?

```bash
# Upgrade everything ghdeb manages
ghdeb upgrade

# Upgrade a specific package
ghdeb upgrade LeisureLinux/ghdeb

# Upgrade by package name
ghdeb upgrade rustdesk
```

## How to find unmanaged GitHub .deb packages on my system?

```bash
# Scan for orphan packages (installed .deb with no apt source)
ghdeb scan

# Deep scan: also fetch Homepage URLs to find GitHub links
ghdeb scan --deep
```

ghdeb discovers `.deb` packages installed on your system that have no corresponding apt repository — these are "orphan packages" that won't receive updates through `apt upgrade`.

## How to view package information?

```bash
# Show latest release info (without installing)
ghdeb info LeisureLinux/ghdeb

# List all managed packages with versions
ghdeb list

# View operation history for a package
ghdeb history LeisureLinux/ghdeb
```

## How does architecture matching work?

ghdeb intelligently matches release assets to your system architecture:

| System Arch | Matches Filename Keywords |
|-------------|---------------------------|
| amd64       | amd64, x86_64, x86-64, x64 |
| arm64       | arm64, aarch64            |
| armhf       | armhf, armv7l, armv7      |
| i386        | i386, x86, i686, 386      |

It prefers standard packages and skips musl/static/portable variants.

## How to configure a proxy for GitHub downloads?

```bash
# Via environment variable
export https_proxy=http://your-proxy:8080
ghdeb install LeisureLinux/ghdeb

# Or via config file (~/.config/ghdeb/config.json)
echo '{"proxy": "http://your-proxy:8080"}' > ~/.config/ghdeb/config.json
```

ghdeb also supports download retry (3 attempts with exponential backoff) and resume downloads.

## How to set up shell autocompletion?

```bash
# zsh — system-wide
sudo cp /usr/share/ghdeb/completion/ghdeb.zsh /usr/share/zsh/site-functions/_ghdeb

# bash — system-wide
sudo cp /usr/share/ghdeb/completion/ghdeb.bash /etc/bash_completion.d/ghdeb
```

## How does ghdeb compare to other tools?

| Tool | Approach | Limitation |
|------|----------|------------|
| **ghdeb** | Client-side CLI, zero config | — |
| gitdeb | Shell script | No stars, no community validation |
| debian-package-installer | Python + JSON config | Requires configuration files |
| github-apt-repos (★110) | Server-side APT repo builder | Overkill for personal use |
| inapt (★8) | Server-side APT proxy | Requires infrastructure |
| apt-transport-github (★13) | APT transport | Marked "NOT YET READY", unmaintained |

**ghdeb fills the gap for a lightweight, zero-config client-side CLI tool.**

## Environment Variables

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` / `GH_TOKEN` | GitHub PAT — raises API rate limit (60→5000 req/hr) |
| `https_proxy` / `http_proxy` | Proxy for GitHub downloads |
| `XDG_CACHE_HOME` | Override download cache directory |
| `XDG_STATE_HOME` | Override state file directory |

**Note:** If neither `GITHUB_TOKEN` nor `GH_TOKEN` is set, ghdeb will automatically try to get the token from `gh auth token` (GitHub CLI). This helps avoid API rate limiting when you have gh CLI installed and authenticated.

## Data Storage (XDG-compliant)

| Path | Purpose |
|------|---------|
| `/usr/share/ghdeb/catalog.toml` | System package catalog |
| `~/.config/ghdeb/catalog.toml` | User package catalog (overrides system) |
| `~/.cache/ghdeb/` | Downloaded .deb file cache |
| `~/.local/state/ghdeb/installed.json` | Package state tracking |
| `~/.config/ghdeb/config.json` | Configuration (proxy, etc.) |

## FAQ

### Does ghdeb replace apt?
No. ghdeb manages packages that are **not** in any apt repository — typically `.deb` files distributed via GitHub Releases. Regular system packages should still be managed with `apt`.

### What happens if I remove a package with `apt remove`?
ghdeb detects the actual installation state via `dpkg-query`. Running `ghdeb upgrade` will automatically reinstall packages that were removed but are still tracked.

### Does ghdeb support non-.deb assets?
ghdeb focuses on `.deb` packages. For other asset types (AppImage, tarball), use the GitHub release page directly.

### Is ghdeb safe to use?
ghdeb downloads `.deb` files from official GitHub Releases and installs them with `dpkg -i`. It sets `DEBIAN_FRONTEND=noninteractive` to avoid interactive prompts. Always review packages from third-party repositories.

## License

MIT
