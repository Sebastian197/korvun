# Sub-phase 4 — Retry decorator over model.Model: Design Spec

> **Status:** approved (three [NEEDS CLARIFICATION] decided by co-pilot review
> 2026-07-12 — see the resolution notes on FR-B1, FR-A1/A2/A3, D-agent)
> **Date:** 2026-07-12
> **Relates:** ADR-0031 Decisions 2, 3, 4, 5 + the intermediate-state note;
> mirrors the `brain.WithModelID` decorator pattern. This is the heaviest
> sub-phase of the ADR.

## Goal

Add a per-instance retry decorator implementing `model.Model` that wraps each
adapter, owns the per-attempt deadline for **every** dispatch shape (final SV3
state — closes the intermediate state), and retries **only** genuinely transient
post-load errors, built so it **never fires for the cold-load case** (F6). With
retry disabled it still applies the per-attempt deadline, so a hanging model
expires at `perAttempt` and never hangs (SV3).

## Context (verified 2026-07-12 by reading — not memory)

- **Decorator pattern to mirror** (`brain/named.go`): `named{inner, id}` wraps a
  `model.Model`, shallow-copies the request, delegates; `Name()` delegates to
  inner. The retry decorator follows the same shape (wrap, delegate, delegate
  `Name()`), adding the per-attempt ctx + retry loop.
- **Error grammar** (`internal/model/errors.go`, refined in sub-phase 3):
  `ErrProviderUnavailable`, `ErrRateLimited` + `*RateLimitError{Provider,
  RetryAfter}`, `ErrAuthInvalid`, `ErrProviderResponse`. Both adapters map to
  these; `model.ParseRetryAfter` is shared.
- **The load-bearing deadline matter — verified in the adapters.** On an
  in-flight timeout, `client.Do` returns a `*url.Error` wrapping
  `context.DeadlineExceeded`, and the adapter wraps it as
  `fmt.Errorf("%w: %w", model.ErrProviderUnavailable, err)`
  (`ollama.go:117`, `groq.go:171`). So a per-attempt expiry satisfies
  **BOTH** `errors.Is(err, model.ErrProviderUnavailable)` **and**
  `errors.Is(err, context.DeadlineExceeded)`. The classifier MUST test
  `context.DeadlineExceeded` (and the parent `ctx.Err()`) **before** the
  Unavailable class, or it would retry the cold-load pathology (F6).
- **Per-attempt timeout source** (`config.go`): `Config.EffectiveRequestTimeout(m)`
  resolves per-model `request_timeout` → top-level default → `DefaultRequestTimeout`
  (120s). Retry count is per-model `ModelConfig.MaxRetries` (`>=0`, 0 disables).
- **Retry on/off toggle** (`config.go:174`): `BrainConfig.Retry *bool`, nil=on.
  A `retry:true` on a sequential brain **already fails loud at config load**
  (`config.go:366-367`, tested) — SV2 config guard is done; sub-phase 4 only
  consumes the toggle.
- **Models are NOT shared between brains** (decisive for D-agent): `buildModel`
  calls `ollama.New`/`groq.New` with no cache (`app.go:658,669`); `buildCatalog`
  builds a fresh adapter per `(brain, model)`; the AgentBrain uses `selected[0]`
  from its own catalog. No cross-brain sharing exists.
- **Intermediate state today** (ADR-0031 §2 note): fan-out/sequential inherit the
  router ceiling ctx alone (coordinator/adapter timeouts removed in sub-phase 1);
  the AgentBrain still owns its per-attempt via `WithAgentPerModelTimeout`
  (`app.go:605`, `agent.go:126`), fed `b.perModelTimeout`. `ceilingForBrain`
  currently sources the agent per-attempt from `agentPerModelTimeout` and sets
  `backoffBudget = 0` (`ceiling.go:131,159`) — both change here.

## Section 1 — Decorator contract (FR-C*)

- **FR-C1 (shape):** new package `internal/model/retry` exporting a constructor
  (e.g. `retry.New(inner model.Model, cfg Config, opts ...Option) model.Model`)
  returning a value implementing `model.Model`. `Name()` delegates to `inner`
  (attribution stays the provider identity, like `named.Name()`).
- **FR-C2 (per-attempt owner, every attempt incl. 0th):** on each attempt —
  including attempt 0, with retry ON or OFF — the decorator derives
  `attemptCtx, cancel := context.WithTimeout(parentCtx, perAttempt)` and calls
  `inner.Generate(attemptCtx, req)`, cancelling before the next iteration. It is
  the **single owner** of the per-attempt deadline (final SV3 state).
- **FR-C3 (per-instance, never shared):** one decorator per model instance; safe
  under concurrent `Generate` with `-race` (no shared mutable state; the injected
  rand source must be guarded or per-instance — see FR-B3).
- **FR-C4 (request not mutated):** the decorator passes `req` straight through
  (it does not copy/mutate Model; `WithModelID` remains responsible for the id).
  It must not write shared request fields (same race discipline as `named`).

## Section 2 — Retryable classification (one FR per row)

Evaluated **after each failed attempt, in this exact order** (order is
load-bearing — F3 before F6 before the Unavailable class):

| FR | Condition (in order) | Action | Why |
|----|----------------------|--------|-----|
| **FR-R1 (F3)** | `parentCtx.Err() != nil` (parent/ceiling cancelled or expired) | **STOP**, return err — no retry | shutdown / ceiling / a fan-out-cancelled sibling (sub-phase 2). The parent ctx, NOT the derived attemptCtx, is inspected. |
| **FR-R2 (F6)** | `errors.Is(err, context.DeadlineExceeded)` (parent still alive ⇒ it was OUR per-attempt that expired) | **NO retry**, return err | per-attempt deadline expiry = mid-load; retrying re-triggers a cold load and re-aborts (hardware-verified). |
| **FR-R3** | `errors.As(err, &*model.RateLimitError)` | **retry**, wait `max(backoff, cappedRetryAfter)`; give up if the wait exceeds remaining budget (FR-A2) | recoverable by waiting |
| **FR-R4** | `errors.Is(err, model.ErrProviderUnavailable)` (and NOT DeadlineExceeded — already excluded by FR-R2) | **retry** with backoff | genuine fast transient: connection refused, 5xx, EOF mid-flight |
| **FR-R5** | `errors.Is(err, model.ErrAuthInvalid)` | **NO retry**, return err | credentials; retry never fixes it |
| **FR-R6** | `errors.Is(err, model.ErrProviderResponse)` | **NO retry**, return err | 4xx / malformed / empty; a bad response does not improve by repeating |
| **FR-R7** | anything else (validation sentinels, unknown) | **NO retry**, return err | misconfiguration / unknown — fail closed, never spin |
| **FR-R8** | retryable (R3/R4) but `attempts > maxRetries` | return the last err | attempt cap reached |

**Load-bearing note (pin in a comment + a test):** FR-R1 inspects the *parent*
ctx; FR-R2 inspects the *error*. Both a cancelled parent and an expired
per-attempt surface as `ErrProviderUnavailable` wrapping a ctx error, so
`errors.Is` alone cannot separate "give up" (F3) from "per-attempt expired" (F6)
from "retryable transient" — the parent `ctx.Err()` check is what disambiguates.

## Section 3 — Backoff + jitter (injectable clock/rand)

- **FR-B1 (schedule, `[NEEDS CLARIFICATION → recommendation]`):** exponential,
  **base = 200ms**, **multiplier = 2** (200ms, 400ms, 800ms, …), **per-wait cap =
  2s**. **Full jitter** (AWS-style): `wait = rand[0, min(perWaitCap, base·2^n)]`.
  Rationale: full jitter decorrelates concurrent fan-out retries (avoids a
  thundering herd against a recovering provider) better than equal/decorrelated
  jitter, and is trivially deterministic under an injected rand. With the default
  `max_retries = 2` and the 2s per-wait cap, the per-model backoff budget is
  ≤ ~4s — modest, keeping the derived ceiling in the ~2 min order (ADR §2).
- **FR-B2 (injectable clock — zero real sleeps in tests):** a minimal seam, e.g.
  `Clock` with `Sleep(ctx, d) error` (returns early on `ctx.Done()`), defaulting
  to a `time`-backed impl; tests inject a fake that records the requested
  durations and returns instantly. No test sleeps in wall-clock time. (Mirrors
  the existing `now` seam in `agent.go:62`.)
- **FR-B3 (injectable rand, race-safe):** an injected rand source
  (`func() float64` or `*rand.Rand` behind `WithRand`), stdlib `math/rand` only.
  Per-instance or mutex-guarded so concurrent `Generate` is `-race` clean.
  Determinism in tests comes from a fixed seed / stubbed function, not from
  `Math.random`-style nondeterminism.
- The `Sleep` must be **ctx-aware**: a parent cancel during a backoff wait
  returns immediately and then hits the FR-R1 guard (no sleeping past a dead
  ctx).

## Section 4 — RetryAfter cap (F10)

- **FR-A1 (`[NEEDS CLARIFICATION → recommendation]`):** the honored wait for a 429
  is `cappedRetryAfter = min(RetryAfter, retryAfterCap)` with an absolute
  **`retryAfterCap` default = 30s**, AND further bounded by the remaining parent
  budget (FR-A2). Rationale: a 429 with `Retry-After: 3600` must never sleep an
  hour and blow the ceiling; 30s is a generous absolute ceiling for a transient
  rate-limit while staying an order below the ~2 min brain ceiling.
- **FR-A2 (budget guard — give up early, never sleep-then-fail past the
  ceiling):** if `parentCtx` has a deadline and the intended wait
  `> time.Until(deadline)`, **do not sleep — give up now** and return the err.
  The decorator reads `parentCtx.Deadline()`; with no deadline it falls back to
  `retryAfterCap`. This guarantees a retry wait can never consume budget the
  brain does not have.
- **FR-A3 (ceiling reflects the budget — RESOLVED):** `ceilingForBrain` (today
  `backoffBudget = 0`, `ceiling.go:159`) must populate
  `backoffBudget_i = maxRetries_i · perWaitCap` (= `maxRetries_i · 2s`). It
  budgets the **backoff** only, NOT the RetryAfter — a long RetryAfter wait is
  trimmed at runtime by FR-A2 (give up if it exceeds the remaining parent
  budget), so it never needs a ceiling term. (The earlier confusing
  `min(2s, 30s)` phrasing is dropped.) Sequential stays sum-of-perAttempt with no
  backoff term (retry off).

## Section 5 — D-agent decision (`[NEEDS CLARIFICATION → recommendation]`)

**Question:** with the decorator owning the per-attempt deadline, do we **remove**
`WithAgentPerModelTimeout` from the AgentBrain (single owner, per-model
configurable `request_timeout`) or **keep it nested** (finer per-agent budget
over a model that might be shared between brains)?

**Analysis of the real wiring:** models are **not shared** between brains —
`buildModel` creates a fresh adapter per `(brain, model)` with no cache
(`app.go:658,669`), and the AgentBrain consumes `selected[0]` from its own
catalog. The "finer budget over a shared model" premise is therefore **vacuous**;
there is no sharing to protect.

**Recommendation: REMOVE `WithAgentPerModelTimeout`** (wire the agent's
`perModelCall` to 0, so `fanout.CallOne` applies no timeout — `agent.go:126-129`
already documents "non-positive leaves the call sharing the Handle ctx"), letting
the decorated model own the per-attempt deadline. Reasons:

1. No cross-brain sharing ⇒ the keep-it argument is empty in practice.
2. **Single owner (final SV3):** keeping it would nest two deadlines over the same
   `Generate` (the decorator's + `CallOne`'s), re-introducing the double
   application sub-phase 1 removed for fan-out/sequential.
3. The agent per-attempt becomes **per-model configurable** (`EffectiveRequestTimeout`),
   strictly more expressive than the global `b.perModelTimeout` (30s) it uses today.

**Consequence for `ceiling.go` (in scope here):** `ceilingForBrain` must source
the agent's per-attempt from `cfg.EffectiveRequestTimeout(bc.Models[0])` instead
of `agentPerModelTimeout` (`ceiling.go:141`), and `deriveRouterCeiling`'s
`agentPerModelTimeout` parameter is dropped/ignored.

**RESOLVED (co-pilot 2026-07-12):** remove `WithAgentPerModelTimeout`; the agent
per-attempt is `EffectiveRequestTimeout(bc.Models[0])`. The ADR-0031
intermediate-state note (which still says the AgentBrain keeps
`WithAgentPerModelTimeout`) is updated in the sub-phase CLOSE commit, **not
now**. NOTE: the sub-phase-1 test that pins the agent ceiling to
`agentPerModelTimeout` will COLLIDE with this change — reported to Chano, awaiting
his decision before green (do not silently adjust it).

## Section 6 — Wiring (FR-W*)

- **FR-W1 (where):** the decorator is applied **per instance in `buildCatalog`**
  (it has both `bc` and `m`, and — via the builder — `cfg` for
  `EffectiveRequestTimeout`), wrapping the raw adapter from `buildModel` **before**
  `WithModelID`: `Model: brain.WithModelID(retry.New(adapter, retryCfg), m.ModelID)`.
  This covers **all** shapes uniformly (fan-out, sequential, agent, single) — the
  agent gets the decorated model through `selected[0]`. `buildModel` keeps
  returning the raw adapter.
- **FR-W2 (retry cfg per model+brain):** `perAttempt = cfg.EffectiveRequestTimeout(m)`;
  `maxRetries = m.MaxRetries`; `enabled = retryEnabledForBrain(bc)` where the
  per-brain toggle `BrainConfig.Retry` (nil=on) decides, **EXCEPT sequential = OFF
  by construction** (SV2) regardless of the toggle (and `retry:true` on sequential
  is already rejected at config load). When `enabled == false`, `maxRetries` is
  forced to 0 — the decorator still applies the per-attempt deadline (SV3).
- **FR-W3 (agent):** `buildAgentBrain` drops `WithAgentPerModelTimeout` (D-agent);
  `ceilingForBrain` updates the agent per-attempt source (Section 5 consequence).
- **FR-W4 (no behavioural change to fan-out/sequential mechanisms):** the
  decorator sits below the coordinator (they call `Generate` through the decorated
  model); `fanout.Run`/`sequential` code is untouched (ADR-0011 boundary; SV1
  cancellation already landed in sub-phase 2).

## Section 7 — Committed bite-proofs (Success Criteria)

- **BP-a (SV2 must bite):** with retry temporarily enabled for a sequential model,
  `TestRun_failingModelConsumesExactlyOneAttempt` (sequential/sv2_oneattempt_test.go)
  must turn **RED** (calls > 1). Procedure: inject the retrying decorator into a
  sequential path in a throwaway test, observe red, revert — documented in the
  report. (The config guard already blocks `retry:true`; this proves the mechanism
  guard bites too.)
- **BP-b (F3 sibling):** a fan-out with a fast-OK model + a slow model → the slow
  sibling is cancelled at first usable success (sub-phase 2) and its decorator,
  seeing `parentCtx.Err() != nil`, fires **zero** retries. Assert the slow model's
  attempt count did not multiply.
- **BP-c (F6 per-attempt):** an `httptest` server that delays past `perAttempt` →
  the decorated model (retry enabled) is invoked **exactly once**, never twice
  (deadline-expiry is non-retryable). Assert call count == 1.

## Acceptance Scenarios (Given/When/Then)

- **AS-1 (FR-C2 / SV3, retry OFF):** *Given* a decorator with retry disabled over
  a model that hangs, *when* `Generate` runs, *then* it returns by `perAttempt`
  wrapping `ErrProviderUnavailable`+`context.DeadlineExceeded`, and the inner model
  was called exactly once.
- **AS-2 (FR-R4):** *Given* a model returning `ErrProviderUnavailable` (5xx, fast)
  then success, with `max_retries=2`, *then* `Generate` succeeds and the inner was
  called twice; the fake clock recorded one backoff wait in `[0, 400ms]`.
- **AS-3 (FR-R2 / F6):** *Given* a model whose per-attempt always expires, *then*
  `Generate` returns after exactly **one** call (no retry) with the deadline error.
- **AS-4 (FR-R1 / F3):** *Given* a parent ctx cancelled between attempts, *then*
  the next iteration stops immediately with zero further calls.
- **AS-5 (FR-R3 + FR-A1):** *Given* a `*RateLimitError{RetryAfter: 3s}` then
  success, *then* the fake clock recorded a wait of `min(3s, 30s)` bounded by
  backoff/budget, and the second attempt succeeded.
- **AS-6 (FR-A2):** *Given* a `RetryAfter` (or backoff) exceeding the remaining
  parent budget, *then* the decorator gives up **without sleeping** and returns
  the err.
- **AS-7 (FR-R5/R6):** *Given* `ErrAuthInvalid` (and, separately,
  `ErrProviderResponse`), *then* `Generate` returns after exactly one call.
- **AS-8 (FR-C3):** concurrent `Generate` on one decorator instance is `-race`
  clean over `-count=20`.
- **AS-9 (FR-A3 ceiling):** the derived fan-out ceiling for a brain with
  `max_retries=2` includes the backoff budget term (non-zero), asserted in
  `ceiling` tests.

## Success Criteria

- FR-C*, FR-R1..R8, FR-B*, FR-A*, FR-W* satisfied; AS-1..AS-9 pass as
  table-driven tests, `-race`, with the fake clock/rand (zero real sleeps).
- **BP-a, BP-b, BP-c** demonstrated (SV2 bites, F3 sibling no-retry, F6 one
  attempt).
- New `internal/model/retry` package coverage **≥ 90%**; `internal/model`,
  `internal/app` (ceiling), `internal/config` do not regress; `internal/` ≥ 85%.
- `make quality` green `-race` over the WHOLE suite.
- SV3 holds end-to-end: no `Generate` path without a per-attempt deadline; the
  decorator is the single owner for all four shapes.

## Explicitly OUT of scope (named, per ADR sub-phase order)

- **Cold-start test (Chano)** — sub-phase 5 (uses the decorator built here).
- **Boot warmup** for local models — sub-phase 6.
- **Differentiated fallback text + retry metrics** (`korvun_provider_retries_total`,
  `retry_budget_exhausted_total`) and the **F8 latency-is-total** semantic shift —
  sub-phase 7. This sub-phase builds the decorator + classification + wiring only;
  it emits no new metrics.

## Review Checklist (green before writing tests)

- [ ] Decorator contract (FR-C1..C4) unambiguous; mirrors `WithModelID`; single
      per-attempt owner incl. attempt 0 with retry on/off (SV3).
- [ ] Classification order F3 → F6 → RateLimit → Unavailable → Auth/Response/other
      is explicit and justified (the `parentCtx.Err()` vs error distinction pinned).
- [x] FR-B1 backoff values (200ms / ×2 / 2s cap / full jitter) **CONFIRMED**.
- [x] FR-A1 `retryAfterCap` (30s) + FR-A2 budget guard **CONFIRMED**;
      FR-A3 ceiling `backoffBudget_i = maxRetries_i · 2s` **CONFIRMED**.
- [x] **D-agent** (remove `WithAgentPerModelTimeout`) **CONFIRMED** by co-pilot,
      incl. the `ceiling.go` agent per-attempt source change (collision on the
      sub-phase-1 agent-ceiling test to be reported, not silently adjusted).
- [ ] Wiring (FR-W1..W4): decorator in `buildCatalog`, all shapes, sequential
      forced off, agent decorated via `selected[0]`.
- [ ] BP-a/b/c committed and scoped; out-of-scope list agreed.
- [ ] No `[NEEDS CLARIFICATION]` left unresolved.

---

**STOP:** paused for co-pilot review. No tests, no implementation, no commit
until FR-B1 / FR-A1 / D-agent are confirmed and the checklist is green.
