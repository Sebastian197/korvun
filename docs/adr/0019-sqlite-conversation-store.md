# ADR-0019: Conversation store — the durable SQLite implementation (Stage 9, ADR-B)

> **Status:** accepted
> **Date:** 2026-06-21
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Stage 9 adds persistence, split into two ADRs of very different blast radii
(the same split discipline as Stage 6's ADR-0015 / ADR-0016):

- **ADR-A (ADR-0018, accepted, on master).** The `conversation.Store` seam —
  append-only, atomic, three methods (`LoadRecent` + `Append` + the
  atomic-per-group `AppendTurns`) — its in-memory `MemStore`, and its injection
  into the **stateless** `Orchestrator` via `WithConversationStore`. Zero new
  dependencies. It made ADR-B a drop-in: a second implementation of one
  interface, with a concurrency contract already proven under `-race`.
- **ADR-B — this ADR.** A durable implementation of that same seam, backed by
  SQLite through the pure-Go `modernc.org/sqlite` driver. Schema, bootstrap,
  the dependency, the concurrency strategy, the crash-consistency guarantee,
  the boot lifecycle, and the config→store wiring. **No interface change** —
  ADR-A's seam is the contract, unchanged.

**Stage 9 closes when this ADR is accepted, implemented (TDD, `-race`), and
`docs/stages/STAGE-09.md` is written.**

### The constraints ADR-A hands ADR-B (binding)

ADR-0018 is the contract this ADR must satisfy, verbatim from
`internal/conversation/conversation.go:68-100`:

1. **Safe for concurrent use, including concurrent `Append`/`AppendTurns` on the
   same `Key`** — the router does not serialize a conversation (N workers, no
   per-conversation affinity). No committed turn may be lost.
2. **`AppendTurns` is atomic per group** — a user+assistant pair gets consecutive
   `Seq` and stays contiguous, never interleaved with another concurrent
   message's turns (which would yield a non-alternating, provider-rejected
   history).
3. **`LoadRecent` is a best-effort snapshot** — it may miss an in-flight write,
   but never loses a committed one. `n <= 0` → no turns; unknown key → empty
   slice; neither is an error.
4. **`Seq` is store-assigned on write** (callers leave it zero), monotonic per
   key; `Turn` is value-only (`Role`, `Content`, `Timestamp`, `Seq`).

ADR-0018 §"What this asks / costs" already pre-committed the durable engine to
honour this "with WAL + `busy_timeout`, or a serialized writer", and ADR-0018
§5 / the reconciliation note deferred exactly one thing to ADR-B: **crash
atomicity of the group** (a durable store must not leave a half-written group
after a crash). This ADR closes that.

### External-docs verification — the dependency gate (per CLAUDE.md non-negotiable)

This is the **first external dependency beyond `github.com/go-telegram/bot`**.
`go.mod` goes from one direct dependency to two; the SQLite driver is the only
addition.

**Context7 verification (`/gitlab_cznic/sqlite`, source repo
`gitlab.com/cznic/sqlite`, reputation High; `/websites/pkg_go_dev_modernc_org_sqlite`).**

- **Bundled SQLite 3.53.0 on every supported target**, including `linux/arm`,
  `linux/arm64`, `windows/arm64`, `darwin/arm64`, `riscv64`, `ppc64le`,
  `s390x`, `loong64` — the exact CI cross-compile ×6 matrix, all on the same
  SQLite version. This is the decisive property and the reason the driver is
  chosen (below).
- **Concurrency knobs confirmed present:** WAL via DSN `_pragma=journal_mode(WAL)`;
  `busy_timeout` via `_pragma=busy_timeout(N)`; transaction lock mode via
  `_txlock=deferred|immediate|exclusive`. The `database/sql` contract holds
  (*"the returned connection is only used by one goroutine at a time"*) — the
  driver does not subvert pool-based goroutine safety.
- **Honest gap:** Context7 surfaced the API surface and the concurrency knobs,
  **not** the module semver and **not** a changelog of concurrency defects in
  the pure-Go port. Those are not derivable from Context7 and were pinned at the
  source (below) rather than from memory.

**Dependency gate (`go get` / proxy query, this session):**

- **Pinned semver: `modernc.org/sqlite v1.53.0`** (latest stable, resolved via
  the module proxy with `go list -m modernc.org/sqlite@latest`; `go.mod` left
  untouched — the bump lands with the implementation, not this ADR).
- **Concurrency-issue skim:** the public `database/sql`-level discourse on this
  driver is dominated by the generic SQLite `SQLITE_BUSY` / "database is locked"
  class, whose standard remedies are WAL + `busy_timeout` + `BEGIN IMMEDIATE`
  or a single serialized writer. No driver-specific data-race or
  goroutine-safety defect surfaced. The cznic issue tracker is JS-rendered and
  could not be enumerated headlessly; this is a best-effort skim, not an
  exhaustive audit. **Mitigation that makes this safe regardless:** the chosen
  concurrency strategy (single serialized writer, §3) structurally eliminates
  the entire `SQLITE_BUSY`/locked class — Korvun never issues two concurrent
  writes to the DB.

**Four-axis dependency test (CLAUDE.md: capability vs hand-roll cost vs
maintenance vs risk/volatility):**

| Axis | Verdict |
|------|---------|
| Capability gain | Durable, transactional, crash-consistent, indexed, queryable storage. Strong — hand-rolling a durable append store means owning fsync, locking, and crash recovery. |
| Hand-roll cost | Very high. A durable, concurrent, crash-consistent append store is exactly the "intersection of two features" race class the project has hit twice (HANDOFF honest record). |
| Maintenance / cross-compile | **Decisive.** Pure-Go, no cgo → `CGO_ENABLED=0` keeps the ×6 cross-compile to ARM/RPi trivial; same SQLite 3.53.0 on all targets. A cgo driver (`mattn/go-sqlite3`) would break or massively complicate that matrix. This is the whole reason. |
| Risk / volatility | Large transpiled-from-C codebase; primarily single-maintainer (bus-factor); the pure-Go port can diverge subtly from cgo SQLite on edge behaviour. **Bounded by the seam** (ADR-0018): the impl is behind `conversation.Store`; swapping to cgo or Postgres later is additive. Acceptable. |

**Net:** the driver passes, winning on the maintenance/cross-compile axis exactly
as anticipated, with risk bounded by the ADR-0018 seam.

## Decision

### 1. Where the impl lives — a subpackage, so `conversation` stays a pure leaf

`internal/conversation` is a **pure leaf** (imports only `internal/envelope`).
Putting `database/sql` and the SQLite driver import inside it would destroy that
invariant. So the durable store lives in a **subpackage**, mirroring the
existing `internal/model/{ollama,groq}` layout:

```
            internal/envelope            (pure leaf)
                  ▲
          internal/conversation          (pure leaf: Key, Turn, Role, Store, MemStore)
                  ▲                 ▲
        ┌─────────┘                 └──────────────┐
   internal/brain               internal/conversation/sqlite   (NEW)
   internal/router                  imports: conversation + database/sql
        ▲                                    + _ "modernc.org/sqlite"
        │                                     ▲
        └──────────────┬──────────────────────┘
                  internal/app                  (opens + owns the store; wires it)
```

- `internal/conversation/sqlite` is the **only** package that imports the
  driver (`_ "modernc.org/sqlite"`, registered as the `"sqlite"` `database/sql`
  driver). The external dependency is confined to one package.
- It exposes `New(path string) (*Store, error)` returning a value that
  implements `conversation.Store` **and** `io.Closer`.
- `internal/app` imports `internal/conversation/sqlite` to open the store at
  boot. `internal/conversation` itself gains **no** new import — its leaf status
  is preserved.

### 2. Schema — opaque key, natural composite PK, `WITHOUT ROWID`

```sql
CREATE TABLE IF NOT EXISTS turns (
    key     TEXT    NOT NULL,   -- the conversation Key, opaque ("channel::conv-id")
    seq     INTEGER NOT NULL,   -- store-assigned, monotonic per key (ADR-0018 §2)
    role    TEXT    NOT NULL,   -- "user" | "assistant" | "system"
    content TEXT    NOT NULL,
    ts      INTEGER NOT NULL,   -- unix nanoseconds, UTC
    PRIMARY KEY (key, seq)
) WITHOUT ROWID;
```

- **Opaque `key TEXT`, NOT decomposed into `(channel, conversation)`.** The seam
  hands the store a `conversation.Key` that is **already** the joined
  `Channel + "::" + conversationID` (`conversation.go:108-117`). Re-parsing it on
  `"::"` is fragile (a conversation id may itself contain `"::"`), and widening
  the seam to pass channel and id separately would break the ADR-0018 contract.
  So the store treats the Key as one opaque token. **Future retention by channel
  is an additive query** — `WHERE key LIKE 'telegram::%'` — not a schema change.
- **Natural composite PK `(key, seq)`.** It *is* the identity; it serves
  `LoadRecent`'s access path directly; and it makes a Seq collision **fail loudly
  as a PRIMARY KEY violation** rather than silently lose a turn — the durable
  echo of ADR-0018 §7's "no duplicate or skipped sequence" invariant.
- **`WITHOUT ROWID`** clusters rows physically in `(key, seq)` order, so
  `LoadRecent`'s `WHERE key = ? ORDER BY seq DESC LIMIT n` is a direct b-tree
  suffix scan with no secondary-index hop. (The PK index alone would suffice;
  this is a small, justified locality optimization for the one hot read.)
- **No secondary index in ADR-B scope.** Retention/compaction indexes are
  additive later; `ts` + `seq` already travel on every row.

**`ts` encoding — INTEGER unix-nanoseconds (UTC), converted explicitly in Go.**
`Turn.Timestamp` is a `time.Time` the Orchestrator sets. The store writes
`t.Timestamp.UTC().UnixNano()` and reads back `time.Unix(0, n).UTC()` — an
explicit `int64` round-trip with no timezone or format ambiguity, compact, and
correctly ordered. The driver's `_time_format` / `_time_integer_format` magic is
**not** used; conversion stays in our code (project value: explicit over clever).
RFC3339 TEXT was the alternative (human-readable under the `sqlite3` CLI),
rejected for the integer's compactness, ordering, and zero-ambiguity round-trip.

### 3. Concurrency — single serialized writer (Option B), transaction for crash-consistency

**`db.SetMaxOpenConns(1)`.** The `database/sql` pool holds exactly one
connection, so every statement against the DB serializes through it. This makes
**`SQLITE_BUSY` and write-write deadlock structurally impossible** — there is
never a second connection to contend with. At a chat-gateway's throughput
(human messages, not a high-QPS OLTP store) the cost — reads also queue behind
writes — is irrelevant. This is the boring-by-default, 3am-safe choice.

**DSN (WAL on anyway, for robustness):**

```
file:<path>?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)
```

WAL and `busy_timeout` are belt-and-suspenders (resilience to WAL checkpoints
and any external reader such as a `sqlite3` inspection), **but the serialization
guarantee comes from `MaxOpenConns(1)`, not from `busy_timeout`.** `foreign_keys`
is on for future-proofing (the single `turns` table has none today).

**`AppendTurns` wraps `SELECT MAX(seq)+1` and the `INSERT`s in ONE transaction
per group** — not for concurrency (the single writer already provides it) but
for **crash-consistency**: a crash mid-group commits the whole user+assistant
pair or none of it. This closes precisely what ADR-0018 §5 / the reconciliation
note deferred to ADR-B (no orphaned user turn persisted with no reply). Sketch:

```go
tx, _ := db.BeginTx(ctx, nil)                 // deferred BEGIN is sufficient: single writer
var base int                                  // next seq for this key
tx.QueryRowContext(ctx,
    `SELECT COALESCE(MAX(seq), -1) + 1 FROM turns WHERE key = ?`, key).Scan(&base)
for i, t := range turns {
    tx.ExecContext(ctx,
        `INSERT INTO turns(key, seq, role, content, ts) VALUES (?,?,?,?,?)`,
        key, base+i, string(t.Role), t.Content, t.Timestamp.UTC().UnixNano())
}
tx.Commit()                                   // group is all-or-nothing
```

`Append` (single turn) delegates to `AppendTurns(key, turn)` — one source of Seq
logic, exactly as `MemStore.Append` delegates to `MemStore.AppendTurns`.
`LoadRecent` runs `SELECT role, content, ts, seq FROM turns WHERE key = ? ORDER
BY seq DESC LIMIT ?`, then reverses the slice in Go to oldest-first (the seam's
order).

**Why a plain (deferred) transaction is safe here.** With one connection there
is no concurrent writer to read a stale `MAX(seq)` and collide; serialization
makes `BEGIN IMMEDIATE` unnecessary. `IMMEDIATE` belongs to Option A (below),
where `MaxOpenConns > 1` reintroduces concurrent writers.

**Option A recorded as a future variant.** If a load profile ever justifies
concurrent reads during writes (improbable for chat memory): WAL + `MaxOpenConns
> 1` + `_txlock=immediate` so each `AppendTurns` takes the write lock from its
first statement (mandatory then, to stop two groups reading the same `MAX(seq)`).
That is strictly more moving parts than Korvun needs today; it is the documented
upgrade path, not the ADR-B decision.

### 4. Bootstrap — open, configure, migrate, ping, all fatal at boot

`sqlite.New(path)`:

1. `sql.Open("sqlite", dsn)` (lazy — does not connect yet).
2. `db.SetMaxOpenConns(1)` (§3).
3. `CREATE TABLE IF NOT EXISTS turns (...)` — first boot creates it; later boots
   no-op and open the existing file. This `Exec` forces a real connection, so a
   corrupt, unwritable, or locked DB file fails **here**, not on first message.
4. Return the `*Store` (which holds the `*sql.DB`) as a `conversation.Store` +
   `io.Closer`.

Any failure in 1–3 is returned as a **named** error and is fatal at boot (§5).

**Path — stdlib only, no XDG library.** Default to
`<os.UserConfigDir()>/korvun/korvun.db` (cross-platform: `~/.config` on Linux,
`~/Library/Application Support` on macOS, `%AppData%` on Windows), overridable by
config. `os.UserConfigDir` is stdlib, so **`go.mod` stays at +1 (the driver
only), never +2.** The parent directory is created with `os.MkdirAll` before open.

### 5. Boot lifecycle — reuse ADR-0017 §5's golden rule, no new principle

`internal/app` already enforces the golden rule (`app.go:11-15`): configuration
and boot errors are **fatal and named** (`Build` returns an error); a provider
unreachable at *runtime* degrades, it does not fail boot.

- **Store configured + fails to open** (bad path, corrupt file, failed
  `CREATE TABLE`) → `Build` returns a named fatal error. The operator asked for
  persistence; silently dropping to stateless would be a surprise data-loss.
  This is the same shape as the `getMe`/missing-secret boot-fatals — no new
  principle.
- **No store configured** → no store injected → `Orchestrator` stays exactly
  stateless (Stage 11 / ADR-0018 behaviour, unchanged).

### 6. Wiring — open once, share across brains, own the Closer

The `Key` namespaces by `channel::conversation`, so brains do **not** need
separate stores. Therefore:

- **Open once in `app.Build`, before the `for _, bc := range cfg.Brains` loop**
  (`app.go:119`), from the resolved config path.
- **Inject the same store into every brain** via the existing
  `brain.WithConversationStore(store)` Option in `buildBrain` (`app.go:152-171`)
  — `buildBrain` takes the store (or the builder carries it).
- **`App` owns the store's `io.Closer`.** `App` currently holds `router` +
  `channels` (`app.go:54-58`); it gains a `store io.Closer` (nil when no store
  is configured). **`Shutdown` closes it LAST**, after draining the router, so
  the final in-flight `AppendTurns` lands before `Close`:

  ```
  stop channels  →  router.Shutdown (drains brain workers; last AppendTurns commits)  →  store.Close
  ```

  This extends the existing ADR-0008 shutdown order (`app.go:276-283`); a nil
  store Closer is skipped.

**Config — additive, top-level, optional:**

```json
"storage": { "path": "/var/lib/korvun/korvun.db" }
```

Absent (or absent `path`) → stateless, no store opened. Additive over the Stage
11 JSON descriptor (`config.Config`): existing configs keep working untouched,
no migration. An empty/omitted `path` with the `storage` block present resolves
to the §4 default.

### 7. Tests (TDD, mandatory, `-race`)

- **Re-run ADR-0018 §7's load-bearing contract test against `SqliteStore`:** N
  goroutines `AppendTurns` to the **same** Key concurrently, under `-race`,
  `-count`; assert exactly N groups land, `Seq` is the contiguous `0..N-1` set
  (no duplicate/skip), and each group stays contiguous. The single writer must
  make this pass with zero `SQLITE_BUSY`. This is the durable mirror of the
  proof that justified the seam.
- **Crash-consistency intent:** a test that a failed/rolled-back `AppendTurns`
  (e.g. an injected error on the second insert) leaves **zero** rows for the
  group — the pair is all-or-nothing.
- **`LoadRecent` semantics:** oldest-first order; `n <= 0` → empty; unknown key
  → empty; `n` larger than history → all turns.
- **Bootstrap:** fresh file creates the table; reopening an existing file
  preserves rows; an unwritable path returns a named error (boot-fatal path).
- **Round-trip:** `Timestamp` survives the unix-nano `int64` conversion to UTC.
- **App wiring:** store opened once and shared; `Shutdown` closes it after the
  router drains; no store configured → stateless (existing behaviour).

`make quality` green with `-race` over the whole tree before close;
coverage ≥ 85% (core) on the new package.

## Consequences

### What this enables

- **Durable conversation memory** — history survives restarts; the binary the
  operator runs on a Pi remembers conversations across reboots.
- **Crash-consistency of the user+assistant pair** — the one thing ADR-0018
  deferred is closed by the per-group transaction.
- **Stage 9 closes** — the persistence stage is done once this lands with
  `STAGE-09.md`.
- **Postgres / cgo / multi-node remain drop-ins** — same `conversation.Store`
  seam; a future `internal/conversation/postgres` is additive, no Orchestrator
  or app-contract change.

### What this asks / costs

- **`go.mod` gains its first non-Telegram dependency** (`modernc.org/sqlite
  v1.53.0`), confined to `internal/conversation/sqlite`. Five stages shipped at
  one direct dependency; this is the justified second, behind a seam.
- **`App` gains an owned resource** (`io.Closer`) and a shutdown-ordering
  obligation (close after router drain).
- **An additive top-level config block** (`storage.path`).
- **The single-writer serialization** trades read concurrency for correctness
  and simplicity — accepted at chat-gateway scale; Option A is the recorded
  upgrade if that ever changes.

### Trade-offs accepted

- **Opaque key, not `(channel, conversation)` columns.** Retention-by-channel
  becomes a `LIKE 'prefix::%'` query rather than an indexed equality, accepted
  to keep the ADR-0018 seam intact and avoid fragile `"::"` re-parsing.
- **Reads serialize behind writes** (`MaxOpenConns(1)`). Accepted; irrelevant at
  this scale and the price of zero-`SQLITE_BUSY` simplicity.
- **Best-effort cross-message ordering persists** (inherited from ADR-0018): two
  concurrent messages of one conversation may each load history without the
  other's just-written turn, and their group order is not fixed. Unchanged by
  durability; revisit only if strict per-conversation ordering is required.

## Alternatives Considered

### A1 — cgo SQLite (`mattn/go-sqlite3`)
**Rejected.** It would break or heavily complicate the `CGO_ENABLED=0` ×6
cross-compile to ARM/RPi — the exact thing the pure-Go driver makes trivial.
The cross-compile is a hard project constraint; this is the deciding axis.

### A2 — Decompose the key into `(channel, conversation)` columns
**Rejected.** The seam hands an already-joined opaque `Key`. Splitting requires
re-parsing on `"::"` (fragile when a conversation id contains `"::"`) or widening
the `conversation.Store` interface (breaking ADR-0018). The opaque column plus a
future `LIKE 'prefix::%'` retention query costs nothing now and keeps the seam
intact.

### A3 — Option A concurrency (WAL + `MaxOpenConns > 1` + `BEGIN IMMEDIATE`)
**Rejected for now, recorded as the future variant (§3).** It buys concurrent
reads during writes at the cost of `SQLITE_BUSY` handling and mandatory
`IMMEDIATE` transactions. Korvun's load (human chat) does not justify the extra
moving parts; the single writer is simpler and 3am-safe.

### A4 — Put `SqliteStore` inside `internal/conversation`
**Rejected.** It would make the leaf package import `database/sql` and the SQLite
driver, destroying the leaf invariant ADR-0018 §3 established. A subpackage
(`internal/conversation/sqlite`) confines the dependency, mirroring
`internal/model/{ollama,groq}`.

### A5 — Per-brain stores
**Rejected.** The `Key` already namespaces by `channel::conversation`; brains
sharing one store cannot collide. One store opened once is simpler, uses one file
handle, and matches the key design.

### A6 — TEXT (RFC3339) timestamps
**Rejected** in favour of INTEGER unix-nano (§2): more compact, unambiguously
ordered, and round-tripped explicitly in Go without relying on the driver's
time-format conversion. Readability under the raw `sqlite3` CLI did not outweigh
those.

## Out of scope (recorded, not silently dropped)

- **Stateful budgets / accounting persistence** — a later stage; the policy
  layer's `Accounting` is computed per-dispatch today.
- **Long / analytical / queryable history** — beyond "last N turns for reply
  context"; additive later over the same table.
- **Postgres / multi-node / shared DB** — a future `conversation.Store` impl
  behind the same seam; additive, no contract change.
- **Compaction / retention / eviction** — deferred; `ts` + `seq` on every row
  make it an additive query, not a migration.
- **Per-sender-within-a-group keying** — inherited deferral from ADR-0018 §3;
  the canonical key stays the chat.
- **Option A concurrency** — recorded (§3, A3) as the upgrade path, not built.
```
