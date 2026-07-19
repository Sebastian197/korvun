# Piece 5 SP2 — `internal/shell` Controller (design spec)

> **Status:** approved for TDD (SP2). Governing ADRs: ADR-0035 §§1, 3(a), 4
> (bearer), 6 (port policy). External-docs note: SP2 uses ONLY stdlib +
> existing `internal/` packages — no new external API surface, so no Context7
> pass applies; the `doc.go` contract (**no Wails import in this package**) is
> law. Template note: `docs/superpowers/specs/TEMPLATE.md` referenced by
> CLAUDE.md does not exist in the tree; this spec follows the structure of
> `2026-07-12-piece-3-cli-design.md` (the house pattern).

## Goal

The desktop shell's lifecycle logic as plain, framework-free Go: load a
config, start/stop the in-process core **under the reload supervisor** (the
builder's mount precondition), enforce the ephemeral-port policy, provision
the per-cycle admin bearer, and report status — all testable with `go test
-race`, no window, no Wails.

## Functional requirements

- **FR-1 `New(opts...) *Controller`** — `WithLogger(*slog.Logger)`; nil-safe
  default (`slog` discard-style, matching `app.Build`'s convention).
- **FR-2 `LoadConfig(path string) error`** — `config.Load` + a clear wrapped
  error naming the file; rejected with `ErrRunning` while the core runs
  (switching config files is a stopped-state operation; live mutation belongs
  to the builder/reload path, not the shell).
- **FR-3 `Start(ctx context.Context) error`** — `ErrNoConfig` if nothing
  loaded; `ErrAlreadyRunning` if running. Wires `supervisor.New` exactly as
  `cmd/korvun serve` does (late-bound `sup`; build seam →
  `app.Build(c', app.WithLogger, app.WithReloader(sup))`; preflight seam →
  `app.Preflight`; persist seam → `supervisor.WriteConfigAtomic(path, c)`),
  launches `sup.Run` on a shell-owned cancellable context, and returns only
  when the core either **confirmed Start** (a notifying `supervisor.App`
  wrapper signals after `Start` returns nil) or **failed boot** (the
  `sup.Run` error is returned). No process signals are hijacked: the shell
  passes its own never-signalled channel via `WithSignalChan`.
- **FR-4 Ephemeral-port policy (ADR-0035 §6)** — the build seam applies the
  override to a **shallow copy** of whatever config it receives (initial boot
  AND every reload): `Observability` is replaced by a copied block with
  `Addr: "127.0.0.1:0"` (Enabled semantics preserved; a nil block becomes
  `{Addr: "127.0.0.1:0"}`). The incoming `*config.Config` is never mutated,
  so the persist seam always writes the user's pristine config and **the file
  on disk never learns the ephemeral port**. This decides the seam ADR-0035
  §6 left open: in-memory copy inside the shell's build seam; the core gains
  no new options.
- **FR-5 Per-cycle bearer (ADR-0035 §4)** — on each `Start`, if
  `cfg.Admin.TokenEnv` is non-empty: generate 32 bytes via `crypto/rand`
  (hex), `os.Setenv(TokenEnv, token)` **before** the supervisor's initial
  build. Always overwritten (the ADR mandates per-cycle generation for the
  bearer; the env-wins precedence of §4 applies to keychain secrets, not to
  the bearer). `Stop` unsets the variable — the shell wrote it, the shell
  removes it. The keychain does NOT enter SP2 (that is SP3).
- **FR-6 `Stop(ctx context.Context) error`** — `ErrNotRunning` when stopped;
  cancels the supervisor context and waits for `sup.Run` to return, bounded
  by ctx; returns `sup.Run`'s error (nil on a clean stop).
- **FR-7 `Status() Status`** — `{Running bool, ConfigPath string, AdminAddr
  string}`; `AdminAddr` is the EFFECTIVE bound address (via FR-8) while
  running, `""` when stopped.
- **FR-8 (additive, `internal/app`)** — two exports, `cmd/korvun` untouched:
  (a) `App.AdminAddr() string`, nil-safe accessor over the private
  `adminServer` (`""` when observability is off or before Start); (b)
  `WithChannelFactory(func(config.ChannelConfig) (Channel, error)) Option`,
  the existing private test seam exported with a clean signature — required
  because the shell's hermetic lifecycle tests boot a FULL real App from
  another package, and the suite's no-network discipline (ADR-0034: "no real
  Discord connection needed in the suite") forbids real channel dials. The
  Controller gains `WithBuildOptions(...app.Option)` so tests thread the
  factory into the shell's own build seam.
- **FR-9 Boot-failure honesty** — a failing initial start (e.g. missing
  channel secret) surfaces the wrapped supervisor error from `Start`, leaves
  the controller stopped, and leaks nothing.

## Acceptance scenarios (Given / When / Then)

- **AS-1** Given a path to a malformed/missing file, When `LoadConfig`, Then
  the error names the file and `Status().Running` is false.
- **AS-2** Given no config loaded, When `Start`, Then `ErrNoConfig`.
- **AS-3** Given the minimal real config (discord channel with a dummy token
  env — its Start is async and network-free by design; ollama model pointed
  at an `httptest` server; priority brain; one route; `admin.token_env`),
  When `Start`, Then: `Status().Running` is true; `AdminAddr` is
  `127.0.0.1:<ephemeral>` responding 200 on `/healthz`; the bearer env var
  holds a 64-hex token; `GET /api/config` with the bearer → 200 and without
  → 401 (builder precondition MOUNTED — the SP-review P1 closed by tests).
- **AS-4** Given the config FILE pins `observability.addr:"127.0.0.1:2112"`,
  When `Start`, Then the effective port ≠ 2112 and the file's bytes are
  identical before/after.
- **AS-5** Given a running controller, When `Stop`, Then it returns nil, the
  admin port no longer accepts connections, the bearer env var is unset, and
  `Status()` reports stopped with empty `AdminAddr`.
- **AS-6 (TRIPWIRE, ADR-0035 §1)** Given the AS-3 config, When N=10
  Start/Stop cycles run in ONE process under `-race`, Then the goroutine
  count settles back to the pre-test baseline (settle-wait tolerance, no
  external deps) and every cycle's admin listener is provably closed. This
  test failing is the documented trigger for fallback B.
- **AS-7** `Start` while running → `ErrAlreadyRunning`; `Stop` while stopped
  → `ErrNotRunning`; `LoadConfig` while running → `ErrRunning`.
- **AS-8** Given a config whose channel token env is unset, When `Start`,
  Then a wrapped boot error returns, `Status().Running` is false, and the
  goroutine baseline holds (nothing half-started leaks).

## Success criteria

- `internal/shell` coverage ≥ 85% (house floor for real logic).
- `make quality` green with `-race` over the whole suite; headless
  `cmd/korvun` byte-untouched; `internal/app` diff = FR-8 only (+ test).
- No Wails import anywhere in `internal/shell` (doc.go contract).

## Decisions folded in (surface calls within the SP2 mandate)

Stopped-state-only `LoadConfig`; bearer always regenerated per cycle and
unset on `Stop`; `Stop` on a stopped controller is an error (honesty over
idempotence); reload-path integrity rides the same two seams (apply-on-copy
in build, pristine config in persist) so no supervisor behavior changes.

## `[NEEDS CLARIFICATION]`

None blocking — every open point above resolves inside ADR-0035's existing
decisions. (The missing spec TEMPLATE.md is reported as a repo nit, not a
blocker.)
