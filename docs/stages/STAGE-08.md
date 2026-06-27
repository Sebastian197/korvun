# Stage 8 — Agents (tool-use)

> **Status:** closed
> **Started:** 2026-06-27
> **Closed:** 2026-06-27
> **ADR:** [ADR-0021](../adr/0021-agents.md) (accepted)

## Objective

Let a Brain call external tools **during** its reasoning: instead of answering in
one shot, the model may ask to invoke a tool, receive the result, and reason
again — a model→tool→model loop until a final answer. This is Korvun's first
multi-step reasoning path. Everything before Stage 8 is single-step: the router
consumes `brain.Brain` and the `Orchestrator` (ADR-0014) implements it as
stateless sequential glue (`translate → coord.Run → policy.Apply → translate`),
explicitly NOT structural concurrency.

The stage was framed with `/office-hours` (alternatives generation) and
`/plan-eng-review` (blast radius, boring-by-default + innovation tokens,
reversibility, essential-vs-accidental complexity), reviewed by the operator's
copilot, then pinned by ADR-0021 before any code.

## The line that framed the stage

**An agent is a different KIND of Brain, not a different dispatch shape.** So it
is a new `AgentBrain` implementing `brain.Brain`, a SIBLING of the `Orchestrator`,
added alongside it (strangler-fig), touching no other seam. The minimal cut is a
**seam-validation slice** (like `demo-policy`/`demo-brain` were for the reducers),
NOT an agent framework: a single-model loop with three PURE tools.

## What landed

### `AgentBrain` — decision B2 (`internal/brain/agent.go`)

A new `brain.Brain` implementation, mounted from config exactly like the
`Orchestrator` (both satisfy `brain.Brain`; the router and `cmd/korvun` are
agnostic to which one a brain wires). It does NOT mutate or wrap the
`Orchestrator`, `fanout`, `sequential`, `policy`, or `model.Model`.

**Why not B1 (a third `brain.Coordinator`):** the Coordinator seam
(`Run(ctx, req, models) (*fanout.Result, error)`) is a dispatch-shape seam, not a
reasoning-loop seam. Forcing an agent through it would return `*fanout.Result`
(the shape of one shot to N providers) to represent an M-iteration loop with a
tool trace — no place for the trace — and make `policy.Apply` a ceremonial no-op.
Rejected.

### The `Tool` seam + three PURE tools (`internal/tool`)

A leaf package (stdlib only): `Tool{ Name() string; Description() string;
Execute(ctx, args) (string, error) }` plus an injected `Registry`
(`map[string]Tool`), the same seam discipline as `conversation.Store` /
`metrics.Metrics`. The godoc declares the **concurrency-safe contract**: N router
workers may call ONE Tool instance at once.

The three built-ins are ALL **PURE** — a deliberate **security decision**
(ADR-0021 §8), not a feature gap: `time`, `echo`, and `calc`. Shell, arbitrary
HTTP, filesystem, email, and DB writes are excluded by construction — a
network/OS-reaching tool turns a reasoning bug into a remote exploit. **`calc` is
its own bounded recursive-descent arithmetic parser** (+ - * /, unary minus,
parentheses, standard precedence) — NEVER `eval`, no expression-evaluator
library, no new dependency. `tool.Builtin(name)` is the single source of truth for
the safe-toolset boundary; a dangerous name returns `ok=false`.

### The prompt-protocol — decision D2 (`internal/brain/protocol.go`)

Tool-use rides entirely inside `model.Message.Content` over the existing
`model.Model` interface — **zero change to `model.Model` / `Request` / `Response`
/ the adapters / their 94%+ tests**. There is no Tool role; observations ride as
`user` messages. The system-prompt builder lists the tools deterministically
(sorted) with the grammar; the reply parser classifies the first MEANINGFUL line
(blank lines, code fences, surrounding backticks, whitespace, keyword case all
treated as formatting noise) as either `TOOL: name(args)` or a final answer.
Prose-before-tool falls to a final answer (the documented minimal-cut
simplification).

**Native function-calling is deferred** explicitly as a future SIBLING capability
interface `ToolCallingModel` (mirroring `StreamingModel`, ADR-0009 §2) — NOT a
widening of `model.Model`. That dissolves the blast-radius worry: the additive
path is itself additive.

### Safety invariants (the central property, ADR-0021 §2)

The Stage-8 equivalent of the env-only-API-key invariant — load-bearing, not
options:

- **max-iterations**: hard cap at construction (default 5); cap hit → fallback.
- **total timeout**: REUSES the router's `Handle` ctx — NO new knob; checked
  between every step and passed to every `CallOne` and every `Tool.Execute`.
- **per-tool timeout**: each `Tool.Execute` gets a ctx bounded by `perTool`
  (mirrors `fanout.WithPerModelTimeout`).
- **tool-failure**: a `Tool.Execute` error (or an unknown tool) is NOT fatal — it
  is fed back to the model as an `OBSERVATION`, the loop continues.
- **model-failure**: a `fanout.CallOne` error aborts the loop → fallback
  (ADR-0014 §3), distinct from a tool failure.

### Statelessness + the load-bearing `-race` test (ADR-0021 §5)

`AgentBrain` holds NO per-call mutable state: the running `[]model.Message`, the
iteration counter, and each tool result are LOCALS in `Handle`; `req.Messages`
grows local to the call. **DRY:** the loop reuses `fanout.CallOne` for each model
call (inheriting panic-recover, per-call timeout, `%w` sentinel-grammar). The
mandatory `-race` test (`TestAgentBrain_Handle_concurrent_race`) runs `Handle`
concurrently on ONE `AgentBrain` whose registry holds a **stateful** fake tool
(mutex counter), proving the Tool concurrency contract and that loop state never
leaks between workers under `brainWorkers > 1` — the "intersection of two
features" bug class the project hit twice. Green under `-race -count=10`.

### Persistence — final pair only (ADR-0021 §6)

`AgentBrain` persists exactly the final user+assistant pair via
`Store.AppendTurns` on a cancellation-detached context (ADR-0019 §6), reusing the
same discipline as the Orchestrator. **The intermediate tool-use trace
(`TOOL:`/`OBSERVATION:`) is NOT persisted** — it is reasoning scratch, not
conversation; if ever wanted it is observability (Stage 12), not memory. A test
asserts that a Handle with two tool calls leaves EXACTLY two turns in the store.

### Wiring (ADR-0021 §1)

An optional `agent` block in `config.BrainConfig` (`tools`, `max_iterations`,
`system_prompt`). `buildBrain` now returns `brain.Brain` and mounts an
`AgentBrain` when the block is present, otherwise the Orchestrator. The agent is
single-model: exactly one model must survive selection (`ErrAgentModelCount`), and
each configured tool name resolves through `tool.Builtin` (`ErrUnknownTool` —
fail-loud safe-toolset boundary). `cmd/demo-agent` is a disposable live skeleton
against Groq.

## `/review` findings (1 P2 + 3 P3, all fixed)

An adversarial `/review` on the multi-step loop, the concurrency-safe tools, and
the parser found and we fixed:

- **P2 (`agent.go`)** — an empty (non-error) model reply was treated as a final
  answer, sending the user a blank message and persisting a one-sided user turn
  (asymmetric memory). Now degrades to the fallback; nothing is persisted.
- **P3 (`protocol.go`)** — a tool call wrapped in a single-line code fence
  (` ```TOOL: calc(2+2)``` `) was dropped as a fence delimiter. A pure-delimiter
  regex now distinguishes formatting noise from fenced payload.
- **P3 (`builtin.go` calc)** — added a hard input-length cap (bounds parser
  recursion depth by construction) and rejected non-finite results (overflow to
  ±Inf / NaN), tightening the bounded-arithmetic contract.

Reviewed and confirmed CLEAN: `req` aliasing / instance-field mutation under
`brainWorkers > 1`, loop boundedness, ReDoS (Go's RE2 is backtrack-free), and
context-cancel leaks.

## Deferred (recorded, not dropped)

- Provider-native structured function-calling — the sibling `ToolCallingModel`
  interface (own ADR, Context7-verified against Groq's tool-use contract).
- Multi-model agents (fan-out inside the loop) · planning (ReAct) · dangerous /
  side-effecting tools (shell, HTTP, fs, email, DB) · multi-agent · stateful
  budget/cost · a persisted tool-use trace (observability, not memory).
- **DRY debt (deliberate):** `AgentBrain.loadHistory` / `persistPair` mirror the
  Orchestrator's by design — this cut adds alongside the sibling rather than
  refactoring it (ADR-0021 §1, "no mutation of Orchestrator"). Unifying the two
  into a shared conversation-memory helper is a deferred refactor.

## Metrics

- `make quality` green with `-race`: gofmt, goimports, vet, golangci-lint
  (govet / staticcheck / errcheck / gosec). New-package coverage: `internal/tool`
  95.3%, `internal/brain` 94.5% (≥ 90% bar held). Total tree ≈ 91.6%.
- Cross-compile ×6 `CGO_ENABLED=0` green (linux / darwin / windows × amd64 /
  arm64).
- **`go.mod` stays at 3 direct dependencies** — the prompt-protocol + the bounded
  custom calc added NOTHING.
