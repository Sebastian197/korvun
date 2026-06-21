# ADR-0018: Conversation store — the injected, append-only interface (Stage 9, ADR-A)

> **Status:** accepted
> **Date:** 2026-06-21
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Stage 9 adds persistence. The first thing persistence enables — and the thing
ADR-0014 §4 already pre-committed to — is **conversation memory**: the Brain
should answer message N with the previous turns of the same conversation in
context, instead of treating every message in isolation (the single-turn,
stateless v1 of Stage 7).

ADR-0014 §4 is binding and frames this ADR:

> when conversation memory / history lands (Stage 9), it is an **injected store
> keyed by conversation** (e.g. `sender`/`chat` id), NOT fields on the
> `Orchestrator` instance. Per-instance mutable history would be **shared state
> across the router's workers** — the exact race class §2 warns about […] the
> Brain stays a stateless orchestrator over a `ConversationStore` interface.

Stage 9 is split into **two ADRs** so the two decisions, which have very
different blast radii, are decided and reviewed separately (the same split
discipline as Stage 6's ADR-0015 / ADR-0016):

- **ADR-A — this ADR.** The `ConversationStore` interface + an in-memory
  implementation + its injection into the stateless `Orchestrator` + the
  **concurrency contract**. Zero new dependencies. Closes ADR-0014 §4.
- **ADR-B — future.** A durable SQLite implementation behind the same
  interface: schema, bootstrap/migration, the `modernc.org/sqlite` (pure-Go,
  no cgo — chosen for the trivial Pi/ARM cross-compile) dependency, and the
  boot-fatal-vs-stateless lifecycle decision. **Not this ADR.** ADR-A makes
  ADR-B a drop-in: a second implementation of one interface.

### The single line that frames the whole ADR

**The router does not serialize a conversation.** A brain has one bounded
inbound queue drained by N competing worker goroutines (`WithBrainWorkers`,
default 1 but `>1` is first-class), with **no conversation→worker affinity**
(`router.go:151-181`, `runBrainWorker` at `router.go:305-317`). Two messages of
the same conversation can therefore be handled by two workers **at the same
time**. So the store is the new shared mutable state across workers, and its
interface must be **append-only and atomic** — never a read-modify-write the
Brain performs, which would lose a turn under concurrency (the "intersection of
two features" race class the project has hit twice; HANDOFF "honest record").
This is the load-bearing constraint of the whole design.

### External-docs verification (per CLAUDE.md non-negotiable)

ADR-A adds **no external dependency** (stdlib only: `sync`, `context`, `time`).
No Context7 verification is required for this ADR. The SQLite driver
verification (Context7) is a prerequisite of **ADR-B**, recorded there.

## Decision

### 1. The interface — append-only, atomic, two methods

```go
// Package conversation owns the conversation-memory domain: the canonical
// conversation key, the Turn record, and the Store seam the Brain reads/writes.
package conversation

// Store persists and retrieves the turns of a conversation.
//
// Implementations MUST be safe for concurrent use by multiple goroutines,
// INCLUDING concurrent Append on the same Key. The router does not serialize a
// conversation (N workers, no per-conversation affinity), so two goroutines may
// Append to the same Key simultaneously; no turn may be lost. This is the same
// concurrency discipline model.Model and the fan-out carry.
type Store interface {
    // LoadRecent returns up to the last n turns for key, oldest first.
    // It is a best-effort snapshot: it MAY omit a turn that a concurrent
    // Append has not yet committed. Losing a write is NOT acceptable; missing
    // an in-flight read is. n <= 0 returns no turns, nil error. An unknown key
    // returns an empty slice, nil error (absence is not an error).
    LoadRecent(ctx context.Context, key Key, n int) ([]Turn, error)

    // Append atomically adds one turn to key and returns it with its
    // store-assigned Seq filled in (the caller never sets Seq). It is the only
    // writer path; the Brain never read-modify-writes history. Concurrent
    // Appends to the same key are serialized by the implementation.
    Append(ctx context.Context, key Key, turn Turn) (Turn, error)
}
```

**Why append-only and not get/set.** A get/set (or "load history, mutate,
store history") invites the Brain to do a read-modify-write: worker-1 and
worker-2 both `LoadRecent("chat#42")`, both append locally, both store — last
writer wins, one turn lost. `Append` makes the store the single ordering point
for writes. `LoadRecent` is explicitly a snapshot whose staleness (missing a
turn another worker is mid-Appending) is acceptable for building reply context;
**only losing a committed write is unacceptable**, and append-only prevents it.

### 2. The Turn — minimal now, retention-ready for free

```go
// Role is the author of a turn. Kept dependency-free (no model import) so
// conversation stays a leaf; the Orchestrator maps Role <-> model roles.
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleSystem    Role = "system"
)

// Turn is one message in a conversation's history.
type Turn struct {
    Role      Role
    Content   string
    Timestamp time.Time // when the turn was recorded
    Seq       int       // monotonic per-key sequence, assigned on Append
}
```

`Timestamp` and `Seq` are carried **now even though ADR-A uses neither for
retention** (F6). A future compaction / retention / "last N by recency" query
is then an **additive read over an existing column**, not a schema migration
once SQLite (ADR-B) lands. `Seq` is assigned by the store on `Append` (the only
writer that can order it correctly), monotonic per key.

### 3. The key — reuse the router's composition, do not redefine it; avoid the import cycle

The router already computes the canonical conversation key
(`router.ConversationKey` = `Channel + "::" + Meta["conversation.id"]`,
`router.go:285-297`) and `DispatchInbound` already **requires** it on every
inbound (`ErrNoConversationID`, `router.go:214`). The Telegram adapter fills it
with the **chat id** (`telegram/adapter.go:293`), so the canonical conversation
is the **chat**, not the sender — which makes group memory "share one thread per
chat" fall out for free (and defers per-sender-within-a-group semantics).

The key must be a **named type** at the interface border for type safety
(`type Key string` — not a bare `string` anyone could pass, not a new struct
that re-derives the composition).

**The import-cycle constraint decides where the type lives.** `router` imports
`brain` (`router.go:12`); `brain` does not import `router`. So the
`Orchestrator` (in `brain`) cannot call `router.ConversationKey` — that would be
`brain → router → brain`, a cycle. The composition therefore relocates to a
**leaf package both can import**:

```
            internal/envelope        (pure leaf, imports no internal pkg)
                  ▲
                  │
          internal/conversation      (NEW leaf: Key, Turn, Store, MemStore,
                  ▲   ▲               KeyFromEnvelope, MetaConversationID)
                  │   │
        ┌─────────┘   └─────────┐
   internal/brain          internal/router
        ▲                        ▲
        └────────────┬───────────┘
                cmd/korvun        (top: wires everything)
```

`internal/conversation` imports only `internal/envelope`. It owns:

- `type Key string`
- `MetaConversationID` (relocated from `router`) and
  `KeyFromEnvelope(env *envelope.Envelope) (Key, error)` — the **single**
  canonical composition (error when the conversation id is absent/empty, the
  same condition `DispatchInbound` rejects with `ErrNoConversationID`).
- `Turn`, `Role`, `Store`, and the in-memory implementation.

`router.ConversationKey` becomes a thin delegator
(`return string(conversation.KeyFromEnvelope(env))`) and `router.MetaConversationID`
becomes an alias (`= conversation.MetaConversationID`), so **`DispatchInbound`
and the Telegram adapter are untouched** and there is exactly one definition of
the composition. The `Orchestrator` calls `conversation.KeyFromEnvelope(env)` in
`Handle` — `brain → conversation → envelope`, no cycle.

**Dependency direction (documented, as required):** `conversation` is a leaf
depending only on `envelope`; `brain` and `router` both depend on
`conversation`; nothing depends back into a cycle.

### 4. N is the Brain's parameter, not the store's config

How many turns to load is a **Brain concern** — the Brain owns the request
shape and the token budget. So `n` is an argument to `LoadRecent`, passed by the
Orchestrator. The store stays dumb: it stores and returns turns, it does not
decide how many matter.

### 5. Injection — the Orchestrator stays stateless; the store is optional

The `Orchestrator` gains a `store conversation.Store` field, injected via a new
`WithConversationStore(...)` Option (the 7th injected field, alongside
`coord`/`models`/`policy`/`fallback`/`systemPrompt`/`logger`). **State lives in
the store, never in `Orchestrator` instance fields** — the instance stays safe
to share across the router's N workers (closes ADR-0014 §4; A4 there stays
rejected).

`Handle` becomes:

```
key, _ := conversation.KeyFromEnvelope(env)     // canonical, no router import
history, _ := store.LoadRecent(ctx, key, n)     // snapshot before dispatch
... translate(history + this message) -> request
... coord.Run -> policy.Apply -> reply ...
store.Append(ctx, key, userTurn)                // atomic, append-only
store.Append(ctx, key, assistantTurn)           // independent append (see below)
... translate(reply) -> outbound Envelope
```

**Pair atomicity — decided for ADR-A, flagged for ADR-B.** ADR-A appends the
user turn and the assistant turn as **two independent `Append`s**. With the
in-memory store a crash is moot (nothing persists), and a half-written pair is
self-healing: the next message simply continues the history. **ADR-B (durable
SQLite) MUST reconsider this**: with a durable store a crash between the two
Appends could leave an **orphaned user turn persisted with no reply**, so ADR-B
decides whether to commit the pair atomically (e.g. an `AppendTurns(ctx, key,
...Turn)` in one transaction) or to tolerate and self-heal orphans. The
interface in ADR-A does not foreclose either: `AppendTurns` is an additive
method, not a breaking change.

**The store is optional.** With no store injected, `Handle` behaves exactly as
today: single-turn, stateless, no memory. This preserves Stage 11's behavior
unchanged and is the natural seam for ADR-B's "no store configured → run
stateless; store configured but unopenable → boot-fatal" decision.

### 6. The in-memory implementation — permanent, not a throwaway

```go
// MemStore is the in-memory Store: map[Key][]Turn guarded by a mutex.
// It is the permanent test double for the Orchestrator AND the enforcer of the
// Store concurrency contract under -race. It is NOT a discardable prototype.
type MemStore struct {
    mu sync.Mutex
    m  map[Key][]Turn
}
```

`Append` takes `mu.Lock()`, assigns `Seq = len(m[key])`, appends, and returns
the stored `Turn` (Seq filled). `LoadRecent` takes the lock, copies out the last
`n` turns (a copy, so the caller cannot mutate stored state). No goroutines of its own — the only delicate thing is the
lock discipline that satisfies the contract. It earns its place permanently:
every `Orchestrator` test needs a fake store anyway, and this one doubles as the
`-race` contract proof.

### 7. The load-bearing test (TDD, mandatory)

The contract is "no committed write is lost under concurrent Append to the same
key." The test that proves it, written **red first**:

> N goroutines each `Append` one turn to the **same** Key, concurrently, under
> `-race`, repeated (`-count`). Assert the key ends with **exactly N** turns
> (no lost update), and that `Seq` values are the contiguous set `0..N-1` (no
> duplicate or skipped sequence). A naive non-locked map impl fails this (lost
> writes and/or `-race` report); the locked `MemStore` passes.

This is the test that justifies the whole append-only design. Build order:
red on this test against a stub → implement `MemStore` → green → then the
`Orchestrator` wiring with its own `Handle` tests (history-in-context,
store-optional-behaves-as-today, Append-after-reply).

### 8. Structural concurrency? No — but the contract is the risk

`MemStore` holds a lock but spawns no goroutines, so this is **not** structural
concurrency the way the router or fan-out are. The delicate part is the
**concurrency contract** (§1) and its proof (§7), exercised on the hot path
(`Handle`) with new stateful behavior. Therefore ADR-A ships on a **short
feature branch with `/review`** (not direct-to-master like the stateless Stage 7
glue): the concurrent-Append test + `MemStore` + the `Handle` wiring are exactly
where an independent review pays off. TDD per phase, `-race` mandatory,
`make quality` green over the whole tree before close.

## Consequences

### What this enables

- **Conversation memory** — the Brain answers with prior turns in context;
  the first capability persistence unlocks, and the close of ADR-0014 §4.
- **ADR-B is a drop-in** — SQLite becomes a second `Store` implementation
  behind an interface whose shape and concurrency contract are already proven.
  No Orchestrator change when the durable engine arrives.
- **A reusable conversation-key home** — `internal/conversation` centralizes a
  key that was already anticipated (`router.ConversationKey`) but dormant
  (zero non-test callers today), with one canonical composition.
- **Group memory for free** — keying on chat id (what the router already
  produces) means a group thread shares one memory without extra design.

### What this asks / costs

- **A new leaf package** (`internal/conversation`) and a small, mechanical
  change to `router` (delegate `ConversationKey`, alias `MetaConversationID`).
  No behavior change in the router or Telegram adapter.
- **A 7th injected field on the Orchestrator** and a `Handle` that now does a
  Load before and two Appends after — the hot path gains state, hence the
  branch+`/review`.
- **A concurrency contract that every future `Store` impl inherits** — ADR-B's
  SQLite must satisfy it (WAL + `busy_timeout`, or a serialized writer).

### Trade-offs accepted

- **`LoadRecent` is best-effort, not linearizable.** It may miss an in-flight
  Append. Accepted: reply context tolerates a one-message-stale snapshot;
  losing a committed write does not, and append-only prevents that.
- **Memory is unbounded in ADR-A.** `MemStore` grows per key with no eviction,
  and ADR-B defers compaction. Accepted for the minimal cut; `Timestamp`/`Seq`
  in `Turn` make retention an additive query later, not a migration.
- **The canonical key composition relocates** from `router` to
  `conversation`. Accepted over a key-function-injection alternative (A2) that
  would keep it in `router` but leave the canonical conversation key in an
  arguably wrong home and add constructor plumbing.

## Alternatives Considered

### A1 — Memory as `Orchestrator` instance fields
**Rejected, pre-emptively, by ADR-0014 §4/A4.** Per-instance history is shared
mutable state across the router's N workers — a data race. The whole point of an
injected store is to keep the Brain instance stateless and shareable.

### A2 — Keep the key in `router`; inject a key-function into the Orchestrator
Keep `router.ConversationKey` canonical; give the Orchestrator a
`KeyFunc func(*envelope.Envelope) conversation.Key`, wired in `cmd/korvun` as
`conversation.Key(router.ConversationKey(env))`. Avoids touching `router`, but
adds constructor plumbing and leaves the canonical conversation key in `router`
(which imports `brain`) rather than a domain leaf. Rejected in favor of the leaf
package (§3); the `router` change there is two mechanical lines with no behavior
change. **This is the one sub-decision worth confirming in review** (see Open
questions).

### A3 — get/set interface (`Load`/`Save` whole history)
Rejected: invites a Brain-side read-modify-write that loses turns under the
non-serialized worker model (§1). Append-only removes the hazard at the type
level.

### A4 — Put `Store` in `internal/brain` instead of its own package
Workable (brain is the only consumer in ADR-A), but `router` also needs the key
type, and brain↔router cannot share a type without a cycle or duplication. A
neutral leaf (`conversation`) that both import is the clean resolution and gives
the durable store impl (ADR-B) a non-brain home.

### A5 — Define the key as a bare `string`
Rejected: no type safety at the border; any string would satisfy the signature.
A named `type Key string` costs nothing and prevents passing an arbitrary string
where a conversation key is meant.

## Out of scope (recorded, not silently dropped)

- **Durable storage / SQLite / schema / migrations / the new dependency** —
  ADR-B (next), with Context7 verification of `modernc.org/sqlite`.
- **Boot-fatal-vs-stateless lifecycle** on store open failure — ADR-B (the
  optional-store seam in §5 is where it attaches).
- **Stateful budget, long history/retention/compaction, brain state, queryable
  / analytical history, multi-node / shared DB** — later stages; named here so
  the minimal cut stays minimal.
- **Per-sender-within-a-group keying** — deferred; the canonical key is the chat
  (§3). Revisited only when a group channel with per-sender memory is needed.

## Resolved in review (copilot, 2026-06-21)

1. **Key home — leaf package, `router` delegating.** Confirmed (§3). `router`
   keeps one delegating `ConversationKey` and a `MetaConversationID` alias; the
   key-function-injection alternative (A2) is rejected.
2. **Pair atomicity — two independent `Append`s in ADR-A; reconsidered in
   ADR-B.** Confirmed (§5). The explicit ADR-B note is recorded: a durable store
   must not leave orphaned user turns persisted with no reply after a crash
   between the two Appends; `AppendTurns` stays an additive, non-breaking
   escape hatch.
3. **`Role` home — leaf-local in `conversation`.** Confirmed (§2). Any
   translation to a `model` role type lives in the `Orchestrator`, keeping
   `conversation` a pure leaf with no `model` dependency.
