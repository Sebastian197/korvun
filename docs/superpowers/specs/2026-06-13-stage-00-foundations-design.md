# Stage 0 — Project Foundations: Design Spec

> **Status:** approved
> **Date:** 2026-06-13

## Goal

Set up the repository structure, quality tooling, CI/CD pipeline, and project
conventions so that all subsequent stages build on a verified, automated
foundation. No business logic in this stage.

## Phase 0.1 — Structure and Module

**Deliverables:**

- `go.mod` with `module github.com/Sebastian197/korvun`, minimum Go 1.22
- Directory layout:
  ```
  cmd/korvun/main.go    — minimal entry point (package main, empty func main)
  internal/             — internal packages (empty for now)
  docs/adr/             — architecture decision records
  docs/stages/          — stage closure docs
  ```
- Every `.go` file starts with:
  ```go
  // Copyright 2026 Sebastián Moreno Saavedra
  // SPDX-License-Identifier: Apache-2.0
  ```
- **Verification:** `go build ./...` succeeds

## Phase 0.2 — Quality Tooling

**Deliverables:**

- `Makefile` with targets:
  - `build` — compile `cmd/korvun`
  - `test` — `go test -race ./...`
  - `lint` — `gofmt` check + `goimports` check + `golangci-lint run`
  - `cover` — test with coverage, enforce threshold
  - `quality` — runs `lint` + `test` + `cover` (the gate)
- `.golangci.yml` enabling: govet, staticcheck, errcheck, gosec
- **Verification:** `make quality` passes on the skeleton

## Phase 0.3 — CI/CD

**Deliverables:**

- `.github/workflows/quality.yml` — GitHub Actions workflow
- Reduced matrix (expand to 6 in Stage 11):
  - `linux/amd64`, `macos/amd64`, `windows/amd64`
- Pipeline steps: lint → vet → test -race → coverage → build → gosec → govulncheck
- **Verification:** pipeline passes green on all 3 platforms

## Phase 0.4 — Conventions

**Deliverables:**

- `docs/adr/TEMPLATE.md` — ADR template
- `docs/stages/TEMPLATE.md` — stage closure template
- `.github/PULL_REQUEST_TEMPLATE.md` — PR template
- Conventional Commits documented and enforced

## Closure

- `docs/stages/STAGE-00.md` documents everything done
- Master document updated if any new decisions were made

## Decisions

- **Reduced CI matrix:** 3 platforms (amd64 only) until Stage 11. Rationale:
  no platform-specific code yet; saves CI minutes.
- **golangci-lint v1.64.8:** installed via `go install`, located at `~/go/bin/`.
  Makefile uses `$(shell go env GOPATH)/bin/golangci-lint` to avoid PATH issues.