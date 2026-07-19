# Dependabot triage — 2026-07-19 (the wails/v2 tree's 14 alerts)

> **Scope:** the 14 open Dependabot alerts that arrived with the `wails/v2`
> dependency (SP1, commit `d1c5f20`) — the ADR-0036 maintenance-axis audit
> cost materializing. Evidence-based triage, not manifest-level guessing:
> per-artifact linkage via `go version -m` on freshly built binaries, plus
> symbol-level reachability via govulncheck v1.6.0 (Go 1.26.5). **No bumps
> performed in this note** — the one recommended bump is reported for the
> copilot's call, because any shared-module bump re-triggers the SP1 headless
> `go version -m` diff discipline. Dismissals in the GitHub UI are Chano's
> clicks, guided by the table.

## The two modules behind all 14 alerts

| Module | In `go.mod` today | Vulnerable range | Fixed in | Why it is in the graph |
|---|---|---|---|---|
| `golang.org/x/crypto` | v0.51.0 (indirect) | < 0.52.0 | 0.52.0 | wails CLI tooling (go-git → ssh); **no Korvun binary links it** |
| `golang.org/x/net` | v0.54.0 (indirect) | < 0.55.0 | 0.55.0 | wails **runtime**; links in `korvun-desktop` only |

## Evidence (reproduced 2026-07-19)

- **Headless `cmd/korvun`** (plain `go build`): `go version -m` lists
  **neither** `x/crypto` nor `x/net` → none of the 14 alerts touches the
  headless artifact. (Consistent with the SP1 Gate 1 identical-diff.)
- **Desktop `cmd/korvun-desktop`** (`-tags desktop,production`,
  macOS-13 `CGO_LDFLAGS`): links **`x/net v0.54.0`**; does NOT link
  `x/crypto`.
- **govulncheck `./...`** (no tags — the CI scope): `No vulnerabilities
  found.`
- **govulncheck `-tags desktop,production ./cmd/korvun-desktop`:** the
  desktop code IS affected by **5 x/net vulnerabilities** (GO-2026-5025,
  -5027, -5028, -5029, -5030; Dependabot groups them under alert #1), all
  fixed in `x/net@v0.55.0`, reachable as
  `korvun-desktop main → wails.Run → … → html.Parse`. The x/crypto module
  is reported as required-but-never-called.

## Verdict per alert

| # | Module | Severity | GHSA | Links in | Verdict | Evidence |
|---|---|---|---|---|---|---|
| 1 | golang.org/x/net | medium | GHSA-5cv4-jp36-h3mw | desktop ONLY (runtime, reachable) | **ACTUALIZAR** → `x/net v0.55.0` | `go version -m` desktop shows v0.54.0; govulncheck traces `wails.Run → html.Parse` (5 Go IDs, all fixed in 0.55.0) |
| 2 | golang.org/x/crypto | high | GHSA-q4h4-gmj2-qvw2 | nowhere (module graph only) | **IGNORABLE** | absent from both binaries' `go version -m`; govulncheck: not called |
| 3 | golang.org/x/crypto | medium | GHSA-45gg-vh54-h5m9 | nowhere | **IGNORABLE** | same |
| 4 | golang.org/x/crypto | medium | GHSA-78mq-xcr3-xm33 | nowhere | **IGNORABLE** | same |
| 5 | golang.org/x/crypto | medium | GHSA-qpw4-5x99-6vjp | nowhere | **IGNORABLE** | same |
| 6 | golang.org/x/crypto | critical | GHSA-vgwf-h737-ff37 | nowhere | **IGNORABLE** | same |
| 7 | golang.org/x/crypto | high | GHSA-w879-237q-wc7r | nowhere | **IGNORABLE** | same |
| 8 | golang.org/x/crypto | critical | GHSA-89gr-r52h-f8rx | nowhere | **IGNORABLE** | same |
| 9 | golang.org/x/crypto | critical | GHSA-rm3j-f69w-wqmq | nowhere | **IGNORABLE** | same |
| 10 | golang.org/x/crypto | critical | GHSA-5cgq-3rg8-m6cv | nowhere | **IGNORABLE** | same |
| 11 | golang.org/x/crypto | critical | GHSA-x527-x647-q7gg | nowhere | **IGNORABLE** | same |
| 12 | golang.org/x/crypto | critical | GHSA-jppx-rxg9-jmrx | nowhere | **IGNORABLE** | same |
| 13 | golang.org/x/crypto | critical | GHSA-f5wc-c3c7-36mc | nowhere | **IGNORABLE** | same |
| 14 | golang.org/x/crypto | medium | GHSA-9m57-25v3-79x9 | nowhere | **IGNORABLE** | same |

**Totals: 13 IGNORABLE · 1 ACTUALIZAR · 0 VIGILAR.**

## Recommended actions (for the copilot / Chano)

1. **Alert #1 — bump `golang.org/x/net` to `v0.55.0`** (a one-line
   `go get golang.org/x/net@v0.55.0 && go mod tidy`). NOT done in this
   commit by discipline: a shared-module bump requires re-running the SP1
   headless `go version -m` diff (expected outcome: identical — the headless
   binary does not link x/net — but the check is the rule, not the
   expectation). Natural slot: the first SP3 commit.
2. **Alerts #2–#14 — dismiss in the GitHub UI** with reason "vulnerable code
   is not actually used": x/crypto sits only in wails' CLI-tooling module
   graph (go-git → ssh); no Korvun artifact links it, confirmed by both
   binaries' `go version -m` and govulncheck. Re-check automatically holds:
   any future import that DID pull x/crypto into a binary would resurface in
   govulncheck.
3. **Standing note:** the desktop lane should eventually run
   `govulncheck -tags desktop,production ./cmd/korvun-desktop` in CI (the
   untagged CI run cannot see desktop reachability) — candidate for the
   packaging sub-phase's workflow.
