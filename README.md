# gits

> Git manager for multiple repositories with GitHub/GitLab/Bitbucket support.

![gits animated overview](http://rafi.io/img/project/gits/overview.gif)

<!-- vim-markdown-toc GFM -->

- [Features](#features)
- [Concepts](#concepts)
- [Install](#install)
- [Usage](#usage)
- [Configuration](#configuration)
- [Config Examples](#config-examples)

<!-- vim-markdown-toc -->

## Features

Use as a git clone manager, and while developing on multiple git repositories.

- [x] GitHub/GitLab/Bitbucket/filesystem support with cache
- [x] Interactive browsing of gits projects/repositories/branches/tags
- [x] Clone/fetch/pull for multiple repositories
- [x] Show one-line status with icons for all repositories
- [x] List gits projects as table/tree/json/name
- [x] Checkout branches interactively
- [x] Configurable by YAML/JSON/TOML

## Concepts

A **gits project** is a named group of Git repositories managed together by
`gits`. It is not itself a Git repository or a software build project. Before
using commands such as `list`, `status`, or `pull`, decide how you want to group
your repositories into one or more gits projects.

For example, a gits project named `backend` could contain every backend service
repository, allowing `gits status backend` to check them together.

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

Usage: `gits [command] <gits-project>`

Available Commands:

- `add` —      Add repositories to a gits project
- `browse` —   Browse branches and tags
- `cd` —       Get repository path
- `checkout` — Traverse repositories and optionally checkout branch
- `clone` —    Clone all repositories for specified gits project(s)
- `fetch` —    Fetch and prune from all remotes
- `help` —     Help about any command
- `list` —     List all gits projects or their repositories
- `orphan` —   Finds orphan repository
- `pull` —     Pull repositories
- `status` —   Shows Git repositories short status
- `sync` —     Synchronize gits project caches
- `version` —  Shows current version

`gits` stores gits project definitions in a configuration file. See
[examples](#config-examples). It looks for `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/config.yaml`.

You can pass configured gits project names to commands. You can also pass a
local path to discover the Git repositories beneath it dynamically, without
creating a permanent gits project.

Examples:

```bash
gits                # list all commands
gits list           # list all gits projects
gits list acme      # list repositories in the 'acme' gits project

gits status acme    # show status for repositories in the 'acme' gits project
gits status ~/code  # show status for all repositories at path
gits status .       # show status for all repositories at current path
```

The `add` command can add the current repository, clone and add a remote
repository, or add multiple local repositories matched by a glob:

```bash
gits add acme
gits add acme https://github.com/acme/api.git
gits add acme 'backend*'
```

Quote glob patterns so `gits` can expand them consistently. Matching paths that
are not Git repositories are ignored, and repositories already mapped to the
gits project are skipped. If no config file exists, `gits add` creates
`~/.gits.yaml` automatically.

To use `gits cd` — source [./contrib/cdgit.sh](./contrib/cdgit.sh) in your shell
`~/.bashrc` or `~/.zshrc`, and use `cdgit` to navigate to a repository.

## Configuration

Configuration is loaded from `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/config.yaml`. If neither exists, `gits add`
automatically creates `~/.gits.yaml`.

> [!WARNING]
> Each gits project in the config file can have either a `source` or `repos`
> key, not both.

The structure of the config file is as follows:

```yaml
---
# ~/.gits.yaml

# Gits project definition
projectname:          # Gits project name
  desc: My repositories # Optional
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

Each gits project in the following example is defined differently:

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

# Absolute directories and explicit remote source URL. (No gits project path)
rafi:
  desc: My dotfiles
  repos:
  - dir: ~/.config
    src: git@github.com:rafi/.config.git
  - dir: ~/.config/nvim
    src: git@github.com:rafi/vim-config.git
```
