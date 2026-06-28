# Stage 14 — Builder foundation, Phase 1 (event bus + read-only live-view)

> **Status:** Phase 1 closed (the builder's *foundation*; the builder proper is
> Phase 2+)
> **Started:** 2026-06-28
> **Closed:** 2026-06-28 (Phase 1)
> **ADRs:** [ADR-0023](../adr/0023-event-bus-and-router-hook.md) (accepted),
> [ADR-0024](../adr/0024-builder-live-view-sse-and-ui.md) (accepted)

## Objective

Give an operator a **live window into Korvun working** — received / sent /
dropped / failed events streaming in real time — in a browser or via `curl`,
plus the resolved wiring Stage 13 already exposes. This is the **foundation the
no-code builder renders on**, not the builder itself.

Stage 14 (the no-code builder) is the biggest remaining stage and the last real
design fork. The framing (`/office-hours` + `/plan-eng-review`, copilot-reviewed)
found that **"Stage 14" is mis-sized — it is a sequence of stage-sized
deliverables**, partitioned into a read-only **foundation** first and the builder
proper (mutation, edit UI, visual canvas) later. **Phase 1 is the foundation**,
split by blast radius into two ADRs (the same move Stage 9 made for interface vs
engine):

- **Phase 1a / ADR-0023** — the event bus + the router event hook: the piece with
  concurrency risk, scrutinized under `-race` on a feature branch, merged FIRST.
- **Phase 1b / ADR-0024** — the SSE live-view + the minimal embedded UI: pure
  additive on the existing `internal/httpserver` (Stage 13's low-risk profile),
  merged AFTER the bus as the consumption layer that validates it.

This split gave a clean stopping point: merge the `-race`-validated bus+hook
first, the SSE+UI after.

## The deferred bus finally has a consumer

Stage 10 deferred the bus as YAGNI (`docs/notes/bus-design-sketch.md`): zero real
subscribers. The channel↔router↔brain decoupling already existed via the router's
point-to-point queues, and the one second-consumer (metrics) was wired directly
in Stage 12. **Phase 1's live-view is that first real subscriber.** ADR-0023 built
the bus; ADR-0024 built the consumer that validates it — shipped together as
Phase 1, so the seam is never speculative. The Stage 10 deferral is now closed
correctly: built when, and only when, a consumer arrived to validate it.

---

## Phase 1a — the event bus + the router hook (ADR-0023, merged `464f8c2`)

A new leaf package `internal/bus`, plus one additive nil-safe hook on the router.

### The bus (`internal/bus`)

An **in-process, best-effort pub/sub** behind a `Bus` seam:

```go
type Bus interface {
    Publish(ctx context.Context, ev Event)              // best-effort, non-blocking, never errors
    Subscribe(t EventType, h Handler) (unsubscribe func())
}
```

- **`InMemoryBus`** is the only implementation. The `Bus` interface is the seam
  for a future `natsBus` (multi-process) — **NATS stays OUT** (it would break the
  single-binary Pi promise). `go.mod` adds **zero** deps (Go channels + `sync`).
- **Best-effort, non-blocking, never backpressures the hot path.** Each subscriber
  owns a bounded buffer (`DefaultSubscriberBuffer = 64`) drained by its own
  goroutine. A full buffer (slow subscriber) → the event is **dropped for that
  subscriber and counted** (`DroppedCount`). `Publish` never blocks, allocates
  unboundedly, or errors. Delivery is **at-most-once**; no persistence, no replay.
- **Concurrency-safe under `brainWorkers > 1`.** `Publish` is called concurrently
  by N brain workers and the channel pumps; the subscriber set is guarded by an
  `RWMutex`, and removal-from-map happens-before channel close so `Publish` can
  never send on a closed channel (the load-bearing race, proven by a `-race` test
  with concurrent publishers + a deliberately-slow subscriber).
- **Panic-safe at the boundary** — a subscriber `Handler` that panics is recovered
  in `invoke`; it never crashes the bus, a publisher, or another subscriber.
- **Teardown:** `unsubscribe` (and `Close`) stop a subscriber's goroutine — no
  leak, PROVIDED the `Handler` returns. `unsubscribe` is **NOT synchronous with
  handler quiescence** (a buffered event may still fire the handler once more after
  it returns, and an in-flight handler runs to completion) — the F2 foot-gun the
  live-view must tolerate (see Phase 1b).

### The four lifecycle events and their publish points

| Event | Published from (precise) | Operator meaning |
|-------|--------------------------|------------------|
| `MessageReceived` | **`router.go`, inside `DispatchInbound` on a SUCCESSFUL enqueue** (ownership transferred to the brain's queue) — NOT at worker dequeue. | "Korvun accepted this inbound into the target brain's queue." |
| `ReplySent` | **`router.go`, inside `deliver` AFTER `Channel.Send` returns nil** — NOT at outbound enqueue. | "This reply was actually delivered to the channel." |
| `MessageDropped` | **app-level `onRouterError` funnel, no router change** — `ErrKindInboundDispatch` / `ErrKindOutboundSaturated` / `ErrKindSend`. | "A message or reply did not complete its path (saturation or a failed send)." |
| `HandleFailed` | **app-level `onRouterError` funnel, no router change** — `ErrKindHandle`. | "The brain failed to produce a reply." |

The pair brackets the brain's work: `MessageReceived → (handling) → ReplySent`, or
a terminal `HandleFailed` / `MessageDropped`. Only the two in-router events earn a
hook; **drops and failures ride the existing app-level funnel — zero router change
for them** (alternative C, rejected: publishing them from inside `router.go` would
be strictly less additive).

### The router hook — the ONE thing that grazes the router

An optional `WithEventPublisher(p EventPublisher)` Option, **nil-safe and
best-effort, the SAME shape as the existing `WithErrorHandler`**. No publisher =
zero cost (a nil check). `EventPublisher` is a narrow interface (`Publish` only)
so the router does not hard-depend on the concrete bus and tests use a fake. The
queues, workers, `WaitGroup` lifecycle, and shutdown order are **untouched**.

The concurrency `/review` APPROVED it (Publish-on-closed-channel proven race-free);
F1/F2/F3 doc-hardening applied. **The hook stayed DORMANT in the binary** (nil
publisher, zero cost) until Phase 1b wired the real bus — the project's "no
producer without a consumer" discipline.

---

## Phase 1b — the SSE live-view + the embedded UI (ADR-0024, merged `4f36447`)

A new leaf package `internal/liveview`, plus additive wiring in `internal/app`.
Pure additive on the existing admin httpserver (Stage 13 profile); the router is
untouched beyond waking its already-merged hook.

### The live-view (`internal/liveview`)

- **`GET /api/events`** — a read-only handler that **subscribes to the bus** and
  streams lifecycle events as Server-Sent Events: `Content-Type: text/event-stream`,
  one `data: <json>\n\n` frame per event, flushed via stdlib `http.Flusher`.
  Server→client only — the exact fit; WebSocket rejected (bidirectional, needs a
  dep). **Zero new runtime deps.**
- **`/ui`** — a minimal **vanilla HTML/JS** page (no React/TS/Vite toolchain),
  embedded via `go:embed` and served by `http.FileServerFS`. It renders
  `/api/brains`, `/api/channels`, and the `/api/events` feed (a few lines of
  `EventSource`). Read-only: no forms, no writes, no auth.

### Binding invariant 1 — SECRET-FREE frames (by construction)

The bus `Event` wraps a `*envelope.Envelope` reference, but the SSE `frame` type
serializes **only NON-secret fields** — `type`, `channel`, `brain`, a server
`timestamp`, and a minimal envelope descriptor (`envelope_id`, `direction`).
**The `frame` type has no field that can carry a secret**: it never touches
`Envelope.Parts` (message content), `Envelope.Meta`, or `Event.Err` (which may
carry provider error text — its detail stays in the structured logs of
`onRouterError`, never on the wire). Secret-free **by construction**, the same
discipline as ADR-0022 §4, asserted by `TestSSE_frameIsSecretFree` (decoy content
+ meta, no-leak).

### Binding invariant 2 — TEARDOWN-SAFE against the F2 foot-gun (by decoupling)

ADR-0023's contract warns that `unsubscribe` cannot be made synchronous with
handler quiescence: a buffered event can fire the bus `Handler` once more after
`unsubscribe` returns, and an in-flight handler runs to completion. The naive SSE
handler would write to the `ResponseWriter` from that handler and could write to a
torn-down connection.

**The fix is to decouple, not to synchronize** (the correct answer to a foot-gun
that says synchronization is impossible): the bus `Handler` writes **ONLY** to an
in-process per-connection buffered channel (`buf`), via a non-blocking
`select { case buf <- ev: default: drop }`. The `ResponseWriter` is touched
**solely by the request goroutine's serve loop**. Once that loop returns (client
disconnect via `r.Context().Done()`, `Close`, or a failed write), no further write
to the writer can occur — even if the bus fires the handler again, it only does a
non-blocking send to `buf` and returns. `TestSSE_teardownNoWriteAfterClose`
deliberately parks the loop mid-write, cancels, releases, marks the writer closed,
then publishes 50 more events and asserts **zero writes after teardown**.

Two more correctness points: the handler **subscribes BEFORE writing the response
headers**, so an event racing the client's first read at connection startup is not
lost; and a **slow client drops** (per-connection buffer full → counted via
`liveview.DroppedCount`), never blocking the bus or another client
(`TestSSE_slowClientDropsCounted`, `TestSSE_slowClientDoesNotTumbleServer`).

### App wiring — the bus wakes (`internal/app`)

- **The real `InMemoryBus` is built ONLY when observability is ON** (`adminServer
  != nil`). Its only consumer — the SSE live-view — rides the admin server, so
  with observability off there is no subscriber and the router's hook stays
  dormant at zero cost. Same conscious coupling as ADR-0022 §5 (no admin server →
  no `/api`). The "no producer without a consumer" discipline, enforced in the
  binary.
- **`WithEventPublisher(eventBus)`** is added to the router options only when the
  bus exists — this is what **wakes** `MessageReceived` / `ReplySent`.
- **`onRouterError`** now also publishes the matching failure event to the bus when
  it is live: `routerErrorToEvent` maps `ErrKindHandle → HandleFailed` and every
  other kind → `MessageDropped`. Zero router change for drops/failures.
- **Pull metrics:** `bus.DroppedCount` → `korvun_bus_events_dropped_total` and
  `liveview.DroppedCount` → `korvun_sse_events_dropped_total`, both via the new
  `prom.RegisterPullCounter` (the same `NewCounterFunc` no-double-instrument
  pattern as `RegisterDroppedSource`, read at scrape time). A registration error
  is logged and skipped, never fatal (review F2 precedent).

### Shutdown ordering (decided + documented)

`App.Shutdown`, in order:

1. **Stop channels** — inbound streams close, the router's pump drains.
2. **`router.Shutdown`** — brain + outbound workers drain; **the bus's producers
   (the router hook + `onRouterError`) quiesce** — no more `Publish`.
3. **Close the store** (gated on a clean router drain, ADR-0019 §6, unchanged).
4. **`liveView.Close()`** — closes a `done` channel the SSE serve loops select on,
   so each long-lived streaming connection returns PROMPTLY (otherwise
   `adminServer.Shutdown` would block on them until the deadline).
5. **`adminServer.Shutdown`** — `/metrics` + `/healthz` stay observable across the
   whole drain above, then the last network surface closes; **the SSE consumers
   are now gone.**
6. **`eventBus.Close()` LAST** — the bus is an *observer* sitting between producers
   and consumers; it closes once both its producers (step 2) and its consumers
   (step 5) are quiescent, tearing down any residual subscriber goroutine with
   nothing left to publish into it. Idempotent and nil-safe.

---

## Verification

- **`make quality` green with `-race`** on integrated master (total **93.9%**).
  `internal/liveview` **92.1%**, `internal/bus` **100%**, `internal/app` **90.7%**.
- **Cross-compile ×6 `CGO_ENABLED=0`** (linux/windows/darwin × amd64/arm64) green.
- **`go.mod` stays at 3 direct deps** (`go-telegram/bot` + `modernc.org/sqlite` +
  `prometheus/client_golang`) — SSE is stdlib `http.Flusher`, the UI is `go:embed`,
  **zero new deps** across all of Stage 14 Phase 1.
- **Headline integration test** (`TestLiveView_endToEnd_inboundProducesSSEEvent`):
  a real inbound routed through the running app wakes the router hook
  (`publishReceived → eventBus`) and the SSE live-view delivers a `message_received`
  frame to a connected client — the bus validated end-to-end against a real
  consumer — while `/healthz`, `/metrics`, `/api/brains`, `/api/channels`, `/ui/`
  stay intact and `korvun_bus_events_dropped_total` is exposed.
- **Reviews:** Phase 1a concurrency `/review` APPROVED (F1/F2/F3 doc-hardening
  applied); Phase 1b reviewed by copilot reading `liveview.go` — APPROVED (the F2
  teardown resolved at the root by decoupling, secret-free by construction, the
  Shutdown ordering and the bus-only-if-observability-ON coupling well-reasoned).

## What this enables

An operator watches Korvun work in real time — received / sent / dropped / failed —
in a browser (`/ui`) or via `curl /api/events`, plus the resolved wiring from
`/api/brains` + `/api/channels`. The `natsBus` seam exists for a future
multi-process story (NATS out). The single binary is intact.

## Out of scope — this is the foundation, NOT the builder

Phase 1 is **read-only visualization**. The builder proper is the following
phases, each its own future ADR:

- **Mutation** of the wiring (add/remove brains, channels, routes) — **add-only or
  reload-and-rebuild, NEVER granular live editing of the running router** (the
  router registry is boot-time; live granular mutation is a concurrency/lifecycle
  change, not a handler).
- **Auth** — **the trigger of mutation.** Read-only is exactly what keeps the
  loopback-no-auth calculus of `/metrics` (ADR-0020) and `/api` (ADR-0022) valid;
  when the write path arrives a token becomes essential (the UI and API share it,
  the operator may bind beyond loopback).
- **The edit UI and the visual canvas / arbitrary flow logic** — where React / TS /
  Vite / a real frontend toolchain earns its innovation token (rejected for this
  read-only cut: a page rendering JSON + an `EventSource` is a few lines of vanilla
  JS).
- **NATS, event persistence / sourcing / replay, webhooks-out** — the `Bus`
  interface is the future `natsBus` drop-in; none built now. The live-view shows
  events from connection time forward, no backfill.

## Files

```
internal/bus/            event bus (Phase 1a, ADR-0023): InMemoryBus, Event, EventType
internal/liveview/       live-view (Phase 1b, ADR-0024): SSE /api/events + go:embed /ui
  ui/index.html          vanilla read-only UI (brains + channels + EventSource feed)
internal/router/         + WithEventPublisher hook (Phase 1a, additive; queues untouched)
internal/app/            + bus wiring, onRouterError → Dropped/Failed, pull metrics, Shutdown order
internal/metrics/prom/   + RegisterPullCounter (bus + SSE drop counters)
docs/adr/0023-*.md       Phase 1a ADR (accepted)
docs/adr/0024-*.md       Phase 1b ADR (accepted)
```
