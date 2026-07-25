BINARY := requestCLI
PKG    := ./...

.PHONY: all build test cover vet fmt lint tidy clean install

all: fmt vet test build

build:
	go build -o $(BINARY) .

install:
	go install .

test:
	go test -race $(PKG)

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
	rm -f $(BINARY) coverage.out
