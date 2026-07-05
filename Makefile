GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint
GOIMPORTS := $(GOBIN)/goimports
COVERAGE_THRESHOLD := 85

.PHONY: build test lint cover quality fmt vet frontend-install frontend-build

# The builder frontend's node_modules vendors a stray Go package (flatted), which
# `./...` would otherwise pick up. Exclude it from Go tooling. FOLLOW-UP (by
# construction): a nested go.mod in web/builder would make root ./... skip it
# without this filter — see the 2b.1 report.
GO_PKGS := $(shell go list ./... 2>/dev/null | grep -v '/web/builder/node_modules/')

# The builder frontend (web/builder) is built to web/builder/dist and embedded via
# go:embed (ADR-0029 §4). `build` (the shipped binary) rebuilds it FIRST so the
# binary carries the real UI. `quality` and `test` do NOT depend on it: they use the
# committed dist placeholder, so the Go pipeline never needs Node (ADR-0029 §4/§6 —
# Node never gates the Go build/cross-compile/release).
frontend-install:
	cd web/builder && npm ci

frontend-build:
	cd web/builder && npm run build

build: frontend-build
	go build ./cmd/korvun

test:
	go test -race $(GO_PKGS)

vet:
	go vet $(GO_PKGS)

fmt:
	@echo "Checking gofmt..."
	@test -z "$$(gofmt -l .)" || { echo "Files need gofmt:"; gofmt -l .; exit 1; }
	@echo "Checking goimports..."
	@test -z "$$($(GOIMPORTS) -l .)" || { echo "Files need goimports:"; $(GOIMPORTS) -l .; exit 1; }

lint: fmt vet
	$(GOLANGCI_LINT) run ./...

cover:
	@go test -race -coverprofile=coverage.out ./internal/... 2>&1 | tee /dev/stderr | grep -q 'ok' && \
	grep -q '^mode:' coverage.out 2>/dev/null && \
	grep -v '^mode:' coverage.out | grep -q '.' 2>/dev/null && \
	{ \
		total=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | tr -d '%'); \
		echo "Coverage: $${total}%"; \
		if [ "$$(echo "$${total} < $(COVERAGE_THRESHOLD)" | bc)" -eq 1 ]; then \
			echo "FAIL: coverage $${total}% < $(COVERAGE_THRESHOLD)% threshold"; \
			exit 1; \
		fi; \
	} || echo "No testable packages yet — skipping coverage threshold"
# Note: coverage scope is intentionally internal/... only — the cmd/
# packages today are temporary live-skeleton CLIs (cmd/demo-model,
# cmd/demo-groq) that are exercised manually against real backends,
# not via go test. Lint, vet and test still cover ./... above; only
# the coverage threshold excludes cmd/.

quality: lint test cover
	@echo "Quality gate passed."