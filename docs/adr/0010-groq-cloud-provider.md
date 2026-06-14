# ADR-0010: Groq cloud provider — hand-rolled OpenAI-compatible client, env-only API key, cloud-shaped error mapping

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra
> **Amends:** [ADR-0009](0009-model-interface-and-ollama.md) (additively — no breaking changes to the `Model` interface; adds two new sentinels to `internal/model`)

## Context

Phase 4.2 adds the **second** implementation of `internal/model.Model`:
a cloud provider, Groq. Groq was chosen because it (a) speaks the
**OpenAI-compatible** chat-completions endpoint (so the wire shape
is the same most cloud LLM providers expose), (b) has a real free
tier suitable for development without a credit card on file, and
(c) inference is fast enough that the live-skeleton round-trip
stays comfortable.

The point of 4.2 — bigger than "ship Groq" — is to **stress-test
the `Model` interface against a provider whose nature is different
from Ollama**:

| Axis | Ollama (4.1) | Groq (4.2) |
|---|---|---|
| Locality | Local server on the operator's machine | Public HTTPS API |
| Auth | None | Bearer API key |
| Rate limits | None (whatever the machine can serve) | Hard caps per RPM / RPD / TPM / TPD |
| Failure modes the Brain has to handle | network down, model not pulled, OOM | network down, 401 invalid key, 429 quota exhausted, 5xx provider |
| Cost model | Free (electricity) | Free tier with hard ceiling; paid above |

If `model.Model` survives 4.2 unchanged, it has earned its keep.
If something has to flex, this is the right moment to learn that.

### What is already on master that this design must mesh with

- `internal/model.Model` — `Generate(ctx, *Request) (*Response, error)` + `Name() string`.
- `internal/model.Request{Model, Messages}`, `model.Response{Message, Provider, ModelName}`.
- `model.Role{System, User, Assistant}` with `Role.String()` producing the lowercase form every OpenAI-compatible API expects.
- `model.ValidateRequest` — the universal upstream invariant check (non-nil request, non-empty Model, ≥1 Message, each Message with a recognised Role and non-empty Content). Adapters call it as the first thing in Generate.
- Sentinels: `ErrNilRequest`, `ErrEmptyModel`, `ErrEmptyMessages`, `ErrInvalidRole`, `ErrEmptyContent`, `ErrProviderUnavailable`, `ErrProviderResponse`.
- Existing adapter: `internal/model/ollama` (hand-rolled `net/http` + `encoding/json`, the same pattern this ADR proposes for Groq).
- Live skeleton: `cmd/demo-model` — temporary CLI, marked deletable in Stage 5+.

### External docs verification (per CLAUDE.md non-negotiable)

The Groq API surface and behaviour relevant to 4.2 were verified via
Context7 (`/websites/console_groq`) plus targeted WebFetch + WebSearch
of the live `console.groq.com/docs` site at the day of writing
(2026-06-14). Verified facts that this design depends on:

- **Endpoint:** `POST https://api.groq.com/openai/v1/chat/completions`.
  The `/openai/v1` prefix is intentional and stable — Groq exposes
  the OpenAI-compatible surface as its primary API; native Groq
  endpoints live at `/v1/` but the chat completions path Korvun
  needs is the OpenAI-compatible one. Verified against the Groq
  rate-limits, models, errors, and tool-use docs (all curl examples
  point at the same `/openai/v1/chat/completions` URL).
- **Headers:** `Content-Type: application/json` + `Authorization:
  Bearer $GROQ_API_KEY`. (Lowercase `authorization: bearer ...` also
  works per Groq's own examples; HTTP headers are case-insensitive.)
- **Request body** (subset Korvun sends): `{model: string, messages:
  [{role: string, content: string}, ...], stream: false}`. Same
  shape as Ollama plus the field-name `stream` (no `Stream` upper
  case) common to OpenAI-compatible APIs.
- **Response body** (subset Korvun reads): `{id, object:
  "chat.completion", created, model, choices: [{index, message:
  {role: "assistant", content}, logprobs, finish_reason}], ...}`.
  The OpenAI-shaped `choices[0].message.content` is where the
  assistant text lives — different from Ollama's
  `response.message.content`.
- **Error body** (consistent across 4xx and 5xx): `{error: {message:
  string, type: string}}`. The `type` field is a short
  classification like `"invalid_request_error"`.
- **Rate-limit headers** (always present, regardless of status):
  `x-ratelimit-limit-requests`, `x-ratelimit-limit-tokens`,
  `x-ratelimit-remaining-requests`, `x-ratelimit-remaining-tokens`,
  `x-ratelimit-reset-requests`, `x-ratelimit-reset-tokens`.
- **429-only header:** `retry-after` (seconds).
- **Free-tier models confirmed available on 2026-06-14**:
  - `llama-3.1-8b-instant` — 30 RPM, 14,400 RPD, 6,000 TPM,
    500,000 TPD. The most permissive free-tier model. Recommended
    for the live skeleton.
  - `llama-3.3-70b-versatile` — 30 RPM, 1,000 RPD, 12,000 TPM,
    100,000 TPD. Lower daily quota but a much stronger model.
  - `openai/gpt-oss-20b` and `openai/gpt-oss-120b` listed in
    Groq's models doc; tier eligibility per-organisation.
  - Deprecated as of 2025-08-30 (do NOT use): `llama3-70b-8192`,
    `llama3-8b-8192` — replaced by the `3.3-70b-versatile` /
    `3.1-8b-instant` pair above.
- **Official Go SDK status:** **none**. Groq's "Client Libraries" doc
  page lists only `groq-python` and `groq-typescript` as official.
  Community Go libraries exist (`jpoz/groq`, `hasitpbhatt/groq-go`,
  `connerohnesorge/groq-go`), all unofficial, none of them with
  tagged releases at the time of writing.

### Structural questions to answer

1. **Does `model.Model` survive Groq unchanged?** If not, what
   exactly needs to flex, and why.
2. **What does Korvun depend on to talk to Groq?** Official SDK
   (doesn't exist for Go), a community library, a generic
   OpenAI-compatible Go client, or hand-rolled like Ollama.
3. **How does Korvun handle the API key — at the source level, at
   the build level, and at runtime?** Strictly: env var only, never
   the repo, never a committed config file.
4. **How do cloud-specific failures (401, 429, quota exhausted)
   surface to the Brain?** Re-use existing `model.Err*` sentinels,
   or grow the sentinel set with cloud-shaped ones?
5. **Which Groq model does the live skeleton hit by default**, given
   the free-tier rate-limit shape?

## Decision

### 1. `model.Model` survives Groq unchanged. The cloud-shaped error categories grow the sentinel set in `internal/model` additively.

Groq fits the existing `Generate(ctx, *Request) (*Response, error)`
shape exactly: the request is a `model.Request` with the same
`Model` + `Messages` slice, the response is a `model.Response` with
`Message` (the assistant turn), `Provider = "groq"`, `ModelName`
echoed from the Groq response. The role-tagged conversation model
matches the OpenAI-compatible wire shape 1:1 — no glue beyond
mapping `model.Role.String()` to a JSON string field.

**Therefore the `Model` interface does not change.** The Brain layer
written against `model.Model` from 4.1 calls a Groq adapter and an
Ollama adapter identically.

What does grow — additively, no break — is the sentinel set in
`internal/model`. Cloud providers have two failure categories
local-only providers do not, and squeezing them into the existing
`ErrProviderUnavailable` / `ErrProviderResponse` pair would force
the future fan-out / retry policy (Phase 4.3, then Stages 5–6) to
inspect error strings to recover the distinction. The clean answer
is to name them at the sentinel layer:

```go
// internal/model/errors.go (additions)

// ErrAuthInvalid wraps a provider rejection caused by missing or
// invalid credentials (e.g. HTTP 401, "Invalid API Key"). Distinct
// from ErrProviderUnavailable because it is permanent until the
// caller fixes config — a retry will not help.
var ErrAuthInvalid = errors.New("model: provider authentication failed")

// ErrRateLimited wraps a provider rejection caused by the caller
// exceeding a quota (e.g. HTTP 429). Recoverable by waiting; a
// future fan-out / retry policy can read it via errors.As against
// the *RateLimitError type below to recover the RetryAfter hint.
var ErrRateLimited = errors.New("model: provider rate-limited the caller")

// RateLimitError is the concrete error type returned when a provider
// signals a rate-limit hit. Provider identifies the source (handy
// when a fan-out sees several errors from different providers);
// RetryAfter is the suggested wait (zero when the provider did not
// advise one). Wraps ErrRateLimited so errors.Is(err, ErrRateLimited)
// keeps working without callers having to know the concrete type.
type RateLimitError struct {
    Provider   string
    RetryAfter time.Duration
}
func (e *RateLimitError) Error() string { ... }
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }
```

Both new sentinels live in `internal/model` (the package every
adapter already imports), NOT in `internal/model/groq`. Justification:

- They are universal cloud-provider categories. The next provider
  Korvun adds (OpenAI direct, Anthropic, Vertex Gemini, Mistral)
  will surface the same two on its own 401 / 429 paths. Keeping
  them in `model` means every adapter speaks the same vocabulary
  to the Brain, which is what the abstraction is for.
- They are **additions, not changes.** Existing 4.1 code that
  reads `ErrProviderUnavailable` / `ErrProviderResponse` continues
  to work; the Ollama adapter is not edited by this commit.
- The fan-out work in 4.3 has a clean answer to "should I retry?":
  - `ErrRateLimited` (with `RetryAfter`) → yes, after a wait.
  - `ErrProviderUnavailable` (transient transport) → yes, soon.
  - `ErrAuthInvalid` → no, surface to operator.
  - `ErrProviderResponse` (bad request shape, malformed response,
    empty assistant content) → no, this is our bug, not the
    provider's.

#### Why these two specifically, and not also `ErrQuotaExhausted`, `ErrModelNotFound`, etc.

Constraint discipline. Both `ErrAuthInvalid` and `ErrRateLimited`
have **distinct recovery semantics** the Brain cares about. The
others either don't (a 404 on model-not-found is a config bug just
like a malformed request — `ErrProviderResponse` covers it) or
don't exist in the free-tier API surface (Groq returns 429 on quota
exhaustion regardless of whether it's per-minute or per-day; the
distinction is in the headers, not the status code). Sentinels that
do not change retry behaviour earn nothing by being a separate
identity. We can add more later when a real consumer reads them.

### 2. Hand-rolled `net/http` + `encoding/json` for the Groq adapter, mirroring `internal/model/ollama`

Same shape as ADR-0009 §4 chose for Ollama, for the same kind of
reasons evaluated against today's Groq landscape. The full table:

| Axis | Ollama (ADR-0009) | Groq (this ADR) |
|---|---|---|
| Endpoints Korvun needs | 1 (`/api/chat`) | 1 (`/openai/v1/chat/completions`) |
| Type surface to maintain | 3 unexported structs (3-4 fields each) | 3 unexported structs (3-5 fields each, including `choices[0].message.content` nesting) |
| API volatility | Stable native endpoint | Stable OpenAI-compatible endpoint (Groq has every incentive not to break OpenAI shape) |
| Official Go SDK | Exists (`github.com/ollama/ollama/api`) but drags 16 direct deps via the parent module | **Does not exist.** Groq publishes Python + JS/TS only |
| Community Go libs | n/a (official available) | `jpoz/groq` (MIT, zero deps, **no tagged releases, 2 commits on master** — early-stage), `hasitpbhatt/groq-go`, `connerohnesorge/groq-go` — none mature |
| OpenAI-compatible Go SDKs | n/a | e.g. `sashabaranov/go-openai`. Real library, broad surface, used by many projects — but its scope (every OpenAI endpoint: embeddings, assistants, audio, fine-tuning, batch, files, moderations, …) is dozens of types Korvun does not need, plus its own dep graph |
| Korvun's hand-roll cost | Low | **Low** — slightly higher than Ollama because of `choices[0].message.content` nesting and the error envelope, but still under 100 LOC |
| Stdlib sufficiency | Trivial | Trivial |
| Minimal supply chain | Argued against the official Ollama module | Same argument, even stronger: **the only "official" path doesn't exist for Go**, the community alternatives are nascent, and the OpenAI-compatible SDKs solve a much bigger problem than Korvun has |

Decision is identical to ADR-0009: **hand-rolled, stdlib-only,
under `internal/model/groq/groq.go`**. The cumulative effect across
4.1 + 4.2 is that every Korvun model adapter ships zero external
dependencies beyond what's already in `go.mod`. Cloud expansion
later (Anthropic, OpenAI direct, Vertex) is the right moment to
re-evaluate per-provider — small and stable endpoints stay
hand-rolled; complex or volatile ones earn a binding.

#### Rejected: `jpoz/groq` community library

MIT-licensed, zero dependencies (good), but **no tagged release**
and only 2 commits on `master` as of 2026-06-14. Supply-chain
risk: a project this early-stage might be abandoned at any time,
its master HEAD might break API at any commit, and Korvun's
production guarantee should not depend on a hobbyist project with
no release discipline. If `jpoz/groq` (or a successor) reaches
v1.0 with sustained activity and Korvun adds Groq-specific
endpoints beyond chat completions, this is reevaluable then.

#### Rejected: `sashabaranov/go-openai` (or any OpenAI-compatible Go SDK)

A real, well-maintained library with broad community use. Its scope
is much larger than Korvun's: it models every OpenAI endpoint
(embeddings, assistants, audio, fine-tuning, batch, files,
moderations, …) as dozens of typed structs, plus its own
dependency graph. Importing it to call ONE endpoint is the same
mistake ADR-0009 §4 rejected for the official Ollama client: the
binding's cost is real, the maintenance saved is small (because we
were only going to call one endpoint anyway). Re-evaluable when
Korvun consumes ≥3 OpenAI-shape providers via the same SDK.

### 3. API key — environment variable only, never the repo, never a committed file

This is a **non-negotiable principle**, recorded at the ADR layer so
no shortcut takes it down later.

**The key never touches the repository.** Not in source, not in
`go.mod`, not in a `config.yaml`, not in a `.env.example` with the
real value, not in a comment, not in a test fixture, not in a docs
sample. The repo carries the **placeholder name** (`GROQ_API_KEY`)
and that is the only thing about the key that is ever committed.

The adapter resolution chain mirrors Ollama's `OLLAMA_HOST`:

1. `WithAPIKey(string)` — if used, wins. Production callers that
   load secrets from a vault or a secret-manager pass the value
   here.
2. `os.Getenv("GROQ_API_KEY")` — default. This is the chain that
   the live skeleton + every dev-on-laptop flow uses.
3. Empty after both — refuse: `New(...)` returns `ErrMissingAPIKey`
   (new sentinel, lives in `internal/model/groq` because it is
   provider-specific). Fail fast, no network call attempted.

Operational discipline pinned by this ADR:

- The adapter **never logs the API key value, the prefix, the
  suffix, or its length**. Logging "we are using an API key" is
  fine; logging anything that would survive a log-leak with
  attacker-useful data is not.
- The adapter **never includes the key in an error message**, not
  even truncated. Errors include `Provider: "groq"` and the wrapped
  cause, never the secret.
- The Adapter struct stores the key in an unexported field. No
  accessor exposes it.
- `cmd/demo-groq` (the live skeleton, see §5) reads
  `GROQ_API_KEY` from the environment, never accepts it as a
  command-line argument (`os.Args` ends up in `/proc/<pid>/cmdline`
  on Linux and in shell history). If the env var is empty, the
  binary exits 1 with a clear message — same shape as Ollama's
  "service not reachable".
- `.gitignore` is updated only if a real risk of accidental
  commit emerges (e.g. `.envrc` from `direnv` workflows). The
  repo today carries no `.env`-shaped files; adding one would
  itself be an anti-pattern.
- This ADR's `accepted` status is a checkpoint: future ADRs that
  touch model adapters reaffirm or amend this principle, never
  silently regress it.

#### Why not flag-driven (`--api-key`) or stdin-piped

Flags: argv leaks via process listing on multi-user hosts and
shell history. Rejected.

Stdin: works for the live skeleton in principle, but the demo's
prompt input also flows over stdin (pipe form). Coupling the two
would be confusing for two-second-test ergonomics. The env-var
shape is what every cloud-LLM SDK already standardised on,
including Groq's own Python and JS clients.

### 4. Error-mapping table from Groq wire → `model.Err*`

Verified against `console.groq.com/docs/errors` plus the
Authorization / Rate-Limits docs. The adapter maps as follows:

| Groq response | model sentinel surfaced |
|---|---|
| Network error (refused, DNS, TLS, timeout, ctx cancel) | wraps `ErrProviderUnavailable` |
| 401 Unauthorized | wraps `ErrAuthInvalid` (new) |
| 403 Forbidden | wraps `ErrAuthInvalid` (treats key-without-access the same as no key — both are "operator must fix config") |
| 429 Too Many Requests | wraps `ErrRateLimited` (new) via a `*RateLimitError{Provider: "groq", RetryAfter: <retry-after seconds, 0 if absent>}` |
| 400 Bad Request | wraps `ErrProviderResponse` — request shape is wrong; this is a Korvun-side bug |
| 404 Not Found (unknown model name) | wraps `ErrProviderResponse` — same category; this is a config bug |
| Other 4xx | wraps `ErrProviderResponse` — defensive default for the "client error, not our retry to make" class |
| 5xx Server Error | wraps `ErrProviderUnavailable` — transient transport-side problem |
| 2xx with malformed JSON | wraps `ErrProviderResponse` |
| 2xx with empty `choices` or empty `message.content` | wraps `ErrProviderResponse` |

Error strings include the wire status code, the Groq `error.type`
when present, and a **1 KiB-capped** snippet of `error.message`.
No header values are reflected into the error string (the
`retry-after` header surfaces via `RateLimitError.RetryAfter`, not
the string).

The rate-limit headers that are always present in successful
responses (`x-ratelimit-remaining-tokens` etc.) are **not** modelled
in `model.Response` in 4.2. Surfacing them is observability work
(later stage); the adapter can grow a callback or a tap when a
consumer reads them. For now they are discarded.

### 5. Live skeleton — `cmd/demo-groq`, defaulting to `llama-3.1-8b-instant`

A new temporary binary, sibling to `cmd/demo-model`:

```
cmd/demo-groq/main.go    # new — Groq end-to-end check
cmd/demo-model/main.go   # existing — Ollama end-to-end check
```

Why a sibling and not a flag on `demo-model`:

- Each demo is the operator's "does this provider work from this
  machine" check. Each has its own env vars (`OLLAMA_HOST` vs
  `GROQ_API_KEY`) and its own failure modes. Two thin binaries are
  clearer than one binary with branching modes.
- Both are explicitly temporary (deleted in Stage 5+ when
  `cmd/korvun` boots the real process). The disposal cost is
  identical whether they are siblings or one branched binary.
- The 4.3 fan-out demo will live as its own `cmd/demo-fanout`
  using both providers from the same process; that's where the
  "one binary, both providers" shape naturally lands.

Defaults:

- `GROQ_API_KEY` — required env. No default. Exits 1 with a clear
  message if absent.
- `KORVUN_DEMO_MODEL` — default `llama-3.1-8b-instant`. Reasoning:
  it's the free-tier model with the most permissive limits (14,400
  RPD, 30 RPM, 6,000 TPM, 500,000 TPD as of 2026-06-14), it returns
  in seconds, and it is the safest default for "I just want to
  prove the cable works".
- `KORVUN_DEMO_SYSTEM` — optional system prompt, same shape as
  `cmd/demo-model`.
- `KORVUN_DEMO_TIMEOUT` — default 60 (Groq returns faster than
  Ollama cold-starts; the slot is generous but tighter).

Same input handling as `cmd/demo-model`: prompt from `os.Args[1:]`
if present, otherwise from stdin (pipe form). Same stdout/stderr
separation: stdout is the assistant content, stderr is diagnostics
(`provider=groq model=... timeout=... ok in ...`).

The doc comment on `cmd/demo-groq/main.go` mirrors
`cmd/demo-model`'s "this is temporary, delete or convert to
integration test in Stage 5+".

## The 4.2 plan — what implementation will look like after this ADR

Four commit-shaped pieces of work, direct to master per the
user's instruction (4.2 is not a structural-concurrency phase):

1. **Sentinel additions in `internal/model`** (red→green). Add
   `ErrAuthInvalid`, `ErrRateLimited`, `RateLimitError{Provider,
   RetryAfter}` to `internal/model/errors.go`. Tests cover
   `errors.Is(err, ErrRateLimited)` for an `errors.As`-cast
   `*RateLimitError`, the `RateLimitError.Error()` message format,
   and `Unwrap()`.

2. **`internal/model/groq` adapter** (red→green). `Adapter` struct
   with `WithBaseURL`, `WithAPIKey`, `WithHTTPClient`,
   `WithRequestTimeout` options. `New(opts ...Option) (*Adapter,
   error)` — note `error` because `ErrMissingAPIKey` is raised at
   construction. `Name()` returns `"groq"`. `Generate(ctx, req)`
   POSTs `/openai/v1/chat/completions` with `Bearer <key>` and
   `stream:false`, decodes `choices[0].message.content`, wraps
   into `*model.Response{Provider: "groq", ModelName: <echoed>}`.
   Tests use `httptest.NewServer` with canned responses, covering:
   happy path; sends `Authorization: Bearer <key>` exactly once
   (the test server asserts it); sends the role-tagged messages;
   wraps 401 → `ErrAuthInvalid`; wraps 429 with `retry-after` →
   `*RateLimitError{RetryAfter}`; wraps 429 WITHOUT `retry-after`
   → `*RateLimitError{RetryAfter: 0}`; wraps 5xx →
   `ErrProviderUnavailable`; wraps 4xx (non-401/403/429) →
   `ErrProviderResponse`; wraps empty `choices` →
   `ErrProviderResponse`; ctx cancel mid-flight wraps
   `ErrProviderUnavailable`; request timeout option fires within
   the bound.

3. **Live skeleton `cmd/demo-groq/main.go`** (build-only commit).
   Mirrors `cmd/demo-model` with the env-var changes from §5.

4. **STAGE-04.md** (closure doc). Covers 4.1 and 4.2 (4.3 has its
   own status row, in-progress when this ADR-0010 implementation
   lands). Documents the live-skeleton end-to-end checks for both
   providers as the closure evidence; same shape as STAGE-02-EXT.md.

`make quality` green over the whole tree before closing each
step. Coverage target: ≥90% for `internal/model/groq` (the
httptest approach makes this trivially achievable, as it did for
`internal/model/ollama`).

## Consequences

### What this enables

- `model.Model` now has **two adapters** of materially different
  shape (local + cloud). Phase 4.3 fan-out has a real test case
  immediately (point N providers at the same request, see what
  they say); the policy engine (Stages 5–6) has a real
  classification problem (one provider sees the data, one does
  not — the privacy attribute ADR-0002 §"Consequence for the
  policy engine" anticipated).
- The Brain author (Stage 7) gets a clear sentinel grammar for
  retry logic: `ErrRateLimited` → wait; `ErrProviderUnavailable`
  → retry-soon; `ErrAuthInvalid` → page the operator;
  `ErrProviderResponse` → fail loudly. None of these required
  changes to `model.Model` itself.
- Two `cmd/demo-*` binaries form a complete operator self-check
  pair (local + cloud). When `cmd/korvun` proper lands in Stage
  5+, both delete in the same commit.

### What this asks of every future cloud adapter

- Honour the **env-only API-key principle** at the source level.
  Use `os.Getenv` (or its `t.Setenv` test surface), never argv,
  never a committed file.
- Surface 401 as `ErrAuthInvalid`, 429 as `*RateLimitError` /
  `ErrRateLimited`, 5xx as `ErrProviderUnavailable`, malformed /
  empty as `ErrProviderResponse`. Caller-side mistakes that
  return 4xx fall under `ErrProviderResponse` so the Brain can
  treat them as "fix the request, don't retry".
- Never log the key. Never include it in an error message.

This becomes the de facto cloud-provider contract; the next ADR
that adds a cloud provider can cite §3 and §4 here as the
already-agreed shape and only argue what's genuinely
provider-specific.

### What this does NOT do

- **No streaming.** Same disposition as ADR-0009 §2: lands behind
  a sibling `StreamingModel` when a real caller needs it.
- **No fan-out.** That is Phase 4.3.
- **No Registry.** Same as 4.1 — lands when 4.3 needs it.
- **No automatic retry.** Even when the adapter knows a 429 with
  `retry-after` is a transient signal, retrying is policy, not
  transport. The Brain (or the fan-out layer in 4.3) reads the
  `*RateLimitError`, decides whether to wait, and re-calls Generate
  itself. Adapter stays dumb-by-design.
- **No rate-limit observability.** The
  `x-ratelimit-remaining-*` headers are present in every Groq
  response; surfacing them as metrics is observability work
  (later stage). The adapter discards them.
- **No streaming-back of error metadata into `model.Response`.**
  `Response` is the success path; errors travel via the error
  return.
- **No fine-tuning, embeddings, audio, tool-use endpoints.** All
  out of scope; each is a sibling capability ADR when a Korvun
  consumer needs it.

### Trade-offs accepted

- **Two new sentinels in `internal/model`.** Acknowledged. They are
  additive (no break to 4.1 callers); they encode the two retry
  classes the Brain genuinely cares about; they save the
  alternative (every consumer string-matches the wrapped cause).
- **`RateLimitError` is a concrete type, not just a sentinel.** Go
  idiom: `errors.Is(err, ErrRateLimited)` continues to work via
  the `Unwrap` chain, and consumers that want `RetryAfter` use
  `errors.As(&rle)`. Same shape as `net.OpError`, `os.PathError`,
  etc.
- **Hand-rolled, again.** Acknowledged. Two adapters now follow
  the same pattern; the principle is consistent
  ("minimal supply chain, use a maintained binding when it pays"),
  the headlines stay aligned ("hand-rolled stdlib") because both
  endpoints are small and the alternatives are heavier.
- **API key required at construction time.** A constructor that
  returns an error is the right shape — the alternative ("lazy
  check on first Generate") delays a fail-fast moment for no
  upside.
- **403 lumped with 401 under `ErrAuthInvalid`.** Both mean
  "operator must fix credentials before this works again"; the
  distinction (no-key vs key-without-access) is not actionable at
  the Brain layer. If a future policy needs them split, splitting
  is additive.
- **No retry inside the adapter.** Re-stated for emphasis: a
  provider that returns `retry-after: 30` is signalling "try
  again in 30 seconds"; the adapter does not honour this
  automatically because the right policy varies per Brain (fan-out
  may want to fail over to Ollama immediately; a single-provider
  Brain may want to wait; an interactive operator may want to
  abort and notify). Policy is upstream.
- **Free-tier model defaults captured at the date of this ADR.**
  Rate limits and model lists are vendor decisions and will drift.
  The implementation reads them from the doc URL captured here;
  if they change before 4.2 lands the adapter still works (it
  doesn't hard-code limits, only the default `KORVUN_DEMO_MODEL`).
  An amending note suffices when a default model becomes
  unavailable.

## Alternatives Considered

### Interface — A1: extend `model.Model` with per-provider capability metadata (e.g. `RequiresAuth`, `Cloud`, `RateLimits`)

Push the differences between Ollama and Groq into the interface
itself: a `Model.Capabilities()` method, or new optional methods.

**Rejected.** The Brain doesn't need this at the call site — the
errors it gets back tell it what it needs to know. Capability
queries are a Registry concern (Phase 4.3), not a per-call shape
concern. Adding them to `model.Model` would force every adapter to
answer them, including local-only adapters where the answer is
trivially "no auth, no rate limit".

### Interface — A2: add `Provider` field to `Request` so the Model is selected by the consumer

A consumer-facing `Request.Provider = "groq"` field that the
adapter inspects.

**Rejected.** Wrong direction. The consumer holds a `model.Model`
value and calls `Generate` on it; the choice of provider already
lives in *which* `model.Model` it holds. Adding `Provider` to the
request would double-encode the same decision and force adapters
to refuse requests addressed to other providers — every error case
that the existing `var m model.Model = grAdapter` already prevents
at the type system.

### Cloud client — C1: official OpenAI Python/JS-shape compatibility via `sashabaranov/go-openai`

Use a community OpenAI-compatible Go client and point its base URL
at Groq.

**Rejected** per §2 table: solves a problem Korvun doesn't have
(all the other OpenAI endpoints) at the cost of a bigger dep and
indirection through a layer that has its own design assumptions
(e.g. how it models errors). Re-evaluable if Korvun consumes ≥3
OpenAI-shape providers.

### Cloud client — C2: `jpoz/groq` (or another community Groq Go lib)

Adopt a community Go library.

**Rejected** per §2: no tagged release, 2 commits on master,
unknown long-term maintenance. The hand-roll cost is lower than
the supply-chain risk.

### API key — K1: configuration file (`config.yaml`) loaded from disk

Read the key from a YAML/TOML file at startup.

**Rejected.** Introduces the "did I `.gitignore` the right path"
foot-gun. The env var avoids the question entirely (an env var is
never accidentally committed). When Korvun grows a real config file
(Stage 5+ bootstrap), it will load the env var into the resolved
config — not the file into the env.

### API key — K2: vault / secret-manager integration in the adapter

The adapter reads from HashiCorp Vault, AWS Secrets Manager, etc.

**Rejected from the adapter, deferred to the operator layer.** The
adapter accepts `WithAPIKey(string)`. The operator (Stage 5+
bootstrap) is the right place to pull from a vault and call
`WithAPIKey(secret)`. The adapter should not know about specific
secret stores.

### Error mapping — E1: keep using only the 4.1 sentinels (`ErrProviderUnavailable`, `ErrProviderResponse`); surface 401 / 429 by inspecting the wrapped cause

No new sentinels; consumers parse error strings or status codes
embedded in the cause.

**Rejected.** Every consumer would re-implement the same
classification (rate-limit-vs-auth-vs-bad-shape) inconsistently.
Lifting it to the sentinel layer is what the abstraction is for.

### Error mapping — E2: also add `ErrQuotaExhausted`, `ErrModelNotFound`, `ErrContextTooLong`, ...

A richer sentinel set up front.

**Rejected** per §1's discipline argument: sentinels that do not
change retry behaviour earn nothing. Add when a consumer reads
them.

### Live skeleton — D1: one shared `cmd/demo-model` with `--provider=...`

Merge the Ollama and Groq demos.

**Rejected** per §5: each provider has its own env shape and
failure mode; two thin binaries are clearer and the 4.3 fan-out
demo is the natural home for "one binary, both providers".

## Open follow-ups (not blockers for Phase 4.2)

- **Rate-limit observability.** Expose `x-ratelimit-remaining-tokens`
  / `x-ratelimit-remaining-requests` as adapter-side metrics so
  Stage 12 observability can alert before a hard 429.
- **Per-provider Registry.** Lands with 4.3 fan-out, sized to its
  consumers.
- **`Request.Options` map.** Provider-side knobs (temperature,
  top_p, max_tokens, …) when a Brain wants to expose them.
- **Streaming.** `StreamingModel` for both Ollama and Groq when
  the voice / interactive caller exists.
- **Conversion of `cmd/demo-groq` to a CI smoke test.** Same
  consideration as `cmd/demo-model` — requires a strategy for
  carrying a Groq API key in CI without leaking. Deferred until
  there is a Korvun CI worth wiring through to.
- **Embedding / tool-use / vision endpoints.** Sibling capability
  ADRs when a real consumer exists.
