GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint
GOIMPORTS := $(GOBIN)/goimports
COVERAGE_THRESHOLD := 85

.PHONY: build test lint cover quality fmt vet frontend-install frontend-build desktop-frontend-install desktop-frontend desktop dmg

# The builder frontend's node_modules vendors a stray Go package (flatted), which
# `./...` would otherwise pick up. Exclude it from Go tooling. FOLLOW-UP (by
# construction): a nested go.mod in web/builder would make root ./... skip it
# without this filter — see the 2b.1 report.
GO_PKGS := $(shell go list ./... 2>/dev/null | grep -v '/node_modules/')

# The builder frontend (web/builder) is built to web/builder/dist and embedded via
# go:embed (ADR-0029 §4). `build` (the shipped binary) rebuilds it FIRST so the
# binary carries the real UI. `quality` and `test` do NOT depend on it: they use the
# committed dist placeholder, so the Go pipeline never needs Node (ADR-0029 §4/§6 —
# Node never gates the Go build/cross-compile/release).
frontend-install:
	cd web/builder && npm ci

frontend-build:
	cd web/builder && npm run build

# The desktop chrome (cmd/korvun-desktop/frontend, SP6) builds to its own
# dist/ and is embedded by the DESKTOP binary only (//go:embed
# all:frontend/dist, ADR-0029 §4 stub pattern). Never part of `build` or
# `quality` — the headless pipeline stays Node-free.
desktop-frontend-install:
	cd cmd/korvun-desktop/frontend && npm ci

desktop-frontend:
	cd cmd/korvun-desktop/frontend && npm run build

build: frontend-build
	go build ./cmd/korvun

# --- SP7: desktop packaging (local reproducible recipe, cut 7a) ------------------
# The deterministic recipe (spec §1b), law from the private-pass re-bake lesson:
# ALWAYS rebuild BOTH frontends fresh, then the binary — never trust a cached or
# working-tree dist. Wails runs in cmd/korvun-desktop (where main.go + wails.json
# + build/ live); output lands in cmd/korvun-desktop/build/bin (gitignored).
WAILS := $(GOBIN)/wails
# Honest local version stamped into main.version via ldflags (goodbye "dev").
# git describe --tags --always --dirty; 7b passes the exact clean tag.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# CFBundleShortVersionString / the installer resource want a clean X.Y.Z, not a
# git-describe string — take the NEAREST tag, v-stripped (spec §1h: productVersion
# from the tag). Stamped into wails.json for the build, then restored.
PRODUCT_VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo 0.0.0)
# Artifact FILENAMES follow the headless GoReleaser scheme, which stamps the
# version WITHOUT the leading "v" (korvun_0.3.0_… precedent) — while the binary
# stamp keeps it (`korvun vX.Y.Z`). Strip it once here so both artifact families
# name the version identically inside the same release (NC-2).
ARTIFACT_VERSION := $(patsubst v%,%,$(VERSION))
# Per-OS lane. Darwin: macOS 13 / Xcode CLT need UTType made explicit (SP1
# empirical); the ×6 headless lane is unaffected (it never imports Wails).
# Windows (7b): -nsis emits the installer, -webview2 download is the ADR-0035 R4
# bootstrap strategy; GNU make there is the Chocolatey install driven from Git
# Bash, so pin SHELL to bash to keep the trap/function recipe identical.
# Linux (7b): ubuntu-24.04 ships only webkit2gtk-4.1 — wails v2.13.0 gates its
# pkg-config lines on the `webkit2_41` build tag (source-verified:
# internal/frontend/desktop/linux/*.go `#cgo webkit2_41 pkg-config:
# webkit2gtk-4.1`), so the tag rides DESKTOP_TAGS on Linux only.
DESKTOP_TAGS := desktop,production
DESKTOP_WAILS_EXTRA :=
ifeq ($(OS),Windows_NT)
SHELL := bash
DESKTOP_PLATFORM ?= windows/amd64
DESKTOP_CGO_LDFLAGS :=
DESKTOP_WAILS_EXTRA := -nsis -webview2 download
else ifeq ($(shell uname -s),Darwin)
DESKTOP_PLATFORM ?= darwin/universal
DESKTOP_CGO_LDFLAGS := -framework UniformTypeIdentifiers
else
DESKTOP_PLATFORM ?= linux/amd64
DESKTOP_CGO_LDFLAGS :=
DESKTOP_TAGS := desktop,production,webkit2_41
endif

# One recipe shell with a `trap ... EXIT` so the working-tree restore ALWAYS
# runs — even if a frontend build or `wails build` fails midway. Otherwise a
# partial failure would leave the freshly-built (gitignored) web/builder/dist on
# disk, and the NEXT headless `go build ./cmd/korvun` would silently go:embed it
# instead of the committed stub, breaking the headless byte-identity that is
# SP7's engraved law (review 7a #1). The trap restores both dist stubs + the
# tag-stamped wails.json and drops the ignored build outputs.
desktop:
	@set -e; \
	wjbak=$$(mktemp); cp cmd/korvun-desktop/wails.json "$$wjbak"; \
	restore() { \
		git checkout -- web/builder/dist cmd/korvun-desktop/frontend/dist; \
		git clean -fdX web/builder/dist cmd/korvun-desktop/frontend/dist >/dev/null; \
		cp "$$wjbak" cmd/korvun-desktop/wails.json; rm -f "$$wjbak"; \
	}; \
	trap restore EXIT; \
	echo "[desktop] (a) builder frontend — fresh (ADR-0029 §6)"; \
	( cd web/builder && npm ci && npm run build ); \
	echo "[desktop] (b) chrome frontend — fresh"; \
	( cd cmd/korvun-desktop/frontend && npm ci && npm run build ); \
	echo "[desktop] stamp wails.json productVersion=$(PRODUCT_VERSION)"; \
	perl -0pi -e 's/"productVersion":\s*"[^"]*"/"productVersion": "$(PRODUCT_VERSION)"/' cmd/korvun-desktop/wails.json; \
	echo "[desktop] (c) wails build — $(DESKTOP_PLATFORM), -s (own go:embed), -skipbindings, -clean"; \
	( cd cmd/korvun-desktop && CGO_LDFLAGS="$(DESKTOP_CGO_LDFLAGS)" $(WAILS) build \
		-s -skipbindings -clean -platform $(DESKTOP_PLATFORM) $(DESKTOP_WAILS_EXTRA) \
		-tags $(DESKTOP_TAGS) -ldflags "-X main.version=$(VERSION)" ); \
	echo "Built cmd/korvun-desktop/build/bin for $(DESKTOP_PLATFORM) ($(VERSION), bundle $(PRODUCT_VERSION))"

# Wrap the universal .app into a distributable .dmg (macOS built-in hdiutil,
# zero external tool). Named to the NC-2 scheme. `dmg` depends on `desktop`, so
# run `make dmg` alone — `make desktop dmg` would rebuild everything twice.
dmg: desktop
	hdiutil create -volname Korvun \
		-srcfolder cmd/korvun-desktop/build/bin/Korvun.app \
		-ov -format UDZO \
		cmd/korvun-desktop/build/bin/korvun-desktop_$(ARTIFACT_VERSION)_darwin_universal.dmg
	@echo "Wrapped cmd/korvun-desktop/build/bin/korvun-desktop_$(ARTIFACT_VERSION)_darwin_universal.dmg"

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