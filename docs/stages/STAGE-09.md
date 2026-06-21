# Stage 9 — Persistence (conversation memory)

> **Status:** closed
> **Started:** 2026-06-21
> **Closed:** 2026-06-21
> **ADRs:** [ADR-0018](../adr/0018-conversation-store-interface.md) (ADR-A, accepted),
> [ADR-0019](../adr/0019-sqlite-conversation-store.md) (ADR-B, accepted)

## Objective

Give Korvun memory. Through Stage 7 the Brain answered every message in
isolation (single-turn, stateless). Stage 9 lets the Brain answer message N
with the previous turns of the same conversation in context, and makes that
memory **durable** — it survives a process restart, even a graceful one.

This is the first capability persistence unlocks and the thing ADR-0014 §4
pre-committed to: conversation history is an **injected store keyed by
conversation**, never per-instance state on the Orchestrator (which would be a
data race across the router's workers).

The stage was split into **two ADRs of very different blast radius** (the same
discipline as Stage 6's ADR-0015 / ADR-0016), each framed with `/office-hours`
(premise challenge + forced alternatives) and `/plan-eng-review` (boring-by-
default, blast-radius, reversibility) before any code, and each `/review`-checked
on the code.

## The two phases

### Phase 1 — ADR-A (ADR-0018): the seam + in-memory store

Merged to master in `057ee73` (`feat/conversation-store`, `--no-ff`).

- **`internal/conversation`** — a pure leaf (imports only `envelope`): the
  canonical `Key` (`channel::conversation.id`), the value-only `Turn`
  (`Role`, `Content`, `Timestamp`, `Seq` — `ts`+`seq` carried from day one so
  retention is later an additive query, not a migration), `Role`, the
  **append-only `Store` seam** (`LoadRecent` + `Append` + the atomic-per-group
  `AppendTurns`), the in-memory `MemStore`, and `KeyFromEnvelope`.
- **`router`** delegates `ConversationKey` and aliases `MetaConversationID` /
  `ErrNoConversationID` to `conversation` — one canonical key composition, no
  import cycle, Telegram adapter and `DispatchInbound` behaviorally unchanged.
- **`Orchestrator`** takes an optional injected store (`WithConversationStore`):
  `LoadRecent` before dispatch, `AppendTurns` (user+assistant as one group)
  after a successful reply. It stays **stateless** (state in the store, never
  instance fields — closes ADR-0014 §4). No store / no conversation id → exact
  Stage 11 behavior (stateless, no dropped reply).

### Phase 2 — ADR-B (ADR-0019): the durable SQLite store

Merged to master in `65549cf` (`feat/sqlite-store`, `--no-ff`).

- **`internal/conversation/sqlite`** — a subpackage so `conversation` stays a
  pure leaf (mirroring `internal/model/{ollama,groq}`): `SqliteStore`
  implementing the same `Store` seam, backed by SQLite through the **pure-Go
  `modernc.org/sqlite` driver** (no cgo — decisive for the Pi/ARM cross-compile).
  Schema `turns(key, seq, role, content, ts)`, natural PK `(key, seq)`
  `WITHOUT ROWID`; opaque `key` (the seam hands an already-joined Key).
- **Concurrency = single serialized writer** (`db.SetMaxOpenConns(1)`):
  `SQLITE_BUSY` and write-write deadlock are structurally impossible. WAL +
  `busy_timeout` are set for robustness, but serialization comes from the
  one-connection pool. **`AppendTurns` wraps `SELECT MAX(seq)+1` and the inserts
  in ONE transaction per group** → group atomicity **and** crash-consistency (a
  crash mid-group commits the whole pair or none — closing what ADR-0018 §5
  deferred).
- **Boot lifecycle (reuses ADR-0017 §5 golden rule):** a configured store that
  fails to open → `Build` returns a named fatal error; no store configured →
  Orchestrator stays stateless. Path from an additive top-level `storage.path`
  config (empty → `<os.UserConfigDir>/korvun/korvun.db`, stdlib only).
- **Wiring:** the store is opened once in `app.Build`, shared across all brains
  (the Key namespaces by `channel::conversation.id`), owned as an `io.Closer`.
- **Durable through a graceful shutdown:** `brain.persistTurns` writes on a
  **cancellation-detached context** (`context.WithoutCancel` + a 5s timeout), so
  the final turn commits even though `router.Shutdown` cancels the context
  `Handle` runs under. `App.Shutdown` closes the store **only after the router
  drains cleanly**, so no in-flight `AppendTurns` races into a closing DB.

## What this persists, and what it does NOT

**Persisted:** conversation memory — the turns of a conversation, keyed by
`channel::conversation.id` (the chat, so group memory is one shared thread for
free). Durable across restarts.

**Out of scope (recorded, not silently dropped):**

- Stateful budgets / accounting persistence — a later stage.
- Long / analytical / queryable history — additive over the same table later.
- Postgres / multi-node / shared DB — a future `Store` impl behind the same
  seam; additive, no contract change.
- Compaction / retention / eviction — deferred; `ts`+`seq` on every row make it
  an additive query, not a migration.
- Per-sender-within-a-group keying — the canonical key stays the chat.

## The concurrency contract, verified against the router

The load-bearing constraint of the whole stage: **the router does not serialize
a conversation.** A brain has N competing worker goroutines with no
conversation→worker affinity, so two messages of the same conversation can be
handled at the same time. The store is therefore the new shared mutable state,
and its interface must be **append-only and atomic** — never a Brain-side
read-modify-write (which would lose a turn).

The contract is proven by the same load-bearing test in both implementations
(`-race`, `-count`): N goroutines `AppendTurns(user, assistant)` to the **same**
key → exactly 2N turns, each pair contiguous and identity-matched, `Seq` the
contiguous `0..2N-1` with no gap, duplicate, or PK collision. `MemStore` passes
via a mutex; `SqliteStore` passes via the single serialized writer.

## Review findings that shaped the design

- **F3 — turn-pair ordering (ADR-A).** The drafted design appended the user and
  assistant turns as two independent `Append`s; `/review` + copilot found that
  under `brainWorkers > 1` two concurrent messages interleave into a
  non-alternating, provider-rejected history. Fixed by the atomic-per-group
  `AppendTurns` (one critical section, consecutive `Seq`, pair stays contiguous).
- **Durability through a clean shutdown (ADR-B).** The adversarial `/review`
  found that `router.Shutdown` cancels the context `Handle` runs under, so an
  in-flight `AppendTurns` on the durable store was rolled back at a graceful
  shutdown — silently losing the user's last turn (the most visible thing a
  durable-memory store can drop). Fixed by the cancellation-detached persist
  context; a follow-up review then caught a `persist`-vs-`Close` race, fixed by
  gating `store.Close` on a clean router drain.
- Two AUTO-FIXes from the same `/review`: a zero-value `Timestamp` corrupting to
  ~1754 on round-trip (normalized to a 0 sentinel for parity with `MemStore`),
  and a path containing `?` swallowing the DSN pragmas (DSN now built with
  `net/url`).
- Deferred by review, on purpose: a real mid-group **crash** test (atomicity is
  structural — one transaction + `defer Rollback` — verified by reading, not by
  fault injection; a real crash test is for a hardening stage), and moving the
  synchronous persist off the reply critical path.

## Quality gate

`make quality` green with `-race` over the whole tree. Per-package coverage:

| Package                         | Coverage |
|---------------------------------|----------|
| `internal/conversation`         | 97.2%    |
| `internal/conversation/sqlite`  | 86.6%    |
| `internal/brain`                | 92.6%    |
| `internal/app`                  | 90.6%    |
| `internal/config`               | 98.6%    |
| `internal/router`               | 96.0%    |

**Cross-compile ×6 (`CGO_ENABLED=0`, `{linux,windows,darwin} × {amd64,arm64}`)
green** with the driver in the import graph — the dependency gate the pure-Go
driver was chosen to pass.

**`go.mod` now has TWO direct dependencies:**
`github.com/go-telegram/bot v1.21.0` and `modernc.org/sqlite v1.53.0` — the
first external dependency beyond Telegram, adopted behind the `Store` seam after
a four-axis test (won on the maintenance / cross-compile axis) and a Context7 +
`go get` dependency gate.

## Outcome

Korvun has its first persistent memory. The binary an operator runs on a
Raspberry Pi remembers conversations across reboots, including a graceful
shutdown. The `Store` seam keeps the durable engine swappable: a future Postgres
or multi-node store is a second implementation of one interface, no Orchestrator
or app-contract change. What remains is operability, not more memory.
