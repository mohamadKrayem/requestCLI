BINARY := rq
LEGACY := requestCLI
PKG    := ./...

.PHONY: all build test smoke server cover vet fmt lint tidy clean install

all: fmt vet test build

# Builds rq and a requestCLI symlink beside it, for one release of backward
# compatibility. See ARCHITECTURE.md and the argv[0] deprecation notice in
# main.go.
build:
	go build -o $(BINARY) .
	ln -sf $(BINARY) $(LEGACY)

install:
	go install .

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
