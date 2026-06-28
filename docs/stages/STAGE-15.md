# Stage 15 — Packaging + release pipeline

> **Status:** closed (the release MACHINERY; the first real release tag is a
> separate, conscious decision still pending)
> **Started:** 2026-06-28
> **Closed:** 2026-06-28
> **ADR:** [ADR-0025](../adr/0025-packaging-and-release-pipeline.md) (accepted)

## Objective

Turn the system Korvun already is (Stages 0–14·P1) into something installable as a
binary, instead of `go build` from a clone with a Go toolchain — an **audience of
one** until now. Stage 15 builds and validates the release **machinery**.

The stage was framed with `/office-hours` (light, alternatives generator) and
`/plan-eng-review` (the value), copilot-reviewed, before ADR-0025. The framing
verdict: Approach A (GoReleaser), the boring standard tool for Go binary releases.

## The honest scope (do not oversell)

With the repo **private**, a GitHub Release is NOT publicly downloadable, so Stage
15 does **not** make Korvun "installable by anyone" — that is the public flip of
**Stage 16**. What Stage 15 delivers, concretely:

1. **The author, cross-machine.** Versioned, reproducible, checksum-verified
   artifacts the owner (or an authenticated collaborator) pulls with
   `gh release download` onto a fresh Raspberry Pi or a second machine — no
   toolchain, no clone.
2. **The release machinery, proven.** Tag → artifacts working end-to-end (validated
   with `--snapshot`), so the Stage 16 public flip is a one-line change (repo
   visibility), not a build-out.

Same real-consumer discipline that deferred the bus (Stage 10) and the builder's
mutation (Stage 14 Phase 2): build the machinery when it has a real consumer (the
author cross-machine + the pipeline); let Stage 16 unlock the external consumer.

## The 15 / 16 line

| Concern | Stage 15 — **produce artifacts** | Stage 16 — **public + trusted** |
|---|---|---|
| ×6 binaries + SHA256 checksums + archives + changelog | ✅ | |
| SemVer tag → GitHub Release (the machinery) | ✅ | |
| `--version` (ldflags) | ✅ | |
| Example `edge`/`cloud` configs + install guide | ✅ | |
| **Basic** systemd unit (example) | ✅ | **hardened** unit (sandboxing, limits) |
| Per-release **SBOM** (describes the build) | ✅ | |
| Repo goes **public** (visibility flip) | | ✅ |
| **Signing / provenance / SLSA** (proves the build), Scorecard revives | | ✅ |
| Full developer-facing docs site | | ✅ |
| `.deb`/`.rpm`/Homebrew/container images | | ⏸ deferred until demand |

## What landed

Pure additive on a build-time/CI/docs surface plus ONE tiny production-code touch.
**`go.mod` stays at 3 direct deps** — GoReleaser is build-time, never imported, never
in `go.mod`. Two commits on master (`fe87f52` ADR accepted, `a8075f9` machinery).

### The release pipeline — `.goreleaser.yaml` (GoReleaser v2)

Triggered by a pushed SemVer tag, it produces a GitHub Release with:

- **×6 binaries** — `linux`/`darwin`/`windows` × `amd64`/`arm64`, `CGO_ENABLED=0`
  (the matrix already green in `quality.yml`, now producing artifacts not
  `/dev/null`).
- **SHA256 `checksums.txt`** over all artifacts.
- **Archives** — `.tar.gz` (unix), `.zip` (windows), each bundling the binary +
  `LICENSE` + `README.md`.
- **Changelog** — `use: git` (no GitHub API, works on a private repo + offline
  during `--snapshot`), grouping Conventional Commits (feat/fix), excluding
  docs/test/chore/refactor.
- **Per-release SBOM** — SPDX JSON via Syft, one per archive.
- **ldflags** — `-s -w -X main.version=v{{ .Version }}` (the `v` prefix is added
  back because GoReleaser's `{{ .Version }}` strips it — see the `/review` finding).
- GoReleaser distribution pinned `~> v2`.

### The release CI workflow — `.github/workflows/release.yml`

`on: push: tags: ['v*']`, `permissions: contents: write`, `checkout` with
`fetch-depth: 0` (the changelog needs full history), Syft installed before
GoReleaser, then the GoReleaser run. **Tags are pushed by hand** (a tag pushed
from inside an Action does not retrigger workflows, and the SemVer bump is a
deliberate decision). Actions pinned to verified full SHAs:

- `goreleaser/goreleaser-action@ec59f474b9834571250b370d4735c50f8e2d1e29` (v7.0.0)
- `anchore/sbom-action/download-syft@e22c389904149dbc22b58101806040fa8d37a610`
  (v0.24.0)
- `actions/checkout@v6` / `actions/setup-go@v6` (the repo's existing major-tag
  convention).

### `--version` — the only production-code touch

- `cmd/korvun/main.go`: `var version = "dev"` (GoReleaser injects the SemVer via
  ldflags on release; a local `go build` keeps `"dev"`) + a `--version` flag that
  prints the formatted version and exits 0, **short-circuiting before any config
  load / network / `app.Build`**.
- **`internal/buildinfo`** (new leaf): a pure `Format(version string, bi
  *debug.BuildInfo) string` that renders `korvun <version>` plus a short VCS
  revision (with a `+dirty` marker) from `runtime/debug.ReadBuildInfo()`, so even a
  local `dev` build reports a useful identity. `main` stays the deliberately
  un-unit-tested glue (ADR-0017); the helper carries the logic and is **TDD
  red-first, 8 table cases, 100% coverage**.

### Example configs + docs

- **`configs/edge.json`** — Raspberry Pi: a single local Ollama model,
  `sensitivity: private` (dispatch stays local-only — the privacy selector keeps
  the local model), durable memory on, minimal.
- **`configs/cloud.json`** — server/VM: a wider fan-out across local Ollama + a
  cloud Groq model, `sensitivity: public`, durable memory + observability on
  loopback.
- These are **example files, NOT a runtime profile system** — a `--profile`
  config-merge mechanism would be production code outside this stage's build-time
  blast radius, and has no consumer today; deferred.
- **`internal/config/examples_test.go`** asserts every `configs/*.json` loads
  (parse + validate), so a schema drift can never silently break the install story.
- **`docs/packaging/INSTALL.md`** — download per OS/arch → verify checksum →
  extract → run; secrets are env-var NAMES, exported never inlined.
- **`docs/packaging/korvun.service`** — a **basic** systemd example
  (`EnvironmentFile` for secrets, `Restart=on-failure`,
  `After=network-online.target`, `TimeoutStopSec=20s` > Korvun's own 15s drain).
  **NOT hardened** — sandboxing/resource directives are Stage 16.

### SBOM — describe vs prove (resolved)

A per-release SBOM is in Stage 15: it **describes** the build (a bill of
materials), is free via GoReleaser's native Syft integration, and makes the
published artifact self-describing. **Signing / provenance / SLSA *prove* the
build** and need the public release to matter — they are Stage 16. So the
descriptive artifact ships now; the cryptographic trust layer waits.

## Verification

- **Validated with `goreleaser release --snapshot --clean`** (builds without
  publishing): all **6 archives + 6 SBOMs + `checksums.txt` (SHA256)** produced,
  each archive carrying binary + LICENSE + README; the released binary prints
  `korvun v0.0.1-snapshot (<rev>+dirty)` — the v-prefixed, ldflags-injected version.
  `goreleaser check` validates the config schema.
- **`make quality` green with `-race`** (total 93.3%); `internal/buildinfo` **100%**.
- **Cross-compile ×6 `CGO_ENABLED=0`** unchanged. **`go.mod` stays at 3 direct deps**.
- **Light `/review`** (independent reviewer): **0 P1**, **1 P2 found and fixed** —
  GoReleaser's `{{ .Version }}` strips the leading `v`, so the binary would have
  printed `1.0.0` instead of the documented `vX.Y.Z`; fixed with `v{{ .Version }}`
  and re-verified in a snapshot. Everything else (truncation logic, flag
  short-circuit, format_overrides, pinned SHAs, profile validity, systemd
  non-hardening) confirmed correct.

## NO real release tag pushed yet (a conscious pending decision)

The machinery is built and snapshot-validated, but **no `vX.Y.Z` tag has been
pushed** — so no GitHub Release exists yet. Pushing the first tag (e.g. `v0.1.0`)
is a separate, deliberate decision (which version, when), taken with the operator
after seeing the machinery green. The release workflow fires ONLY on a tag, so
nothing publishes until that decision is made.

## Out of scope (Stage 16, recorded)

Repo goes public + signing/provenance/SLSA + Scorecard reviving + the hardened
systemd unit + the full developer-facing docs site. `.deb`/`.rpm`/Homebrew/
container images are deferred until real demand. A runtime `--profile` mechanism is
deferred (no consumer; would be production code).

## Files

```
.goreleaser.yaml                    GoReleaser v2 config (build-time)
.github/workflows/release.yml       release CI (on tag v*, pinned SHAs)
cmd/korvun/main.go                  + var version + --version flag (the only prod touch)
internal/buildinfo/                 Format() helper for --version (100%, TDD)
internal/config/examples_test.go    asserts every configs/*.json loads
configs/edge.json, configs/cloud.json   example profiles (files, not a runtime system)
docs/packaging/INSTALL.md           install guide (no Go toolchain)
docs/packaging/korvun.service       basic systemd example (un-hardened)
.gitignore                          + dist/ (GoReleaser output)
```
