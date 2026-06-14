# ADR-0009: Model interface — role-based message contract + hand-rolled Ollama adapter

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra

## Context

Stage 4 (Models) adds the abstraction every reasoning component in
Korvun will talk through to call an LLM, plus the first concrete
provider behind it. Phase 4.1 (this ADR) covers:

- The `model.Model` interface that providers implement.
- The first provider: a local Ollama server.
- A "live skeleton" — a tiny end-to-end demo that takes a Spanish
  prompt, hits a real Ollama-served model, prints the response.

Two later phases extend this work and are explicitly out of scope
for 4.1:

- Phase 4.2 adds a second provider (cloud — OpenAI / Anthropic /
  Vertex; chosen there).
- Phase 4.3 adds parallel fan-out across multiple providers
  (consensus / privacy / cost dispatch is policy-engine territory,
  Stages 5–6; 4.3 ships only the mechanism for fanning out).

### What is already on master that the design must mesh with

- The **router** (Stage 3, ADR-0003) consumes `brain.Brain` via
  `Handle(ctx, env) ([]*envelope.Envelope, error)`. Routing,
  backpressure, per-brain isolation, and shutdown all live there.
- The **Brain** (Stage 3 forward slice in `internal/brain/`) is the
  layer that will own *what model to call, with which message
  history, and how to fold the result back into an Envelope*.
  Stage 7 is when the real Brain ships; for the 4.1 live skeleton
  the Brain is either a trivial pass-through or the skeleton calls
  `Model` directly.
- The **Envelope** (Stage 1) is the canonical messaging payload —
  one *event* with sender, direction, parts. It is NOT a
  conversation history; nothing in 2.1–2E.8 modelled multi-turn
  state because that belongs to the Brain.
- The **channel adapters** (Stage 2 / Stage 2-EXT) translate native
  formats to and from `envelope.Envelope` as **pure converter
  functions**, then a thin transport implements `channel.Channel`.
  The Model side should reuse the same architectural shape rather
  than invent a new one.

### External-docs verification (per CLAUDE.md non-negotiable)

`github.com/ollama/ollama@v0.30.8` — the official Go module that
contains the `api/` package — was inspected via Context7 plus a
direct read of the v0.30.8 source under
`$GOPATH/pkg/mod/github.com/ollama/ollama@v0.30.8/api/`. Confirmed
facts that the design depends on:

- License: **MIT** (`LICENSE` in the module root). Compatible with
  Apache-2.0 redistribution; attribution required.
- Package surface:
  - `func ClientFromEnvironment() (*Client, error)` — honors the
    `OLLAMA_HOST` env var (and the `127.0.0.1:11434` default).
  - `func NewClient(base *url.URL, http *http.Client) *Client` —
    for callers that want to inject their own `*http.Client` and
    base URL.
  - `type ChatRequest struct { Model string; Messages []Message;
    Stream *bool; Format json.RawMessage; KeepAlive *Duration;
    Tools; Options map[string]any; Think *ThinkValue }`.
  - `type Message struct { Role string; Content string; Images
    []ImageData; ToolCalls []ToolCall; ToolName string; ... }`.
  - `type ChatResponse struct { Model; CreatedAt; Message; Done;
    DoneReason; Metrics ...; }`.
  - `type ChatResponseFunc func(ChatResponse) error`.
  - `func (c *Client) Chat(ctx context.Context, req *ChatRequest,
    fn ChatResponseFunc) error` — **the public signature is
    callback-based**. With `Stream` set to `*bool(false)` the
    callback is invoked exactly once with the full response; with
    `Stream` true (or nil → the documented default of true) it is
    invoked once per chunk.
- Wire endpoint: `POST {OLLAMA_HOST}/api/chat`, JSON
  request/response per `docs/api.md` of the project.
- Module direct dependencies in `github.com/ollama/ollama@v0.30.8`'s
  `go.mod`: 16 direct requires — gin, cobra, sqlite, go-sqlite3,
  tablewriter, protobuf, uuid, console, x/sync, x/sys, etc. Most
  of these support the binary server and CLI; the `api/` sub-
  package imports only intra-module `auth`, `envconfig`, `format`,
  and `version`. Importing `api/` still drags those four
  sub-packages and their own dependency closure into Korvun's
  `go.sum`.

### Structural questions to answer

Five decisions need to be pinned before any 4.1 code lands:

1. **Shape of the `Model` interface.** Does it speak in
   `envelope.Envelope` (the canonical Korvun message), or in its
   own role-based message type (the way LLM APIs natively think)?
2. **Response shape: full response vs streaming.** Phase 4.1 ships
   non-streaming. The interface must grow to streaming later
   without a breaking change.
3. **Context and cancellation.** Coherent with router / channel
   shutdown semantics.
4. **Ollama dependency.** Use the official Go client
   (`github.com/ollama/ollama/api`) or hand-roll `net/http` +
   `encoding/json` against `/api/chat`.
5. **Package layout.** Where do `internal/model`,
   `internal/model/ollama`, and (later) the cloud provider and the
   fan-out coordinator live?

## Decision

### 1. `Model` speaks its own role-based message type, not `envelope.Envelope`

`Model.Generate` accepts a `*model.Request` and returns a
`*model.Response`, where Request carries an explicit role-tagged
message slice (system / user / assistant) and Response carries an
assistant message plus provider metadata. The Brain is responsible
for translating `envelope.Envelope` to and from these types, the
same way `internal/channel/telegram` translates `models.Update`
to and from Envelope.

```go
package model

type Role int

const (
    RoleSystem Role = iota + 1
    RoleUser
    RoleAssistant
)

func (r Role) String() string { /* "system" / "user" / "assistant" / "unknown" */ }

type Message struct {
    Role    Role
    Content string
}

type Request struct {
    // Model is the provider-side model identifier, e.g. "llama3.2"
    // for Ollama, "gpt-4o" for OpenAI. Required.
    Model    string
    // Messages is the conversation as the caller wants the model
    // to see it. Order is preserved. System prompts (if any) come
    // first by convention but the Model adapter does not enforce.
    Messages []Message
}

type Response struct {
    // Message is the model's answer. Role is always RoleAssistant
    // on a successful response.
    Message Message
    // Provider is the canonical provider name (e.g. "ollama",
    // "openai") for log / metric attribution.
    Provider string
    // ModelName is the model that actually answered (echoed from
    // the provider; useful when a provider auto-routes).
    ModelName string
}

type Model interface {
    // Generate produces an assistant message from the given
    // conversation. ctx bounds the call; provider implementations
    // MUST propagate ctx to their underlying HTTP request.
    Generate(ctx context.Context, req *Request) (*Response, error)

    // Name returns the canonical provider name (e.g. "ollama").
    // Used by the future Registry and by error wrapping.
    Name() string
}
```

#### Why role-based and not `envelope.Envelope`

- **An LLM's mental model is a role-tagged conversation, not a
  messaging event.** Every major API (Ollama, OpenAI, Anthropic,
  Mistral, Vertex Gemini, the local OpenAI-compatible variants
  Ollama exposes itself) takes `messages: [{role, content}, ...]`
  as its primary input. Pushing the same shape down to the Model
  interface means every Korvun provider is a 1:1 translation away
  from its native wire format. Pushing `envelope.Envelope` down
  instead would force every provider to do the
  `Envelope → []Message` translation **plus** its own native
  conversion, and they'd all do it the same way.
- **`Envelope` has no concept of `system` role.** Envelope models a
  *messaging event* — one `Sender`, one `Direction`, one or more
  Parts. The "system prompt" of an LLM call is neither a sender
  nor a direction; it's a behavioural directive the operator
  injects. Forcing Envelope to carry it would either grow Envelope
  with concepts that don't belong there (and that ADR-0006 / 0007
  worked hard to keep out) or smuggle the system prompt through
  `Meta`, which is the wrong tool — `Meta` is for transport
  metadata, not for content with semantic role.
- **`Envelope` has no concept of multi-turn history.** Every prior
  ADR's exemplar — Telegram inbound `models.Update`, Telegram
  outbound `*bot.SendMessageParams` — is a single event. The
  `Brain` is the layer that decides whether to send 1 message
  (stateless) or the last 20 (chat with memory) to the Model.
  That decision is a Brain concern, not a transport concern.
- **Symmetry with the channel side, deliberately.** Channels:
  native ↔ Envelope is the converter boundary (purely functional,
  per Phases 2.3 / 2E.*). Brains: Envelope ↔ `[]model.Message` is
  the same kind of converter boundary, owned by Stage 7's real
  Brain. The Model adapter (`internal/model/ollama` etc.) only
  needs to know its own native shape ↔ `model.Message`, exactly
  as the Telegram adapter only knows `models.Update` ↔ Envelope.
- **One canonical shape across providers.** A second provider
  (4.2 cloud) and an N-way fan-out (4.3) consume `model.Model`.
  If Model spoke Envelope, the second provider's first job would
  be writing its own `Envelope → wire` translator with all the
  same edge cases the first provider had. If Model speaks
  `[]Message`, the per-provider translation is a thin
  role-to-string mapping.

#### Why `Generate` returns `*Response`, not `Message`

`Response` is a small struct on purpose: it carries the assistant
message **plus** `Provider` and `ModelName`. The fan-out work in
4.3 will want to label results by source ("which provider said
what") without re-threading that metadata through a side channel,
and a per-provider observability layer wants `Provider` for
log / metric attribution. Adding the wrapper now costs nothing and
removes a renaming churn later.

`Response` deliberately does NOT carry token counts, latency,
finish reason, or cost. Each is a real concern that 4.2 / 4.3 /
observability will surface; 4.1 stays minimal. The struct can grow
fields without breaking callers because every new field is
additive.

### 2. Phase 4.1 ships only `Generate`; streaming arrives as `GenerateStream` in a later phase

Phase 4.1 implements the synchronous `Generate(ctx, req)
(*Response, error)` only. Streaming is deferred without prejudice:
when a real consumer needs it (the voice loop, the partial-UI
case, or interactive chat in the no-code builder), a sibling
method is added:

```go
type Chunk struct {
    Delta    string // incremental assistant text
    Done     bool   // true on the terminal chunk
    Provider string // same as Response.Provider; convenient for fan-out
}

type StreamingModel interface {
    Model
    GenerateStream(ctx context.Context, req *Request, fn func(Chunk) error) error
}
```

`Model` remains the universal contract. Providers that support
streaming additionally satisfy `StreamingModel`. Callers that need
streaming type-assert to `StreamingModel` and fall back to
`Generate` otherwise. No existing call site breaks.

This decision is documented here, not implemented now: the
streaming method is a hypothesis until a real consumer exists.

#### Why a sibling method instead of a unified streaming-with-callback signature

The unified shape would be `Generate(ctx, req, fn func(Chunk)
error) (*Response, error)` with `fn == nil` meaning "no
streaming". It compiles for both cases but has three problems:

- **Every caller pays the cognitive cost of streaming**, even
  callers that just want one string. Phase 4.1's brains do not
  need streaming and shouldn't have to ignore a parameter to
  prove it.
- **The provider implementations get harder to test.** A
  conditional callback per-call is a different mock per test
  case; two methods are two test fixtures.
- **It pre-commits the `Chunk` shape now.** Phase 4.1 has no
  consumer of `Chunk`; designing its shape under that uncertainty
  is exactly the speculative API ADR-0006 / 0007 are careful to
  avoid.

### 3. `ctx` first, propagated all the way through the HTTP call

`Generate(ctx context.Context, req *Request)` — same shape as
`brain.Brain.Handle`, `channel.Channel.Send`, `router.Route`.
Cancellation of ctx aborts the in-flight HTTP request via
`http.NewRequestWithContext`; the Ollama adapter is responsible
for propagating ctx to the underlying `*http.Request`. A
configurable per-call timeout is layered on top via
`WithRequestTimeout(d)` (the Option pattern below); a request
without a configured timeout inherits its bound from the caller's
ctx alone.

This matches the surface ADR-0003 already chose for the router
side, so a brain handler whose ctx is cancelled propagates
cancellation into the Model call automatically.

### 4. Hand-rolled `net/http` + `encoding/json` for the Ollama adapter

The Ollama adapter is `internal/model/ollama`. It speaks directly
to `POST {baseURL}/api/chat` with `stream: false`, using stdlib
`net/http` + `encoding/json`. **No external dependency added.**

#### Why hand-rolled and not `github.com/ollama/ollama/api`

This is the inverse of ADR-0001's decision for Telegram. Three
axes drive the asymmetry:

| Axis | Telegram (ADR-0001) | Ollama (this ADR) |
|---|---|---|
| Endpoints Korvun needs | ~30 across the Bot API | 1 (`/api/chat`); maybe 1 more (`/api/tags`) when we list models |
| Type surface to maintain | dozens of `models.*` structs that mirror the Bot API | 3 structs: ChatRequest, Message, ChatResponse — and we only use the `role`, `content`, `model`, `message` fields |
| API volatility | Bot API ships breaking-ish minor versions every quarter (10.0 today, was 7.x last year) | `/api/chat` is stable; Ollama adds OpenAI-compatible endpoints rather than churning the native one |
| Module dep footprint | `go-telegram/bot`: 0 transitive deps, advertised invariant | `github.com/ollama/ollama@v0.30.8`: 16 direct deps in `go.mod` (gin, cobra, sqlite, …) for the binary + tooling; importing `api/` drags the intra-module sub-packages and their own closures |
| Korvun's hand-roll cost | High: Bot API schema is large + moving target | **Low**: 1 endpoint, 3 fields per struct, stable shape |
| Stdlib sufficiency | Marginal: requires re-implementing the Bot API model | **Trivial**: `http.NewRequestWithContext` + `json.Marshal` + `json.NewDecoder.Decode` |
| Korvun's "minimal supply chain" principle | Argued for adopting `go-telegram/bot` (one dep, zero transitives) | Argues for **avoiding** the Ollama module (one direct dep, many transitives) |

The decisive axis is the **shape of the dependency cost vs the
shape of the maintenance cost**. For Telegram the dep is cheap
and the hand-roll is expensive. For Ollama the dep is heavy and
the hand-roll is one short file. Picking the cheaper option each
time keeps the principle consistent ("prefer the maintained binding
when it pays for itself; prefer stdlib when it doesn't") even
though the headline choice flips.

A second reason: keeping the Ollama adapter at stdlib lets it
serve as the **reference shape** for the cloud provider adapter
in 4.2. OpenAI and Anthropic both have official Go SDKs but also
have small enough HTTP+JSON surfaces that 4.2 can credibly make
the same choice. ADR-0009 does not pre-decide 4.2 — it just makes
sure 4.1 is not the one driving 4.2 toward a particular dep.

#### What hand-rolling actually means

The adapter is one file (`ollama.go`) plus a test file. The
internal request/response structs are unexported and mirror only
the fields Korvun reads:

```go
type chatRequest struct {
    Model    string         `json:"model"`
    Messages []chatMessage  `json:"messages"`
    Stream   bool           `json:"stream"`
}
type chatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
type chatResponse struct {
    Model   string      `json:"model"`
    Message chatMessage `json:"message"`
    Done    bool        `json:"done"`
}
```

The forward-compat promise of JSON (unknown fields ignored) means
new Ollama response fields don't break us. Tests use
`httptest.NewServer` with a canned JSON body — no real Ollama
needed for unit tests; the live skeleton is the only piece that
talks to a real server.

### 5. Package layout: `internal/model/` + `internal/model/ollama/`, mirroring `internal/channel/`

```
internal/
  model/
    doc.go           # package doc + canonical name constant if needed
    model.go         # Role, Message, Request, Response, Model interface
    errors.go        # sentinel errors (ErrEmptyMessages, etc.)
    model_test.go    # interface-level tests via in-test fake
  model/
    ollama/
      doc.go         # package doc + DefaultBaseURL constant
      ollama.go      # Adapter, New, Options, Generate
      ollama_test.go # httptest-based fake-Ollama tests
```

Notes:

- Singular `internal/model/` matches `internal/channel/`'s
  singular path (post-ADR-0008 rename).
- Phase 4.1 does **not** ship a Registry. The Registry only
  earns its keep when 4.3 fan-out needs to address multiple
  providers by name; until then a Brain holds one `Model` field
  and that's enough. Adding a Registry preemptively would be the
  same "speculative API" mistake §2 warns against.
- The cloud provider adapter in 4.2 lands as
  `internal/model/<provider-name>/` next to ollama. The exact
  provider is decided in ADR-0010.

## The 4.1 plan — what implementation will look like after this ADR

Three commit-shaped pieces of work:

1. **`internal/model` types + interface (red→green).**
   `Role`, `Message`, `Request`, `Response`, `Model` interface,
   sentinel errors. Tests cover Role.String, basic builder /
   round-trip, a no-op fake Model satisfying the interface.

2. **`internal/model/ollama` adapter (red→green).**
   `Adapter` struct with `WithBaseURL(string)`,
   `WithHTTPClient(*http.Client)`, `WithRequestTimeout(d)`
   options. `New(opts ...Option) *Adapter`. `Name()` returns
   `"ollama"`. `Generate(ctx, req)` performs the POST against
   `/api/chat` with `stream: false`, validates the response,
   returns `*model.Response{Message, Provider: "ollama",
   ModelName: ...}`. Tests use `httptest.NewServer` with a
   canned response, cover: happy path, ctx cancellation, non-2xx
   status, malformed JSON, empty messages refused upstream,
   ModelName echoed.

3. **Live skeleton: `cmd/demo-model/main.go` (temporary).**
   Reads `OLLAMA_HOST` (default `http://127.0.0.1:11434`),
   reads `KORVUN_DEMO_MODEL` (default `llama3.2`), reads the
   prompt from `os.Args[1]` or stdin, calls the adapter, prints
   the assistant content. Marked clearly as a temporary
   end-to-end harness; deleted (or rewritten as a real
   integration test) when Stage 5+ ships the main bootstrap.
   This is not a unit test; it is a "did the wiring actually
   work against a real Ollama" check, run manually by the
   operator (or by CI if we ever decide to spin up an Ollama
   container).

`make quality` green over the whole tree before closing the
phase. Coverage target: ≥90 % for `internal/model` (small
package, easy to cover) and ≥90 % for `internal/model/ollama`
(the httptest fake covers everything that matters).

## Consequences

### What this enables

- Stage 7's Brain can be written against `model.Model` with no
  knowledge of which provider answers. The same Brain composes
  with Ollama today, OpenAI/Anthropic tomorrow, and a fan-out
  meta-provider in 4.3.
- Phase 4.3's fan-out is straightforward: a `MultiModel` type
  that itself implements `model.Model`, dispatches the request
  to its children in parallel, collapses the responses according
  to whatever policy the Brain hands it. The children are
  ordinary `model.Model` values. No special-casing.
- Stage 5–6's policy engine reads provider names off
  `Response.Provider` for cost / privacy attribution without
  needing to inspect the adapter's internals.
- The live skeleton makes "is Ollama wired correctly on this
  machine" a one-command check independent of the rest of
  Korvun, which is what a stage closure of "live skeleton"
  should be.

### What it asks of the Brain

- The Brain owns the **`Envelope → []model.Message`** mapping.
  At minimum: the inbound text becomes one `RoleUser` Message;
  an operator-configured system prompt (if any) becomes a
  leading `RoleSystem` Message; multi-turn history (if the
  Brain keeps any) is interleaved. Stage 7 specifies the exact
  shape.
- The Brain owns the **`Response → []*envelope.Envelope`**
  mapping. The assistant content becomes an outbound Envelope's
  text Part with `Direction = Outbound`, `Sender` set to the
  bot's identity.
- The Brain decides **which model name to call** and how to
  derive it (config, per-conversation, per-policy).

None of these decisions live in the Model adapter; this ADR is
the contract that frees Stage 7 to make them.

### What this does NOT do

- **No second provider.** That is Phase 4.2.
- **No fan-out.** That is Phase 4.3.
- **No streaming.** Deferred per §2 with the contract for how it
  enters cleanly when needed.
- **No Registry.** Deferred to 4.3.
- **No embeddings, no tool calling, no vision.** Each is a real
  capability some provider exposes; none drives 4.1's
  minimal-shape decision. They land as additional methods on a
  capability-specific interface when a real caller appears (e.g.
  `Embedder`, `ToolUser`, `VisionModel`).
- **No `Model` configuration at runtime beyond what
  `Request.Model` carries.** Provider-specific tuning
  (temperature, top_p, num_ctx, system-of-units) is deferred to
  a `Request.Options map[string]any` if and when a Brain needs
  to expose it. 4.1 ships without it.

### Trade-offs accepted

- **Two formats in the codebase (`Envelope` and
  `model.Message`).** Acknowledged. The justification is in §1
  decision: forcing one format on both messaging and LLM domains
  conflates two genuinely different concepts. The translation
  glue lives in the Brain, the same way the Telegram converter
  glue lives in the channel adapter.
- **Hand-rolled HTTP client for Ollama.** Acknowledged that
  this inverts ADR-0001's choice for Telegram; §4 argues the
  inversion is principled, not capricious — the dep cost / hand-
  roll cost trade-off lands differently for each.
- **No streaming in 4.1.** A future caller who needs streaming
  will pay the type-assertion cost (`m, ok :=
  model.(StreamingModel)`). Cheap, idiomatic Go.
- **`Response.Provider` is duplicated with `Model.Name()`.**
  Deliberate: a fan-out result that loses its source provider is
  worse than a one-string redundancy in the result struct. The
  alternative (lookup-by-pointer) couples the fan-out caller to
  the adapter graph.
- **The live skeleton is a CLI demo, not a Go test.** Verifying
  end-to-end requires a real Ollama running locally; trying to
  shoehorn that into `go test` adds a `//go:build integration`
  gate and a CI conditional skip. A `cmd/demo-model` binary is
  simpler, deletable, and matches the project's "live skeleton"
  language for stage closure.

## Alternatives Considered

### Interface shape — A1: `Model` takes and returns `envelope.Envelope`

`Generate(ctx, *Envelope) (*Envelope, error)`. One canonical type
everywhere; no glue between layers.

**Rejected** for the four reasons in §1: Envelope lacks `system`,
lacks conversation history, conflates messaging metadata with
content, and would force every provider to redo the same
Envelope→wire translation.

### Interface shape — A2: free-form `string` prompts

`Generate(ctx, model string, prompt string) (string, error)`.
Pragmatic for a quick proof-of-concept.

**Rejected.** Loses the system / user / assistant distinction
that every provider uses internally anyway, so the first multi-
turn caller (Stage 7's real Brain) would either re-encode roles
into a single string with magic prefixes ("[SYSTEM] …") or
build the role list right after the call — both regressions
versus making roles a first-class part of the contract.

### Streaming — B1: unified `Generate(ctx, req, fn) (*Response, error)`

One method, optional callback.

**Rejected** for the three reasons in §2: every caller pays
streaming's cognitive cost, providers double their test surface,
and the `Chunk` shape gets pre-committed before any consumer
exists.

### Streaming — B2: ship streaming now in 4.1

Define `Chunk` today; both providers implement streaming from
the start.

**Rejected.** YAGNI: 4.1's only consumer is the live skeleton,
which wants one printable line. Streaming has its own concerns
(backpressure, partial decode, completion sentinel) that the
ADR would have to design without a real driving use case. The
sibling-interface plan in §2 keeps the door open at zero cost.

### Ollama dependency — C1: adopt `github.com/ollama/ollama/api`

Use the official Go client.

**Rejected** per §4's three-axis comparison. The official
binding's strengths (typed structs, maintained signatures) are
real but solve a problem Korvun doesn't have: maintaining a
large native schema. The cost (16 direct deps in the consumed
module, plus the four intra-module sub-packages and their
closures) is real and SBOM-visible.

Re-evaluable if any of the following changes:

- Ollama adds a complex tool-calling protocol Korvun wants to
  expose verbatim, and the hand-rolled struct count crosses some
  reasonable threshold (say, 10).
- Korvun starts using two or more Ollama endpoints whose
  request/response shapes are non-trivial (e.g. structured
  outputs, embeddings with multiple variants).
- The official client materially shrinks its dep footprint
  (a sub-module with a slim closure).

Documented here so a future ADR can flip the decision with the
evidence in hand.

### Ollama dependency — C2: third-party Go client wrappers

Several community projects exist (`tmc/langchaingo`,
`parakeet-nest`, etc.).

**Rejected.** Supply-chain risk (single-maintainer projects,
slower release cadence than the official client), and they all
either wrap the official client (adding a layer) or re-implement
HTTP+JSON (which we can do ourselves with no abstraction tax).
For the cost of pulling one of them in we'd rather pay the
trivial hand-roll cost.

### Package layout — D1: flat `internal/llm/`

Single package containing the interface and every provider.

**Rejected.** Mixing the interface contract with concrete
adapters makes the boundary harder to police — and the
`internal/channel` precedent (one package for the interface,
sub-packages for the adapters) already proved itself in Phase
2-EXT. Same shape here for the same reasons.

### Package layout — D2: each provider as a top-level
`internal/<provider>/` (no shared `internal/model/`)

`internal/ollama/`, `internal/openai/`, …; the interface lives
in `internal/brain/` or wherever it is consumed.

**Rejected.** Loses the explicit "every provider implements one
contract" signal that having `internal/model/model.go` provides.
Future-readers (and the future-Brain author) lose a single place
to consult for "what is a Model?".

## Open follow-ups (not blockers for Phase 4.1)

- **Embeddings interface.** Several providers offer
  embeddings; defining an `Embedder` interface alongside `Model`
  is the natural shape, deferred until a caller needs it
  (probably the RAG layer in a later stage).
- **Tool-calling.** Same shape consideration; deferred until the
  Brain has a concrete tool to call.
- **`Request.Options`.** A `map[string]any` of provider-tuning
  knobs (temperature, top_p, num_ctx, …) lands the first time
  a Brain needs to expose any.
- **Provider Registry.** Lands with 4.3 fan-out, sized to that
  consumer's needs.
- **`Response` metadata fields.** Token counts, latency, finish
  reason, cost estimate. Each lands when a real consumer
  (observability stage, cost-policy stage) reads it.
- **Streaming.** Lands behind `StreamingModel` when a real
  consumer needs partial outputs — voice loop or interactive
  chat in the no-code builder.
- **Conversion of the live skeleton into a CI smoke test.**
  Requires deciding how Korvun runs Ollama in CI (containerised
  or skipped). Out of scope for 4.1; the cmd binary is enough.