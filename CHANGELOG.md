# Change-log

- - -

## [v0.64.0](https://github.com/rafi/gits/compare/v0.25.0...v0.64.0) - 2026-10-24

### Features

- **(add)** create a config file when the user has none - (406e9f0)
- **(discover)** build projects from clones on disk - (bf900d7)
- **(labels)** address repositories by label - (adac1f0)
- **(config)** discover from self-hosted forge hosts - (5501a65)
- **(providers)** pace and retry forge requests - (537955e)
- **(providers)** discover from Gitea and Forgejo - (5a4dd33)
- **(providers)** discover from Gerrit - (3513e49)
- **(status)** color each status glyph - (dafa667)
- **(theme)** add palette, role mapping and Build - (6c76dc7)
- **(theme)** add ansi default theme - (89bd3f4)
- **(theme)** add 30 curated themes and registry - (b2d6b3d)
- **(config)** add theme name and style overrides - (117580d)
- **(cli)** select theme via --theme and GITS_THEME - (02d230c)
- **(theme)** add gits theme list and show - (56a0886)
- **(status)** color Age column by commit recency - (de1e8cd)
- **(sync)** list repositories new since last cache - (c4e854c)

- - -

## [v0.25.0](https://github.com/rafi/gits/compare/v0.11.2...v0.25.0) - 2026-09-14

### Features

- **(core)** subprocess git client, worker pool and charm.land v2 UI - (6fc6898)
- **(status)** compact table with --stat, --dirty and --unsynced - (4fd9dbd)
- **(push,status,config)** [**breaking**] push command, JSON status and provider tokens - (6d1db3c)
- **(exec,doctor,completion)** run commands, diagnose setup, complete flags - (5e02f63)
- **(sync)** say what each project refreshed - (8401582)
- **(add)** take many repositories, keep the file intact - (46ae168)
- **(status)** report as it goes, drop a lone footer - (99ec386)

### Bug Fixes

- **(providers,cache,config)** harden discovery, caching and configuration - (b4c25bc)
- **(cli,loader)** exit codes, ordering and an injected slog logger - (472d5f8)

### Performance Improvements

- one snapshot per repository and fewer subprocesses - (d409d31)

### Refactoring

- **(bulk)** one module behind every bulk command - (5c049f4)
- **(wire)** move JSON envelope to service - (5165c61)
- separate view deps from business deps - (d260f72)
- split config loading from CLI styling - (a90845d)
- dismantle the shared cli helpers package - (ea8b0cf)
- split argument resolution from prompting - (51ab59f)
- retire the internal/types grab-bag - (a13f6aa)
- separate execution from rendering - (a33e4b9)
- **(pull)** report a result instead of a line - (0b46ef3)
- **(fetch)** report a result instead of a line - (cb931d2)
- **(clone)** report a result instead of a line - (b29d96a)
- **(exec)** report a result instead of a line - (2c169c8)
- **(push)** report a result, drop the ANSI stripper - (768e93a)
- **(status)** split the probe from its renderers - (f7196ce)
- **(catalog)** rename and split the project loader - (79ff432)
- **(doctor)** split the checks from the table - (ee63723)
- **(checkout)** split branch listing from the prompt - (17b834a)
- complete the layer tree and enforce it - (c6c7f80)
- **(orphan,sync)** move the work into the service - (946dc2c)
- organize the codebase by responsibility - (3a599ba)

### Documentation

- state each rule where the code relies on it - (b622947)
- rewrite the README around getting started - (2659106)

### Build System

- require Go 1.27 - (4faedd5)

- - -

## v0.11.2 - 2026-01-06

### Features

- (**contrib**) introduce cdgit for fish shell - (19c96c6)

### Bug Fixes

- (**exec**) use alternate command with context - (90cbb18)

### Miscellaneous Chores

- (**ci**) update golangci.yml for version 2 - (2e41ed4)
- (**ci**) update actions - (31b41f2)
- (**deps**) fix deprecations - (e311a0c)
- (**deps**) update packages - (db0d821)
- upgrade deps - (d6d46a3)
- upgrade go-git - (b7c9341)

- - -

## [v0.11.1](https://github.com/rafi/gits/compare/v0.11.0...v0.11.1) - 2024-12-17

### Miscellaneous Chores

- **(ci)** use proper actions - (d58ad33)
- align pointer receivers - (b566bda)
- use go 1.23 and update dependencies - (f78e266)

- - -

## [v0.11.0](https://github.com/rafi/gits/compare/v0.10.0...v0.11.0) - 2024-09-24

### Bug Fixes

- **(repo)** use 'src' as remote - (4306ec4)

### Miscellaneous Chores

- **(ci)** replace drone with git actions - (d83676e)
- **(deps)** update vulnerable packages - (934de7d)

### Performance Improvements

- clone, fetch and pull in parallel - (3452ff8)

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
