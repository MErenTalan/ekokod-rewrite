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
SQLC_VERSION := v1.30.0

.PHONY: build test test-integration test-perf lint fmt tidy tools vuln ci check-script-modes

build: ## Build the ekokod binary
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ekokod

test: ## Run unit tests
	go test ./... -race -coverprofile=coverage.out -covermode=atomic

test-integration: ## Run integration tests (requires Docker)
	go test ./... -tags=integration -race -count=1

# test-perf runs Task 13's slow F1 acceptance suite: 1,000,000 synthetic
# readings across 100 analyzers, asserting the chunk layout and EXPLAIN plan
# TestOneMillionReadingsChunkLayout expects (~70s insert + assertions on this
# machine). It is gated by EKOKOD_PERF=1, NOT by an extra build tag: the test
# still builds under plain -tags=integration (so `-run` can name it and CI's
# scheduled job needs no extra tag), but is skipped unless this variable is
# set — which is why test-integration above runs fast and unchanged. CI runs
# this target on a schedule, not per push.
test-perf: ## Run the slow F1 performance/acceptance suite (requires Docker; CI runs this on a schedule, not per push)
	EKOKOD_PERF=1 go test ./internal/store/postgres/ -tags=integration -race -count=1 -run 'TestOneMillionReadingsChunkLayout' -v

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

tools: ## Install pinned developer tools into $(GOPATH)/bin
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

lint: ## Run gofmt check, go vet and golangci-lint
	@test -z "$$(gofmt -l ./cmd ./internal)" || (echo "gofmt needed:"; gofmt -l ./cmd ./internal; exit 1)
	go vet ./...
	golangci-lint run

vuln: ## Scan dependencies for known vulnerabilities
	govulncheck ./...

# F1 final review pass B, I6: scripts/gen-env-docker.sh and
# scripts/offline-bundle.sh were committed as mode 100644 (not executable),
# which broke `make env-docker`/`make up`/`make migrate`/`make seed`/
# `make offline-bundle` — every one of them runs `./scripts/....sh` — on a
# fresh Linux clone, while the main checkout's DrvFs mount hid the problem
# by presenting every file as executable regardless of what git had
# recorded. shellcheck (already in CI) parses script CONTENTS; it has never
# looked at file MODES, and neither had anything else here.
#
# `git ls-files -s` prints the mode git has COMMITTED for each path (third
# field), independent of what the current working tree's filesystem
# reports, which is exactly why this check catches the bug DrvFs hides: a
# `chmod +x` on a live checkout never touches the committed mode, only
# `git update-index --chmod=+x <path>` does.
check-script-modes: ## Fail if a committed scripts/*.sh file lacks the executable bit in the git index
	@bad="$$(git ls-files -s scripts/*.sh | awk '$$1 != 100755 { print }')" || exit 1; \
	if [ -n "$$bad" ]; then \
		echo "scripts/*.sh committed without the executable bit (fix with: git update-index --chmod=+x <path>):"; \
		echo "$$bad"; \
		exit 1; \
	fi

ci: lint check-generate check-script-modes test build web-lint web-test web-build web-audit ## Everything CI runs, except integration tests, govulncheck and shellcheck

.PHONY: up down dev logs ps migrate seed generate check-generate offline-bundle env-docker

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

generate: ## Regenerate sqlc types from the migrations and internal/store/postgres/queries
	sqlc generate

# `git status --porcelain --untracked-files=all` rather than `git diff
# --exit-code`: diff compares TRACKED files only and is blind to a new,
# uncommitted one. Tasks 9-11 each add a query file, and each produces a new
# sqlcgen/<name>.sql.go — with a diff-only check, forgetting to commit one
# leaves this green and `go build` only notices once something calls the
# missing method. porcelain reports added, modified and untracked alike, and
# unlike `git add` + `git diff --cached` it does not mutate the caller's index,
# which matters now that `make ci` runs this.
#
# The `|| exit 1` is not decoration. An empty `changed` means "nothing to
# report", and git failing — not installed, not a checkout, a broken index —
# also produces an empty string, so without it this guard PASSES precisely when
# it cannot run. The `git diff --exit-code` it replaced failed loudly in that
# case, and losing that would have been a straight regression.
check-generate: ## Fail if the committed sqlcgen output is not what sqlc produces
	sqlc generate
	@changed="$$(git status --porcelain --untracked-files=all -- internal/store/postgres/sqlcgen)" || exit 1; \
	if [ -n "$$changed" ]; then \
		echo "sqlcgen is not current — run 'make generate' and commit the result:"; \
		echo "$$changed"; \
		exit 1; \
	fi

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
