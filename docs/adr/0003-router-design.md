# ADR-0003: Router design — conversation correlation and backpressure

> **Status:** accepted
> **Date:** 2026-06-13
> **Deciders:** Sebastián Moreno Saavedra

## Context

Stage 3 builds the **router**: the in-process component that sits
between the channel layer (Phase 2.1 `channel.Channel`) and the brain
layer (introduced by Stage 3). Two design decisions must be pinned
before any router code lands, because both shape its public contract
and any later change would ripple into channels and brains:

1. How a *conversation* is identified across the system, so the same
   user thread can be routed consistently between a channel and a brain
   on every turn.
2. How the router behaves when a brain or a channel cannot keep up with
   inbound traffic — the canonical "slow consumer" problem in any
   message pipeline.

Both decisions are written here as ADRs rather than as code comments
because they are protocol-level invariants (callers must respect them),
not implementation details.

## Decision 1 — Conversation correlation

### What

Every **Inbound** Envelope MUST carry `env.Meta["conversation.id"]`,
populated by the channel adapter at the moment the Envelope is built.

The router's internal identifier for a conversation is the tuple
`(env.Channel, env.Meta["conversation.id"])`, serialised as
`"<channel>::<conversation.id>"` when used as a map key. This guarantees
that two channels which happen to use the same opaque ID never collide
on a router routing table.

`DispatchInbound` rejects any inbound Envelope that does not carry
`conversation.id` with a sentinel error (`ErrNoConversationID`). This
makes the convention enforceable at runtime, not merely documented.

### Why `Meta` and not a typed field on `Envelope`

- Conversation correlation is a **transport-level concept**, not part of
  the message content. The Envelope already uses `Meta` as the agreed
  contract carrier for transport metadata (the Telegram adapter, for
  example, already namespaces `telegram.chat_id` / `telegram.chat_type`
  / `telegram.message_id` in `Meta`).
- Adding a typed `ConversationID` field would force a v2 of the
  Envelope, defeating the canonical-stable contract pinned in Stage 1
  (Phase 1.3 `envelope.Validate`). Each existing and future adapter
  would have to be updated and tested again.
- Validation as a router-level invariant keeps the cost local to the
  router: brain implementations stay simple, and adapters only have to
  set one extra key.

### Channel-side conventions (informative)

To make the rule operational, the canonical mapping per channel is
the following — to be enforced when each `channel.Channel`
implementation lands:

| Channel             | `conversation.id` value                         |
|---------------------|-------------------------------------------------|
| `telegram`          | `telegram.chat_id` (the int64 chat ID, stringified) |
| `webhook` (generic) | the configured `ConversationIDField` of that webhook |

The Telegram pure-converter package in master already stores
`telegram.chat_id`. When Phase 2E.8 wraps it into a `channel.Channel`,
the wrapper will additionally set `conversation.id` to the same value,
preserving the channel-specific key for downstream debugging.

### Alternatives considered

- **A.1 — Router holds a per-channel lookup table `channel → meta key`.**
  Rejected: forces the router to know every channel; defeats the whole
  point of the `channel.Channel` interface.
- **A.2 — `channel.Channel` exposes a `ConversationKey(*Envelope) string`
  method.** Rejected: pushes the abstraction into the interface and
  raises the bar for third-party adapter authors. The same goal is
  achieved by the `Meta` convention, with one less method on the port.
- **A.3 — Use `env.Sender.ID` as the conversation key.** Rejected:
  fails in group chats where many distinct senders share a chat; would
  silently mis-route messages.

## Decision 2 — Backpressure when a brain (or channel) is slow

### What

**Bounded per-brain queue + short enqueue timeout + explicit error.**

Concretely:

- The router holds one `chan inboundJob` per registered brain, with a
  fixed capacity (`DefaultQueueCapacity = 64`).
- `DispatchInbound(ctx, env)` enqueues the job via:

  ```go
  select {
  case queue <- job:
      return nil
  case <-ctx.Done():
      return ctx.Err()
  case <-time.After(enqueueTimeout): // DefaultEnqueueTimeout = 250 ms
      return ErrBrainSaturated
  }
  ```

- A worker pool per brain runs goroutines that dequeue and call
  `Brain.Handle(ctx, env)`. Phase 3.1 ships with **1 worker per brain**;
  Phase 3.2 makes the worker count configurable and adds a per-call
  context timeout (`DefaultBrainTimeout = 5 s`).
- On `ErrBrainSaturated`, the originating channel decides what to do:
  typically, surface a "system is busy" reply to the user. Phase 3.2
  defines a default fallback policy; in 3.1 the error simply propagates.

**Outbound dispatch** (router → channel `Send`) uses the same principle
but at the call site: every `ch.Send(ctx, env)` is wrapped in a context
with a deadline (`DefaultSendTimeout = 5 s`) so that one slow channel
does not block a worker indefinitely. In Phase 3.1 outbound is
synchronous from the brain's worker; Phase 3.2 introduces a per-channel
outbound queue with the same shape as the inbound one (bounded capacity
+ enqueue timeout) so that a single misbehaving channel cannot freeze
all the brain workers.

### Why this shape, and not the obvious alternatives

- **Unbounded buffered channel.** This is the "no backpressure" option:
  under load the router accumulates Envelopes in memory until the
  process is OOM-killed. Rejected — a gateway exists to survive
  congestion, not to amplify it.
- **Drop-on-full (silent loss).** Rejected — silent message loss is the
  worst possible UX for a messaging gateway. If a message is dropped,
  the user must know. An explicit error gives the channel the
  information needed to tell the user something useful.
- **One goroutine per inbound, no queue.** Same memory problem as the
  unbounded buffer, plus uncontrolled goroutine sprawl. The router
  would lose the ability to enforce per-brain fairness, apply rate
  limits, or observe per-brain backlog, all of which are future
  Stage 3.2 / 4 concerns.
- **Disk-backed queue.** Out of scope for Stage 3. The router contract
  defined here is in-memory; a persistence layer can later wrap a
  `Brain` to add durability without touching the router.

### Default knobs

| Knob                            | Default  | Made configurable in |
|---------------------------------|----------|----------------------|
| Per-brain queue capacity        | 64       | Phase 3.1            |
| Enqueue timeout                 | 250 ms   | Phase 3.1            |
| Per-channel outbound timeout    | 5 s      | Phase 3.1            |
| Workers per brain               | 1        | Phase 3.2            |
| Brain handler timeout (per call)| 5 s      | Phase 3.2            |
| Per-channel outbound queue cap  | 64       | Phase 3.2            |

The defaults are intentionally conservative; the goal of 3.1 is to make
the wiring correct and the contract explicit, not to tune for any
particular workload.

## Consequences

- **Channels** gain a small extra responsibility: set
  `Meta["conversation.id"]` before emitting an inbound Envelope. The
  Telegram and webhook adapters either already store equivalent
  channel-specific keys or will be wired up at the moment they grow a
  `channel.Channel` wrapper.
- **Brains** stay simple: they receive an Envelope and return zero or
  more Envelopes back. They do not see queues, timeouts, or
  `conversation.id` semantics.
- **Inbound dispatch is non-blocking up to the queue size.** Saturation
  surfaces as `ErrBrainSaturated`, never as silent memory growth or
  silent loss. This is testable directly (Phase 3.2 has explicit tests
  for it).
- **A slow brain does not affect other brains**: each has its own
  queue. Likewise, a slow channel `Send` is bounded by the outbound
  timeout, so it cannot freeze the worker permanently.
- **The router consumes only `channel.Channel`** (Phase 2.1) and the
  `Brain` interface (introduced by Phase 3.1). It imports no concrete
  adapter; tests use an in-test fake Channel and an in-test fake Brain.

## Open follow-ups (not blockers for Stage 3)

- Metrics. Per-brain queue depth, dispatch latency, saturation counter,
  worker utilisation. Stage 3 emits `slog` events; a Prometheus layer
  belongs to a later stage.
- Retry policy. A retry-with-backoff for `ErrBrainSaturated` (in
  channels that can absorb the retry, e.g. webhook) belongs to a later
  policy ADR, not here.
- Persistent queue. A disk-backed queue is left to a later stage. Any
  implementation can wrap the in-memory router by implementing the
  `Brain` interface.
