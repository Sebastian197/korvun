# ADR-0036: Dependency — `wailsapp/wails/v2` (desktop shell framework)

> **Status:** accepted
> **Date:** 2026-07-19
> **Deciders:** Sebastián Moreno Saavedra
>
> **Accepted 2026-07-19, copilot review passed.**
>
> Companion architecture ADR: ADR-0035 (desktop app). `go.mod` is NOT touched
> by this ADR — the dependency lands with the first TDD sub-phase (`go get` at
> that point), the ADR-0034 precedent.

## Context

ADR-0035 fixes the desktop app's shape: a sibling binary `cmd/korvun-desktop`
with the core in-process and the existing builder in a native WebView window.
That shell needs a desktop framework: window management, the platform WebView
(WebView2 / WKWebView / WebKitGTK), JS↔Go bindings, asset serving, and per-OS
packaging glue. **None of this is remotely stdlib territory.**

This would be the **5th direct dependency** (`go.mod` goes from 4 to 5:
telegram-bot, sqlite, prometheus, coder/websocket, wails). The dependency
discipline (CLAUDE.md: ADR + Context7 verification + the four-axis test)
governs the call. Everything below was verified 2026-07-19 against Context7
(`/websites/wails_io`, `/websites/v3_wails_io`) and primary sources
(`github.com/wailsapp/wails/releases`, the tagged `go.mod`) — not from memory.

## Decision

Adopt **`github.com/wailsapp/wails/v2`**, pinned at **`v2.13.0`** (latest
stable, July 2026), imported **only by `cmd/korvun-desktop` and the thin Wails
adapter over the framework-free `shell` package** (ADR-0035 §3a: the lifecycle
logic itself never imports Wails). Explicitly NOT v3: `v3.0.0-alpha2.117` is
pre-release with no announced stable date.

**Verified facts the decision rests on:**

- **v2.13.0 is the current stable release** (July 2026, active maintenance);
  v3 remains alpha with daily pre-releases (source: releases page, v3 status
  page).
- **Go compatibility — the §10 framing gate, now closed:** the tagged
  `go.mod` of `wails/v2@v2.13.0` declares **`go 1.25.0`**; the repo is on
  **Go 1.26.5**, which satisfies it (Go toolchains build modules declaring
  older directives). Verified at source
  (`raw.githubusercontent.com/wailsapp/wails/v2.13.0/v2/go.mod`), not assumed.
- **Platforms:** Windows 10/11 (AMD64/ARM64), macOS 10.15+/11+ with
  `darwin/universal`, Linux (AMD64/ARM64) — covers the ADR-0035 v1 matrix.
- **Runtime model:** system WebViews (no bundled Chromium); Linux desktop
  builds require cgo + GTK3/WebKitGTK; Windows needs the WebView2 runtime
  (installable via built-in strategies).
- **Dependency tree (honest cost):** the tagged `go.mod` of
  `wails/v2@v2.13.0` lists **42 direct + 63 indirect = 105 modules** (source:
  `raw.githubusercontent.com/wailsapp/wails/v2.13.0/v2/go.mod`). A large share
  serves the **CLI/build tooling** (go-git, pterm, glamour) rather than the
  runtime library, but the module graph and `go.sum` grow regardless.
- **Frontend-agnostic assets:** any static bundle via `embed.FS` — the
  existing `web/builder/dist` embeds unchanged, and the AssetServer handler
  seam exists for the ADR-0035 §3(b) proxy (its SSE flush behavior is the
  first sub-phase's verification gate).

### Four-axis dependency test (capability vs hand-roll cost vs maintenance vs risk/volatility)

| Axis | Verdict |
|------|---------|
| **Capability gain** | Native window + platform WebView integration + JS↔Go bindings + asset server + packaging glue (`.app`, NSIS) on 3 OSes with `darwin/universal`. This is an entire platform-integration layer per OS — capability Korvun cannot get from stdlib at any price. |
| **Hand-roll cost** | **Prohibitive and misplaced.** Hand-rolling means per-OS cgo against WebView2 COM, WKWebView/Cocoa, and WebKitGTK, plus a bindings bridge and packaging — years of platform code that is not Korvun's value (the policy engine is). Even the minimal `webview/webview` route leaves bindings, assets, menus, and packaging to build by hand. |
| **Maintenance / cross-compile** | **The weak axis, consciously accepted and bounded.** cgo + native toolchains break the beloved ×6 cross-compile — for THIS binary only: desktop builds move to native runners in their own workflow, and the headless `CGO_ENABLED=0` ×6 pipeline is untouched because the headless binary never imports Wails (Go links only packages transitively imported by each `main` — no Wails code can enter `cmd/korvun`). **MVS caveat (honest):** wails/v2 requires modules the headless binary already links (`golang.org/x/sys`, `google/uuid` — today at identical versions); a future wails bump that raises a shared minimum WOULD change the headless binary's bytes, so the `go get` sub-phase must diff `go version -m` on the headless artifact before/after. The 105-module tree is the real ongoing cost: a larger audit/SBOM surface, mitigated by the CLI-vs-runtime split and by confinement to one leaf binary. |
| **Risk / volatility** | Moderate-low. v2 is the stable, maintained line of the segment's dominant Go framework; the version is pinned (v2.13.x); the v2→v3 migration is documented by upstream, so the eventual v3-stable transition is a bounded, known chore (ADR-0035 R1: no mid-flight re-decision). The seam bounds blast radius: only `cmd/korvun-desktop` + the thin `shell` adapter import it; the lifecycle logic is plain Go testable without Wails (ADR-0035 §3a). |

**Net:** the capability is unobtainable by hand at sane cost, the toolchain
burden is confined to one new leaf binary with its own CI lane, and the risk
is pinned and seam-bounded. The gate passes — with the dependency-tree growth
named as the honest price, not hidden.

**Honest gap (mirrors ADR-0034):** Context7 verified the capability and
platform claims, NOT the specific v2 API signatures Korvun will call (app
options, bindings, AssetServer handler). That exact-surface pass gates the
first TDD sub-phase — no code is written against remembered signatures.

## Consequences

- `go.mod` goes from 4 to 5 direct dependencies; `go.sum` grows substantially
  (wails' 105-module graph). No Wails code can link into the headless
  artifact, but shared transitive minimums may move under MVS (see the
  maintenance axis) and the repo's dependency audit surface grows.
- Desktop builds require native runners (macOS/Windows/Linux) and platform
  packages (GTK/WebKitGTK on Linux CI); a new, separate workflow. The
  existing release pipeline is untouched.
- The first TDD sub-phase must (a) `go get` the pinned version, (b) run the
  Context7 pass over the exact v2 API surface used (app options, bindings,
  AssetServer handler), and (c) verify the AssetServer SSE flush gate of
  ADR-0035 §3(b) before the proxy decision is final.
- Reversible in the architectural sense: the shell adapter is thin and the
  lifecycle logic framework-free, so swapping frameworks (v3, or Tauri in an
  extreme scenario) rewrites the adapter, not the piece.

## Alternatives Considered

- **Wails v3 (`v3.0.0-alpha2.x`)** — rejected for now: pre-release, no stable
  date; the maturity half of the risk axis fails outright. Better packaging
  and API arrive with the documented v2→v3 migration once stable.
- **Tauri v2 + sidecar** — rejected (ADR-0035): stable and well-tooled, but a
  Rust toolchain and second language for a one-maintainer Go project, and the
  sidecar duplicates artifacts.
- **`webview/webview` (minimal cgo webview)** — rejected: lighter tree, but
  provides only the window+webview; bindings, asset serving, packaging, and
  per-OS glue would be hand-rolled — the hand-roll axis returns through the
  back door.
- **Electron** — rejected: Chromium+Node bundle per app; maximal footprint,
  antithetical to Korvun's single-small-binary identity.
- **Hand-rolled per-OS shells** — rejected on the four-axis table above.
