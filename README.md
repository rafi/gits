# Gits [![Build Status](https://travis-ci.org/rafi/gits.svg?branch=master)](https://travis-ci.org/rafi/gits)

> A Fast CLI Git manager for multiple repositories

![gits animated overview](http://rafi.io/static/img/project/gits/overview.gif)

## Install

Requirements: [Git](https://git-scm.com/)

```bash
go get github.com/rafi/gits
```

Or on macOS:

```bash
brew install rafi/tap/gits --HEAD
```

## Usage

Usage: `gits [command] <project>`

Available Commands:

* `checkout` - Traverse repositories and optionally checkout branch
* `clone` -    Clones all repositories for specified project(s)
* `fetch` -    Fetches and prunes from all remotes
* `help` -     Help about any command
* `status` -   Shows Git repositories short status
* `version` -  Shows gits current version

## Config

Configuration file must be present at `~/.gits.yaml`

### Definitions

#### Root

Name | Description |
-----|-------------|
`projects` | Collection of projects, key/value.

Example:

```yaml
projects:
  acme: {}
  myapp: {}
  myotherapp: {}
  secretproject: {}
```

#### Project

Name | Description |
-----|-------------|
`repos` | List of projects
`path` | _Optional_. Base path for all project repositories.
`desc` | _Optional_. Description to display when running commands.

Example:

```yaml
projects:
  acme:
    repos: []
  myapp:
    path: ~/code/myapp
    repos: []
```

#### Project Repositories

Name | Description |
-----|-------------|
`dir` | Directory where repository should/is cloned. Can be absolute or relative to project base path if specified.
`src` | _Optional_. Remote address to clone from.

```yaml
projects:
  acme:
    repos:
      - dir: ~/code/acme/api
      - dir: ~/code/acme/web
  myapp:
    path: ~/code/myapp
    repos:
      - dir: api
        src: https://github.com/myapp/api.git
      - dir: ios
        src: https://github.com/myapp/ios.git
      - dir: android
        src: https://github.com/myapp/android.git
```

### Example Config

```yaml
---

projects:
  acme:
    path: ~/code/python/acme
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
    - dir: awesome-vim-colorschemes
    - dir: badge
    - dir: denite-mpc
    - dir: denite-session
    - dir: denite-task
    - dir: denite-z
    - dir: unite-issue
    - dir: vim-sidemenu
```
