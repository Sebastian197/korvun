# Stage 03 — Router / gateway core

> **Status:** open (Phase 3.1 done, Phase 3.2 pending)
> **Started:** 2026-06-13
> **Closed:** —

## Objective

Wire inbound Envelopes from channels to the right brain, route replies
back to the originating channel, and keep the router from blocking on a
slow brain or a slow channel — all with stdlib-only concurrency
(goroutines, `context`, channels) and no external dependencies.

## Phases

| Phase | Description                              | Status |
|-------|------------------------------------------|--------|
| 3.1   | Routing core                             | done   |
| 3.2   | Concurrency and resilience               | pending |

## Phase 3.1 — Routing core

### Deliverables

- `docs/adr/0003-router-design.md` (committed before any router code)
  pins the two protocol-level decisions: conversation correlation via
  `Meta["conversation.id"]` and bounded-queue + enqueue-timeout
  backpressure. Numbers 0002 is reserved for the WhatsApp ADR; the
  router ADR uses 0003.
- `internal/brain/brain.go` — minimal `brain.Brain` interface
  (`Handle(ctx, *Envelope) ([]*Envelope, error)`). Forward slice of
  Stage 7; everything else (state, model config, policy attachment,
  telemetry) is deliberately absent.
- `internal/router/` package:
  - `doc.go` — package doc + `MetaConversationID` + the three Phase 3.1
    defaults (`DefaultQueueCapacity` 64, `DefaultEnqueueTimeout`
    250 ms, `DefaultSendTimeout` 5 s).
  - `errors.go` — sentinel errors matchable via `errors.Is`.
  - `router.go` — `Router`, `Option`s, `New`, `RegisterChannel`,
    `RegisterBrain`, `Route`, `DispatchInbound`, `Shutdown`,
    `ConversationKey`, plus the per-brain worker loop.
- `internal/router/router_test.go` — black-box suite, table-driven for
  the validation paths; in-process `fakeChannel` (implements
  `channel.Channel`) and `fakeBrain` (implements `brain.Brain`)
  exercise routing without any real transport.

### Workflow compliance

| Step                         | Result                                          |
|------------------------------|-------------------------------------------------|
| External docs verified first | n/a (stdlib only)                               |
| ADR written before code      | ADR-0003, committed before any router source    |
| TDD red before green         | Red commit precedes the green implementation    |
| `make quality` green         | yes                                             |
| Stage doc updated            | this document                                   |

### Behaviour fixed in Phase 3.1

- Inbound dispatch validates: nil envelope, direction must be Inbound,
  `Meta["conversation.id"]` must be non-empty, channel must be
  registered, route must exist.
- Inbound dispatch never blocks beyond the enqueue timeout. Saturation
  is surfaced explicitly via `ErrBrainSaturated`, never via silent
  drop or unbounded buffering.
- One worker goroutine per registered brain. Worker dequeues, calls
  `Brain.Handle`, and dispatches each reply back to `env.Channel` via
  `Channel.Send` under a context bounded by `WithSendTimeout`.
- `Shutdown(ctx)` is idempotent, cancels the router context, closes
  every brain queue, and waits up to ctx for in-flight handlers.
  Operations on a shut-down router return `ErrShutdown`.
- `ConversationKey(env)` returns `"<channel>::<conversation.id>"` —
  the canonical address used for any future per-conversation state.

### What is **not** in Phase 3.1 (per ADR-0003)

- Configurable worker count per brain (defaults to 1).
- Per-call brain-handler timeout.
- Per-channel outbound queue.
- Error-reporting hook from worker errors (errors are currently
  swallowed; an observability hook arrives in Phase 3.2 alongside the
  per-call brain timeout).
- High-contention/-race exhaustive concurrency tests. The Phase 3.1
  test suite runs under `-race` but stays focused on contract
  correctness; saturation under load is Phase 3.2.

### Quality

- `make quality`: pass with `-race`.
- Coverage per package (master, post-merge):
  - `internal/channel` 100.0%
  - `internal/channel/webhook` 91.4%
  - `internal/channels/telegram` 100.0%
  - `internal/envelope` 97.8%
  - `internal/router` **93.0%** (target ≥ 90% for a critical package)
  - `internal/brain` not measured — interface-only forward slice with
    no executable statements (re-added to the critical list when
    Stage 7 implements a concrete brain)
  - **total 94.7%**
- CI coverage gate: fixed in the same commit as the router
  implementation. The previous gate used `-coverpkg=./internal/...`,
  which made every per-package number a fraction of the whole tree
  instead of the package's own coverage; that was masked when only
  `envelope` existed and surfaced as soon as a second package
  (`router`) joined.

## Key Decisions

- ADR-0003 — Router design (conversation correlation +
  backpressure).
- `brain` stays out of the critical coverage list until it has
  executable code (Stage 7).

## Notes

- The router imports only `internal/channel`, `internal/brain`,
  `internal/envelope`, and Go stdlib. It does **not** import any
  concrete adapter; the test suite uses in-test fakes that satisfy
  `channel.Channel` and `brain.Brain`.
- Phase 3.2 will introduce: configurable worker pools per brain, the
  per-call brain-handler timeout (default 5 s), the per-channel
  outbound queue, an error-reporting hook, and the high-contention
  concurrency suite (race-heavy load tests, deadlock guards, fairness
  smoke tests).