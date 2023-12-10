# Change-log

## [v0.5.0](https://github.com/rafi/gits/compare/v0.3.5...v0.5.0) / 2023-12-10

### ⚠ BREAKING CHANGES

* **config:** structure change, remove 'projects:' key

### Features

* **providers:** introduce GitHub/GitLab/Bitbucket support
* **list:** new output formats json/wide/table/tree/name
* **theme:** improve theme support
* **git:** use `go-git` client for some operations

### Bug Fixes

* **checkout:** fix remote branch checkout
* improve error handling

## [v0.3.5](https://github.com/rafi/gits/compare/v0.3.0...v0.3.5) / 2023-05-18

* Add '--force' to git fetch
* Refactor code
* Upgrade to Go 1.18

## [v0.3.0](https://github.com/rafi/gits/compare/v0.2.1...v0.3.0) / 2020-09-04

* List command can list project directories
* Improve error handling
* Migrate from dep to Go modules

## [v0.2.1](https://github.com/rafi/gits/compare/v0.2.0...v0.2.1) / 2020-02-24

* Fix bug with stored project collection

## [v0.2.0](https://github.com/rafi/gits/compare/v0.1.0...v0.2.0) / 2018-11-09

* Add bash completion
* Introduce list command
* Document YAML config definition
* Introduce clone command
* Optimize code and use struct methods
* Add animated overview

## v0.1.0 / 2018-11-03

* Introduce version command
* Add CHANGELOG.md and travis badge
* Fix travis api_key placement
* Add dependencies as vendor/ and dep Gopkg
* Add release management scripts and configuration
* Fix lint issues
* Use combined output when executing commands
* Introduce README.md
* Introduce checkout command
* Initial commit
