# hostebin task runner. Run `just` to list every recipe.
# Install just: https://github.com/casey/just

module_path := "github.com/readeem/hostebin"
version_pkg := module_path + "/internal/version"

# Version comes from the git tag, so local builds and CI builds agree.
git_version := `git describe --tags --dirty --always 2>/dev/null || echo dev`
version := trim_start_match(git_version, "v")
commit := `git rev-parse --short HEAD 2>/dev/null || echo none`
date := `date -u +%Y-%m-%dT%H:%M:%SZ`

ldflags := "-s -w" + \
    " -X " + version_pkg + ".Version=" + version + \
    " -X " + version_pkg + ".Commit=" + commit + \
    " -X " + version_pkg + ".Date=" + date

bin := "hostebin"

[private]
default:
    @just --list --unsorted

# Print the version this tree would build
version:
    @echo "{{ version }} ({{ commit }})"

# Build ./hostebin with version metadata baked in
build:
    CGO_ENABLED=0 go build -trimpath -ldflags "{{ ldflags }}" -o {{ bin }} ./cmd/hostebin

# Build without embedded Tailscale support (smaller binary, rejects --tailscale)
build-slim:
    CGO_ENABLED=0 go build -tags notsnet -trimpath -ldflags "{{ ldflags }}" -o {{ bin }} ./cmd/hostebin

# Install into $GOBIN (defaults to ~/go/bin) with version metadata
install:
    go install -trimpath -ldflags "{{ ldflags }}" ./cmd/hostebin

# Run the server from source
serve *ARGS:
    go run -ldflags "{{ ldflags }}" ./cmd/hostebin serve {{ ARGS }}

# Upload files from source, e.g. `just up report.html`
up *ARGS:
    @go run -ldflags "{{ ldflags }}" ./cmd/hostebin up {{ ARGS }}

test:
    go test ./...

# Everything CI runs on the test matrix
test-all:
    go test ./...
    go test -tags notsnet ./...
    go test -race ./...

fmt:
    gofmt -w .

vet:
    go vet ./...
    go vet -tags notsnet ./...

tidy:
    go mod tidy

# Optional: runs golangci-lint when it is installed
lint:
    @command -v golangci-lint >/dev/null && golangci-lint run || echo "golangci-lint not installed; skipping"

# The full gate CI enforces: formatting, vet, tests
check: vet test
    @test -z "$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

# Validate .goreleaser.yaml
release-check:
    goreleaser check

# Build every release artifact locally into ./dist without publishing
release-snapshot:
    goreleaser release --snapshot --clean --skip=publish

# Tag a release and push it; the release workflow does the rest. `just tag 0.1.0`
tag VERSION:
    #!/usr/bin/env sh
    set -eu
    tag="v$(printf '%s' '{{ VERSION }}' | sed 's/^v//')"
    git diff --quiet || { echo "working tree is dirty; commit first" >&2; exit 1; }
    git tag -a "$tag" -m "$tag"
    git push origin "$tag"
    echo "pushed $tag; watch: gh run watch"

# Build the container image
docker:
    docker build --build-arg VERSION={{ version }} -t hostebin:{{ version }} -t hostebin:latest .

clean:
    rm -rf dist {{ bin }} {{ bin }}.exe
