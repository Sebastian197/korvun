# ADR-0032: CLI interface contract

> **Status:** accepted
> **Date:** 2026-07-18
> **Deciders:** Sebastián Moreno Saavedra

## Context

Piece 3 gave Korvun a first-class command line: `internal/cli.Run(args, stdout,
stderr) int` behind a 3-line `cmd/korvun/main` (ADR-0017). The framing (HANDOFF,
PIECE 3) and the design spec
(`docs/superpowers/specs/2026-07-12-piece-3-cli-design.md`) called for a short
**interface-contract ADR** — not a dependency ADR (there is no dependency; styling
is pure stdlib) — to pin the surface so it is a deliberate contract rather than an
accident of implementation. The design spec's Review Checklist and Success Criteria
require this ADR to be `accepted`.

This records the contract as it was **landed and pinned by tests** across sub-phases
1–4. It introduces nothing new; every clause below is already enforced by
`internal/cli`'s unit suite and by cross-compile ×6 + windows-latest CI.

## Decision

The `korvun` command line is the following fixed contract.

**Subcommand set.** Five verbs, dispatched on `args[0]`, each with its own
`flag.NewFlagSet` (no Cobra — a small, stable set, zero dependencies):

- `serve [--config <path>]` — load config, wire channels/brains, serve until
  SIGINT/SIGTERM. `--config` defaults to `korvun.json`.
- `config check [--preflight] [path]` — offline `config.Load`/`Validate` by default;
  `--preflight` additionally runs the online `app.Preflight` (secret resolution,
  privacy selector, channel getMe).
- `status [--addr host:port]` — a thin read-only client of the admin API
  (`/healthz` + `/api/brains` + `/api/channels`, ADR-0022). `--addr` defaults to
  `config.DefaultObservabilityAddr` (`127.0.0.1:2112`). No token, no new server code.
- `version` (and the `--version`/`-version` flag forms) — the build identity via
  `internal/buildinfo.Format`, machine-clean to stdout.
- `help` (and `--help`/`-h`, and bare `korvun`) — the usage screen to stdout.

**Retrocompat shim (narrowed).** The pre-CLI invocation `korvun -config <path>` (and
`--config`, incl. the `=value` form) still boots as an implicit `serve`, so the
hardware-validated docs/systemd invocation keeps working. The shim is scoped to the
`-config`/`--config` flag **only** — a bare display flag such as `korvun --plain`
does NOT slip through and boot a server. `korvun serve --config <path>` is the
canonical form; the shim form is legacy.

**Exit codes.** `0` success (incl. `help`/`version`/`-h`, which are queries) · `1`
runtime failure (boot error, failed preflight, admin API unreachable) · `2` usage
error (unknown subcommand/sub-verb, bad flag, unexpected positional). `-h`/`--help`
on any subcommand is a query routed to stdout with exit 0, never the exit-2 usage
path.

**Stream discipline.** Machine-readable payload (version line, `status` table,
`config check` OK line) goes to **stdout**; all decoration, banners, and error
messages go to **stderr**. `stdout` under a non-TTY or an opt-out carries zero ANSI
escape bytes.

**Uniform positional strictness.** Every subcommand rejects an unexpected positional
argument with a usage error (exit 2) rather than silently ignoring it or booting with
a default — uniformly, including via the shim (`korvun -config x.json extra` → exit
2). No documented invocation carries a trailing positional, so none regress.

**Styling gate (R1–R4).** Styling (ANSI color, the banner) is emitted only when the
target stream is an interactive TTY **and** none of `--plain`, `--no-color`, or
`NO_COLOR` is set (R1+R2), **and** the terminal can render VT sequences (R4: on
Windows a raw kernel32 `SetConsoleMode` enables VT once; on failure the CLI degrades
to plain rather than printing literal escapes; on Unix this is a no-op). The palette
is KORVUN's identity (ADR-0030): a violet accent plus the fixed event roles
(`received`/`sent`/`dropped`/`failed`), never rainbow. **Color is never the only
channel** — a status color always accompanies a textual label (ADR-0030). `serve`'s
structured `slog` JSON is not restyled (FR-STY-8).

## Consequences

- The surface is now a written contract: a change to the verb set, exit codes,
  shim scope, or stream discipline is an ADR-level change, caught in review rather
  than drifting silently.
- The design spec's closure criterion (an `accepted` interface-contract ADR) is met.
- Because the contract mirrors what tests already assert, the ADR and the suite
  cannot disagree without a test failing — the record stays honest.
- Future verbs (e.g. `status --json`, shell completion) extend the set under the
  same rules; they do not need to re-litigate the gating or exit-code conventions.

## Alternatives Considered

- **A dependency ADR (Cobra / a styling library).** Rejected in the design spec: the
  set is small and stable, and the elegant finish is achievable in pure stdlib
  (`flag`, `text/tabwriter`, hand-rolled ANSI, a build-tagged VT syscall). `go.mod`
  stays at 3 direct dependencies. This ADR is therefore an interface record, not a
  dependency justification.
- **No ADR (leave the contract implicit in the tests).** Rejected: the framing
  explicitly required a short contract ADR so the subcommand set / retrocompat / exit
  codes are a deliberate, discoverable decision, not archaeology across test files.
- **A broad "any leading dash → serve" shim.** Rejected during SP2 review: it let a
  bare display flag boot a server. Narrowed to the config flag (above).
