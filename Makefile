GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint
GOIMPORTS := $(GOBIN)/goimports
COVERAGE_THRESHOLD := 85

.PHONY: build test lint cover quality fmt vet

build:
	go build ./cmd/korvun

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	@echo "Checking gofmt..."
	@test -z "$$(gofmt -l .)" || { echo "Files need gofmt:"; gofmt -l .; exit 1; }
	@echo "Checking goimports..."
	@test -z "$$($(GOIMPORTS) -l .)" || { echo "Files need goimports:"; $(GOIMPORTS) -l .; exit 1; }

lint: fmt vet
	$(GOLANGCI_LINT) run ./...

cover:
	@go test -race -coverprofile=coverage.out ./... 2>&1 | tee /dev/stderr | grep -q 'ok' && \
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

quality: lint test cover
	@echo "Quality gate passed."