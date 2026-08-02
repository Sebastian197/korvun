GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint
GOIMPORTS := $(GOBIN)/goimports
COVERAGE_THRESHOLD := 85

.PHONY: build test lint cover quality fmt vet guard-gopkgs frontend-install frontend-build desktop-frontend-install desktop-frontend desktop dmg website-check

# Go tooling must NEVER walk node_modules. npm packages ship stray .go files
# (flatted), and desktop sync tools mint junk like "x 2.go" in there — ONE such
# file aborts `go list ./...` ENTIRELY (empty list → every tool silently runs
# on nothing; the 2026-08-01 incident). Nested go.mod files are NOT an option:
# both UIs ride //go:embed from the ROOT module (web/builder and the desktop
# chrome, ADR-0029 §4), and a module boundary would break those embeds. So the
# package DIRS are discovered by find with node_modules PRUNED (go never
# enters it), handed to go list; the grep stays as a second belt.
GO_PKG_DIRS := $(shell find . \( -name node_modules -o -name .git \) -prune -o -type f -name '*.go' -print 2>/dev/null | sed 's|/[^/]*$$||' | sort -u)
# -e + the Error filter replicates `./...` semantics over explicit dirs: a
# package whose files are ALL excluded by build tags (the wails desktop main)
# is skipped silently instead of erroring the whole listing.
GO_PKGS := $(shell go list -e -f '{{if not .Error}}{{.ImportPath}}{{end}}' $(GO_PKG_DIRS) 2>/dev/null | grep -v '^$$' | grep -v '/node_modules/')
# golangci-lint takes DIRECTORY paths, not import paths — derive them from
# GO_PKGS (so the tag-excluded packages are filtered identically).
GO_LINT_DIRS := $(patsubst github.com/Sebastian197/korvun%,.%,$(GO_PKGS))

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
# Forward-slash the path: on Windows `go env GOPATH` yields C:\Users\…, and make
# pastes $(WAILS) into the recipe TEXT, where the shell then eats the backslashes
# as escapes ("C:Usersrunneradmingo/bin/wails", dry-run #30194244673). C:/… is
# valid for both Git Bash and the Windows API; no-op on Darwin/Linux.
WAILS := $(subst \,/,$(GOBIN))/wails
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
	@test -z "$$(gofmt -l $(GO_PKG_DIRS))" || { echo "Files need gofmt:"; gofmt -l $(GO_PKG_DIRS); exit 1; }
	@echo "Checking goimports..."
	@test -z "$$($(GOIMPORTS) -l $(GO_PKG_DIRS))" || { echo "Files need goimports:"; $(GOIMPORTS) -l $(GO_PKG_DIRS); exit 1; }

lint: fmt vet
	$(GOLANGCI_LINT) run $(GO_LINT_DIRS)

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

# THE GUARD: a relapse must be an IMMEDIATE, explained red — never
# silently-empty tooling. Raw `go list ./...` can never be required clean
# (flatted legitimately ships Go files inside node_modules); what MUST hold is
# that the EFFECTIVE package list is non-empty and node_modules-free.
guard-gopkgs:
	@test -n "$(strip $(GO_PKGS))" || { echo "FAIL: GO_PKGS is EMPTY — go list aborted (node_modules junk again? see the header of this Makefile); Go tooling would silently run on NOTHING"; exit 1; }
	@case "$(GO_PKGS)" in *node_modules*) echo "FAIL: GO_PKGS contains node_modules — the exclusion regressed"; exit 1;; esac
	@echo "GO_PKGS guard: $(words $(GO_PKGS)) packages, node_modules-free."

quality: guard-gopkgs lint test cover
	@echo "Quality gate passed."

# --- Web track SP1: the site check harness (spec AS-1 + AS-9, ADR-0040) ----------
# The public website (website/) is build-time only and Pages-published; it NEVER
# gates the Go pipeline (`quality` does not depend on this target — the ADR-0029
# §6 rule). This harness is what the SP5 pages.yml build job will run:
# (1) npm ci TWICE with a lockfile compare — the CLAUDE.md determinism rule (a
#     macOS `npm install` silently omitting the optional-of-optional WASM subtree
#     is exactly what this catches BEFORE Linux CI does);
# (2) a production build under base '/korvun/' (the project-page subdirectory);
# (3) link + asset integrity over the built dist — a root-absolute path outside
#     the base is the AS-1 violation that 404s on sebastian197.github.io/korvun/.
website-check:
	@set -e; \
	cd website; \
	echo "[website-check] (1/3) npm ci x2 — lockfile determinism (AS-9)"; \
	lock1=$$(mktemp); cp package-lock.json "$$lock1"; \
	npm ci; \
	rm -rf node_modules; \
	npm ci; \
	cmp -s package-lock.json "$$lock1" || { rm -f "$$lock1"; echo "FAIL: package-lock.json drifted across npm ci runs"; exit 1; }; \
	rm -f "$$lock1"; \
	echo "[website-check] (2/3) vitepress build under base '/korvun/' (AS-1)"; \
	npm run build; \
	echo "[website-check] (3/3) link + asset integrity over dist (AS-1)"; \
	node scripts/check-dist.mjs; \
	echo "website-check passed."