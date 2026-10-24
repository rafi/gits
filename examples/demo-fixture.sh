#!/usr/bin/env bash
# Build a throwaway sandbox for the VHS demo: fake repositories, fake remotes
# and a config file, all under one directory.
#
# Everything the demo touches lives under $1 - the config file, the git
# "remotes", the clones, the cache and even $HOME. Nothing outside it is read
# or written, so recording a demo can never reach a real repository or a real
# ~/.gits.yaml.
#
# The remotes are bare repositories under $1/remotes, addressed as
# `git@github.com:...` URLs that a sandbox-local `url.insteadOf` rewrites back
# to them. The config file and every `origin` therefore read like the real
# thing while nothing leaves the sandbox.
#
# Usage: demo-fixture.sh <sandbox-dir> <gits-binary>

set -euo pipefail

usage="usage: demo-fixture.sh <sandbox-dir> <gits-binary>"
SANDBOX=${1:?$usage}
GITS_BIN=${2:?$usage}

mkdir -p "$SANDBOX"
SANDBOX=$(cd "$SANDBOX" && pwd)
GITS_BIN=$(cd "$(dirname "$GITS_BIN")" && pwd)/$(basename "$GITS_BIN")

HOME_DIR="$SANDBOX/home"
REMOTES="$SANDBOX/remotes"
SEED="$SANDBOX/seed"
CODE="$HOME_DIR/code"

rm -rf "$HOME_DIR" "$REMOTES" "$SEED" "${SANDBOX:?}/bin"
mkdir -p "$HOME_DIR" "$SANDBOX/bin"
ln -s "$GITS_BIN" "$SANDBOX/bin/gits"

# Isolate every git and gits lookup inside the sandbox.
export HOME="$HOME_DIR"
export XDG_CONFIG_HOME="$HOME_DIR/.config"
export XDG_CACHE_HOME="$HOME_DIR/.cache"
export GIT_CONFIG_GLOBAL="$HOME_DIR/.gitconfig"
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_TERMINAL_PROMPT=0
export TZ=UTC

# Seeding rewrites every `git@github.com:` URL to the sandbox's bare
# repositories, so the whole fixture is built without a network. The rewrite is
# narrowed afterwards, by write_git_config, to only the namespaces the demo
# itself fetches from — see the note there.
write_git_config() {
	cat >"$GIT_CONFIG_GLOBAL" <<EOF
[user]
	name = Ada Lovelace
	email = ada@example.com
[init]
	defaultBranch = main
[advice]
	detachedHead = false
[push]
	default = current
EOF
	local ns
	for ns in "$@"; do
		printf '[url "%s"]\n\tinsteadOf = git@github.com:%s\n' \
			"$REMOTES/$ns" "$ns" >>"$GIT_CONFIG_GLOBAL"
	done
}

# While seeding: every namespace, so nothing reaches a real forge.
write_git_config ''

# Commit timestamps are a fixed number of days behind "now", so the Age column
# reads the same whenever the tape is recorded.
now=$(date +%s)

# days_ago <days> - a git date, "@<epoch>", that many days back.
days_ago() { echo "@$((now - 86400 * $1))"; }

# commit <dir> <message> [days-ago]
commit() {
	GIT_AUTHOR_DATE=$(days_ago "${3:-0}") GIT_COMMITTER_DATE=$(days_ago "${3:-0}") \
		git -C "$1" commit --quiet --allow-empty -m "$2"
}

# write <dir> <file> <content>
write() { printf '%s\n' "$3" >"$1/$2"; }

# new_remote <namespace/name> - a bare repository standing in for a forge, plus
# a seed worktree to push history into. Echoes the seed worktree path.
new_remote() {
	local work="$SEED/$1"
	git init --quiet --bare "$REMOTES/$1.git"
	git init --quiet -b main "$work"
	write "$work" README.md "# $(basename "$1")"
	git -C "$work" add -A
	commit "$work" "Initial commit" 30
	git -C "$work" remote add origin "git@github.com:$1.git"
	git -C "$work" push --quiet -u origin main
	echo "$work"
}

# publish <seed> <message> <days-ago> [tag] - stage everything in the seed
# worktree, commit it, optionally tag it, and push to the remote.
publish() {
	git -C "$1" add -A
	commit "$1" "$2" "$3"
	[ -z "${4:-}" ] || GIT_COMMITTER_DATE=$(days_ago "$3") git -C "$1" tag -a "$4" -m "$4"
	git -C "$1" push --quiet --follow-tags origin main
}

# clone_repo <namespace/name> <destination>
#
# `insteadOf` rewrites the URL before git records it, so the clone's `origin`
# would otherwise hold the sandbox path that `gits list` then prints. Put the
# forge URL back: the rewrite still applies on every fetch and push.
clone_repo() {
	git clone --quiet "git@github.com:$1.git" "$2"
	git -C "$2" remote set-url origin "git@github.com:$1.git"
}

echo "seeding fake repositories..." >&2

# ---------------------------------------------------------------------------
# Project 'acme' - one project covering every repository state gits renders.
# ---------------------------------------------------------------------------

# api - clean, tagged, in step with its upstream.
seed=$(new_remote acme/api)
write "$seed" main.go 'package main'
publish "$seed" "feat(api): add health endpoint" 12 v1.4.0
clone_repo acme/api "$CODE/acme/api"

# web - uncommitted work: one staged, one modified, one untracked.
seed=$(new_remote acme/web)
write "$seed" index.html '<h1>acme</h1>'
write "$seed" style.css 'body { margin: 0 }'
publish "$seed" "fix(web): correct header layout" 4 v2.1.3
clone_repo acme/web "$CODE/acme/web"
write "$CODE/acme/web" index.html '<h1>acme corp</h1>'
git -C "$CODE/acme/web" add index.html
write "$CODE/acme/web" style.css 'body { margin: 0; padding: 2rem }'
write "$CODE/acme/web" scratch.todo 'rewrite the footer'

# worker - a feature branch, one commit ahead of its upstream.
seed=$(new_remote acme/worker)
write "$seed" queue.go 'package queue'
publish "$seed" "refactor(queue): drain on shutdown" 9
clone_repo acme/worker "$CODE/acme/worker"
git -C "$CODE/acme/worker" checkout --quiet -b feat/retries
commit "$CODE/acme/worker" "feat(queue): retry failed jobs" 2
git -C "$CODE/acme/worker" push --quiet -u origin feat/retries
commit "$CODE/acme/worker" "test(queue): cover retry backoff" 1

# docs - behind its upstream, so `gits pull` has something to do.
seed=$(new_remote acme/docs)
write "$seed" guide.md '# Guide'
publish "$seed" "docs: write the getting started guide" 20
clone_repo acme/docs "$CODE/acme/docs"
commit "$seed" "docs: document the config file" 6
publish "$seed" "docs: add a troubleshooting section" 3
git -C "$CODE/acme/docs" fetch --quiet origin

# infra - clean and tagged, with a stale branch to browse.
seed=$(new_remote acme/infra)
write "$seed" main.tf 'terraform {}'
publish "$seed" "feat(infra): provision the staging cluster" 15 v0.9.1
clone_repo acme/infra "$CODE/acme/infra"

# mobile - never cloned, so `gits clone` has something to do.
seed=$(new_remote acme/mobile)
write "$seed" App.swift 'import SwiftUI'
publish "$seed" "feat(mobile): add the onboarding flow" 7 v3.0.0

# ---------------------------------------------------------------------------
# Project 'oss' - sub-projects, for the tree view.
# ---------------------------------------------------------------------------

i=0
for repo in charm/vhs charm/gum tools/fzf; do
	i=$((i + 1))
	seed=$(new_remote "$repo")
	write "$seed" main.go 'package main'
	publish "$seed" "feat: cut the first public release" $((22 + i)) "v0.$i.0"
	clone_repo "$repo" "$CODE/oss/$repo"
done
write "$CODE/oss/charm/gum" main.go 'package main // patched locally'

# ---------------------------------------------------------------------------
# Project 'dotfiles' - a hand-listed project with absolute directories.
# ---------------------------------------------------------------------------

for repo in rafi/dotfiles rafi/vim-config; do
	seed=$(new_remote "$repo")
	write "$seed" install.sh 'echo installing'
	publish "$seed" "chore: rearrange the config tree" 18
done
clone_repo rafi/dotfiles "$HOME_DIR/.config"
clone_repo rafi/vim-config "$HOME_DIR/.config/nvim"
write "$HOME_DIR/.config" install.sh 'echo installing everything'

# An orphan: cloned under a project's path, declared by no project.
clone_repo acme/api "$CODE/acme/legacy-api"

# ---------------------------------------------------------------------------
# Clones no project declares, for `gits discover` and `gits add` to claim.
#
# They live outside ~/code so the demo's `discover` run has something to write
# every time: everything under ~/code is already covered by the config file
# below, and a covered repository is never proposed twice.
# ---------------------------------------------------------------------------

WORK="$HOME_DIR/work"
i=0
for repo in backend/auth backend/billing platform/gateway platform/runner; do
	i=$((i + 1))
	seed=$(new_remote "$repo")
	write "$seed" main.go 'package main'
	publish "$seed" "feat: extract the service boundary" $((5 + i))
	clone_repo "$repo" "$WORK/$repo"
done

# A lone clone somewhere else entirely, for `gits add` to record by hand.
seed=$(new_remote labs/prototype)
write "$seed" sketch.go 'package sketch'
publish "$seed" "feat: sketch the new indexer" 2
clone_repo labs/prototype "$HOME_DIR/labs/prototype"

rm -rf "$SEED"

# Seeding is done, so narrow the rewrite to the namespaces the demo actually
# fetches, pushes or clones from: `acme` and the `oss` projects.
#
# `gits add` and `gits discover` write the *expanded* remote URL as `src:`, so
# a rewrite still covering `backend/`, `platform/` or `labs/` would put the
# sandbox's own path into the config file they build, in full view of the
# recording. Left un-rewritten, those clones report the forge URL their origin
# holds, and the demo never fetches from them.
write_git_config acme/ charm/ tools/ rafi/

# ---------------------------------------------------------------------------
# The config file the demo runs against.
# ---------------------------------------------------------------------------

cat >"$HOME_DIR/.gits.yaml" <<'EOF'
---
# ~/.gits.yaml

# Acme's services, listed by hand under one project path.
acme:
  desc: Acme Corp services
  path: ~/code/acme
  repos:
    - dir: api
      src: git@github.com:acme/api.git
    - dir: web
      src: git@github.com:acme/web.git
    - dir: worker
      src: git@github.com:acme/worker.git
    - dir: docs
      src: git@github.com:acme/docs.git
    - dir: infra
      src: git@github.com:acme/infra.git
    - dir: mobile
      src: git@github.com:acme/mobile.git

# Nested projects, each with its own path.
oss:
  desc: Open source I follow
  subprojects:
    - name: charm
      path: ~/code/oss/charm
      repos:
        - dir: vhs
          src: git@github.com:charm/vhs.git
        - dir: gum
          src: git@github.com:charm/gum.git
    - name: tools
      path: ~/code/oss/tools
      repos:
        - dir: fzf
          src: git@github.com:tools/fzf.git

# Listed by hand, with absolute directories.
dotfiles:
  desc: My dotfiles
  repos:
    - dir: ~/.config
      src: git@github.com:rafi/dotfiles.git
    - dir: ~/.config/nvim
      src: git@github.com:rafi/vim-config.git

settings:
  cache: false
EOF

echo "sandbox ready at $SANDBOX" >&2
