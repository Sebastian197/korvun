# Stage 05 — Policy engine (post-dispatch phase)

> **Status:** closed
> **Started:** 2026-06-20
> **Closed:** 2026-06-20

## Objective

Turn the raw outcomes the mechanism layer (Stage 4 fan-out) returns
into the behaviour the operator configures. Stage 4 deliberately stopped
at "call every provider and surface every `Outcome` in input order, pick
nothing". Stage 5 is the first layer that *chooses*: it reduces a
`*fanout.Result` to a single rich `Decision`.

The policy engine is framed as **two phases**:

- **Post-dispatch** — reducers that run *after* fan-out has called and
  collected every provider. They select / vote over outcomes already in
  hand. **This is the whole of Stage 5.**
- **Pre-dispatch** — a `Selector` that runs *before* fan-out to decide
  *which* providers to call (privacy / cost routing), plus a sequential
  coordinator (a sibling of fan-out) for cost-saving fail-over. **This is
  Stage 6, not Stage 5** — see "Boundary with Stage 6" below.

Stage 5 closes the **post-dispatch phase** with two reducers of
materially different nature validating one unchanged `Decision` contract.

## Phases

| Phase | Description                                          | Status |
|-------|------------------------------------------------------|--------|
| 5.1   | `Policy` interface + `Decision` + `PriorityReducer`  | done   |
| 5.2   | `ConsensusReducer` on the same unchanged contract    | done   |

## Phase 5.1 — Policy interface + priority reducer

### Deliverables

- **`docs/adr/0012-policy-engine.md`** (status `accepted`, commit
  `c4e519b`), pinning the policy-engine protocol. Framed by
  `/office-hours`, stress-tested by `/plan-eng-review` before any code;
  the eng-review pushback is absorbed *into* the ADR, not parked as open
  questions. The four findings that changed the design:
  - **The central type is a `Policy` interface returning a rich
    `Decision`, NOT a `model.Model` decorator.** A conscious correction
    of ADR-0011's "Open follow-ups", which had hypothesised policy-layer
    wrappers implementing `model.Model` over the fan-out. `model.Response`
    is lossy for provenance and consensus dissent; the `model.Model` shape
    survives only as the opt-in lossy `AsModel` adapter — the SECONDARY
    path, never the default, and deferred to Stage 7 (its natural
    consumer, the Brain).
  - **`Decision{Response, Provenance, Accounting}` is defined rich on day
    one**, but the first reducer fills only the selection subset. No
    invented fields (no consensus score / confidence until a consensus
    reducer needs them). The first cut is a strict *subset* of the final
    engine, not a throwaway prototype.
  - **Two-phase model is the frame; only the post-dispatch reducer ships.**
    The pre-dispatch `Selector` (privacy + cost routing) is deferred — it
    needs an Envelope sensitivity model that does not exist yet (only
    `Meta map[string]string` today). → Stage 6.
  - **Selection vs cost-saving.** The wedge is a SELECTION reducer, not a
    cost-saver: wait-all fan-out has already called and paid every
    provider before the reducer runs. Cost-saving fail-over needs a
    sequential coordinator (a sibling of fan-out) — its own future ADR. →
    Stage 6.
- **`internal/policy/`** package:
  - `Policy` interface — `Apply(ctx, *fanout.Result) (*Decision, error)`.
  - `Decision` / `Provenance` / `Contribution` / `ProviderCost` — the
    rich result type, defined in full on day one.
  - Sentinels `ErrNilResult` and `ErrNoUsableOutcome`.
  - `PriorityReducer` — selects the highest-priority successful `Outcome`
    by operator-declared provider `Order`. A pure function over
    `*fanout.Result`. `bestRank` starts at `math.MaxInt` so the rank
    comparison can never collide with a genuine rank 0.

### Behaviour established by Phase 5.1

- The mechanism / policy boundary that ADR-0011 drew is now load-bearing
  *in code*: the reducer reads a `*fanout.Result` and emits a `Decision`;
  the fan-out layer did not flex to accommodate it.
- `PriorityReducer` is a **SELECTION** reducer. ADR-0012 §4–§5 record,
  explicitly, that this is not cost-saving: every provider was already
  called and paid. Stateful budgets need a persistence ADR first; both
  are out of Stage 5 scope.
- The sentinel grammar is preserved end-to-end: an all-failed `Result`
  yields `ErrNoUsableOutcome` joining every upstream cause, so
  `errors.Is` keeps working from the adapter all the way to the policy
  read.

### `/review` pass (Phase 5.1)

`/review` ran on the **code** (not the ADR — the 4.3 lesson), two
independent reviewers (adversarial edge-case + test-coverage).
**Zero correctness bugs** — the design held under all eight edge-case
vectors (empty / duplicate `Order`, both-non-nil and both-nil invariant
violations, all-failed `errors.Join`, mid-slice winner). The inverse of
the 4.3 signal: on pure / simple code `/review` did not invent logic
bugs. It surfaced real test-quality findings, all applied — removed a
no-op `errUnwrap` helper for a positive `errors.Is` check; added table
rows for the both-non-nil poison-skip, the mid-slice winner, and
duplicate `Order`; added a both-nil all-failed test; strengthened the
all-failed accounting assertions (provider + latency, not just length).

## Phase 5.2 — Consensus reducer

### Deliverables

- **`docs/adr/0013-consensus-reducer.md`** (status `accepted`, commit
  `0b1d6b7`), adding `ConsensusReducer` on the **same** `Policy` /
  `Decision` contract. This was the contract's fitness test — a reducer
  of a different nature (several Outcomes *jointly decide by agreeing*) —
  and **`Decision` held unchanged**, exactly as Groq validated the
  `Model` interface against a differently-shaped provider. Multiple
  `Contribution.Used == true` is the case `Contribution`'s godoc already
  anticipated; no field added.
- **`internal/policy/`** (extended, no contract change):
  - `ConsensusReducer{Order, Normalize}` — both fields optional, zero
    value valid. Votes over a **normalized form** of
    `Response.Message.Content` (for structured / label output, never free
    prose; the `Normalize` seam enforces it, default trim + lowercase,
    configurable).
  - **Strict majority of the successful outcomes, plus a floor of two.**
    A 2-2 tie is not a majority → `ErrNoConsensus` (this dissolves the
    group-tie question). A single success is not consensus →
    `ErrNoConsensus`.
  - `ErrNoConsensus` — new bare sentinel for disagreement, distinct from
    `ErrNoUsableOutcome` (all-failed, checked first, joins causes).
  - Shared `rankByOrder` helper — the representative reply reuses
    `PriorityReducer`'s ranking (latency rejected as a tie-break: not
    reproducible). `PriorityReducer`'s `rank` was refactored to
    `rankByOrder` with behaviour proven identical.

### Behaviour established by Phase 5.2

- **The `Decision` contract is validated by two reducers of materially
  different nature** — one *selects* by trust order, one *agrees* by
  majority vote — without adding a single field. This is the
  post-dispatch phase's contract-fitness proof, mirroring the
  two-providers-one-`Model`-interface proof of Stage 4.
- The two reducers **compose**: `ConsensusReducer` → `PriorityReducer`
  reads as "agree if you can, else fall back to the trusted provider".
- `Contribution.Class` was **named but NOT added** — per-minority-voter
  class is recoverable from the paired `fanout.Result`; additive only if
  the builder ever needs the spread from `Decision` alone (ADR-0013 §9).
  Same discipline as the deferred fields in ADR-0012: no field without a
  consumer.

### `/review` pass (Phase 5.2)

`/review` ran again on the code, two independent reviewers: **zero
correctness bugs** — the threshold math was proven to yield a unique
winner (so the early `break` is safe), determinism holds under map
iteration, and the `rank → rankByOrder` refactor is behaviorally
identical. Same inverse-of-4.3 signal. Test-quality findings applied: a
`normalize()` double-call hoisted; tests added for a both-non-nil voter
(must not vote), a both-nil outcome (bare `ErrNoUsableOutcome`), an
empty-string winning class, a minimal 2-of-2 consensus, and `Accounting`
value assertions across all consensus paths.

## Live validation

The post-dispatch reducers are validated at two levels.

- **`cmd/demo-policy`** (disposable, delete in Stage 11) runs **both
  reducers over the same hand-built `Result`** and prints each
  `Decision`. The flagship contrast: on identical data, `PriorityReducer`
  follows the top-priority provider while `ConsensusReducer` follows the
  agreeing majority — and on a 2-2 split, priority still decides while
  consensus returns `no consensus`. First *visible* proof of the
  project's differentiator (fabricated data).
- **Live end-to-end via the Brain (Stage 7).** The Stage 7 `Orchestrator`
  passes a **real** `*fanout.Result` — produced by a real Ollama + Groq
  fan-out — through `PriorityReducer`, and a `PriorityReducer`-over-real-
  fan-out integration test anchors the seam (the prior `Handle` tests used
  a `fakePolicy`, bypassing it). `cmd/demo-brain` exercises the whole path
  against real providers, including the no-answer contract where the
  policy returns `ErrNoUsableOutcome` and the Brain logs the provenance
  and serves a fallback reply. The reducers are therefore validated not
  only on fabricated demo data but on live model-driven dispatch.

## Boundary with Stage 6 (explicit)

**Stage 5 closes the POST-DISPATCH phase of the policy engine.** What
remains of the policy engine is **Stage 6 work, deliberately NOT started
here**, each requiring its own framing and ADR before any code:

- **Pre-dispatch `Selector`** — privacy- and cost-aware routing that
  decides *which* providers to call *before* fan-out runs (personal data
  → local-only providers; cloud only for non-sensitive payloads).
  Deferred by ADR-0012 because it needs an Envelope sensitivity model
  that does not exist yet (only `Meta map[string]string` today).
- **Sequential coordinator** — a sibling of `fanout.Coordinator` that
  calls providers in order and stops at the first acceptable answer, the
  mechanism a *cost-saving* fail-over policy needs (wait-all fan-out has
  already paid everyone, so it cannot save cost). Its own future ADR.
- **Retry policy** — `ErrRateLimited` + `RetryAfter` → wait and re-Run;
  `ErrProviderUnavailable` → backoff; `ErrAuthInvalid` → page the
  operator, never retry. Composes over the sentinel grammar Stage 4 laid
  down.
- **Stateful budgets** — hard per-Brain daily cost ceilings. Needs a
  persistence ADR first.

None of these were touched in Stage 5. The post-dispatch reducers are a
strict subset of the final engine, not a prototype to be thrown away —
the `Decision` they fill is the same `Decision` the Stage 6 pieces will
read.

## Workflow compliance

| Step                         | 5.1 | 5.2 |
|------------------------------|-----|-----|
| Design framed before code    | `/office-hours` + `/plan-eng-review` → ADR-0012 | ADR-0013 on the unchanged contract |
| ADR written before code      | ADR-0012 accepted before any code | ADR-0013 accepted before any code |
| TDD red before green         | red on a stub, then green | red on a stub, then green |
| `make quality` green         | yes, with `-race` | yes, with `-race` |
| `/review` on code, not ADR   | yes — zero correctness bugs | yes — zero correctness bugs |
| Stage doc updated            | this document | this document |

## Quality gate (stage-wide, on master)

| Package                          | Coverage |
|----------------------------------|----------|
| `internal/policy`                | **100.0%** (≥ 90% critical-package target) |
| `internal/brain`                 | 100.0%   |
| `internal/channel`               | 100.0%   |
| `internal/channel/webhook`       | 91.4%    |
| `internal/channel/telegram`      | 90.5%    |
| `internal/envelope`              | 97.0%    |
| `internal/model`                 | 100.0%   |
| `internal/model/ollama`          | 96.0%    |
| `internal/model/groq`            | 94.7%    |
| `internal/model/fanout`          | 100.0%   |
| `internal/router`                | 96.3%    |
| **total**                        | **94.4%** |

`make quality` green with `-race`. `go.mod` still carries a single
direct dependency (`github.com/go-telegram/bot v1.21.0`) — the policy
engine is stdlib-only (`math`, `errors`, `sort`-free deterministic
ranking).

## Key decisions

| ADR | Decision |
|-----|----------|
| ADR-0012 | Policy engine is a `Policy` interface → rich `Decision`, NOT a `model.Model` decorator; `Decision` rich on day one but only the selection subset filled; two-phase frame, only post-dispatch ships; `PriorityReducer` selects by operator trust order; `AsModel` deferred to Stage 7; pre-dispatch `Selector` + cost-saving sequential coordinator deferred to Stage 6. |
| ADR-0013 | `ConsensusReducer` on the SAME unchanged `Decision` contract; votes over a normalized `Content` form; strict majority + floor of two; `ErrNoConsensus` distinct from `ErrNoUsableOutcome`; shared `rankByOrder`; `Contribution.Class` named but not added (no field without a consumer). |

## Notes

- **Two reducers, one contract, zero new fields.** Stage 5's headline
  result: `Decision` survived a second reducer of an entirely different
  nature. The same discipline as Stage 4 (two providers, one `Model`
  interface) now holds at the policy layer.
- **`/review` rule confirmed again.** Both phases ran `/review` on the
  code only, never on the ADR (the durable 4.3 rule). On pure / simple
  reducer code the skill found zero correctness bugs both times and
  delivered its value as test-quality findings — the inverse of the
  concurrency-heavy 4.3 code where it caught two real contract bugs.
- **`cmd/demo-policy` is temporary.** It is deleted in Stage 11 when
  `cmd/korvun` proper boots and the policy layer is exercised through the
  real binary instead of a hand-built `Result`.
- **No pre-dispatch code exists.** This document closes the post-dispatch
  phase only. The pre-dispatch `Selector` and the sequential coordinator
  are Stage 6 — new design work, to be framed and ADR'd deliberately, not
  chained automatically onto this close.