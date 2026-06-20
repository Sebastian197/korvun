# ADR-0013: Consensus reducer — majority vote over normalized structured output, on the unchanged `Policy`/`Decision` contract

> **Status:** accepted
> **Date:** 2026-06-20
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0012](0012-policy-engine.md) (the `Policy` interface,
> the `Decision` / `Provenance` / `Contribution` / `ProviderCost` types,
> `PriorityReducer`, `ErrNoUsableOutcome`, the deterministic tie-break) and
> [ADR-0011](0011-model-fanout.md) (`fanout.Result` / `Outcome`,
> deterministic order). Cashes in the forward reference ADR-0012 §5d left
> for consensus.

## Context

ADR-0012 shipped the first post-dispatch reducer (`PriorityReducer`) on a
deliberately rich `Policy` / `Decision` contract. This ADR adds the **second**
post-dispatch reducer — consensus / majority — on the SAME contract.

The second reducer is the contract's test of fitness. `PriorityReducer` is a
*selection* policy: it picks exactly one Outcome and discards the rest. Consensus
is a policy of a materially different nature: several Outcomes can jointly
determine the result by agreeing. ADR-0012 §2 made two forward bets to
accommodate it — `Decision` is rich on day one, and `Contribution.Used` means
"contributed", not "won" (which is why `Provenance.Chosen` was removed). This ADR
verifies those bets hold. It is the same move as Stage 4 validating the
`model.Model` interface against a provider of a different shape (cloud-bearer-token
Groq vs local-no-auth Ollama): a second, differently-shaped consumer either
confirms the abstraction or exposes it.

### Result of the fitness check (decided up front)

**`Policy` and `Decision` hold UNCHANGED. No field is added or modified.** The
consensus reducer fits `Apply(ctx, *fanout.Result) (*Decision, error)` exactly,
and fills the same `Decision{Response, Provenance, Accounting}` with multiple
`Contribution.Used == true` — precisely the case `Contribution`'s godoc already
anticipates. The one place a future additive field *could* land is named in §9
and is explicitly NOT required by this cut.

### External-docs verification (per CLAUDE.md non-negotiable)

Pure internal domain over the standard library (`context`, `errors`, `strings`,
`sort`/manual scan) plus the in-repo `internal/model` and `internal/model/fanout`
packages. No external library, SDK, or API — Context7 does not apply, no new Go
module. `go.mod` stays at one direct dependency.

## Decision

### 1. The contract is unchanged — `Policy.Apply` + the rich `Decision`

```go
// ConsensusReducer satisfies the existing Policy interface verbatim:
//   Apply(ctx context.Context, result *fanout.Result) (*Decision, error)
```

It returns the same `*Decision`:
- `Response` — the single representative reply of the winning class (§4).
- `Provenance.Considered` — one `Contribution` per Outcome, in fan-out order,
  with **every member of the winning class marked `Used: true`** (§3). This is
  the multi-contributor case `Contribution`'s godoc named; no new field.
- `Accounting` — per-provider `Latency`, identical to `PriorityReducer`.

No change to `Decision`, `Provenance`, `Contribution`, or `ProviderCost`. The
contract passes the fitness test.

### 2. What is compared — normalized content, structured output only

The hard constraint from ADR-0012 §5d is binding: **consensus runs over a
normalizable structured value (a label / class), never free LLM prose.** Two
models never write the same paragraph, so exact-match over prose would never fire
(the same failure mode as the rejected exact-match-on-prose). The real use case
is "N models classify something → vote the majority class", not "N models draft →
pick the common wording".

The reducer compares a **normalized form of `Response.Message.Content`**:

```go
type ConsensusReducer struct {
    // Order is the provider priority used ONLY to pick the representative
    // within the winning class and to break representative ties (§4). Same
    // semantics as PriorityReducer.Order. Optional; nil/empty is valid.
    Order []string

    // Normalize maps a Response's content to its comparison key (the "class").
    // nil defaults to defaultNormalize = strings.ToLower(strings.TrimSpace(s)),
    // which makes label voting fire across trivial differences ("Yes" / "yes" /
    // " YES " collapse to "yes"). Operators needing JSON-canonicalisation or a
    // case-sensitive key supply their own. This is the seam that enforces the
    // "structured output, not prose" contract: a normalizer for prose would
    // never collapse two paragraphs to the same key, and that is by design.
    Normalize func(string) string
}
```

Two fields, both optional; the zero value `ConsensusReducer{}` is valid (nil
`Order`, nil `Normalize` → default). The vote key for a successful Outcome is
`Normalize(outcome.Response.Message.Content)`. Only **usable** outcomes vote —
`Err == nil && Response != nil`, the same usability predicate `PriorityReducer`
uses (and the same conservative handling of the exactly-one-of-Response/Err
invariant: a both-non-nil Outcome is a failure and does not vote; a both-nil
Outcome does not vote). A usable Outcome whose normalized key is the empty string
votes for the empty class — the reducer does not judge content quality, only
agreement; this is documented, not special-cased.

Semantic-equivalence voting (embeddings, a judge model) is explicitly NOT this
cut — it is a heavier, I/O-bearing future reducer (and an innovation token we are
not spending). The `Normalize` seam keeps the door open: a future
embedding-bucketing function could plug into the same field, but the v1 reducer
stays pure and offline.

### 3. Provenance with multiple contributors — the fitness test, passed

Under consensus, **every successful Outcome whose class is the winning class gets
`Used: true`**. This is exactly why ADR-0012 removed `Provenance.Chosen` and
defined `Used` as "contributed", not "won": all members of the majority are why
the decision is what it is, so all of them contributed.

```
Considered[i].Used  := outcome i succeeded AND its class == winning class
Considered[i].Err   := outcome i's failure (nil for successes)
```

- Failed Outcome:                 `Used=false`, `Err=<cause>`.
- Successful, minority class:      `Used=false`, `Err=nil`.
- Successful, winning class:        `Used=true`,  `Err=nil`.

A minority success (`Used=false, Err=nil`) is distinguishable from a failure
(`Err!=nil`). The vote tally is recoverable without a new field: the winning
count is `len(Used==true)`; the per-provider raw vote is recoverable by the Brain
from the paired `fanout.Result` (`Considered[i]` ↔ `Outcomes[i]`, same provider,
same index), which the Brain already holds. No `Decision` change is required to
log the breakdown (§9 names the only future additive option and why it is not
needed now).

### 4. Which `Response` becomes `Decision.Response` — deterministic representative

When several Outcomes share the winning class, `Decision.Response` is **one**
concrete reply: the **representative**, chosen by the SAME deterministic ranking
`PriorityReducer` uses (ADR-0012 §4/§5b) — highest priority by `Order`, then
lowest fan-out index. Reusing one ranking rule keeps both reducers consistent and
the choice reproducible (`fanout.Result.Outcomes` is in deterministic input
order, an ADR-0011 guarantee). With an empty `Order`, the representative is simply
the lowest-index member of the winning class.

The representative is NOT specially flagged in provenance — it carries `Used=true`
like the rest of its class, because per ADR-0012 it merely contributed alongside
them. A consumer that needs "whose text became the Response" matches
`Decision.Response.Provider` against `Considered`. (`Decision.Response` is the
representative's full `*model.Response`, so its `Provider`/`ModelName` attribution
is intact.)

### 5. Vote ties (2 vs 2) — dissolved by majority semantics, not arbitrated

The reducer requires a **strict majority of the successful outcomes** (§6).
Because at most one class can hold more than half the votes, **a winning group is
always unique — a 2-vs-2 split has no majority and is therefore NOT a consensus**;
it returns `ErrNoConsensus` (§6). This dissolves the group-tie question instead of
arbitrating it: there is no "which group wins on a tie" because a tie is, by
definition, not a majority.

The only tie-break that remains is *within* the winning class — choosing the
representative (§4) — and that reuses `Order`-then-index. We explicitly **reject a
latency-based tie-break**: `Outcome.Latency` is wall-clock and varies run to run,
so it would make selection non-reproducible, contradicting the determinism
guarantee this design leans on. (If a future `PluralityReducer` ever allows a
non-majority largest group to win, group ties resurface and the same
`Order`-then-index rule applies — but that is a different reducer, §"Alternatives".)

### 6. The threshold — strict majority of successful outcomes, minimum two

A class is the **consensus class** iff it is held by **strictly more than half of
the successful outcomes** AND by **at least two** of them:

```
winning := class C such that  count(C) * 2 > len(successful)   and   count(C) >= 2
```

- Strict majority (`*2 > len`) makes the winner unique and matches the plain
  meaning of "majority / consensus": more models agreed on it than on everything
  else combined.
- The **minimum-two floor** kills the degenerate single-success case: with exactly
  one successful Outcome, that one is a trivial "majority of one", which is not
  consensus. The floor only ever bites when `len(successful) == 1` (for
  `len(successful) >= 2`, any strict-majority class already has `>= 2` members).
  An operator who wants "use the lone answer if it is all there is" composes a
  fallback (consensus, then `PriorityReducer`) — exactly the composition the
  policy engine exists to enable.

The threshold is **not configurable in v1** (hard-coded strict-majority + floor).
A configurable threshold and a plurality mode are deferred as additive options
(the struct can grow a field without breaking callers) — see "Alternatives".

### 7. No consensus — `ErrNoConsensus`, `Decision` non-nil, provenance preserved

When successful outcomes exist but no class meets §6, `Apply` returns a **non-nil
`*Decision`** (`Response == nil`, `Provenance` and `Accounting` fully populated for
the log, **no** `Used` set since nothing won) together with a new sentinel:

```go
// ErrNoConsensus is returned by a consensus Policy when usable outcomes
// existed but no class reached the agreement threshold (§6). Distinct from
// ErrNoUsableOutcome: there WERE answers, they just disagreed. The vote
// breakdown is in Decision.Provenance (+ the paired fanout.Result). The Brain
// surfaces an operator-configured fallback on this sentinel.
var ErrNoConsensus = errors.New("policy: no consensus")
```

`ErrNoConsensus` is a **bare** sentinel (no `errors.Join` of causes): disagreement
is not a set of provider failures, it is the absence of a majority. The detail
lives in `Decision.Provenance`, mirroring how `PriorityReducer` keeps a non-nil
`Decision` on `ErrNoUsableOutcome` (ADR-0012 §5a).

### 8. All-failed — reuse `ErrNoUsableOutcome`, distinct from `ErrNoConsensus`

If **no** outcome is usable (all failed — nothing to vote on), `Apply` returns
`ErrNoUsableOutcome`, reusing ADR-0012's exact semantics: a non-nil `Decision`
(`Response` nil, provenance with every failure) and `errors.Join(ErrNoUsableOutcome,
<each cause>)` so `errors.Is` reaches the sentinel and every upstream model error.

The two sentinels are distinct and ordered by the check sequence in `Apply`:

```
result == nil ........................ ErrNilResult              (caller misuse)
        │
        ▼  build Considered + Accounting for every Outcome (fan-out order)
        │
   len(successful) == 0  ──yes──►  ErrNoUsableOutcome            (nothing to vote on)
        │ no
        ▼  group successful by Normalize(content); find strict-majority class (§6)
        │
   majority class found ──no───►  ErrNoConsensus                 (answers disagreed)
        │ yes
        ▼  representative = best-ranked member (§4); mark winning class Used
        │
       *Decision{Response, Provenance, Accounting}, nil          (consensus)
```

`ErrNoUsableOutcome` (all failed) and `ErrNoConsensus` (disagreement) never
combine: all-failed is checked first and short-circuits, because with zero
successful votes there is no consensus question to ask.

### 9. Where it lives — same package, additive files

```
internal/policy/
  policy.go      Policy / Decision / Provenance / Contribution / ProviderCost   (UNCHANGED)
  errors.go      + ErrNoConsensus   (ErrNilResult, ErrNoUsableOutcome unchanged)
  priority.go    PriorityReducer    (unchanged)
  consensus.go   ConsensusReducer + Apply + defaultNormalize   (NEW)
```

`ConsensusReducer` lives beside `PriorityReducer` and satisfies the same `Policy`.
The representative-ranking logic (`Order`-then-index) is shared in spirit with
`PriorityReducer`; the implementation may extract a small unexported `rank`
helper so both reducers compute priority identically (a refactor the tests will
pin). No change to the package's public surface beyond adding `ConsensusReducer`
and `ErrNoConsensus`.

**Provenance and the no-code builder (ADR-0012 §6).** The winning-class membership
(`Used==true` set) answers "why this class won" directly. The only thing
`Provenance` alone (without the paired `Result`) cannot show is each *minority*
voter's specific class. That is recoverable by the Brain via the `Result`, so it
is not needed now. IF a future requirement forces the builder's debug view to read
the full vote spread from `Decision` alone, the fix is an **additive** field on
`Contribution` (e.g. `Class string`) — explicitly sanctioned by ADR-0012 §2's
additive-growth principle, and explicitly NOT taken in this cut. This is the one
named extension point; taking it now would be inventing a field ahead of need.

## Consequences

### What this enables
- A genuinely different-natured reducer on the unchanged contract, validating
  ADR-0012's forward bets (rich `Decision`, `Used` = "contributed").
- Composable fallbacks: `ConsensusReducer` then `PriorityReducer` gives
  "agree if you can, otherwise prefer the trusted provider" — two small policies,
  no special code.
- A pure, offline, table-testable reducer: `Apply` is a pure function of
  `(*fanout.Result, Order, Normalize)`. The majority math, the minimum-two floor,
  the representative tie-break, `ErrNoConsensus`, and the all-failed reuse are all
  unit-testable with hand-built Outcomes — no network, no models.

### What this asks / costs
- The Brain (Stage 7) must handle a second sentinel, `ErrNoConsensus`, distinctly
  from `ErrNoUsableOutcome` (different fallback: "models disagreed" vs "all
  providers failed").
- Consensus is constrained to structured/label output. Prose consensus is not
  offered and would not fire; operators must shape provider output to a class (a
  prompt-engineering concern, not a reducer concern).
- The threshold is fixed (strict majority + floor of two) in v1.

### Trade-offs accepted
- **Strict majority over plurality.** A 40%-plurality is not declared consensus;
  it returns `ErrNoConsensus`. Accepted: "most models agreed" is a stronger,
  more honest contract than "the largest minority won", and it removes the group
  tie-break entirely (§5).
- **Single successful outcome → `ErrNoConsensus`, not a free pass.** A lone answer
  is discarded by the consensus reducer (the operator composes a fallback to keep
  it). Accepted: a consensus policy that fabricates consensus from one voter is
  lying.
- **No configurable threshold yet.** Fixed semantics ship first; configurability
  is additive when a real operator needs it.

## Alternatives Considered

### A1 — Add a vote-tally / class field to `Decision` or `Contribution` now
**Rejected.** The reducer functions without it, the winning set is captured by
`Used`, and the per-provider class is recoverable from the paired `Result`. Adding
it now is inventing a field ahead of need (ADR-0012 §2). Named as the one future
additive extension point (§9), not taken.

### A2 — Plurality (largest group wins, even without a majority)
**Rejected for v1.** Plurality can declare "consensus" on a minority and
reintroduces the group-tie problem (two groups of equal largest size), forcing an
arbitrary group tie-break. Strict majority is the honest "consensus" and is
unique by construction. A separate `PluralityReducer` can be added later if a real
use case wants it; it would reuse the `Order`-then-index rule for its group ties.

### A3 — Relative threshold as `>50%` only vs absolute `MinAgreement int`
**Deferred, not taken.** v1 hard-codes strict-majority-of-successful plus a
floor of two. A configurable `MinAgreement` (absolute) or `Quorum` (relative)
field is an additive struct change when an operator needs to tune it; shipping it
now is speculative configurability.

### A4 — Semantic-equivalence consensus (embeddings / judge model)
**Rejected for this cut.** It is I/O-bearing (needs `ctx` for real cancellation),
spends an innovation token, and changes the reducer from pure to networked. The
`Normalize` seam keeps it reachable later without a contract change.

### A5 — Latency-based representative / group tie-break
**Rejected.** `Latency` is wall-clock and non-reproducible; a determinism-breaking
tie-break contradicts the guarantee the rest of the design relies on. Tie-breaks
use the deterministic fan-out order + `Order` only (§4/§5).

## Out of scope (recorded, not silently dropped)
- Plurality mode (A2) and configurable threshold (A3) — additive later.
- Semantic-equivalence / judge-model consensus (A4).
- A `Class`/`Vote` field on `Contribution` (§9, A1) — additive iff the builder
  needs the full spread from `Decision` alone.
- Weighted voting (provider reputation/weight) — needs a weight source, a future
  concern.
- Everything ADR-0012 already deferred (pre-dispatch `Selector`, cost-saving
  sequential coordinator, stateful budgets, monetary cost, `RunStream`).
