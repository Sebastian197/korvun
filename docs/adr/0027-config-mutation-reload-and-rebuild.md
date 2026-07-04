# ADR-0027: Stage 14 Phase 2a — config mutation via reload-and-rebuild

> **Status:** proposed
> **Date:** 2026-07-04
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Stage 14 Phase 2 is the no-code builder. Its v0.2 cut (framed via `/office-hours`
+ `/plan-eng-review`, copilot-approved) is **form-based editing of the existing
config over a mutation API**; the visual canvas is deferred (zero new capability
over forms for the current flat config — the same honest lens that deferred the
bus in Stage 10). Phase 2 is split by blast radius: **2a = the backend mutation
(this ADR) + auth ([ADR-0028](0028-admin-auth-bearer-token.md), separate — surface
risk vs blast-radius risk, different lenses)**; **2b = the React edit UI (its own
ADR)**.

Until now the control API is **read-only** (ADR-0022): `GET /api/brains`,
`GET /api/channels`. Making it mutate state is the highest-risk change in the
project, because of a hard constraint discovered and recorded in the HANDOFF and
re-confirmed against the code:

- `internal/router` (`router.go:33-103`) holds a **single global**
  `cancel context.CancelFunc`. `RegisterChannel` / `RegisterBrain` / `Route` run
  under one `sync.RWMutex`; there is **no `Unregister`, no per-brain cancel**.
  `Shutdown` calls that one `r.cancel()` for the whole router (`router.go:282`).
- So **adding** a brain/channel/route live is a thin extension of code that
  already exists and is `-race`-tested; **removing or editing** a running brain
  needs a **per-brain lifecycle** (its own derived context + drain + join,
  reconciled with the single-cancel invariant the `-race` tests rely on) — a
  rewrite of the most delicate code in the project.

### The decision that dissolves the risk

Instead of granular live mutation (the router rewrite), Phase 2a uses
**reload-and-rebuild**: a config change tears the whole system down with the
**existing** graceful shutdown and rebuilds it with the **existing** boot. The
router and its lifecycle are **frozen, untouched** — the "no per-brain cancel"
problem is not solved, it is **dissolved** (you never cancel one brain; you
`Shutdown` everything and `Build` again). This is lower blast radius (the
`-race` router is not modified) AND higher capability (full edit, not add-only):
a clean rebuild applies any config change, not just additions.

### What the code already gives (verified by reading, not memory)

The two paths reload-and-rebuild reuses already exist and are tested:

- **Boot:** `config.Load(path)` (parse + `Validate`, `internal/config`) →
  `app.Build(cfg, opts...)` (wires channels/router/brains/store/admin server,
  `internal/app/app.go:151`) → `app.Run(ctx)` (`app.go:663`).
- **Graceful shutdown:** `app.Shutdown(ctx)` (`app.go:696`) stops channels first
  (ADR-0008 order → the router pump drains and exits), then `router.Shutdown`
  (drains brain + outbound workers), then closes the store LAST (only after a
  clean router drain — `app.go:713`), then the live-view, admin server, and bus.
- **Store handoff:** `openStore` (`app.go:395`) opens the single-writer SQLite
  store at Build; `Shutdown` closes it after the drain. Reopen-after-close is the
  same path a process restart already takes.

### The bootstrapping subtlety (load-bearing)

The mutation endpoint lives on the **admin HTTP server**, which is **part of the
`App` that reload-and-rebuild tears down** (`controlapi.Register(adminServer,
app)`, `app.go:268`). A handler cannot synchronously shut down the app that hosts
it — it would cancel its own request goroutine mid-reload. Therefore the reload is
driven by a **supervisor that owns the lifecycle ABOVE `App`** (in `cmd/korvun`);
the handler validates, persists, and **signals** the supervisor, then returns. The
supervisor performs the swap from a goroutine that outlives any single `App`.

## Decision

Add a **write endpoint** to the control API and a thin **supervisor** in
`cmd/korvun` that owns the app lifecycle and performs reload-and-rebuild. The
router is not touched.

### The mutation flow

1. **Accept** a new config on the write endpoint (full config document;
   patch-over-current is a UI convenience deferred to 2b — the wire contract in
   2a is a full validated config, so the server has one authority to validate and
   persist). Auth per ADR-0028 gates this endpoint.
2. **Validate** with the *existing* `config.Validate` (in memory, no side effect).
   Invalid → `400`, old app **untouched, still serving**.
3. **Build** a new `*App` from the new config in memory (`app.Build`). Build does
   the heavy validation (secrets resolve, Telegram token `getMe`, privacy selector
   per brain). Build failure → report it (`409`/`422`), old app **untouched, still
   serving** — Build never touches the running app, so a failed build is free.
4. **Signal the supervisor** and return `202 Accepted` with a status handle. The
   supervisor (outliving the request) performs the **cutover**: `Shutdown` the old
   app, `Run` the new app, swap the reference.
5. **Persist** the new config to the `-config` file **only after a successful
   cutover** (see §Persistence), so the on-disk config is always a known-good one.

### (a) In-flight messages during the cutover — required section

Honest accounting of what happens to messages around the blip:

- **Already accepted** (in the router's queues / mid-`Handle`): the graceful
  shutdown drains them. `app.Shutdown` stops channels first, so the pump drains,
  then `router.Shutdown` waits for every brain worker to return; brain workers
  persist their final turn on a **cancellation-detached context**
  (`brain.persistTurns`, ADR-0019 §6), so an in-flight reply still commits to the
  store before the old app is gone. **Not lost.**
- **Arriving DURING the blip** (after channel `Stop`, before the new channel
  `Start`): the only channel today is **Telegram polling**. Telegram **holds
  updates server-side (~24h) and re-delivers from the last acknowledged offset**;
  the new app's channel resumes polling and **picks them up on resume**. **Not
  lost** — Telegram is the buffer. (A future webhook channel would have a genuine
  loss window during the blip; recorded as a per-channel caveat for whoever adds
  webhook mutation, not a 2a concern since polling is the only mode.)
- **The blip itself:** the admin surface (`/api`, `/ui`, SSE) closes and re-binds
  the same loopback port (sub-second). The 2b UI reconnects; SSE clients
  reconnect. Acceptable for a single-operator self-hosted gateway (premise 3).

### (b) SQLite store handoff — required section

The store is single-writer (`MaxOpenConns(1)`, ADR-0019). Cutover sequence keeps
it clean: old `app.Shutdown` closes the store **only after the router has fully
drained** (`app.go:713` — no `AppendTurns` can race a closing DB); the new
`app.Build` reopens it via `openStore`. Between close and reopen **no writer
exists** (old router drained, new one not started), so there is no concurrent
access and no corruption. This is the exact close/reopen a process restart already
performs; the path is tested. If the old router does **not** drain within the
cutover deadline, `Shutdown` leaves the store open (SQLite WAL is
crash-consistent) rather than racing `Close` — the reopen tolerates it.

### (c) Drop-free cutover + mandatory rollback — required section

**A builder that can leave the system down is not acceptable.** The ordering
guarantees the process is always either running the old config or the new one,
never neither:

- **Bad config (validate/Build fails):** old app keeps running, untouched (steps
  2-3 never touch it). This is the common failure and it is fully safe.
- **Cutover fails (new app Built but `Run` fails — e.g. admin re-bind race, a
  channel `Start` error the token check didn't catch):** the supervisor attempts a
  **rollback**: re-`Build` + `Run` from the **old** config (still in memory / still
  on disk, since persistence happens only on success). If rollback also fails, the
  process exits non-zero; systemd (`ADR-0026` hardened unit, `Restart=on-failure`)
  restarts, and because the on-disk `-config` was **never overwritten**, it boots
  the last known-good config. There is no crash-loop into a bad config.

### Config persistence — named decision

The edited config is written to **the `-config` JSON file** (atomic: write a
temp file in the same dir, then `rename`), **not** the SQLite store. Rationale:
the config file is already the single source of truth at boot (`config.Load(path)`);
keeping it authoritative means a plain restart reloads exactly what the builder
produced, and avoids a second config authority. The SQLite store is conversation
memory — a different lifecycle; mixing config into it would couple the two. The
write happens **only after a successful cutover**, so the file is always a config
that is known to boot (the backstop in §c).

### The one new production seam

`App` (or the control API) gains a **reload-request seam** the supervisor injects:
the handler calls it to hand the validated new config to the supervisor and get a
status handle. This is the only new coupling; the router, brains, channels, store,
and the read-only control API are unchanged.

## Consequences

### What this enables

- The operator edits Korvun's config through an API (and, in 2b, a UI) and the
  change takes effect via a clean rebuild — full edit of any field, not add-only.
- The `-race`-tested router and its lifecycle are **frozen**: the project's most
  dangerous code is not modified.

### Reversibility (explicit)

Additive and reversible: the mutation is **one write endpoint + a supervisor loop**
that wraps the existing Build/Run/Shutdown. Removing the endpoint and collapsing
the supervisor back to "Build once, Run, Shutdown on signal" reverts to today's
behavior exactly. No schema, no router change, no data migration.

### Trade-offs accepted

- **A reload blips the pipeline** (sub-second admin re-bind + a brief pause while
  the domain rebuilds). Accepted for a single-operator gateway (premise 3); it is
  the price of freezing the router.
- **The admin surface is rebuilt each reload** (simplest — reuses Build/Shutdown
  wholesale). Splitting a stable control plane from the rebuilt data plane (so the
  admin server survives reloads) is a cleaner but larger refactor, deferred unless
  the blip proves unacceptable.

## Alternatives Considered

- **Reload-and-rebuild (CHOSEN)** — freeze the router, reuse Build + graceful
  Shutdown, full edit at the cost of a blip.
- **Add-only live registry** — extend the existing `Register*` path to run
  post-boot. **Rejected:** a capability trap (can only add, so editing a policy —
  the most common builder action — is impossible) that still mutates the live
  router. Lower capability AND still touches the delicate code.
- **Per-brain live lifecycle (granular hot editing)** — give each brain worker its
  own cancel, drain/join one worker. **Rejected:** rewrites the most delicate,
  `-race`-tested code in the project for a "no blip" benefit a single operator does
  not need. Explicitly out of scope.

## Out of scope (recorded)

Granular live editing, per-brain cancel, an add-only fast path parallel to reload,
a stable-control-plane refactor, config versioning/history/undo, patch-over-current
on the wire (a 2b UI convenience), webhook-channel mutation loss handling, and the
visual canvas.

## Delivery

Phase 2a ships on a **branch** with a full **`/review`** and `-race` on the
**quiesce → rebuild → swap** path: the mutation touches the cutover of the whole
system, so it earns the structural bar even though the router stays frozen. Auth
lands alongside it per [ADR-0028](0028-admin-auth-bearer-token.md). `make quality`
green; `go.mod` stays at 3 deps (the supervisor and the endpoint are stdlib).