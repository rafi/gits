# Change-log

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
