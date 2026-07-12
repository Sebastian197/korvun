# Sub-phase 6 — Boot warmup for local models: Design Spec

> **Status:** approved (co-pilot 2026-07-12 — FR-C2 fail-loud, FR-M2 via decorated
> model, FR-B1 background after Start; plus two added suite requirements: the
> benign-503 guard and the Shutdown-during-warmup race test)
> **Date:** 2026-07-12
> **Relates:** ADR-0031 Decision 1(b) (boot warmup, optional/fast-follow) + F6
> (cold load / abort-on-disconnect) + Caveat C2 (`keep_alive` eviction).

## Goal

Optionally warm up local models at boot so the first real user message does not
pay the cold-load latency spike. It is **opt-in per model**, **best-effort** (a
warmup failure NEVER fails boot), and honors F6 (a cold load is given a generous
window and is NEVER retried into a re-abort). With the toggle absent/false the
behaviour is byte-for-byte unchanged.

## Context (verified 2026-07-12 by reading — not memory)

- **Boot flow** (`app.go`): `Run` → `Start` (admin server FIRST — ADR-0020 §4 —
  then every channel) → `Serve` (`<-ctx.Done()`). `Shutdown` stops channels →
  router → store; the caller cancels the `ctx` passed to `Run`.
- **Decorated models live inside the brains.** `buildCatalog` builds each
  adapter, wraps it `brain.WithModelID(retry.New(adapter, cfg), id)`, and feeds
  the selector; `App` holds no direct model handle. Warmup targets must be
  collected where the model is built (`buildCatalog` has `m ModelConfig` + the
  decorated model) and carried onto `App`.
- **`ModelConfig`** has `Provider`, `Locality` (`local`/`cloud`), `BaseURL`,
  `ModelID`, `RequestTimeout`, `MaxRetries` — **no `Warmup` field yet**.
  `Config.EffectiveRequestTimeout(m)` already resolves the generous per-attempt
  window the decorator applies.
- **`model.Request`** is `{Model, Messages}` — **no MaxTokens** in the canonical
  contract, so a warmup is a minimal `Generate` with one short user message
  (`"hi"`). `WithModelID` overwrites `Request.Model` with the bound id, so the
  warmup request's `Model` field is set from the collected id for clarity.
- **Retry decorator (sub-phase 4)** applies `EffectiveRequestTimeout` per attempt
  and NEVER retries a `context.DeadlineExceeded` (R2/F6); it DOES retry a fast
  `ErrProviderUnavailable` (503) and honors `*RateLimitError`.

## Section 1 — Config toggle (FR-C*)

- **FR-C1 (`[NEEDS CLARIFICATION → recommendation]`):** add
  **`ModelConfig.Warmup bool`** (JSON `warmup`, default `false`). A `true` marks
  this local model to be warmed at boot. Absent/false → no warmup, zero
  behaviour change.
- **FR-C2 — warmup on a cloud model (`[NEEDS CLARIFICATION → recommendation]`):**
  **fail loud in `config.Validate`** (`ErrInvalidConfig`, naming
  `brains[i].models[j].warmup`) when `warmup:true` and `locality != "local"`.
  Rationale: a cloud model has no cold load to hide, and a warmup call bills the
  user real money (tokens). `warmup:true` on a cloud model is almost certainly a
  copy-paste mistake; catching it at config load — before any spend and before
  boot — matches the project's fatal-config-error rule (ADR-0017 §5) and is the
  money-safe default. (Rejected: warning + ignore — laxer, and one future change
  to the gate could silently start charging the user. Fail loud.)

## Section 2 — Warmup mechanics (FR-M*)

- **FR-M1 (what):** one minimal `Generate(ctx, &model.Request{Model: id,
  Messages: []Message{{Role: RoleUser, Content: "hi"}}})` per target. The
  canonical request has no max-tokens knob; `"hi"` keeps generation trivial while
  still forcing the model LOAD (the real F6 cost).
- **FR-M2 (through the DECORATED model — recommendation):** call the **decorated**
  `model.Model` (the same object that serves production), NOT the raw adapter.
  Rationale — this gets the F6 guarantee for free:
  - The decorator applies the **generous `EffectiveRequestTimeout`** the operator
    already configured for the cold load — no second timeout to re-derive.
  - If the cold load **completes** within that window → one attempt, success.
  - If it **exceeds** the window → `context.DeadlineExceeded` → **R2/F6: NOT
    retried** — exactly the "a retry would re-abort the load" guarantee, obtained
    for free.
  - A fast `503` (service still coming up, not yet listening) IS retried — which
    is correct and benign for warmup (no load has started to re-abort; it just
    means "Ollama isn't listening yet").
  - Warming the exact production object also warms the real serving path.
  (Rejected: raw adapter — would lose the configured `EffectiveRequestTimeout`
  and hand-roll a timeout, buying no extra guarantee. "Never retry" is already
  satisfied for the only case that matters, F6.)
- **FR-M3 (parent ctx):** the warmup passes the **app boot ctx** (cancelable in
  Shutdown); the decorator owns the per-attempt window. No extra deadline is
  stacked on top (single-owner discipline, SV3).
- **FR-M4 (dedup):** collect targets deduplicated by `(provider, baseURL,
  modelID)` — the same local model used in two brains builds two adapter
  instances, but warming the Ollama server twice is redundant (the 2nd finds it
  already warm). Warm each distinct backend model once. (log what was deduped.)

## Section 3 — When & how in boot (FR-B*)

- **FR-B1 (background, after Start — `[NEEDS CLARIFICATION → recommendation]`):**
  run warmups in the **background AFTER `Start` returns** (admin + channels up),
  NOT blocking before channels. `Run` launches the warmup goroutine(s) with the
  boot `ctx`, then proceeds to `Serve`. Rationale:
  - Boot stays **instant**; a slow or hung model never delays the service coming
    up (best-effort must not gate availability).
  - The point of warmup is to *hide* first-message latency, not guarantee it. If
    a real message arrives mid-warmup, sub-phase 1's generous per-attempt timeout
    already lets that message complete the same in-flight load (Ollama shares the
    loaded model via `keep_alive`), so nothing is lost — the warmup merely
    front-runs the load.
  - Blocking-before-channels would lengthen boot by seconds × N models and let a
    single stuck model stall startup; the weak "guaranteed warm on message #1" it
    buys is not worth that (and only holds if the operator waits).
  `Start` keeps its narrow "fallible resource startup" contract — the best-effort
  warmup does not belong inside it.
- **FR-B2 (parallel across models):** launch each distinct target's warmup in its
  own goroutine (parallel), tracked by a `sync.WaitGroup`. Parallel is faster;
  Ollama serializes/serves loads per its own resources. Caveat logged: on a
  memory-poor host (Raspberry Pi) N parallel loads compete — but the operator
  chooses what to mark `warmup`.
- **FR-B3 (Shutdown-safe):** the warmup ctx hangs off the boot ctx, so Shutdown
  (caller cancels ctx) cancels any in-flight warmup; each goroutine returns
  promptly (the decorator sees ctx cancelled → stops; Ollama aborts the load —
  acceptable for best-effort warmup). `App` holds a `warmupDone chan struct{}`
  (closed when the WaitGroup drains) so Shutdown can await warmup completion
  bounded by its own ctx, leaving no goroutine dangling after Shutdown returns.

## Section 4 — Best-effort (FR-E*)

- **FR-E1:** a warmup error → **`WARN` log** with `provider`, `model`, `error`;
  boot continues. No warmup error ever propagates to `Start`/`Run`.
- **FR-E2:** NO metric is emitted (retry/warmup metrics are sub-phase 7 — not
  mixed in here).

## Section 5 — Minimal observability (FR-O*)

- **FR-O1:** `INFO "warming up model"` (`provider`, `model`) when a target starts.
- **FR-O2:** `INFO "model warm"` (`provider`, `model`, `took`) on success, where
  `took` is `time.Since(start)` (real clock; tests assert the log line fired, not
  the exact duration).
- **FR-O3:** the `WARN` of FR-E1 on failure. The operator can read from the log
  exactly what warmed, how long it took, and what failed.

## Acceptance Scenarios (Given/When/Then)

- **AS-1 (happy warm):** *Given* a local model with `warmup:true` and an httptest
  backend that answers a `"hi"` chat after a short delay, *when* the app runs,
  *then* the backend receives a warmup request and an `INFO "model warm"` is
  logged; the app is serving (Start returned before the warmup completed).
- **AS-2 (best-effort failure):** *Given* a `warmup:true` model whose backend
  returns 500 (or is unreachable), *when* the app runs, *then* boot succeeds,
  `Start`/`Run` return nil, and a `WARN` names the provider/model/error.
- **AS-3 (F6 — no retry on warmup deadline):** *Given* a `warmup:true` model with
  a short `request_timeout` and a backend that never answers within it, *when*
  warmup runs, *then* the backend is hit **exactly once** (deadline-expiry is not
  retried), the warmup logs a WARN, and boot is unaffected.
- **AS-4 (toggle off = no change):** *Given* no model marked `warmup` (or
  `warmup:false`), *when* the app runs, *then* no warmup request is sent and no
  warmup log line appears — behaviour identical to today.
- **AS-5 (cloud warmup rejected):** *Given* `warmup:true` on a `locality:"cloud"`
  model, *when* the config is loaded, *then* `config.Load`/`Validate` fails with
  `ErrInvalidConfig` naming the field — before any spend.
- **AS-6 (Shutdown cancels in-flight warmup):** *Given* a warmup still loading,
  *when* Shutdown is called, *then* the warmup ctx is cancelled, the goroutine
  returns, and Shutdown completes without leaking a goroutine (`warmupDone`
  observed closed).
- **AS-7 (dedup):** *Given* the same local model marked `warmup` in two brains,
  *when* the app runs, *then* the backend is warmed once (deduped by
  provider/baseURL/modelID), logged.
- **AS-8 (benign-503 guard — co-pilot requirement):** *Given* a `warmup:true`
  model whose backend answers 503 once then 200, with `max_retries:1`, *when*
  warmup runs, *then* the backend is warmed with **exactly 2 hits** — the fast
  refusal IS retried (emergent, correct decorator behaviour, pinned on purpose to
  document that warmup inherits FR-R4, NOT just F6's no-retry).

## Success Criteria

- FR-C*, FR-M*, FR-B*, FR-E*, FR-O* satisfied; AS-1…AS-7 pass as table/integration
  tests via `httptest` (no real network / Ollama), `-race`.
- `internal/config` (the new field + validation) and `internal/app` (collection +
  warmup runner) do not regress; `internal/app` coverage stays ≥ its bar;
  `internal/` ≥ 85%.
- `make quality` green `-race` over the whole suite.
- **Zero behaviour change with the toggle absent/false** (a boot with no
  `warmup` model sends no warmup request and logs nothing new — pinned by AS-4).
- Best-effort proven: a failing warmup never fails boot (AS-2), and a warmup
  deadline is never retried (AS-3, F6).

## Explicitly OUT of scope (named)

- **Retry/warmup metrics** (`korvun_provider_retries_total`, any
  `warmup_*` counter) and the **F8 latency-is-total** shift — **sub-phase 7**.
- **Differentiated "starting" vs "down" fallback text** — **sub-phase 7**.
- A max-tokens/`num_predict` knob on the warmup request — the canonical
  `model.Request` has none; `"hi"` suffices to force the load. Not added here.
- A global warmup on/off switch — per-model `warmup` is enough (YAGNI).

## Review Checklist (green before writing tests)

- [ ] FR-C1 field shape (`ModelConfig.Warmup`) confirmed.
- [ ] FR-C2 cloud-warmup **fail-loud** (vs warn+ignore) confirmed.
- [ ] FR-M2 through the **decorated** model (F6 for free) confirmed vs raw adapter.
- [ ] FR-B1 **background after Start** (vs blocking before channels) confirmed.
- [ ] FR-B2 parallel + FR-B3 Shutdown-safe (`warmupDone`) confirmed.
- [ ] FR-M4 dedup key (provider/baseURL/modelID) confirmed.
- [ ] Best-effort (FR-E1) + no-metrics (FR-E2) + logging (FR-O*) agreed.
- [ ] Out-of-scope list agreed (metrics/fallback → sub-phase 7).
- [ ] No `[NEEDS CLARIFICATION]` left unresolved.

---

**STOP:** paused for co-pilot review. No tests, no implementation, no commit
until FR-C2 / FR-M2 / FR-B1 are confirmed and the checklist is green.
