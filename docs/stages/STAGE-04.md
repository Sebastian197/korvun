# Stage 04 — Models

> **Status:** closed
> **Started:** 2026-06-14
> **Closed:** 2026-06-20

## Objective

Define the abstraction every reasoning component in Korvun talks
through to call an LLM, and ship the mechanism every multi-provider
component will eventually use to dispatch to several at once. Three
cuts:

- **4.1** the `model.Model` interface and the first concrete provider
  (Ollama, local).
- **4.2** a second concrete provider of materially different shape
  (Groq, cloud) to stress-test the interface against a real
  cloud-provider failure surface (401 / 429 / 5xx / quota).
- **4.3** the fan-out coordinator that dispatches a single request to
  N providers in parallel and collects every outcome — the mechanism
  layer of the policy engine that lands in Stages 5–6.

The line that frames the stage: this is the **mechanism** of model
invocation. Choosing among outcomes, voting / consensus, privacy /
cost-aware routing, retry policy — all OUT of scope for Stage 4; all
in scope for Stages 5–6.

## Phases

| Phase | Description                                         | Status |
|-------|-----------------------------------------------------|--------|
| 4.1   | `model.Model` interface + Ollama adapter            | done   |
| 4.2   | Groq cloud adapter + cloud-shaped error grammar     | done   |
| 4.3   | Fan-out coordinator (`internal/model/fanout`)       | done   |

## Phase 4.1 — `model.Model` interface + Ollama adapter

### Deliverables

- **`docs/adr/0009-model-interface-and-ollama.md`** (status
  `accepted`), pinning the five protocol decisions: (1) `Model`
  speaks role-tagged messages, not `envelope.Envelope`; (2) Phase 4.1
  ships only synchronous `Generate`, streaming via a sibling
  `StreamingModel` later; (3) `ctx` first, propagated all the way
  through the HTTP request; (4) hand-rolled `net/http` for Ollama
  (the official Go client drags 16 direct deps for one endpoint);
  (5) package layout `internal/model/` + `internal/model/ollama/`,
  mirroring `internal/channel/`. Registry deferred to 4.3.
- **`internal/model/`** package:
  - `doc.go`, `model.go` — `Role`, `Message`, `Request`, `Response`,
    `Model` interface (`Generate(ctx, *Request) (*Response, error)` +
    `Name() string`).
  - `errors.go` — five validation sentinels (`ErrNilRequest`,
    `ErrEmptyModel`, `ErrEmptyMessages`, `ErrInvalidRole`,
    `ErrEmptyContent`) + two provider-side sentinels
    (`ErrProviderUnavailable`, `ErrProviderResponse`); plus
    `ValidateRequest` — the universal upstream invariant check every
    adapter calls as the first thing inside `Generate`.
- **`internal/model/ollama/`** adapter, hand-rolled `net/http` +
  `encoding/json` against `/api/chat` with `stream: false`.
  `Adapter` struct, `WithBaseURL` / `WithHTTPClient` /
  `WithRequestTimeout` options, `New(...)`, `Name()` →
  `"ollama"`, `Generate(ctx, req)`. Tests use `httptest.NewServer`
  with canned responses; happy path, ctx cancel mid-flight, non-2xx,
  malformed JSON, empty messages, `ModelName` echoed.
- **`cmd/demo-model/main.go`** — live skeleton: reads `OLLAMA_HOST`
  (default `http://127.0.0.1:11434`), `KORVUN_DEMO_MODEL` (default
  `llama3.2`), prompt from `os.Args[1]` or stdin, calls the adapter,
  prints the assistant content. Marked as a temporary stage-closure
  check, deleted (or rewritten as integration test) when `cmd/korvun`
  proper boots in Stage 5+.

### Behaviour established by Phase 4.1

- The `Model` interface speaks `(*Request, *Response)`, not
  `Envelope`. The Brain (Stage 7) owns the
  `Envelope ↔ []model.Message` translation, the same way the channel
  adapters own `nativeFormat ↔ Envelope`.
- `Response.Provider` and `Response.ModelName` are first-class fields
  — deliberate redundancy with `Model.Name()` so a fan-out result
  (Phase 4.3) keeps its attribution without a side channel.
- ctx propagation reaches the underlying `*http.Request` via
  `http.NewRequestWithContext`. Cancelling ctx aborts the in-flight
  HTTP call.
- Hand-rolled adapter shape (3 unexported structs mirroring the
  fields Korvun reads, JSON unknown-fields ignored) becomes the
  reference pattern for any future provider.

### Live skeleton, end-to-end check

`cmd/demo-model` exercised by the operator against a real local
Ollama: the round-trip succeeded with `llama3.2` (1B parameter
variant) returning a Spanish answer within seconds.

## Phase 4.2 — Groq cloud adapter

### Deliverables

- **`docs/adr/0010-groq-cloud-provider.md`** (status `accepted`),
  pinning the five Phase 4.2 decisions: (1) `model.Model` survives
  Groq unchanged, but the sentinel set grows by two cloud-shaped
  categories; (2) hand-rolled `net/http` + `encoding/json` against
  `POST https://api.groq.com/openai/v1/chat/completions` (no
  official Go SDK, community libs lack release discipline); (3) API
  key strictly from env (`GROQ_API_KEY`) or `WithAPIKey(...)`,
  **never** from argv, **never** committed to the repo, **never**
  reflected into logs or error messages — non-negotiable principle
  recorded at the ADR layer; (4) error-mapping table from Groq wire
  → `model.Err*` sentinels; (5) live skeleton `cmd/demo-groq`
  defaults to `llama-3.1-8b-instant` (the most permissive free-tier
  model as of the ADR date).
- **`internal/model/errors.go`** — two new sentinels:
  - `ErrAuthInvalid` — wraps 401 / 403; not recoverable until the
    operator fixes credentials.
  - `ErrRateLimited` — wraps 429; recoverable by waiting.
  - `RateLimitError{Provider, RetryAfter}` — concrete error type
    returned on 429 paths. `Unwrap` returns `ErrRateLimited` so
    `errors.Is(err, ErrRateLimited)` keeps working; `errors.As(err,
    &rle)` recovers the metadata (Provider + RetryAfter from the
    `retry-after` header).
- **`internal/model/groq/`** adapter, hand-rolled stdlib, same shape
  as the Ollama adapter. Constructor returns `error` because
  `ErrMissingAPIKey` is raised at `New(...)` time (fail fast, no
  network call attempted on a missing key).
  - Error mapping: network / ctx cancel / 5xx →
    `ErrProviderUnavailable`; 401 / 403 → `ErrAuthInvalid`; 429 →
    `*RateLimitError{Provider: "groq", RetryAfter: <seconds>}`; 400
    / 404 / malformed body / empty content / unexpected status →
    `ErrProviderResponse`.
  - Tests use `httptest.NewServer` covering every error category +
    the happy path + ctx cancel + `Authorization: Bearer ...` header
    asserted exactly once + the `retry-after` header parsed both
    with and without a value.
- **`cmd/demo-groq/main.go`** — live skeleton, sibling to
  `cmd/demo-model`. Reads `GROQ_API_KEY` (required), `KORVUN_DEMO_MODEL`
  (default `llama-3.1-8b-instant`), `KORVUN_DEMO_SYSTEM` (optional),
  `KORVUN_DEMO_TIMEOUT` (default 60). Same disposable shape as
  `demo-model`.

### Behaviour established by Phase 4.2

- The `model.Model` interface **survived an entirely different
  failure surface unchanged**. Two adapters of materially different
  shape (local-no-auth-no-quota vs cloud-bearer-token-quota) now
  satisfy the same contract; the Brain layer that lands in Stage 7
  will call them identically.
- The cloud sentinel grammar (`ErrAuthInvalid`, `ErrRateLimited` +
  `*RateLimitError`, plus the existing `ErrProviderUnavailable` /
  `ErrProviderResponse`) gives the future policy engine a clean
  classification of "should I retry, when, with what cooldown" with
  one `errors.Is` per branch and `errors.As` for the rate-limit
  metadata.
- The API-key contract is now binding for every future cloud
  adapter (env-only at the operator layer, never argv, never logged,
  never in errors). ADR-0010 §3 holds the principle in stone for
  later providers.

### Live skeleton, end-to-end check

`cmd/demo-groq` exercised by the operator with a free-tier Groq API
key: `llama-3.1-8b-instant` returned in **433 ms** including network
RTT, content valid, `ModelName` echoed correctly.

## Phase 4.3 — Fan-out coordinator

### Deliverables

- **`docs/adr/0011-model-fanout.md`** (status `accepted`), 1085
  lines pinning the seven Phase 4.3 decisions. The load-bearing one:
  **the mechanism returns ALL outcomes in input order; it does NOT
  pick a winner, retry, or filter.** Policy belongs upstream
  (Stages 5–6). Concrete decisions: (1) one mechanism, wait-all
  with deadline, no strategy flag; (2) total deadline via caller
  ctx + opt-in per-model timeout via `WithPerModelTimeout`, late
  results never surface; (3) per-model failures preserve the
  upstream `model.*` sentinel grammar untouched, two new
  mechanism-level sentinels (`ErrNoModels`, `ErrNilModel`); (4)
  pre-allocated `[]Outcome` indexed by input position (no
  collection channel, no close-race surface — verified by
  `WaitGroup.Done→Wait` happens-before); (5) Registry deferred AGAIN
  (zero consumers in 4.3); (6) `internal/model/fanout` sub-package,
  coordinator does NOT implement `model.Model` (forcing it to pick
  one Response would be policy); (7) stdlib only — `errgroup`
  rejected because its fail-fast semantics are the inverse of what
  fan-out wants.
- **`internal/model/fanout/`** package:
  - `doc.go`, `errors.go` — `ErrNoModels`, `ErrNilModel`.
  - `fanout.go` — `Outcome{Provider, Response, Err, Latency}`,
    `Result{Outcomes []Outcome}`, `Coordinator` struct + `Option` +
    `WithPerModelTimeout(d)` (no-op on `d ≤ 0`), `New(opts ...)`,
    `Run(ctx, req, models)`. Internal `callOne` spawns one goroutine
    per model, writes into the pre-allocated slot, recovers panics
    so a buggy adapter never tumbles the host process, captures
    Latency on every exit path (normal, error, panic) via an
    always-run defer.
- **`internal/model/fanout/*_test.go`** — table-driven, `-race`
  mandatory, 100% statement coverage. Includes: validation
  pre-spawn for every `model.Err*` sentinel; per-Outcome sentinel
  preservation across the four cloud-shaped categories; the
  `*RateLimitError{RetryAfter}` metadata survives via `errors.As`;
  deterministic input-order outcomes across 50 runs of 5 models
  with mixed delays; ctx cancel mid-flight; per-model timeout fires
  only on the slow model; per-model timeout zero / negative
  no-op'd; per-model timeout chained under caller ctx; panic in
  `Generate` becomes Outcome (with sentinel preservation via `%w`);
  panic in `Name()` becomes Outcome with cross-slot isolation;
  zero-value Coordinator (one-shot use only, per the doc); test
  seam for `c.now`; latency captured on success / error / panic
  paths; concurrent `Run` invocations on the same Coordinator;
  exactly-one-of (Response or Err) invariant pinned with a table
  test covering plain err / sentinel / `*RateLimitError` / panic in
  Generate / panic in Name; no goroutine leak (poll-with-retry
  helper, not sleep-based).
- **`cmd/demo-fanout/main.go`** — live skeleton: reads
  `OLLAMA_HOST` + `GROQ_API_KEY` + `KORVUN_DEMO_MODEL`, constructs
  both adapters, calls `fanout.New(...).Run(ctx, req, []model.Model{
  ollama, groq})`, prints per-Outcome metadata to stderr and
  successful content to stdout. The catalog mismatch between Ollama
  and Groq model names is intentional — the demo exhibits the
  partial-failure shape the fan-out exists to expose, which the
  policy engine of Stages 5–6 will resolve via per-provider model
  mapping.

### Pre-merge `/review` cycle (Phase 4.3 specific)

The gstack `/review` skill was invoked twice during 4.3, with
materially different value each time:

- **First invocation (on ADR-0011, code-less design doc):** overkill.
  Specialist dispatch and Codex review were no-ops on a markdown
  document; the manual-review-with-confidence-calibration tooling
  inside the skill caught some genuine but minor gaps that a
  focused subagent prompt would have caught more cheaply. Documented
  conclusion: do not invoke `/review` on ADRs in future phases.
- **Second invocation (on the 4.3 code):** real value. The red-team
  specialist caught two concrete bugs that the manual review (both
  the user's and the agent's) missed and that **violated contracts
  written in ADR-0011 itself**:
  - **P1 — panic recovery used `%v` instead of `%w`.** A buggy
    adapter that panicked with a `model.*` sentinel would have lost
    `errors.Is` chain identity at the fan-out boundary, breaking
    ADR-0011 §3's promise that the upstream sentinel grammar is
    preserved untouched.
  - **P2 — data race between the zero-value defense and concurrent
    reuse.** The Coordinator doc claimed "safe for concurrent reuse"
    but the zero-value `c.now == nil` lazy default was an
    unsynchronized write that races against the post-`New()`
    concurrent-Run path's reads. No existing test combined the two
    code paths; `-race` did not flag it because the combination was
    not exercised.

Both fixes landed on `feat/4.3-fanout` (commits `e633874` for P1
with `TestRun_panicWithSentinelPreservesGrammar`; `4d35541` for P2,
doc-only narrowing). Five additional fix commits applied
informational-tier specialist findings (I1 Latency-on-panic, I2
sentinel disjointness, I3 timeout/cancel sentinel assertions, I5
panic-Name cross-slot, I7 dead state, I8 exactly-one invariant, I11
goroutine-leak detection switched from sleep+sample to
poll-with-retry).

### Live skeleton, end-to-end check

`cmd/demo-fanout` exercised against the operator's Ollama + Groq
setup: both providers called in parallel, Outcomes returned in input
order, Latency captured per slot, partial failure (Ollama OK, Groq
returning `ErrProviderResponse` on the Ollama-shaped model name) is
the expected demonstration of the policy/mechanism boundary.

## Workflow compliance (all three phases)

| Step                         | 4.1 | 4.2 | 4.3 |
|------------------------------|-----|-----|-----|
| External docs verified first | Context7 + source-read of `github.com/ollama/ollama@v0.30.8` | Context7 + WebFetch of `console.groq.com/docs` (current Groq surface, rate limits, error envelope) | Context7 of `golang.org/x/sync/errgroup` (verified semantics, rejected) |
| ADR written before code      | ADR-0009 accepted before any code | ADR-0010 accepted before any code | ADR-0011 accepted before any code |
| TDD red before green         | Per commit, per sub-phase | Per commit, per sub-phase | Per commit (8 commits total: 3 feature + 5 fix); each `fix(...)` had its RED test documented in the commit body |
| `make quality` green         | yes, with `-race` | yes, with `-race` | yes, with `-race -count=10` on `internal/model/fanout` |
| Stage doc updated            | this document | this document | this document |

## Quality gate (stage-wide, on master)

| Package                          | Coverage |
|----------------------------------|----------|
| `internal/channel`               | 100.0%   |
| `internal/channel/webhook`       | 91.4%    |
| `internal/channel/telegram`      | 90.5%   |
| `internal/envelope`              | 97.0%    |
| `internal/model`                 | 100.0%   |
| `internal/model/ollama`          | 96.0%    |
| `internal/model/groq`            | 94.7%    |
| `internal/model/fanout`          | **100.0%** (≥ 90% critical-package target) |
| `internal/router`                | 96.3%    |
| **total**                        | **90.9%** |

`make quality` green with `-race`. `-race -count=10` on
`internal/model/fanout` settles in ~9 s with zero flakes.

## Key decisions

| ADR | Decision |
|-----|----------|
| ADR-0009 | `Model` speaks role-tagged messages; hand-rolled Ollama adapter; Registry deferred. |
| ADR-0010 | Hand-rolled Groq adapter; env-only API key (non-negotiable); cloud-shaped sentinel grammar (`ErrAuthInvalid` + `ErrRateLimited` / `*RateLimitError`). |
| ADR-0011 | Fan-out as mechanism, not policy; wait-all with deadline; pre-allocated slice indexed by input position (no collection channel); panic recovery preserves `errors.Is` chain via `%w`; zero-value Coordinator for one-shot use only; Registry deferred AGAIN. |

## Open follow-ups (deferred from Stage 4)

### From `/review` Phase 4.3 specialist pass (informational tier, not blockers)

- **I4 — Validation pre-spawn coverage gap.** `TestRun_rejectsInvalidRequest`
  exercises only `ErrEmptyModel`; the package doc promises any
  `model.Err*` validation sentinel round-trips. Convert to a
  table-driven test covering `ErrEmptyMessages`, `ErrInvalidRole`,
  `ErrEmptyContent` too.
- **I6 — `cmd/demo-fanout` doubles the timeout** by passing
  `perModelTimeout` to both `*.WithRequestTimeout` and
  `fanout.WithPerModelTimeout`. Functionally benign but the
  semantics of `KORVUN_DEMO_TIMEOUT` are ambiguous. Demo-only;
  collapses when `cmd/korvun` proper replaces all three demos in
  Stage 5+.
- **I10 — `TestRun_rejectsNilCtx` uses `strings.Contains`** for the
  rejection check; the nil-ctx path is the only validation branch
  without an exported sentinel. Consider exporting `ErrNilCtx`.
- **I12 — `TestRun_concurrentInvocationsOnSameCoordinator`** only
  exercises happy-path fakes. A variant with mixed outcomes (one Run
  with panics, another with errors, another with success, all
  concurrent on one Coordinator) would surface any accidental shared
  state in future Coordinator extensions.

### Architectural / capability

- **Registry.** Deferred a third time. Lands with the configuration /
  bootstrap layer in Stage 5+ when there is a real consumer that
  needs name-based Model lookup.
- **Streaming.** `StreamingModel` sibling interface (ADR-0009 §2) +
  `(*Coordinator).RunStream(...)` (ADR-0011 follow-up) when a real
  caller needs partial output (voice loop, interactive chat in the
  no-code builder).
- **Embeddings.** Sibling `Embedder` interface when a caller exists
  (probably a RAG layer in a later stage).
- **Tool-use.** Sibling `ToolUser` interface when the Brain has a
  concrete tool to call.
- **Vision.** Sibling `VisionModel` interface when a multimodal
  caller exists.
- **Rate-limit observability.** Expose Groq's
  `x-ratelimit-remaining-tokens` / `x-ratelimit-remaining-requests`
  headers as adapter metrics so Stage 12 observability can alert
  before a hard 429.
- **CI conversion of the live skeletons.** Requires a CI strategy
  for carrying provider credentials safely. Deferred until Korvun
  has a CI worth wiring through.
- **Per-Outcome metrics surfaced.** `Outcome.Latency` is captured at
  the mechanism layer; surfacing as Prometheus histograms is Stage
  12 observability work.

## Notes

- The model adapters import only stdlib + their package-internal
  helpers. `go.mod`'s single direct dependency line
  (`github.com/go-telegram/bot v1.21.0`) survived the entire stage
  unchanged. Three new packages (`internal/model`,
  `internal/model/ollama`, `internal/model/groq`,
  `internal/model/fanout`) and three new `cmd/demo-*` binaries, zero
  modules added.
- The `cmd/demo-model`, `cmd/demo-groq`, and `cmd/demo-fanout`
  binaries are explicitly temporary. They will be deleted (or
  rewritten as integration tests with a proper credential carrier)
  in the same commit as `cmd/korvun` proper boots in Stage 5+.
- The two `/review` invocations during Phase 4.3 produced a durable
  project-level rule: invoke `/review` on **code**, not on **design
  docs**. ADRs get adversarial reading manually or via a focused
  subagent prompt; the specialist dispatch shines on real diffs with
  concurrency or other non-obvious failure surfaces.