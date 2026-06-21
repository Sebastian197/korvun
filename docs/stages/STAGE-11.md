# Stage 11 — The real assembly (`cmd/korvun`)

> **Status:** closed
> **Started:** 2026-06-21
> **Closed:** 2026-06-21
> **ADR:** [ADR-0017](../adr/0017-assembly-cmd-korvun.md) (accepted)

## Objective

Turn five proven parts into one process that boots, serves, and stops
cleanly. Stages 0–7 built and demonstrated every piece (channel, router,
model adapters, two dispatch shapes, the two-phase policy engine, the Brain) —
but only through **four disposable demos that called `Handle` directly**. There
was no `main.go` that boots a channel, drains its inbound stream into the
router, lets a Brain backed by real models answer, and ships the reply back,
until a signal stops it.

Stage 11 is that single-binary assembly. It closes **V1 checklist criterion 1**
(a real message in and out through a real binary, not a demo) and replaces the
demos with the `korvun` binary — the first thing in the repo that is a
*product*, not a component.

The stage was framed with `/office-hours` (premise challenge + forced
alternatives) and `/plan-eng-review` (reversibility, boring-by-default,
own-your-code-in-production), then pinned by ADR-0017 before any code.

## The finding that reframed the stage

The router was wired only **halfway**, and the four demos hid it.
`RegisterChannel` started the *outbound* worker (drains the reply queue to
`Channel.Send`), but the *inbound* side had no owner: `router.DispatchInbound`
was called **from tests only** — nothing in production drained
`Channel.Receive()` into it. Closing that asymmetry (the inbound pump, in the
router) was the heart of the stage.

## The four decisions (ADR-0017)

| # | Decision | Why |
|---|----------|-----|
| §1 CONFIG | Get the SCHEMA right now; decode with stdlib `encoding/json`; defer YAML | The schema is the one-way door (a contract once operators write files); the format is swappable. YAML would be a 2nd direct dependency on the first binary's critical path. `go.mod` stays at one direct dep. |
| §2 PUMP | The **router** owns the inbound pump (symmetric with the outbound worker) | The pump shares the router's shutdown fences; it is the mirror of a worker the router already owns. Hosting it elsewhere would split the router's two halves. |
| §3 COORDINATOR | Widen `Orchestrator.coord` to a `brain.Coordinator` interface from the first commit | Without it the binary cannot mount the cost-saving sequential fail-over (ADR-0016) — Korvun's selling point. `fanout` and `sequential` satisfy it unchanged. |
| §4 TOKEN | A `getMe` boot health-check that fails loud on an invalid token | A binary that boots and is silently deaf violates the success criterion. **Reconciliation:** `getMe` already lives in `bot.New` (verified via Context7); the gap is closed by construction, no redundant call added. |

Golden rule fixed by §5: **config/boot errors are FATAL and named (exit≠0, no
panic); runtime provider errors DEGRADE (a downed Ollama still boots; the first
message falls to the Brain fallback).**

## What landed

Implemented in four steps (the order ADR-0017 §7 set by dependency and risk):

1. **`refactor(brain)` `c30becc`** — the `brain.Coordinator` interface;
   `Orchestrator.coord` is now the interface, satisfied unchanged by both
   `fanout.Coordinator` and `sequential.Coordinator` (compile-time asserted; a
   test runs `Handle` end-to-end through the sequential shape). Behaviour
   identical. **Direct to master, TDD.**
2. **`feat(router)` pump (branch `feat/router-inbound-pump`, merged `--no-ff`
   `7703ebc`)** — `RegisterChannel` now starts the inbound pump too
   (`runChannelPump`), draining `Channel.Receive()` into `DispatchInbound` under
   the same `channelWg`. Dual exit (closed channel / `ctx`), log-and-continue on
   a bad Envelope (`ErrKindInboundDispatch`), never crashes. **Structural
   concurrency → feature branch + `/review`** (the 2E.8 race class). `/review`
   added the concurrent register-vs-shutdown test and pinned the
   "`Add` under `r.mu`" invariant. Tests run under `-race -count`.
3. **`feat(config)` `9b98a5e`** — `internal/config`: JSON deployment descriptor
   → typed `Config`; fatal validation naming the offending field; secrets by
   env-var reference, never value.
4. **`feat(app)` `5837e09` + `feat(cmd)` `4ae51ed`** — `internal/app` wires
   `Config` → models → `WithModelID` → catalog → `SelectModels` →
   `NewOrchestrator` → router → register/route → channel; the `korvun` binary is
   ~25 thin lines (`flags → config.Load → app.Build → signal.NotifyContext →
   Run → Shutdown`). Boot errors testable because `app` is not `main`.

Plus `docs(router)` `3fa8f23` correcting stale worker-exit comments.

### Layering (boring-by-default, testable)

```
cmd/korvun/main.go   ~25 lines, NOT unit-tested — only glue
  └─ internal/config  parse + validate → Config (fatal, field-named errors)
  └─ internal/app     Config → wired router (catalog, selector, getMe boot, lifecycle)
       └─ internal/router (pump), brain, model/*, policy
```

### Lifecycle (ADR-0008/0010)

```
STARTUP:  build models → SelectModels → NewOrchestrator → router
          → RegisterBrain/Channel/Route → channel.Start (getMe) → pump runs
SHUTDOWN: SIGINT/SIGTERM (signal.NotifyContext) → channel.Stop (closes inbound)
          → pump exits → router.Shutdown (drains workers)
```

## The minimal cut and what was verified live

Example config at `configs/korvun.example.json`: one Telegram polling channel,
one public brain with an Ollama (local) + Groq (cloud) catalog and a
`PriorityReducer`.

**Verified live (this environment, 2026-06-21):** the `korvun` binary boots,
loads and validates the config, resolves secrets from the environment, and runs
the boot health-check. Concretely, run against the real binary:

| Scenario | Result |
|----------|--------|
| Missing config file | fatal `stage=config`, exit 1 |
| Malformed JSON | fatal `stage=config` (parse), exit 1 |
| Invalid schema (unknown sensitivity) | fatal naming `brains[0].sensitivity`, exit 1 |
| Missing `GROQ_API_KEY` | fatal naming the env var, exit 1 |
| Missing `TELEGRAM_BOT_TOKEN` | fatal naming the env var, exit 1 (no network) |
| **Bogus `TELEGRAM_BOT_TOKEN`** | **fatal `telegram: bot.New: error call getMe, unauthorized`, exit 1 — the §4 getMe boot health-check, over the real network to Telegram** |

**NOT verified live (honest):** the full message round-trip — a real Telegram
message → fan-out → policy → reply back to Telegram — because this environment
has no valid bot token, no reachable Ollama, and no Groq API key. The boot path
(config + secret resolution + getMe) is proven against the real binary; the
in/out message path is proven only by the package-level tests
(`TestOrchestrator_Handle_*`, the router routing tests, the pump delivery test),
not by a live end-to-end run. See ROADMAP-V1 criterion 1 (marked **partial**).

## Demos deleted

The seven `cmd/demo-*` commands (`demo-model`, `demo-groq`, `demo-fanout`,
`demo-policy`, `demo-brain`, `demo-selector`, `demo-sequential`) were deleted —
the real `korvun` binary supersedes them. Confirmed nothing in the production
tree imported them (they were `package main`, visible checkpoints only).

## Quality gate

`make quality` green with `-race`. New/changed package coverage:

| Package            | Coverage |
|--------------------|----------|
| `internal/config`  | 98.6%    |
| `internal/app`     | 91.5%    |
| `internal/router`  | 96.0%    |
| `internal/brain`   | 100.0%   |

(`app` is capped below 100% only by `telegram.New`'s real-network success path,
which is not unit-testable; every boot-error branch is covered via an injectable
channel factory.) `go.mod` still has a **single direct dependency**
(`github.com/go-telegram/bot`).

## Outcome

`korvun` is the product that boots. With `TELEGRAM_BOT_TOKEN` + `GROQ_API_KEY`
set and Ollama running, one binary reading one config file connects Telegram to
real models end to end. What remains is operability, not more engine —
persistence, observability, agents, the bus, the control API, the no-code
builder, packaging, hardening.
