# ADR-0016: Sequential coordinator (Stage 6) — cost-saving fail-over over a shared per-call primitive

> **Status:** accepted
> **Date:** 2026-06-20
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0011](0011-model-fanout.md) (`fanout.Coordinator` owns
> the parallel dispatch; `Outcome`/`Result`; the per-call discipline —
> `recover`+`%w`, latency capture, per-model timeout; the mechanism/policy
> boundary; the P1 `%w` fix and the P2 zero-value-clock race lesson),
> [ADR-0012](0012-policy-engine.md) (a `Policy` reducer CONSUMES a
> `*fanout.Result`; the wait-all fan-out has already called and paid every
> provider before any policy runs — §4; cost-saving fail-over deferred to "a
> sequential coordinator, a sibling of fan-out, its own future ADR"),
> [ADR-0015](0015-pre-dispatch-selector.md) (Stage 6 has two halves; the
> Selector is the policy half, shipped; **this is the mechanism half**), and
> [ADR-0009](0009-model-interface-and-ollama.md) (`model.Model`; the upstream
> sentinel grammar).

## Context

Stage 6 closes the dispatch policy engine with two pieces on opposite sides of
the mechanism/policy boundary (ADR-0015 §1). The first, the pre-dispatch
**Selector**, shipped: it decides *which* models enter the fan-out (a Private
Brain excludes cloud providers before they are called). The second is this ADR.

The Selector saves the cost of providers a Brain should never touch. It does
**not** save the cost of the providers it *does* admit: a Public Brain wired with
Ollama + Groq still pays both, because `fanout.Coordinator` is wait-all — it
calls every model in parallel and blocks until all return (ADR-0011, ADR-0012
§4). "Prefer the local/free model, and only pay for the cloud one if the local
one failed" is not a *filter* (which model enters) and not a *reducer* (what to
do with outcomes). It is a different **dispatch shape**: serial, stop at the
first success. That shape is the sequential coordinator.

This is **mechanism, not policy**: a sibling of `fanout.Coordinator` living under
`internal/model/`, touching neither the Envelope nor the policy layer. Per the
Stage 6 framing it therefore needs no `/office-hours` or `/plan-eng-review` — a
normal ADR suffices.

### The single line that frames the whole ADR

> **The cheapest call is the one you never make. A serial dispatcher that stops
> at the first success turns a parallel pay-everyone fan-out into a
> pay-until-one-works fail-over — and it does so by REUSING fan-out's hard-won
> per-call discipline (recover+`%w`, latency, timeout), never re-deriving it.**

### External-docs verification (per CLAUDE.md non-negotiable)

Pure internal Go over the standard library: a serial loop, the existing
`context`/`time` usage, no goroutines. No external library, SDK, or new
dependency is involved, so Context7 verification is not applicable (it covers
third-party code libraries, of which this ADR adds none). `go.mod` stays at its
single direct dependency.

## Decision

### 1. Stage 6's mechanism half — a sibling package returning the SAME Result

A new package `internal/model/sequential` holds a `Coordinator` whose `Run` has
the same signature family as the fan-out, but dispatches serially:

```go
// package sequential
func (c *Coordinator) Run(
    ctx context.Context, req *model.Request, models []model.Model,
) (*fanout.Result, error)
```

It returns **`*fanout.Result`** (and reuses `fanout.Outcome`), so every existing
`Policy` reducer consumes its output unchanged (§5). The two dispatch shapes are
siblings under the mechanism layer; the policy layer cannot tell which one
produced the `Result`.

```
                         []model.Model (filtered by the Selector, ADR-0015)
                                        │
                 ┌──────────────────────┴──────────────────────┐
                 ▼                                              ▼
   ┌──────────────────────────┐                 ┌──────────────────────────────┐
   │ fanout.Coordinator       │  MECHANISM      │ sequential.Coordinator        │
   │ PARALLEL, wait-all       │  (ADR-0011)     │ SERIAL, stop at first success │
   │ N goroutines + WaitGroup │  ── OR ──       │ one goroutine, a loop          │
   │ calls & pays EVERYONE     │  (THIS ADR)     │ calls until one works          │
   └────────────┬─────────────┘                 └───────────────┬──────────────┘
                │                                                │
                └───────────────► *fanout.Result ◄──────────────┘
                                        │  (same type, either way)
                                        ▼
                          Policy.Apply (ADR-0012/0013)  ── unchanged
```

### 2. REUSE, not duplication — the shared per-call primitive (the critical decision)

The dangerous machinery is the **per-call** discipline in `fanout.callOne`
(fanout.go): catching a provider panic and re-wrapping it with `%w` so the
upstream sentinel grammar survives the boundary (the P1 bug — using `%v` there
silently broke `errors.Is`), capturing latency via the clock seam, and applying
the optional per-model timeout. Re-implementing that in the sequential
coordinator would re-create exactly the divergence P1 cost effort to fix.

**This ADR commits to extracting that primitive into ONE shared
implementation**, exported from the `fanout` package (which already owns the
`Outcome`/`Result` vocabulary), and called by BOTH coordinators:

```go
// package fanout — the shared per-call primitive.
//
// CallOne runs Model.Generate for a single model and returns its Outcome,
// applying the recover+%w panic discipline (preserving the upstream sentinel
// grammar — ADR-0011 §3), latency capture via the clock seam, and the optional
// per-model timeout. It contains NO concurrency: the parallel Coordinator wraps
// it in a goroutine + WaitGroup; the sequential coordinator calls it in a loop.
// One implementation so the sentinel grammar is preserved in exactly one place.
func CallOne(
    ctx context.Context, req *model.Request, m model.Model,
    perModelTimeout time.Duration, now func() time.Time,
) Outcome
```

The entry validation is shared too (nil ctx, `model.ValidateRequest`,
`ErrNoModels`, `ErrNilModel`, already-cancelled ctx) so both coordinators reject
the same misconfigurations with the same sentinels. What each Coordinator keeps
private is only its tiny config (per-model timeout, clock) and its orchestration.

| | **fanout.Coordinator** | **sequential.Coordinator** |
|---|---|---|
| Entry validation | shared (nil ctx, `ValidateRequest`, `ErrNoModels`, `ErrNilModel`, ctx.Err) | **same shared code** |
| Per-call primitive | `fanout.CallOne` (recover+`%w`, latency, timeout) | **same `fanout.CallOne`** |
| Sentinel grammar | upstream `model.*` preserved via `%w` | **same, by construction** |
| Orchestration | N goroutines + `WaitGroup`, **all** outcomes | serial loop, **stop at first success** |
| Outcomes returned | one per input model, input order | only the models actually called, call order |

**Panic-message prefix.** `fanout.callOne` currently prefixes a recovered panic
with `"fanout: provider panicked:"`. Once the primitive is shared, that prefix
would falsely read `fanout` on a *sequential* outcome. The prefix is made
dispatch-shape-neutral (e.g. `"model dispatch: provider panicked:"`); the
`%w` wrapping — the load-bearing part `errors.Is`/`errors.As` depend on — is
unchanged. `TestRun_panicWithSentinelPreservesGrammar` is updated to assert the
neutral prefix plus the unchanged `errors.Is`, which is the part that matters.

### 3. The stop predicate — minimal and mechanical (preserves the boundary)

The predicate is hard-coded and mechanical: **`Outcome.Err == nil` is success →
stop; otherwise advance to the next model.** (By the `Outcome` invariant —
exactly one of `Response`/`Err` non-nil, ADR-0011 §3 — `Err == nil` implies a
usable `Response`.)

There is **no injected predicate and no quality hook** in this cut. A predicate
like "stop if the answer scores above X" is a judgement about answer *quality* —
that is **policy**, and putting it in the dispatch mechanism would breach the
mechanism/policy boundary ADR-0011 drew (and that the policy package preserves by
construction: `internal/model/*` never imports `internal/policy`). "Call Groq
only if Ollama failed" is expressed precisely by `Err == nil`, and that minimal
predicate is what delivers the real cost saving the wait-all fan-out cannot.

### 4. What Run returns — only the called models; absence means "skipped"

`Result.Outcomes` holds an `Outcome` for **each model actually called, in call
order**. The three states are represented with no new field on the canonical
`Outcome`:

```
models = [ollama, groq, anthropic]   (call order = input order, §6)

 success on the 1st:   Outcomes = [ollama:ok]
                       groq, anthropic NOT CALLED  → ABSENT from Outcomes
                       (len 1 < len 3 ⇒ the rest were skipped, never paid)

 1st fails, 2nd ok:    Outcomes = [ollama:err, groq:ok]
                       anthropic NOT CALLED → absent

 all fail:             Outcomes = [ollama:err, groq:err, anthropic:err]
                       every model called; len == len(models)
```

- **"called and failed"** = an `Outcome` present in `Outcomes` with `Err != nil`,
  carrying the upstream sentinel untouched.
- **"not called"** (skipped after an earlier success, or after ctx cancellation
  between calls) = **absent** from `Outcomes`. Absence is the honest, zero-cost
  representation; it needs no third state.
- **all-failed** = every model present, every `Err != nil`. Like the fan-out,
  this is **not** a Run-level error: `Run` returns `(*Result, nil)` and a
  downstream reducer turns it into `ErrNoUsableOutcome`, joining the per-provider
  causes behind the sentinel (ADR-0012 §5a). The mechanism reports what happened;
  the policy decides it means "no usable answer."

`Run` returns a non-nil **error only** for mechanism-level configuration bugs —
the same set the fan-out rejects (nil ctx, invalid request, `ErrNoModels`,
`ErrNilModel`, ctx already cancelled at entry), via the shared entry validation
(§2). A ctx cancelled *during* a call surfaces as that outcome's `Err` (ctx
propagates into `Generate` through `CallOne`); a ctx found cancelled *between*
calls stops the loop, leaving the remaining models absent.

### 5. How it fits Brain/policy — the Decision/Outcome types hold unchanged (verified)

Because `Run` returns `*fanout.Result`, `PriorityReducer` and `ConsensusReducer`
consume it with **zero changes** — verified against their code: both iterate
`result.Outcomes`, build one `Contribution`/`ProviderCost` per outcome, and
select. A shorter `Outcomes` slice is not a special case; the reducer simply
considers the outcomes that exist, and `Provenance.Considered` then truthfully
records only the models actually called.

- **Sequential + `PriorityReducer`** is the natural pairing: the loop stops at
  the first success, so there is exactly one successful outcome and the reducer
  selects it. The cost-saving *preference* lives in the **call order** (wire the
  local/cheap model first); the reducer just reads off the single success.
- **Sequential + `ConsensusReducer`** is a configuration mismatch, not a type
  problem: consensus needs several successes to vote, and a stop-at-first
  dispatcher yields one — so it returns `ErrNoConsensus` (a single success is not
  consensus, ADR-0013). Consensus wants the parallel fan-out; that is an operator
  wiring choice, and the **types still fit**.

This is the `Result` contract's fitness test, a third independent confirmation of
the same kind the project has used before: Groq validated the `Model` interface
against a differently-shaped provider, `ConsensusReducer` validated `Decision`
against a differently-shaped reducer, and now a differently-shaped *coordinator*
validates `Result` — and `Result`/`Outcome`/`Decision` all hold unchanged.

### 6. Determinism — call order is the input order

The sequential coordinator calls `models` in input-slice order and stops at the
first success, reusing the deterministic-order discipline the fan-out relies on
for reproducible attribution (ADR-0011). Given deterministic provider behaviour,
the returned `Result` is reproducible.

### 7. Not structural concurrency — single goroutine, direct to master

The sequential coordinator has **no goroutines, no `WaitGroup`, no channels** —
it is a serial loop calling a synchronous primitive. It is therefore **not
structural concurrency** (the same reasoning ADR-0014 used for the Brain and
ADR-0015 for the Selector), so it ships **directly to master with TDD, no feature
branch**.

The one shared-state lesson it *does* reuse is P2 (HANDOFF): the fan-out's
zero-value lazy clock default (`c.now = time.Now`) races under concurrent reuse.
`sequential.New` sets the clock eagerly for the same reason; the zero-value
`Coordinator` is documented as one-shot, and concurrent reuse requires `New`.
All other per-call state is local to `Run`, so a `New`-constructed coordinator is
safe to share across callers — but it is single-goroutine internally regardless.

## Consequences

### What this enables

- **Real cost saving**, which the Selector and the reducers cannot provide: a
  cheap/local-first serial dispatch pays for the cloud provider only on local
  failure. This completes the cost story Stage 6 set out to tell.
- **One implementation of the per-call sentinel discipline** (`fanout.CallOne`),
  so the P1 `%w` contract can never silently diverge between the two dispatch
  shapes.
- **A second dispatch shape with zero policy-layer changes** — `Result`/`Outcome`
  /`Decision` and every reducer are reused as-is.
- **Composability**: Selector → (parallel OR sequential) → reducer are three
  independent wiring choices over the same `Result`.

### What this asks / costs

- A small, careful refactor of `fanout`: extract `CallOne` + the entry
  validation, re-point `Coordinator.Run` at them (behaviour-preserving), and
  update the panic-prefix test. "Make the change easy, then make the change":
  the extraction is a pure refactor landed first, the new coordinator second.
- The `fanout` package gains one exported symbol (`CallOne`) and the new
  `sequential` package depends on `fanout` for the shared types + primitive +
  sentinels (a one-directional, honest dependency).
- Table-driven tests for the sequential coordinator to the mechanism layer's
  bar (first-success-stops-early and the rest are absent; first-fails-second-ok;
  all-failed returns `(*Result, nil)` with the joined causes usable by a reducer;
  per-model timeout; ctx cancelled at entry → error; ctx cancelled between calls
  → loop stops; a panicking provider preserves the sentinel via the shared
  primitive; determinism by input order), `make quality` green under `-race`,
  ≥90% coverage in the new package and `fanout` unchanged.

### Trade-offs accepted

- **Mechanical-only stop predicate (`Err == nil`).** No quality-based early stop
  in v1. Accepted: quality is policy; the minimal predicate delivers the cost
  story without breaching the boundary (§3).
- **Skipped = absent, not a marked state.** Accepted: it keeps the canonical
  `Outcome` invariant intact and forces no reducer to learn a third state (§4,
  A5).
- **Sequential depends on `fanout`.** Accepted over moving the shared types into
  a third package (A3): it keeps `*fanout.Result` valid for policy/brain with no
  changes, at the cost of a one-way sibling dependency.

## Alternatives Considered

### A1 — Re-implement the per-call discipline inside `sequential`
**Rejected (§2).** Re-creates exactly the P1 divergence (a second copy of the
`recover`+`%w`+latency logic that can drift). One shared `CallOne` is the whole
point of "reuse, not duplication."

### A2 — Put the sequential coordinator in the `fanout` package
**Rejected (§1).** A package literally named `fanout` imported as
`fanout.Sequential` reads as a contradiction (a fan-out calls everyone; this
stops at one). A sibling `sequential` package is the honest name; it reuses
fan-out's types and primitive without inheriting its name.

### A3 — Move `Outcome`/`Result` to a shared package, alias them in `fanout`
**Rejected (§2, trade-offs).** A larger refactor (move the canonical types,
add `type Result = …` aliases) for marginal symmetry. Keeping the types in
`fanout` and exporting only the small `CallOne` primitive is the smaller diff
that still removes the duplication. Revisit only if a third dispatch shape ever
makes the asymmetry costly.

### A4 — A configurable / quality-aware stop predicate now
**Rejected (§3).** "Stop if quality > X" is a policy judgement; injecting it into
the mechanism breaches the ADR-0011 boundary. The minimal `Err == nil` predicate
is sufficient for the cost story and keeps the layers clean.

### A5 — Represent skipped models as a third `Outcome` state/field
**Rejected (§4).** Adds a state to the canonical `Outcome` (which every reducer
consumes) for a case only the sequential path produces — an invented field
(against ADR-0012 §2) that would force every reducer to handle "skipped".
Absence from `Outcomes` is honest and zero-cost.

### A6 — Return a Run-level error on all-failed
**Rejected (§4).** Inconsistent with the fan-out, where all-failed is a
successful mechanism with failed outcomes and the policy owns "no usable answer".
The reducers already join the causes behind `ErrNoUsableOutcome`; duplicating
that as a Run-level error would split one decision across two layers.

## Out of scope (recorded, not silently dropped)

- **Quality-aware / configurable stop predicates** — policy, not mechanism (A4).
- **Retry / backoff** on `ErrRateLimited` (with `RetryAfter`) or
  `ErrProviderUnavailable` — a retry *policy*, its own future design; the
  sequential coordinator advances on any error, it does not wait-and-retry.
- **Cost-tier pre-dispatch *selection*** ("only cheap models enter") — additive
  on the Selector's catalog (ADR-0015 §7), a filter, not this dispatch shape.
- **Hybrid shapes** (parallel a subset, then fall back) — not needed for the
  cost story; compose later if a real consumer appears.
- **`cmd/korvun` real bootstrap** wiring channel → router → brain (Stage 11).
