PKGS := $(shell go list ./... | grep -v /vendor)
GOLANGCI_URL = https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh
GOLANGCI_VERSION = v1.23.6

.PHONY: test
test: lint
	go test -v $(PKGS)

GOLANGCI := bin/golangci-lint

$(GOLANGCI):
	curl -sfL $(GOLANGCI_URL) | sh -s $(GOLANGCI_VERSION)

.PHONY: lint
lint: $(GOLANGCI)
	./bin/golangci-lint run

BINARY := gits
VERSION := $(shell git describe --always --tags)
PLATFORMS := linux darwin
os = $(word 1, $@)

.PHONY: $(PLATFORMS)
$(PLATFORMS):
	mkdir -p release
	GOOS=$(os) GOARCH=amd64 go build \
			-ldflags="-X github.com/rafi/gits/cmd.Version=$(VERSION)" \
			-o release/$(BINARY)-$(VERSION)-$(os)-amd64

.PHONY: release
release: linux darwin
