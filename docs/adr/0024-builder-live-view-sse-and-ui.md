# ADR-0024: Stage 14 Phase 1b — Read-only live-view (SSE) + minimal embedded UI

> **Status:** accepted
> **Date:** 2026-06-28
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Stage 14 Phase 1 (the builder's foundation, not the builder) is split by blast
radius into two ADRs (the Stage 9 move):

- **[ADR-0023](0023-event-bus-and-router-hook.md): the event bus + router hook** —
  the concurrency-risk piece, `-race`-reviewed on a feature branch, merged FIRST.
- **ADR-0024 (this one): the SSE live-view + the minimal embedded UI** — pure
  additive on the existing `internal/httpserver`, Stage 13's low-risk profile,
  merged AFTER the bus as the consumption layer.

**This is the piece that validates the bus.** ADR-0023's bus is built against a
real subscriber: the SSE endpoint here. The SSE is curl-able with no UI, so it
closes the loop ("the bus finally has a consumer") on its own; the UI is
presentation on top of it.

### What the repo gives (verified by reading, not memory)

- **The httpserver already serves the surface.** `httpserver.Handle(pattern,
  http.Handler)` mounts routes on the shared `ServeMux` before `Start`
  (`httpserver.go:70`). An SSE handler (stdlib `http.Flusher`) and static assets
  (`http.FileServerFS` over a `go:embed` FS) both ride it with **zero new runtime
  dependencies**.
- **The control API is the precedent.** ADR-0022 mounted read-only `/api/brains` +
  `/api/channels` here, secret-free, loopback, no auth. The live-view + UI extend
  that exact posture with `/api/events` + `/ui`.
- **The bus is a `Subscribe` seam** (ADR-0023). The SSE handler is a bus
  subscriber; per-connection it drains its own buffer, and a slow client drops
  events (a gap), never blocking the bus or the hot path.

### External-docs verification (per CLAUDE.md non-negotiable)

**No new dependency, none to verify.** SSE is stdlib `net/http` (`http.Flusher`,
`text/event-stream`). The UI is `go:embed` + `http.FileServerFS` + vanilla
HTML/JS (no framework, no build toolchain, no WebSocket library, no Wails).
`go.mod` stays at **3 direct deps** — this ADR adds **none**.

## Decision

Ship a read-only **SSE live-event stream** and a **minimal embedded vanilla UI**,
both on the existing `internal/httpserver`, loopback and no auth (the ADR-0022
posture). The SSE subscribes to ADR-0023's bus; the UI renders it.

### 1. The live-view — SSE on the existing httpserver

`GET /api/events` is a read-only handler that **subscribes to the bus** (ADR-0023)
and streams lifecycle events as Server-Sent Events:

- `Content-Type: text/event-stream`, one `data: <json>\n\n` frame per event,
  flushed via `http.Flusher`. Server→client only — the exact fit; **WebSocket is
  rejected** (bidirectional, needs a dep; nothing here is client→server).
- **Per-connection bounded buffer.** Each connection has its own buffered channel
  fed by the bus subscription. If the client cannot keep up and the buffer fills,
  events are **dropped for that connection** (the operator sees a gap, the server
  never stalls) — mirroring the bus's own best-effort contract. The drop is
  counted (the metrics seam) so it is visible.
- **Clean teardown.** The handler returns (and unsubscribes from the bus) on
  client disconnect (`r.Context().Done()`) or server shutdown. No goroutine leak.
- **Secret-free serialization (the binding invariant).** The bus `Event` wraps a
  `*envelope.Envelope` reference, but the SSE frame serializes only NON-secret
  fields — type, channel, brain, a timestamp, and a minimal envelope descriptor
  (id, direction), **never message content nor any secret/secret-reference**. Same
  discipline as ADR-0022 §4, asserted by a test that no frame contains a secret
  value or an env-var name. (Whether to include message text is an open question,
  §Open questions — default is to exclude it.)

### 2. The UI — minimal embedded read-only web page

A minimal **vanilla HTML/JS** page (no React/TS/Vite toolchain), embedded via
`go:embed` and served under `/ui` by the httpserver. It renders `/api/brains`,
`/api/channels`, and the `/api/events` SSE feed (a few lines of `EventSource`).

- **Ships inside the binary** (`go:embed`) — the single-binary promise holds, zero
  runtime deps.
- **Read-only.** No forms, no writes, no auth — it visualizes what the read-only
  API already exposes plus the live event feed.
- **Wails/desktop is rejected** — native webview per platform + a new packaging
  model break the `CGO_ENABLED=0` cross-compile ×6 single-binary promise (Pi →
  cloud). **React/TS/Vite is rejected for this cut** — a page rendering JSON + an
  `EventSource` is a few lines of vanilla JS; a whole build toolchain is an
  innovation token spent before the visual canvas (Phase 4+) needs it.

### 3. Security — read-only keeps the Stage 12/13 calculus intact

`/api/events` and `/ui` are read-only on loopback, no auth = the posture
`/metrics` (ADR-0020) and `/api` (ADR-0022) already accepted, valid because
**nothing mutates**. Both are secret-free. **AUTH is the trigger of MUTATION
(Phase 2):** when the write path arrives, a token becomes essential (the UI and
API share it; the operator may bind beyond loopback). Starting read-only is what
keeps auth out of this cut.

### 4. Coexistence

All on the same `internal/httpserver` mux, mounted in `Build()` before `Start`:
`/healthz`, `/metrics`, `/api/brains`, `/api/channels` (existing) + `/api/events`
(SSE) + `/ui` (static). Rides the observability server, off when observability is
off (the documented ADR-0022 §5 coupling).

```
httpserver ServeMux (loopback, read-only, no auth):
  /healthz  /metrics  /api/brains  /api/channels         (existing)
  /api/events  (SSE) ◄── subscribes to ── Bus (ADR-0023)  (new)
  /ui          (go:embed vanilla HTML/JS, reads the above) (new)
```

## Consequences

### What this enables

- **An operator watches Korvun work in real time** — received / sent / dropped /
  failed — in a browser (`/ui`) or via `curl /api/events`, plus the resolved
  wiring from `/api/brains` + `/api/channels`. The foundation the builder renders
  on.
- **The bus is validated against a real subscriber** (ADR-0023's deferral closed
  correctly).
- **Zero new dependency, single-binary intact.**

### What this asks / costs

- **A new SSE handler** (a bus subscriber with a bounded per-connection buffer) and
  **an embedded static asset**, wired in `Build()`. No router impact at all (the
  bus already exists from ADR-0023).
- **A second read-only network surface** on the same loopback bind — still
  secret-free, nothing exposed externally by default.

### Trade-offs accepted

- **Not the builder.** Read-only visualization; editing and the canvas are Phase
  2+. The stage doc says so plainly.
- **Slow clients miss events** (a counted gap, not a stall). Acceptable for a
  live-view; no replay.
- **Vanilla UI, not a framework.** Minimal and embeddable now; React/Vite waits
  for the canvas.

## Alternatives Considered

### A — WebSocket instead of SSE
**Rejected:** the live-view is server→client only; SSE is the exact stdlib fit
with zero deps. WebSocket (a dependency or a hand-rolled protocol) is only
justified when client→server editing arrives (Phase 3+).

### B — React / TypeScript / Vite for the UI
**Rejected for this cut:** a read-only page rendering JSON + an `EventSource` is a
few lines of vanilla JS; a full build toolchain is an innovation token spent
before the canvas needs it.

### C — Wails / desktop app
**Rejected:** native webview per platform + a new packaging model break the
`CGO_ENABLED=0` single-binary cross-compile ×6 promise. A web UI embedded in the
existing binary keeps it.

### D — Poll `/api/brains` + a separate event log instead of SSE
**Rejected:** polling cannot show the message flow in real time; the bus exists
precisely to stream lifecycle events, and SSE is its natural, dep-free transport.

## Out of scope (recorded, not silently dropped)

- **Mutation, auth, the edit UI, the visual canvas / arbitrary flow logic** —
  Phase 2+ (each its own ADR).
- **WebSocket, a frontend framework/toolchain, Wails** — deferred until the canvas
  justifies them.
- **Event replay / history in the UI** — the bus is at-most-once (ADR-0023); the
  live-view shows events from connection time forward, no backfill.
- **Including message content in event frames** — defaulted OUT for secret-safety
  (§1); revisited only with a redaction story.

## Delivery — additive, Stage 13 profile (decided)

This is pure additive on the existing httpserver — no router change (the bus and
hook are ADR-0023, already merged and `-race`-validated by then). The blast radius
is Stage 13's (a leaf + handlers + an embedded asset, mounted in `Build`). A
**light `/review`** (secret-free assertion on the SSE frames, slow-client drop,
clean teardown, no goroutine leak) suffices; it can go direct to master or a short
branch per the operator's call at delivery time. TDD red-first; `make quality`
green with `-race` + cross-compile ×6; `go.mod` stays at 3 deps. Merges AFTER
ADR-0023's bus+hook.