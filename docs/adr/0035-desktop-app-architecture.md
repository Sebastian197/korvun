# ADR-0035: Desktop app architecture (Piece 5)

> **Status:** proposed
> **Date:** 2026-07-19
> **Deciders:** Sebastián Moreno Saavedra
>
> Codifies the resolutions of the 8 `[NC-*]` items from the Piece 5 framing
> (`docs/notes/piece-5-framing.md`, hardened through two adversarial review
> rounds during the framing session). NC-1/2/4/8 were resolved by Chano
> (2026-07-19); NC-3/5/6/7 by the copilot.
> Companion dependency ADR: ADR-0036 (`wailsapp/wails/v2`). **Zero code lands
> with this ADR** — implementation starts with the TDD sub-phase plan after the
> copilot accepts both ADRs.

## Context

Piece 5 delivers the desktop app: a native window that serves the existing
builder and operates an embedded Korvun end to end — no terminal. The framing
verified (2026-07-19, Context7 + primary sources) that Wails v3 is still alpha
and **Wails v2.13.x is the stable line**, that a Wails Linux binary links
GTK/WebKitGTK via cgo, and that the headless binary's `CGO_ENABLED=0` ×6
cross-compile is a hard constraint that cannot absorb a WebView.

Two structural facts force the shape of everything below:

1. **The same binary is impossible.** The headless artifact must stay pure-Go,
   cgo-free, and byte-for-byte unaffected — the Pi has no desktop.
2. **The builder and the admin API already exist** (React 19 + Vite bundle
   embedded via `go:embed`, served **token-gated at `/builder/`**; `/ui/` is
   the minimal read-only live-view page; admin routes `/healthz`, `/metrics`,
   `/api/brains`, `/api/channels`, `/api/events` SSE, `GET/POST /api/config`
   behind a bearer token, `/api/reload/{handle}`). The builder and the
   mutation API mount **only when `Build` is wired with the reload supervisor
   (`WithReloader`) AND the `admin.token_env` variable resolves** — a
   precondition the desktop shell must satisfy (§§1, 4, 5). The desktop app is
   a shell around them, not a rewrite.

## Decision

### 1. Form — sibling binary, core in-process (NC-3, copilot)

A new **`cmd/korvun-desktop`** binary in the same module, importing the same
`internal/` packages. The core runs **in-process**: choose config →
`config.Load` + `app.Build(..., WithReloader)` — the shell wires the
**`internal/supervisor` reload seam exactly as `cmd/korvun serve` does**, so
the builder and the mutation API mount; start → `App.Run`; stop →
`App.Shutdown`. The headless `cmd/korvun` and its GoReleaser ×6 pipeline are
untouched.

- **Fallback (documented, not built):** option B — the shell launches the
  existing `korvun serve` binary as a supervised subprocess. It is adopted only
  if the in-process rebuild proves leaky in practice.
- **Mandated test (TDD):** N repeated start/stop cycles in one process must
  show no goroutine, port, or file-handle leak (`-race`, goroutine snapshot
  before/after). This test is the tripwire that would trigger fallback B.

### 2. Naming and release cadence (NC-5, copilot)

The binary and artifacts are named **`korvun-desktop`**. Releases ride the
**same SemVer tag and the same GitHub release** as the headless binary: one
release, both artifact families. The trigger for `v0.4.0` (release outlook)
already binds the piece to the shared cadence.

### 3. Lifecycle exposure and asset seam (NC-6, copilot)

- **(a) Bindings:** start/stop/config-selection are exposed to the UI as
  **Wails bindings**, implemented as **plain Go functions testable without
  Wails** (a `shell` package with no Wails import; the Wails glue is a thin
  adapter over it). Ratified here after weighing testability: the logic tests
  as ordinary Go; only the adapter layer needs the framework.
- **(b) API origin:** the WebView's frontend origin is Wails' own asset
  server, so calls to the loopback admin API are cross-origin and the admin
  API has no CORS (verified). The decision is to **proxy `/api/*` through the
  Wails AssetServer handler** (restores same-origin, core untouched),
  **conditioned on** the first technical sub-phase verifying that the
  AssetServer supports **streamed/flushed responses** (the SSE live-view must
  not buffer). If it buffers, the fallback is **restricted CORS** on the admin
  API (allow only the Wails origin, loopback bind unchanged).
- **(c) Shell chrome survives core shutdown** (confirmed requirement): the
  start/stop controls, config picker, and health indicator are served by the
  shell, not by the core's HTTP server — pressing "stop" must never kill the
  page that holds the "start" button. `web/builder/dist` is embedded **once**
  and served through both paths from the same `embed.FS` (no bundle drift).

### 4. Secrets — keychain → own-process env injection (NC-7, copilot-approved direction)

The env-only rule (ADR-0010 §3, ADR-0028) stands **unchanged**: the core reads
secrets exclusively from environment variables, never from argv, config files,
logs, or errors. What changes is **who writes the environment**: a GUI app
launched by double-click inherits no shell env, so the shell provisions it.

- **Mechanism:** on boot, the shell reads secrets from the **OS keychain** and
  injects them into **its own process environment** (`os.Setenv`) **before**
  `app.Build`. The core is oblivious — it still just reads env.
- **Precedence (preserves ADR-0010 semantics):** a variable already present in
  the process env (terminal launch, CI) **wins** over the keychain entry;
  keychain fills what the env lacks; absence remains the existing
  `ErrMissingSecret` boot error surfaced in the UI.
- **Storage contract:** service `korvun`, one entry per env-var name (e.g.
  account `KORVUN_TELEGRAM_TOKEN`), value = the secret. Editing a secret in
  the UI writes the keychain entry; deleting it in the UI **deletes the
  keychain entry** (no orphan copies); nothing is ever written to config
  files, logs, or the DOM beyond masked input fields.
- **Per-OS backends:** macOS Keychain Services; Windows Credential Manager;
  Linux Secret Service (D-Bus / libsecret, i.e. GNOME Keyring / KWallet). No
  file-based fallback in v1 — a Linux desktop without a Secret Service
  provider surfaces an honest error with docs, rather than a silently weaker
  store.
- **Migration:** none needed — existing terminal users keep using env (it has
  precedence); the keychain is additive for GUI users.
- **Admin bearer:** generated **in-process** (crypto/rand) and exposed to the
  core as the `admin.token_env` variable via the same own-process `os.Setenv`
  path, **before each `app.Build`** — so it is regenerated per core
  start/stop cycle. This **amends ADR-0028 F9's reasoning for the desktop**:
  F9's "restart-only rotation" assumed the env is fixed at process launch;
  in-process the shell rotates the value at every core cycle, which is
  strictly more conservative. **Delivery to the builder** (never typed, never
  persisted — amending the ADR-0028/0030 paste-into-the-UI operator flow for
  the desktop only): injected as the `Authorization` header at the §3(b)
  AssetServer proxy, so the token never enters the DOM; if the CORS fallback
  is taken instead, delivered to the frontend at runtime via a Wails binding.
- **Library note:** keychain access needs a Go library (or per-OS syscall
  code); that choice is a **dependency decision with its own four-axis gate**
  in the TDD sub-phase, per CLAUDE.md — this ADR fixes the contract, not the
  library.

### 5. First run — embedded template + 3-step onboarding (NC-8, Chano 2026-07-19)

On first launch (no config file present), the shell writes a **minimal
embedded template derived from `configs/edge.json`** to the platform config
dir, then runs a **3-step onboarding**: check model (is Ollama reachable /
pick provider) → first channel (guided token setup, stored per §4) → start.
The builder (`POST /api/config`) takes over from there once the core runs.

**Template amendment (required):** `configs/edge.json` has **no `admin`
block**, and without `admin.token_env` the mutation API and the builder never
mount. The embedded template therefore ADDS an `admin` block whose
`token_env` names the shell-managed variable of §4 — this is what makes the
"builder takes over" flow reachable at all (found in review; the raw
`edge.json` copy would have shipped a builder-less desktop).

### 6. Packaging matrix v1 (NC-4, Chano 2026-07-19)

| OS | v1 artifact | Deferred to v1.x of the piece |
|---|---|---|
| macOS | **`.dmg` universal** (amd64+arm64) | — |
| Windows | **NSIS installer, AMD64** | Windows ARM64 |
| Linux | **`tar.gz` AMD64** (binary + `.desktop`) | `.deb`, AppImage, Linux ARM64 desktop |

Desktop builds run on native runners (cgo forbids the ×6 cross-compile) in a
**separate workflow**; the headless release pipeline is untouched.

**Port policy (a framing §5 gap, resolved here — not part of NC-4):** the
desktop core **always binds an ephemeral admin port** (`127.0.0.1:0`,
discovered via `Server.Addr()`), overriding whatever the loaded config says
**in memory only** (the file on disk is not rewritten) — so a running headless
`korvun` on `127.0.0.1:2112`, or a user-picked config pinning any port, never
collides. The exact override seam (a new `Build` option vs. mutating the
in-memory `Config` before `Build` — today no addr-override option exists) is
fixed in the TDD sub-phase plan; the policy itself is decided here.

### 7. Platform code-signing policy (NC-1/NC-2, Chano 2026-07-19)

**Zero spend on platform signing in v1.** No Apple Developer Program, no
Windows certificate. The workarounds — Gatekeeper right-click → Open on
macOS, SmartScreen "run anyway" on Windows — are **documented with
screenshots** in the install guide when the piece ships. Linux costs $0. The
**free cosign chain (Stage 16) keeps signing checksums for ALL artifacts**,
desktop included — integrity is covered; platform identity is what is
deferred. Revisable in the future **only by Chano's explicit decision**.

## Consequences

- The desktop app becomes a thin, testable shell: plain-Go lifecycle functions
  + the existing admin API + the existing builder. No router/brain/channel
  code changes.
- The headless artifact, its pipeline, and its users are unaffected in code
  (the binary does not import Wails; the module does). One caveat, owned by
  ADR-0036's maintenance axis: shared transitive module minimums can move
  under MVS when the dependency lands or is bumped — the `go get` sub-phase
  diffs `go version -m` on the headless artifact to keep the claim honest.
- **Closure criteria carried from the framing (§8):** validation on real
  hardware (Chano's iMac) from the packaged artifact, `make quality` green
  over the whole suite, and the headless binary intact. **v1 exclusions stand
  as framed** (framing §8): remote mode, `.deb`/AppImage, Windows/Linux ARM64
  desktop, auto-update, tray/login-start, app stores.
- The env-only security contract survives GUI-ification with its semantics
  intact; the keychain is an input source, not a new secret store contract.
- v1 ships unsigned for platform identity; user trust relies on the cosign
  chain + docs until Chano decides otherwise.
- A future remote mode ("connect to the Pi") reuses the same admin API without
  rework — explicitly out of v1 scope.

## Alternatives Considered

- **Same binary for headless and desktop** — impossible, not just rejected:
  cgo + WebKitGTK cannot enter the `CGO_ENABLED=0` ×6 matrix (framing §3).
- **Option B, subprocess supervision of the existing binary** — viable,
  retained as the documented fallback; loses the single-artifact property and
  doubles the signing/packaging surface (framing §3).
- **Tauri v2 sidecar** — stable and well-documented, but introduces a Rust
  toolchain and a second language for a single-maintainer Go project, and the
  sidecar model duplicates artifacts (framing §§1–2).
- **Wait for Wails v3** — no announced stable date; conditioning the roadmap
  on a third party's unannounced schedule cedes control (framing §2).
- **Electron** — bundles Chromium + Node per app; antithetical to the
  single-small-binary identity and the heaviest possible footprint. Not
  seriously entertained; recorded for completeness.

## Risks and mitigations (from the framing)

- **R1 — Wails v3 goes stable mid-piece:** stay on v2 regardless; migration is
  a later, bounded chore documented by upstream. No mid-flight re-decision.
- **R2 — unsigned first impression** (Gatekeeper/SmartScreen): decided
  consciously (§7); mitigation is honest documentation with screenshots, the
  same discipline as the Discord intent manual step.
- **R3 — a core failure takes down the window (in-process):** the core is
  no-panic by discipline; the N-cycle leak test (§1) plus documented fallback
  B bound the damage.
- **R4 — WebView2 absent on older Windows:** DECIDED here, as the framing
  instructed (§8: "en el ADR, no improvisarla"): the **`download` strategy**
  (download-on-demand bootstrap of the WebView2 runtime) — smallest artifact,
  the standard path for non-Store distribution. Revisit only if the TDD
  sub-phase's Context7 pass over the exact `windows.Options` surface
  contradicts it.
- **R5 — builder bundle drift** (`make build` regenerating `dist`): inherited
  parked chore; §3(c)'s single-embed rule prevents the desktop app from adding
  a second copy of the problem.
