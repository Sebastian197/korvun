# ADR-0025: Stage 15 — Packaging + release pipeline (GoReleaser)

> **Status:** accepted
> **Date:** 2026-06-28
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Korvun is a capable system today — it boots, serves Telegram live, remembers
across restarts, is observable, uses tools, is introspectable, and streams its
pipeline lifecycle (Stages 0–14·P1). But it has **an audience of one**: the only
way to run it is `go build` from a clone with a Go toolchain. Stage 15 turns the
work already done into something installable as a binary.

The stage was framed with `/office-hours` (light, alternatives generator) and
`/plan-eng-review` (the value), copilot-reviewed, before this ADR. The framing
verdict and the three approaches are preserved in that exchange; this ADR pins
the chosen cut.

### The honest scope of Stage 15 (the P2 correction — do not oversell)

With the repo **private today**, a GitHub Release is NOT publicly downloadable,
so Stage 15 does **not** make Korvun "installable by anyone." That is the public
flip of **Stage 16**. What Stage 15 delivers, concretely:

1. **The author, cross-machine.** Versioned, reproducible artifacts the owner (or
   an authenticated collaborator) pulls with `gh release download` onto a fresh
   Raspberry Pi or a second machine — instead of dragging a Go toolchain + source
   onto every box.
2. **The release machinery, proven.** Tag → artifacts working end-to-end, so the
   Stage 16 public flip is a one-line change (repo visibility), not a build-out.

This is the same real-consumer discipline that deferred the bus (Stage 10) and
the builder's mutation (Stage 14 Phase 2): build the machinery when it has a real
consumer (the author cross-machine + the pipeline), and let Stage 16 unlock the
external consumer by flipping visibility and adding the trust layer.

### The 15 / 16 line (decided)

| Concern | Stage 15 — **produce artifacts** | Stage 16 — **make them public + trusted** |
|---|---|---|
| ×6 binaries + SHA256 checksums + archives + changelog | ✅ | |
| SemVer tag → GitHub Release (the machinery) | ✅ | |
| `--version` (ldflags) | ✅ | |
| Example `edge` / `cloud` configs + install guide | ✅ | |
| **Basic** systemd unit (example) | ✅ | **hardened** unit (sandboxing, limits) |
| Per-release **SBOM** (GoReleaser-native) | ✅ | |
| Repo goes **public** (visibility flip) | | ✅ |
| **Signing / provenance / SLSA**, Scorecard revives | | ✅ |
| Full developer-facing docs site | | ✅ |
| `.deb` / `.rpm` / Homebrew / container images | | ⏸ deferred until demand (neither by default) |

### What the repo gives (verified by reading, not memory)

- **CI already cross-compiles ×6** (`linux`/`darwin`/`windows` × `amd64`/`arm64`,
  `CGO_ENABLED=0`) in `.github/workflows/quality.yml`, but **to `/dev/null`** — it
  proves the build compiles, it does NOT produce artifacts. Stage 15 turns that
  proven matrix into released artifacts.
- **No git tags, no `VERSION`, no ldflags/`--version`, no `.goreleaser.yaml`.** The
  binary cannot today report which build it is.
- **`cmd/korvun/main.go` already uses `flag`** (`-config`), so a `--version` flag
  is a one-line addition on an existing parser.
- **An `sbom` CI job already exists** (the SBOM is produced for CI today); Stage 15
  attaches an SBOM **per release** so the published artifact is self-describing.
- `configs/` holds `korvun.example.json` + `korvun.local.json`; there are no
  `edge`/`cloud` examples yet.

### External-docs verification (per CLAUDE.md non-negotiable)

- **GoReleaser is build-time, NOT a runtime dependency.** It is a standalone
  release tool invoked in CI (the `goreleaser/goreleaser-action`), configured by a
  declarative `.goreleaser.yaml`. It is **never imported by Go code and never
  enters `go.mod`** — the 3-direct-deps discipline (a runtime/`go.mod` invariant)
  is untouched. Verified as the de-facto standard for Go binary releases
  (cross-compile + checksums + archives + changelog + GitHub Release in one run),
  and `CGO_ENABLED=0`-friendly — exactly Korvun's case.
- **Action tag pinned at source (CLAUDE.md rule for CI tooling).** The current
  major is **`goreleaser/goreleaser-action@v7`**; `v7.0.0` resolves to commit SHA
  **`ec59f474b9834571250b370d4735c50f8e2d1e29`** (verified at the Action's
  releases page). The release workflow pins to that full SHA (most secure), with a
  `# v7.0.0` comment, consistent with CLAUDE.md's "full commit SHA or at least a
  fixed major tag, never a floating ref." The GoReleaser distribution itself is
  pinned to `~> v2`. `actions/checkout` / `actions/setup-go` stay on the repo's
  existing `@v6` major-tag convention (already in `quality.yml`).

## Decision

Ship a **GoReleaser release pipeline triggered by a SemVer git tag** that produces
the release artifacts, plus the minimal config/docs that make a downloaded binary
runnable. No new runtime dependency; one tiny, additive production-code touch.

### 1. The release pipeline (GoReleaser, tag-triggered)

A `.goreleaser.yaml` + a new release CI workflow, triggered **on a pushed SemVer
tag** (`v*`), producing a **GitHub Release** with:

- **×6 binaries** — `linux`/`darwin`/`windows` × `amd64`/`arm64`, `CGO_ENABLED=0`
  (the matrix already green in `quality.yml`).
- **SHA256 checksums** — one `checksums.txt` over all artifacts (the file users
  verify; signing it is Stage 16).
- **Archives** — `.tar.gz` for unix targets, `.zip` for windows, each bundling the
  binary + `LICENSE` + `README.md`.
- **Changelog** — generated from Conventional Commits since the previous tag
  (the project already uses Conventional Commits).
- **Per-release SBOM** — see §6.

**Versioning is the git SemVer tag.** Pushing `vX.Y.Z` is the release trigger;
GoReleaser derives the version from the tag. Tags are pushed **by hand**, not by
an Action (a tag pushed from inside an Action does not retrigger workflows — a
known GoReleaser/Actions footgun; manual tagging sidesteps it).

### 2. `--version` via ldflags — the ONLY production-code touch

- Declare `var version = "dev"` in `cmd/korvun/main.go`. Local `go build` keeps
  `"dev"`; GoReleaser injects the tag version through `-ldflags "-X main.version=…"`
  on release builds (its default convention).
- Add a `--version` flag that prints the version and exits 0. It also surfaces the
  VCS revision from `runtime/debug.ReadBuildInfo()` when available, so even a local
  `dev` build reports a useful identity.
- **TDD seam (keeps `main` thin + testable):** `main` stays the deliberately
  un-unit-tested glue (ADR-0017); the version STRING is formatted by a tiny pure
  helper (e.g. `internal/buildinfo.Format(version string, bi *debug.BuildInfo)
  string`) that IS unit-tested red-first. `main` injects `version` and prints the
  helper's output. This is the whole production-code change: additive, reversible
  (delete the helper + flag + var → status quo), zero domain impact.

### 3. Example `edge` / `cloud` configs — files, NOT a runtime profile system

Two example JSON configs (alongside the existing examples, under `configs/` or
`docs/packaging/`): an **`edge`** profile (Raspberry Pi — local Ollama only,
storage on, minimal footprint) and a **`cloud`** profile (cloud providers, larger
fan-out). They are **example files**, documentation-class, with **zero runtime
code**.

- **Rejected as scope creep into runtime:** a `--profile` flag that loads/merges
  profile configs at runtime is *production code* that would violate the
  build-time-only blast radius — and there is no consumer for it today. Deferred
  until a real need exists; example files cover the "Pi vs cloud" story now.
- The configs ship with a startup doc: the env-var NAMES the secrets resolve from
  (`TELEGRAM_BOT_TOKEN`, `GROQ_API_KEY` — `token_env`/`api_key_env` are NAMES, not
  values; ADR-0017 §4 live-finding) and how `-config` selects the file.

### 4. Install guide (short, per-OS where needed)

A concise install doc: download the binary/archive for your OS/arch from the
release, **verify the checksum** against `checksums.txt`, extract, run with a
`-config`. Per-OS notes only where they differ (e.g. the Windows `.zip` vs unix
`.tar.gz`, marking the macOS binary executable). It explicitly does NOT assume a
Go toolchain — that is the whole point of Stage 15.

### 5. Example systemd unit — basic, NOT hardened

A working **example** `korvun.service` (under `docs/packaging/`) showing how to
run Korvun as a service on Linux / a Pi: `ExecStart` with a `-config`, an
`EnvironmentFile` for the secret env-vars (never inline), `Restart=on-failure`,
`After=network-online.target`. It is the **basic functional** unit only.

- **Hardening is Stage 16:** sandboxing directives (`DynamicUser`, `ProtectSystem`,
  `NoNewPrivileges`, `MemoryMax`, `ReadOnlyPaths`) belong with the hardening +
  release stage, not here. Shipping the hardened unit now would over-promise a
  security posture this stage does not own.

### 6. SBOM — resolved: per-release SBOM in Stage 15

**A per-release SBOM is attached via GoReleaser** (its native `sbom` config, e.g.
Syft). Rationale: an `sbom` job already exists in CI, so an SBOM is already
produced; attaching one **per release** is free, makes the published artifact
self-describing (an operator can see exactly what is inside the binary), and is a
supply-chain *artifact*, not a supply-chain *attestation*.

- **The line vs Stage 16:** an SBOM **describes** the build (a bill of materials);
  **signing / provenance / SLSA attestation** *prove* it. The descriptive artifact
  (SBOM) is cheap and additive → Stage 15. The cryptographic trust layer (signing
  the checksum file, provenance, SLSA, Scorecard reviving on the public repo) needs
  the public release to matter and is the explicit subject of Stage 16. So: SBOM in
  15, trust/attestation in 16.

## Consequences

### What this enables

- **The author installs Korvun cross-machine** from a versioned, checksum-verified
  release artifact (`gh release download`) — no toolchain, no clone — and a running
  binary can report `--version`.
- **The release machinery is proven**, so Stage 16's public flip is a visibility
  change, not a build-out.
- **The published artifact is self-describing** (archive + checksum + SBOM).
- **Zero new runtime dependency; `go.mod` stays at 3 direct deps; single binary
  intact.**

### What this asks / costs

- **One tiny production-code touch** (`var version` + a `--version` flag routed
  through a tested helper) — additive and reversible.
- **New build-time surface**: a `.goreleaser.yaml`, a release CI workflow, example
  configs, an install doc, a systemd example. All build-time / CI / docs.
- **A new build-time tool** (GoReleaser) in the release path — but the boring,
  standard one; not in `go.mod`.

### Trade-offs accepted

- **Not "installable by anyone" yet.** Private repo → releases are author/collab
  only until the Stage 16 public flip. Stated plainly so the stage is not
  oversold.
- **Example configs, not a profile system.** No `--profile` runtime mechanism;
  example files only.
- **Basic systemd unit, not hardened.** Hardening is Stage 16.
- **SBOM (descriptive), not signing (attestation).** Trust layer is Stage 16.

## Alternatives Considered

### A — GoReleaser release pipeline (CHOSEN)
The boring, standard tool: cross-compile ×6 + checksums + archives + changelog +
SBOM + GitHub Release from one declarative file, build-time only. **Chosen** —
boring-by-default, zero runtime cost, reuses the green matrix, makes Stage 16 a
flip.

### B — Hand-rolled Makefile + scripts (no GoReleaser)
A `make release` target cross-compiling ×6 + `sha256sum` + `tar`/`zip` +
`gh release create`. **Rejected as the default:** you re-implement and maintain
forever what GoReleaser does for free (archives, checksums, changelog, SBOM,
reproducibility). The project's "hand-rolled where it matters" ethos is about
*runtime correctness-critical* code (the `calc` parser, the bus, the adapters)
where a dependency adds risk to the running binary — NOT release plumbing, which
is not correctness-critical runtime. Here, hand-rolling is the *adventurous* path
and GoReleaser is the boring one. A legitimate fallback only if a hard "zero new
tools in the chain" preference outweighs the maintenance cost.

### C — Big-bang packaging (GoReleaser + `.deb`/`.rpm` + containers + Homebrew + signing now)
**Rejected:** Brooks's accidental complexity — channels with **zero demand today**
(no external users, private repo), repeating the speculative-infra trap the project
keeps avoiding. Signing + public artifacts are Stage 16; system packages wait for
real demand.

### D — Auto-tag from a CI Action (vs manual SemVer tag)
**Rejected:** a tag pushed from inside an Action does not retrigger workflows (a
documented GoReleaser/Actions footgun), and auto-tagging hides the deliberate
SemVer decision. The human pushes `vX.Y.Z`; that push triggers the release.

## Out of scope (recorded, not silently dropped)

- **Repo goes public + binary signing / provenance / SLSA + OpenSSF Scorecard
  reviving + full developer-facing docs site** — Stage 16 (hardening + release).
- **`.deb` / `.rpm` / Homebrew tap / container images** — deferred until real
  demand (neither Stage 15 nor 16 by default).
- **Hardened systemd unit** (sandboxing / resource limits) — Stage 16.
- **A runtime `--profile` config-merge mechanism** — deferred (no consumer; would
  be production code outside this stage's build-time blast radius).

## Delivery — build-time, additive, light review (proposed)

This is almost entirely build-time / CI / docs. The single production-code touch
(`--version`) is TDD'd red-first via the `internal/buildinfo` helper; `main` stays
thin. The pipeline is validated locally with `goreleaser release --snapshot
--clean` (builds all artifacts without publishing) and a CI dry-run before the
first real `vX.Y.Z` tag. The `goreleaser-action` is pinned to the verified SHA
`ec59f474b9834571250b370d4735c50f8e2d1e29` (`# v7.0.0`). `make quality` stays
green with `-race`; cross-compile ×6 unchanged; **`go.mod` stays at 3 direct
deps**. Blast radius is build-time/CI/docs + the additive `--version`; fully
reversible (delete `.goreleaser.yaml` + the workflow + the helper/flag → status
quo). A **light review** (the pipeline produces correct artifacts; `--version`
prints; checksums verify; the example configs boot) suffices; a short feature
branch or direct-to-master is the operator's call at delivery time, since it
touches CI + a thin `main` only.