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
the handler validates and **signals** the supervisor, then returns `202`. The
handler does **not** persist and does **not** open the store; the supervisor — a
goroutine that outlives any single `App` — performs the heavy checks, the swap, and
(only on success) the persist.

## Decision

Add a **write endpoint** to the control API and a thin **supervisor** in
`cmd/korvun` that owns the app lifecycle and performs reload-and-rebuild. The
router is not touched.

### The mutation flow

The flow splits deliberately between a **fast handler** (returns `202` after only
cheap, effect-free work) and the **supervisor** (does every heavy or stateful step,
asynchronously, reporting through the status handle). This split is what makes F1
(no double-open store), F5 (persist only after cutover), F7 (a slow Telegram never
hangs the request) and F11 (a self-locking config is refused) all hold at once.

**Handler (synchronous — returns `202` or a `4xx`):**

1. **Auth** per ADR-0028 gates the endpoint. **Accept** a full config document
   (patch-over-current is a 2b UI convenience; the 2a wire contract is a full
   config, so the server has one authority to validate and persist).
2. **Validate** with the *existing* `config.Validate` (pure, in memory, no side
   effect — no store, no network, no worker). Invalid → `400`, old app **untouched**.
3. **Refuse a self-locking config (F11):** if the mutation surface is currently
   mounted (a token is configured, ADR-0028) and the new config would leave it
   **unmounted** (no admin token, or a token env-var that resolves empty), reject
   with `409` naming the manual recovery (edit `-config` + restart). A builder must
   not permit the one action that irreversibly disconnects it from itself: a config
   that both drops the token and persists leaves no API path back, and — because the
   persisted file survives restart — **not even a restart recovers the control
   plane**. This check is cross-referenced by ADR-0028 §1.
4. **Single-flight (F3):** acquire the supervisor's "reload in progress" lock. If a
   reload is already running, reject the second request with `409` (never a second
   concurrent build/`openStore`). On acquire, hand the validated config to the
   supervisor and return **`202 Accepted` with an opaque status-handle ID** (§seam,
   F4). The handler does **no** network I/O and **no** store open, so a
   slow/unreachable Telegram cannot hang it (F7).

**Supervisor (asynchronous — updates the status handle, holds the single-flight
lock until done):**

5. **Pre-cutover checks (effect-free; old app still serving):** resolve secrets from
   the environment, run the Telegram `getMe` health-check, run the privacy selector
   per brain — all **without opening the store and without touching the running
   app**. Any failure → status `failed`, old app **untouched, still serving**, lock
   released. The common "bad secret / bad token / no eligible model" failures are
   caught here, cheaply and safely — the heavy network work lives behind the `202`,
   not in the request (F7).
6. **Cutover (F1 — the only window the store or the port move):** `Shutdown` the old
   app (channels → router drain → **old store closed**), **then** build the new app —
   which is when `openStore` runs and the new store is opened. Because the open
   happens strictly **after** the old `Shutdown` closed the old store, **the store is
   never open twice; there is never a second writer.** Then `Run` the new app and
   swap the supervisor's reference.
7. **On cutover failure** (build / `openStore` / `Run` fails — e.g. admin re-bind, a
   channel `Start` error): `Shutdown`/`Close` the partially-built new app so it leaks
   no worker goroutine or store handle (F2), then roll back per §(c). Status →
   `rolled-back`, or (if rollback also fails) the process exits — still safe, §(c).
8. **On success:** the **supervisor** (not the handler) **persists** the new config
   to the `-config` file (F5 — only now, after a clean cutover), status →
   `succeeded`, lock released.

**Signal-coordination invariant (F6):** the supervisor is the single owner of the
app lifecycle, so `SIGINT`/`SIGTERM` and a reload travel the **same** control path.
A signal arriving mid-cutover is **ordered** by the supervisor (finish or abort the
in-flight cutover, then shut down), never a second concurrent lifecycle driver
racing the reload.

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
- **Telegram single-consumer (F14 caveat):** Telegram allows only **one
  `getUpdates` long-poll consumer per bot token**, so the cutover is **strictly
  sequential** — `app.Shutdown` fully stops the old channel's polling **before** the
  new channel `Start`s — precisely to avoid a `409 Conflict` from an overlapping
  poll. A botched cutover that transiently overlapped old-polling with new-`Start`
  would surface that `409`; strict sequencing is what prevents it. Defending the
  partial-overlap edge is out of scope (§Out of scope).

### (b) SQLite store handoff — required section

The store is single-writer (`MaxOpenConns(1)`, ADR-0019), and the cutover is ordered
so it is **never open twice** (F1). The key correction over a naive "build the new
app first" flow: `openStore` does **not** run in advance — it runs **inside** the
cutover (§ step 6), **after** `app.Shutdown` on the old app has closed the old store
(`app.go:713` — Close only after the router fully drained, so no `AppendTurns` can
race a closing DB). The real order is therefore:

```
old router drains → OLD store Close → NEW openStore → new writer
```

Between the close and the open **no writer exists** (old drained, new not yet
opened), so there is no double-writer window and no WAL-lock contention — the exact
close/reopen a process restart already performs, and the path is tested. If the old
router does **not** drain within the cutover deadline, `Shutdown` leaves the old
store open (SQLite WAL is crash-consistent) and the supervisor **rolls back** rather
than opening a second handle onto the same file.

### (c) Drop-free cutover + mandatory rollback — required section

**A builder that can leave the system down is not acceptable.** The ordering
guarantees the process is always either running the old config or the new one,
never neither:

- **Bad config (validate / self-lock / pre-cutover check fails):** old app keeps
  running, untouched — the handler's checks (steps 2-3) and the supervisor's
  pre-cutover checks (step 5) never touch it. This is the common failure and it is
  fully safe. A config that would self-lock the control plane is refused here (F11).
- **Cutover fails (old already `Shutdown`; new build / `openStore` / `Run` fails —
  e.g. admin re-bind, a channel `Start` error the `getMe` check didn't catch):**
  first `Shutdown`/`Close` the partially-built new app so it leaks no worker
  goroutine or store handle (F2). Then attempt a **rollback**: rebuild + `Run` the
  **old** config (still in memory; still on disk, since persistence happens only on
  success). **Do not oversell the rollback (F8):** for a *bind* failure the rollback
  re-binds the **same** port that just failed, so it may fail identically. The real
  backstop is **not** the rollback — it is that the process then exits non-zero,
  systemd (ADR-0026 hardened unit, `Restart=on-failure`) restarts it, and because
  the on-disk `-config` was **never overwritten** it boots the last known-good
  config. There is no crash-loop into a bad config.
- **Windows re-bind caveat (F8):** the "sub-second re-bind" assumes unix
  `SO_REUSEADDR` semantics on a just-closed listener. Windows (a supported target)
  does not grant the same immediate rebind, so on Windows a cutover-time rebind is
  likelier to fall through to the systemd-style restart backstop than to an
  in-process rollback. Named, not solved — a single-operator gateway tolerates the
  restart.

### Config persistence — named decision

The edited config is written to **the `-config` JSON file** (atomic: write a
temp file in the same dir, then `rename`), **not** the SQLite store. Rationale:
the config file is already the single source of truth at boot (`config.Load(path)`);
keeping it authoritative means a plain restart reloads exactly what the builder
produced, and avoids a second config authority. The SQLite store is conversation
memory — a different lifecycle; mixing config into it would couple the two. The
write happens **only after a successful cutover, and it is the supervisor — not the
request handler — that performs it** (F5): the handler that returned `202` never
persists, so a failed or rolled-back cutover can never leave an edited config on
disk. The file is therefore always a config known to boot (the backstop in §c).

### The one new production seam + the status handle (F4)

`cmd/korvun`'s **supervisor** is the one new production element: it owns the app
lifecycle above `App`, holds the single-flight reload lock, and — crucially — owns
the **reload status**, which lives **in the supervisor, NOT on the admin server**.
This is the load-bearing fix for F4: the admin server is part of the `App` the
cutover tears down and rebuilds, so status kept there would be lost across the very
blip it reports on. Instead:

- The handler calls a **reload-request seam** to hand the validated config to the
  supervisor and receives an **opaque status-handle ID**.
- The supervisor exposes a read-only `GET` status route (re-mounted on each rebuilt
  admin server, but reading **supervisor-owned** state that survives the cutover)
  reporting one of: `pending` → `cutover-in-progress` → `succeeded` | `rolled-back`
  | `failed`.
- A 2b UI polls this handle to learn the outcome across the reconnect. The heavy
  work is observable here, not blocking the original request (F7).

This is the only new coupling; the router, brains, channels, store, and the
read-only control API are unchanged.

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
behavior exactly at the **code** level. No schema, no router change, no data
migration. **Precision (F13): reversibility is code-only** — once a mutation has
rewritten the on-disk `-config`, reverting the *feature* does not revert the
operator's persisted edits; the file stays as last written.

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
on the wire (a 2b UI convenience), webhook-channel mutation loss handling, the
visual canvas, and hardening against a Telegram `getUpdates` `409` from an
overlapping poll during a botched cutover (strict-sequential cutover avoids it,
§(a); defending the partial-overlap edge is out of scope — F14).

## Delivery

Phase 2a ships on a **branch** with a full **`/review`** and `-race` on the
**quiesce → rebuild → swap** path: the mutation touches the cutover of the whole
system, so it earns the structural bar even though the router stays frozen. Auth
lands alongside it per [ADR-0028](0028-admin-auth-bearer-token.md). `make quality`
green; `go.mod` stays at 3 deps (the supervisor and the endpoint are stdlib).