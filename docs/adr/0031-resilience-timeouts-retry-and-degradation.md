# ADR-0031: Production resilience — timeouts, retry, and graceful degradation

> **Status:** proposed
> **Date:** 2026-07-11
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

This ADR pins ROAD-TO-BETA **Piece 2** — "handle a down/slow provider without
falling over" — the last of the two open V1 criteria after Piece 1 (user docs)
merged. It was framed via `/office-hours` and stress-tested via `/plan-eng-review`
(findings in `docs/notes/piece-2-framing.md`); the delicate points and one
hardware-verified fact below **change the framing's original shape**, so they are
fixed here before any code.

### What the code already gives (verified by reading, not memory)

- **Error grammar (intact end-to-end).** `internal/model/errors.go`: sentinels
  `ErrProviderUnavailable`, `ErrRateLimited` + the concrete `*RateLimitError{Provider,
  RetryAfter}`, `ErrAuthInvalid`, `ErrProviderResponse`. Groq maps them correctly
  (`groq.go:208` `mapHTTPError`: 401/403→`AuthInvalid`, 429→`RateLimitError`,
  5xx→`ErrProviderUnavailable`, other 4xx→`ErrProviderResponse`). **Ollama maps ALL
  non-2xx to `ErrProviderResponse`** (`ollama.go:121`) — a gap.
- **Retries today: zero.** `fanout.CallOne` calls `Generate` exactly once
  (`fanout.go:265`).
- **Degradation-with-survivors already exists.** Fan-out + `PriorityReducer` picks
  the highest-priority *successful* outcome (`priority.go:88`); `sequential.Coordinator`
  stops at the first success (`sequential.go:119`). If a model fails and another
  succeeds, the survivor already wins. Only all-failed → `ErrNoUsableOutcome` →
  fallback (`orchestrator.go:207`, `defaultFallback` `orchestrator.go:41`).
- **Timeouts today are four uncoordinated layers**, and the one that governs the
  worst case is not wired at all:

  | Layer | Site | Value | Config? |
  |---|---|---|---|
  | Router → `Brain.Handle` | `DefaultBrainHandlerTimeout` (`router/doc.go:67`) | **5s** | no |
  | Coordinator → per-model | `DefaultPerModelTimeout` (`app.go:55`), applied by `CallOne` (`fanout.go:254`) | 30s | no |
  | Adapter → HTTP | `WithRequestTimeout` (`ollama.go:57`, `groq.go:72`), fed `perModelTimeout` (`app.go:636,646`) | no |
  | (new) retry decorator | — | — | — |

  The router wiring only sets `WithErrorHandler` + `WithEventPublisher`
  (`app.go:217-224`); it never calls `WithBrainHandlerTimeout`, so it runs on the
  **5s default**. Since 5s is set first (outermost) and is the smallest, everything
  dies at 5s regardless of the 30s per-model value. **This 5s is the ~5.2s that cut
  Chano's first message**, not the 30s per-model timeout.

### The hardware-verified fact that reshapes the design (F6)

The `/plan-eng-review` flagged an empirical unknown: when Korvun cancels the POST to
a cold Ollama, does Ollama **keep loading** the model or **abort** the load? The
answer decides whether retry even helps. It was **resolved on Chano's Mac**
(iMac Intel, macOS 13, Ollama 0.30.8, `llama3.2:1b`, 2026-07-11):

> When the client disconnects **during the model load**, Ollama **ABORTS the load**.
> Server log, reproducing Chano's exact line:
> `WARN llama_server.go:1137 "client connection closed before llama-server finished
> loading, aborting load"` + `[GIN] 499 | POST "/api/chat"`. After the abort,
> `ollama ps` stays **empty** — the model does NOT keep loading. (If the cut lands
> *after* the load, during generation, the runner does survive warm — but that is not
> Chano's failure.)

**Consequence:** the framing's assumption — "retry with a short timeout rescues the
cold start" — is **false**. Each retry with the same short per-attempt timeout
re-triggers a cold load and re-aborts it at the same point; worse, it **wastes CPU**
discarding the partial load every time. Cold start is **not** a retry problem. This
fact is load-bearing for the Decision below.

Measured: cold load ~5.8s total, warm **0.86s** (llama3.2:1b). The abort behavior is
**deterministic**; the seconds vary with OS disk cache — Chano's real first-message
(disk-cold, 1.3 GB uncached after boot) is slower than a file-cached reload, which is
why his load exceeded 5s.

## Decision

Seven decisions. Each marks whether it is **beta-critical** (closes "handle a down
provider") or **deferred**.

### 1. Cold start — generous per-attempt timeout, plus optional boot warmup (BETA-CRITICAL)

The lever is **NOT retry** (F6, verified). It is:

- **(a) A generous, configurable per-attempt timeout** — mandatory. The first attempt
  must be long enough for the model load to *complete* on cold hardware (disk-cold
  worst case). A slow success beats a fast false-failure.
- **(b) Optional boot warmup for local models** — a trivial `Generate` at startup (or
  Ollama's `keep_alive`) so the model is warm before the first user message.

**Recommendation: do both, but (a) is the floor and (b) is polish.** Reasoning:

- (a) is **necessary regardless of (b)**. Ollama evicts an idle model after
  `keep_alive` (default 5 min). So even with boot warmup, a brain that goes 5 min idle
  serves the next message cold again — and only a generous per-attempt timeout saves
  that. (b) alone is insufficient.
- (a) is **provider-agnostic and one mechanism**: it covers any slow provider (a
  cold local model, a temporarily slow cloud endpoint), not just Ollama-at-boot.
- (b) is a **quality-of-life additive** that hides the first-message latency spike. It
  is best-effort (a warmup failure must NOT be fatal to boot — log and continue) and
  MAY ship as a fast-follow if (a) lands first.

**Explicit, hardware-cited:** retry with a short timeout is *counterproductive* for
cold start — it re-triggers and re-aborts the Ollama load (F6: "aborting load", 499),
never completing and wasting CPU. The retry layer (Decision 4) is therefore scoped to
**transient post-load errors only**, and its classification (Decision 5) is built so
it **never fires for the cold-load case**.

### 2. Timeout hierarchy — collapse four layers to two (BETA-CRITICAL; the delicate one)

Fix the incoherence at the root by **collapsing**, not reconciling:

```
  TWO layers, unambiguous:

  router ceiling  (WithBrainHandlerTimeout, DERIVED)  >=  worst-case Handle duration
        │
        └── per-attempt timeout  (the retry decorator, per Generate call)  = config value
```

- **The config timeout value is the PER-ATTEMPT deadline.** It is owned by the retry
  decorator (Decision 4), which applies `context.WithTimeout(ctx, perAttempt)` around
  each underlying `Generate`.
- **Remove `WithPerModelTimeout` from the coordinator** (`CallOne` stops applying a
  timeout — `fanout.go:254-258`) **and remove the double `WithRequestTimeout` on the
  adapters** from the wiring (`app.go:636,646`). One owner of the per-attempt
  deadline: the decorator. (The adapter `WithRequestTimeout` option stays in the
  adapter code for direct/test use; it is simply no longer wired.)
- **The router ceiling is DERIVED automatically by the app**, not a separate knob a
  user can misconfigure into a guillotine. Given the brain's per-model timeouts, retry
  counts, and dispatch shape:
  - **fan-out** (parallel): `ceiling = max_i( perAttempt_i × (maxRetries_i + 1) + backoffBudget_i ) + margin`
  - **sequential** (serial): `ceiling = Σ_i( perAttempt_i × (maxRetries_i + 1) + backoffBudget_i ) + margin`
  The app sets `router.WithBrainHandlerTimeout(ceiling)`. An optional explicit
  override is allowed **only if it passes validation `≥ derived`** (fail loud at config
  load otherwise — never silently guillotine a model).

**Test (concrete, F5):** a model that always times out, N retries → `Handle` returns
**by the ceiling**, not in N×perAttempt with no bound; and the ceiling ≥ derived
worst case is asserted.

### 3. Config level — per-model, with a top-level default (BETA-CRITICAL)

The timeout field goes on **`ModelConfig`** (`config.go:150`), NOT top-level or brain.
A fan-out brain mixes radically different latencies (a cold `llama3.2:1b` ~tens of
seconds vs Groq <1s); one shared value must be either generous-for-all (a down Groq
now takes 60s to fail instead of 1s) or tight (the local model never loads). Timeout is
a property of the provider/model.

- Field: **`ModelConfig.RequestTimeout`** (JSON `request_timeout`, a duration string
  e.g. `"60s"`), with a **top-level default** (JSON top-level `request_timeout`) that a
  per-model value overrides.
- The **default errs generous** (candidate 120s) — the safe floor for cold local
  loads; cloud models are tightened per-model (e.g. Groq `"15s"`). Documented: a slow
  success beats a fast false-failure.
- Retry **count** is **per-model** (`ModelConfig.MaxRetries`, JSON `max_retries`, default
  small e.g. 2, 0 disables — the transient nature is per-provider). The retry **on/off**
  is a **per-brain toggle** (`BrainConfig.Retry`, JSON `retry`, default on) — the F2
  amplification lever (Decision 4). See Decision 4/6.
- The router ceiling is derived from these per-model values (Decision 2); the config
  field never feeds the router directly.

### 4. Retry — a per-instance decorator over `model.Model`, transient-only (BETA-CRITICAL)

- **Location:** a new `internal/model/retry` decorator implementing `model.Model`,
  wrapping each adapter **per-instance** (one decorator per model, never shared — assert
  with `-race` under concurrent `Generate`). Same pattern as `brain.WithModelID`. Built
  in the app wiring; **fan-out and sequential are NOT touched** (they call `Generate`
  through the decorated model and inherit retry for free — respects the
  mechanism/policy boundary of ADR-0011).
- **Scope:** transient **post-load** errors ONLY (Decision 1). NOT the cold start.
- **Mandatory `ctx.Err()` guard (F3):** before each retry, check `ctx.Err() != nil`;
  if the parent/ceiling context is cancelled, **stop — do not retry against a dead
  context** (a shutdown or ceiling expiry must not spin retries). Load-bearing: a
  cancelled parent and a per-attempt timeout both surface as `ErrProviderUnavailable`
  wrapping `context deadline exceeded`, so `errors.Is` alone cannot tell them apart —
  the `ctx.Err()` inspection is what distinguishes "give up" from "retry".
- **Backoff:** exponential + jitter, **stdlib only** (`time` + `math/rand` behind an
  injectable clock/rand seam for deterministic tests). Low attempt cap.
- **Fan-out amplification bound (F2):** because fan-out `wg.Wait()`s on every model
  (`fanout.go:187`), a retrying dead model makes the whole request as slow as that
  model's full retry budget even when a survivor answered instantly. Bounded by (i) the
  small default `max_retries`, (ii) the derived ceiling (Decision 2), and (iii) a
  first-class **per-brain `retry` toggle** (`BrainConfig.Retry`, default on): an operator
  disables retry on a fan-out/consensus brain that already has survivors, so the survivor
  never waits on a dying model's retries. The retry COUNT stays per-model
  (`max_retries`, transient nature); the on/off for amplification is per-brain — F2's
  exact lever.

### 5. Retryable classification — hardware-grounded (BETA-CRITICAL)

The subtle part, built so retry **never fires for the cold-load case**:

| Situation | Surfaces as | Retry? | Why |
|---|---|---|---|
| Error returns **before** the per-attempt deadline: connection refused, 5xx, EOF mid-flight | `ErrProviderUnavailable` (fast) | ✅ yes | genuine transient; a second try may succeed |
| 429 rate-limit | `*RateLimitError` | ✅ yes, honor `RetryAfter` | recoverable by waiting |
| **Per-attempt deadline itself expires** (`context.DeadlineExceeded` from the decorator's own derived ctx) | `ErrProviderUnavailable` (slow) | ❌ **no** | **F6: the model was mid-load; retrying re-triggers a cold load and re-aborts. The fix was the generous timeout (Decision 1); if even that expired, retry cannot help and wastes CPU.** |
| Parent/ceiling ctx cancelled (`ctx.Err() != nil`) | any | ❌ no (stop) | F3: shutdown or ceiling — give up now |
| 401/403 | `ErrAuthInvalid` | ❌ never | credentials; retry never fixes it |
| 400/404, malformed body, empty content | `ErrProviderResponse` | ❌ no | client/config error |
| Validation (`ErrEmptyModel`, …) | validation sentinels | ❌ never | misconfiguration |

The load-bearing distinction is **"error before deadline" (retryable) vs "the deadline
expired" (not retryable)** — the decorator inspects whether its *own* per-attempt
`context.DeadlineExceeded` fired vs an error arriving earlier. This is exactly what
keeps the F6 cold-load pathology out of the retry loop.

**Ollama mapping refinement (F9, completeness):** map Ollama 5xx→`ErrProviderUnavailable`
and 429→`*RateLimitError` (as Groq, `groq.go:208`), fixing the `ollama.go:121`
"all non-2xx → `ErrProviderResponse`" gap. NOTE honestly: this is **completeness, not
Chano's fix** — his failure is a transport `context deadline exceeded` that already
maps to `ErrProviderUnavailable` and is governed by Decision 1, not by this refinement.

**`RetryAfter` capping (F10):** if a 429's `RetryAfter` exceeds the remaining
budget/ceiling, **give up early** — do not sleep-then-fail past the ceiling.

### 6. Graceful degradation — differentiated fallback (BETA-CRITICAL)

- Degradation-with-survivors **already exists** (fan-out/priority + sequential) — not
  rebuilt. Chano's case is a **single-model brain** with no survivor; only Decision 1
  (generous timeout / warmup) saves it.
- **Differentiated fallback** replaces the single `defaultFallback` string
  (`orchestrator.go:41`): distinguish **"starting/busy, try again"** (all-failed due to
  a timeout on a loading provider) from **"provider is down"** (hard failure). The
  canned fallback stays un-persisted (only real answers persist).

### 7. Circuit breaker — DEFERRED post-beta (conscious decision, not an oversight)

Retry + generous timeout + existing degradation already close "does not fall over," so
the breaker is not needed to *not crash*. It is deferred. **Honest cost, not YAGNI
hand-waving:**

- **F2** (fan-out amplification: a survivor waits for the dying model's full retry
  budget) and **F7** (`DefaultBrainWorkers = 1`, `router/doc.go`: a slow retrying
  `Handle` blocks the single worker → inbound queue fills → `ErrBrainSaturated`) are
  **exactly what a breaker would mitigate**. We accept them for beta because the retry
  budget is modest and the ceiling is bounded (Decision 2), but the cost is real.
- Re-classify the breaker as **post-beta** in `ROAD-TO-BETA.md` (currently listed as a
  Piece 2 item). Revisit if telemetry (Decision below) shows wasted retries against a
  sustained-down provider.

### Observability (BETA-CRITICAL, additive)

Behind the existing `metrics.Metrics` seam (Stage 12, ADR-0020), additive:

- `korvun_provider_retries_total{provider}` and
  `korvun_provider_retry_budget_exhausted_total{provider}`.
- **Latency semantics shift (F8):** `ObserveProviderDuration` (`orchestrator.go:232`,
  captured by `CallOne` at `fanout.go:260-263`) now measures **total time including all
  retry attempts + backoff**, not one `Generate`. Pin this meaning with a test; document
  it. (Optional per-attempt latency is a future refinement.)

## Consequences

**Easier:**
- Chano's cold-start case is fixed by a generous per-attempt timeout that is now
  **configurable per-model** (was a hardcoded 5s guillotine).
- One coherent timeout story (per-attempt + derived ceiling) replaces four
  contradictory layers; the router ceiling can no longer silently guillotine a slow
  model.
- Transient blips (5xx, dropped connections, 429) recover automatically without
  touching fan-out/sequential.
- Per-provider retry/latency visibility for the operator.

**Harder / accepted costs:**
- Fan-out latency for a brain with a dying model rises to that model's retry budget
  (F2) — bounded, not eliminated (breaker deferred).
- A slow retrying `Handle` can stall a single-worker brain's queue (F7) — bounded by
  the modest budget and derived ceiling.
- The `ObserveProviderDuration` metric changes meaning (total incl. retries) — a
  one-time semantic shift, test-pinned.
- Boot warmup (if built) adds first-message startup latency for local models and only
  helps until `keep_alive` eviction — which is why Decision 1(a) remains mandatory.

**Security/secrets:** unchanged. No new surface; the retry decorator never logs
request content or secrets (it wraps the same secret-free error grammar). ADR-0010
env-only key contract untouched.

## Alternatives Considered

- **Retry as the cold-start fix (the framing's original shape).** Rejected — F6
  verified on hardware: Ollama aborts the load on client disconnect, so retry with a
  short timeout re-aborts and wastes CPU. Cold start is a timeout/warmup problem.
- **Retry inside each adapter.** Rejected — duplicates backoff/classification across
  `ollama.go`, `groq.go`, and every future channel; re-deriving classification per
  adapter is how the P1 `%w` bug crept in (4.3, HANDOFF). One decorator, one place.
- **Retry inside the coordinator (`CallOne`).** Rejected — retry is per-provider
  (honor *this* 429's `RetryAfter`); in the coordinator it would mix the fan-out
  goroutines' clocks and re-retry the whole set. Below the coordinator (decorating the
  model) keeps each model's retry isolated in its own goroutine.
- **A single top-level timeout.** Rejected (F4) — cannot be both generous-for-local and
  tight-for-cloud. Per-model with a default is the coherent shape.
- **A separate router-ceiling config knob.** Rejected — a user could set a 5s ceiling
  that guillotines a 60s model (today's exact bug, made configurable). Derive it.
- **Circuit breaker now.** Rejected for beta (Decision 7) — not needed to avoid
  crashing; its real value (F2/F7) is deferred with the cost acknowledged.
- **A circuit-breaker or backoff library.** Rejected — stdlib (`context`, `time`,
  `math/rand`) suffices; no dependency, no Context7 needed. Revisit only if a deferred
  breaker's four-axis test ever justifies a dependency.

## Caveats (named)

- **C1 — Disk-cold vs file-cached load.** The verified ~1.5s Ollama load was
  file-cached; Chano's real disk-cold first-message is slower (>5s). The generous
  default (120s) must cover the disk-cold worst case, not the measured file-cached one.
  The abort behavior is deterministic; only the seconds vary.
- **C2 — `keep_alive` eviction.** Boot warmup only helps until Ollama evicts the idle
  model (default 5 min). Decision 1(a) (generous per-attempt timeout) is the durable
  fix; warmup is polish.
- **C3 — Fan-out amplification (F2) and single-worker stall (F7)** are accepted beta
  costs of deferring the breaker, not solved. Bounded by the modest retry budget and
  the derived ceiling.
- **C4 — Latency metric meaning changes (F8)** to total-incl-retries; a one-time,
  test-pinned semantic shift.
- **C5 — Retry never rescues a genuine per-attempt-deadline expiry** (Decision 5); that
  is intentional (F6). If a model is too slow even for its generous timeout, the answer
  is a larger `request_timeout`, not more retries.

## TDD sub-phases (reordered: cold start is timeout/warmup, not retry)

Each sub-phase: red → green, `/review`, `make quality` green with `-race` over the
whole suite before closing. No provider needed — `httptest` simulates timeout / 5xx /
429 / connection-refused.

1. **Timeout hierarchy + per-model config + derived ceiling** (Decisions 2, 3).
   `ModelConfig.RequestTimeout` + top-level default; remove the coordinator/adapter
   double-application; app derives and sets `WithBrainHandlerTimeout`. Tests:
   config parse (per-model override + default), ceiling derivation per dispatch shape,
   and the integration test — an always-expiring model, N retries, `Handle` returns
   **by the ceiling**, not N×perAttempt; explicit-override < derived fails loud.
2. **Ollama mapping refinement** (Decision 5): 5xx→`Unavailable`, 429→`RateLimited`
   via `httptest`; `errors.Is` asserted. (Red: today all non-2xx → `ErrProviderResponse`.)
3. **Retry decorator** (Decisions 4, 5): transient-only classification, the
   before-deadline vs deadline-expiry distinction, the `ctx.Err()` parent-cancel guard,
   `RetryAfter` honor + cap, backoff+jitter with injectable clock/rand, `max_retries`.
   Table-driven + `-race` under concurrent `Generate` (per-instance, not shared).
4. **THE cold-start test (Chano)** (Decisions 1, 5): an `httptest` server that responds
   after a delay > short-timeout but < generous-timeout. With the generous per-attempt
   timeout the **single** attempt succeeds; with a short timeout it fails and retry does
   **NOT** rescue it (deadline-expiry is non-retryable) — encoding F6 in a test.
5. **Boot warmup for local models** (Decision 1b, optional/fast-follow): a trivial
   `Generate` at startup for `Locality == local` models; best-effort — a warmup failure
   logs and does NOT fail boot (test the non-fatal path).
6. **Differentiated fallback + retry metrics** (Decisions 6, observability): "starting"
   vs "down" fallback text; `korvun_provider_retries_total` /
   `retry_budget_exhausted_total` via a fake `Metrics`; pin the latency-is-total meaning.

## References

- ROAD-TO-BETA.md — Piece 2; F6 "Motivación DEMOSTRADA".
- `docs/notes/piece-2-framing.md` — framing + `/plan-eng-review` findings F1–F10, F6 verified.
- ADR-0010 (env-only keys), ADR-0011 (mechanism/policy boundary), ADR-0014 (fallback
  contract), ADR-0017 §3 (dispatch shape), ADR-0020 (Metrics seam).
