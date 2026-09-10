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

.PHONY: build test test-integration lint fmt tidy tools vuln ci

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

ci: lint test build web-lint web-test web-build web-audit ## Everything CI runs, except integration tests, govulncheck and shellcheck

.PHONY: up down dev logs ps migrate seed generate offline-bundle env-docker

env-docker: ## Generate .env.docker with fresh development secrets (idempotent)
	./scripts/gen-env-docker.sh

up: env-docker ## Build and start the whole stack
	docker compose up -d --build

down: ## Stop the stack and remove volumes
	docker compose down -v

dev: up logs ## Start the stack and follow the logs

logs:
	docker compose logs -f api worker scheduler

ps:
	docker compose ps

migrate: env-docker ## Apply migrations inside the stack
	docker compose run --rm migrate migrate up

seed: env-docker ## Load reference datasets inside the stack
	docker compose run --rm api seed

generate: ## Regenerate sqlc types and the OpenAPI client (populated from F1)
	@echo "no generators configured yet"

offline-bundle: ## Build the air-gapped install bundle
	./scripts/offline-bundle.sh

.PHONY: web-install web-lint web-test web-build web-audit

web-install:
	cd web && pnpm install --frozen-lockfile

web-lint:
	cd web && pnpm lint && pnpm typecheck && pnpm check:i18n-parity

web-test:
	cd web && pnpm test

web-build:
	cd web && pnpm build

web-audit: ## Fail on high-severity frontend dependency vulnerabilities
	cd web && pnpm audit --audit-level=high
