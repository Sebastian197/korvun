# ADR-0022: Control API — read-only operator introspection on the existing admin server (Stage 13)

> **Status:** accepted
> **Date:** 2026-06-28
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Korvun boots, serves Telegram live, remembers across restarts, and is observable
through `/metrics` + `/healthz` (Stages 0–9, 11, 12). What an operator still
cannot do is **ask the running binary what it is actually wired to** in a form
that is neither the raw config file nor a Prometheus scrape: which brains exist,
what privacy sensitivity each declares, which models *survived the privacy
selector*, which channels are connected and how many messages each has dropped.
A control API is the surface that answers this — and, eventually, the backend the
Stage 14 no-code builder drives. This ADR opens that surface with the **minimum
cut that has a real consumer today**.

### The framing line — read-only first; deferring mutation IS the security decision

The Stage 13 framing (`/office-hours` alternatives + `/plan-eng-review` lens,
reviewed by copilot) reached the same kind of honest verdict the bus framing did
(Stage 10, deferred), but with the **opposite outcome because the situation
differs**:

- **A control API can grow without bound** (manage everything). The disciplined
  question is the minimum subset with a *real consumer*.
- **Read has a real consumer NOW:** an operator with `curl` who wants to see what
  is wired and live without grepping logs or re-reading JSON. The single most
  valuable datum — *which models survived the privacy selector for a Private
  brain* — exists **only in the running binary** (`policy.SelectModels` runs at
  boot, `app.go:316`); it is not in the config file, not in `/metrics`, not
  derivable statically.
- **Mutation has no real consumer until the builder** (Stage 14). Live
  create/update/delete of brains/routes/channels is a builder-backend feature,
  and the router's registry is **boot-time** (`RegisterBrain`/`Route` run once in
  `wire()`); mutating it live under load is a concurrency/lifecycle change, not a
  handler. Same lens as the bus: do not build a seam ahead of its consumer.
- **The load-bearing insight is about security.** The admin server is loopback,
  no auth (ADR-0020 §4) — *justified because it only exposes, never mutates*. A
  **read-only** control API keeps that calculus **exactly intact**: zero new
  attack surface beyond what `/metrics` already accepted. **Mutation** is what
  would break it (a local process, an SSRF, or a future `0.0.0.0` bind could
  change state). Therefore **deferring mutation is not a scope cut, it is the
  security decision** — it is what keeps "loopback, no auth" correct.

### What the repo already gives (verified by reading, not memory)

- **The seam is built and exact:** `httpserver.Handle(pattern string, h
  http.Handler)` (`httpserver.go:70`) mounts routes on the shared
  `http.ServeMux`. ADR-0020 §4 built it explicitly "so the Stage 13 control API
  mounts on the same server." **`Handle` must be called before `Start`** (the mux
  is not safe to mutate once serving, `httpserver.go:68`), so routes are
  registered in `app.Build()`, not at runtime.
- **The admin server exists only when observability is enabled** (default-on,
  loopback `127.0.0.1:2112`; `app.go:135-143`). The control API rides this server,
  so its availability is coupled to observability (§5, an open question).
- **Config carries no secrets** — only env-var NAMES (`token_env`, `api_key_env`;
  `config.go:14-17`). Exposing resolved wiring is secret-free by construction.
  But `App` currently **discards** `*config.Config` after `Build` (it keeps
  router, channels, store, metrics; `app.go:62-78`) — summaries must be assembled
  while the config is in hand, i.e. in `wire()`.
- **Brains** live in the private `router.brains` map with **no public accessor**;
  they are immutable for the process lifetime in this cut (no mutation). **Channels**
  are already held by `App` (`[]Channel`, with `Name()` and the
  `DroppedCount()` accessor on telegram, `app.go:199-201`) and their drop count
  is **live**.
- **`/healthz` is liveness-only by decision** (ADR-0014 §3 / ADR-0020 §4):
  per-provider health is deliberately not tracked, so a "detailed health"
  endpoint is out of this cut.
- **Security precedents:** loopback-no-auth accepted on `/metrics`;
  `readHeaderTimeout` already closes Slowloris (gosec G112, `httpserver.go:32`);
  secrets env-only, never logged or echoed (ADR-0010).

### External-docs verification (per CLAUDE.md non-negotiable)

**No new dependency, and none to verify.** The control API is stdlib
`net/http` over the `http.ServeMux` the binary already serves — the same library
ADR-0020 introduced for `/metrics`+`/healthz`. `go.mod` is at `go 1.26`, so
**method-pattern routing** (`mux.Handle("GET /api/brains", ...)`) is a stdlib
feature available by construction (Go 1.22+); no third-party router, no Context7
lookup needed. `go.mod` stays at **3 direct dependencies** — Stage 13 adds none.

## Decision

Ship a **read-only control API** as a new leaf package `internal/controlapi`,
mounted on the existing `httpserver` mux under the `/api` prefix, registered in
`app.Build()` before `Start`. **Additive: the domain and the router are
untouched.** All mutation is deferred to Stage 14.

### 1. Package and mount — additive, on the server that already exists

```
   internal/controlapi          ◄── new leaf: HTTP handlers + small reader interfaces
        │ depends on the READER interfaces, never on router/brain concretes
        ▼
   net/http handlers  ── mounted via httpserver.Handle("GET /api/...", h) in Build()
                          on the SAME ServeMux as /metrics + /healthz
```

`controlapi` builds an `http.Handler` (its routes) from a small set of reader
interfaces (§3). `app.Build()` constructs it after the admin server and calls
`adminServer.Handle("GET /api/brains", ...)` etc. — **before** `Run` starts the
server. `/healthz` and `/metrics` are unchanged. No second server, no new
listener, no new port.

### 2. Endpoints — the minimum cut

**Exactly two `GET` endpoints**, all read-only, all responding
`application/json`. Two endpoints cover the real consumer (an operator seeing
what is live); a third "deployment summary" endpoint without a concrete consumer
is the pattern deferred for the bus, so it is **not** in this cut (Resolved §1.2).

| Endpoint | Returns |
|----------|---------|
| `GET /api/brains` | one entry per registered brain: `name`, `sensitivity` (public/private), `policy` (priority/consensus), `dispatch` (fanout/sequential), and the **resolved models that survived the privacy selector** — each `{provider, model_id}` only (name/id; nothing near credentials). This last field is the headline: post-`SelectModels` state that exists only in the running binary. |
| `GET /api/channels` | one entry per channel: `type`, `mode`, `name`, and live `dropped` count (from `DroppedCount()` where the channel exposes it; omitted otherwise). |

**Resolved, not raw.** `GET /api/brains` reports the *effect of the wiring*, not a
config echo: if a Private brain was configured with a cloud model, the selector
dropped it at boot (`ErrNoEligibleModels` would have failed the boot otherwise),
so the response shows only the models that actually remain dispatchable. That is
the operational truth `/metrics` and the file cannot give.

`GET /api/info` (a resolved deployment view: counts + the channel→brain route
table) is **deferred, additive-later**: it has no concrete consumer today, and
its content folds into the two endpoints above if ever needed. Adding it later is
a new route on the same surface, not a schema change.

### 3. The reader seam — interfaces implemented by App, never domain concretes

The handlers depend on **small reader interfaces**, the established Korvun pattern
(`conversation.Store`, `metrics.Metrics`): `controlapi` imports neither `router`
nor `brain` concrete types.

```go
// Package controlapi serves read-only operator introspection. It depends only
// on these reader seams; App implements them. No mutation, no domain imports.
package controlapi

type BrainSummary struct {
    Name        string         `json:"name"`
    Sensitivity string         `json:"sensitivity"`
    Policy      string         `json:"policy"`
    Dispatch    string         `json:"dispatch"`
    Models      []ModelSummary `json:"models"` // post-selector survivors
}

// ModelSummary is the minimal secret-free identity of a surviving model:
// provider name + model id, nothing that grazes credentials. Locality is NOT
// exposed (the brain's sensitivity already conveys the privacy posture, and a
// Private brain's survivors are local by selector guarantee); it is additive
// later if a real need appears.
type ModelSummary struct {
    Provider string `json:"provider"`
    ModelID  string `json:"model_id"`
}

type ChannelSummary struct {
    Type    string  `json:"type"`
    Mode    string  `json:"mode"`
    Name    string  `json:"name"`
    Dropped *uint64 `json:"dropped,omitempty"` // nil when the channel has no counter
}

// Reader is the read-only seam the API depends on. App implements it.
type Reader interface {
    BrainSummaries() []BrainSummary
    ChannelSummaries() []ChannelSummary
}
```

**Where the data comes from (confirmed in review, Resolved §1.1):**

- **Brain summaries are assembled in `App.wire()`**, where the config is in hand
  and `SelectModels` already runs per brain (`app.go:307-334`). `App` records the
  resolved `BrainSummary` for each brain into a slice it owns, and `BrainSummaries()`
  returns a copy. **This keeps the router 100% untouched** — strictly more
  additive than adding an accessor to `router.brains`. It is correct here because
  brains are immutable at runtime in the read-only cut (a boot-time snapshot is
  the live truth for the process lifetime).
- **Channel summaries are live:** `App` already holds `[]Channel`; `DroppedCount()`
  is read at request time so the `dropped` count is current.

`App` satisfies `controlapi.Reader`. The handlers marshal the summaries to JSON
and never touch a concrete router/brain type.

**The boot snapshot is NOT debt — it is the correct impl of an interface that
survives Stage 14.** When Stage 14 introduces mutation, brains become mutable at
runtime, so `BrainSummaries()` must reflect a **live view** of the registry
rather than a boot snapshot. The `Reader` interface is unchanged across that
transition: only its *implementation* moves (from an `App`-held snapshot to a
live read of the then-mutable registry). Today's snapshot is the right
implementation for a cut where brains are immutable; tomorrow's live view is the
right implementation for a cut where they are not — same seam, different source.

### 4. Security — read-only justifies loopback-no-auth; auth is the mutation trigger

- **Same profile as `/metrics`.** Read-only on loopback with no auth is the
  posture ADR-0020 §4 already accepted, and it stays valid **because nothing
  mutates**. No new attack surface beyond the accepted one.
- **Responses are secret-free — a fixed INVARIANT asserted in the handlers
  (defense in depth).** Config is already secret-free by construction (env-var
  names only), but the summaries deliberately expose **neither secrets nor
  secret-references**: even the env-var *names* (`token_env`, `api_key_env`) are
  omitted — the operator does not need to know *which* env var holds a secret,
  only that the binary is wired. The only fields ever served are non-secret wiring
  facts: brain `name`/`sensitivity`/`policy`/`dispatch`, surviving-model
  `provider`/`model_id`, and channel `type`/`mode`/`name`/`dropped`. This is a
  binding invariant, not a one-off: a test asserts no response contains a secret
  value or an env-var name (§Implementation), so a future field cannot silently
  leak.
- **Slowloris already covered** by `httpserver.readHeaderTimeout` (10s); the
  control API inherits it (same server).
- **AUTH is explicitly the trigger of MUTATION, not of this cut.** When mutation
  arrives (Stage 14), auth becomes essential — a token — because the builder may
  run as a separate process and/or the operator may bind beyond loopback to reach
  it. **Starting read-only is precisely what keeps the auth requirement low**:
  there is no state to protect, so loopback is sufficient. This ADR records the
  trigger so the minimal cut stays minimal and the next stage inherits a clear
  obligation.

### 5. Coexistence and config

- **Same mux, same server, `/api` prefix.** `/healthz` + `/metrics` are
  untouched. The control API lives entirely under `/api/...`.
- **Default-on, loopback — derived from read-only.** It rides the admin server's
  default-on loopback posture (ADR-0020 §4) **because it is read-only**: not
  mutating means default-on does not change the security calculus. (Were it
  mutation, the posture would be opt-in.) No new config block in the minimal cut.
- **Coupling to observability — a conscious, documented consequence (Resolved
  §1.3).** The admin server exists only when `observability.enabled`. The minimal
  cut **rides that server**, the boring choice. The consequence, accepted with
  eyes open: **turning off `observability.enabled` also turns off `/api`
  introspection**, even though the two are conceptually distinct (metrics for
  Prometheus vs introspection for an operator). This is acceptable for a
  read-only cut. If a future need ever wants one without the other (introspection
  without a metrics scrape, or vice versa), that forces **separate enablement** —
  an additive config split, named here and deferred, not built now.

## Consequences

### What this enables

- **An operator can introspect the live wiring.** `GET /api/brains` shows the
  post-selector model set per brain (the privacy decision made visible),
  `GET /api/channels` shows connectivity and drops — neither available from the
  file or `/metrics`.
- **Stage 14 inherits a backend shape.** The builder mounts its read views on the
  same `/api` surface and the same reader seam; this ADR builds the read half,
  Stage 14 adds the mutation half (with auth).
- **Zero new dependency, zero domain/router change.** The cut is handlers plus a
  reader interface `App` already has the data for.

### What this asks / costs

- **`App` retains resolved summaries.** A new slice populated in `wire()` and a
  `controlapi.Reader` implementation — small, boot-time, additive.
- **The first *introspective* network surface.** `/metrics` already opened a port;
  `/api/*` adds read-only operational detail on the same loopback bind. Still
  nothing externally exposed by default, still secret-free.
- **A boot-time brain snapshot.** Because brains are immutable in this cut, the
  snapshot is accurate; it would need to become live-backed only when mutation
  lands (Stage 14) — recorded so the snapshot is not mistaken for a permanent
  shortcut.

### Trade-offs accepted

- **No mutation.** The operator cannot change anything via the API; that is the
  point (it is the security decision, §4). Mutation is Stage 14.
- **No conversation browsing / no per-provider health.** Both need new
  capability (`Store` query; health probing) and have their real consumer in the
  builder — deferred (Out of scope).
- **Availability coupled to observability** in the minimal cut (§5) — accepted as
  the boring default; revisited if an operator wants `/api` without `/metrics`.

## Alternatives Considered

### A — Defer the whole control API to Stage 14 (the bus treatment)
Build nothing now; let the builder define every endpoint. **Rejected:** unlike
the bus, the read half has a **real consumer today** (the operator) and reuses a
seam Stage 12 built *explicitly* for it. Deferring everything wastes cheap,
already-enabled value. The honest lens gives a different answer because the
situation is different (a real consumer exists).

### B — Read-only PLUS one mutation operation now
Add a single concrete mutation (e.g. reload config). **Rejected:** no mutation has
a real consumer today, and **any** mutation breaks the loopback-no-auth calculus
(would force auth up front) and touches the router's boot-time registry lifecycle.
The value does not justify changing the security posture before the builder needs
it.

### C — A web framework (chi / gin / echo) instead of stdlib
Nicer routing/middleware. **Rejected:** boring-by-default — Korvun has added no
web framework, and a handful of read-only GETs do not justify spending an
innovation token or a dependency. Go 1.22+ `ServeMux` method-pattern routing
(stdlib, already in `go.mod`'s `go 1.26`) covers the need.

### D — A second HTTP server for the control API
Isolate `/api` from `/metrics`. **Rejected:** a second listener/port/lifecycle for
no benefit; ADR-0020 §4 designed one mux precisely so Stage 13 reuses it. The
`/api` prefix gives clean separation on one server.

### E — A new `router.brains` accessor feeding live reads
Expose the registry and read it per request. **Considered, not chosen for the
minimal cut:** brains are immutable at runtime here, so an `App`-assembled
boot-time snapshot is equally correct and leaves the router **completely
untouched** (more additive). The accessor becomes the right design only when
mutation makes the registry live (Stage 14) — the `Reader` interface stays
stable across that transition, only its implementation moves (§3). Confirmed in
review (Resolved §1.1).

## Out of scope (recorded, not silently dropped)

- **All mutation** — create/update/delete brains, routes, channels; hot config
  reload; start/stop a channel. The router registry is boot-time; mutation is
  Stage 14 (and brings auth with it, §4).
- **Conversation browsing / history endpoints** — needs a new `conversation.Store`
  query capability (it is append-only + `LoadRecent` today); a builder feature.
- **Detailed / per-provider health** — provider health is not tracked
  (ADR-0014 §3 keeps `/healthz` liveness-only); deferred until there is a probe.
- **Auth / token / RBAC / TLS / network exposure (`0.0.0.0`)** — the mutation
  trigger (§4); lands with Stage 14's externally-reachable, state-changing surface.
- **The no-code builder UI** — Stage 14.
- **Any new dependency** — stdlib `net/http` only.

## Delivery — direct to master with TDD, light `/review` (decided here)

The ADR was asked to decide branch-vs-master. **Direct to master, TDD red-first,
with a light `/review` after** — the same call as the `brain.Orchestrator`
(ADR-0014 §6, stateless additive glue shipped to master).

Justification: the change is **additive and does not touch the router at all, nor
any concurrency contract**. The brain snapshot is assembled **once** in
`App.wire()` (boot, single goroutine); the handlers only **read** an immutable
snapshot and call the already-`-race`-safe `DroppedCount()`. New surface is a leaf
package (`internal/controlapi`) plus a `Reader` impl on `App` and a mount line in
`Build()`. The one place it **grazes tested code** is `App.wire()` /
`buildBrain` (capturing the resolved summary alongside the existing brain build),
so a **light `/review`** runs after `make quality` to check that the capture did
not perturb the tested wiring — not because concurrency is at risk (it is not),
but because additive edits to tested boot code deserve a second pass.

## Implementation (TDD red-first; no code in this ADR)

1. **`internal/controlapi` (leaf):** the `Reader` interface, the `BrainSummary` /
   `ModelSummary` / `ChannelSummary` structs, the two `GET` handlers, and a
   route-registration function that takes the mux-like `Handle` and registers
   `GET /api/brains` + `GET /api/channels`. Handlers are secret-free with the
   no-secrets invariant (§4).
2. **`App`:** assemble the brain-summary snapshot in `wire()` (retain just the
   summaries, not the whole `*config.Config`, unless a later need wants more);
   implement `controlapi.Reader`; mount the routes in `Build()` via
   `httpserver.Handle` **before** `Start`, only when the admin server exists.
3. **Tests:** each endpoint returns the expected JSON; a test asserting **no
   response contains a secret value or an env-var name** (the §4 invariant);
   `/healthz` + `/metrics` stay intact; and the `observability.disabled` case —
   with no admin server, `/api` is simply not served (documented behavior, the §5
   coupling), asserted by a test.
4. **Gate:** `make quality` green with `-race` + cross-compile ×6
   `CGO_ENABLED=0`. `go.mod` stays at **3 direct deps** (stdlib `net/http`, zero
   new).

## Resolved in review (copilot, 2026-06-28)

1. **Brain summaries — `App.wire()` boot snapshot, NOT a `router.brains`
   accessor.** Confirmed (§3): correct and strictly more additive for a read-only
   cut where brains are immutable at runtime. Documented transition: when Stage 14
   adds mutation, the `Reader` interface is unchanged — its implementation moves
   from a boot snapshot to a live registry view (§3). So the snapshot is the
   correct impl of a seam that survives Stage 14, not debt.
2. **Keep exactly TWO GETs — no `GET /api/info` in the cut.** Confirmed (§2). Two
   endpoints cover the operator-seeing-what-is-live consumer; a third
   "deployment summary" endpoint without a concrete consumer is the pattern
   deferred for the bus. `/api/info` is additive-later if a real need appears.
3. **Control API rides the observability server — conscious consequence.**
   Confirmed (§5). The boring minimal choice. Documented consequence:
   `observability.enabled = false` also disables `/api` introspection, though the
   two are conceptually distinct; wanting one without the other would force a
   future, additive, separate enablement (deferred, named).
4. **Minimal secret-free fields, env-var names omitted.** Confirmed (§2, §4).
   `/api/brains`: name, sensitivity, policy, dispatch, surviving models as
   `{provider, model_id}` only (locality dropped). `/api/channels`: type, mode,
   name, dropped count (`omitempty`). Neither `token_env` nor `api_key_env` is
   ever served. Binding invariant, test-asserted (§Implementation).
