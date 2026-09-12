# gits

> Git manager for multiple repositories with GitHub/GitLab/Bitbucket support.

![gits animated overview](http://rafi.io/img/project/gits/overview.gif)

<!-- vim-markdown-toc GFM -->

- [Features](#features)
- [Install](#install)
- [Upgrading](#upgrading)
- [Usage](#usage)
- [Configuration](#configuration)
- [Config Examples](#config-examples)

<!-- vim-markdown-toc -->

## Features

Use as a git clone manager, and while developing on multiple git repositories.

- [x] GitHub/GitLab/Bitbucket/filesystem support with cache
- [x] Interactive browsing of projects/repositories/branches/tags
- [x] Clone/fetch/pull/push for multiple repositories
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
go install github.com/rafi/gits/cmd/gits@latest
```

`gits` shells out to `git` for repository state. The `Version` column of
`gits status` reads a tag description that needs git ≥ 2.32 (2021); an older
git leaves that one column blank and everything else works.

## Upgrading

Changes that need action on an existing setup. Anything not listed here
upgrades in place.

### v1.0.0 — derived directory names keep their extension

A repository that declares no `dir:` gets its directory name from the basename
of its source URL. That name used to have any file extension stripped; only a
literal `.git` suffix is stripped now. The two rules agree for ordinary names —
`git@github.com:rafi/gits.git` still lives in `gits` — and differ only for a
repository whose name contains a dot:

| Source URL                  | Old directory | New directory    |
| --------------------------- | ------------- | ---------------- |
| `…/rafi/rafi.github.io.git` | `rafi.github` | `rafi.github.io` |
| `…/vercel/next.js.git`      | `next`        | `next.js`        |

The new name is the correct one; the old rule truncated any repository whose
name ended in a dotted segment.

A clone made by an earlier version sits under the old, truncated name. `gits`
now looks for the new one, finds nothing, and reports the repository as
`not-cloned` — so `gits clone` would place a second copy beside the one you
already have. Either rename the directory to match:

```bash
mv ~/code/github/rafi.github ~/code/github/rafi.github.io
```

Or keep it where it is by naming it explicitly, which opts that repository out
of the derivation entirely:

```yaml
repos:
  - src: git@github.com:rafi/rafi.github.io.git
    dir: rafi.github
```

### v1.0.0 — `pull` and `push` skip an unusable upstream instead of failing

A branch whose upstream is gone — still configured, but with no remote branch
behind it, the ordinary end of a branch that was merged and cleaned up — used
to fail both commands, putting git's raw `ambiguous argument '@{upstream}'`
fatal on the repository's line and a non-zero exit code on the run. `gits
pull` also failed, more quietly, on a branch with no upstream at all, the case
`gits push` already passed over. Both commands now report such a repository as
skipped, naming the upstream that went away, and leave the exit code alone.

Nothing that passes today starts failing; the change only ever turns a failure
into a non-failure. What needs action is a script that asserts one of those
failures — a `gits pull acme || …` guard, or a CI step relying on the non-zero
exit to catch a branch whose upstream was deleted — which stops firing. Ask
for the state directly instead: `gits status acme` marks a gone upstream with
`⊘`, and `gits status -o json acme` reports it under the repository's
`upstream` object as `"tracked": false`.

### v1.0.0 — an unknown project name exits non-zero

Naming a project that does not exist — a typo, or one that has been removed
from the config — used to print a warning and exit 0. It is now a real
failure: `gits status typo`, `gits pull typo`, `gits status -o json typo` and
every other command exit non-zero and name the project on stderr.

What needs action is a script that relied on the old exit 0 for a missing
project. Cancelling an interactive project prompt is still not a failure, and
per-repository conditions still follow the existing rules — the JSON form of a
Bulk Command still exits zero when only a repository, not the project itself,
is at fault.

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
- `push` —     Push current branch to its upstream
- `status` —   Shows Git repositories short status
- `sync` —     Synchronize project caches
- `version` —  Shows current version

`gits` is configured by a YAML file. See [examples](#config-examples). `gits`
will look for a config file at `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/config.yaml`.

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

### Pushing

`gits push` pushes each repository's current branch to its upstream, and skips
any repository it cannot push: one whose current branch has no upstream, and
one whose upstream is gone — still configured, but with no remote branch
behind it, the ordinary end of a branch that was merged and cleaned up.
Pushing that second one would succeed and re-create the branch someone
deliberately deleted, so it is passed over with the upstream named. Both are
skips, not failures, so the run still exits zero.

```bash
gits push acme          # push every repository in project 'acme'
gits push acme -n       # git's own --dry-run, per repository
gits push acme --tags   # push tags instead of the current branch
```

It passes a chosen subset of `git push` flags through: `--all`, `--branches`
and `--tags` (mutually exclusive, rejected up front), `--follow-tags`,
`--atomic`, `--prune`, and `-n`/`--dry-run`. Under `--all`, `--branches` or
`--tags` the upstream skip is suspended, since those flags say for themselves
which refs to push.

There is deliberately no way to reach `--force`, `--force-with-lease`,
`-u`/`--set-upstream` or `--mirror`: one mistyped flag applied to forty remotes
is not recoverable the way one against a single remote is. See
[ADR-0002](./docs/adr/0002-push-safety-model.md).

### JSON output

`list` and `status` both take `-o json` and emit the same document: an object
keyed by project name, carrying the project tree and every repository's
identity and state. `list -o json` stops there; `status -o json` additionally
nests the working-tree data it gathered under each repository.

```bash
gits list -o json                      # the project tree, no git commands run
gits status -o json acme               # the same tree, plus work-tree data
gits status -o json --dirty acme       # only repositories with local changes
gits status -o json --stat acme        # adds head.added / head.deleted
```

Each repository reports one of five `state` values:

| State         | Meaning                                                |
| ------------- | ------------------------------------------------------ |
| `ok`          | a local clone exists and is readable                    |
| `not-cloned`  | a local path is known, and nothing is there             |
| `remote-only` | provider-backed, and the config gives it no local home  |
| `error`       | the config is defective, or the path is not readable    |
| `unknown`     | not yet classified                                      |

An `error` repository also carries a `reason`. These strings are a stable
contract — display icons and labels are derived from them, never the reverse.

The nested `status` object appears only where git was actually consulted, so
its presence is what tells you the numbers were measured:

```jsonc
{
  "acme": {
    "id": "", "name": "acme", "path": "~/code/acme",
    "repos": [
      {
        "name": "api", "src": "git@github.com:acme/api.git", "state": "ok",
        "status": {
          "branch": "main",
          "staged": 1, "unstaged": 0, "untracked": 2,
          "ahead": 3, "behind": 0,
          "compared": true,             // false: nothing to compare against
          "upstream": { "name": "origin/main", "tracked": true },
          "version": "v1.2.3",          // git describe, when there is a tag
          "head": { "added": 27, "deleted": 8 },   // --stat only
          "commit": { "hash": "abc1234", "subject": "Add feature",
                      "time": "2026-08-23T12:00:00Z" }
        }
      },
      { "name": "web", "state": "not-cloned" }
    ],
    "subprojects": []
  }
}
```

A repository whose work-tree probe failed carries `"status": {"error": "…"}`
and nothing else — a failed probe measured no counts, and zeroes beside the
error would read as a clean work tree. `status -o json` exits zero for these
per-repository conditions; they are data in this format. The table format is
unchanged, and still exits non-zero.

## Configuration

Configuration file must be present at `~/.gits.yaml` or
`$XDG_CONFIG_HOME/gits/config.yaml`. JSON and TOML are supported too, so
`.json`, `.yml` and `.toml` extensions work at either location. Use
`gits -c <path>` to point at any other file.

See [examples/config.yaml](./examples/config.yaml) for a fully documented
config file listing every setting with its default value, or
[examples/simple.yaml](./examples/simple.yaml) for a minimal one.

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

> [!NOTE]
> When `dir` is omitted, the local directory is derived from the last path
> segment of `src`, with only a trailing `.git` suffix stripped. Earlier
> versions stripped everything after the last dot, so a dotted repository
> name without a `.git` suffix (e.g. `rafi.github.io`) previously mapped to
> `rafi.github` and now maps to `rafi.github.io`. If you cloned such a repo
> with an older version, rename the directory (or set `dir:` explicitly) to
> match.

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

#### Provider tokens

Remote providers need an API token. Configure it per provider under
`settings:`, either verbatim or as a command that prints it:

```yaml
settings:
  github:
    tokenCommand: pass tokens/github   # `token-cmd` is also accepted
  gitlab:
    tokenCommand: op read op://private/gitlab/token
  bitbucket:
    token: my-user:my-app-password     # username:app-password
```

For each provider the first of these wins:

1. `token` — used as-is. Keep in mind your config file is plain text.
2. `tokenCommand` (alias `token-cmd`) — run through the shell, so pipes and
   quoting work; the first non-empty output line is the token. It runs at
   most once per command per `gits` invocation, so a passphrase prompt
   appears once even with several projects on the same provider. A failing
   command is an error — there is no silent fallback.
3. Environment: `GITHUB_TOKEN` (or `HOMEBREW_GITHUB_API_TOKEN`),
   `GITLAB_TOKEN`, `BITBUCKET_TOKEN`.

Cached projects (see `cache`) don't need a token until the cache expires or
`gits sync` refreshes it.

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
