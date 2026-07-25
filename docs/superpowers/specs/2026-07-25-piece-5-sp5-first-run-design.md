# Piece 5 SP5 — first run (default-config provisioning): Design Spec

> **Status:** draft — **BLOCKED on NC-1** (do not proceed to TDD).
> Governing ADRs: ADR-0035 §5 (first run: embedded template derived from
> `configs/edge.json`, with the MANDATORY `admin` block — the review P1 —
> so the builder can take over), §4 (bearer: `token_env` names the
> shell-managed variable), §6 (ephemeral port overrides whatever the file
> says, in memory only). External-docs note: SP5 uses ONLY stdlib +
> existing `internal/` packages (`config.Load`/`Validate`,
> `supervisor.WriteConfigAtomic`, the SP2–SP4 Controller) — no new external
> API surface, so no Context7 pass applies. Inherited law: the
> `internal/shell` doc.go contract (no Wails import); secrets are NEVER
> written to the config file (ADR-0010 §3).

## Goal

The backend of the first launch: when no config exists at the desktop's
default path, the shell provisions a minimal, working one — atomically,
never overwriting anything that exists — derived from `configs/edge.json`
plus the amendments ADR-0035 §5 mandates (admin block, so the builder and
mutation API mount). The template must pass the core's own
`config.Load` + `Validate` untouched and boot under the real Controller.
Out of scope, deferred: the 3-step onboarding UI and any visual flow
(SP6), packaging (SP7). The headless `cmd/korvun` is untouched.

## Functional requirements

- **FR-FIRST-1 Default path** — the desktop config path is
  `<os.UserConfigDir()>/korvun/korvun.json`, the exact pattern the core
  already uses for the default DB (`internal/app/app.go:502-506`:
  `os.UserConfigDir()` → `filepath.Join(dir, "korvun", "korvun.db")`),
  so config and DB land in the same per-user directory on every OS.
  Exposed from `internal/shell` as a function (e.g.
  `DefaultConfigPath() (string, error)`) so `cmd/korvun-desktop` and SP6
  never hardcode it.
- **FR-FIRST-2 EnsureDefaultConfig** — a shell function (name fixed here:
  `EnsureDefaultConfig`) that: (a) if the file exists → returns
  "already present" and NEVER touches it (not even a rewrite of identical
  bytes); (b) if absent → creates the parent directory (0o700 — it will
  hold the DB too) and writes the embedded template ATOMICALLY via
  `supervisor.WriteConfigAtomic` (temp file + rename, the existing seam),
  returning "created". The two outcomes are distinguishable by the caller
  (SP6 shows different first-run UI on "created").
- **FR-FIRST-3 The embedded template** — a `go:embed` JSON document in
  `internal/shell`, derived from `configs/edge.json` with the ADR-0035 §5
  amendments: an `admin` block with `token_env: "KORVUN_ADMIN_TOKEN"`
  (the shell-managed per-cycle bearer variable, ADR-0035 §4);
  `observability` present and enabled (the effective address is the
  shell's ephemeral override — the file's value is documentation);
  one brain `default` (`sensitivity: private`, priority policy) over
  `ollama`/`llama3.2:1b` at `http://localhost:11434`; storage left to the
  core's default path resolution. NO secret value anywhere in the file,
  ever — only env-var NAMES. **The channel block is BLOCKED on NC-1.**
- **FR-FIRST-4 Fidelity round-trip** — the embedded template, written to
  disk and read back, passes `config.Load` (which runs `Validate`)
  byte-for-byte as embedded. If the core's schema ever moves, this test
  breaks first.
- **FR-FIRST-5 The template BOOTS** — end-to-end: write the template to a
  temp dir, `LoadConfig` + `Start` on the real Controller (the SP2/SP3/SP4
  no-network pattern), assert core up, builder mounted (the admin block
  doing its job: `GET /api/config` 200 through the SP4 proxy or with the
  cycle bearer), clean `Stop`. **BLOCKED on NC-1** — see below: with
  today's core, no channel-bearing template can boot without a real
  secret.

## Acceptance scenarios (Given / When / Then)

Drafted, to be finalized after NC-1 is resolved:

- **AS-1** Given an empty temp config dir, When `EnsureDefaultConfig`,
  Then the file exists at the default path, the report is "created", the
  parent dir has 0o700, and the content equals the embedded template.
- **AS-2** Given an existing config file with arbitrary user bytes, When
  `EnsureDefaultConfig`, Then the report is "already present" and the
  file's bytes (and mtime) are untouched.
- **AS-3** Given the embedded template, When `config.Load` runs on it,
  Then it validates with zero errors, and the loaded struct carries the
  admin block (`token_env == "KORVUN_ADMIN_TOKEN"`) and enabled
  observability.
- **AS-4** Given the template written to a temp dir, When
  `LoadConfig` + `Start` + `Stop` on the real Controller, Then the core
  confirms Start, the builder/mutation surface is mounted, and Stop is
  clean. *(Blocked on NC-1.)*
- **AS-5** Given a read-only parent directory (or an unwritable path),
  When `EnsureDefaultConfig`, Then a wrapped error names the path and no
  partial file is left behind (atomicity).

## Success criteria

- New `internal/shell` code covered ≥ 85%; package stays ≥ 85%.
- `make quality` green with `-race` over the WHOLE suite.
- Headless binary and pipeline untouched; no new dependencies; no Wails
  import in `internal/shell`.

## Decisions folded in

- **Path**: `korvun.json` (not `.jsonc`/`.toml`) — `config.Load` is JSON;
  the name mirrors `korvun.db` in the same directory.
- **Dir perms 0o700**: the directory will also hold `korvun.db`
  (conversation memory) — private-by-default is the only defensible
  choice.
- **Reuse `supervisor.WriteConfigAtomic`** rather than a second
  atomic-write helper (one write discipline in the repo).
- **`KORVUN_ADMIN_TOKEN`** as the bearer variable name: SP2's Controller
  generates and sets it per cycle from `cfg.Admin.TokenEnv`; naming it in
  the template is what makes the builder mount without any user action —
  ADR-0035 §5's review P1 honored.
- **The template never auto-starts anything in SP5** — SP5 is
  provisioning only; when Start happens (and what gates it) is SP6's
  onboarding flow.

## `[NEEDS CLARIFICATION]`

- **NC-1 — the zero-channels / starter-channel question. BLOCKS FR-FIRST-3
  channel block, FR-FIRST-5, AS-4, and all TDD.** The evidence, read from
  the code (not from memory):
  1. `config.Validate` REJECTS zero channels —
     `internal/config/config.go:315-316`: `if len(c.Channels) == 0 {
     return nil, fmt.Errorf("%w: channels: at least one channel is
     required", ...) }`.
  2. The webhook starter does NOT fit cleanly: the validator's type enum
     is exactly `telegram` (mode `polling`) and `discord` (mode
     `gateway`) — `config.go:323-335` rejects any other type as
     `unknown channel type %q (supported: telegram, discord)` — and
     `token_env` is REQUIRED for every channel (`config.go:337-338`).
     `internal/channel/webhook` exists (Stage 2) but was never wired into
     config or the app factory: `defaultChannelFactory`
     (`internal/app/app.go:807-843`) builds only telegram/discord and
     hard-fails on a missing secret (`ErrMissingSecret`; telegram
     additionally performs a live getMe at boot). Making webhook a
     first-class config type means touching the core validator (enum +
     relaxing `token_env` for the type), the app factory, and deciding
     its config surface (field mapping, outbound URL, who serves its
     inbound HTTP) — core-relaxation territory, exactly what this spec
     must not decide unilaterally.
  3. Consequence: with today's core, NO template can both validate and
     boot without a real channel secret already provisioned — a
     first-launch double-click has none by definition.

  Options for the decision (each names its blast radius):
  - **(A) Wire `webhook` as a first-class starter channel type**: core
    validator enum + per-type `token_env` optionality + app factory case
    + minimal config surface for it. Most faithful to the original SP5
    instruction; largest core blast radius, and the Stage-2 adapter's
    inbound HTTP serving needs a home (it is not currently mounted
    anywhere).
  - **(B) Relax the validator to accept ZERO channels** (and zero
    routes), making "gateway with no channels yet" a valid state; the app
    wiring loop must tolerate empty; the builder adds the first real
    channel. Smaller, philosophical change to the core's notion of a
    valid deployment; the desktop first-run boots channel-less with the
    builder mounted — arguably the honest desktop story.
  - **(C) Keep the core untouched; reframe SP5**: the template ships the
    edge.json telegram channel as-is, passes Load+Validate (FR-FIRST-4
    holds), but FIRST BOOT is deferred until onboarding provisions the
    first secret (SP6 gates Start). FR-FIRST-5/AS-4 as mandated ("la
    plantilla DEBE arrancar" on first run) would be DROPPED from SP5 —
    a scope change to this instruction, so it needs your explicit call
    too.

  Per CLAUDE.md and the SP5 instruction ("relajar el validador del core
  es decisión que quiero ver antes"): STOPPED here. No tests, no
  implementation, until NC-1 is resolved.
