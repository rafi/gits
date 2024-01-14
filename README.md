# gits

> Git manager for multiple repositories with GitHub/GitLab/Bitbucket support.

![gits animated overview](http://rafi.io/img/project/gits/overview.gif)

Features:

- [x] GitHub/GitLab/Bitbucket/filesystem support
- [x] Configurable by YAML
- [x] Checkout branch for multiple repositories
- [x] Clone multiple repositories
- [x] Fetch and prune from all remotes
- [x] List projects as table/tree/json/name
- [x] Show short status for all repositories

## Usage

Usage: `gits [command] <project>`

Available Commands:

- `browse` —   Browse branches and tags
- `cd` —       Get repository path
- `checkout` — Traverse repositories and optionally checkout branch
- `clone` —    Clone all repositories for specified project(s)
- `fetch` —    Fetch and prune from all remotes
- `help` —     Help about any command
- `list` —     List all projects or their repositories
- `pull` —     Pull repositories
- `status` —   Shows Git repositories short status
- `sync` —     Sync caches
- `version` —  Shows current version

`gits` is configured by a YAML file. See [examples/](#config-examples). `gits`
will look for a config file at `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/.git.yaml`.

You can run `gits` with project names as arguments, or a local path to a
directory containing multiple projects.

Examples:

```bash
gits           # list all commands
gits list      # list all projects
gits list foo  # list all project 'foo' repositories

gits status foo     # show status for project 'foo' repositories
gits status ~/code  # show status for all repositories at path
gits status .       # show status for all repositories at current path
```

## Install

On macOS with Homebrew:

```bash
brew install rafi/tap/gits
```

Or install `gits` with Go:

```bash
go install github.com/rafi/gits
```

## Config

Configuration file must be present at `~/.gits.yaml` or `$XDG_CONFIG_HOME/gits/.git.yaml`.

Structure examples:

```yaml
---

# Github source, note that `path` is optional.
myproject:
  desc: My projects   # Optional
  path: ~/code/github # Optional
  source:             # Optional, default: filesystem
    type: github      # Options: github|gitlab|bitbucket|filesystem
    search:           # Search query
      owner: rafi

# Filesystem source that will be searched recursively.
explore:
  desc: Explore projects
  path: ~/code/explore
  source:
    type: filesystem

# Repositories with explicit directory name and remote source URL.
foobar:
  path: ~/code/myapp
  repos:
  - dir: api                             # Can be absolute or relative to path
    src: https://github.com/app/api.git  # Optional remote clone URL
  - dir: ios
    src: https://github.com/app/ios.git
  - dir: android
    src: https://github.com/app/android.git
```

## Config examples

```yaml
---

mybitbucket:
  source:
    type: bitbucket
    search:
      owner: rafi

mygithub:
  source:
    type: github
    search:
      owner: rafi

work:
  path: ~/code/work
  source:
    type: gitlab
    search:
      groupID: "123456"

explore:
  desc: Exploring projects
  path: ~/code/explore
  source:
    type: filesystem

acme:
  path: ~/code/acme
  desc: Acme is a really cool app.
  repos:
  - dir: admin
  - dir: ant-design-pro
  - dir: api
  - dir: infra
  - dir: ios
  - dir: react-native
  - dir: web
  - dir: webapp

rafi:
  desc: My dotfiles
  repos:
  - dir: ~/.config
    src: git@github.com:rafi/.config.git
  - dir: ~/.config/nvim
    src: git@github.com:rafi/vim-config.git

vim:
  path: ~/code/vim
  desc: Vim plugins
  repos:
  - dir: venom
    src: git@github.com:rafi/vim-venom.git
  - dir: awesome-vim-colorschemes
  - dir: badge
  - dir: vim-sidemenu
```
