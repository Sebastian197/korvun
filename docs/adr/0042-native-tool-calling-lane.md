# ADR-0042: The native tool-calling lane — `ToolCallingModel` as a sibling interface, Ollama first, governance intact

> **Status:** accepted
> **Date:** 2026-08-09
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0021](0021-agents.md) §3.4 (the deferred native path,
> framed as a sibling capability interface — the ADR-0009 §2 StreamingModel
> precedent), [ADR-0041](0041-governed-tools-shadow-shield-skills.md) (the
> gate, shadow, shield, cages, audit — all of which this lane must preserve
> UNCHANGED), [ADR-0031] (the retry decorator owns the per-attempt deadline).
> Trigger: the 2026-08-09 live demo failure, quantified by the isolation
> probe (raw model output, 3 runs per condition, llama3.2 3B): the
> prompt-protocol yields the `TOOL:` grammar **1/3** on a clean context and
> **0/3** with a poisoned history in front (the model imitates prior bad
> turns); the SAME model through Ollama's native tools API calls the tool
> **3/3** with correct arguments. The prompt-protocol's documented
> fragility is a measured beta blocker for local models; the native lane is
> the measured cure. (The session-reset cut itself was verified sound at
> three levels — the tripwire tests pin it.)

## External-docs verification (per CLAUDE.md non-negotiable — done FIRST)

Verified via Context7 (`/ollama/ollama`, docs/api.md + api/types.go +
docs/capabilities/tool-calling.mdx) on 2026-08-09:

- **Request** (`POST /api/chat` — the endpoint Korvun's adapter ALREADY
  uses): `"tools": [{"type": "function", "function": {"name",
  "description", "parameters": <JSON Schema object>}}]`.
- **Response**: `message.tool_calls: [{"function": {"name": ...,
  "arguments": {…}}}]` — arguments is a JSON OBJECT (the OpenAI-compat
  endpoint stringifies it; the native endpoint does not).
- **Result cycle**: append the assistant turn (carrying its `tool_calls`)
  plus one `{"role": "tool", "content": <result>, "tool_name": <name>}`
  message per result, then call again with the SAME `tools`.
- **Model capability**: verified on the target machine itself —
  `ollama show llama3.2` (3B, already pulled on the operator's iMac) lists
  `Capabilities: completion, tools` (so does 1b, for what its size is
  worth). No new model download is required for the demo.

Zero new dependencies: the lane extends the existing hand-rolled
HTTP+JSON adapter.

## Decision

### 1. `ToolCallingModel` — the sibling interface (never a widening)

```go
package model

// ToolSpec advertises one tool to a native tool-calling model. The v1
// parameter schema is UNIFORM: every tool takes a single string argument
// "args", described by the tool's own Description (which already documents
// the format — "args = the URL", "args = the expression"). The Tool seam's
// Execute(ctx, args string) contract stays untouched; richer per-tool
// schemas are an additive future extension.
type ToolSpec struct {
    Name        string
    Description string
}

// ToolCall is one native tool request from the model. Arguments carries the
// raw JSON object the provider returned.
type ToolCall struct {
    Name      string
    Arguments map[string]any
}

// ToolCallingModel is the native-lane capability interface (ADR-0021 §3.4,
// the StreamingModel precedent): providers that support structured tool
// calling ALSO satisfy it. model.Model is never widened.
type ToolCallingModel interface {
    Model
    GenerateWithTools(ctx context.Context, req *Request, tools []ToolSpec) (*Response, error)
}
```

### 2. Additive DTO growth (the Message transport)

The verified cycle needs two things `model.Message` cannot carry: the
assistant turn's `tool_calls` and the `role:"tool"` result turns. The DTO
grows ADDITIVELY — `RoleTool`, `Message.ToolCalls []ToolCall` and
`Message.ToolName string` (both zero by default). This does NOT widen the
`model.Model` interface (the prohibition); existing adapters build their
wire structs reading only Role/Content, so zero-values are invisible to
every existing request — pinned by tests.

### 3. The Ollama adapter implements the lane

`GenerateWithTools` extends the existing `/api/chat` wire structs:
`tools` on the request (the uniform `{"args": string}` schema built from
each ToolSpec), `tool_calls` parsed from the response message, and the
serialization of RoleTool/ToolName/ToolCalls turns per the verified
contract. Groq (OpenAI-compat) is a follow-up, NOT this cut — one provider
proves the lane (the ADR-0015 discipline of validating against the
concrete shape first).

### 4. The retry decorator PROPAGATES the capability

`retry.Wrap` returns a wrapper that also satisfies `ToolCallingModel`
IF AND ONLY IF the wrapped model does (two wrapper types, chosen at Wrap
time — the http middleware/Flusher pattern), applying the SAME retry
policy to `GenerateWithTools`. A capability must not vanish inside a
decorator, and the native lane must not silently lose retry.

### 5. The AgentBrain picks the lane — governance IDENTICAL by construction

At Handle time: if the brain's model satisfies `ToolCallingModel`, the
native loop runs; otherwise today's prompt-protocol loop runs untouched
(graceful degradation, nothing existing breaks).

The native loop:

- **advertises** exactly the gate's advertised registry (allow ∪ shadow)
  as ToolSpecs — a denied tool is never announced (ADR-0041 §2 stands);
- the system prompt keeps persona + operator prompt + skills block but
  DROPS the `TOOL:`/`OBSERVATION:` grammar (the structure replaces it);
- each returned tool_call routes through the SAME `runTool` as the
  prompt-protocol lane — so the two-point gate, shadow's
  announced-never-executed simulation text, the cages, the dial-time
  shield, and the three-surface metadata-only audit are IDENTICAL BY
  CONSTRUCTION, not re-implemented;
- the tool result (or simulation/denial observation) returns as a
  RoleTool message with ToolName, per the verified contract;
- args extraction: `Arguments["args"]` when it is a string (the uniform
  schema); otherwise the whole object re-serialized as compact JSON — the
  tool still owns parsing, a mismatch is an ordinary tool error;
- the iteration cap, per-tool timeout, model-failure→fallback, and
  final-pair-only persistence all stand unchanged (ADR-0021 §2/§6).
- Multiple tool_calls in one reply are processed IN ORDER, sequentially,
  each through runTool — parallel execution is a deferred refinement.

## Consequences

- Local models with the `tools` capability (llama3.2 included) drive the
  governed toolset reliably; the prompt-protocol remains the fallback for
  models without it.
- A model without native support keeps working exactly as today — the
  assertion simply fails and the old lane runs.
- Groq's native lane is a small follow-up behind the same interface.

## Alternatives considered

- **Widening model.Model / Request / Response** — rejected again
  (ADR-0009 §2, ADR-0021 D1): the sibling interface keeps every non-agent
  consumer untouched.
- **Per-tool JSON Schemas in v1** — rejected for this cut: the uniform
  `{"args": string}` schema preserves the Tool seam verbatim and needs no
  per-tool surface; rich schemas are additive later.
- **Unwrap() on the retry decorator** — rejected: it would run native
  calls UNDECORATED, silently losing retry on the new lane.
- **Parallel tool_call execution** — deferred: sequential-in-order is
  correct and simple; parallelism is an optimization with concurrency
  hazards the minimal cut does not need.
