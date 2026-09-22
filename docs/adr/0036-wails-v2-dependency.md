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

> **AMENDED 2026-09-19 (Wails CLI train).** The pin is no longer `v2.13.0`.
> Dependabot's go-dependencies group (#34, merged as `45d046b`) moved the
> library to **`v2.15.0`**, and this train moves the build-time CLI that
> `release-desktop.yml` installs to the same `v2.15.0`, so the library and
> the CLI name one version again. Re-verified at source for `v2.15.0`: the
> tag resolves to `713dc89694c789ee0a45355c2886cef1986e9a01`; its `go.mod`
> still declares `go 1.25.0` (the repo is on Go 1.26.6). The library code
> that moved between the two pins is two upstream changes: `v2.14.0` fixes a
> nil-pointer crash and a swallowed network error in
> `v2/internal/webview2runtime/webview2runtime.go` when the WebView2
> bootstrapper download fails, a path the Windows desktop binary reaches
> through `internal/wv2installer`'s default download strategy (derived from
> the build tags, not executed); `v2.15.0` adds the opt-in macOS option
> `EnableAutoplayWithoutUserAction`, default unchanged. Under `v2/cmd/wails`
> only `internal/version.txt` differs, and
> `internal/system/packagemanager/apt.go` is byte-identical to `v2.13.0`'s.
> The MVS caveat in the maintenance axis was measured, not assumed:
> `v2.15.0`'s `go.mod` requires 28 modules that ours also requires, and for
> each of the 28 its version is at or below ours (`golang.org/x/sys`
> v0.46.0 against v0.48.0, `golang.org/x/net` v0.56.0 against v0.57.0,
> `github.com/google/uuid` v1.6.0 against v1.6.0), so the bump raised no
> shared minimum. The headless binary's `go version -m` did change between
> `ab9370e` and `3652601`: nine module versions moved and
> `go.yaml.in/yaml/v2` left the list, each of the ten a module that #34's
> own `go.mod` diff changes or drops, and no Wails module appears in that
> list. The historical facts of this ADR (v2.13.0 as the July stable, the
> `v2.13.x` pin in the risk axis, its 105-module count) are left as they
> were verified then.

> **AMENDED 2026-09-22 (the pin guard train).** The pin is no longer
> `v2.15.0`. Dependabot's go-dependencies group (#56, merged as `cf05372`)
> moved the library to **`v2.16.0`**, and this train moves the build-time CLI
> to the same `v2.16.0`. Re-verified at source for `v2.16.0`: the annotated
> tag `84940621c87fc4c95dda612cf87ed572df0fcf32` resolves to commit
> `457bf210d5eb976ff3726c8629f7725575163307`, tagged 2026-09-14; its `go.mod`
> still declares `go 1.25.0` (the repo is on Go 1.26.6). What moved under
> `v2/` was established from the two trees rather than from the release's file
> list, which the compare API truncates: both hold 1036 blobs, neither listing
> truncated, and **exactly three differ** — `cmd/wails/internal/version.txt`
> and the `calloc.go` of the darwin and linux desktop frontends. Those two are
> one fix. `Calloc` carried **value receivers**, so `String()` appended the new
> pointer to a copy's pool, the caller's pool stayed empty and `Free()`
> released nothing: every C string allocated **through `Calloc.String`** leaked.
> The scope is that method and no wider — the frontends allocate plenty of C
> strings that never touch `Calloc` and free them explicitly (`Run`, `ExecJS`
> and `SetTitle` in `darwin/window.go` each `C.CString` and `C.free`), and
> `v2.16.0` does not touch those. It changes both `Calloc` methods to pointer
> receivers. That closes an
> unbounded-growth path in the code the published macOS and Linux desktop
> artifacts link against; Windows is untouched. The MVS caveat needs no
> measurement this time: `v2/go.mod` is byte-identical between the two tags, so
> no shared minimum can have moved. `internal/system/packagemanager/apt.go` is
> likewise unchanged, and the packages `release-desktop.yml` names were read
> again at `v2.16.0` — `libgtk-3-dev`, `libwebkit2gtk`, `build-essential`,
> `pkg-config` — as was the `webkit2_41` cgo gating that `Makefile`'s Linux
> comment cites, still present in eight files of
> `internal/frontend/desktop/linux` (`clipboard.go`, `frontend.go`, `gtk.go`,
> `keys.go`, `menu.go`, `screen.go`, `webkit2.go`, `window.go` — the same eight
> at `v2.15.0`). Only the version in each citation moved.
>
> **And the rule this ADR states now has something enforcing it.** Twice in a
> row — #34, then #56 — a dependency bump moved the library and left the CLI
> behind, and both times nothing failed; the second was caught by a human
> reading a diff. `scripts/wails_pin.py` now enforces it, and what reddens a
> pull request is the `Wails pin guard` step of `quality.yml`, which runs it —
> not `make quality`, which that workflow never invokes. The Makefile target of
> the same name is the local and pre-commit form. Every `cmd/wails@v…` install
> pin must equal `go.mod`'s library version, hunted across `Makefile`,
> `.github/workflows/`, `.github/actions/`, `scripts/` and any `Dockerfile*`;
> and every Wails version named in the first three of those must equal it too.
> It fails closed on an unreadable or ambiguous `go.mod`, on a `replace` for
> Wails, on an absent `Makefile`, on any unreadable file it scans, and on a tree
> carrying no install pin at all.
>
> **Where the prose exemption reaches, precisely.** Inside a build file a
> version string is an instruction, not prose, so a Wails version other than the
> pinned one fails there even when it is meant as history or contrast. The
> exemption is for DOCUMENTATION: this ADR must keep naming `v2.13.0` and
> `v2.15.0`, and a guard that rewrote history to match the present would be
> worse than the drift it catches. What the guard does not see at all, declared
> rather than implied: a pin assembled at run time — split across a shell line
> continuation or composed from an `env:` value — and a version named in a
> makefile reached through `include`.

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
