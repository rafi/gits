# gits justfile

BINNAME := "gits"
BINDIR  := "bin/release"

VERSION    := `git describe --always --tags`
GIT_COMMIT := `git rev-parse HEAD`
GIT_SHA    := `git rev-parse --short HEAD`
GIT_TAG    := `git describe --tags --abbrev=0 --exact-match 2>/dev/null || true`
GIT_DIRTY  := `test -n "$(git status --porcelain)" && echo "dirty" || echo "clean"`

BINARY_VERSION := if VERSION == '' { GIT_TAG } else { VERSION }
VERSION_METADATA := if GIT_TAG == '' { 'unreleased' } else { '' }

GOMODULE := `go list -m`
LDFLAGS := (
  "-s -w"
  + " -X " + GOMODULE + "/internal/version.version=" + BINARY_VERSION
  + " -X " + GOMODULE + "/internal/version.metadata=" + VERSION_METADATA
)

# Tool requirements, as semver requirements (e.g. "^2", ">=2.12", "*"). A tool
# already on PATH that satisfies its requirement is used as-is — provided it
# was also built by the active Go toolchain, see _go_tool_version.
GOLANGCI_VERSION := ">=2.13.1"
GOIMPORTS_VERSION := "^0"

[private]
default:
  @just --list

# build all release binaries
release: _build-linux _build-darwin
_build-linux:  (build "linux"  "amd64") (build "linux"  "arm64")
_build-darwin: (build "darwin" "amd64") (build "darwin" "arm64")

# build binary
build $GOOS='' $GOARCH='':
  go build \
    -trimpath \
    -ldflags='{{ LDFLAGS }}' \
    -o {{ BINDIR / BINNAME }}{{ if GOOS != '' {'-'+GOOS} else {''} }}{{ if GOARCH != '' {'-'+GOARCH} else {''} }} \
    ./cmd/gits

# run tests
test *flags:
  go test -race {{ flags }} ./...

# run golangci-lint checks
lint *flags: golangci-lint
  golangci-lint run {{ flags }}

# format with goimports
format: goimports
  go list -f '{{{{.Dir}}' ./... | xargs goimports -w -local '{{ GOMODULE }}'

# bump version, changelog and tag (requires: brew install git-cliff)
bump version:
  echo "Bumping from $(git describe --tags --abbrev=0) to {{ version }}"
  sed -Ei '' 's,(version = )"v[0-9.]+",\1"{{ version }}",' internal/version/version.go
  git cliff --unreleased --tag "{{ version }}" --prepend CHANGELOG.md
  git add CHANGELOG.md internal/version/version.go
  git commit -m "chore(version): bump {{ version }}"
  git tag -a "{{ version }}" -m "{{ version }}"
  echo "Tagged {{ version }}. Push with: git push --follow-tags"

# video tape a demo
vhs:
  vhs examples/demo.tape

# remove build artifacts
clean:
  rm -rf '{{ BINDIR }}'

# print build version info
info:
  @echo 'Version:        {{ VERSION }}'
  @echo 'Git Tag:        {{ GIT_TAG }}'
  @echo 'Git Commit:     {{ GIT_COMMIT }}'
  @echo 'Git Tree State: {{ GIT_DIRTY }}'

# TOOLS
# ---

[private]
golangci-lint: (_fetch recipe_name() GOLANGCI_VERSION \
  "github.com/golangci/golangci-lint/v2/cmd/golangci-lint")

[private]
goimports: (_fetch recipe_name() GOIMPORTS_VERSION \
  "golang.org/x/tools/cmd/goimports")

# Get go tool version, or a sentinel that satisfies no requirement.
#
# A tool built by a Go toolchain other than the active one reports "stale" so
# it gets reinstalled: golangci-lint type-checks against the stdlib source and
# dies inside it when the two disagree, which reads as a crash rather than as
# a version problem. That failure hid a formatting regression on `next` until
# the linter was reinstalled by hand.
_go_tool_version bin:
  @go version -m "$(command -v {{ bin }})" 2>/dev/null | \
    awk -v active="$(go env GOVERSION)" \
        'NR == 1 { built = $2 } \
         $1 == "mod" { v = substr($3, 2) } \
         END { if (built == "") print "0.0.0-absent"; \
               else if (built != active) print "0.0.0-stale"; \
               else print (v ? v : "0.0.0-absent") }'

# Install `url@latest` unless `bin` already satisfies requirement `req`.
_fetch bin req url \
  have=shell(just_executable() + " _go_tool_version $1", bin):
  @[ "{{ semver_matches(have, req) }}" = true ] || { \
    (cd / && go install "{{ url }}@latest") && echo "installed {{ bin }}"; }
