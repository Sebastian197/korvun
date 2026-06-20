# Stage 06 — Policy engine, PRE-dispatch phase (privacy selector + cost-saving fail-over)

> **Status:** closed
> **Started:** 2026-06-20
> **Closed:** 2026-06-20

## Objective

Complete the **pre-dispatch** half of the two-phase policy engine that
ADR-0012 framed and Stage 5 deferred. Stage 5 shipped the **post-dispatch**
reducers (decide *what to do* with the `[]Outcome` a completed fan-out
returns: `PriorityReducer`, `ConsensusReducer`). Stage 6 ships what runs
*before* the dispatch:

```
        Envelope
           │
           ▼
   ┌──────────────────┐   PRE-DISPATCH  ── STAGE 6 ──
   │ Selector         │   privacy: WHICH models enter the fan-out   (6.1, ADR-0015)
   │ (per-Brain)      │
   └────────┬─────────┘
            │ []model.Model (filtered)
            ▼
   ┌──────────────────┐   MECHANISM
   │ fanout OR        │   parallel wait-all  (ADR-0011)
   │ sequential       │   OR serial fail-over  ── STAGE 6 ──        (6.2, ADR-0016)
   └────────┬─────────┘
            │ *fanout.Result
            ▼
   ┌──────────────────┐   POST-DISPATCH  (Stage 5)
   │ Policy (reducer) │   WHAT TO DO with the outcomes
   └──────────────────┘
```

The stage has **two pieces on opposite sides of the mechanism/policy
boundary** (ADR-0011), framed together by `/office-hours` +
`/plan-eng-review` and split into two ADRs precisely so that boundary is not
conflated:

- **6.1 — the privacy Selector (policy):** decides *which* models enter the
  fan-out, declared **per-Brain**, **without touching the canonical
  Envelope** (ADR-0015).
- **6.2 — the sequential coordinator (mechanism):** the cost-saving
  fail-over — call the cheap/local provider first, contact the paid one only
  if it failed — a sibling of `fanout.Coordinator` built over a **shared
  per-call primitive** (ADR-0016).

Neither is structural concurrency, so — like the Stage 5 reducers and the
Stage 7 Brain — both shipped **directly to master with TDD, no feature
branch**.

## Phases

| Phase | Description                                                | Status |
|-------|------------------------------------------------------------|--------|
| 6.1   | Pre-dispatch privacy Selector (per-Brain) — ADR-0015       | done   |
| 6.2   | Sequential coordinator (cost-saving fail-over) — ADR-0016  | done   |

## Phase 6.1 — Pre-dispatch privacy Selector (ADR-0015)

### The framing that reshaped the piece

The Selector was deferred by ADR-0012 §3 with a stated blocker: "it needs
Envelope sensitivity modelling that does not exist." The `/office-hours` +
`/plan-eng-review` framing **inverted that premise**: the minimal cut does
**not** touch the Envelope at all.

- Nothing can correctly *write* per-message sensitivity today, and inferring
  it is forbidden (ADR-0012 §5e, the recursive privacy trap). A typed
  `Envelope.Sensitivity` field would be **write-only with no correct
  writer** — over-engineering on the most important type in the system.
- So sensitivity is **declared per-Brain at construction** ("this Brain is
  private → local-only"), exactly where every other Brain configuration
  lives. The Envelope stays pristine; the typed field is deferred,
  additive, until a real writer (the no-code builder) exists.

### Deliverables

- **`docs/adr/0015-pre-dispatch-selector.md`** (status `accepted`). Load-bearing
  decisions: two ADRs for Stage 6 (this is the policy half); **per-Brain cut,
  Envelope untouched** (blast-radius ranking: A per-Brain *chosen* vs B
  convention-over-`Meta` *rejected* vs C typed field *deferred*); model
  attributes declared in the **wiring catalog**, not in the `model.Model`
  interface; privacy **declared, not inferred**; cost selection named but
  deferred (additive on the same catalog).
- **`internal/policy/selector.go`** — the pre-dispatch Selector in its
  per-Brain form, a **pure function**:
  - `Sensitivity` enum (`Public` / `Private`, zero value intentionally invalid).
  - `Locality` enum (`Local` / `Cloud`).
  - `CatalogEntry{Model, Locality}` — the operator-declared catalog.
  - `SelectModels(catalog, s) ([]model.Model, error)` — Public keeps all;
    Private keeps only `Local` (cloud providers dropped **before** the
    fan-out, never contacted). Order preserved deterministically. No `ctx`
    (no I/O), exactly as `PriorityReducer` ignores the ctx it is handed.
- **`internal/policy/errors.go`** — two new sentinels:
  - `ErrNoEligibleModels` — the filter left no model (e.g. a Private Brain
    wired with only Cloud models, or an empty catalog). **Fails loud at
    construction**, not at the first message. Distinct from
    `fanout.ErrNoModels`.
  - `ErrUnknownSensitivity` — an unrecognised `Sensitivity` (including the
    zero value): fail loud rather than silently default to Public (which
    would leak to cloud) or Private (which would hide the bug). The safe
    default in a privacy system is "don't know → refuse." The offending
    value is wrapped via `%w`.
- **`cmd/demo-selector/main.go`** — the visible proof: the **same payload**
  to a Public Brain (Ollama + Groq both enter) vs a Private Brain (Ollama
  only; **Groq excluded before any call**), plus the misconfiguration guard.

### Behaviour established by Phase 6.1

- **Pre-dispatch privacy selection works end-to-end without touching the
  canonical Envelope.** A Private Brain provably excludes cloud providers
  *before* contacting them — the "Ollama first" of Stage 6 in the privacy
  key.
- **The Orchestrator does not change.** Filtering happens once, at wiring,
  before `NewOrchestrator`; the existing Orchestrator consumes the
  already-filtered fixed set. ADR-0014's `models`/`policy` seam stays the
  hook for the *future* per-message `SelectingBrain` — not consumed here.
- **`model.Model` does not change** — attributes live in the wiring catalog,
  so no adapter is touched.

### `/review` pass (Phase 6.1)

`/review` ran on the code (the fifth invocation; the "review code, not ADRs"
rule still holding). **Zero correctness bugs** — same inverse-of-4.3 signal
as the reducers: on pure code it does not invent logic bugs. Applied
findings: package doc drift fixed (`doc.go` no longer says the Selector is
deferred); test rows added (single-entry Local + Private, nil-catalog +
Private); an assertion that `ErrUnknownSensitivity` surfaces the offending
value; `equalStrings` helper replaced with stdlib `slices.Equal`.

## Phase 6.2 — Sequential coordinator (ADR-0016)

### The piece, and why it is mechanism not policy

The Selector saves the cost of providers a Brain should never touch; it does
**not** save the cost of the ones it admits — a Public Brain with Ollama +
Groq still pays both, because `fanout.Coordinator` is wait-all (ADR-0012 §4).
"Prefer the local/free model, pay for the cloud one only if it failed" is a
different **dispatch shape**: serial, stop at the first success. That is
**mechanism** (a sibling of `fanout.Coordinator`), not policy.

### Deliverables

- **`docs/adr/0016-sequential-coordinator.md`** (status `accepted`).
  Load-bearing decisions: **reuse, not duplication** — extract the per-call
  discipline to ONE shared primitive; the **minimal mechanical stop
  predicate** (`Err == nil` → stop; no quality hook, which would be policy);
  returns the **same `*fanout.Result`** so reducers consume it unchanged;
  "not called" = **absent** from `Outcomes`; all-failed → `(*Result, nil)`;
  not structural concurrency → direct to master.
- **The shared-primitive extraction (two isolated pure-refactor commits,
  fan-out verified behaviorally identical):**
  - `feat(model): extract fanout.CallOne shared per-call primitive` —
    `fanout.CallOne` now holds the `recover`+`%w` panic discipline (P1
    contract: sentinel grammar preserved in **one** place), latency capture,
    and per-model timeout. `Coordinator.Run` is **byte-identical**; only the
    `callOne` method shrank to `*out = CallOne(...)`. Panic prefix
    neutralised to `model dispatch:` so a sequential outcome never falsely
    reads `fanout`.
  - `refactor(model): extract fanout.ValidateRunInputs shared input
    validation` — the entry checks (nil ctx, `ValidateRequest`,
    `ErrNoModels`, `ErrNilModel`, ctx-cancelled) shared by both dispatch
    shapes; nil-ctx message neutralised.
- **`internal/model/sequential/`** — `Coordinator.Run(ctx, req, models)
  (*fanout.Result, error)`: a serial loop calling `fanout.CallOne` in input
  order, stopping at the first `Err == nil`. Reuses
  `fanout.ValidateRunInputs` + `fanout.CallOne`. A ctx cancelled *between*
  calls stops the loop; skipped models are absent. Eager clock via `New`
  (reusing the P2 zero-value-race lesson rather than re-learning it).
- **`cmd/demo-sequential/main.go`** — the visible proof of cost saving: over
  `[ollama, groq]`, scenario 1 (Ollama answers) → **Groq never called**;
  scenario 2 (Ollama fails) → Groq called as fail-over.

### Behaviour established by Phase 6.2

- **Real cost saving**, which neither the Selector nor the reducers provide:
  the paid provider is contacted only on local failure. Demonstrated, and
  asserted at the test level (`TestRun_firstSucceeds_restNotCalled` proves
  `m2.calls == 0` — the skipped model never received `Generate`).
- **`fanout.Result` validated a THIRD time.** A differently-shaped
  coordinator produces the same `Result`; `PriorityReducer` /
  `ConsensusReducer` / `Decision` / `Outcome` all hold unchanged — the same
  kind of fitness test as Groq validating `Model` and `ConsensusReducer`
  validating `Decision`. Verified by an all-failed integration test feeding
  the sequential `Result` into `PriorityReducer` → `ErrNoUsableOutcome` with
  the upstream cause preserved.
- **The fan-out is behaviorally identical post-refactor.** `Run` byte-
  identical; the P1 sentinel-preservation test passes with the shared
  primitive; `-race -count=10` clean; `internal/model/fanout` stays 100%.

### `/review` pass (Phase 6.2)

`/review` ran on the code (the sixth invocation, and — as anticipated — more
valuable than on pure code, because it touched the shared-primitive
extraction over already-validated concurrency). **Zero correctness bugs;
verdict: the refactor preserved fan-out behaviour** (all eight invariants
checked: defer ordering, named-return mutation, `Name()`-panic → Latency 0,
`%w`, prefix consistency, timeout ctx derivation, reducer integration,
zero-value clock claim). One reported "latency changed" finding was a **false
positive** (the reviewer read ADR-0011's illustrative pseudocode, not the
actual code, which already used the latency `defer`; verified via
`git show`). Applied: a test-comment precision fix and a slice-index guard in
the demo.

## Workflow compliance

| Step                         | 6.1 | 6.2 |
|------------------------------|-----|-----|
| Design framed before code    | `/office-hours` + `/plan-eng-review` (joint Stage 6 framing) | covered by the same framing → normal ADR (pure mechanism) |
| ADR written before code      | ADR-0015 accepted before code | ADR-0016 accepted before code |
| TDD red before green         | red on missing symbols, then green; on master (no branch) | red, then green; refactor isolated + verified first |
| `make quality` green         | yes, with `-race` | yes, with `-race -count=10` on the refactor |
| `/review` on code, not ADR   | yes — zero correctness bugs | yes — zero correctness bugs, parity confirmed |
| Stage doc updated            | this document | this document |

## Quality gate (stage-wide, on master)

| Package                          | Coverage |
|----------------------------------|----------|
| `internal/policy`                | **100.0%** (≥ 90% critical-package target) |
| `internal/model/fanout`          | **100.0%** |
| `internal/model/sequential`      | **100.0%** |
| `internal/brain`                 | 100.0%   |
| `internal/channel`               | 100.0%   |
| `internal/channel/webhook`       | 91.4%    |
| `internal/channel/telegram`      | 90.5%    |
| `internal/envelope`              | 97.0%    |
| `internal/model`                 | 100.0%   |
| `internal/model/ollama`          | 96.0%    |
| `internal/model/groq`            | 94.7%    |
| `internal/router`                | 96.3%    |
| **total**                        | **94.5%** |

`make quality` green with `-race`. `go.mod` still carries a single direct
dependency (`github.com/go-telegram/bot v1.21.0`) — both Stage 6 pieces are
stdlib-only.

## Key decisions

| ADR | Decision |
|-----|----------|
| ADR-0015 | Pre-dispatch privacy Selector as a **per-Brain pure function** (`SelectModels` over a wiring catalog), **NOT** an Envelope change — nothing can write per-message sensitivity and inferring it is forbidden, so the typed field is deferred (additive) until the no-code builder exists. Model attributes live in the wiring catalog, not `model.Model`. `ErrNoEligibleModels` / `ErrUnknownSensitivity` fail loud at construction. Privacy declared, never inferred. |
| ADR-0016 | Sequential coordinator as a **mechanism sibling of fan-out** that **reuses, not duplicates**, the per-call discipline (`fanout.CallOne` + `fanout.ValidateRunInputs` extracted as shared primitives). **Minimal mechanical stop predicate** (`Err == nil`), no quality hook (that would breach the mechanism/policy boundary). Returns the same `*fanout.Result`; "not called" = absent; all-failed → `(*Result, nil)` for a reducer to turn into `ErrNoUsableOutcome`. Not structural concurrency → direct to master. |

## What Stage 6 completes — the differentiator is whole

With Stage 6 closed, **the entire policy-engine block plus its orchestration
is done and demonstrable**:

- **Post-dispatch** (Stage 5): `PriorityReducer`, `ConsensusReducer` over a
  completed `fanout.Result`.
- **Pre-dispatch** (Stage 6.1): the per-Brain privacy `Selector`.
- **Cost-saving dispatch** (Stage 6.2): the sequential fail-over.
- **Orchestration** (Stage 7): the Brain wiring Envelope → (selector →)
  fan-out/sequential → policy → Envelope.

Korvun's differentiator — *privacy/cost/consensus-aware multi-model dispatch*
— now exists end-to-end in code and is shown by four disposable demos
(`demo-policy`, `demo-brain`, `demo-selector`, `demo-sequential`). What
remains is not more of the engine; it is **operability**: wiring it into a
real binary, persistence, observability, and the no-code builder.

## What is NOT yet wired (downstream)

- **The single-binary assembly** — channel → router → brain → channel inside
  a real `cmd/korvun` `main.go` — is **Stage 11**, the last step for V1
  checklist criterion 1 (a real message in/out through a real binary, not a
  demo). See `docs/ROADMAP-V1.md`.
- **Per-message sensitivity** (the typed `Envelope.Sensitivity` field + the
  env-taking `Selector` interface) — deferred, additive, needs a writer
  (ADR-0015 A1/A4).
- **Cost-tier selection** (additive on the catalog) and **retry/backoff
  policy** — deferred (ADR-0015 §7, ADR-0016 out-of-scope).

## Notes

- **The four demos are temporary.** `demo-selector` and `demo-sequential`
  join `demo-policy` and `demo-brain` — all deleted in Stage 11 when
  `cmd/korvun` boots and the engine is exercised through the real binary.
- **The mechanism/policy boundary held through both pieces.** The Selector is
  policy (lives in `internal/policy`); the sequential coordinator is
  mechanism (lives in `internal/model/sequential`, never imports
  `internal/policy`). Fusing them into one ADR would have conflated the
  boundary that keeps the layering legible.
- **Stage 6 closes the policy-engine pre-dispatch phase, completing the
  engine block (Stages 5+6) and its orchestration (Stage 7).** With this
  document, Stages 0–7 are all closed, each with its own STAGE doc; there are
  zero half-open stages. The next big step is **undecided** and chosen by
  operator + copilot — likely Stage 11 (the real `cmd/korvun` assembly),
  Stage 8 (agents), or Stage 9 (persistence).
