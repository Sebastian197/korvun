# ADR-0034: WebSocket client dependency — `coder/websocket`

> **Status:** proposed
> **Date:** 2026-07-18
> **Deciders:** Sebastián Moreno Saavedra
>
> The flip to `accepted` is authorized by the copilot after its review, before any
> TDD. Companion channel ADR: ADR-0033 (Discord). `go.mod` is NOT touched by this ADR
> — the dependency lands with the first TDD sub-phase (`go get` at that point).

## Context

The Discord channel (ADR-0033) can only receive messages over the Gateway, a
WebSocket (RFC 6455) protocol. **The Go standard library has no WebSocket
implementation** — neither client nor server. So the channel needs a WebSocket
client, and Korvun must either hand-roll one or take a dependency.

This would be the **4th direct dependency** (`go.mod` goes from 3 to 4). Korvun's
dependency discipline (CLAUDE.md: "prefer the Go standard library whenever
reasonable"; every dependency needs a justifying ADR + Context7 verification + the
four-axis test) governs this call. Precedent: `github.com/go-telegram/bot` is the
existing channel dependency; the **hand-rolled precedent is the Ollama/Groq REST
adapters over `net/http`** — NOT Telegram (which uses the library). A WebSocket client
is a materially bigger protocol than an HTTP call.

## Decision

Adopt **`github.com/coder/websocket`**, pinned at **`v1.8.15`** (latest stable,
resolved via the module proxy with `go list -m -versions`; `go.mod` left untouched —
the bump lands with the implementation, not this ADR). It is the minimal, idiomatic,
context-first Go WebSocket library (formerly `nhooyr.io/websocket`), with **zero
transitive dependencies**.

**Context7 verification (`/coder/websocket`, reputation High, benchmark 89.85).** The
client surface Korvun uses, confirmed from the docs:

- `websocket.Dial(ctx, url, *DialOptions) (*Conn, *http.Response, error)` — context-
  first dial; `DialOptions{Subprotocols, CompressionMode, HTTPHeader}`.
- `wsjson.Read(ctx, conn, &v)` / `wsjson.Write(ctx, conn, v)` — JSON frames (the
  Gateway speaks JSON), and `conn.Read(ctx)` / `conn.Write(ctx, MessageText, b)` for
  raw frames.
- `conn.Ping(ctx)` for keepalive; `conn.CloseRead(ctx)` for control-frame handling on
  write-mostly connections.
- `conn.Close(status, reason)` / `conn.CloseNow()`; `websocket.CloseError` inspected
  via `errors.As` and `websocket.StatusNormalClosure` et al.
- **Zero external dependencies** (verified: "the main websocket package has no
  external dependencies"). Pure Go.

**Honest gap:** Context7 surfaced the API surface, not the module semver or a defect
changelog; the version was pinned at the source (module proxy), not from memory.

### Four-axis dependency test (CLAUDE.md: capability vs hand-roll cost vs maintenance vs risk/volatility)

| Axis | Verdict |
|------|---------|
| **Capability gain** | A correct, tested RFC 6455 client: Upgrade handshake, framing, mandatory client masking, control frames (ping/pong/close), fragmentation, UTF-8 validation, close handshake. Strong — this is a real protocol, not an HTTP call, and the stdlib provides none of it. |
| **Hand-roll cost** | **High and misplaced.** ~500–1000 LOC of security-sensitive framing (client masking is mandatory; a framing bug is subtle and exploitable), plus ongoing maintenance — for commodity code that is NOT Korvun's value (the policy engine is). This is the "reinvent a solved, well-tested wheel" trap. |
| **Maintenance / cross-compile** | **Decisive.** Pure-Go, `CGO_ENABLED=0` → the ×6 cross-compile (incl. ARM/RPi + windows) stays trivial. **Zero transitive dependencies** → no dependency-tree growth (unlike a heavier SDK). Actively maintained, widely used. |
| **Risk / volatility** | Low. Small, stable, minimal API; reputable maintainer (Coder), high adoption. **Bounded by the seam**: the WS client is used only inside the Discord adapter (ADR-0033), behind the unchanged `channel.Channel` interface; swapping it later is additive and local. |

**Net:** stdlib is NOT a reasonable option here (Go ships no WebSocket), and
hand-rolling RFC 6455 spends effort on security-sensitive commodity code that is not
the product's value. `coder/websocket` passes the gate cleanly — zero transitive deps,
CGO-free, context-idiomatic — winning on the maintenance/cross-compile axis with risk
bounded by the ADR-0033 seam.

### Why `coder/websocket` over the alternatives

- **Hand-rolled RFC 6455** — rejected (four-axis, above): high-effort, security-
  sensitive, not the product's value.
- **`golang.org/x/net/websocket`** — rejected: a bare, low-level package the Go team
  does not recommend for new code (no ping/pong or close helpers, effectively frozen).
  Deprecated in practice.
- **`gorilla/websocket`** — a real option (zero deps, battle-tested), but its API is
  the older callback/deadline style, not context-first, and the package went through an
  archival/revival churn. `coder/websocket`'s first-class `context.Context` on every
  operation fits Korvun's context discipline (every cancellable op carries a ctx)
  better, at the same zero-transitive-dep cost.
- **A full Discord SDK (discordgo)** — rejected in ADR-0033: pulls a large surface for
  unused features; a thin adapter over a minimal WS client keeps the footprint to one
  small, auditable library.

## Consequences

- `go.mod` goes from 3 to 4 direct dependencies. The rule is "stdlib **if
  reasonable**"; a WebSocket client is not reasonably stdlib, so this is within the
  rule, not an exception to it. The addition is one pure-Go, zero-transitive-dep
  library used behind one adapter.
- The cross-compile ×6 `CGO_ENABLED=0` matrix is unaffected (pure Go).
- The dependency is testable behind a fake Gateway (an `httptest`/WS test server) with
  `-race` — no real Discord connection needed in the suite.
- Reversible: the library is confined to the Discord adapter behind `channel.Channel`;
  replacing it is a local change.

## Alternatives Considered

See "Why `coder/websocket` over the alternatives" above (hand-rolled RFC 6455,
`x/net/websocket`, `gorilla/websocket`, a full Discord SDK). The decision is
`coder/websocket` on the strength of: not-reasonably-stdlib + zero transitive deps +
CGO-free cross-compile + context-first API + seam-bounded risk.
