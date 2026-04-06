BINARY    := preflight
BUILD_DIR := dist
GO        := /usr/local/go/bin/go

.PHONY: all build test test-unit test-cover lint fmt clean run-setup tidy help

all: build

## build: compile the binary into dist/
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY) ./cmd/preflight

## test: run all tests with race detector
test:
	$(GO) test -race -count=1 ./...

## test-unit: run unit tests only (skip integration)
test-unit:
	$(GO) test -race -count=1 -short ./...

## test-cover: run tests and open HTML coverage report
test-cover:
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	open coverage.html

## lint: vet + staticcheck
lint:
	$(GO) vet ./...
	staticcheck ./... 2>/dev/null || true

## fmt: format all Go source
fmt:
	$(GO) fmt ./...

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html

## run-setup: build and run preflight setup
run-setup: build
	$(BUILD_DIR)/$(BINARY) setup

## tidy: tidy module dependencies
tidy:
	$(GO) mod tidy

## help: print available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
