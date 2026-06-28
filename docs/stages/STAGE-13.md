# Stage 13 — Control API (read-only operator introspection)

> **Status:** closed
> **Started:** 2026-06-28
> **Closed:** 2026-06-28
> **ADR:** [ADR-0022](../adr/0022-control-api.md) (accepted)

## Objective

Let an operator **ask the running binary what it is actually wired to**, in a form
that is neither the raw config file nor a Prometheus scrape: which brains exist,
what each declares, **which models survived the privacy selector**, which channels
are connected and how many messages each has dropped. Korvun is observable for
"is it alive / how slow / what's failing" (Stage 12); Stage 13 adds "what is
wired and live right now."

The stage was framed with `/office-hours` (alternatives generation) and
`/plan-eng-review` (blast radius, boring-by-default, reversibility, and above all
the real-consumer-vs-speculative and security lenses), then pinned by ADR-0022
before any code. The framing note is preserved at
`docs/notes/stage-13-control-api-framing.md`.

## The cut — read-only now, mutation deferred to Stage 14

The honest Step 0 verdict, the same lens applied to the bus (Stage 10, deferred)
but with the **opposite outcome because the situation differs**:

- **Read has a real consumer today** — an operator with `curl`. The headline
  datum, *which models survived the privacy selector for a Private brain*, exists
  **only in the running binary** (`policy.SelectModels` runs at boot); it is not
  in the config file, not in `/metrics`. The bus had zero consumers; read-only
  introspection has one.
- **Mutation has no real consumer until the builder** (Stage 14), and the router
  registry is boot-time (`RegisterBrain`/`Route` run once), so live mutation is a
  concurrency/lifecycle change, not a handler — deferred.
- **The load-bearing insight is security.** The admin server is loopback, no auth
  (ADR-0020 §4), *justified because it only exposes, never mutates*. A read-only
  control API keeps that calculus **exactly intact** — zero new attack surface.
  **Mutation** is what would break it. So **deferring mutation is not a scope cut,
  it is the security decision** — it is what keeps "loopback, no auth" correct.

## What landed

A new leaf `internal/controlapi` plus additive wiring in `internal/app`. **The
router is 100% untouched** (verified: empty diff in `internal/router/`). Two
commits on master:

- **`docs` ADR-0022 accepted `d491b6f`** — the contract, with the four review
  resolutions documented.
- **`feat` controlapi `ac88478`** — the implementation (TDD red-first):
  - **`internal/controlapi`** (leaf, stdlib only): the `Reader` seam
    (`BrainSummaries()` / `ChannelSummaries()`), the `BrainSummary` /
    `ModelSummary` / `ChannelSummary` JSON structs, a `Mounter` interface (so the
    leaf never imports `httpserver`), `Register` (mounts the two routes), the two
    GET handlers, and `writeJSON` (marshal-first so an error is a 500 before any
    header).
  - **`internal/app`** assembles a boot **snapshot** of brain summaries in
    `wire()` (where the config is in hand and the selector runs), reads channel
    drop counts **live**, implements `controlapi.Reader`, and mounts the routes in
    `Build()` via `httpserver.Handle` **before** `Start`. `buildBrain` is left
    untouched — the summary is derived from config by a pure `brainSummary`
    helper.

```
   internal/controlapi (leaf, net/http only)          internal/httpserver
        │ depends on the Reader + Mounter seams             (existing admin server,
        ▼ (never on router/brain concretes)                 /healthz + /metrics)
   controlapi.Reader  ── implemented by *app.App                 ▲
        ▲                                                        │ App mounts
        │ snapshot (brains) + live atomic read (drops)           │ GET /api/* in Build
   internal/app.wire() builds it once at boot  ──────────────────┘  (before Start)
```

## The two endpoints (the minimal cut — exactly two GETs)

| Endpoint | Returns |
|----------|---------|
| `GET /api/brains` | per brain: `name`, `sensitivity`, `policy`, `dispatch`, and `models` = the survivors of the privacy selector, each `{provider, model_id}` (post-`SelectModels`, only the running binary knows this). |
| `GET /api/channels` | per channel: `type`, `mode`, `name`, and live `dropped` count (omitted for a channel with no counter). |

`GET /api/info` was **rejected from the cut** — two endpoints cover the
operator-seeing-what-is-live consumer; a third "deployment summary" without a
concrete consumer is the pattern deferred for the bus. Additive-later if a real
need appears.

## The four decisions (ADR-0022, resolved in review)

| # | Decision | Why |
|---|----------|-----|
| 1 SNAPSHOT | Brain summaries are an **`App.wire()` boot snapshot**, not a `router.brains` accessor | Brains are immutable at runtime in a read-only cut, so the snapshot is the live truth for the process lifetime — and it leaves the router **completely untouched** (strictly more additive). The `Reader` interface survives Stage 14: when mutation arrives, only the *implementation* moves from a snapshot to a live registry view. The snapshot is the correct impl of a seam that survives Stage 14, not debt. |
| 2 TWO GETS | Exactly **two GET endpoints**, no `/api/info` | Two cover the real consumer; a third without a consumer is the deferred-bus pattern. |
| 3 RIDES OBS | The control API **rides the observability server** | The boring minimal choice. Documented consequence: `observability.enabled = false` also disables `/api`, though the two are conceptually distinct. Wanting one without the other would force a future additive separate enablement (deferred, named). |
| 4 SECRET-FREE | **Minimal secret-free fields; env-var names never served** | `/api/brains` carries surviving models as `{provider, model_id}` only (no locality, the brain's sensitivity already conveys the posture). Neither `token_env` nor `api_key_env` is ever served — the operator does not need to know *which* env var holds a secret, only that the binary is wired. A binding invariant, test-asserted end to end. |

**Security posture (the load-bearing decision).** Read-only loopback, no auth =
the same profile as `/metrics`, valid because nothing mutates. **AUTH is the
trigger of MUTATION (Stage 14):** when mutation arrives, a token becomes essential
(the builder may run as a separate process and/or bind beyond loopback). Starting
read-only is what keeps the auth requirement low.

## Review findings (`/review` — light, additive change grazing `wire()`)

The adversarial pass (fresh context) confirmed every load-bearing invariant by
trying to break it:

- **Additive — router 100% untouched** (empty `internal/router/` diff).
- **Race-safe** — the snapshot is built once in `wire()` (boot, single goroutine)
  and never mutated; `BrainSummaries()` deep-copies (outer slice + per-brain
  `Models`); `ChannelSummaries()` reads `telegram.DroppedCount()` =
  `atomic.Uint64.Load()`; the `&dropped` pointer is a fresh per-iteration local
  (no aliasing). Lock-free concurrent reads are sound.
- **Secret-free** — only `provider`/`model_id` and channel `type`/`mode`/`name`
  reach a body; no secret value and no env-var name. Asserted end to end.
- **No selector drift** — `brainSummary` applies the identical rule as
  `policy.SelectModels` over the same `bc.Models` in the same order; the only
  divergence (empty result) is **unreachable** because `buildBrain` fails the boot
  before `brainSummary` runs in the same `wire()` iteration.
  `TestBrainSummary_matchesSelector` cross-checks it for public and private.
- **Read-only enforced by construction** — a `POST` to a `GET /api/...` route is a
  405 at the mux; observability-disabled ⇒ `adminServer == nil` ⇒ `/api` not
  served (tested), while `Reader` still functions.

**F1 (P3, deferred) — agent brains report inert `dispatch`/`policy`.** A brain
with an `agent` block runs `buildAgentBrain` (a single-model tool loop) that uses
neither the coordinator (`dispatch`) nor the reducer (`policy`), yet the summary
still reports them from config. The headline `models` field stays correct; nothing
leaks and nothing crashes. Deciding the API shape for agents (omit / mark N/A /
flag as agent) is a conscious API-form decision (ADR-0022 §2 does not carve out
agents) deferred from Stage 13 — recorded in HANDOFF's deferred list, likely
revisited with Stage 14's mutation surface.

## Live verification (operator's use case)

The full `korvun` binary cannot `Build` without a real Telegram token (the `getMe`
check inside `bot.New`), so — exactly as Stage 12 did — the live check ran the
production packages (`internal/app.Build` + `internal/httpserver` +
`internal/controlapi`) wired the way the binary wires them, over a real socket,
with a two-brain config: a **Private** brain (`ollama` local + `groq` cloud) and a
**Public** one (same two), plus a telegram channel with 3 dropped messages.

```
GET /api/brains ->
[{"name":"private-brain","sensitivity":"private","policy":"priority","dispatch":"sequential",
  "models":[{"provider":"ollama","model_id":"llama3.2"}]},
 {"name":"public-brain","sensitivity":"public","policy":"consensus","dispatch":"fanout",
  "models":[{"provider":"ollama","model_id":"llama3.2"},
            {"provider":"groq","model_id":"llama-3.3-70b-versatile"}]}]

GET /api/channels ->
[{"type":"telegram","mode":"polling","name":"telegram","dropped":3}]
```

This is the headline made concrete: the **Private** brain reports **only the
`ollama` survivor** — the `groq`/cloud model was dropped by the privacy selector
at boot and is therefore absent — while the **Public** brain reports both. That
"which models actually remain dispatchable for this brain" answer exists nowhere
else (not the config, not `/metrics`). Both responses are secret-free: no value,
no env-var name. `/healthz` and `/metrics` answered 200 alongside on the same
server.

## What is NOT in scope (recorded, not silently dropped)

- **All mutation** — create/update/delete brains, routes, channels; hot config
  reload; start/stop a channel. The router registry is boot-time; mutation is
  Stage 14, and brings auth with it.
- **Conversation browsing / history endpoints** — needs a new
  `conversation.Store` query capability (it is append-only + `LoadRecent` today);
  a builder feature.
- **Detailed / per-provider health** — provider health is not tracked
  (ADR-0014 §3 keeps `/healthz` liveness-only).
- **Auth / token / RBAC / TLS / network exposure (`0.0.0.0`)** — the mutation
  trigger; lands with Stage 14's state-changing surface.
- **`GET /api/info`** — additive-later if a concrete consumer appears.
- **The no-code builder UI** — Stage 14.

## Dependency

`go.mod` stays at **3 direct deps** — Stage 13 adds **none**. The control API is
stdlib `net/http` over the `http.ServeMux` Stage 12 already serves; Go 1.26 gives
method-pattern routing natively, so no third-party router.

## Quality gate

`make quality` green with `-race` over the whole tree (total coverage **92.1%**;
`internal/controlapi` **100%**, `internal/app` 90.5%, the ≥90% packages —
policy/router/envelope/brain — met). Cross-compile ×6 `CGO_ENABLED=0` green. The
seam invariant holds: `controlapi` imports only the standard library; the router
is untouched.
