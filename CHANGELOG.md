# Change-log

- - -

## [v0.25.0](https://github.com/rafi/gits/compare/v0.11.2...v0.25.0) - 2026-09-14

### Features

- **(core)** subprocess git client, worker pool and charm.land v2 UI - (6fc6898) - Rafael Bodill
- **(status)** compact table with --stat, --dirty and --unsynced - (4fd9dbd) - Rafael Bodill
- **(push,status,config)** [**breaking**] push command, JSON status and provider tokens - (6d1db3c) - Rafael Bodill
- **(exec,doctor,completion)** run commands, diagnose setup, complete flags - (5e02f63) - Rafael Bodill
- **(sync)** say what each project refreshed - (8401582) - Rafael Bodill
- **(add)** take many repositories, keep the file intact - (46ae168) - Rafael Bodill
- **(status)** report as it goes, drop a lone footer - (99ec386) - Rafael Bodill

### Bug Fixes

- **(providers,cache,config)** harden discovery, caching and configuration - (b4c25bc) - Rafael Bodill
- **(cli,loader)** exit codes, ordering and an injected slog logger - (472d5f8) - Rafael Bodill

### Performance Improvements

- one snapshot per repository and fewer subprocesses - (d409d31) - Rafael Bodill

### Refactoring

- **(bulk)** one module behind every bulk command - (5c049f4) - Rafael Bodill
- **(wire)** move JSON envelope to service - (5165c61) - Rafael Bodill
- separate view deps from business deps - (d260f72) - Rafael Bodill
- split config loading from CLI styling - (a90845d) - Rafael Bodill
- dismantle the shared cli helpers package - (ea8b0cf) - Rafael Bodill
- split argument resolution from prompting - (51ab59f) - Rafael Bodill
- retire the internal/types grab-bag - (a13f6aa) - Rafael Bodill
- separate execution from rendering - (a33e4b9) - Rafael Bodill
- **(pull)** report a result instead of a line - (0b46ef3) - Rafael Bodill
- **(fetch)** report a result instead of a line - (cb931d2) - Rafael Bodill
- **(clone)** report a result instead of a line - (b29d96a) - Rafael Bodill
- **(exec)** report a result instead of a line - (2c169c8) - Rafael Bodill
- **(push)** report a result, drop the ANSI stripper - (768e93a) - Rafael Bodill
- **(status)** split the probe from its renderers - (f7196ce) - Rafael Bodill
- **(catalog)** rename and split the project loader - (79ff432) - Rafael Bodill
- **(doctor)** split the checks from the table - (ee63723) - Rafael Bodill
- **(checkout)** split branch listing from the prompt - (17b834a) - Rafael Bodill
- complete the layer tree and enforce it - (c6c7f80) - Rafael Bodill
- **(orphan,sync)** move the work into the service - (946dc2c) - Rafael Bodill
- organize the codebase by responsibility - (3a599ba) - Rafael Bodill

### Documentation

- state each rule where the code relies on it - (b622947) - Rafael Bodill
- rewrite the README around getting started - (2659106) - Rafael Bodill

### Build System

- require Go 1.27 - (4faedd5) - Rafael Bodill

- - -

## v0.11.2 - 2026-01-06

### Features

- (**contrib**) introduce cdgit for fish shell - (19c96c6) - Rafael Bodill

### Bug Fixes

- (**exec**) use alternate command with context - (90cbb18) - Rafael Bodill

### Miscellaneous Chores

- (**ci**) update golangci.yml for version 2 - (2e41ed4) - Rafael Bodill
- (**ci**) update actions - (31b41f2) - Rafael Bodill
- (**deps**) fix deprecations - (e311a0c) - Rafael Bodill
- (**deps**) update packages - (db0d821) - Rafael Bodill
- upgrade deps - (d6d46a3) - Rafael Bodill
- upgrade go-git - (b7c9341) - Rafael Bodill

- - -

## [v0.11.1](https://github.com/rafi/gits/compare/v0.11.0...v0.11.1) - 2024-12-17

### Miscellaneous Chores

- **(ci)** use proper actions - (d58ad33) - Rafael Bodill
- align pointer receivers - (b566bda) - Rafael Bodill
- use go 1.23 and update dependencies - (f78e266) - Rafael Bodill

- - -

## [v0.11.0](https://github.com/rafi/gits/compare/v0.10.0...v0.11.0) - 2024-09-24

### Bug Fixes

- **(repo)** use 'src' as remote - (4306ec4) - Rafael Bodill

### Miscellaneous Chores

- **(ci)** replace drone with git actions - (d83676e) - Rafael Bodill
- **(deps)** update vulnerable packages - (934de7d) - Rafael Bodill

### Performance Improvements

- clone, fetch and pull in parallel - (3452ff8) - Rafael Bodill

- - -

## [v0.10.0](https://github.com/rafi/gits/compare/v0.9.0...v0.10.0) / 2024-03-20

### Features

- introduce 'add' command
- introduce 'orphan' command

### Bug Fixes

- implement github repository pagination
- completion and repository selection
- bust cache only when struct changes
- cache version mismatch
- repo name selection when defined with path only

### Misc

- improve error handling
- document 'gits cd' usage

- - -

## [v0.9.0](https://github.com/rafi/gits/compare/v0.5.0...v0.9.0) / 2024-02-15

### Features

- introduce 'pull' and 'cd' commands
- introduce 'browse' command
- add include/exclude filters and syntax
- 2nd argument can be a sub-project path
- adjust cache expiration to 1 week
- nicer layout for fzf

### Bug Fixes

- path based argument as project name
- disregard archived/empty repositories

### Misc

- refactor project loader package
- clarity in error messages
- add license

- - -

## [v0.5.0](https://github.com/rafi/gits/compare/v0.3.5...v0.5.0) / 2023-12-10

### ⚠ BREAKING CHANGES

- **config:** structure change, remove 'projects:' key

### Features

- **providers:** introduce GitHub/GitLab/Bitbucket support
- **list:** new output formats json/wide/table/tree/name
- **theme:** improve theme support
- **git:** use `go-git` client for some operations

### Bug Fixes

- **checkout:** fix remote branch checkout
- improve error handling

- - -

## [v0.3.5](https://github.com/rafi/gits/compare/v0.3.0...v0.3.5) / 2023-05-18

- Add '--force' to git fetch
- Refactor code
- Upgrade to Go 1.18

- - -

## [v0.3.0](https://github.com/rafi/gits/compare/v0.2.1...v0.3.0) / 2020-09-04

- List command can list project directories
- Improve error handling
- Migrate from dep to Go modules

- - -

## [v0.2.1](https://github.com/rafi/gits/compare/v0.2.0...v0.2.1) / 2020-02-24

- Fix bug with stored project collection

- - -

## [v0.2.0](https://github.com/rafi/gits/compare/v0.1.0...v0.2.0) / 2018-11-09

- Add bash completion
- Introduce list command
- Document YAML config definition
- Introduce clone command
- Optimize code and use struct methods
- Add animated overview

- - -

## v0.1.0 / 2018-11-03

- Introduce version command
- Add CHANGELOG.md and travis badge
- Fix travis api_key placement
- Add dependencies as vendor/ and dep Gopkg
- Add release management scripts and configuration
- Fix lint issues
- Use combined output when executing commands
- Introduce README.md
- Introduce checkout command
- Initial commit

- - -
