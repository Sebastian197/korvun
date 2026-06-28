# ADR-0023: Stage 14 Phase 1a — Event bus + router event hook

> **Status:** accepted
> **Date:** 2026-06-28
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Stage 14 (the no-code builder) is the biggest remaining stage and the last real
design fork. The framing (`/office-hours` + `/plan-eng-review`, reviewed by
copilot) found that **"Stage 14" is mis-sized — it is a sequence of stage-sized
deliverables**, partitioned into a read-only **foundation** first and the builder
proper (mutation, edit UI, visual canvas) much later (each its own future ADR).

**Stage 14 Phase 1 is the foundation, split by blast radius into two ADRs**
(the same move Stage 9 made for interface vs engine):

- **ADR-0023 (this one): the event bus + the router event hook** — the piece with
  concurrency risk, the one `/review` scrutinizes under `-race`. It grazes the
  router's hot path.
- **[ADR-0024](0024-builder-live-view-sse-and-ui.md): the SSE live-view + the
  minimal embedded UI** — pure additive on the existing `internal/httpserver`
  (Stage 13's low-risk profile), the consumption layer over the bus.

This split gives a clean stopping point: **merge the `-race`-validated bus+hook
first, the SSE+UI after** as a consumption layer.

### The bus finally has a consumer

Stage 10 deferred the bus because no real subscriber existed
(`docs/notes/bus-design-sketch.md`): the channel↔router↔brain decoupling already
existed via the router's point-to-point queues, and the one second-consumer
(metrics) was wired directly in Stage 12. **Phase 1's live-view (ADR-0024's SSE
endpoint) is that first real subscriber.** This ADR builds the bus; ADR-0024
builds the consumer that validates it — shipped together as Phase 1, so the seam
is never speculative.

### What the repo gives (verified by reading, not memory)

- **The router's failure events already funnel app-side.** Router drops and handle
  failures reach the app-level `onRouterError` closure (`app.go:223`) via
  `WithErrorHandler` — observable WITHOUT touching `router.go`. Only
  `MessageReceived` / `ReplySent` live inside `router.go` and need a new hook.
- **The `WithErrorHandler` shape is the precedent for the hook.** The router
  already takes one optional, nil-safe async hook (`router.go:39`,
  `notifyError` at `:489`). The event publisher is the same shape: optional,
  nil-safe, never blocking the hot path.
- **`brainWorkers > 1` is real.** `RegisterBrain` starts N workers on one queue
  (`router.go:175-181`); the channel pumps run concurrently. So `Publish` is
  called concurrently and must be concurrency-safe by construction — the same
  discipline `model.Model` and `conversation.Store` carry.

### External-docs verification (per CLAUDE.md non-negotiable)

**No new dependency, none to verify.** The bus is Go channels + stdlib `sync`. No
NATS, no message-queue library. `go.mod` stays at **3 direct deps** — this ADR
adds **none**.

## Decision

Ship an **in-memory event bus** behind a `Bus` seam, plus **one additive,
nil-safe, best-effort event hook on the router** for the two events that live
inside `router.go`. The router's queues, workers, and shutdown lifecycle are
**unchanged** except that hook.

### 1. Naming

Recorded as **Stage 14 Phase 1a (bus + hook)**; ADR-0024 is Phase 1b (SSE + UI).
The roadmap order (14 → 15 → 16) is unchanged. The stage doc and HANDOFF make
explicit this is the foundation, not the builder.

### 2. The event bus

A new leaf package (`internal/bus`), based on `docs/notes/bus-design-sketch.md`:

```go
// EventType is a lifecycle fact about the message pipeline.
type EventType int
const (
    MessageReceived EventType = iota + 1 // the router accepted an inbound Envelope into a brain's queue
    ReplySent                            // a reply Envelope was delivered to its channel (Send returned nil)
    MessageDropped                       // a message/reply did not complete its path (saturation or Send failure)
    HandleFailed                         // Brain.Handle failed to produce a reply
)

// Event is a typed lifecycle fact that WRAPS A REFERENCE to the Envelope plus
// metadata. It is NOT the Envelope itself, and never a copy of the domain message.
type Event struct {
    Type     EventType
    Envelope *envelope.Envelope // reference; consumers serialize only non-secret fields (ADR-0024)
    Channel  string
    Brain    string
    Err      error // set for MessageDropped / HandleFailed; nil otherwise
}

type Handler func(Event)

type Bus interface {
    // best-effort, non-blocking, never errors. ctx is the natsBus seam; the
    // in-memory impl is non-blocking and does not consult it (cf. ADR-0015,
    // PriorityReducer ignoring its ctx).
    Publish(ctx context.Context, ev Event)
    Subscribe(t EventType, h Handler) (unsubscribe func())
}
```

- **`inMemoryBus`** is the only implementation in this cut. The `Bus` interface is
  the seam for a future `natsBus` — NATS stays OUT (it would break the
  single-binary Pi promise; roadmap has it as planned/future).
- **Lives ALONGSIDE the router queues, never replaces them.** The router keeps its
  point-to-point queues as the delivery mechanism; the bus is a secondary
  observation tap. Replacing the `-race`-tested queues is rejected.
- **Best-effort, non-blocking, never backpressures the hot path.** `Publish` hands
  the event to each subscriber's bounded buffer and returns immediately; if a
  subscriber's buffer is full (a slow consumer), the event is **dropped for that
  subscriber and counted** (reuse the `DroppedCount` precedent + the
  `metrics.Metrics` seam — e.g. a `korvun_bus_events_dropped_total`). `Publish`
  MUST NOT block, allocate unboundedly, or error. Delivery is **at-most-once**; no
  persistence, no replay.
- **Concurrency-safe under `brainWorkers > 1`.** `Publish` is called concurrently
  by N brain workers and the channel pumps. The subscriber set is guarded so
  `Publish` never races `Subscribe`/`unsubscribe` (a copy-on-write snapshot of the
  subscriber list, or an RWMutex). Each subscriber owns its own goroutine draining
  its buffer, so one slow subscriber never blocks `Publish` or another subscriber.
- **`Publish` is `panic`-safe at the boundary** — a subscriber handler that panics
  must not crash a router worker (the bus runs handlers on their own goroutines;
  the dispatch path only enqueues into buffers).

### 3. The router event hook — the ONE thing that grazes the router

An optional `WithEventPublisher(p EventPublisher)` Option, **nil-safe and
best-effort, the SAME shape as the existing `WithErrorHandler`**. No publisher =
zero cost (a nil check). It is the only router change, and it is additive — the
queues, workers, `WaitGroup` lifecycle, and shutdown order are untouched.

```go
// EventPublisher is the narrow publish-side seam the router publishes lifecycle
// events to (defined in package router). *bus.InMemoryBus satisfies it; kept as an
// interface so the router does not hard-depend on a concrete bus and tests use a fake.
type EventPublisher interface{ Publish(ctx context.Context, ev bus.Event) }
```

As implemented, package `router` imports `internal/bus` (a leaf — it imports only
`envelope`, so no cycle) and constructs `bus.Event` values directly; the
`EventPublisher` interface keeps the dependency on the *concrete* bus out, for
testability.

**Exact publish points and what each event means to the operator:**

| Event | Published from (precise) | Operator meaning |
|-------|--------------------------|------------------|
| `MessageReceived` | **`router.go`, inside `DispatchInbound` on a SUCCESSFUL enqueue** (`bw.queue <- env` returns, ownership transferred to the brain's queue) — NOT at worker dequeue. | "Korvun accepted this inbound message into the target brain's queue." A saturated queue yields `MessageDropped` instead, never `MessageReceived`. |
| `ReplySent` | **`router.go`, inside `deliver` AFTER `cw.channel.Send` returns nil** (`router.go:473`) — NOT at outbound enqueue. | "This reply was actually delivered to the channel." Enqueue success is not delivery; a reply can still drop or fail to send after queuing. |
| `MessageDropped` | **app-level `onRouterError` funnel, no router change** — `ErrKindInboundDispatch` (inbound saturation / no-route / unknown-channel), `ErrKindOutboundSaturated` (reply queue full), `ErrKindSend` (channel delivery failed). | "A message or reply did not complete its path (lost to saturation or a failed send)." |
| `HandleFailed` | **app-level `onRouterError` funnel, no router change** — `ErrKindHandle`. | "The brain failed to produce a reply for this message." |

So the bus has **two feeders**, wired by the app: (1) the router itself, via
`WithEventPublisher`, for the two in-router events; (2) the existing
`onRouterError` closure (`app.go`), which already sees every `RouterError` and
additionally publishes the failure events to the same bus — **zero router change
for drops and failures**. The two in-router publish calls are the entire
hot-path footprint, each a non-blocking enqueue guarded by a nil check.

**Why enqueue-time for received, delivery-time for sent.** `MessageReceived` at
enqueue marks the moment the router takes ownership (the honest "arrived and
accepted" boundary; pre-enqueue saturation is a drop, not a receive).
`ReplySent` at `Send`-success marks actual delivery (the honest "the user got it"
boundary); a reply that is queued but then dropped or fails to send is NOT a
`ReplySent` — it surfaces as `MessageDropped`. The pair brackets the brain's work:
`MessageReceived → (handling) → ReplySent`, or a terminal `HandleFailed` /
`MessageDropped`.

## Consequences

### What this enables

- **A real event stream of the message pipeline** — received, sent, dropped,
  failed — that ADR-0024's SSE endpoint subscribes to. The bus is built and
  (with ADR-0024) validated against a real consumer, closing the Stage 10 deferral
  correctly.
- **The `natsBus` seam exists** for a future multi-process story, with NATS out.
- **Zero new dependency, single-binary intact.**

### What this asks / costs

- **One additive router hook** on the hot-path funnels (the `-race` review zone),
  plus a new leaf package. The router lifecycle is otherwise untouched.
- **A new drop counter** (slow-subscriber events lost), surfaced via the metrics
  seam so the loss is visible, never silent.

### Trade-offs accepted

- **At-most-once, best-effort.** A slow subscriber misses events (a gap, counted),
  never stalls the hot path. No replay/persistence (deferred).
- **The hook touches `-race`-tested code.** Mitigated by: additive nil-safe shape,
  non-blocking publish, and a mandatory concurrency-focused `/review` on a feature
  branch before merge (see Delivery).

## Alternatives Considered

### A — Bus alone, no consumer (defer the SSE to a later stage)
**Rejected:** repeats the Stage 10 "seam without a subscriber" trap. ADR-0024
ships the consumer in the same Phase 1 so the bus is validated, not speculative.

### B — Replace the router queues with the bus
**Rejected:** high blast radius on `-race`-tested delivery code for no benefit;
the bus lives alongside as an observation tap.

### C — Publish ALL events from inside `router.go`
Move the drop/failure publishing into the router too. **Rejected:** drops and
failures already funnel through the app-level `onRouterError` closure, so
publishing them there is **zero router change** — strictly more additive. Only the
two events with no existing funnel (`MessageReceived` / `ReplySent`) earn an
in-router hook.

### D — A blocking / buffered-then-block publish
Guarantee delivery by blocking when a subscriber is slow. **Rejected:** that
backpressures the hot path — the one thing the bus must never do. Best-effort with
a visible drop counter is the contract.

## Out of scope (recorded, not silently dropped)

- **Binding the bus into the running binary (app wiring)** — giving the router its
  real `EventPublisher`, publishing `MessageDropped` / `HandleFailed` from the
  existing `onRouterError` funnel, exposing `bus.DroppedCount()` on `/metrics`, and
  `Close`-ing the bus on shutdown — **lands in ADR-0024 alongside the SSE
  consumer**, so the producer is wired together with its consumer, not before it
  (the project's "no seam without a consumer" discipline). This ADR (1a) delivers
  the bus package and the router hook CAPABILITY, validated by unit tests with a
  fake publisher and a concurrency `-race` test — the binary's hook stays dormant
  (nil publisher, zero cost) until ADR-0024 wires it.
- **The SSE live-view and the UI** — ADR-0024 (Phase 1b).
- **Mutation, auth, the edit UI, the visual canvas** — Phase 2+ (each its own ADR).
- **NATS, event persistence, event sourcing, replay, webhooks-out** — the `Bus`
  interface is the future `natsBus` drop-in point; none built now.

## Delivery — feature branch + mandatory concurrency `/review` (decided)

Unlike Stage 13 (which only grazed `wire()`), this ADR adds a publish call on the
router's hot-path funnels and a concurrent bus — the bus sketch flagged it as
"concurrencia pesada, zona de `/review`", and it sits at the **structural-concurrency
bar** (the Stage 8 / Stage 9 class). **A feature branch with a mandatory
concurrency-focused `/review` under `-race` before merge** — NOT direct to master.
The review must verify: non-blocking publish, slow-subscriber drop (counted, never
silent), safety under `brainWorkers > 1`, no hot-path backpressure, panic-safety at
the subscriber boundary, and that the router lifecycle is genuinely untouched. TDD
red-first (a `-race` test with concurrent publishers + a deliberately-slow
subscriber is the load-bearing test). `make quality` green with `-race` +
cross-compile ×6; `go.mod` stays at 3 deps.