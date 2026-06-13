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
- Quality job matrix: `ubuntu-latest`, `macos-latest`, `windows-latest`
- Quality pipeline: build → lint → test -race → coverage gate → gosec → govulncheck
- `cross-compile` job: full 6-target matrix `{linux,windows,darwin}×{amd64,arm64}`
- `codeql` job: GitHub CodeQL SAST analysis on Go sources
- `sbom` job: SPDX-JSON SBOM generated via `anchore/sbom-action` (pinned by SHA),
  uploaded as workflow artifact
- Coverage gate: pipeline fails if `internal/` total < 85%, or if any of
  `envelope`, `policy`, `router`, `brain` falls below 90%
  (skips packages that do not yet exist)
- `.github/dependabot.yml` — weekly grouped updates for `gomod` and
  `github-actions` ecosystems, 5-PR cap per ecosystem

### Phase 0.4
- `docs/adr/TEMPLATE.md` — ADR template
- `docs/stages/TEMPLATE.md` — stage closure template
- `.github/PULL_REQUEST_TEMPLATE.md` — PR checklist

## Key Decisions

- **Quality matrix runs on 3 OS / amd64 runners:** GitHub-hosted runners are
  amd64 by default. ARM64 coverage is provided in CI via the dedicated
  cross-compile job (no execution, just `go build`), which is the standard
  practice for Go projects without CGO.
- **Cross-compile in its own job:** keeps the quality matrix small while
  guaranteeing every supported `(GOOS, GOARCH)` target still links cleanly.
- **SBOM action pinned by full commit SHA:** the upstream tag is pre-1.0
  (`v0.x`), so a SHA pin is preferred over a floating major tag for
  supply-chain safety. Other Actions are pinned to a fixed major tag.
- **Coverage gate skips not-yet-existing packages:** lets the same gate
  flow through future stages without breaking before the corresponding
  packages land.
- **golangci-lint via `go install`:** avoids external package managers;
  resolved via `$(go env GOPATH)/bin/` in Makefile and CI.

## Quality Gate

- `make quality`: pass
- CI: quality job on 3 platforms + cross-compile on 6 targets +
  CodeQL + SBOM artifact + coverage gate
- Coverage gate verified locally against Stage 1 (`envelope` at 97.8%)

## Notes

- `goimports` and `golangci-lint` are not in the default PATH on this machine;
  Makefile and CI use full `GOPATH/bin` paths.
- Action version verification follows the rule in `CLAUDE.md`: tags verified
  at the Action's own repo/releases page, not from memory.