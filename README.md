# gits

> Git manager for multiple repositories with GitHub/GitLab/Bitbucket support.

![gits animated overview](http://rafi.io/img/project/gits/overview.gif)

<!-- vim-markdown-toc GFM -->

- [Features](#features)
- [Install](#install)
- [Usage](#usage)
- [Configuration](#configuration)
- [Config Examples](#config-examples)

<!-- vim-markdown-toc -->

## Features

Use as a git clone manager, and while developing on multiple git repositories.

- [x] GitHub/GitLab/Bitbucket/filesystem support with cache
- [x] Interactive browsing of projects/repositories/branches/tags
- [x] Clone/fetch/pull for multiple repositories
- [x] Show one-line status with icons for all repositories
- [x] List projects as table/tree/json/name
- [x] Checkout branches interactively
- [x] Configurable by YAML/JSON/TOML

## Install

On macOS with Homebrew:

```bash
brew install rafi/tap/gits
```

Or install `gits` with Go:

```bash
go install github.com/rafi/gits
```

## Usage

Usage: `gits [command] <project>`

Available Commands:

- `add` —      Add repository to a project
- `browse` —   Browse branches and tags
- `cd` —       Get repository path
- `checkout` — Traverse repositories and optionally checkout branch
- `clone` —    Clone all repositories for specified project(s)
- `fetch` —    Fetch and prune from all remotes
- `help` —     Help about any command
- `list` —     List all projects or their repositories
- `orphan` —   Finds orphan repository
- `pull` —     Pull repositories
- `status` —   Shows Git repositories short status
- `sync` —     Synchronize project caches
- `version` —  Shows current version

`gits` is configured by a YAML file. See [examples](#config-examples). `gits`
will look for a config file at `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/.git.yaml`.

You can run `gits` with project names as arguments, or a local path to a
directory containing multiple projects.

Examples:

```bash
gits                # list all commands
gits list           # list all projects
gits list acme      # list all project 'acme' repositories

gits status acme    # show status for project 'acme' repositories
gits status ~/code  # show status for all repositories at path
gits status .       # show status for all repositories at current path
```

To use `gits cd` — source [./contrib/cdgit.sh](./contrib/cdgit.sh) in your shell
`~/.bashrc` or `~/.zshrc`, and use `cdgit` to navigate to a repository.

## Configuration

Configuration file must be present at `~/.gits.yaml` or `$XDG_CONFIG_HOME/gits/.gits.yaml`.

> [!WARNING]
> Each project in config file can either have a `source` or `repos` key, not both.

The structure of the config file is as follows:

```yaml
---
# ~/.gits.yaml

# Project definition
projectname:          # Project name
  desc: My projects   # Optional
  path: ~/code/github # Optional if 'repos' are specified and have absolute paths.
  source:             # Required if no 'repos' defined, default: filesystem
    type: github      # Required: github|gitlab|bitbucket|filesystem
    search: rafi      # Required search query (organization, user name, group id)
  repos:              # Required if no 'source' defined
    - dir: foo        # Optional, default: repository name
      src: git@...    # Optional, default: repository remote URL
    - ...

anotherproject:
  ...
```

## Config Examples

Each project in the following example is defined differently:

```yaml
---
# ~/.gits.yaml

# Github source.
mygithub:
  source:
    type: github
    search: rafi

# GitLab source, note that `path` and `desc` are optional.
work:
  path: ~/code/work
  desc: My work GitLab projects
  source:
    type: gitlab
    search: "12345678"  # Make sure GitLab group id is quoted

# Bitbucket source.
mybitbucket:
  source:
    type: bitbucket
    search: rafi

# Filesystem source that will be searched recursively.
explore:
  desc: Exploring projects
  path: ~/code/explore
  source:
    type: filesystem

# Relative directory name and implicit remote source URL.
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

# Relative directory name and explicit remote source URL.
myapp:
  path: ~/code/myapp
  repos:
  - dir: api                             # Can be absolute or relative to path
    src: https://github.com/app/api.git  # Optional remote clone URL
  - dir: ios
    src: https://github.com/app/ios.git
  - dir: android
    src: https://github.com/app/android.git

# Absolute directories and explicit remote source URL. (No project path)
rafi:
  desc: My dotfiles
  repos:
  - dir: ~/.config
    src: git@github.com:rafi/.config.git
  - dir: ~/.config/nvim
    src: git@github.com:rafi/vim-config.git
```
