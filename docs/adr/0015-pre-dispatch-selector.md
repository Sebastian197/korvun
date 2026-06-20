# ADR-0015: Pre-dispatch Selector (Stage 6) — per-Brain privacy filtering, the Envelope untouched

> **Status:** accepted
> **Date:** 2026-06-20
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0011](0011-model-fanout.md) (the mechanism/policy
> boundary; `fanout.Coordinator` owns dispatch and the post-Wait fence),
> [ADR-0012](0012-policy-engine.md) (the two-phase policy model; the
> pre-dispatch `Selector` SKETCHED and deferred in §3; **privacy is DECLARED,
> not inferred** in §5e), [ADR-0014](0014-brain-orchestrator.md) (the
> `Orchestrator`; `models`/`policy` typed as interfaces — "the seam"), and
> [ADR-0009](0009-model-interface-and-ollama.md) (`model.Model` is
> `Generate` + `Name()`, nothing else).

## Context

Korvun's differentiator is the configurable dispatch policy engine: privacy,
cost, and consensus routing. ADR-0012 split that engine into two phases and
shipped only the **post-dispatch** half (reducers over a completed
`fanout.Result`: `PriorityReducer`, `ConsensusReducer`). The **pre-dispatch**
half — a `Selector` that decides *which* models enter the fan-out *before* it
runs — was deferred with a stated reason (ADR-0012 §3):

> The selector requires **sensitivity introspection of the Envelope that is not
> modelled today**: the Envelope has only `Meta map[string]string`, no
> sensitivity field. Building the selector means first deciding how sensitivity
> is declared and represented, which is its own design.

This ADR is Stage 6's first half: the **Selector**. It pins how sensitivity is
declared and represented — and the answer, after a `/office-hours` +
`/plan-eng-review` framing, is the opposite of what ADR-0012 anticipated: the
first cut declares sensitivity **per-Brain at construction** and **does not
touch the canonical Envelope at all**.

The second half of Stage 6 — a cost-saving **sequential coordinator** (call
Groq only if Ollama failed) — is a separate concern and gets its own future
ADR (see §1).

### The single line that frames the whole ADR

> **Privacy-aware selection does not need a change to the canonical Envelope.
> It needs a declaration at the one place that already declares everything else
> about a Brain — the wiring. The smallest honest cut filters a model catalog
> by a per-Brain sensitivity level, once, at construction; the Orchestrator
> never learns it happened.**

### External-docs verification (per CLAUDE.md non-negotiable)

This change is pure internal Go domain modelling over the standard library: a
small enum, a catalog struct, a pure filter function, and a sentinel error. **No
external library, SDK, framework, or new dependency is involved**, so Context7
verification is not applicable here (it covers third-party code libraries, of
which this ADR adds none). `go.mod` stays at its single direct dependency.

## Decision

### 1. Two ADRs for Stage 6 — this one is the Selector (policy); the coordinator is mechanism

Stage 6 has two pieces that live on **opposite sides of the load-bearing
mechanism/policy boundary** drawn by ADR-0011. They change for different reasons
and at different layers, so they get separate ADRs.

```
                       Envelope
                          │
                          ▼
        ┌────────────────────────────┐   PRE-DISPATCH  (THIS ADR — Selector)
        │  Selector  (policy)         │   privacy + cost DECIDE WHICH models
        │  per-Brain filter           │   enter the fan-out. Changes when
        │  catalog + sensitivity      │   data-governance / pricing change.
        │      → []model.Model        │
        └──────────────┬─────────────┘
                       │ []model.Model  (already filtered)
                       ▼
        ┌────────────────────────────┐   MECHANISM  (ADR-0011, unchanged)
        │  fanout.Coordinator        │    wait-all, parallel, deterministic.
        │      (parallel)            │    ── OR ──
        │  Sequential coordinator    │    serial-until-stop  ◀── SEPARATE
        │      (FUTURE ADR)          │        FUTURE ADR (mechanism sibling)
        └──────────────┬─────────────┘
                       │ *fanout.Result
                       ▼
        ┌────────────────────────────┐   POST-DISPATCH  (ADR-0012/0013)
        │  Policy  (reducer)         │    DECIDE WHAT TO DO with []Outcome.
        └────────────────────────────┘
```

- The **Selector** is *policy*: it decides *which* models go in. It is the
  pre-dispatch sibling of the reducer, and it belongs in `internal/policy`
  alongside it (§4).
- The **sequential coordinator** is *mechanism*: a different dispatch shape
  (serial, stop-at-first-acceptable), a sibling of `fanout.Coordinator`. It is
  what actually *saves* cost (the wait-all fan-out has already called and paid
  every provider before any policy runs — ADR-0012 §4). It is **out of scope
  here** and gets its own ADR.

Fusing selection and sequential dispatch into one ADR would conflate the
boundary that keeps the project's layering legible. Same stage, two ADRs —
exactly as ADR-0011 (mechanism) and ADR-0012 (policy) were kept separate.

### 2. Per-Brain cut — the Envelope is NOT touched (the key decision)

The first cut declares sensitivity **per-Brain, at construction**: "this Brain
is private → local-only providers." The canonical `Envelope` type is **not
modified**. This is a deliberate reversal of ADR-0012 §3's anticipation that the
Selector would need an Envelope sensitivity field first, and it rests on a
premise challenge:

- **Nothing can correctly *write* per-message sensitivity today.** A channel
  adapter receiving "my medical results are X" has no way to know the message is
  sensitive. The only thing that could set a per-message flag is a classifier
  that asks "is this private?" — and **that is explicitly forbidden** by
  ADR-0012 §5e (privacy is declared, not inferred; the classifier is a recursive
  cost/privacy trap — you leak the very data you are trying to protect to decide
  whether to protect it).
- **A typed `Envelope.Sensitivity` field would therefore be write-only with no
  correct writer** — a field on the most important type in the system that
  nobody can populate honestly. Adding it now is over-engineering on the
  canonical type: maximum blast radius for zero usable behaviour.

So sensitivity is declared where every other Brain configuration is declared:
the wiring. A `medical-bot` Brain is constructed `Private`; a `support-bot` is
constructed `Public`. The declaration is operator intent, recorded once.

**Blast-radius ranking of the "where does sensitivity live?" options** (the
eng-review lens — cost of being wrong measured against the canonical type):

| Option | Change | Blast radius / reversibility | Verdict |
|--------|--------|------------------------------|---------|
| **A — Per-Brain (this ADR)** | Declared at Brain construction; Envelope untouched | **Zero** on the canonical type. Trivial to revert | **CHOSEN** |
| **B — Convention over `Meta`** | `Meta["sensitivity"]="private"` | No type change, but stringly-typed (no compile-time safety, typo-prone) **and overloads `Meta`**, which today carries channel *addressing* | **REJECTED** — worst of both: unsafe *and* semantically muddies a field with a different job |
| **C — Typed `Envelope.Sensitivity`** | New enum on the canonical struct (style of `Direction`/`PartType`) | **High**: touches the struct, every constructor, JSON serialization, every test. Additive (zero-value safe, `omitempty`) makes it the *least-bad* canonical change, but still the expensive-to-revert one | **DEFERRED** — additive, until a real writer exists |

**C is deferred, not killed.** When a real per-message sensitivity *writer*
exists — almost certainly the no-code visual builder, which lets the operator
define per-message routing rules — `Envelope.Sensitivity` can be added
additively (zero value = unset = fall back to the per-Brain default), and the
per-message Selector interface (§4, A4) lands with it. The two are the same
future capability and ship together. This cut does not pre-build either.

### 3. Model attributes live in the WIRING catalog, not in `model.Model`

The Selector routes on model attributes (local vs cloud now; cost tier later).
`model.Model` is `Generate` + `Name()` only (ADR-0009) and **stays that way**.
Attributes are declared by the operator at wiring, in a **catalog** that pairs
each model with its attributes:

```go
// Locality classifies where a model runs, for privacy-aware selection.
type Locality int

const (
    Local Locality = iota + 1 // runs on the operator's own hardware (e.g. Ollama)
    Cloud                      // third-party hosted API (e.g. Groq)
)

// CatalogEntry pairs a model with the attributes the Selector routes on.
// Attributes are DECLARED by the operator at wiring, NOT read from model.Model
// (which is Generate + Name() only) — attribute declaration stays out of the
// provider contract.
type CatalogEntry struct {
    Model    model.Model
    Locality Locality
    // CostTier is added additively when cost-aware selection ships (§7).
}
```

Rejected alternative: extending `model.Model` with attribute methods (e.g.
`Locality()`). That is a global contract change — every adapter, present and
future, would have to implement it — to satisfy a need that is local to the
Selector. Boring-by-default: declare the attribute next to where the operator
already wires the model, not in the interface every provider must satisfy.

### 4. The Selector's shape — a pure filter at construction, in `internal/policy`

The per-Brain Selector is a **pure function** that filters the catalog by the
declared sensitivity, returning the models in catalog order. It lives in
`internal/policy` (it is pre-dispatch *policy*; placing it here closes the
deferral that `internal/policy/doc.go` and ADR-0012 §3 both record):

```go
// Sensitivity is the per-Brain declared data-sensitivity level. It is
// DECLARED by the operator at construction, never inferred (ADR-0012 §5e).
type Sensitivity int

const (
    Public  Sensitivity = iota + 1 // any provider, local or cloud
    Private                        // local-only: cloud providers excluded pre-dispatch
)

// SelectModels filters a catalog down to the models permitted for the given
// declared sensitivity, in catalog order. Private excludes every Cloud entry;
// Public keeps all. It is a pure function run ONCE, at Brain construction, in
// the per-Brain cut — there is no per-message state and no I/O, so (unlike the
// future per-message Selector, A4) it takes no context.Context, exactly as
// PriorityReducer is free to ignore the ctx it is handed.
//
// It returns ErrNoEligibleModels when the filter yields an empty set (e.g. a
// Private Brain wired with only Cloud models). That is an operator
// misconfiguration and must fail LOUD at construction, not silently produce a
// Brain that can never answer.
func SelectModels(cat []CatalogEntry, s Sensitivity) ([]model.Model, error)
```

New sentinel: `ErrNoEligibleModels` (bare, in the policy package), distinct from
the fan-out's `ErrNoModels` (empty input) — this one means "input was non-empty
but the sensitivity filter removed everything." Catching this at wiring instead
of at the first inbound message is the explicit-over-clever, handle-the-edge-case
choice.

**Reconciliation with ADR-0012's sketched `Selector` interface.** ADR-0012 §271
sketched `Selector.Select(ctx, env) ([]model.Model, error)` — an interface that
selects **per Envelope**. That is the **per-message** form, and it is deferred
together with `Envelope.Sensitivity` (§2, C): an env-taking interface whose only
implementation would *ignore* `env` (because per-Brain sensitivity does not vary
per message) is an invented abstraction, precisely the smell ADR-0012 §2 and
ADR-0014 A1 warn against. This cut ships the honest per-Brain form (a function +
a catalog); the per-message interface arrives when there is a per-message writer
to make it non-trivial. The pre-dispatch *phase* is real and shipped; only its
per-message variation is deferred.

### 5. How it fits the Orchestrator — it does not change (seam verification)

The framing flagged a subtlety in ADR-0014's "composition seam" and asked this
ADR to verify it. Verified, and the conclusion is clean:

- The `Orchestrator` holds a **fixed** `[]model.Model` set at construction and
  calls `coord.Run(ctx, req, o.models)` with it (`orchestrator.go`). ADR-0014's
  claim that `models`/`policy` being interfaces "lets a future `SelectingBrain`
  wrap this Brain additively" is true at the level of *interface dependency*,
  but a per-**message** model-set variation would still require either a
  `SelectingBrain` decorator that duplicates the `translate → run → policy →
  translate` pipeline, or a per-call `models` parameter on the Orchestrator.
- **The per-Brain cut needs none of that.** Filtering happens **once, before
  `NewOrchestrator`**: the wiring computes
  `models, err := policy.SelectModels(catalog, policy.Private)` and hands the
  already-filtered set to the existing constructor. The Orchestrator consumes a
  fixed set exactly as it does today. **Zero Orchestrator change.** The
  per-message seam question is sidestepped entirely — another reason the
  per-Brain cut is the right base for Stage 6.

```
WIRING (cmd/...)                          Orchestrator (UNCHANGED)
─────────────────────────────────        ─────────────────────────
catalog := []policy.CatalogEntry{
    {Model: ollamaM, Locality: Local},
    {Model: groqM,   Locality: Cloud},
}
models, err := policy.SelectModels(        ┌───────────────────────┐
    catalog, policy.Private)  ───────────▶ │ NewOrchestrator(coord, │
//   → [ollamaM]  (groqM dropped)          │   models, pol, ...)     │
//     BEFORE any Generate call            │ Handle: coord.Run(req,  │
                                           │   o.models)  ← fixed set │
                                           └───────────────────────┘
```

ADR-0014's seam stays exactly what it was advertised as: the extension point for
the **future** per-message `SelectingBrain`. This ADR does not consume it and
does not need it.

### 6. The minimal demonstrable cut — same payload, two Brains

The wedge that proves pre-dispatch selection: the **same inbound payload** sent
to two Brains constructed differently.

- A **Public** Brain over the full catalog (Ollama + Groq) → both providers
  enter the fan-out.
- A **Private** Brain over the same catalog filtered to `Private` (Ollama only)
  → **Groq is excluded from the `[]model.Model` before `coord.Run` is ever
  called**, so it is never contacted and never paid.

This is the "Ollama first" of Stage 6 in the **privacy** key (the cost key —
"don't call Groq if Ollama already answered" — is the sequential coordinator's
job, the other ADR). It is demonstrated by a disposable `cmd/demo-selector`
(deleted in Stage 11 with the other demos), kept separate from `cmd/demo-brain`
so the public-vs-private contrast on identical input is the single thing it
shows. Implementation and the demo follow in the Stage 6 phase work — **this ADR
adds no code.**

### 7. Cost selection — named, deferred; two distinct faces

"Prefer cheap providers" splits into two mechanisms that must not be confused:

1. **Pre-dispatch cost *filter*** ("only free-tier models enter the fan-out") —
   this is the *same* mechanism as privacy filtering: a `CostTier` attribute on
   `CatalogEntry` (§3) plus a filter predicate. It is **additive** to
   `SelectModels` and lands here naturally when wanted. Not built in this cut.
2. **Cost-*saving* fail-over** ("don't call the paid provider if the free one
   already succeeded") — this is *not* a filter; it is a different **dispatch
   shape** (serial, stop-at-first-acceptable). It belongs to the **sequential
   coordinator** (the mechanism-layer future ADR, §1), because pre-dispatch
   filtering cannot save the cost of a wait-all fan-out that calls everyone in
   parallel.

Naming both keeps the cost story honest and prevents the cost-saving claim from
leaking into this selection-only cut. Neither is built now.

### 8. Privacy declared, not inferred (carried forward from ADR-0012 §5e)

Reaffirmed and binding here: the `Sensitivity` value is **set by the operator at
Brain construction**. Korvun never inspects message content to guess
sensitivity. There is no classifier, now or later. The future
`Envelope.Sensitivity` field (§2, C) will also be *declared* (set upstream by a
human-configured rule in the no-code builder), never inferred.

## Consequences

### What this enables

- **The pre-dispatch phase of the policy engine exists**, closing the deferral
  ADR-0012 §3 and `internal/policy/doc.go` both record. Privacy-aware routing is
  demonstrable end-to-end: a Private Brain provably excludes cloud providers
  before contacting them.
- **The canonical `Envelope` is untouched**, so nothing downstream
  (serialization, channels, router, the four prior Envelope ADRs) is disturbed.
  The highest-blast-radius change available was avoided, not merely managed.
- **`model.Model` is untouched**, so no adapter changes — present or future.
- **The Orchestrator is untouched** (§5): selection is a wiring-time concern.
- **Cost-aware selection is a named, additive next step** (§7) on the very same
  catalog/filter machinery.

### What this asks / costs

- The operator must declare, at wiring, each model's `Locality` and each Brain's
  `Sensitivity`. This is configuration the no-code builder will eventually emit;
  until then it is hand-wired in `cmd/...`.
- `SelectModels` must be table-tested for the full matrix (Public keeps all;
  Private drops Cloud; empty eligible set → `ErrNoEligibleModels`; empty catalog;
  order preserved; single-entry catalogs of each locality), to the policy
  package's ≥90% bar, `make quality` green under `-race`.

### Trade-offs accepted

- **Per-Brain, not per-message, sensitivity in v1.** A single Brain cannot mix
  sensitive and non-sensitive messages on different routes. Accepted because no
  honest per-message *writer* exists yet (§2) and inferring one is forbidden
  (§8). The per-message field + interface are deferred together, additively.
- **Sensitivity declared in wiring code, not as config (yet).** Accepted because
  wiring is exactly where Brain configuration already lives, and the no-code
  builder will later source the same declaration from config without changing
  the Selector.
- **A pure function, not an interface, for the first cut.** Accepted to avoid an
  invented abstraction (A4); the interface arrives with its per-message consumer.

## Alternatives Considered

### A1 — Add `Envelope.Sensitivity` typed field now
**Rejected (§2).** Write-only with no correct writer today; inferring is
forbidden (§5e). Maximum blast radius on the canonical type for zero usable
behaviour. Deferred as additive until a real writer (the no-code builder) exists.

### A2 — Convention over `Meta` (`Meta["sensitivity"]="private"`)
**Rejected (§2).** Stringly-typed (no compile-time safety, typo-prone) and
overloads `Meta`, which carries channel *addressing*. Worst of both worlds: unsafe
and semantically muddied.

### A3 — Extend `model.Model` with attribute methods (e.g. `Locality()`)
**Rejected (§3).** A global provider-contract change — every adapter must
implement it — to serve a need local to the Selector. The wiring catalog
declares the attribute without touching the interface.

### A4 — Ship the env-taking `Selector` interface now (`Select(ctx, env)`)
**Rejected (§4).** This is the per-**message** form ADR-0012 sketched. With no
per-message sensitivity, its only implementation would ignore `env` — an invented
abstraction (the smell ADR-0012 §2 / ADR-0014 A1 name). Deferred together with
`Envelope.Sensitivity` (its required input).

### A5 — A `SelectingBrain` decorator over the Orchestrator now
**Rejected (§5).** A decorator implies per-message selection behaviour, but the
per-Brain cut has no per-message variation — the wrapper would do nothing per
message. The `SelectingBrain` shape is the future per-message design (ADR-0014
A1), not this cut. Per-Brain filtering at construction needs no decorator and no
Orchestrator change.

## Out of scope (recorded, not silently dropped)

- **The sequential coordinator** — Stage 6's mechanism half, the cost-*saving*
  fail-over. Its own future ADR (§1, §7).
- **`Envelope.Sensitivity` typed field** — deferred, additive, needs a writer
  (§2, A1).
- **The per-message `Selector` interface** (`Select(ctx, env)`) — deferred with
  the field above (§4, A4).
- **Cost-tier *selection*** — additive on the same catalog/filter once wanted
  (§7).
- **The no-code visual builder** that will eventually source `Sensitivity` and
  `Locality` from operator-defined config rather than hand-wiring.
- **`cmd/korvun` real bootstrap** wiring channel → router → brain (Stage 11).