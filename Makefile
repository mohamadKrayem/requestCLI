BINARY := rq
LEGACY := requestCLI
PKG    := ./...

# Stamped into core.Version at link time. Falls back to the source default when
# the tree has no tags.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= rq

.PHONY: all build test smoke server cover vet fmt lint tidy clean install \
        release docker-build docker-run docker-smoke docker-up docker-down

all: fmt vet test build

# Builds rq and a requestCLI symlink beside it, for one release of backward
# compatibility. See ARCHITECTURE.md and the argv[0] deprecation notice in
# cmd.Execute.
build:
	go build -o $(BINARY) ./cmd/rq
	ln -sf $(BINARY) $(LEGACY)

install:
	go install ./cmd/rq

test:
	go test -race $(PKG)

# End-to-end run of the real binary against the local fixture. See TESTING.md.
smoke:
	bash scripts/smoke.sh

# Local fixture server for manual testing: http://127.0.0.1:8080 and https://127.0.0.1:8443
server:
	go run ./scripts/echoserver

cover:
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out | tail -1

vet:
	go vet $(PKG)

fmt:
	gofmt -l -w .

# Requires golangci-lint: https://golangci-lint.run/welcome/install/
lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) $(LEGACY) coverage.out
	rm -rf dist/

# --- Release --------------------------------------------------------------

# Builds every release archive into dist/, exactly as the Release workflow
# does, so a tag can be rehearsed locally before it is pushed. The archives are
# byte-reproducible, so this is also how a published checksum gets audited.
release:
	VERSION=$(VERSION) bash scripts/release.sh

# --- Docker ---------------------------------------------------------------
# The image is self-contained; none of these targets need a local Go toolchain.

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

# Usage: make docker-run ARGS="get example.com -S"
docker-run: docker-build
	docker run --rm -i $(IMAGE):latest $(ARGS)

# End-to-end check of the image itself: non-root user, CA trust store, stdin,
# and real requests against the containerised fixture server. `make smoke` stays
# the exhaustive suite and runs against a locally built binary.
docker-smoke:
	VERSION=$(VERSION) bash scripts/docker-smoke.sh

# Fixture server for manual testing, in a container instead of `make server`.
docker-up:
	docker compose up -d --build echoserver

docker-down:
	docker compose down -v
