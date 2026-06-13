# Stage 03 — Router / gateway core

> **Status:** closed
> **Started:** 2026-06-13
> **Closed:** 2026-06-13

## Objective

Wire inbound Envelopes from channels to the right brain, route replies
back to the originating channel, and keep the router from blocking on a
slow brain or a slow channel — all with stdlib-only concurrency
(goroutines, `context`, channels) and no external dependencies.

## Phases

| Phase | Description                              | Status |
|-------|------------------------------------------|--------|
| 3.1   | Routing core                             | done   |
| 3.2   | Concurrency and resilience               | done   |

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

## Phase 3.2 — Concurrency and resilience

### Deliverables

- `internal/router/options.go` — `Option` type plus the new knobs:
  - `WithBrainWorkers(n int)` — concurrent workers per brain
    (default 1; matches Phase 3.1).
  - `WithBrainHandlerTimeout(d time.Duration)` — per-call ctx
    deadline on `Brain.Handle` (default 5 s; 0 disables).
  - `WithOutboundQueueCapacity(n int)` — bounded queue per channel
    (default 64).
  - `WithOutboundEnqueueTimeout(d time.Duration)` — symmetric to the
    inbound enqueue timeout (default 250 ms; 0 disables).
  - `WithErrorHandler(h func(RouterError))` — asynchronous error
    hook; without one, errors are silently dropped (Phase 3.1
    behaviour).
- `internal/router/errors.go` extended with `ErrChannelSaturated`,
  `ErrorKind` (`ErrKindHandle`, `ErrKindSend`,
  `ErrKindOutboundSaturated`), and the `RouterError` struct
  (`Kind`, `Brain`, `Channel`, `Envelope`, `Err`) implementing
  `error` and `Unwrap` for `errors.Is` matching.
- `internal/router/router.go` — Router now owns a per-channel
  outbound worker (drains a bounded buffered queue → `Channel.Send`
  under `sendTimeout`); brain workers spawn `WithBrainWorkers` × N
  goroutines per brain, all draining the same per-brain inbound
  queue; `handleAndReply` wraps `Brain.Handle` in a context bounded
  by `brainHandlerTimeout`; `sendReply` enqueues into the channel's
  outbound queue with `outboundEnqueueTimeout`; `notifyError` skips
  events caused by the router's own shutdown-time context
  cancellation.
- `internal/router/router_phase32_test.go` — black-box tests for
  every Phase 3.2 surface, including the **explicit isolation
  tests** for both brains and channel outbound queues.

### Workflow compliance

| Step                         | Result                                          |
|------------------------------|-------------------------------------------------|
| External docs verified first | n/a (stdlib only)                               |
| ADR written before code      | ADR-0003 (Phase 3.1)                            |
| TDD red before green         | Red commit precedes green for Phase 3.2 too     |
| `make quality` green         | yes, with `-race`                               |
| Stage doc updated            | this document                                   |

### Behaviour fixed in Phase 3.2

- **Configurable brain workers.** Each brain runs `n` worker
  goroutines (`WithBrainWorkers(n)`), all draining its single bounded
  inbound queue. Default 1 (Phase 3.1 serialisation).
- **Brain handler timeout.** Each `Brain.Handle` call runs under a
  context derived from the router's own with a deadline of
  `brainHandlerTimeout` (default 5 s, configurable, 0 disables).
- **Per-channel outbound queue.** `Channel.Send` is no longer called
  inline from the brain worker; instead the reply is pushed onto a
  bounded queue (capacity `outboundQueueCapacity`, default 64) and a
  dedicated outbound goroutine drains it. The brain worker therefore
  never blocks on `Send`, and a slow channel never affects either the
  brain queues or sibling channels' outbound queues.
- **Error hook.** A `func(RouterError)` registered via
  `WithErrorHandler` receives every asynchronous failure: brain
  handler errors (including deadline-exceeded), channel send errors,
  outbound enqueue saturation. Without a hook, errors are dropped
  silently — Phase 3.1 behaviour is preserved.
- **Isolation guarantees, demonstrated, not assumed.**
  - `TestBrainIsolation_SlowBrainDoesNotBlockFastBrain` — a brain
    stuck in `Handle` does not stop another brain from draining its
    own queue.
  - `TestChannelOutboundIsolation_SlowChannelDoesNotBlockFastChannel`
    — a slow `Channel.Send` on one channel does not stop another
    channel from delivering replies.
- **Concurrent dispatch under `-race`.** 20 goroutines × 10
  envelopes = 200 envelopes against 4 brain workers, all processed,
  race detector clean.
- **Shutdown bounded by caller ctx.** In-flight handlers respect the
  cancelled router context and return promptly; errors caused by
  this in-flight cancellation are suppressed from the hook.

## Key Decisions

- ADR-0003 — Router design (conversation correlation +
  backpressure), pinned for both phases.
- `brain` stays out of the critical coverage list until it has
  executable code (Stage 7); it remains an interface-only forward
  slice in this stage.

## Quality Gate (stage-wide, on master)

| Package                          | Coverage |
|----------------------------------|----------|
| `internal/channel`               | 100.0%   |
| `internal/channel/webhook`       | 91.4%    |
| `internal/channels/telegram`     | 100.0%   |
| `internal/envelope`              | 97.8%    |
| `internal/router`                | **96.3%** (≥ 90% critical target) |
| **total**                        | **96.1%** |

`make quality` green with `-race`.

## Notes

- The router imports only `internal/channel`, `internal/brain`,
  `internal/envelope`, and Go stdlib. It does **not** import any
  concrete adapter; the test suite uses in-test fakes that satisfy
  `channel.Channel` and `brain.Brain`.
- The Telegram `channel.Channel` wrapper continues to be tracked as
  Phase 2E.8 of the Telegram-completion plan (see STAGE-02.md);
  Stage 3 does not depend on it.