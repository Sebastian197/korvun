# ADR-0021: Agents (Stage 8) — a bounded single-model tool-use loop as an `AgentBrain` sibling, prompt-protocol first

> **Status:** accepted
> **Date:** 2026-06-27
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0003](0003-router-design.md) (the router owns concurrency:
> per-brain queue + N workers via `WithBrainWorkers`, per-call `Handle` timeout,
> outbound queue, error hook), [ADR-0009](0009-model-interface-and-ollama.md)
> (`model.Model` / `Request` / `Response`; **§2 precedent: per-provider capability
> divergence — streaming — lives in a SIBLING interface `StreamingModel`, not as a
> widening of `model.Model`**), [ADR-0011](0011-model-fanout.md)
> (`fanout.CallOne` — the shared per-call primitive: panic-recover, per-call
> timeout, `%w` sentinel-grammar preservation), [ADR-0014](0014-brain-orchestrator.md)
> (the stateless `Orchestrator`, the no-answer/fallback contract §3, `WithModelID`
> copy-don't-mutate §2, statelessness §4), [ADR-0018](0018-conversation-store-interface.md)
> (the injected `ConversationStore` seam), and [ADR-0020](0020-observability.md)
> (the `Metrics` seam — the natural home for a future tool-use trace).

## Context

Stage 8 gives a Brain the ability to call external tools **during** its
reasoning: instead of answering in one shot, the model may ask to invoke a tool,
receive the result, and reason again — a model→tool→model loop until it produces
a final answer. This is the project's first multi-step reasoning path.

Everything before Stage 8 is single-step. The router consumes `brain.Brain` via
`Handle(ctx, env) ([]*envelope.Envelope, error)`; the `Orchestrator` (ADR-0014)
implements it as **stateless sequential glue** — `translate → coord.Run →
policy.Apply → translate` — explicitly NOT structural concurrency (the router
owns the workers, the fan-out owns the parallelism). An agent is the first thing
that makes several **sequential** network calls on the hot path, so the design
question is where that loop lives without breaking the stateless single-step
property the whole orchestration layer rests on.

This ADR was framed by `/office-hours` (premise challenge + forced alternatives)
and stress-tested by `/plan-eng-review` (blast-radius / boring-by-default /
reversibility lenses), then reviewed by the operator's copilot, before any code.
The eng-review findings that changed the design are absorbed below, not parked:
B1 is a semantic lie (forcing `*fanout.Result` onto a loop), the native path is a
sibling capability interface (not a `model.Model` widening), and the minimal cut
is a seam-validation slice (not a feature).

### The single line that frames the whole ADR

**An agent is a different KIND of Brain, not a different dispatch shape — so it is
a new `AgentBrain` implementing `brain.Brain`, a sibling of the `Orchestrator`,
added ALONGSIDE it (strangler-fig), touching nothing else.** The safety limits
(max-iterations, total timeout, per-tool timeout, tool-failure handling) are the
**central property** of this cut, not options — the Stage-8 equivalent of the
env-only-API-key invariant (ADR-0010 §3): a tool loop without a hard iteration
cap is an infinite loop burning cloud quota. The minimal cut is a single-model
loop with three PURE tools (time, echo, calc) — a seam-validation slice, **not an
agent framework**.

### External-docs verification (per CLAUDE.md non-negotiable)

The minimal cut uses the **prompt-protocol** (§3): the tool-use grammar rides
entirely inside `model.Message.Content` over the **existing** `model.Model`
interface and the standard library (`context`, `errors`, `fmt`, `regexp`,
`strings`, `time`, `log/slog`) plus the in-repo `envelope`, `model`,
`model/fanout`, `brain`, and (optionally) `conversation` packages. **No external
library, SDK, or API is added — Context7 does not apply, `go.mod` stays at three
direct dependencies.** The deferred provider-native path (`ToolCallingModel`,
§3.4) WILL require Context7 verification of the Groq/OpenAI-compatible tool-use
request/response contract; that verification is a precondition of its own future
ADR, not of this one.

## Decision

### 1. `AgentBrain` — a new `brain.Brain`, sibling of `Orchestrator` (decision B2)

No new top-level seam. `AgentBrain` is a concrete type satisfying the existing
`brain.Brain` contract verbatim, mounted from config exactly like `Orchestrator`
(both are `brain.Brain`; the router and `cmd/korvun` are agnostic to which one is
wired). It does **not** mutate or wrap `Orchestrator`, `fanout`, `sequential`,
`policy`, or `model.Model`. It lives next to the Orchestrator.

```go
package brain

// AgentBrain is a stateless Brain (ADR-0014 §4) that runs a BOUNDED single-model
// tool-use loop: it asks one model, and while the model requests a tool it
// executes the tool and feeds the result back as an OBSERVATION, until the model
// answers or the iteration cap is hit (§2). It holds NO per-call mutable state —
// model, tools, limits, fallback, systemPrompt, logger, metrics are read-only
// after construction; every per-call value (the running []model.Message, the
// iteration counter, each tool result) is a local in Handle. It is therefore
// safe to share across the router's N worker goroutines (§5).
type AgentBrain struct {
    model    model.Model            // ONE model, id-bound via WithModelID (ADR-0014 §2)
    tools    map[string]tool.Tool   // injected registry (§4); read-only after construction
    maxIters int                    // hard loop cap (§2); default 5
    perTool  time.Duration          // per-Tool.Execute timeout (§2)
    fallback string                 // reply when the loop yields no answer (ADR-0014 §3)
    systemPrompt string             // operator system prompt, prepended once
    logger   *slog.Logger
    metrics  metrics.Metrics        // Nop by default (ADR-0020 §2)
}

func NewAgentBrain(m model.Model, tools map[string]tool.Tool, opts ...AgentOption) *AgentBrain
```

```
        inbound *envelope.Envelope
                  │
                  ▼  envelopeToRequest (PURE, ADR-0014 §5) — "nothing to ask" → no reply
                  │  seed: [system(toolPrompt+systemPrompt), user(text)]
                  │
       ┌──────────▼─────────────────────────────────────────────┐
       │  LOOP  (iter = 1 .. maxIters)                           │
       │    out := fanout.CallOne(ctx, req, model, perCall, now) │  reuse the proven
       │    if out.Err != nil → break → fallback (logged)        │  per-call primitive
       │    parse out.Response.Message.Content:                  │  (ADR-0011: recover,
       │      ┌─ first line is  TOOL: name(args)  ──────────┐    │   timeout, %w grammar)
       │      │     result := tools[name].Execute(ctx,args) │    │
       │      │     append assistant(tool request)          │    │
       │      │     append user("OBSERVATION: " + result)   │    │
       │      │     continue                                │    │
       │      └─ otherwise → FINAL ANSWER → exit loop ───────┘    │
       └──────────┬─────────────────────────────────────────────┘
                  │  final text  (or fallback if cap hit / model failed)
                  ▼  decisionToEnvelopes (PURE, ADR-0014 §5) — echoes inbound addressing
                  │  persistTurns(key, userText, finalText)  — FINAL PAIR ONLY (§5)
        []*envelope.Envelope  →  router ships them to the channel's outbound queue
```

`ctx` is the router's per-call `Handle` ctx (`WithBrainHandlerTimeout`); the
loop forwards it to every `CallOne` and every `Tool.Execute`, so the router's
existing total deadline bounds the whole multi-step loop (§2). `AgentBrain`
reuses the Orchestrator's pure translators (`envelopeToRequest`,
`decisionToEnvelopes`) directly — that is the reason it lives in `package brain`,
sibling to `orchestrator.go`, rather than in a subpackage that would have to
re-export them.

**Why not B1 (a third `brain.Coordinator` doing the loop) — REJECTED.** The
`brain.Coordinator` seam is `Run(ctx, req, models) (*fanout.Result, error)`: a
**dispatch-shape** seam (how to call N models in one shot), not a
**reasoning-loop** seam. Forcing an agent through it would (a) take `models
[]model.Model` but use only one — a misleading contract; (b) return
`*fanout.Result` (`[]Outcome`, the shape of one shot to N providers) to represent
an M-iteration loop with a tool trace — there is **no place for the trace**; and
(c) an `Orchestrator` containing it would run `policy.Apply` over a 1-element
Result, making the reducer a ceremonial no-op (the agent already decided). An
agent is not a dispatch shape; B1 smuggles a loop into a fan-out-shaped box.

### 2. Safety invariants — the central property, not options (essential, not accidental)

These are the Stage-8 equivalent of ADR-0010 §3 (env-only keys): load-bearing
invariants, documented so they are never "optimised" away. The systems-over-
heroes / 3am lens: a tool loop must be safe for a tired operator, not just on the
happy path.

```
INVARIANT                WHAT                                         WHY
─────────────────────────────────────────────────────────────────────────────────
max-iterations    hard cap set at construction (default 5).    without it the loop
                  Each model call is one iteration. Cap hit     is INFINITE — every
                  with no final answer → fallback reply         tool round-trip burns
                  (ADR-0014 §3) + logged provenance.            paid cloud quota.

total timeout     REUSE the router's Handle ctx — NO new        boring: inherit the
                  knob. Checked between every step; passed      mechanism the router
                  to every CallOne AND every Tool.Execute.      already owns; one
                  ctx.Err() between steps → break → fallback.   deadline bounds all.

per-tool timeout  each Tool.Execute gets a ctx bounded by       a hung tool must not
                  perTool (mirrors fanout.WithPerModelTimeout). hang the whole loop.

tool-failure      a Tool.Execute error is NOT fatal: it is      a failing tool
                  fed back to the model as an OBSERVATION       degrades gracefully,
                  ("OBSERVATION: tool <name> failed: <err>").   exactly like a
                  The model decides to retry / abandon. The     failing provider does
                  iteration cap still bounds the retries.       today (ADR-0014 §3).
─────────────────────────────────────────────────────────────────────────────────
```

A **model call** failure (`fanout.CallOne` returns `out.Err != nil` — provider
down, ctx cancelled, panic recovered) is distinct from a **tool** failure: the
model call failure aborts the loop and yields the fallback reply (logged), the
way `coord.Run`/`policy.Apply` failures degrade in the Orchestrator; the tool
failure is an observation that keeps the loop alive. This split mirrors
ADR-0014 §3's "product outcome vs mechanism bug" discipline.

### 3. The prompt-protocol — exact format (decision D2)

The minimal cut carries tool-use **entirely inside `model.Message.Content`**,
over the existing `model.Model` interface. Zero change to `model.Model`,
`Request`, `Response`, the adapters, or their tests (94%+ coverage untouched).
The protocol has no `Tool` role — `model.Role` stays `system|user|assistant`
(ADR-0009); observations ride as `user` messages by convention.

#### 3.1 How tools are advertised (loop → model)

A single `system` message, prepended once at loop seed, lists the registry and
the grammar:

```
You can use tools. To call a tool, reply with EXACTLY one line and nothing else:
TOOL: <name>(<args>)
You will then receive a line starting with "OBSERVATION:" carrying the result.
When you have the final answer, reply normally WITHOUT a TOOL: line.
Available tools:
- time: returns the current UTC time. args ignored.
- echo: returns its args verbatim. args = the text to echo.
- calc: evaluates a basic arithmetic expression. args = the expression, e.g. 2+2*3.
```

(The operator `systemPrompt`, if configured, is appended after this block.)

#### 3.2 How the model requests a tool (model → loop)

The loop parses the model's reply by its **first non-whitespace line** against
the strict grammar:

```
^\s*TOOL:\s*([a-zA-Z][a-zA-Z0-9_]*)\((.*)\)\s*$
```

- group 1 = tool name; group 2 = the raw `args` string, passed **verbatim** to
  `Execute(ctx, args)` (the tool owns any further parsing — `calc` parses its own
  expression). This keeps the seam domain-agnostic: the protocol transports a
  string, not a typed schema, in the minimal cut.
- If group 1 is not a registered tool → the loop returns an OBSERVATION
  `tool <name> not found` (a tool-failure-class observation, §2), never a crash.
- If the first line does NOT match the grammar → the reply is the **final
  answer** (§3.3). A reply that mixes prose and a `TOOL:` line is resolved by the
  first-line rule; this is a documented simplification of the minimal cut (the
  native path in §3.4 removes the ambiguity structurally).

#### 3.3 How the result is returned, and how the final answer is signalled

```
model reply "TOOL: calc(2+2)"
   → loop executes calc → "4"
   → loop appends:  assistant: "TOOL: calc(2+2)"          (the model's own turn)
                    user:      "OBSERVATION: 4"            (the result, as observation)
   → loop re-calls Generate with the grown message slice
model reply "The answer is 4."   (no TOOL: first line)
   → FINAL ANSWER → exit loop, this text is the reply
```

The final answer is signalled by **absence**: the first line is not a `TOOL:`
call. No `FINAL:` marker is required (keeping the model's job minimal); adding one
is an additive refinement if a model proves chatty.

#### 3.4 Native function-calling — DEFERRED as a sibling capability interface

The provider-native path is **explicitly out of this cut** and, when it lands, is
**not** a widening of `model.Model`/`Request`/`Response`. It is a sibling
capability interface, exactly mirroring the streaming precedent (ADR-0009 §2:
streaming-capable providers ALSO satisfy `StreamingModel`):

```go
// FUTURE (own ADR, Context7-verified against Groq's tool-use contract):
type ToolCallingModel interface {
    model.Model
    GenerateWithTools(ctx, *Request, []ToolSpec) (*Response /* +ToolCalls */, error)
}
```

Groq (OpenAI-compatible) would satisfy it; Ollama may not. `AgentBrain` would
detect the capability with a type assertion and prefer the structured path,
falling back to the prompt-protocol otherwise. Framing this as a capability
interface dissolves the blast-radius worry: the additive path is itself additive
(a new interface), the core seam and all adapters stay pristine, and non-agent
callers pay zero cognitive cost.

### 4. The `Tool` seam — interface + injected registry (reversibility)

A leaf seam, same shape as `ConversationStore` (ADR-0018) / `Metrics` (ADR-0020):
an interface plus an injected registry, so a tool can be defined anywhere without
importing `brain`, and a bad tool is removed by simply not registering it.

```go
package tool // internal/tool — a leaf (imports only the standard library)

// Tool is one external capability an AgentBrain may invoke mid-reasoning.
//
// CONCURRENCY CONTRACT (load-bearing): an implementation MUST be safe for
// concurrent Execute calls on a SINGLE instance. The router runs N brain workers
// (ADR-0003) over ONE shared AgentBrain (ADR-0014 §4), so two workers may call
// the same Tool instance at the same time. This is the same discipline
// model.Model and conversation.Store already carry. The three built-in tools
// (time, echo, calc) are PURE and therefore trivially safe; a future stateful
// tool (a counter, a cache) OWNS its own synchronization.
type Tool interface {
    // Name is the protocol identifier the model uses to call the tool.
    Name() string
    // Description is the one-line capability advertised in the system prompt.
    Description() string
    // Execute runs the tool. args is the raw string from the protocol (§3.2),
    // parsed by the tool itself. ctx is the per-tool-bounded context (§2). A
    // returned error becomes an OBSERVATION fed back to the model (§2), never a
    // loop-killing panic.
    Execute(ctx context.Context, args string) (string, error)
}
```

The registry is `map[string]tool.Tool` injected at construction
(`WithTools(...)` / the `tools` arg), the `WithConversationStore` / `WithMetrics`
pattern. The whole agent is reversible by config: mount `Orchestrator` instead of
`AgentBrain` and the tools vanish.

### 5. Statelessness and the mandatory `-race` test (inherits brainWorkers>1)

`AgentBrain` holds **no per-call mutable state** (§1): the running
`[]model.Message`, the iteration counter, and each tool result are **locals in
`Handle`**, never instance fields. This is the same property that makes the
`Orchestrator` shareable across the router's workers (ADR-0014 §4) — re-asserted
here because a loop accumulating messages is exactly where someone would be
tempted to hang state on the struct.

The agent loop × `brainWorkers > 1` is precisely the **"intersection of two
features"** bug class the project has hit twice (the 2E.8 `close`-after-Wait
race, the fan-out P2 zero-value-clock race — HANDOFF "honest record"): each part
is correct alone, the bug lives in the combination, and `-race` only catches it
if a test exercises the combination. **Mandatory:** a `-race` test runs `Handle`
**concurrently** on ONE `AgentBrain` whose registry contains a **stateful fake
tool** (e.g. a call-counter guarded by a mutex), asserting the tool's concurrency
contract (§4) holds and the per-`Handle` loop state never leaks between workers.

**DRY:** the loop calls `fanout.CallOne(ctx, req, model, perCall, now)` for each
model step (ADR-0011), inheriting the proven per-call discipline — panic-recover,
per-call timeout, `%w` sentinel-grammar preservation — instead of re-deriving it
(re-deriving the per-call grammar is exactly how the 4.3 `%w` P1 crept in). The
single model is id-bound with `WithModelID` (ADR-0014 §2) like any provider;
because the agent is single-model there is no shared-`*req` fan-out race, but the
copy-don't-mutate decorator is reused for uniformity.

### 6. Persistence — the FINAL user+assistant pair only (memory, not trace)

`AgentBrain` persists exactly what the `Orchestrator` does: on a successful
reply, the **final** user turn + assistant turn as one atomic group via
`Store.AppendTurns` (ADR-0018), through the same cancellation-detached
`persistTurns` (ADR-0019 §6) so the turn survives a graceful shutdown.

**The intermediate tool-use trace (each `TOOL:` request and `OBSERVATION:`) is
NOT persisted.** It is reasoning scratch, not conversation: reloading it into a
later request would feed the model its own prior tool chatter as if it were
dialogue. If a tool-use trace is ever wanted, it is **observability (Stage 12,
the `Metrics` seam / structured logs — ADR-0020), not conversation memory** — a
different seam with a different lifetime. `persistTurns` is therefore reused
**unchanged**; no new persistence surface in this cut.

### 7. Where it lives — proposed layout, feature branch + `/review`

```
internal/tool/                 NEW leaf package (stdlib only)
  tool.go         the Tool interface + the concurrency-contract godoc (§4)
  builtin.go      time, echo, calc — the three PURE built-in tools (§8)
internal/brain/
  agent.go        AgentBrain + Handle (the loop) + NewAgentBrain + AgentOption
  protocol.go     the prompt-protocol: system-prompt builder + first-line parser (§3)
  translate.go    envelopeToRequest / decisionToEnvelopes — REUSED, unchanged
  orchestrator.go UNCHANGED
```

Dependency direction (one way): `tool` is a leaf; `brain` imports `tool` (plus its
existing `envelope`, `model`, `model/fanout`, `policy`, `conversation`, `metrics`).
Nothing imports `brain` except the router and `cmd/korvun`.

**Unlike ADR-0014, this ships on a FEATURE BRANCH with `/review`, not direct to
master.** ADR-0014 went direct because the Orchestrator is stateless sequential
glue — *not* structural concurrency. The agent loop IS multi-step on the hot path
with concurrency-safe tools under `brainWorkers > 1` (§5) — exactly the
structural-concurrency bar the branch-plus-`/review` ritual exists for (4.3, 2E).
TDD on the branch, `/review` on the **code** (the multi-step loop with
concurrency-safe tools is where `/review` has historically caught the
intersection bugs), `make quality` green under `-race`, then merge `--no-ff`.

### 8. The minimal toolset — a SECURITY decision

The cut ships exactly three tools, all **PURE** (no I/O, no side effects, no
external reach):

- **time** — returns the current UTC time (reads the injected clock; args ignored).
- **echo** — returns its args verbatim (the trivial loop-proving tool).
- **calc** — evaluates a basic arithmetic expression from its args.

**Excluded as a deliberate security decision, not an omission:** shell execution,
arbitrary HTTP, filesystem access, email, and database writes. The minimal cut's
job is to prove the loop + the `Tool` seam with the **safest possible payload** —
a tool that can reach the network or the OS turns a reasoning bug into a remote
exploit. Dangerous/side-effecting tools are deferred behind a much higher bar
(their own ADR, with sandboxing / allow-listing / operator consent designed
first). This mirrors the env-only-key posture: keep the blast radius of a mistake
near zero by construction.

## Consequences

### What this enables
- The project's first multi-step reasoning path: a model can consult a tool
  mid-answer and incorporate the result — the seam every future agentic feature
  (richer tools, native function-calling, planning) extends.
- A reversible, config-mounted capability: `AgentBrain` is wired exactly where
  `Orchestrator` is; turning agents off is a config change, not a code change.
- A clean additive trajectory: the native path is a sibling `ToolCallingModel`
  (§3.4); richer/stateful tools are new `Tool` implementations; none of them touch
  the core `model.Model` seam or the Orchestrator.

### What this asks / costs
- A mandatory `-race` test exercising concurrent `Handle` over one `AgentBrain`
  with a **stateful** fake tool (§5). Non-negotiable — it is the test that proves
  the concurrency contract under the real router shape.
- The operator configures the model id, the tool registry, the iteration cap, the
  per-tool timeout, the fallback text, and (optionally) a system prompt — all
  construction options.
- Prompt-protocol fragility (§3): small local models may not emit the `TOOL:`
  grammar reliably; the agent demo therefore runs against Groq, and robustness is
  the job of the deferred native path, not this cut.

### Trade-offs accepted
- **Prompt-protocol over native function-calling.** Zero blast radius on
  `model.Model` now; structured robustness deferred to `ToolCallingModel` (§3.4).
- **Single-model loop over a multi-model agent.** No fan-out inside the loop —
  that combines two concurrent features (loop × fan-out) and is deferred.
- **Final-pair-only persistence over a persisted trace.** Memory stays clean
  conversation; the trace, if ever wanted, is observability (§6).
- **Three pure tools over a useful toolset.** A seam-validation slice, not a
  feature — usefulness is deferred behind the security bar (§8).

## Alternatives Considered

### B1 — a third `brain.Coordinator` that runs the loop
**Rejected** (§1). The Coordinator seam is dispatch-shape, not reasoning-loop;
forcing `*fanout.Result` onto an M-step loop has nowhere for the trace and makes
`policy.Apply` a ceremonial no-op. An agent is a different kind of Brain, so it
implements `brain.Brain` directly (B2).

### B3 — the loop WRAPS the Orchestrator (a decorator calling `Handle` repeatedly)
**Rejected** (framing). `Handle` takes/returns `Envelope`, not `Request`/
`Response`; wrapping it would serialise the tool-use state through Envelopes —
absurd impedance. The loop belongs at the message level, inside one Brain.

### D1 — widen `model.Model` (`Request.Tools`, `Response.ToolCalls`) now
**Rejected for this cut** (§3.4). Spends blast radius on the 5-adapter seam at
94% coverage BEFORE the loop is validated, and contradicts the established
precedent (ADR-0009 §2): per-provider capability divergence is a sibling
interface (`StreamingModel`), not a core-seam widening. The native path returns
as `ToolCallingModel`, additively, in its own Context7-verified ADR.

### A2 — multi-model agent (fan-out inside the loop), planning agent (ReAct)
**Rejected for this cut.** Multi-model combines the loop with the fan-out — two
concurrent features whose intersection is the project's known bug class (§5);
planning (plan-then-execute) is a heavier control structure with no consumer yet.
Both are single-model-loop extensions, deferred.

### A3 — dangerous tools (shell / http / fs) in the minimal cut
**Rejected** (§8). A network/OS-reaching tool turns a reasoning bug into a remote
exploit. The cut proves the seam with pure tools; dangerous tools need sandboxing
/ allow-listing / consent designed first, behind their own ADR.

## Out of scope (recorded, not silently dropped)
- Provider-native structured function-calling — the sibling `ToolCallingModel`
  interface (§3.4; own ADR, Context7-verified against Groq's tool-use contract).
- Multi-model agents — fan-out inside the loop (A2).
- Planning agents — ReAct / plan-then-execute (A2).
- Dangerous / side-effecting tools — shell, arbitrary HTTP, filesystem, email,
  DB writes (A3, §8).
- Multi-agent — agents that invoke other agents.
- Stateful budget / cost accounting — still deferred since Stage 9 (the iteration
  cap already bounds per-message cost; a hard budget needs its own state ADR).
- A persisted tool-use trace — if ever wanted, it is observability (Stage 12,
  ADR-0020 `Metrics` / logs), not conversation memory (§6).
