# github.com/rafi/gits
# ---
BINARY := gits
VERSION := $(shell git describe --always --tags)
RELEASEDIR := bin/release
PLATFORMS := darwin linux
ARCHES := amd64 arm64
export CGO_ENABLED := 0

# Concat all release paths. (e.g. bin/release/gits-darwin-amd64, etc.)
ALL_TARGETS := $(foreach plat,$(PLATFORMS),$(foreach arch,$(ARCHES),\
	$(RELEASEDIR)/$(BINARY)-$(plat)-$(arch)))

.DEFAULT_GOAL := release
.PHONY: release
release: vendor $(ALL_TARGETS)

.PHONY: $(ALL_TARGETS)
$(ALL_TARGETS):
	@mkdir -p "$(RELEASEDIR)"
	GOOS=$(word 2,$(subst -, ,$@)) GOARCH=$(word 3,$(subst -, ,$@)) \
		go build -mod=vendor -o $@ \
			-ldflags="-s -w -X github.com/rafi/gits/cmd.version=$(VERSION)"

.PHONY: vendor
vendor:
	go mod vendor

.PHONY: test
test: lint
	go test -mod=vendor -v ./...

GOLANGCI_URL = https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh
GOLANGCI_VERSION = v1.52.2
GOLANGCI := bin/golangci-lint-$(GOLANGCI_VERSION)
$(GOLANGCI):
	curl -sfL "$(GOLANGCI_URL)" | sh -s "$(GOLANGCI_VERSION)"
	mv ./bin/golangci-lint "$(GOLANGCI)"

.PHONY: lint
lint: vendor $(GOLANGCI)
	$(GOLANGCI) run

.PHONY: clean
clean:
	rm -rf vendor "$(RELEASEDIR)" "$(GOLANGCI)"
