# Gits [![Build Status](https://travis-ci.org/rafi/gits.svg?branch=master)](https://travis-ci.org/rafi/gits)

> A Fast CLI Git manager for multiple repositories

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

* `checkout` -  Traverse repositories and optionally checkout branch
* `fetch` -     Fetches and prunes from all remotes
* `help` -      Help about any command
* `status` -    Shows Git repositories short status

## Config

Configuration file must be present at `~/.gits.yaml`, here is an example:

```yaml
---

projects:
  acme:
    path: ~/code/python/acme
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
    repos:
    - dir: ~/.config
    - dir: ~/.config/nvim

  vim:
    path: ~/code/vim
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
