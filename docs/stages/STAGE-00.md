# Stage 00 — Project Foundations

> **Status:** closed
> **Started:** 2026-06-13
> **Closed:** 2026-06-13

## Objective

Set up the repository structure, quality tooling, CI/CD pipeline, and project
conventions before writing any business logic.

## Phases Completed

| Phase | Description                | Status |
|-------|----------------------------|--------|
| 0.1   | Structure and Go module    | done   |
| 0.2   | Quality tooling            | done   |
| 0.3   | CI/CD (GitHub Actions)     | done   |
| 0.4   | Conventions and templates  | done   |

## Deliverables

### Phase 0.1
- `go.mod` — `module github.com/Sebastian197/korvun`, Go 1.22+
- `cmd/korvun/main.go` — minimal entry point
- Directory layout: `cmd/`, `internal/`, `docs/adr/`, `docs/stages/`
- Copyright + SPDX header on all `.go` files

### Phase 0.2
- `Makefile` with targets: `build`, `test`, `lint`, `cover`, `quality`, `fmt`, `vet`
- `.golangci.yml` — govet, staticcheck, errcheck, gosec enabled
- `golangci-lint` v1.64.8, `goimports` installed

### Phase 0.3
- `.github/workflows/quality.yml` — GitHub Actions workflow
- Reduced matrix: `linux/amd64`, `macos/amd64`, `windows/amd64`
- Pipeline: build → lint → test -race → coverage → gosec → govulncheck
- Full 6-target matrix deferred to Stage 11

### Phase 0.4
- `docs/adr/TEMPLATE.md` — ADR template
- `docs/stages/TEMPLATE.md` — stage closure template
- `.github/PULL_REQUEST_TEMPLATE.md` — PR checklist

## Key Decisions

- **Reduced CI matrix (3 platforms, amd64 only):** no platform-specific code
  yet; saves CI minutes. Expand in Stage 11.
- **golangci-lint via `go install`:** avoids external package managers; resolved
  via `$(go env GOPATH)/bin/` in Makefile and CI.

## Quality Gate

- `make quality`: pass (no test files yet — coverage threshold skipped)
- CI: configured for 3 platforms

## Notes

- `goimports` and `golangci-lint` are not in the default PATH on this machine;
  Makefile and CI use full `GOPATH/bin` paths.