SHELL := /bin/bash
MODULE := github.com/MErenTalan/ekokod-rewrite
BIN := bin/ekokod
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(MODULE)/internal/buildinfo.version=$(VERSION) \
           -X $(MODULE)/internal/buildinfo.commit=$(COMMIT) \
           -X $(MODULE)/internal/buildinfo.date=$(DATE)

.PHONY: build test test-integration lint fmt tidy

build: ## Build the ekokod binary
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ekokod

test: ## Run unit tests
	go test ./... -race -coverprofile=coverage.out -covermode=atomic

test-integration: ## Run integration tests (requires Docker)
	go test ./... -tags=integration -race -count=1

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
