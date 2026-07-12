# Sub-phase 7 — Differentiated fallback + retry metrics + F8: Design Spec

> **Status:** approved (co-pilot 2026-07-12 — FR-F2 any-hopeful-wins, FR-M2
> retry.WithMetrics wired in buildCatalog with provider-only label, FR-F8
> pin+godoc no emission change; fallback TEXTS decided by Chano — option A below)
> **Date:** 2026-07-12
> **Relates:** ADR-0031 Decision 6 (differentiated fallback), Observability
> section (retry metrics), F8 (ObserveProviderDuration now total-incl-retries).
> The LAST sub-phase of Piece 2.

## Goal

Three additive things, all behind the existing seams:
1. When ALL models fail, tell the user WHICH kind of failure it was — "starting
   up / busy, try again soon" (there is hope a retry helps) vs "provider
   unavailable" (a retry will not help; the operator must look).
2. Emit per-provider retry counters from where retry actually happens (the
   decorator): `korvun_provider_retries_total` and
   `korvun_provider_retry_budget_exhausted_total`.
3. Pin + document that `ObserveProviderDuration` now measures the TOTAL time
   including all retry attempts + backoff (F8), which is already true by
   construction once the model is decorated.

With observability off (Nop metrics) nothing new is emitted; the fallback change
is independent of metrics.

## Context (verified 2026-07-12 by reading — not memory)

- **Fallback lives in the Orchestrator** (`brain/orchestrator.go`):
  `const defaultFallback = "Sorry, no answer is available right now. Please try
  again."`; when `policy.Apply` returns `ErrNoUsableOutcome`/`ErrNoConsensus`
  (all failed) the brain returns `decisionToEnvelopes(o.fallback, env), nil`
  (line 208). At that point `res.Outcomes[]` (each `{Provider, Err, Latency,
  Response}`) IS in hand — the brain can classify the errors there. `WithFallback`
  overrides the text; the canned fallback is not persisted. (The AgentBrain has
  its own fallback and is OUT of scope — see below.)
- **Metric emission** (`orchestrator.go:226 observeOutcomes`): iterates
  `res.Outcomes`, calls `metrics.ObserveProviderDuration(oc.Provider, ok,
  oc.Latency)` + `IncProviderFailure` on failure. `oc.Latency` is captured by
  `fanout.CallOne` around `m.Generate` — and `m` is the **decorated** model, so
  `oc.Latency` already spans all retry attempts + backoff. **F8 holds by
  construction**; no emission change needed, only a pin + godoc.
- **`metrics.Metrics`** (`metrics/metrics.go`): push interface —
  `IncMessages`, `ObserveProviderDuration`, `IncProviderFailure`,
  `IncRouterError`, `ObserveTurnsPersisted` — with a `Nop` default and a leaf
  `prom` impl. Concurrency-safe by contract. Adding methods requires updating the
  interface, `Nop`, and `prom`.
- **The retry decorator** (`internal/model/retry`) is where retries happen; it
  already knows the provider via `inner.Name()`. It takes functional options
  (`WithClock`, `WithRand`), the natural place for a `WithMetrics`.
- **Error vocabulary** (`internal/model/errors.go`): `ErrProviderUnavailable`,
  `ErrRateLimited` (+ `*RateLimitError`), `ErrAuthInvalid`, `ErrProviderResponse`
  — plus stdlib `context.DeadlineExceeded`. This is the SHARED vocabulary the
  brain classifies against; the brain must NOT import `retry`.

## Section 1 — Differentiated fallback (FR-F*)

- **FR-F1 (two classes):** at the all-failed fallback point, classify the failure
  set into one of two user-facing classes:
  - **`retry-soon`** — at least one model failed with `context.DeadlineExceeded`
    (a cold load still in progress, F6) OR `model.ErrRateLimited` (429). Retrying
    in a moment has real hope.
  - **`unavailable`** — otherwise (persistent `ErrProviderUnavailable` without a
    deadline, `ErrAuthInvalid`, `ErrProviderResponse`, validation). Retrying will
    not help; the operator must look.
- **FR-F2 (aggregation rule when N models fail with DIFFERENT classes —
  `[NEEDS CLARIFICATION → recommendation]`):** **`retry-soon` wins if ANY failed
  outcome is deadline/rate-limit.** Rationale: the fallback is advice to the user,
  and "try again in a moment" is only honest if a retry could help; if even one
  model was mid-cold-load or rate-limited, a later retry has real hope, so the
  optimistic-but-honest message wins. Only when NO model gave a hopeful error
  (all hard-down/auth/bad-response) do we say "unavailable". (Rejected:
  majority-vote or highest-priority-model-wins — more complex, and a single
  loading model is exactly the case where "try again" is the right advice.)
- **FR-F3 (where it lives — boundary):** the classification lives in the
  **Orchestrator**, inspecting `res.Outcomes[].Err` with `errors.Is` against the
  `model` sentinels + `context.DeadlineExceeded`. The brain does NOT import
  `retry`; the `model` sentinels are the shared vocabulary (ADR-0011 boundary
  intact). A small pure helper `classifyFailure(outcomes) fallbackClass`.
- **FR-F4 (override precedence):** `WithFallback(text)` still wins when set
  (backward compatible — an operator-configured single fallback overrides the
  differentiation). When it is NOT set, the two differentiated defaults are used
  per class. (So today's single default splits into two defaults; an explicit
  override collapses back to one.)
- **FR-F5 (TEXTS — DECIDED by Chano, option A):**
  - `retry-soon`  → **"The model is still starting up or busy. Please try again
    in a moment."**
  - `unavailable` → **"The model provider is currently unavailable. Please try
    again later."**
  These are the final exact strings the tests pin.

## Section 2 — Retry metrics (FR-M*)

- **FR-M1 (two counters):**
  - `korvun_provider_retries_total{provider}` — incremented once per EFFECTIVE
    retry (each time the decorator commits to another attempt after a retryable
    failure, i.e. after the budget/parent checks pass, before the backoff sleep).
  - `korvun_provider_retry_budget_exhausted_total{provider}` — incremented once
    when a model gives up on a RETRYABLE error without success: either
    `attempt >= MaxRetries` or the FR-A2 give-up (the wait would exceed the parent
    budget). A non-retryable failure (auth/response/deadline/parent-cancel) does
    NOT bump it — it was never a budget question.
- **FR-M2 (injection — `[NEEDS CLARIFICATION → recommendation]`):** inject
  `metrics.Metrics` into the decorator via a new **`retry.WithMetrics(m)`** option
  (default `metrics.Nop{}`), wired in `buildCatalog`
  (`retry.New(adapter, cfg, retry.WithMetrics(b.metrics))`). Rationale: this is
  exactly how the rest of the domain receives metrics (Orchestrator
  `WithMetrics`, AgentBrain `WithAgentMetrics`, telegram) — a pushed interface,
  not a callback. `internal/model/retry` importing `internal/metrics` is the
  observability seam, not an external dependency. (Rejected: a bare callback —
  inconsistent with the domain and it would re-invent the Nop-default discipline.)
- **FR-M3 (labels — cardinality):** label **`provider` only** (from
  `inner.Name()`), NOT `model_id`. Providers are few and bounded; `model_id` is
  operator-defined and unbounded, a cardinality hazard in a counter. (ADR
  Observability section specifies `{provider}`.)
- **FR-M4 (interface additions):** add `IncProviderRetry(provider string)` and
  `IncProviderRetryBudgetExhausted(provider string)` to `metrics.Metrics`, the
  `Nop` (no-ops), and the `prom` impl (two `CounterVec`s). Godoc each.

## Section 3 — F8 (ObserveProviderDuration semantics)

- **FR-F8-1 (pin the meaning):** a test asserts that for a decorated model that
  retries once before succeeding, the `ObserveProviderDuration` value covers
  BOTH attempts + the backoff (total), not a single `Generate`. Uses an injected
  clock so the "backoff" is deterministic and the total is assertable.
- **FR-F8-2 (document — Principle 1, the metric does not lie):** update the
  `ObserveProviderDuration` godoc (and the `prom` help text) to state it measures
  the TOTAL provider time including all retry attempts and backoff when the model
  is retry-decorated (the production wiring). No emission-site code change — the
  value is already the total because `CallOne` times the decorated `m.Generate`.

## Acceptance Scenarios (Given/When/Then)

- **AS-1 (retry-soon fallback):** *Given* an all-failed fan-out where one outcome
  is `ErrProviderUnavailable` wrapping `context.DeadlineExceeded` (or
  `*RateLimitError`), *when* Handle runs, *then* the reply is the `retry-soon`
  text.
- **AS-2 (unavailable fallback):** *Given* an all-failed fan-out where every
  outcome is a hard `ErrProviderUnavailable` (conn refused) / `ErrAuthInvalid` /
  `ErrProviderResponse` (no deadline, no rate-limit), *then* the reply is the
  `unavailable` text.
- **AS-3 (aggregation — mixed classes):** *Given* two failed outcomes, one
  deadline and one hard-unavailable, *then* `retry-soon` wins (FR-F2).
- **AS-4 (override precedence):** *Given* `WithFallback("custom")`, *then* both
  classes yield "custom" (FR-F4).
- **AS-5 (retries_total):** *Given* a decorated model with `WithMetrics(fake)`
  that 503s twice then succeeds (`max_retries≥2`), *then* the fake records exactly
  2 `IncProviderRetry("ollama")` and 0 budget-exhausted.
- **AS-6 (budget exhausted):** *Given* a decorated model that always 503s with
  `max_retries=2`, *then* the fake records 2 retries and exactly 1
  `IncProviderRetryBudgetExhausted("ollama")`.
- **AS-7 (non-retryable → no counters):** *Given* an `ErrAuthInvalid` (and,
  separately, a per-attempt `DeadlineExceeded`), *then* neither counter moves.
- **AS-8 (F8 total):** *Given* a decorated model that retries once (one backoff of
  D on the fake clock) before success, *then* the observed provider duration ≈
  attempt1 + D + attempt2 (total), asserted via a fake Metrics capturing the
  duration.
- **AS-9 (observability off = no new emission):** *Given* Nop metrics, *when* a
  retry happens, *then* nothing panics and no counter exists (the Nop no-ops).

## Success Criteria

- FR-F*, FR-M*, FR-F8-* satisfied; AS-1…AS-9 pass, `-race`.
- `internal/model/retry`, `internal/brain`, `internal/metrics`,
  `internal/metrics/prom`, `internal/app` do not regress; retry stays ≥90;
  `internal/` ≥ 85.
- `make quality` green `-race` over the whole suite.
- **Zero new behaviour when observability is off** (Nop): no counters, no panic,
  and the fallback differentiation is independent of metrics (works with Nop).
- The `prom` counters register without error and carry the `provider` label only.

## Explicitly OUT of scope (named — post-beta per ADR Decision 7 / Caveats)

- **Circuit breaker** — deferred post-beta (ADR Decision 7).
- **F7 (single-worker stall / `DefaultBrainWorkers=1`)** — accepted cost,
  post-beta (ADR Caveat C3).
- **Per-attempt latency metric** (vs the total) — a future refinement (ADR F8
  note); this sub-phase pins TOTAL only.
- **AgentBrain fallback differentiation** — the AgentBrain has its own
  single-model fallback and does not use the Orchestrator's `res.Outcomes` path;
  differentiating it is a separate, later change. This sub-phase differentiates
  the Orchestrator (fan-out + sequential) only.
- **Consensus wait-all latency** — inherent to consensus, not addressed here.

## Review Checklist (green before writing tests)

- [ ] FR-F2 aggregation (`retry-soon` wins on ANY hopeful error) confirmed.
- [ ] FR-F3 classification in the Orchestrator via `model` sentinels +
      `context.DeadlineExceeded`, NO `retry` import — confirmed.
- [ ] FR-F4 `WithFallback` override precedence confirmed.
- [ ] FR-F5 TEXTS — **Chano to decide the final wording**.
- [ ] FR-M1 counter semantics (retry per effective retry; exhausted on
      budget/FR-A2 give-up, not on non-retryable) confirmed.
- [ ] FR-M2 `retry.WithMetrics` injection (vs callback) confirmed.
- [ ] FR-M3 `provider`-only label (no `model_id`) confirmed.
- [ ] FR-F8 total-semantics pin + godoc update confirmed (no emission change).
- [ ] Out-of-scope list (breaker, F7, per-attempt latency, agent fallback) agreed.
- [ ] No `[NEEDS CLARIFICATION]` left unresolved except the TEXTS (Chano's call).

---

**STOP:** paused for co-pilot review. No tests, no implementation, no commit
until FR-F2 / FR-M2 / FR-F8 are confirmed, the checklist is green, and Chano has
picked the fallback texts.
