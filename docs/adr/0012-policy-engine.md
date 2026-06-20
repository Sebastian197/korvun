# ADR-0012: Policy engine — `Policy` interface returning a rich `Decision`, post-dispatch reducer first, two-phase model as the frame

> **Status:** accepted
> **Date:** 2026-06-20
> **Deciders:** Sebastián Moreno Saavedra
> **Corrects:** [ADR-0011](0011-model-fanout.md) §"Open follow-ups" — that ADR
> hypothesised the policy layer as `model.Model` decorators over
> `fanout.Coordinator` (`firstok.Coordinator`, `majority.Coordinator`,
> `consensus.Coordinator`). This ADR consciously rejects that as the central
> shape and explains why (§1). The fan-out mechanism itself (ADR-0011 §1–§7) is
> read and reused unchanged.
> **Builds on:** [ADR-0009](0009-model-interface-and-ollama.md) (`model.Model`,
> `model.Response`), [ADR-0011](0011-model-fanout.md) (`fanout.Result`,
> `fanout.Outcome`).

## Context

Stage 4 shipped the **mechanism**: `fanout.Coordinator.Run(ctx, req, []model.Model)`
dispatches N providers in parallel (wait-all, deterministic order) and returns
`*fanout.Result{ Outcomes []Outcome }`, each `Outcome{ Provider, Response, Err,
Latency }` with the sentinel grammar (`errors.Is` / `errors.As`) preserved
end-to-end. ADR-0011 drew a hard line and re-stated it in three places:
the mechanism chooses **nothing**. Stages 5–6 own *what to do with the outcomes*
according to privacy, cost, and consensus. That is the product's differentiator,
and this ADR opens it.

This ADR was framed by `/office-hours` and stress-tested by `/plan-eng-review`
before any code. The eng-review pushed back hard on the prior hypothesis and on
the wedge's framing; the decisions below absorb that pushback rather than
restating the comfortable version.

### External-docs verification (per CLAUDE.md non-negotiable)

Nothing here touches an external library, SDK, or API. The policy engine is pure
internal domain over the standard library (`context`, `errors`, `time`) plus the
in-repo `internal/model` and `internal/model/fanout` packages. Context7
verification does not apply; there is no external signature to get wrong. No new
Go module dependency — `go.mod` stays at one direct dependency, holding the line
from ADR-0011.

### The single line that frames the whole ADR

**A policy CONSUMES fan-out outcomes and produces a Decision. It is not a new
dispatch mechanism, and it is not a `model.Model`.** The mechanism/policy
boundary ADR-0011 drew is load-bearing and stays intact: `internal/model/fanout`
does not learn about policy, and `internal/policy` does not flex the fan-out. The
fan-out layer must not bend to accommodate policy (HANDOFF "Hard constraints
carried forward").

### What is already on master that this design must mesh with

- **`internal/model`** — the `Model` interface, `Request`/`Response`, the role
  types, and the seven sentinel errors. `model.Response` is `{ Message, Provider,
  ModelName }`: one assistant turn plus attribution. It carries no field for
  "how many providers agreed", "which dissented", or "what this cost". That fact
  is decisive below (§1).
- **`internal/model/fanout`** — `Run` returns `*Result{ Outcomes []Outcome }` in
  input order; `Outcomes[i]` is the result for `models[i]`. Per-model failure is
  `Outcome.Err`, never a `Run`-level error. `Outcome.Latency` is captured for
  free. The order is deterministic and that is a documented guarantee, not an
  accident — this ADR reuses it as the tie-break rule (§5).
- **`internal/brain`** — the Stage 7 forward slice: `Brain.Handle(ctx, env)
  ([]*envelope.Envelope, error)`. The real Brain ships in Stage 7. It is the
  layer that will own *which models to call, run the fan-out, apply a policy, and
  fold the chosen response back into an Envelope*. For Stage 5 there is no real
  Brain yet; the policy is exercised directly over a `fanout.Result` in tests and
  a live skeleton.
- **`internal/envelope`** — `Envelope{ ID, Channel, Direction, Sender, Parts,
  Timestamp, Meta map[string]string, ... }`. There is **no sensitivity / privacy
  classification field**. The only operator-attachable side channel is the free
  `Meta` map. This is why pre-dispatch privacy routing is out of this cut (§3).

## Decision

### 1. The central type is a `Policy` interface returning a rich `Decision` — NOT a `model.Model` decorator

```go
package policy

// Policy reduces the outcomes of a completed fan-out into a single Decision.
// It is the post-dispatch half of the two-phase model (§3): it runs AFTER
// fanout.Run has returned every Outcome, and decides what to do with them
// (select one, vote, merge). It never dispatches and never becomes a Model.
type Policy interface {
    Apply(ctx context.Context, result *fanout.Result) (*Decision, error)
}
```

**Why not `model.Model` (the ADR-0011 hypothesis).** ADR-0011 §"Open follow-ups"
proposed policy-layer wrappers each "implementing `model.Model` over a
`fanout.Coordinator`." That looked clean — policies would compose by nesting and
the Brain would only ever hold "a Model." It is a trap as the **central** type,
for one concrete reason: `model.Model.Generate` returns `(*model.Response,
error)`, and `model.Response` is `{ Message, Provider, ModelName }`. That single
attributed turn is **lossy for exactly the reasoning the policy engine exists to
do**:

- A consensus policy that finds 3-of-5 agreement with 2 dissenters has nowhere to
  put "3/5 agreed, here is the dissent." It must throw the dissent away.
- The provenance of a decision — which Outcomes contributed, which failed, which
  was chosen and why — has no home. The no-code builder (Stage 14) needs that
  provenance to let an operator *debug* a policy; it is an observability
  requirement, not a nice-to-have (§6).
- Per-provider accounting (latency now, cost later) has no field.

The only way to force this into `model.Model` is to widen `model.Response` with
policy concepts (vote counts, provenance, accounting) — which drags policy
vocabulary down into the model layer and **violates the very mechanism/policy
boundary ADR-0011 protects**. That is the opposite of the discipline we are
trying to keep. So the correction is deliberate: the spine is `Policy` returning
`Decision`; `model.Model` is the wrong altitude for it.

**The `model.Model` adapter survives as an opt-in convenience, not the spine.**
ADR-0011's instinct was not worthless — sometimes a caller genuinely wants
"fan-out-plus-policy that looks like a Model." That is provided by a thin,
clearly-lossy adapter:

```go
// AsModel adapts a Policy over a fixed model set into a model.Model, for
// callers that want fan-out-plus-policy behind the Model interface.
// CONVENIENCE ONLY: Generate collapses the Decision to Decision.Response and
// DISCARDS Provenance and Accounting. That loss is precisely why model.Model is
// not the central policy type (§1). Reach for Policy.Apply when you need the
// full Decision; reach for AsModel only when a Model-shaped seam is required and
// the provenance genuinely does not matter at that seam.
func AsModel(coord *fanout.Coordinator, models []model.Model, p Policy) model.Model
```

The adapter is named, scoped, and documented as lossy. It honours ADR-0011's
follow-up without paying its cost as the default.

**`AsModel` is the SECONDARY path, never the default.** The first-class flow is
`Policy.Apply` over a `*fanout.Result`: the Brain owns the model set, runs the
fan-out, and reads the full `Decision`. `AsModel` exists only for the narrow case
where an external seam demands the `model.Model` shape. A future developer must
not reach for it by habit, and must not treat the model set captured inside the
adapter as a first-class home for that set — it is **hidden state** the moment it
is, and the visible owner of the model set is the caller, not the adapter. This
is the same discipline ADR-0011 applied to `model.Response.Provider`: the
self-declared provider label is *available* but is not the *reliable* identity
(the fan-out routes by `Model.Name()` instead). `AsModel` is available; it is not
the reliable path. When in doubt, use `Policy.Apply`.

> **Deferred (reconciliation note, added 2026-06-20).** The `AsModel` adapter
> described here was NOT implemented in the first cut (ADR-0012, `PriorityReducer`)
> nor with the consensus reducer (ADR-0013, `ConsensusReducer`). It is deferred to
> **Stage 7 (Brain)**, its natural consumer: a lossy secondary adapter with no
> consumer cannot be validated well before one exists. The first cut was scoped to
> `Policy` + `Decision` + `PriorityReducer`; the first-class path remains
> `Policy.Apply` over a `*fanout.Result`, and `AsModel` is the secondary lossy
> convenience, not the primary path. The decisions above are unchanged — this note
> only reconciles the ADR with the code on master.

### 2. `Decision` is defined rich on day one; the first reducer fills a subset

```go
// Decision is what a Policy produces from a fan-out Result. It is the rich type
// the Brain (Stage 7) consumes: the chosen reply plus enough provenance and
// accounting to log, debug, and (later) reason about the choice.
//
// The SHAPE is designed to accommodate the full two-phase engine. The FIRST cut
// (PriorityReducer, §4) fills only what selection needs. Fields are NOT invented
// for cases that do not exist yet: there is deliberately no "consensus score"
// and no "confidence" field until a consensus reducer needs one and lands it
// additively. Principle: make the change easy (define Decision now), then make
// the easy change (the first reducer fills one path). The first cut is a strict
// SUBSET of the final engine, not a prototype to throw away.
type Decision struct {
    // Response is the chosen / synthesised assistant reply the Brain will fold
    // into an outbound Envelope. nil IFF no usable outcome existed — see the
    // all-failed path (§5) and ErrNoUsableOutcome.
    Response *model.Response

    // Provenance records which Outcomes the policy considered and how each was
    // treated. It is the debugging surface for the no-code builder and the seed
    // the consensus reducer will extend (additively) later.
    Provenance Provenance

    // Accounting is the per-provider cost/latency record. In v1 it carries
    // Latency only (free from Outcome.Latency); a monetary Cost field is added
    // when a cost model exists (§5 — deferred, no source for it today).
    Accounting []ProviderCost
}

// Provenance answers "which Outcomes contributed, and how?" Considered lists
// every Outcome the policy looked at, in fanout order (deterministic).
type Provenance struct {
    Considered []Contribution
}

// Contribution is one Outcome's role in the Decision. Used marks the outcome(s)
// that fed Decision.Response (exactly one for selection; possibly several when a
// future merge/consensus reducer synthesises a reply). Err is non-nil when that
// outcome failed in the fan-out, carrying the upstream sentinel grammar
// untouched (errors.Is / errors.As still work through it).
//
// There is intentionally no vote / agreement / score field here yet. A consensus
// reducer adds one additively when it ships; inventing it now would be a field
// for a case that does not exist.
type Contribution struct {
    Provider string
    Used     bool
    Err      error
}

// ProviderCost is the per-provider accounting row. Latency is copied from
// Outcome.Latency. A monetary Cost field is deferred until a cost model exists
// (§5); adding it later is additive and breaks nothing.
type ProviderCost struct {
    Provider string
    Latency  time.Duration
}
```

This is the load-bearing "strict subset" claim made concrete: the throwaway risk
was never the reducer *function*, it was the *return type*. If the first cut
returned a bare `*model.Response`, it would be thrown away the moment consensus
needs provenance. By defining `Decision` rich now and filling only the selection
path, the first reducer is a true subset.

### 3. The two-phase model is the conceptual frame; the first cut implements ONLY the post-dispatch reducer

The engine is conceptually two phases. They run at different times and change for
different reasons, so they are **two separate interfaces, not two methods of
one**:

```
                Envelope
                   │
                   ▼
        ┌────────────────────────┐  PRE-DISPATCH  (DEFERRED — §3)
        │  Selector  (deferred)  │  privacy + cost live here: they decide
        │  env → []model.Model   │  WHICH models enter the fan-out.
        └───────────┬────────────┘
                    │ []model.Model
                    ▼
        ┌────────────────────────┐  MECHANISM  (ADR-0011, unchanged)
        │  fanout.Coordinator    │  wait-all, deterministic order.
        │  Run → *fanout.Result  │
        └───────────┬────────────┘
                    │ *fanout.Result ([]Outcome, input order)
                    ▼
        ┌────────────────────────┐  POST-DISPATCH  (THIS ADR — first cut)
        │  Policy  (reducer)     │  consensus / first-ok / quality-pick live
        │  Apply → *Decision     │  here: they decide WHAT TO DO with []Outcome.
        └───────────┬────────────┘
                    │ *Decision {Response, Provenance, Accounting}
                    ▼
                 Brain (Stage 7)   folds Response into an outbound Envelope;
                                   logs Provenance + Accounting (slog).
```

**Why the split is real, not cosmetic.**
- **Privacy** and **cost** are *pre-dispatch routing constraints*: they decide
  which models go into the `[]model.Model` slice **before** `Run` is called
  (personal data → local-only set; free-tier-first → cheap set). They change when
  data-governance rules or pricing change.
- **Consensus**, **first-ok**, **quality-pick** are *post-dispatch reducers*:
  they decide what to do with the `[]Outcome` **after** `Run` returns. They
  change when answer-quality requirements change.

Fusing selection and reduction into one interface would couple two things that
vary independently. They stay separate. The deferred pre-dispatch half is sketched
but not built:

```go
// Selector (DEFERRED — not in this cut) is the pre-dispatch half: it chooses
// which models enter the fan-out for a given Envelope. Privacy and cost routing
// live here. It is intentionally a SEPARATE interface from Policy.
//
//   type Selector interface {
//       Select(ctx context.Context, env *envelope.Envelope) ([]model.Model, error)
//   }
```

**Why only the reducer ships first.** The reducer consumes the `fanout.Result`
that already exists end-to-end — zero new machinery. The selector requires
**sensitivity introspection of the Envelope that is not modelled today**: the
Envelope has only `Meta map[string]string`, no sensitivity field. Building the
selector means first deciding how sensitivity is declared and represented, which
is its own design (§5 — deferred with constraints). The reducer is the honest
smallest cut. For the first cut the Selector box is bypassed: the Brain (or the
Stage 5 live skeleton) hands the model set to `fanout.Run` directly, exactly as
the Stage 4 demos do today.

### 4. The minimal wedge is a SELECTION demo, NOT cost-saving — stated plainly

The first concrete `Policy`:

```go
// PriorityReducer selects the highest-priority SUCCESSFUL Outcome by
// operator-declared provider order. Order holds provider names (matching
// Outcome.Provider == Model.Name()), highest priority first.
type PriorityReducer struct {
    Order []string
}
```

`Apply` walks the Outcomes, picks the successful one whose Provider ranks highest
in `Order`, and returns a `Decision` whose `Response` is that Outcome's
`Response`, whose `Provenance.Considered` lists every Outcome (the chosen one
`Used: true`, the rest with their `Err`), and whose `Accounting` carries every
provider's `Latency`.

**This is a demonstration of SELECTION, and it does NOT save money. Say it out
loud.** The fan-out is wait-all: by the time `Apply` runs, **every model has
already been called and every call has already been paid for.** "Highest-priority
successful Outcome" over a complete `Result` is selection, full stop. Calling it
"first-ok" invites the operator to think it is a cost-saving fail-over. It is not.

The cost-saving fail-over the operator actually wants — *do not call Groq if the
local Ollama already answered* — requires **sequential / conditional dispatch**:
call provider A, inspect, and only call provider B if needed. The wait-all
fan-out structurally cannot express that. It needs a different mechanism: a
**sequential coordinator, a sibling of `fanout.Coordinator`**, which deserves its
own future ADR (it has its own cancellation, ordering, and partial-result
questions). Conflating the two would mean a "cost policy" that never saves a cent
because the money is already spent before the policy runs.

Therefore, **explicitly out of scope for Stage 5**, each with its reason:
- **Cost-saving fail-over** — needs a sequential dispatch mechanism, not a policy
  over a wait-all `Result`. Future ADR (sequential coordinator).
- **Stateful cost (per-Brain daily budget)** — see §5; needs persistence the
  project has not committed to.

### 5. Risks faced head-on (decided here, not parked as "open questions")

**(a) The all-failed path is specified, not left undefined.** When every Outcome
failed (`Outcome.Err != nil` for all), `Apply` returns a **non-nil** `Decision`
(so provenance and accounting are still available for logging) whose `Response`
is `nil`, whose `Provenance.Considered` lists every failed Contribution with its
`Err` preserved, together with a sentinel error:

```go
// ErrNoUsableOutcome is returned by Policy.Apply (alongside a non-nil Decision
// with Response == nil) when no Outcome was usable — typically every provider
// failed. The per-provider causes live in Decision.Provenance.Considered[i].Err
// and are joined into the returned error via errors.Join, so errors.Is against
// any upstream sentinel (e.g. model.ErrAuthInvalid) still works.
var ErrNoUsableOutcome = errors.New("policy: no usable outcome")
```

The Brain's contract (Stage 7): on `errors.Is(err, ErrNoUsableOutcome)`, emit an
operator-configured fallback reply to the user rather than crashing or going
silent. This is the 3am-tired-human path — it is defined now so it is not
discovered in production. (`errors.Join` is available on Go 1.26.)

**(b) Tie-break is deterministic, reusing the fan-out order.** When two Outcomes
tie on priority — same rank in `Order`, or both absent from `Order` — the one
with the **lower fan-out index wins** (`Outcomes[i]` for the smaller `i`).
Because `fanout.Result.Outcomes` is in input order (a documented ADR-0011
guarantee), selection is reproducible: the same inputs always yield the same
choice. No clock, no map iteration, no provider-self-reported field enters the
tie-break.

**(c) Cost is stateless in v1.** The first cost-aware policy, when it lands, is a
**static operator-declared preference ordering** (the same shape as
`PriorityReducer.Order`), never a budget with a running counter. A per-Brain
daily budget needs a **persistent counter** — which drags in a persistence /
durability decision the project has not made (what survives a restart? per-process
or shared across replicas?). Korvun has no storage layer in any stage shipped so
far. **Budget therefore requires a persistence ADR first** and is out of Stage 5.

**(d) Consensus reduces over structured output, not free prose.** When a
consensus reducer ships, it operates over a **normalisable label / classification**
(e.g. a category, a yes/no, a small enum), not free-form LLM prose. Exact-match
agreement over free text almost never fires (two models phrase the same answer
differently), so a "consensus demo" over prose would be a feature that cannot
trigger. Deferred **with this constraint recorded** so the future cut does not
over-promise. Semantic-equivalence consensus (an embedding or judge model) is a
separate, heavier step and an innovation token we are not spending yet.

**(e) Privacy is declared, not inferred, in v1.** Sensitivity is **declared** —
per-Brain configuration, or via `Envelope.Meta` — never inferred by a "is this
private?" classifier. Inferring it means sending the content to a model to ask
whether it is private, which is a recursive cost/privacy trap (you leak the very
content you were trying to protect, and pay for an extra call). Deferred **with
this constraint recorded**. Adding a typed sensitivity field to the Envelope, if
declaration-via-`Meta` proves too weak, is its own additive change.

### 6. Where it lives: `internal/policy`

```
internal/
  model/
    fanout/         mechanism (ADR-0011) — does NOT import policy
  policy/           THIS ADR — imports model + model/fanout
    policy.go       Policy interface, Decision/Provenance/Contribution/ProviderCost
    errors.go       ErrNoUsableOutcome
    priority.go     PriorityReducer (the first concrete Policy)
    model.go        AsModel convenience adapter (lossy, opt-in) — DEFERRED, see note
  brain/            Stage 7 consumer — will import policy
```

> **Deferred (reconciliation note, added 2026-06-20).** `model.go` / `AsModel` is
> NOT on master. It is deferred to **Stage 7 (Brain)**, its natural consumer (see
> the §1 note). What actually shipped in `internal/policy` is `policy.go`,
> `errors.go`, `priority.go` (ADR-0012), and `consensus.go` (ADR-0013, adding
> `ErrNoConsensus` and a shared `rankByOrder` helper). The layout above is the
> intended end state, not the first cut.

Dependency direction (one way, boundary intact):

```
  brain  ─imports→  policy  ─imports→  model/fanout  ─imports→  model
                       └────────────────imports──────────────────┘
   (fanout NEVER imports policy — the mechanism stays ignorant of policy)
```

`internal/policy` imports `internal/model` (for `Response`) and
`internal/model/fanout` (for `Result` / `Outcome`). It is consumed by
`internal/brain` (Stage 7). The fan-out package never learns about policy, so the
ADR-0011 boundary holds by construction (an import cycle would be a compile
error, which is the cheapest possible enforcement).

**Provenance is an observability requirement, not decoration.** The no-code
builder (Stage 14) must let an operator answer "why did this policy pick that
provider?" without reading Go. `Decision.Provenance` is the data that answers it,
surfaced through structured logging (`slog`) at the Brain and, later, the
builder's debug view. It is load-bearing for the product, which is the second
reason (after consensus) that `model.Model` is the wrong central type.

## Consequences

### What this enables
- A real end-to-end value loop in Stage 5 with the smallest honest cut: many
  Outcomes in → one operator-chosen Response out, with full provenance and
  latency accounting, over the fan-out that already exists.
- A `Decision` type that the consensus reducer, the quality-pick reducer, and the
  pre-dispatch selector all extend **additively** — no return-type rewrite.
- A clean test surface: `PriorityReducer.Apply` is a pure function of
  `*fanout.Result`, table-testable with hand-built Outcomes, no network, no
  models. The all-failed path and tie-break are unit-testable directly.

### What this asks / costs
- The Brain (Stage 7) must handle `ErrNoUsableOutcome` explicitly. That is a
  defined contract, not an afterthought.
- Callers wanting a `model.Model` seam use the lossy `AsModel` adapter knowingly;
  the full `Decision` is only available via `Policy.Apply`.
- The pre-dispatch selector, consensus, quality-pick, cost, and privacy all stay
  unbuilt in Stage 5. This ADR commits to their *shape-compatibility*, not their
  delivery.

### Trade-offs accepted
- **Wait-all cost is paid before any policy runs.** Selection cannot save money;
  cost-saving is a different mechanism (sequential coordinator, future ADR). We
  accept that the first cost story is "static preference ordering", not budgets.
- **`Decision` carries empty-ish Accounting (latency only) in v1.** The monetary
  field is absent until a cost model exists. Accepted over inventing a `Cost`
  field with no source.
- **Two interfaces (`Policy` now, `Selector` later) instead of one.** Slightly
  more surface than a single "policy" type, bought deliberately to keep
  independently-varying concerns uncoupled.

## Alternatives Considered

### A1 — Policy as a `model.Model` decorator (the ADR-0011 hypothesis)
`firstok.Coordinator` / `majority.Coordinator` etc., each implementing
`model.Model` over `fanout.Coordinator`. **Rejected** as the central type: `Generate`'s
single `*model.Response` return is lossy for provenance and consensus dissent,
and widening `model.Response` to fix that pushes policy vocabulary into the model
layer, breaking the ADR-0011 boundary. Kept only as the opt-in lossy `AsModel`
adapter (§1) — explicitly the secondary path, never the default, so its captured
model set never silently becomes the de-facto home for that set (the first-class
owner is the caller, via `Policy.Apply`).

### A2 — One `Policy` interface spanning select + reduce
A single interface that both picks the model set and reduces the outcomes
(`Apply(ctx, env) (*Decision, error)`, owning the fan-out call internally).
**Rejected**: couples pre-dispatch routing (changes with governance/pricing) to
post-dispatch reduction (changes with quality requirements). They vary
independently; §3 keeps them as separate interfaces and ships only the reducer.

### A3 — Return a bare `*model.Response` from the first reducer, enrich later
Ship the wedge returning `*model.Response`; introduce `Decision` when consensus
needs it. **Rejected**: that is the throwaway prototype the eng-review warned
against — the return type, not the function, is the thing that would be rewritten.
Defining `Decision` now (filling a subset) makes the wedge a strict subset of the
final engine (§2).

### A4 — Model the wedge as cost-saving fail-over
Frame "first-ok in priority order" as a fail-over that avoids the paid provider.
**Rejected as physically impossible over wait-all fan-out**: every provider is
already called and paid before the policy runs. Cost-saving requires sequential
dispatch — a different mechanism, a future ADR (§4).

### A5 — Infer privacy from Envelope content; track budget with a counter now
Build a content classifier for sensitivity and a stateful per-Brain budget in
Stage 5. **Rejected**: the classifier is a recursive cost/privacy trap (§5e); the
budget needs a persistence decision the project has not made (§5c). Both are
deferred with explicit constraints rather than rushed.

## Out of scope for Stage 5 (recorded, not silently dropped)
- Pre-dispatch `Selector` (privacy + cost routing) — needs an Envelope
  sensitivity model.
- Consensus / majority reducer — deferred; constrained to structured output (§5d).
- Quality-pick reducer (compare responses against the prompt) — additive later.
- Cost-saving fail-over — needs a sequential coordinator (sibling of fan-out),
  its own ADR.
- Stateful budgets — need a persistence ADR first.
- Monetary `Cost` field on `ProviderCost` — needs a cost model.
- `RunStream` / streaming policy — follows ADR-0009 §2 / ADR-0011 future work.
- The no-code builder's declarative representation of a policy — Stage 14;
  `Decision`/`Policy` are designed to be expressible declaratively, but the
  builder is not built here.
