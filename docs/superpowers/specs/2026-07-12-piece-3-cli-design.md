# Piece 3 — CLI (subcommands + professional finish): Design Spec

> **Status:** APPROVED (copilot, 2026-07-12) — style is **pure stdlib** (hand-rolled
> ANSI with `NO_COLOR` + TTY-only + machine-readable `stdout`); KORVUN's visual
> identity per **ADR-0030** (violet accent + the fixed event palette), with OpenClaw
> as a reference of **LEVEL, not imitation**; **no 4th dependency**. The subcommand
> contract is the approved framing (HANDOFF, PIEZA 3 block).
> **Date:** 2026-07-12
> **Relates:** HANDOFF PIEZA 3 (approved framing) · Chano's 2026-07-12 requirement
> (professional, elegant finish; OpenClaw as a level reference) · **ADR-0030**
> (KORVUN visual identity: violet functional accent, fixed palette
> `received #3B82F6 · sent #22C55E · dropped #F59E0B · failed #EF4444`, gradient
> reserved for identity moments, "color never the only channel") · reuses
> `internal/app.Preflight` (ADR-0027 §5), `internal/config.Validate` (Stage 11),
> `internal/controlapi` GET endpoints (ADR-0022) · `internal/buildinfo.Format`
> (ADR-0025). Produces a short **interface-contract ADR** (subcommand set,
> retrocompat, exit-code conventions) — NOT a dependency ADR (see the style
> decision).
> **Scope note:** the CLI closes **no** V1 criterion; it is DX/finish. Its timing
> priority is that sub-phase 5 **rewrites the Piece-1 docs** (the shim keeps them
> from breaking; they simply stop being canonical).

---

## Goal

Give Korvun a first-class command-line surface — `korvun serve` / `config check`
/ `status` / `version` / `help` — with a **professional, elegant finish** modelled
on the (source-verified) OpenClaw CLI, while (a) not breaking the install/systemd
docs just validated on hardware (a retrocompat shim), (b) keeping `stdout`
machine-parseable, and (c) holding the project's zero-deps discipline (`go.mod`
stays at 3 direct deps). `main.go` collapses to ~3 lines; all logic lives in a
testable `internal/cli` package.

---

## What was verified about OpenClaw's look, and where (per Chano's requirement)

Verified at **source**, not from memory. Third-party "cheat-sheet" sites for
OpenClaw are largely SEO-farm content (meta-intelligence.tech, stack-junkie.com,
cloudvyn.com, stormap.ai, …) with self-evidently wrong figures (e.g. "80k forks")
— **not** used as authority. The two authoritative sources read:

1. **Official CLI reference — `docs.openclaw.ai/cli`.** Concrete, load-bearing
   findings (these become FRs):
   - **A disciplined semantic palette, one strong accent.** Primary accent
     `#FF5A2D` for **headings and labels**; role tokens: `accentBright #FF7A3D`
     (command names / emphasis), `accentDim #D14A22` (secondary), `info #FF8A5B`,
     `success #2FBF71`, `warn #FFB020`, `error #E23D2D`, `muted #8B7F77`
     (metadata). It is **role-based, not rainbow** — that restraint is what reads
     as "elegant."
   - **TTY-gated styling:** "ANSI colors and progress indicators render only in
     TTY sessions."
   - **NO_COLOR + flags:** `--no-color` disables ANSI, `NO_COLOR=1` is respected,
     `--json` / `--plain` disable styling for clean output.
   - **OSC-8 hyperlinks** where supported, "otherwise the CLI falls back to plain
     URLs"; **OSC 9;4 progress** on long-running commands where supported.
   - **Noun-verb subcommand layout** (`gateway status`, `message send`, …) — the
     same git/docker shape Korvun's framing already adopted.
2. **Repo README — `github.com/openclaw/openclaw`.** A banner/logo (themed
   light/dark image) + lobster mascot 🦞 + tagline; semantic status output
   (`gateway status`, `doctor`).

**Translation to Korvun** (what makes it elegant, minus the brand specifics):
a single accent + a small set of **semantic role colors** (success/warn/error/
info/muted), **bold accent for command names and section headings** in help,
**aligned columns** for structured output, **TTY-only** color that respects
`NO_COLOR`/`--no-color`/`--plain`, and a **banner to stderr**. OpenClaw's spinners
/ OSC 9;4 progress are for genuinely long-running commands — Korvun has none in
this set (`serve` is not restyled; `preflight`'s getMe is sub-second), so a
spinner is **out of first scope** (YAGNI), not a reason to take a dependency.

**Windows ANSI reality (verified, Microsoft Learn):** ANSI VT sequences are **not
on by default** on the legacy console host — `ENABLE_VIRTUAL_TERMINAL_PROCESSING`
must be set via `SetConsoleMode`. Modern **Windows Terminal** enables VT itself;
legacy `cmd.exe`/conhost does not. Linux/macOS terminals are ANSI-native. This
shapes FR-STY-9.

---

## The style decision — RESOLVED (copilot-approved 2026-07-12)

**Question (framing):** with OpenClaw's look in front of us, is the elegant finish
achievable with **pure stdlib** (hand-rolled ANSI: color, bold, aligned tables via
`text/tabwriter`) or does it justify a **minimal styling dependency** (e.g.
`fatih/color`, `charmbracelet/lipgloss`)?

**RESOLVED — PURE STDLIB, no new dependency, no dependency-ADR.** OpenClaw is a
reference of **LEVEL, not imitation**: KORVUN's own identity (ADR-0030) supplies the
palette — **violet functional accent** anchored outside the fixed event palette
(`received #3B82F6 · sent #22C55E · dropped #F59E0B · failed #EF4444`), the
teal→violet gradient reserved for identity moments (the logo), and the invariant
**"color is never the only channel"** (an icon/label always accompanies a status
color, ADR-0030 §colorblind). Every
elegant element verified above maps to stdlib:

| OpenClaw element | Korvun stdlib realization | Dep? |
|---|---|---|
| Accent + role colors | ANSI SGR constants (`\x1b[38;5;…m`, `\x1b[1m`) | no |
| TTY-only styling | `os.Stdout.Stat()` → `Mode()&os.ModeCharDevice` | no |
| `NO_COLOR` / `--no-color` / `--plain` | `os.Getenv` + flag gating | no |
| Aligned help / status columns | `text/tabwriter` (stdlib) | no |
| Banner/logo to stderr | a string constant | no |
| OSC-8 hyperlinks (optional) | raw escape, plain-URL fallback | no |
| Windows VT enable | build-tagged raw `kernel32.SetConsoleMode` syscall | no |

**Against the 4-axis dependency test:** a styling lib buys ~50 lines of escape
codes we can inline, drags transitive deps, and would be the **4th** direct
dependency for a leaf, cosmetic concern — it does **not** clear "stdlib if
reasonable." The one platform wrinkle (Windows VT) is a raw syscall in a
`//go:build windows` file, still stdlib. **Decision: keep `go.mod` at 3; add a
tiny `internal/cli` style helper.** The framing's short **interface-contract ADR**
still ships (it pins the subcommand set / retrocompat / exit codes) — but there is
**no dependency ADR**, because there is no dependency.

**Hard requirements regardless of implementation** (all test-asserted):
- **R1 — TTY-only color.** Color/bold emitted only when the target stream is an
  interactive character device.
- **R2 — `NO_COLOR` + `--no-color` + `--plain` honored.** Any one forces plain.
- **R3 — machine-readable `stdout`.** The banner/logo and ALL decoration go to
  `stderr`; the parseable payload (`status` data) goes to `stdout`; under
  non-TTY / `--plain`, `stdout` carries **zero** escape bytes.
- **R4 — cross-platform.** Linux/macOS native; Windows enables VT or **degrades
  to plain** (never emits raw escapes into a console that would show them
  literally).

**Open sub-points flagged for Chano (non-blocking):**
- `[NC-brand]` The **accent is already fixed by ADR-0030 (violet)**; what remains a
  brand decision is only the **concrete ASCII logo art**. Proposal: ship a
  placeholder banner (violet accent, gradient reserved) to `stderr`; Chano swaps the
  art later. Does not block the sub-phases.
- `[NC-status-json]` A `status --json` mode is a natural follow-on. Proposal:
  **defer to post-beta**; the default `status` `stdout` is already column-parseable.
  Confirm this is acceptable for now.

---

## Functional Requirements

### Subcommand contract (approved framing — not re-litigated)

- **FR-CLI-1** — A new leaf package `internal/cli` exposes
  `Run(args []string, stdout, stderr io.Writer) int`. It is **pure and testable**:
  it never calls `os.Exit`, never touches real `os.Std*` directly, and takes its
  boot seams (serve, preflight, an HTTP client) as injectable dependencies so
  tests drive it without booting the app or opening a port.
- **FR-CLI-2** — `cmd/korvun/main.go` collapses to ~3 lines:
  `os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))` (plus the license header
  and `package main`). All parsing/dispatch/finish moves into `internal/cli`.
- **FR-CLI-3** — Subcommands dispatched on `args[0]`: `serve`, `config check`,
  `status`, `version`, `help`. `flag.NewFlagSet` per subcommand (**no Cobra** —
  small stable set, zero-deps).
- **FR-CLI-4** — Exit-code convention: **0** success · **1** runtime failure (boot
  error, unreachable admin, failed preflight) · **2** usage error (unknown
  subcommand, bad flag). `help`/`--help`/`-h` are a **success** (exit 0, help to
  `stdout`). **Pinned decision table (per invocation):**

  | Invocation | Output | Stream | Exit |
  |---|---|---|---|
  | `korvun help` / `--help` / `-h` | help text | **stdout** | **0** |
  | `korvun` (no args) | help text | **stdout** | **0** *(help is a query, not an error — deliberate; see FR-CLI-5)* |
  | `korvun version` / `--version` | version line | **stdout** | **0** |
  | `korvun serve …` / `korvun -config x.json` | → `serve` seam | (serve's) | (serve's) |
  | `korvun frobnicate` (unknown) | usage error | **stderr** | **2** |
  | bad flag (e.g. `korvun version --nope`) | usage error | **stderr** | **2** |

  The logo/banner (FR-STY-7) is decoration → **stderr, TTY-gated**, and is
  **independent** of these payload streams (it never touches `stdout`).
- **FR-CLI-5** — **Retrocompat shim (~5 lines).** `korvun -config x.json` (and
  `--config`) keeps working as **implicit `serve`**: when `args` is non-empty **and**
  `args[0]` begins with `-`/`--` (a flag, not a subcommand token), route to `serve`
  with the original args. This preserves the hardware-validated docs/systemd
  invocation. **Refinement vs the first draft:** bare `korvun` (truly no args) now
  routes to **help** (exit 0), NOT implicit serve — a new user typing `korvun` gets
  oriented rather than booting a server against a possibly-absent `korvun.json`; the
  validated docs always pass the `-config` flag, so this changes nothing for them.
- **FR-CLI-6** — New canonical form `korvun serve --config x.json` (stdlib `flag`
  accepts `-config` and `--config` identically — free).
- **FR-CLI-7** — `korvun version` prints exactly what `--version` prints today
  (`buildinfo.Format(version, buildInfo)`); the `--version` flag keeps working
  (shim, unchanged output). Single source of truth, no divergence.
- **FR-CLI-8** — `korvun help` / `--help` / `-h` / bare `korvun` → the help screen
  to `stdout`, exit 0. Unknown subcommand or bad flag → concise usage error to
  `stderr` + a `try 'korvun help'` hint, exit 2. Help structure (pinned in this
  sub-phase, color arrives with the style logic): a `Usage:` line, a `Commands:`
  list with a one-line description each, and a closing pointer to where to find more
  (docs). Color/bold is **not** asserted in sub-phase 1 (structure + contracts
  first); it is pinned where its logic is born.

### `config check` (offline default + online opt-in)

- **FR-CHK-1** — `korvun config check [path]` (default `korvun.json`) runs
  **offline** `config.Load` + `config.Validate` (structure + enum + timeout
  invariants — no I/O, no secrets, no network). Reports the first violation with
  its field path, exit 2 for a malformed/invalid config; a clean config prints an
  OK line, exit 0.
- **FR-CHK-2** — `--preflight` additionally runs the **online**
  `app.Preflight(cfg)` (secret resolution + per-brain privacy selector +
  channel/getMe round-trip, ADR-0027 §5). A preflight failure (bad token, missing
  secret, no eligible model) exits **1** with the named error; success extends the
  OK line to note preflight passed.

### `status` (thin client of the existing read-only control API)

- **FR-STA-1** — `korvun status [--addr host:port]` (default
  `127.0.0.1:2112`) is a **thin HTTP client** of the already-serving admin
  endpoints: `GET /healthz`, `GET /api/brains`, `GET /api/channels`
  (ADR-0022). **No token, no new server code.**
- **FR-STA-2** — **Honest failure.** If the admin server is off/unreachable (dial
  error, non-200 `/healthz`), print a clear "admin API not reachable at <addr> —
  is Korvun running with observability enabled?" to `stderr`, exit **1**. Never a
  panic, never a stack trace.
- **FR-STA-3** — The resolved wiring (brains + surviving models + channels + drop
  counts) renders to `stdout` as **aligned columns** (`text/tabwriter`), parseable
  under `--plain`/non-TTY; health/up-down is **semantically colored on a TTY only**.

### Style / finish (from the verified OpenClaw look)

- **FR-STY-1** — KORVUN's identity per **ADR-0030**, not OpenClaw's: a **violet
  functional accent** for headings/command names, anchored **outside** the fixed
  event palette; status roles map to that palette (`received #3B82F6` = info,
  `sent #22C55E` = success, `dropped #F59E0B` = warn, `failed #EF4444` = error) +
  a `muted` de-emphasis. Role-based, never rainbow. **Color is never the only
  channel** — an icon/label always accompanies a status color (ADR-0030).
- **FR-STY-2** — Styling emitted **only** when: target stream is an interactive
  TTY **and** `NO_COLOR` unset **and** neither `--no-color` nor `--plain` given
  (R1+R2).
- **FR-STY-3** — `stdout` stays machine-clean (R3): banner/logo + all decoration →
  `stderr`; parseable payload → `stdout`; non-TTY/`--plain` `stdout` has zero
  escape bytes.
- **FR-STY-4** — **Help layout:** `Usage:` / `Commands:` / `Examples:` sections;
  command names in accent+bold; columns aligned via `text/tabwriter`; each command
  one-line described; a couple of copy-paste examples.
- **FR-STY-5** — **Usage errors (exit 2):** a single concise line in the `error`
  role to `stderr` + the `korvun help` hint. No wall of text.
- **FR-STY-6** — **`status` finish:** the tabwriter table with health colored by
  role (up = success, down = error) on a TTY; identical columns, no color, off-TTY.
- **FR-STY-7** — **ASCII logo/banner = placeholder**, printed to `stderr`, gated
  exactly like all styling (TTY-only, `NO_COLOR`/`--plain` off). The identity moment
  may use the reserved teal→violet gradient (ADR-0030); concrete art deferred
  (`[NC-brand]`). It **never** touches `stdout`.
- **FR-STY-8** — **`serve` is NOT restyled.** Its `slog` JSON on `stderr` is
  untouched; the only addition is an optional gated pre-serve banner to `stderr`.
- **FR-STY-9** — **Windows:** a `//go:build windows` file enables VT via a raw
  `kernel32.SetConsoleMode` syscall (stdlib `syscall`, no dep); on failure, **fall
  back to plain** (R4). Linux/macOS ANSI native. Verified against cross-compile ×6.

---

## The 5 TDD sub-phases (style integrated in each, not bolted on at the end)

Each sub-phase: red tests first → minimum code → `/review` → `make quality` green
`-race` → close. Style is part of every sub-phase's contract.

1. **SP1 — scaffold + dispatch + `version` + `help` + the style core.**
   `internal/cli` with `Run`, subcommand dispatch, exit-code convention, `version`
   (via `buildinfo`), the styled `help`, **and** the style helper (palette, TTY /
   `NO_COLOR` / `--plain` gating, `tabwriter` helpers, banner placeholder,
   Windows-VT file). `main.go` slimmed to ~3 lines.
   *Tests:* dispatch + exit codes (0/2); help to `stdout`; `NO_COLOR` and a non-TTY
   writer both strip every escape byte (R1/R2/R3); help layout sections present.
2. **SP2 — `serve` + retrocompat shim.** `serve --config`, legacy `-config`, and
   bare `korvun` all reach one injected serve seam (tests assert the routing, never
   actually booting). Style: banner to `stderr` gated; `serve`'s slog untouched.
   *Tests:* the three invocations route to `serve` with the right config path; bad
   `serve` flag → exit 2; banner absent under `--plain`/non-TTY.
3. **SP3 — `config check [--preflight]`.** Offline validate default + online
   preflight seam (injected). Style: OK line in `success`, errors in `error` to
   `stderr`. *Tests:* valid → exit 0; invalid → exit 2 with field path; injected
   preflight failure → exit 1; `stdout` clean under `--plain`.
4. **SP4 — `status [--addr]`.** Thin client over an `httptest.Server` serving
   `/healthz` + `/api/brains` + `/api/channels`. Style: tabwriter table, colored
   health on TTY, plain off-TTY. *Tests:* table columns/rows; up/down coloring
   gated; unreachable addr → exit 1 honest message; `stdout` parseable under
   `--plain`.
5. **SP5 — docs update + macOS re-validation (closes the piece).** Rewrite
   `INSTALL.md` / `QUICKSTART.md` / `BUILDER.md` / `korvun.service` from
   `./korvun -config …` to `korvun serve …` (shim keeps the old form valid but
   non-canonical); add a repo `korvun.example.json` (coordinate with follow-up
   (a)). **Re-validate the full quickstart on Chano's Mac** with the new commands.

---

## Acceptance Scenarios (Given / When / Then)

1. **Dispatch.** *Given* the built binary, *When* `korvun version`, *Then* it
   prints exactly the `--version` string and exits 0.
2. **Retrocompat shim.** *Given* the hardware-validated invocation, *When*
   `korvun -config korvun.local.json`, *Then* it behaves as `serve` with that
   config (docs stay valid).
3. **New canonical.** *Given* the same, *When* `korvun serve --config
   korvun.local.json`, *Then* identical serve behavior.
4. **Usage error.** *Given* the binary, *When* `korvun frobnicate`, *Then* a
   one-line error to `stderr` + a `korvun help` hint, exit 2, and `stdout` empty.
5. **Machine-clean stdout.** *Given* `korvun status` piped to a file (non-TTY) or
   with `NO_COLOR=1`, *When* it runs against a live admin API, *Then* `stdout`
   contains the wiring table with **zero** ANSI escape bytes.
6. **TTY finish.** *Given* an interactive terminal with color enabled, *When*
   `korvun help`, *Then* command names/headings are accent+bold and columns align.
7. **config check offline.** *Given* a structurally invalid config, *When*
   `korvun config check bad.json`, *Then* the first field-path violation prints,
   exit 2, with no network touched.
8. **config check preflight.** *Given* a config whose token is missing, *When*
   `korvun config check --preflight korvun.json`, *Then* the named preflight error
   prints, exit 1.
9. **status honest failure.** *Given* no running Korvun, *When* `korvun status`,
   *Then* a clear "admin API not reachable" line to `stderr`, exit 1, no panic.

---

## Success Criteria

- `internal/cli` coverage **≥ 85%** by the end of the piece; the style/gating
  branches (TTY vs non-TTY, `NO_COLOR`, `--plain`, Windows fallback) are covered.
  **SP1 note (2026-07-12):** `internal/cli` measures ~70% in SP1 and this is
  accepted by decision. The master document's ≥85% bar applies to the **core
  packages** (`policy`/`router`/`envelope`/`brain`), not to this entry-point
  layer; the shortfall is entirely the relocated boot glue `serveMain` (still
  exempt as un-unit-tested glue, covered by `internal/app` e2e — see the ADR-0017
  addendum, 2026-07-12). The SP1 **dispatch/version/help** surface is ~100%
  covered. **SP2** makes `serve` unit-testable (own flag surface + injectable
  writers) and clears `internal/cli` ≥85 on its own merits.
- `make quality` green with **`-race`** over the whole suite; cross-compile **×6**
  `CGO_ENABLED=0` green (the Windows-VT file compiles and is exercised on
  `windows-latest` in CI).
- **`go.mod` unchanged at 3 direct deps** — no styling dependency added.
- R1–R4 test-asserted: TTY-only color, `NO_COLOR`/`--no-color`/`--plain` honored,
  `stdout` escape-free off-TTY, Windows degrades to plain.
- **SP5:** docs rewritten to `korvun serve …`, the shim proven to keep the old
  form working, and the quickstart **re-validated on Chano's Mac** end to end.
- The short **interface-contract ADR** (subcommand set, retrocompat, exit codes)
  is `accepted` before SP1 code, per the phase workflow.

---

## Explicitly out of scope (post-beta)

- **Shell completion** (bash/zsh/fish) — post-beta.
- **`man` pages** — post-beta.
- **i18n / localized CLI output** — post-beta (CLI stays English).
- **Spinners / OSC 9;4 progress** — no command in this set is long-running enough
  to need one (`serve` isn't restyled; preflight is sub-second). Trivial to add in
  stdlib later; not a dependency justification.
- **`status --json`** (`[NC-status-json]`) — proposed deferred; base `stdout` is
  already column-parseable.
- **Concrete logo art + final accent hex** (`[NC-brand]`) — Chano's brand call;
  placeholders ship meanwhile.

---

## Review Checklist (must be green before "Tests first")

- [ ] Subcommand contract matches the approved framing (serve / config check
      [--preflight] / status [--addr] / version / help; shim; exit 0/1/2;
      `internal/cli.Run(args, stdout, stderr) int`; 3-line `main.go`).
- [ ] Style decision resolved (stdlib-pure, no dep) and its hard requirements
      (R1–R4) are FR-level and test-asserted.
- [ ] What's styled vs not is explicit (help / usage errors / status / logo styled;
      `serve` untouched).
- [ ] The 5 sub-phases each carry their style contract.
- [ ] Acceptance scenarios + success criteria + out-of-scope are concrete.
- [ ] Open brand/JSON points are flagged non-blocking, not silently decided.
