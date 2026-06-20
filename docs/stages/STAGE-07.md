# Stage 07 — Brain orchestrator (first live end-to-end path)

> **Status:** closed
> **Started:** 2026-06-20
> **Closed:** 2026-06-20

## Objective

Make the five pieces one system. Stages 1–5 built the parts in isolation
— the Envelope (the canonical messaging event), the channel/router
plumbing, the `model.Model` adapters, the `fanout.Coordinator`
mechanism, and the post-dispatch policy reducers. The Brain is the
**stateless orchestrator** that wires them into a single path:

```
Envelope → translate → fan-out → policy → translate → Envelope
```

implementing the `brain.Brain` seam the router already consumes
(`Handle(ctx, *Envelope) ([]*Envelope, error)`). **This is the project's
first live end-to-end path**: a message can now travel from a channel
payload, through real LLM providers, through a real policy decision, and
back to a deliverable reply.

The key framing that de-risked the whole stage: **the Brain is NOT
structural concurrency.** The router owns concurrency (workers, queues,
the `Handle` timeout, the error hook); the fan-out owns parallelism. The
Brain in between is therefore *stateless sequential glue* — which is why
it shipped **directly to master with no feature branch** (ADR-0014 §6),
TDD on master exactly like the Stage 5 reducers.

## Phases

| Phase | Description                                              | Status |
|-------|----------------------------------------------------------|--------|
| 7.1   | `Orchestrator` + pure translators + `WithModelID` + §3   | done   |

## Phase 7.1 — Brain orchestrator

### Deliverables

- **`docs/adr/0014-brain-orchestrator.md`** (status `accepted`), pinning
  the Brain. Framed by `/office-hours`, stressed by `/plan-eng-review`,
  the code `/review`-checked. The load-bearing decisions:
  - **The Brain is stateless sequential glue, not structural concurrency**
    — so it ships to master directly, no branch (§6).
  - **Copy-don't-mutate is the correctness invariant** for the per-model
    request decorator (§2).
  - **Fallback vs error split** for the no-answer path (§3).
  - **Clean no-reply** when there is nothing to ask (§5).
  - **Conversation memory deferred** (Stage 9 — needs a persistence ADR +
    an injected conversation-keyed store, §4).
- **`internal/brain/orchestrator.go`** — `Orchestrator` (implements
  `brain.Brain`). `Handle` = `envelopeToRequest` → `coord.Run` →
  `policy.Apply` → `decisionToEnvelopes`. Stateless, safe to share across
  the router's N workers. `coord` / `models` / `policy` / `fallback` /
  `systemPrompt` / `logger` injected via `NewOrchestrator(coord, models,
  p, opts...)` with `WithFallback` / `WithSystemPrompt` / `WithLogger`.
  **`models` and `policy` are interfaces** precisely so a future
  `SelectingBrain` (Stage 6 pre-dispatch) can wrap the `Orchestrator`
  without touching it.
- **`internal/brain/translate.go`** — the two **pure** translators:
  - `envelopeToRequest(in, systemPrompt) (*model.Request, bool)` — the
    latest non-whitespace text part becomes a user `Message`; no text →
    `ok == false` → a clean no-reply (ADR-0014 §5), not an error.
  - `decisionToEnvelopes(content, in) []*envelope.Envelope` — **echoes the
    inbound addressing `Meta`** so the reply is deliverable without the
    Brain knowing channel-specific keys. The Brain stays channel-agnostic;
    the channel adapter owns `Envelope ↔ nativeFormat`.
- **`internal/brain/named.go`** — `WithModelID(m, id)` decorator (the
  unexported `named` type). It gives each provider its own model id by
  **COPYING the request** (`cp := *req; cp.Model = id`), never mutating the
  shared `*req` the fan-out hands every goroutine. **Copy-don't-mutate
  (ADR-0014 §2) is the load-bearing correctness constraint** — the same
  class of bug as Phase 2E.8's `close(channel)`-after-Wait race and the
  4.3 fan-out P2 zero-value race: correct-in-isolation pieces that race at
  their *intersection*. A heterogeneous fan-out test under `-race`
  enforces it.
- **`internal/brain/orchestrator.go` no-answer contract** (ADR-0014 §3) —
  the fallback/error split lives in `Handle` + `classifyPolicyErr`:
  - `coord.Run` error (nil ctx / no models / nil model / request
    validation) → the Brain is *misconfigured*, not "no answer" →
    propagated to the router error hook (`brain: fan-out: %w`).
  - `policy.Apply` returning `ErrNoUsableOutcome` / `ErrNoConsensus` → a
    normal product outcome → `logNoAnswer` records the **provenance**
    (`envelope_id`, `channel`, providers considered, cause) via `slog`,
    and the user gets the **fallback reply** Envelope, **no error**.
  - any *other* policy error (e.g. `ErrNilResult`) → mechanism misuse, a
    Brain bug → propagated (`brain: policy: %w`).
  - The user never sees silence on the common error path; the operator
    always gets the provenance.
- **`cmd/demo-brain/main.go`** — live skeleton running the whole path
  against real Ollama + Groq (Groq auto-skips without `GROQ_API_KEY`).

### Behaviour established by Phase 7.1

- **The five pieces are now one system through `Handle`.** Envelope in →
  translate → fan-out → policy → translate → Envelope out, end to end,
  with the sentinel grammar (`errors.Is`) intact from the adapter all the
  way to the no-answer classification.
- **The router-side half of the no-reply contract is anchored too.**
  `TestHandle_EmptyReplies_NothingSent` in `internal/router` proves that a
  Brain returning `nil, nil` (nothing to ask) sends nothing — the two
  halves of §5 meet at the router boundary.
- **The `model.Model` / `policy.Policy` interfaces held as injection
  seams.** A `SelectingBrain` can wrap the `Orchestrator` for Stage 6
  pre-dispatch routing without modifying it — the abstraction that
  Stage 5 / ADR-0012 promised would have a natural consumer now has one.

### Live skeleton, end-to-end check

`cmd/demo-brain` was exercised by the operator. In the environment as
run (**no Ollama / Groq reachable**), it demonstrated the **§3 no-answer
path precisely**: the fan-out was attempted, the policy returned
`ErrNoUsableOutcome`, the Brain **logged the real provenance** (which
providers were considered, the cause) and **returned the fallback reply
with the inbound addressing preserved** — the user is served a reply, not
silence. This is exactly the §3 fallback/error split working against live
wiring rather than fabricated data: the common failure mode (no provider
answers) resolves to a deliverable fallback, while a misconfiguration
would instead surface to the router error hook.

### `/review` pass (Phase 7.1)

`/review` ran on the code (the fourth invocation across the project, the
durable rule "review code, not ADRs" still holding). **Zero correctness
bugs** — the decorator-over-shared-`*req` race is genuinely closed (the
copy in `named.Generate` is real, the `-race` heterogeneous fan-out test
proves it). Its most valuable test-quality finding was applied: a real
`PriorityReducer`-over-real-fan-out **integration test**, since the prior
`Handle` tests used a `fakePolicy` and so bypassed the policy seam. That
integration test is also what gives Stage 5 its "validated live through
the Brain" claim.

## Workflow compliance

| Step                         | 7.1 |
|------------------------------|-----|
| Design framed before code    | `/office-hours` + `/plan-eng-review` → ADR-0014 |
| ADR written before code      | ADR-0014 accepted before any code |
| TDD red before green         | red on a stub, then green; on master (no branch, §6) |
| `make quality` green         | yes, with `-race` |
| `/review` on code, not ADR   | yes — zero correctness bugs |
| Stage doc updated            | this document |

## Quality gate (stage-wide, on master)

| Package                          | Coverage |
|----------------------------------|----------|
| `internal/brain`                 | **100.0%** (≥ 90% critical-package target) |
| `internal/policy`                | 100.0%   |
| `internal/channel`               | 100.0%   |
| `internal/channel/webhook`       | 91.4%    |
| `internal/channel/telegram`      | 90.5%    |
| `internal/envelope`              | 97.0%    |
| `internal/model`                 | 100.0%   |
| `internal/model/ollama`          | 96.0%    |
| `internal/model/groq`            | 94.7%    |
| `internal/model/fanout`          | 100.0%   |
| `internal/router`                | 96.3%    |
| **total**                        | **94.3%** |

`make quality` green with `-race`. `go.mod` still carries a single
direct dependency (`github.com/go-telegram/bot v1.21.0`) — the Brain is
stdlib-only (`context`, `errors`, `fmt`, `log/slog`).

## Key decisions

| ADR | Decision |
|-----|----------|
| ADR-0014 | Brain is stateless sequential glue, NOT structural concurrency → ships to master with no branch; `WithModelID` decorator uses copy-don't-mutate over the shared `*req` (the load-bearing race-correctness rule); fallback/error split on the no-answer path (`ErrNoUsableOutcome`/`ErrNoConsensus` → fallback + logged provenance, no error; `coord.Run` error → propagate to router); clean no-reply when there is nothing to ask; `models`/`policy` injected as interfaces so a Stage 6 `SelectingBrain` wraps without touching the `Orchestrator`; conversation memory deferred to Stage 9. |

## What is NOT yet wired (Stage 11)

The Brain end-to-end exists and is demonstrated by a demo that calls
`Handle` directly. The **single-binary wiring** — channel → router →
brain → channel inside a real `cmd/korvun` `main.go` — is **Stage 11**,
and is downstream of Stage 6. That is the last step for V1 checklist
criterion 1 (a real message in/out through a real binary, not a demo).
See `docs/ROADMAP-V1.md`.

## Notes

- **`cmd/demo-brain` is temporary.** It is deleted in Stage 11 when
  `cmd/korvun` proper boots and the Brain is exercised through the real
  binary instead of a `Handle`-calling skeleton.
- **The copy-don't-mutate race class has now appeared three times**
  (2E.8 `close`-after-`Wait`, 4.3 fan-out zero-value P2, this stage's
  shared-`*req` decorator). All three live at the *intersection of two
  individually-correct features*. Worth naming explicitly in any future
  phase that introduces concurrency across a shared value — the `-race`
  test must exercise the *combination*, not the pieces in isolation.
- **Stage 7 closes the orchestration seam, not the binary.** With this
  document the Brain is formally closed (code + ADR-0014 + this STAGE-07).
  The next stage is **Stage 6** (policy engine pre-dispatch phase —
  `Selector` + sequential coordinator), new design work that must be
  framed and ADR'd deliberately. The Brain's interface seams
  (`models`/`policy`) are the hooks Stage 6 will wrap.
