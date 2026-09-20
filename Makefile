BINARY_NAME := aegis
MODULE := github.com/rishavkumarj/aegis
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)

.PHONY: all build run test lint clean install

## all: Build the binary (default target)
all: build

## build: Compile the binary with embedded version info
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/aegis

## run: Build and run the proxy server
run: build
	./bin/$(BINARY_NAME) serve

## test: Run all tests with race detector
test:
	go test -race -cover ./...

## test-v: Run tests with verbose output
test-v:
	go test -race -cover -v ./...

## bench: Run benchmarks
bench:
	go test -bench=. -benchmem ./...

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	go fmt ./...
	goimports -w .

## vet: Run go vet
vet:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/
	go clean -cache

## install: Install the binary to GOPATH/bin
install:
	go install -trimpath -ldflags="$(LDFLAGS)" ./cmd/aegis

## deps: Download and tidy dependencies
deps:
	go mod download
	go mod tidy

## config-init: Generate example config file
config-init: build
	./bin/$(BINARY_NAME) config init

## help: Show this help message
help:
	@echo "Aegis - API Key & Security Governance Proxy"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
