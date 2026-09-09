SHELL := /bin/bash
MODULE := github.com/MErenTalan/ekokod-rewrite
BIN := bin/ekokod
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(MODULE)/internal/buildinfo.version=$(VERSION) \
           -X $(MODULE)/internal/buildinfo.commit=$(COMMIT) \
           -X $(MODULE)/internal/buildinfo.date=$(DATE)

GOLANGCI_VERSION := v2.13.2
GOVULNCHECK_VERSION := v1.8.0

.PHONY: build test test-integration lint fmt tidy tools vuln

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

tools: ## Install pinned developer tools into $(GOPATH)/bin
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

lint: ## Run gofmt check, go vet and golangci-lint
	@test -z "$$(gofmt -l ./cmd ./internal)" || (echo "gofmt needed:"; gofmt -l ./cmd ./internal; exit 1)
	go vet ./...
	golangci-lint run

vuln: ## Scan dependencies for known vulnerabilities
	govulncheck ./...
