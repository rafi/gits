# github.com/rafi/gits
# ---

BINNAME ?= gits
BINDIR  := bin/release
VERSION := $(shell git describe --always --tags)

PLATFORMS := darwin linux
ARCHES := amd64 arm64

GOMODULE      = github.com/rafi/gits
GOBIN         = $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN         = $(shell go env GOPATH)/bin
endif
GOX           = $(GOBIN)/gox
GOIMPORTS     = $(GOBIN)/goimports
GOLANGCI_LINT = $(GOBIN)/golangci-lint

# Go options
PKG         := ./...
TAGS        :=
TESTS       := .
TESTFLAGS   :=
LDFLAGS     := -w -s
GOFLAGS     :=
export CGO_ENABLED := 0

# Rebuild the binary if any of these files change
SRC := $(shell find . -type f -name '*.go' -print) go.mod go.sum

# Required for globs to work correctly
SHELL = /usr/bin/env bash

GIT_COMMIT = $(shell git rev-parse HEAD)
GIT_SHA    = $(shell git rev-parse --short HEAD)
GIT_TAG    = $(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
GIT_DIRTY  = $(shell test -n "`git status --porcelain`" && echo "dirty" || echo "clean")

ifdef VERSION
	BINARY_VERSION = $(VERSION)
endif
BINARY_VERSION ?= ${GIT_TAG}

# Only set Version if building a tag or VERSION is set
ifneq ($(BINARY_VERSION),)
	LDFLAGS += -X ${GOMODULE}/internal/version.version=${BINARY_VERSION}
endif

VERSION_METADATA = unreleased
# Clear the "unreleased" string in BuildMetadata
ifneq ($(GIT_TAG),)
	VERSION_METADATA =
endif

LDFLAGS += -X ${GOMODULE}/internal/version.metadata=${VERSION_METADATA}
LDFLAGS += -X ${GOMODULE}/internal/version.gitCommit=${GIT_COMMIT}
LDFLAGS += -X ${GOMODULE}/internal/version.gitTreeState=${GIT_DIRTY}
LDFLAGS += $(EXT_LDFLAGS)

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# ------------------------------------------------------------------------------
#  build

# Concat all release paths. (e.g. bin/release/gits-darwin-amd64, etc.)
ALL_TARGETS := $(foreach plat,$(PLATFORMS),$(foreach arch,$(ARCHES),\
	$(BINDIR)/$(BINNAME)-$(plat)-$(arch)))

.DEFAULT_GOAL := release
.PHONY: release
release: $(ALL_TARGETS)

.PHONY: $(ALL_TARGETS)
$(ALL_TARGETS): $(SRC)
	@mkdir -p "$(BINDIR)"
	GOOS=$(word 2,$(subst -, ,$@)) GOARCH=$(word 3,$(subst -, ,$@)) \
		go build -o $@ $(GOFLAGS) -trimpath -tags '$(TAGS)' -ldflags '$(LDFLAGS)' ./cmd/gits

.PHONY: vendor
vendor:
	go mod vendor

# ------------------------------------------------------------------------------
#  test

.PHONY: test
test: test-style ## Run all tests.
test: test-unit

.PHONY: test-unit
test-unit: ## Run only unit-tests.
	@echo
	@echo "==> Running unit tests <=="
	go test $(GOFLAGS) -run $(TESTS) $(PKG) $(TESTFLAGS)

.PHONY: test-style
test-style: $(GOLANGCI_LINT) ## Run golang-ci linter.
	$(GOLANGCI_LINT) run

.PHONY: format
format: $(GOIMPORTS) ## Format entire code-base with goimports.
	GO111MODULE=on go list -f '{{.Dir}}' ./... | xargs $(GOIMPORTS) -w -local ${GOMODULE}

# ------------------------------------------------------------------------------
#  dependencies

# If go install is run from inside the project directory it will add the
# dependencies to the go.mod file. To avoid that we change to a directory
# without a go.mod file when downloading the following dependencies

$(GOX):
	(cd /; GO111MODULE=on go install github.com/mitchellh/gox@latest)

$(GOIMPORTS):
	(cd /; GO111MODULE=on go install golang.org/x/tools/cmd/goimports@latest)

$(GOLANGCI_LINT):
	(cd /; GO111MODULE=on go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)

# ------------------------------------------------------------------------------

.PHONY: clean
clean:
	@rm -rf '$(BINDIR)' ./vendor

.PHONY: info
info:
	@echo "Version:           ${VERSION}"
	@echo "Git Tag:           ${GIT_TAG}"
	@echo "Git Commit:        ${GIT_COMMIT}"
	@echo "Git Tree State:    ${GIT_DIRTY}"
