# Context

The ubiquitous language of `gits`. Terms here are the canonical names — use
them in issues, tests, commit messages, and output. This file is a glossary,
not a spec: no implementation details.

## Structure

### Project

A named group of repositories, declared in the user's config file. A project
either points at a **Provider Source** (repositories are discovered from
GitHub/GitLab/Bitbucket/filesystem) or lists its repositories explicitly, not
both. Projects nest.

### Sub-project

A project declared inside another project. It inherits its parent's Provider
Source and, when it declares no **Project Path** of its own, sits at a
directory named after it beneath the parent's path.

Avoid: "group", "org", "namespace" for this concept. A GitLab group is a
Provider Source, not a Sub-project.

### Repository

A single git repository belonging to a project. It has an identity from
config or from the provider (name, namespace, description), a **Repo Src**,
optionally a **Repo Dir**, and a **Repo State**.

Avoid: "repo" in prose and documentation; `repo` is fine as an identifier.

### Provider Source

The origin a project's repositories are discovered from — one of `github`,
`gitlab`, `bitbucket`, or `filesystem` — together with the search term that
selects them (an owner, a group ID, or a path).

Avoid: "backend", "remote" for this concept. A **Remote** is a git remote.

## Paths

### Project Path

The directory a project's repositories live under, from the project's `path`
config. May be absolute or `~`-relative. A project may have none, in which
case its repositories must each carry an absolute **Repo Dir**.

### Repo Dir

A repository's explicit local directory, from its `dir` config. Absolute and
`~`-relative values state exactly where the repository lives. A relative
value is resolved against the **Project Path**, and is meaningless without
one.

### Repo Src

Where a repository is cloned from — its clone URL or source path. When a
repository has no **Repo Dir**, its directory name is derived from the Repo
Src basename with any `.git` suffix removed.

## Commands

### Bulk Command

A command that applies one operation to every **Repository** in a **Project**,
reporting the outcome per Repository. Naming a single Repository narrows the
same operation to it; it does not make the command something else.

A Bulk Command runs to completion without asking the user anything. Selecting
*what* to operate on may be interactive; the operation itself is not.

A command that acts on exactly one Repository by nature, that iterates
something other than the Project's Repositories, or that changes configuration
rather than operating on Repositories, is not a Bulk Command.

Avoid: "walker" — see **Traversal**. Avoid "batch": a Bulk Command has no
batch boundary, and the word invites one.

### Traversal

The order a Bulk Command visits a **Project** in: depth-first, a Project's own
**Repositories** before its **Sub-projects**. It names both the ordering and
the machinery that performs it.

The order decides how results are listed, not how they are grouped: a
renderer that needs the tree — `gits status -o json`, the status tables —
walks the **Project** the run reports and looks each Repository's result up.

Avoid: "walker" and "the walk" for this concept, or for the machinery behind
it. Both words are already spoken for elsewhere in this codebase — a
filesystem directory scan and a provider's pagination are each a walk, and
neither is this — so using them here asks the reader to work out which one is
meant.

### Finding

One thing `gits doctor` has to say about the configuration or the
environment: a **Subject** that was inspected, a **Level**, and a sentence.

A Finding is not a **Repo State**. State describes one **Repository**'s local
presence and is computed for every command; a Finding describes a defect in
what the user wrote or in what is installed, and only `doctor` produces one.
The two overlap for exactly one repository condition — a config that gives a
repository no local home is both an `error` state and a Finding — and they say
it to different audiences: the state tells a command to pass the repository
over, the Finding tells the user which line of their config to change.

**Level** is one of `error`, `warning` or `info`, and is wire vocabulary: the
`-o json` document carries these strings. An `error` exits non-zero; the other
two do not.

**Subject** names what was inspected, so Findings stay greppable: a config
key, a binary, a project by its path through the tree (`acme/tools`), or a
repository within one (`acme.api`).

Avoid: "check" for the output. A check is the act; a Finding is what it
produced. Avoid "diagnostic", which collides with **Diagnostic Output** —
Findings are **Result Output**, since they are what `doctor` was asked for.

## Output

### Result Output

What a command was asked for: the status table, the JSON document, the
list of names. It goes to stdout and nothing else does, so any command's
output can be piped or redirected without filtering.

### Diagnostic Output

Everything a command emits about producing its Result Output: live
per-repository progress, the summary footer, the error epilogue. It goes
to stderr and carries nothing a consumer of Result Output needs.

### Debug Tracing

What `gits` did on the way to an answer: pages fetched from a provider,
cache hits and misses, git's own stderr. It exists for `-v` and for a bug
report, is emitted through the `*slog.Logger` on `types.Runtime`, and is off
below warn level by default.

Distinct from **Diagnostic Output**, which is prose addressed to the user and
is always shown. A message a user has to act on is never a log record: it is
written to `deps.Err` as a sentence.

Avoid: "stdout"/"stderr" for these concepts in prose. The streams are how
the split is implemented; the split itself is a promise about what is
safe to pipe.

## Git

### Remote

A named URL in a cloned repository's git config, conventionally `origin`.
Nearly every clone has one. Having a Remote says nothing about whether any
particular branch can be pushed.

Distinct from **Provider Source**, which is a config-level concept about
where `gits` discovers repositories.

### Upstream

The specific remote branch that a local branch is configured to track. A
branch created locally has a **Remote** but no Upstream until it is first
pushed with `-u`. Pushing a branch with no Upstream is undefined, and this
is the condition `gits push` skips on — not the absence of a Remote.

Avoid: using "remote" to mean Upstream. The distinction is load-bearing.

**Gone Upstream** — an Upstream that is still configured but whose **Remote**
branch no longer exists, typically because the branch was merged and cleaned
up. The word is git's own: `git branch -vv` renders it `[origin/feat: gone]`.
It is a case of Upstream, not a peer concept — a branch with a Gone Upstream
has one, which is what makes it different from a branch that was never pushed.

Avoid: "stale", "orphaned", "dangling" for this state.

## State

### Repo State

What `gits` knows about a repository's local presence, computed before any
per-command work. Exactly one of:

- **`ok`** — a local clone exists and is readable.
- **`not-cloned`** — a local path is known, and nothing is there. This is the
  state `gits clone` acts on.
- **`remote-only`** — the repository is provider-backed and the config gives
  it no local home. Nothing to clone it into. This state means only that; a
  repository that is *not* provider-backed and has no local home is a
  configuration defect, and is `error`.
- **`error`** — the configuration for this repository is defective, or a path
  exists but is not a readable git repository. Carries a **Reason**. The
  configuration defects are: no **Project Path** and no **Repo Dir** on a
  repository that isn't provider-backed, and a relative **Repo Dir** on a
  project that declares no **Project Path**.
- **`unknown`** — not yet classified.

These strings are the wire vocabulary: they appear verbatim in JSON output
and are a public contract. Display layers are free to render them as icons
or other labels.

Avoid: "N/A", "missing", "absent" for `not-cloned`. Avoid "remote" alone for
`remote-only` — it collides with git's notion of a remote.

### Reason

The human-readable explanation attached to a repository in the `error`
state. Never populated for any other state.
