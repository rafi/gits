# gits

> Fast CLI Git manager for multiple repositories grouped by projects, with
> GitHub/GitLab/Bitbucket support.

[![tests](https://github.com/rafi/gits/actions/workflows/test.yml/badge.svg)](https://github.com/rafi/gits/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rafi/gits.svg)](https://pkg.go.dev/github.com/rafi/gits)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE.txt)

![gits animated overview](https://rafi.io/img/project/gits/overview.gif)

<!-- vim-markdown-toc GFM -->

- [What is gits?](#what-is-gits)
- [Why gits?](#why-gits)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Usage](#usage)
  - [Addressing projects and repositories](#addressing-projects-and-repositories)
  - [Status](#status)
  - [Adding repositories to a project](#adding-repositories-to-a-project)
  - [Pushing](#pushing)
  - [Running a command everywhere](#running-a-command-everywhere)
  - [Jumping to a repository](#jumping-to-a-repository)
  - [Synchronizing caches](#synchronizing-caches)
  - [Checking your configuration](#checking-your-configuration)
  - [Shell completion](#shell-completion)
- [JSON output](#json-output)
- [Configuration](#configuration)
  - [Filtering repositories](#filtering-repositories)
  - [Skipping a project during clone](#skipping-a-project-during-clone)
  - [Sub-projects](#sub-projects)
  - [Settings](#settings)
  - [Provider tokens](#provider-tokens)
- [Config examples](#config-examples)
- [Development](#development)
- [License](#license)

<!-- vim-markdown-toc -->

## What is gits?

`gits` runs one command across many git repositories at once. Point it at any
directory and it works immediately, no config needed:

```console
$ gits status ~/code
    Repo  Branch  Status  Δ±  Upstream⇅  Version  Commit    Age  Message
  + api   main     !  –   ≠1  ↑3                  c1abd2ef  now  Add feature
    web   main        –                           4f2ba910  2d   Fix layout
  / tools             –                                          not cloned

○ Showing 3 repos, 1 with changes, 1 error
```

Group repositories into **projects** in a config file and address them by name:

```bash
gits status acme     # one-line status for every repository in 'acme'
gits pull acme       # pull them all
gits clone acme      # clone the ones that aren't there yet
```

A project is just a label for a group of repositories. It either lists them by
hand or discovers them from GitHub, GitLab, Bitbucket or a local directory —
cached, so day-to-day commands never wait on the network.

It is a single static Go binary needing nothing but `git`.

## Why gits?

- **One command, many repositories.** Clone, fetch, pull, push, check status, or
  run an arbitrary command across a whole project, in parallel.
- **Your repositories, discovered for you.** Point a project at a GitHub user, a
  GitLab group, a Bitbucket workspace, or a directory, and `gits` finds the
  repositories. Results are cached with a configurable TTL.
- **A status view that fits on a screen.** One line per repository, with branch,
  staged/unstaged/untracked counts, ahead/behind, version and last commit.
- **Scriptable.** `-o json` emits one well-defined document from `list`,
  `status`, and every bulk command, so you can pipe it into `jq`. Results go to
  stdout and progress goes to stderr, so piping is always safe.
- **Interactive when you want it.** Fuzzy-pick projects, repositories, branches
  and tags; `cdgit` jumps your shell straight into a repository.
- **Safe by default.** `gits push` refuses to reach `--force` or `--mirror`, and
  skips branches it should not re-create. `gits doctor` tells you what is wrong
  with your setup before it bites.

## Install

**Homebrew** (macOS and Linux):

```bash
brew install rafi/tap/gits
```

**Go** (1.27 or newer):

```bash
go install github.com/rafi/gits/cmd/gits@latest
```

**Binaries** for Linux and macOS (amd64/arm64) are attached to each
[release](https://github.com/rafi/gits/releases).

### Requirements

| Dependency | Needed for |
| ---------- | ---------- |
| `git` | everything; `gits` drives your own `git` binary |
| [`fzf`](https://github.com/junegunn/fzf) | interactive selection only (`browse`, `checkout`, `cd`, and prompts when you omit a project) |

Run `gits doctor` at any time to see which of these were found and where.

## Quick start

**1. Write a config file** at `~/.gits.yaml` (or
`$XDG_CONFIG_HOME/gits/config.yaml`):

```yaml
---
# ~/.gits.yaml

# Discovered from GitHub.
gh:
  path: ~/code/github
  source:
    type: github
    search: rafi          # user or organization

# Discovered from a local directory, searched recursively.
vim:
  path: ~/code/vim
  desc: Vim plugins

# Listed by hand, with absolute directories.
dotfiles:
  desc: My dotfiles
  repos:
    - dir: ~/.config
      src: git@github.com:rafi/.config.git
    - dir: ~/.config/nvim
      src: git@github.com:rafi/vim-config.git
```

That file is [examples/simple.yaml](./examples/simple.yaml). For every setting
with its default value, see [examples/config.yaml](./examples/config.yaml).

**2. Check that it is sound:**

```bash
gits doctor
```

**3. Use it:**

```bash
gits list              # every project
gits list gh           # every repository in project 'gh'
gits clone gh          # clone the ones not cloned yet
gits status gh         # where does everything stand?
gits pull gh           # bring them all up to date
```

**4. Or point it at a directory**, with no config at all:

```bash
gits status ~/code     # every repository under a path
gits status .          # every repository under the current directory
```

## Commands

Usage: `gits [command] [project] [repo] [flags]`

| Command | What it does |
| ------- | ------------ |
| `add` | Add cloned repositories to a project's list |
| `browse` | Browse branches and tags |
| `cd` | Get repository path (see [`cdgit`](#jumping-to-a-repository)) |
| `checkout` | Traverse repositories and optionally checkout branch |
| `clone` | Clone all repositories |
| `completion` | Generate a shell completion script |
| `doctor` | Report configuration and environment problems |
| `exec` | Run a command in every repository |
| `fetch` | Fetch and prune from all remotes |
| `list` | List project repositories |
| `orphan` | Find repositories on disk that no project declares |
| `pull` | Pull repositories |
| `push` | Push current branch to its upstream |
| `status` | Show Git repositories short status |
| `sync` | Synchronize project caches |
| `version` | Show version |

Global flags:

| Flag | Meaning |
| ---- | ------- |
| `-c`, `--config <path>` | Use a specific config file |
| `-C`, `--color <auto\|always\|never>` | Control colored output |
| `-v`, `--verbose` | Trace what `gits` did: provider pages, cache hits, git's stderr |

Commands that produce a report also take `-o`/`--output`. `list` offers
`table`, `wide`, `tree`, `name` and `json`; `status`, `doctor` and the bulk
commands offer `table` and `json`.

## Usage

### Addressing projects and repositories

Every command takes a project, and optionally one repository inside it:

```bash
gits status acme      # the whole project
gits status acme api  # one repository
gits status acme api/ # the sub-project 'api' — note the trailing slash
gits status ~/code    # a path instead of a project name
gits list acme infra  # several projects at once
```

Omit the project and `gits` asks you to pick one interactively.

### Status

`gits status` prints one line per repository, with branch, change counts,
ahead/behind, version and last commit. Narrow it down with:

```bash
gits status acme --dirty     # only repositories with uncommitted changes
gits status acme --unsynced  # only repositories ahead of or behind upstream
gits status acme --stat      # add a column of uncommitted line counts
```

### Adding repositories to a project

`gits add` writes already-cloned repositories into a project's `repos:` list,
creating the project if needed. Each argument is a directory, a glob, or a clone
URL:

```bash
gits add acme                              # the current directory
gits add acme ./api ../shared/tools        # directories
gits add acme 'backend*'                   # every repository matching a glob
gits add acme git@github.com:acme/api.git  # cloned into the current directory first
```

This is for hand-listed projects. For one backed by a `source`, use
`gits orphan` to find repositories on disk that no project declares.

### Pushing

`gits push` pushes each repository's current branch to its upstream:

```bash
gits push acme          # push every repository in project 'acme'
gits push acme -n       # git's own --dry-run, per repository
gits push acme --tags   # push tags instead of the current branch
```

A branch with no upstream, or one whose upstream is gone, is skipped rather than
failed. Also supported: `--all`, `--branches`, `--follow-tags`, `--atomic` and
`--prune`.

`--force`, `--force-with-lease`, `-u`/`--set-upstream` and `--mirror` are
deliberately unreachable: one mistyped flag across forty remotes is not
recoverable the way it is against one.

### Running a command everywhere

`gits exec` runs one command in every repository. Everything after `--` is the
command:

```bash
gits exec acme -- git gc --quiet     # in every repository of 'acme'
gits exec acme api -- git log -n 1   # in one repository
gits exec acme -- sh -c 'git log -1 --format=%s | tr a-z A-Z'   # pipes need a shell
```

Commands run directly rather than through a shell, each in its repository, with
`GITS_PROJECT`, `GITS_REPO` and `GITS_REPO_PATH` exported.

### Jumping to a repository

`gits cd` prints a repository's path; a child process cannot change your shell's
directory. Source the provided function from your `~/.bashrc` or `~/.zshrc`
(fish users want [`contrib/cdgit.fish`](./contrib/cdgit.fish)):

```bash
source /path/to/gits/contrib/cdgit.sh
```

Then:

```bash
cdgit acme api   # cd straight there
cdgit            # pick a project and repository interactively
```

### Synchronizing caches

`gits sync` drops each project's cached repository list and refetches it from its
code forge:

```console
$ gits sync
[1/2] acme [github:acme] flushed · ~/code/acme · 42 repositories
[2/2] infra [gitlab:mygroup] flushed · ~/work/infra · 17 repositories
Synchronized 2 projects, 59 repositories.
```

Name projects (`gits sync acme infra`) to sync only those. Only forge-backed
projects have a cache to refresh.

### Checking your configuration

`gits doctor` reports what is wrong with your configuration and environment, in
one place:

```console
$ gits doctor
info    config using ~/.gits.yaml
error   config unknown config key "acme.pth" in ~/.gits.yaml, ignored
error   acme.one no `path:` on the project and no `dir:` on the repository, so there is nowhere for it to live
warning vim `path: ~/code/vim` does not exist yet; `gits clone` will create it
info    git git version 2.55.0 (/opt/homebrew/bin/git)
info    finder 0.74.3 (Homebrew) (/opt/homebrew/bin/fzf)
info    cache directory ~/.cache/gits
warning cache.github-acme written by gits v0.10, will be refetched (cached 9d ago)
info    cache.github-rafi cached 1d ago, valid for 168h0m0s
```

It checks config keys, project paths, repositories with no local home, the `git`
and finder binaries, and every cache against `settings.cacheTTL`. It never
contacts a provider, takes `-o json`, and exits non-zero on any error so CI can
gate on it.

Misspelled config keys are reported on every ordinary run too, since YAML
silently ignores them:

```console
$ gits list acme
unknown config key "acme.pth" in ~/.gits.yaml, ignored
```

### Shell completion

`gits completion <bash|zsh|fish|powershell>` prints a completion script; the
sub-command's help says where your shell expects it.

Tab completes project names, repositories, branches, and flag values. Names come
from the cache, so completion never waits on the network.

## Configuration

The configuration file lives at `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/config.yaml`. JSON and TOML are supported too, so
`.json`, `.yml` and `.toml` extensions work at either location. Use
`gits -c <path>` to point at any other file.

See [examples/config.yaml](./examples/config.yaml) for a fully documented config
file listing every setting with its default value, or
[examples/simple.yaml](./examples/simple.yaml) for a minimal one.

> [!WARNING]
> Each project in the config file can have either a `source` or a `repos` key,
> not both.

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
  include: [...]      # Optional allowlist of repositories
  exclude: [...]      # Optional denylist of repositories
  clone: true         # Optional, set false to skip during `gits clone`
  subprojects: [...]  # Optional nested projects

anotherproject:
  ...
```

> [!NOTE]
> When `dir` is omitted, the local directory is derived from the last path
> segment of `src`, with only a trailing `.git` suffix stripped. Earlier
> versions stripped everything after the last dot, so a dotted repository name
> without a `.git` suffix (e.g. `rafi.github.io`) previously mapped to
> `rafi.github` and now maps to `rafi.github.io`. If you cloned such a repo with
> an older version, rename the directory (or set `dir:` explicitly) to match.

### Filtering repositories

`include` and `exclude` narrow the repositories a project contributes, which is
most useful against a `source` you do not control. Both lists match a repository
three ways — its name, its namespace, or `namespace/name` — as exact strings,
not patterns or globs:

```yaml
work:
  path: ~/code/work
  source:
    type: gitlab
    search: "12345678"
  exclude:
    - acme/legacy-api   # namespace/name
    - deprecated-tool   # name
  # include:            # when non-empty, only what it names is kept
  #   - acme/api
  #   - acme/web
```

`exclude` always wins over `include`, and a non-empty `include` is exclusive:
everything it does not name is dropped. Both apply to sub-projects too.

### Skipping a project during clone

`clone: false` passes over a project when `gits clone` runs, along with every
sub-project beneath it. Every other command still sees the project normally:

```yaml
vendor:
  path: ~/code/vendor
  clone: false
```

### Sub-projects

A project can nest others under `subprojects`. Entries are a list rather than a
map, so each one carries its own `name`:

```yaml
org:
  path: ~/code/org
  repos:
    - dir: platform
  subprojects:
    - name: frontend        # path becomes ~/code/org/frontend
      desc: Web clients
      repos:
        - dir: web
          src: git@github.com:myorg/web.git
    - name: backend
      path: ~/code/services # an explicit path wins over the default
      source:               # discovered on its own, like a top-level project
        type: github
        search: myorg-backend
```

A sub-project inherits what it does not declare: a `path` of
`<parent path>/<name>`, and its parent's `source`. A parent with no `path`
passes none down, so a sub-project under one needs absolute `dir` entries or a
`path` of its own. A project that declares nothing but `subprojects` is fine —
it groups them and contributes no repositories itself.

An inherited `source` tells `gits` what kind of origin the sub-project's
repositories have — so a provider-backed one with no local clone is reported as
`remote-only` rather than as an error — but it does not discover anything a
second time. Give a sub-project its own `source` when it should be discovered
separately, or list its repositories under `repos`.

> [!NOTE]
> A project with a `path:` and no `repos:` is searched recursively, and that
> search descends into its sub-projects' directories too. If a sub-project also
> lists those repositories, they appear twice. Give such a parent an explicit
> `repos:` list, or point the sub-projects at directories outside the parent's
> path.

Address a sub-project by giving its name a trailing slash, which is what tells
`gits` you mean a sub-project and not a repository:

```bash
gits status org frontend/   # the sub-project 'frontend'
gits status org web         # the repository 'web'
```

### Settings

Built-in behavior is configured under the reserved `settings:` key:

```yaml
settings:
  cacheTTL: 168h   # How long provider caches stay valid. Go duration
                   # syntax (e.g. "24h", "30m"). Default: 168h (7 days).
                   # Invalid or empty values fall back to the default.
  workerCount: 8   # Concurrent git workers. Default: max(NumCPU, 2).
  verbose: false   # Trace what gits did on the way to an answer — provider
                   # pages, cache hits, git's stderr — to stderr. Same as -v.
  includeArchived: false  # Include archived repositories when listing
                          # from providers (GitHub, GitLab). Default: false.
                          # Bitbucket Cloud has no archived flag, so the
                          # setting does not apply there.
  providerTimeout: 5m     # HTTP timeout for provider API calls. Go
                          # duration syntax. Default: 5m.
  gitTimeout: 5m          # Timeout for network git operations
                          # (clone/fetch/pull). Go duration syntax.
                          # Default: 5m.
  finder:                 # Interactive selector overrides. gits shells out
                          # to fzf; override to use a drop-in replacement or
                          # tune the invocation.
    binary: fzf           #   Executable to run. Must be on PATH.
    # args: [...]         #   Replaces the built-in default options wholesale.
    # extra: [...]        #   Appended after args, adding options.
```

Provider caches are written to `$XDG_CACHE_HOME/gits` (`~/.cache/gits` by
default); `gits doctor` prints the directory in use.

### Provider tokens

Remote providers need an API token. Configure it per provider under `settings:`,
either verbatim or as a command that prints it:

```yaml
settings:
  github:
    tokenCommand: pass tokens/github   # `token-cmd` is also accepted
  gitlab:
    tokenCommand: op read op://private/gitlab/token
  bitbucket:
    tokenCommand: pass tokens/bitbucket
```

For each provider the first of these wins:

1. `token` — used as-is. Keep in mind your config file is plain text.
2. `tokenCommand` (alias `token-cmd`) — run through the shell, so pipes and
   quoting work; the first non-empty output line is the token. It runs at most
   once per command per `gits` invocation, so a passphrase prompt appears once
   even with several projects on the same provider. A failing command is an
   error — there is no silent fallback.
3. Environment: `GITHUB_TOKEN` (or `HOMEBREW_GITHUB_API_TOKEN`),
   `GITLAB_TOKEN`, `BITBUCKET_TOKEN`.

Bitbucket takes an
[Atlassian API token](https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/)
in either of two forms:

```yaml
settings:
  bitbucket:
    token: me@example.com:my-api-token   # Atlassian account email and token
    # token: my-api-token                # the token alone
```

Give it the `read:repository:bitbucket` and `read:workspace:bitbucket` scopes,
which is all discovery reads. A legacy `username:app-password` still works
wherever Bitbucket still honors it, but Atlassian has deprecated app passwords
in favor of API tokens, so prefer a token for anything new.

Cached projects don't need a token until the cache expires or `gits sync`
refreshes it.

## Config examples

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

## Development

`gits` is written in Go and needs no code generation or vendored tools to
build:

```bash
git clone https://github.com/rafi/gits.git && cd gits
go build ./cmd/gits
go test ./...
```

A [`justfile`](./justfile) wraps the common tasks ([just](https://just.systems)
required):

```bash
just build     # build the binary into bin/release
just test      # run the test suite
just lint      # golangci-lint
just release   # cross-compile for linux/darwin, amd64/arm64
```

Contributions are welcome — please open an issue or pull request on
[GitHub](https://github.com/rafi/gits).

## License

MIT © 2018-2026 Rafael Bodill. See [LICENSE.txt](./LICENSE.txt).
