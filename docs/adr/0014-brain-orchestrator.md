# ADR-0014: Brain orchestrator (Stage 7) — stateless sequential glue over the five validated pieces

> **Status:** accepted
> **Date:** 2026-06-20
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0003](0003-router-design.md) (the router owns concurrency:
> per-brain queue + workers, `Handle` timeout, outbound queue, error hook),
> [ADR-0009](0009-model-interface-and-ollama.md) (`model.Model` / `Request` /
> `Response`), [ADR-0011](0011-model-fanout.md) (`fanout.Coordinator` owns the
> parallel dispatch; one `*Request` shared across goroutines), and
> [ADR-0012](0012-policy-engine.md) / [ADR-0013](0013-consensus-reducer.md)
> (`policy.Policy.Apply` → rich `*Decision`).

## Context

The Brain (Stage 7) is the orchestrator that turns Korvun's five validated
pieces into one live system. The router already consumes `brain.Brain` via
`Handle(ctx, env) ([]*envelope.Envelope, error)` (the Stage 3 forward slice in
`internal/brain/`). This ADR pins the first real implementation: an
`Orchestrator` that receives an inbound `Envelope`, runs a fan-out over a set of
models, applies a policy, and returns the reply Envelope(s). It is the seam where
channel → router → fan-out → policy → reply finally connect end-to-end — the
first time the project produces a live result instead of components with demos.

This ADR was framed by `/office-hours` and stress-tested by `/plan-eng-review`
before any code. The eng-review pushback (the `req.Model` race, the no-answer
contract, the statelessness/worker-count entanglement) is absorbed below, not
parked as open questions.

### The single line that frames the whole ADR

**The Brain is stateless sequential glue. It composes three validated owners; it
is not itself a mechanism, a policy, or a piece of structural concurrency.** The
router owns concurrency (per-brain queue, N worker goroutines via
`WithBrainWorkers`, per-call `Handle` timeout, outbound queue, async error hook).
The fan-out owns the parallel dispatch (ADR-0011, validated under `-race`). The
policy owns the decision (ADR-0012/0013, validated). So the Brain's first cut is
a pure-ish function — `translate → coord.Run → policy.Apply → translate` — with
no goroutines, no mutex, no lifecycle of its own. The risk that remains is
**contract fit, not races** (see §2). This is why the Brain ships **directly to
master, not a feature branch**: it is not structural concurrency, the bar a
dedicated branch + `/review` existed for in 4.3 and 2E. (§6 justifies this.)

### External-docs verification (per CLAUDE.md non-negotiable)

Pure internal domain over the standard library (`context`, `errors`, `fmt`,
`log/slog`) plus the in-repo `envelope`, `model`, `model/fanout`, and `policy`
packages. No external library, SDK, or API — Context7 does not apply, no new Go
module. `go.mod` stays at one direct dependency.

## Decision

### 1. A stateless `Orchestrator` implementing the existing `brain.Brain` seam

No new interface. The Brain is a concrete type satisfying the Stage 3
`brain.Brain` contract verbatim. `models` and `policy` are injected as
**interfaces** so a future `SelectingBrain` (per-message model/policy selection)
can wrap this Brain without changing it. No multi-brain registry (the router
already registers brains by name — ADR-0003); Stage 7 ships one Brain
*implementation*, not a brain registry.

```go
package brain

// Orchestrator is the stateless Brain: it fans a request out to a fixed set
// of models, applies a fixed policy to the outcomes, and translates the
// decision back to an outbound Envelope. Safe to share across the router's
// worker goroutines because it holds no per-call mutable state (§4).
type Orchestrator struct {
    coord    *fanout.Coordinator // owns parallel dispatch (ADR-0011)
    models   []model.Model       // id-bound (see §2); interface values
    policy   policy.Policy        // owns the decision (ADR-0012/0013)
    fallback string               // reply text when no usable answer (§3)
    // logger *slog.Logger        // structured logging of provenance (ADR-0012 §6)
}

func NewOrchestrator(coord *fanout.Coordinator, models []model.Model,
    p policy.Policy, opts ...Option) *Orchestrator
```

```
        inbound *envelope.Envelope
                  │
                  ▼  envelopeToRequest (PURE, §5) — "nothing to ask" → no reply
                  │
                  ▼  coord.Run(ctx, req, models)        [fan-out owns concurrency]
                  │  *fanout.Result
                  ▼  policy.Apply(ctx, result)          [policy owns the decision]
                  │  *policy.Decision
                  ▼  decisionToEnvelopes (PURE, §5) — echoes inbound addressing
                  │
        []*envelope.Envelope  →  router ships them to the channel's outbound queue
```

`ctx` is the router's per-call `Handle` ctx (`WithBrainHandlerTimeout`); the
Brain forwards it to `coord.Run` (the fan-out total deadline) and to
`policy.Apply`. The Brain owns the fan-out's per-model timeout knob at
construction (`WithPerModelTimeout` on the Coordinator) — a Brain config option.

### 2. The `req.Model` wrinkle → Brain-local id-bound decorator, COPY-don't-mutate (mandatory)

`model.Request.Model` is a single field, but `fanout.Run(ctx, req, models)` hands
the **same `*req` pointer to every goroutine** (`go c.callOne(runCtx, req, m, …)`,
ADR-0011). Both adapters read `req.Model` (`ollama.go:100`, `groq.go:152`). A
heterogeneous fan-out (Ollama `llama3.2` + Groq `llama-3.3-70b`) therefore needs
each provider to see its own model id without touching the one shared request.

**Resolution: a Brain-local decorator that gives each provider its id by COPYING
the request — never mutating the shared `*req`.**

```go
// named forces req.Model to a provider-specific id. It SHALLOW-COPIES the
// request and overrides Model on the copy. It MUST NOT write req.Model: the
// fan-out passes one *req to every goroutine concurrently, so mutating it is a
// data race AND yields nondeterministic per-provider ids. A shallow copy is
// enough — adapters only READ Messages, never mutate the slice.
type named struct {
    inner model.Model
    id    string
}

func (n named) Generate(ctx context.Context, req *model.Request) (*model.Response, error) {
    cp := *req        // shallow copy: new Model field, shared (read-only) Messages
    cp.Model = n.id
    return n.inner.Generate(ctx, &cp)
}

func (n named) Name() string { return n.inner.Name() } // attribution stays the provider name

// WithModelID binds a model id to a model.Model for use in a fan-out set.
func WithModelID(m model.Model, id string) model.Model { return named{inner: m, id: id} }
```

**The copy-don't-mutate rule is a load-bearing correctness constraint, documented
here so it is never "optimised" into a mutation.** Writing `req.Model = id` would
reintroduce exactly the "intersection of two features" race class the project has
already hit twice — the Phase 2E.8 `close(channel)`-after-Wait race and the
fan-out P2 zero-value-clock race (HANDOFF "honest record"): each component
(fan-out's shared `*req`, the decorator's id override) is correct alone, the bug
lives in their combination, and `-race` only catches it if a test exercises the
combination. The Brain's tests MUST run a heterogeneous fan-out under `-race` so
the copy is enforced by the suite, not just by this prose.

**Rejected — resolution (a): adapters ignore `req.Model` and use a
constructor-set id.** That changes the `model.Model` adapter contract
(`internal/model/ollama`, `internal/model/groq`, their tests, ADR-0009/0010
semantics, the demo commands) and silently redefines what `req.Model` means
everywhere — a global blast radius for a Brain-local need. The decorator keeps
the fan-out, the model interface, and the adapters untouched.

**Rejected — resolution (b): homogeneous-only fan-out in v1.** It dodges the
wrinkle but throws away the multi-provider story the whole project is built on
(Ollama + Groq of materially different shape, the Stage 4 thesis). The decorator
costs ~10 lines and keeps the heterogeneous fan-out the policies were designed
for.

### 3. The no-answer contract — two cases, split (essential, not deferred)

With a real fan-out, providers will sometimes ALL fail or disagree. If that came
back to the router as an `error`, the user would see **silence** — the message
evaporates. The common error path is the one the live-skeleton demo will hit, so
the no-answer reply is part of the minimal cut, not polish (the systems-over-
heroes / 3am lens). `Handle` splits two genuinely different outcomes:

```
policy.Apply returns err?
   │
   ├─ errors.Is(err, ErrNoUsableOutcome)  ─┐  PRODUCT outcome: providers failed/
   ├─ errors.Is(err, ErrNoConsensus)       ├─ disagreed. The Decision is non-nil
   │                                        │  with provenance.
   │                                        ▼
   │        → return ([fallback reply Envelope], nil)   + slog the provenance.
   │          The user sees "no answer right now"; the operator sees the why
   │          in the structured log. NO error propagated.
   │
   └─ any other policy error (e.g. ErrNilResult) ─┐  MECHANISM misuse — a Brain
                                                   ▼  bug, should not happen.
            → return (nil, fmt.Errorf("brain: policy: %w", err))  → router error hook.

coord.Run returns err? (nil ctx / ErrNoModels / ErrNilModel / request validation)
   → return (nil, fmt.Errorf("brain: fan-out: %w", err))  → router error hook.
   This is Brain MISCONFIGURATION (no models, invalid request), not a no-answer.
```

So the router's error hook (`ErrKindHandle`) fires only for **Brain-is-broken**
conditions (misconfiguration, mechanism misuse), never for the normal
"providers gave us nothing usable" outcome. The latter is a visible fallback
reply plus a structured log line carrying `Decision.Provenance` — the operator
can see which providers failed or how the vote split, without the user staring at
silence. The fallback text is operator-configurable (`Option`); SOME fallback is
mandatory.

### 4. Statelessness, pre-committed — and why future memory is an injected store

The `Orchestrator` holds **no per-call mutable state**: `coord`, `models`,
`policy`, `fallback` are read-only after construction; every per-call value
(`req`, `result`, `decision`, the request copy in §2) is a local. This makes it
**safe to share across the router's N worker goroutines** (`WithBrainWorkers`),
and agnostic to how many there are.

**This is a load-bearing property, so the ADR pre-commits its future:** when
conversation memory / history lands (Stage 9), it is an **injected store keyed by
conversation** (e.g. `sender`/`chat` id), NOT fields on the `Orchestrator`
instance. Per-instance mutable history would be **shared state across the
router's workers** — the exact race class §2 warns about, re-created at the Brain
level. Keeping memory external (the Brain stays a stateless orchestrator over a
`ConversationStore` interface) means the Brain instance stays shareable and
worker-count-agnostic forever. One sentence now; a Stage 9 refactor avoided. (The
store itself also waits on the persistence ADR the ROADMAP already tracks.)

### 5. The two pure translators — channel-agnostic, isolated from orchestration

Both translations are PURE functions (no I/O, no Brain state), analogous to the
Telegram adapter's pure converters. This keeps the Brain testable with zero
models and prevents channel-detail coupling.

**`envelopeToRequest(env) (*model.Request, bool)`** — builds the conversation
from the inbound Envelope's text. v1 is single-turn and stateless: the latest
text `Part` becomes one `RoleUser` Message (an optional per-Brain system prompt,
if configured, prepends a `RoleSystem` Message). The `bool` signals
**"something to ask"**: an Envelope carrying no text (a reaction, a location, a
bare callback) yields `false`, and `Handle` returns `(nil, nil)` — no reply, no
fan-out, no invalid request fed to `coord.Run` (which would fail
`model.ValidateRequest` and otherwise surface as a spurious error). This is the
same discipline as §3: a non-question is a clean no-reply, not an error.

**`decisionToEnvelopes(dec, in) []*envelope.Envelope`** — builds the outbound
reply by **echoing the inbound addressing**, so the channel can deliver it
without the Brain knowing channel-specific keys:

```go
out := &envelope.Envelope{
    ID:        newID(),                       // fresh outbound id
    Channel:   in.Channel,                    // route back to the same channel
    Direction: envelope.Outbound,
    Parts:     []envelope.Part{{Type: envelope.Text, Content: dec.Response.Message.Content}},
    Meta:      echoAddressing(in.Meta),       // carry chat_id etc. back, uninterpreted
    // Timestamp: now()
}
```

The Brain does NOT read `telegram.chat_id` or any channel-specific Meta key — it
copies the inbound addressing Meta forward so the round trip works for any channel
(the Telegram adapter reads `Meta[telegram.chat_id]` on Send; a future channel
reads its own key). This keeps the Brain channel-agnostic (the Telegram adapter
already proved the pure-converter pattern). Multimodal replies (images, tool
calls) are deferred — `model.Message` is text-only today.

**v1 echoes ALL of `in.Meta`; the addressing-vs-inbound-metadata split is a
named refinement, not over-designed now.** With a single channel in front, `echoAddressing`
copies the whole inbound Meta map (`echoAddressing` is the v1 identity-ish copy)
with an explicit `// TODO` in the code. When a second channel exists, define which
Meta keys are *addressing* (must round-trip, e.g. `telegram.chat_id`) versus
*inbound-only metadata* that should NOT propagate to the outbound (e.g. a
`telegram.edited_at` / `telegram.reaction_action`-style key). Designing that split
with one channel in front would be guessing; the ADR's job here is to surface the
question, which it does.

**Note — Stage 7 does NOT unblock `AsModel`.** The Brain calls `policy.Apply`
directly so it keeps the full `Decision` (provenance + accounting) to log
(ADR-0012 §6). It does not use the lossy `AsModel` adapter, so `AsModel` still has
no consumer; ADR-0012's "deferred to Stage 7" reconciliation note remains
accurate in spirit — the consumer that wants the `model.Model` shape is later
than Stage 7 (e.g. a nested Brain), not this orchestrator.

### 6. Where it lives — `internal/brain`, direct to master

```
internal/brain/
  brain.go         the Brain interface (Stage 3 forward slice) — UNCHANGED
  orchestrator.go  Orchestrator + Handle + NewOrchestrator + Option   (NEW)
  translate.go     envelopeToRequest / decisionToEnvelopes (pure)     (NEW)
  named.go         the id-bound model decorator + WithModelID         (NEW)
```

Dependency direction (one way): `brain` imports `envelope`, `model`,
`model/fanout`, `policy`. Nothing imports `brain` except the router (already) and
the future `cmd/korvun` bootstrap (Stage 11). The id-bound decorator lives in
`brain` to keep the `req.Model` fix Brain-local (zero blast radius, §2); if a
second consumer ever needs it, lifting `WithModelID` to `internal/model` is an
additive move.

**Direct to master, no feature branch.** The branch-plus-`/review` ritual in 4.3
and 2E existed for structural concurrency (the fan-out, the channel lifecycle).
The Brain is **not** structural concurrency: the router owns the workers, the
fan-out owns the parallelism, and the Orchestrator is stateless sequential glue
(§1, §4). The one place concurrency reappears — the shared `*req` in the fan-out
— is contained by the copy-don't-mutate rule (§2) and pinned by a `-race` test.
So the work proceeds TDD on master like the policy reducers, with `/review` on the
code (not the ADR) before close, and `make quality` green under `-race`.

## Consequences

### What this enables
- The first live end-to-end skeleton: a message can enter a channel, route to the
  Brain, fan out to real models, be decided by a real policy, and a reply returns
  — the project's first system-level result (`cmd/demo-policy` becomes a real
  `Handle` path; Stage 11 wires `cmd/korvun`).
- A Brain that is trivially testable: the two translators are pure (table tests,
  no models), and `Handle` is testable with fake `model.Model`s + a real
  `Coordinator` + a real `Policy`, all in-process under `-race`.
- A composition seam: `models`/`policy` as interfaces let a future
  `SelectingBrain` / pre-dispatch `Selector` wrap this Brain additively.

### What this asks / costs
- The Brain's tests MUST exercise a heterogeneous fan-out under `-race` to enforce
  the copy-don't-mutate rule (§2). Non-negotiable.
- The operator must configure a fallback reply text and (eventually) a system
  prompt and the model id per provider; these are construction options.
- Multimodal, history/memory, per-message selection, and a brain registry are all
  out of this cut (trajectory, §"Out of scope").

### Trade-offs accepted
- **Single-turn, stateless v1.** No memory; each message is answered in isolation.
  Accepted because memory needs the persistence ADR and an injected store (§4),
  and the skeleton's job is to prove the loop, not to remember.
- **Brain-local model decorator over an adapter-contract change.** ~10 lines in
  `brain` instead of touching every adapter — the low-blast-radius choice (§2).
- **Fallback reply on no-answer instead of silence.** A visible "no answer"
  Envelope plus a log line, rather than letting the message evaporate (§3).

## Alternatives Considered

### A1 — Configurable pipeline/strategy Brain (selectors per message)
A Brain taking a `ModelSelector` + policy selector, choosing per message.
**Rejected for this cut.** It builds the deferred pre-dispatch `Selector` (which
needs Envelope sensitivity modelling that does not exist) before the skeleton is
even alive — premature. The interface-typed `models`/`policy` fields keep it a
clean additive wrap later (§1).

### A2 — Brain over `AsModel` (fan-out + policy behind one `Generate`)
**Rejected.** `AsModel` is lossy: it discards `Provenance` and `Accounting`, which
is exactly the data the Brain must log (ADR-0012 §6). The Brain calls
`policy.Apply` directly and keeps the full `Decision` (§5 note).

### A3 — `req.Model` resolution (a) adapter-contract change / (b) homogeneous-only
**Rejected** (§2): (a) is a global contract change for a local need; (b) discards
the multi-provider thesis. The Brain-local copy-don't-mutate decorator is the
boring, low-blast-radius, reversible choice.

### A4 — Memory as `Orchestrator` instance fields
**Rejected pre-emptively** (§4). Per-instance history is shared state across the
router's workers — a race. Future memory is an injected conversation-keyed store;
the Brain stays stateless.

## Out of scope (recorded, not silently dropped)
- Conversation memory / history (Stage 9; injected store, persistence ADR first).
- Per-message model/policy selection and the pre-dispatch `Selector` (A1; needs
  Envelope sensitivity modelling).
- A brain registry in Stage 7 (the router already registers brains by name).
- Multimodal Envelope parts (images/tool-calls) — `model.Message` is text-only.
- `AsModel`'s consumer (A2; later than Stage 7).
- The `cmd/korvun` real bootstrap that wires channel + router + brain (Stage 11).
