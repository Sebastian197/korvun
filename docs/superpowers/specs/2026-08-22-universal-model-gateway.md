# Universal model gateway — provider "openai-compatible": Design Spec

> **Status:** FINAL — Accepted for TDD (copilot adjudication, 2026-08-22;
> Codex adversarial ×2 absorbed).
> Governing ADRs: ADR-0010 (Groq adapter mold §§2-4), ADR-0014 §2 (per-model
> id), ADR-0015 §§3-4 (DECLARED locality, pre-dispatch selector), ADR-0017
> §5 (fail-loud config), ADR-0029 (single binary), ADR-0031 (timeout
> ownership, retry decorator, warmup), ADR-0042 §2 (native tool lane — SP-B).
> External-docs note: SP-A uses ONLY stdlib + existing `internal/` packages
> (net/http + encoding/json, the ADR-0010 hand-rolled pattern — no new
> dependency, no ADR needed). The per-provider wire contract is verified at
> each provider's official documentation, 2026-08-22, in the table below;
> OpenAI reference row verified by the copilot. Product decisions sealed by
> Chano 2026-08-22: D1 provider name `"openai-compatible"`; D2 `base_url`
> REQUIRED fail-loud + `api_key_env` optional-but-must-resolve; D3 two
> sub-phases (SP-A chat, SP-B native tools), SP-A first; D4 ZERO new native
> adapters. Copilot adjudication 2026-08-22 folded in: the Name()-identity
> clarification is RESOLVED as option (a) + the FR-GW-1 collision guard;
> no open clarification remains. Codex adversarial triage 2026-08-22
> absorbed: 13/13 findings (H1-H13) accepted, 4 shaped by the copilot —
> folded into FR-GW-1/2/3/4, the new FR-GW-7, the SP-B sketch, and the
> acceptance scenarios below. v3 (same day) absorbs the re-review
> remainders: the CLOSED quota-code list (H5), Retry-After pinned in
> value AND consumption (H12a), the AS-5 demo artifact (H12b), the TLS
> matrix row (N1), and the softened warmup-identity claim. The second
> Codex pass (2026-08-22, scoped re-review) closed H1-H13 and returned a
> minimal list — N2 (compat-scoped guard positives), N3 (deadline
> tripwire through adapter+decorator), N4 (auth diagnostics pinned by a
> Then), N5 (the no-Authorization proof exercises a real request) — all
> absorbed below.

## Goal

Any OpenAI-compatible chat-completions endpoint — cloud or local — becomes a
first-class Korvun model by CONFIG ALONE, with the house guarantees intact:
locality DECLARED never derived (ADR-0015 §3), fail-loud config naming the
offending field (ADR-0017 §5), key hygiene (the key never surfaces in any
log, format, or error — ADR-0010 §3), the classified error grammar
(ADR-0010 §4), and bounded reads on EVERY response body (error bodies at
the mold's 1 KiB; success bodies at a named 4 MiB cap — deliberately
STRICTER than the mold, FR-GW-3, because the resource-bound invariant
weighs more when `base_url` is arbitrary). Zero per-provider
code: DeepSeek, Moonshot/Kimi, Gemini's compat mode, OpenRouter, LM Studio,
llama.cpp server — and anything else speaking the same wire shape — ride ONE
adapter. SP-A delivers the chat lane; SP-B (native tools on the same wire)
follows as its own RED. Explicitly out of SP-A: streaming, embeddings,
images/audio, tool-calling, model/price discovery, new retry semantics, and
any new native adapter (D4).

## Grounding — Phase 0 reconnaissance (file:line)

The mold and every seam this spec touches, as they exist on master
(`d7c02c7`):

- **Groq mold (the template to generalize)** — `internal/model/groq/groq.go`:
  Adapter struct with unexported `apiKey` (groq.go:35-40); functional options
  `WithBaseURL` (trailing-`/` trim, groq.go:48-52), `WithAPIKey`,
  `WithHTTPClient`, `WithRequestTimeout` (groq.go:48-73); `New` with
  key-resolution chain and fail-fast `ErrMissingAPIKey` (groq.go:89-104);
  `String`/`GoString` redaction (groq.go:112-118); `Generate` appends
  `chatPath = "/chat/completions"` (groq.go:20) to baseURL, POSTs
  `{model, messages, stream:false}` with `Authorization: Bearer` (groq.go:150-166);
  non-2xx mapped by `mapHTTPError` (groq.go:207-229) with 1 KiB bounded error
  body (`maxErrorBodyBytes`, groq.go:25) and `decodeErrorSnippet` reflecting
  only the structured error envelope (groq.go:234-246). Default base
  `https://api.groq.com/openai/v1` (doc.go:42).
- **Key-hygiene tests (the pattern to replicate)** —
  `internal/model/groq/groq_test.go`: formatting never leaks the key
  (groq_test.go:115-128); the key absent from EVERY error including transport
  errors (groq_test.go:458-488, sentinel `gsk_TOPSECRET_must_never_appear`
  against both an httptest 4xx and a refused connection).
- **The seam** — `internal/model/model.go`: `Model` is `Generate` + `Name()`
  only (model.go:93-99); `Request`/`Response` DTOs (model.go:69-82); `Role`
  includes `RoleTool` and `Message` carries additive `ToolCalls`/`ToolName`
  for the native lane (model.go:38-39, 50-62). Error sentinels in
  `internal/model/errors.go`: `ErrProviderUnavailable` (:42),
  `ErrProviderResponse` (:48), `ErrAuthInvalid` (:57), `ErrRateLimited` (:73)
  + `RateLimitError`, `ErrToolsUnsupported` (:66, SP-B relevant).
- **Config validation today** — `internal/config/config.go`: `ModelConfig`
  struct (config.go:520-542; `BaseURL` currently optional, "defaults per
  adapter"); provider whitelist `"ollama"|"groq"` with the fail-loud unknown
  message listing the valid set (config.go:988-993); locality declared
  `local|cloud` required (config.go:998-1004); `api_key_env` required for
  groq (config.go:1005-1009); warmup only for locality local
  (config.go:1030-1032). The `outbound_token_env` mold for D2 lives at
  config.go:228-236 (optional NAME) with boot resolution at app.go:1278-1282
  (named-but-empty → `ErrMissingSecret`, fail-loud with the VARIABLE name).
- **Wiring** — `internal/app/app.go`: `buildModel` provider switch
  (app.go:1171-1201; groq resolves the key from `APIKeyEnv`, empty →
  `ErrMissingSecret` naming the variable, app.go:1184-1187);
  `ErrUnknownProvider` sentinel (internal/app/errors.go:17-20);
  `buildCatalog` wraps every adapter in the retry decorator BEFORE
  `WithModelID` and tags the DECLARED locality (app.go:1092-1119); timeout
  single-owner rule — adapters get NO `WithRequestTimeout`, the decorator
  owns the per-attempt deadline (app.go:1174-1177, ADR-0031 Decision 2).
- **Selector / privacy filter** — `internal/policy/selector.go`:
  `Locality`/`CatalogEntry` (selector.go:34-52); `SelectModels` drops Cloud
  entries for Private brains BEFORE the fan-out, fail-loud
  `ErrNoEligibleModels` when nothing survives (selector.go:78-99). The
  filter is structural over the declared attribute — it covers ANY provider
  by construction, including this one.
- **Warmup** — `internal/app/warmup.go`: `warmupTarget` stores the DECORATED
  model (warmup.go:14-22); `warmupOne` is PROVIDER-AGNOSTIC — a trivial
  `Generate` with content "hi" through the decorated model, best-effort WARN
  on failure (warmup.go:58-71). Registration is gated only by
  `m.Warmup` + the config rule locality==local, deduplicated by
  (provider, baseURL, modelID) (app.go:1114-1116, 1125-1140). **Finding:
  warmup needs ZERO new code for this provider** — no per-provider probe
  exists anywhere; see FR-GW-6.
- **Policy order matching (feeds FR-GW-1's collision guard)** —
  `internal/policy/priority.go:19-20` and consensus.go:47-48: `Order`
  entries match `Outcome.Provider == Model.Name()`.
- **Name() through the decorator chain (feeds FR-GW-2)** — the production
  chain retry(adapter) → WithModelID preserves the adapter's base identity:
  `internal/model/retry/retry.go:129` delegates `Name()` to the wrapped
  model, and `internal/brain/named.go:40` (comment :38-39) returns the
  wrapped PROVIDER's name so "fan-out attribution stays the provider
  identity (e.g. \"ollama\"), not the model id". `Response.ModelName` is
  what carries the per-model distinction (model.go:78-82).

## External verification — per-provider wire table (2026-08-22)

Every row verified TODAY at the provider's own documentation (primary
source; Context7 not applicable — these are HTTP APIs, not code libraries;
the house rule's "verify at source" half applies). The operator-facing rule
each row illustrates: **`base_url` is the FULL prefix, Korvun appends
exactly `/chat/completions`** (FR-GW-1).

| Provider | `base_url` (exact, incl. `/v1` where the provider uses it) | Auth header | Chat endpoint | Notable quirks | Tools (SP-B) | Source |
|---|---|---|---|---|---|---|
| OpenAI (reference) | `https://api.openai.com/v1` | `Authorization: Bearer <key>` | POST `{base}/chat/completions` | reference wire: messages/choices/usage/tools. NOTE (H2): models REQUIRING the `"developer"` instruction role (the o1 family) are OUT of SP-A compatibility — see FR-GW-3 and the `instruction-role-capability` follow-up | Yes | Verified by copilot (platform.openai.com API reference) |
| DeepSeek | `https://api.deepseek.com` (NO `/v1` — only documented form; an `/anthropic` base also exists, irrelevant here) | Bearer | POST `{base}/chat/completions` | thinking/reasoning params; Anthropic-compat sibling base | Yes (dedicated guide) | api-docs.deepseek.com (fetched 2026-08-22) |
| Moonshot / Kimi | `https://api.moonshot.ai/v1` (China platform: `https://api.moonshot.cn/v1`) | Bearer | POST `{base}/chat/completions` | temperature 0-1 on v1 models; partial mode (`"partial": true`); stateless | Yes | platform.kimi.ai/docs/api/chat (fetched 2026-08-22) |
| Gemini (OpenAI-compat mode) | `https://generativelanguage.googleapis.com/v1beta/openai` | `Authorization: Bearer <GEMINI_API_KEY>` | POST `{base}/chat/completions` | compat layer in beta; some OpenAI params silently ignored; `extra_body` for Gemini-only features | Yes | ai.google.dev/gemini-api/docs/openai (fetched 2026-08-22) |
| OpenRouter | `https://openrouter.ai/api/v1` | Bearer | POST `{base}/chat/completions` | model ids are `provider/model`; optional attribution/metadata headers exist but are NOT required | Yes | openrouter.ai/docs/api-reference/chat-completion (fetched 2026-08-22) |
| LM Studio (local) | `http://localhost:1234/v1` (default port 1234) | none required | POST `{base}/chat/completions` | model ids are LM Studio's own | Yes ("Tool Use" listed) | lmstudio.ai/docs/app/api/endpoints/openai (fetched 2026-08-22) |
| llama.cpp server (local) | `http://127.0.0.1:8080/v1` (default host/port; `--host`/`--port` flags) | none by default; optional `--api-key` → Bearer | POST `{base}/chat/completions` | tool calling needs `--jinja`; Jinja chat templates | Yes (with `--jinja`) | github.com/ggml-org/llama.cpp tools/server/README.md (fetched 2026-08-22) |

The table's spread (`/v1` vs no `/v1` vs `/api/v1` vs `/v1beta/openai`) is
exactly why FR-GW-1's zero-magic rule exists: Korvun never guesses the
prefix.

## Functional requirements — SP-A (chat)

- **FR-GW-1 (config)** — `ModelConfig` accepts provider
  `"openai-compatible"` (D1). For this provider: `base_url` REQUIRED and
  must parse as a valid absolute `http`/`https` URL with a non-empty Host,
  and MUST NOT carry userinfo (`URL.User`), a query (`RawQuery`), or a
  fragment (H4 — each of the four violations fails config load naming
  `brains[i].models[j].base_url`, ADR-0017 §5, mold config.go:988-1009); `model_id` REQUIRED (existing rule config.go:995-997);
  `api_key_env` OPTIONAL with the D2 rule (the `outbound_token_env` mold,
  config.go:228-236 + app.go:1278-1282): absent → no Authorization header
  (the local-server case); named-but-unresolvable at boot →
  `ErrMissingSecret` naming the VARIABLE. `locality` stays DECLARED
  `local|cloud`, required, exactly as today (config.go:998-1004). Zero-magic
  rule: Korvun appends EXACTLY `/chat/completions` to `base_url` (after
  trailing-`/` trim, the groq.go:50 mold) — the operator supplies the full
  prefix (`/v1`, `/api/v1`, `/v1beta/openai`, or none, per the table).
  Blast radius: `validateModels` (config.go:983-1035) gains one provider
  case; the unknown-provider message's valid set grows to
  `(ollama|groq|openai-compatible)`.
  **Collision guard (copilot adjudication 2026-08-22, corrected same day;
  identity made canonical per H9):** within one brain, two entries with
  the IDENTICAL CANONICAL triplet — (`provider`, `base_url` NORMALIZED
  [trailing slash trimmed, after provider defaults are applied],
  `model_id`) — fail config load with `ErrInvalidConfig` naming BOTH
  indices (`brains[i].models[j]` and `brains[i].models[k]`). So
  `https://x/v1` and `https://x/v1/` with the same `model_id` ARE a
  collision. In SP-A the guard validates ONLY `openai-compatible` entries;
  extending it to the existing providers is a named additive follow-up —
  **`triplet-guard-existing-providers`** — with its own regression suite,
  OUT of SP-A. Coherence anchor, stated precisely: the guard defines ITS
  OWN normalization; the warmup dedup already uses the same triplet IN
  SPIRIT, but concatenates the RAW values without normalization today
  (`collectWarmup`'s key `provider + "|" + baseURL + "|" + modelID`,
  app.go:1126, invoked from app.go:1114-1116) — so the guard promotes an
  existing identity to validation without claiming byte-for-byte
  equivalence; harmonizing the warmup key onto the canonical form lives
  inside the already-registered `triplet-guard-existing-providers`
  follow-up. A duplicated canonical triplet is the same backend model
  wired twice in one brain: indistinguishable everywhere (rank,
  attribution, warmup) and never a legitimate shape. Blast radius: ZERO
  legitimate configs — `ollama`×N with distinct `model_id`s still loads
  (local fan-out/consensus intact), and any compat pair differing in
  normalized `base_url` or `model_id` loads too.
  Documented limitation, BY DISPATCH SHAPE (H10): entries sharing the
  provider label (an `ollama`×N set, or multi-compat with distinct
  `base_url`s) share their rank label and `Response.Provider` attribution
  (the provider constant, FR-GW-2) under EVERY dispatch — what differs is
  what that means per shape: a `priority` fan-out returns the FIRST usable
  success and cancels the siblings (app.go:1514-1526, the ADR-0031 SV1
  wiring; pinned by coordinator_carveout_test.go:61), `sequential` walks
  the catalog order, and `consensus` waits for every vote and reduces.
  Observability, stated precisely: `Response.ModelName` rides only the
  FINAL response (model.go:78-82); per-model `Outcome`s and metrics are
  labeled by PROVIDER — per-entry observability is a named additive
  follow-up, **`per-entry-observability`** (OUT of SP-A). Registered
  additive follow-up (OUT of SP-A), by name —
  **`model-config-name-override`**: an optional `name` field on
  `ModelConfig` overriding `Name()`, fail-loud on duplicates.
- **FR-GW-2 (adapter)** — New package `internal/model/openaicompat`
  implementing `model.Model` (model.go:93-99) as a generalization of the
  Groq mold: constructor with functional options (`WithBaseURL` required-in
  practice via wiring, `WithAPIKey`, `WithHTTPClient`, `WithRequestTimeout`
  with the same zero-disables semantics), context propagated end-to-end
  (`http.NewRequestWithContext`, groq.go:160 mold). Key hygiene identical to
  groq.go:35-40 + 112-118: unexported field, no accessor, `String`/`GoString`
  redaction. Difference from the mold: NO env-var fallback inside the
  package (there is no canonical env name for a generic provider — the
  wiring resolves `api_key_env` and passes `WithAPIKey`), and an EMPTY key
  is valid (no-auth local servers).
  **`Name()` pinned (copilot adjudication 2026-08-22):** the adapter
  returns the mold identity — `ProviderName = "openai-compatible"`, the
  base provider name, exactly as groq/ollama today. The production
  decorator chain preserves it verbatim: `retry.decorator.Name()`
  delegates (internal/model/retry/retry.go:129) and `named.Name()` returns
  the wrapped provider's name so attribution stays the provider identity,
  not the model id (internal/brain/named.go:38-40). Every `Provider`
  assertion in AS-2/AS-3 hangs from this pin.
  **Auth label (H7):** a `WithAuthLabel(label)` option — the wiring passes
  the NAME of `api_key_env` as a non-secret label; 401 diagnostics include
  it when present ("check <LABEL>") and stay generic when absent. The
  label is a variable NAME, never key material.
  **Redaction belt (H8):** EVERY diagnostic derived from a response body
  or header REDACTS the resolved key — literal replacement with
  `[REDACTED]` — BEFORE wrapping into any error. This is a second belt
  over the mold's "never reflect the key" discipline: it also defuses a
  server that ECHOES the Bearer value back (a real hazard once `base_url`
  is arbitrary). Pinned by AS-3's echo rows.
- **FR-GW-3 (wire)** — Request `{model, messages, stream:false}` (stream
  explicit, groq.go:252-256 mold). Instruction role (H2): instructions
  ride as a `role:"system"` message ONLY, exactly as the mold's
  `toChatMessages` (groq.go:305-314) — endpoints/models that REQUIRE a
  `"developer"` role (OpenAI's o1 family) are EXPRESSLY outside SP-A's
  compatibility (noted on the OpenAI table row; named additive follow-up
  **`instruction-role-capability`**, OUT of SP-A).
  **Response contract (H1):** the modelled response gains the top-level
  `error` object and `choices[0].finish_reason`; processing order is
  FIXED: top-level `error` present → `finish_reason == "error"` →
  `refusal` → `content`. An HTTP 200 carrying an embedded error (either
  of the first two) is a PERMANENT `model.ErrProviderResponse` and any
  partial `content` is NEVER used.
  **Refusal (H2):** a non-empty `message.refusal` with empty `content`
  means the refusal text IS the assistant reply — a VALID response
  (mapped to `RoleAssistant`), never an error, never a fallback trigger.
  **Success-body bound (H3):** the 2xx body is read through
  `io.LimitReader` with the named constant `maxResponseBytes = 4 MiB`
  (generous for chat; if implementation shows it short, it gets vetoed
  with evidence); an over-limit body ⇒ `model.ErrProviderResponse` naming
  the cap. Parity note, deliberate DIVERGENCE from the mold: groq/ollama
  stream-decode success bodies with no cap (groq.go:179; ollama.go:136) —
  the new adapter is STRICTER on purpose, because the resource-bound
  invariant weighs more when `base_url` is operator-arbitrary; the
  retrofit to groq/ollama is a named additive follow-up,
  **`bounded-success-body-retrofit`** (OUT of SP-A). Non-2xx bodies keep
  the mold's 1 KiB cap (groq.go:25, 208; ollama.go:168).
  **Malformed 2xx, ENUMERATED (H12)** — each row ⇒
  `model.ErrProviderResponse`: empty body; invalid JSON; valid JSON
  followed by trailing garbage (the decoder DEMANDS EOF — one extra token
  fails); empty `choices`; empty `content` without `refusal`. `usage`
  stays tolerated-optional (not modelled, the groq.go:267-274
  discipline); response reads `choices[0].message` (groq.go:183-189).
- **FR-GW-4 (error grammar — the explicit matrix, H6)** — The ADR-0010 §4
  grammar generalized (mapHTTPError mold, groq.go:207-229) and ALIGNED
  WITH ADR-0031: the decorator's law rules retryability (`waitFor`,
  retry.go:216-225: retryable ⇔ `*RateLimitError` or
  `ErrProviderUnavailable`), never openai-go's client policy. The matrix:

  | Condition | Sentinel | Retryable? |
  |---|---|---|
  | 401 | `model.ErrAuthInvalid` — RESERVED to 401 (H7); names the auth label (`WithAuthLabel`, FR-GW-2) when present, generic otherwise; never the key | No (permanent) |
  | 403 | `model.ErrProviderResponse` with the bounded snippet — region / permissions / moderation; the text ORIENTS without asserting auth (H7) | No (permanent) |
  | 404 | `model.ErrProviderResponse` — diagnostic steers the operator at the two usual suspects: `model_id` and the `base_url` prefix | No (permanent) |
  | 408 | `model.ErrProviderUnavailable` (H6) | Yes |
  | 409 | `model.ErrProviderResponse` (default bucket, H6) | No (permanent) |
  | 429 whose structured error envelope carries a code/type in the CLOSED list `quotaExhaustedCodes` (H5, below) | `model.ErrProviderResponse` — text directs the operator: quota/credit, human intervention required, zero retry | No (permanent) |
  | 429 with NO recognized quota code | `*model.RateLimitError` (wraps `ErrRateLimited`) — the safe default for genuine rate limits | Yes |
  | 5xx | `model.ErrProviderUnavailable` | Yes |
  | TLS verification failure (certificate / hostname) | `model.ErrProviderResponse` — retrying does not repair trust (N1); detected via `errors.As` against `*tls.CertificateVerificationError` (Go 1.20+, wraps the x509 cause through `Err`/`Unwrap` — covers `x509.UnknownAuthorityError`, `x509.HostnameError`, `x509.CertificateInvalidError`, `x509.SystemRootsError`; types verified at pkg.go.dev crypto/tls + crypto/x509, 2026-08-22; toolchain go 1.26.6) | No (permanent) |
  | Transport error (connection refused/reset, DNS failure) | `model.ErrProviderUnavailable` | Yes |
  | Per-attempt deadline expiry | STOP — F6, the DECORATOR's property (retry.go:171: `context.DeadlineExceeded` is never retried); this prose no longer contradicts it. OBLIGATION on the adapter (N3): its error wrap MUST PRESERVE `context.DeadlineExceeded` in the chain (`%w`) so `errors.Is` holds at the decorator — a flattened cause would fall into `ErrProviderUnavailable` and retry | No (stop) |
  | Other 4xx / malformed 2xx (FR-GW-3's enumeration) | `model.ErrProviderResponse` | No (permanent) |

  **`quotaExhaustedCodes` — the CLOSED, testable list (H5).** A NAMED
  closed set (Go cannot `const` a collection — implementable as a switch
  or individual constants; the name and the closure are the contract); a
  429 is permanent-quota IFF the structured envelope's
  `error.code` OR `error.type` (both fields are already modelled by the
  mold, groq.go:291-299) matches an entry. Contents as verified TODAY
  (2026-08-22) against the official OpenAI error-codes documentation
  (developers.openai.com/api/docs/guides/error-codes — the former
  platform.openai.com URL 301s there):

  - `insufficient_quota` — the billing `error.type` OpenAI documents for
    quota-exhausted 429s (the guaranteed minimum of the list);
  - `credit_balance_exhausted` — `error.code`: "Your organization has no
    prepaid credits remaining";
  - `organization_spend_limit_exceeded`, `project_spend_limit_exceeded`,
    `organization_usage_limit_exceeded` — the `error.code` values OpenAI
    documents for enforced spend/usage limits (all ride HTTP 429).

  Every additional code enters the list ONLY with its source cited in
  this spec; the list is extensible in one line. Any 429 whose envelope
  matches nothing falls to `*model.RateLimitError` — the safe default for
  genuine rate limits. Providers that signal quota exhaustion via a
  DIFFERENT status need no list at all: they already land in the matrix's
  permanent default bucket — verified today: DeepSeek uses 402
  "Insufficient Balance" (api-docs.deepseek.com/quick_start/error_codes)
  and OpenRouter uses 402 "insufficient credits" (openrouter.ai/docs/
  api-reference/errors, which also documents its 429 as a genuine
  rate-limit with Retry-After) — both 402s ⇒ permanent
  `model.ErrProviderResponse` by the "other 4xx" row, with the operator
  text steering at billing.

  Retry-After: seconds form ONLY — the HTTP-date form is DOCUMENTED as
  ignored (yields zero = "no hint") and the wait falls to the decorator's
  full-jitter backoff; that is the parser's sustained decision
  (`model.ParseRetryAfter`, parseretryafter.go:16-25; backoff
  retry.go:229-235, hint consumption retry.go:216-220). The H8 redaction
  belt applies to every row. Tests PIN that the key appears in NO error
  string, INCLUDING transport errors — the groq_test.go:458-488 sentinel
  pattern, replicated — plus AS-3's echo rows.
- **FR-GW-5 (selector/policy)** — An `openai-compatible` entry with
  DECLARED locality `local` is eligible in Private brains (LM Studio is the
  canonical case); one declared `cloud` is dropped by the EXISTING
  structural filter BEFORE the fan-out (selector.go:78-99). ZERO new
  privacy logic: the guarantee is that the existing filter covers this
  provider BY CONSTRUCTION (it routes on the declared attribute, never on
  the adapter). AS-4 pins both directions.
- **FR-GW-6 (wiring + warmup)** — `buildModel` (app.go:1171-1201) gains the
  `"openai-compatible"` case: resolve `api_key_env` per D2, construct the
  adapter with `WithBaseURL(m.BaseURL)` and no `WithRequestTimeout`
  (single-owner rule, ADR-0031 Decision 2); catalog entry with declared
  locality + the retry decorator + `WithModelID`, exactly the
  app.go:1092-1119 path — untouched. Warmup: NO new mechanism — Phase 0
  established that `warmupOne` (warmup.go:58-71) is already
  provider-agnostic (a trivial decorated `Generate`), so `warmup: true` on
  an `openai-compatible` model declared `local` works with ZERO new code
  and stays config-rejected for `cloud` (config.go:1030-1032). This
  SUPERSEDES the pre-spec lean of a best-effort `GET {base}/models`: no
  probe exists for any provider today, and inventing one for this provider
  would be new per-provider surface for no demonstrated need.
- **FR-GW-7 (redirect policy — a privacy invariant, H11)** — The
  adapter's `http.Client` sets `CheckRedirect` to REFUSE: any 3xx ⇒
  permanent `model.ErrProviderResponse` ("endpoint redirected — refusing
  to follow"). Rationale: a redirect can silently re-route the
  conversation body and the Authorization header to a host the operator
  never declared — with arbitrary `base_url` that is an egress-invariant
  hazard, not a convenience. Pinned by AS-6 (Stage-16-class e2e: the spy
  server must receive NOTHING).

## SP-B — native tools (sketch; full FRs at its own RED)

Same wire, additive fields: `tools` on the request, `tool_calls` on the
assistant message, `role:"tool"` result turns — the seam already carries
`Message.ToolCalls`/`ToolName` (model.go:50-62), `RoleTool`
(model.go:38-39), `ToolCallingModel` (internal/model/toolcalling.go) and
the `ErrToolsUnsupported` degrade lane (errors.go:66). CORRECTED (H13):
the current seam does NOT transport `tool_call_id` — `ToolCall` is
`{Name, Arguments}` only (toolcalling.go:41-49) and the `role:"tool"`
turn is labeled by `ToolName`, not by a call id (model.go:50-62), while
the OpenAI wire correlates tool results by `tool_call_id`. Whether and
how the DTO evolves is a DEFERRED SP-B decision (additive, at SP-B's
RED); SP-A's out-of-scope PROHIBITS touching it. Feasibility per the
table: every verified target supports OpenAI-style tools (llama.cpp behind
`--jinja`; Gemini compat mode in beta). Demo criterion sketch: at least one
real tool round-trip (request → `tool_calls` → `role:"tool"` result →
final answer) against a real compat server, per the model-dependent-behavior
law. Scope and FRs close at SP-B's RED.

## Acceptance scenarios — SP-A (Given / When / Then)

- **AS-1 (config fail-loud)** Given a config with provider
  `"openai-compatible"`, a valid `base_url` and `model_id`, When the config
  loads, Then boot proceeds. Given the same config WITHOUT `base_url` (or
  with a non-URL value), When the config loads, Then load fails with
  `ErrInvalidConfig` naming `brains[i].models[j].base_url`. Given
  `api_key_env: "COMPAT_KEY"` with the variable unset, When the app builds,
  Then boot fails with `ErrMissingSecret` naming `COMPAT_KEY` (and never
  any key material). Given `api_key_env` absent, When a `Generate`
  round-trip ACTUALLY RUNS against a no-auth local endpoint (a real
  request — building alone proves nothing, N5), Then the request carries
  NO `Authorization` header, asserted by the httptest server on the
  received request. Given a `base_url` carrying userinfo, a query,
  a fragment, or an empty host (four separate rows, H4), When the config
  loads, Then each fails with `ErrInvalidConfig` naming
  `brains[i].models[j].base_url`. Given a config where one brain holds
  two entries with the IDENTICAL CANONICAL (`provider`, normalized
  `base_url`, `model_id`) triplet — including `https://x/v1` vs
  `https://x/v1/` with the same `model_id` (H9), When the config loads,
  Then load fails with `ErrInvalidConfig` naming BOTH offending indices
  (`brains[i].models[j]` and `brains[i].models[k]`). Given a brain with
  `ollama`×2 whose `model_id`s DIFFER, When the config loads, Then load
  succeeds — the positive assertion that pins the corrected guard in both
  directions. And IN the guard's own scope (N2): Given a brain with two
  `openai-compatible` entries sharing `base_url` but with DISTINCT
  `model_id`s, When the config loads, Then load succeeds; Given the same
  with a shared `model_id` but DISTINCT normalized `base_url`s, When the
  config loads, Then load succeeds — an implementation that rejects any
  second compat entry fails these rows.
- **AS-2 (round-trip)** Given an httptest server speaking the compat wire,
  When `Generate` runs with a system + user conversation, Then the request
  body is `{model, messages, stream:false}` with the system prompt as a
  `role:"system"` message, the path is exactly `<base path>/chat/completions`,
  and the response maps `choices[0].message.content` to a
  `model.Response` with the assistant role; a response without `usage`
  succeeds. Given a 200 whose `message.refusal` is non-empty and
  `content` empty (H2), When `Generate` runs, Then the refusal text IS
  the assistant reply — a valid `model.Response`, no error, no fallback.
- **AS-3 (error grammar + key hygiene, table-driven over the FR-GW-4
  matrix)** Given the httptest server scripted per matrix row, When it
  returns 401 (twice: WITH `WithAuthLabel` — the diagnostic CONTAINS the
  label — and WITHOUT it — the diagnostic is generic and label-free, N4),
  403 (whose diagnostic makes NO authentication claim: not
  `ErrAuthInvalid`, and the text neither says auth-invalid nor names the
  label, N4), 404, 408, 409, a 429 for EACH entry of
  `quotaExhaustedCodes` (one test row per code, H5), a 429 with NO
  recognized code (⇒ `*model.RateLimitError`, the safe default), 429-plain
  (with and without Retry-After, including the ignored HTTP-date form),
  5xx, and when the server is unreachable, Then each maps to its FR-GW-4
  sentinel with the stated retryability. Given a 429 with `Retry-After: 7`
  (H12a), When `Generate` fails, Then the returned `*model.RateLimitError`
  carries `RetryAfter == 7s` — the value pinned at the adapter. And the
  CHAIN pinned, not just the piece (H12a, short integration): Given the
  retry decorator WRAPPING the compat adapter with a fake clock injected
  (`retry.WithClock`, retry.go:37 + 56-57), When the adapter returns that
  429, Then the observed wait on the fake clock is ≥ 7s before the next
  attempt. And the deadline tripwire through the REAL chain (N3): Given
  the retry decorator wrapping the compat adapter with retries ENABLED
  and a per-attempt deadline, and a server slower than that deadline,
  When the decorated `Generate` runs, Then the returned error satisfies
  `errors.Is(err, context.DeadlineExceeded)` AND the server observed
  EXACTLY ONE call — F6 held through the adapter's wrap, no retry fired.
  Given an `httptest.NewTLSServer` whose certificate is NOT in
  the client's root pool (N1), When `Generate` runs, Then the result is
  permanent `model.ErrProviderResponse` with ZERO retries (matched via
  `errors.As` on `*tls.CertificateVerificationError`). Given a
  200 with partial `content` PLUS an embedded error (top-level `error`
  object, and separately `finish_reason == "error"`, H1), When `Generate`
  runs, Then the result is permanent `model.ErrProviderResponse` and the
  partial content is NEVER used. Given the malformed-2xx enumeration
  (H12: empty body; invalid JSON; valid JSON + trailing garbage; empty
  `choices`; empty `content` without `refusal`), When `Generate` runs,
  Then each row maps to `model.ErrProviderResponse`. Given a success body
  of exactly `maxResponseBytes` and one of `maxResponseBytes`+1 (H3),
  When `Generate` runs, Then the first succeeds and the second fails
  naming the cap. Given a hostile server that ECHOES the literal Bearer
  value back in the JSON error envelope AND in a plain-text error body
  (H8), When `Generate` fails, Then the key value appears in NO returned
  error string — across ALL rows of this scenario, including the
  transport-error row (the groq_test.go:458-488 pattern + the H8
  redaction belt).
- **AS-4 (privacy filter, both directions)** Given a Private brain whose
  catalog holds an `openai-compatible` entry declared `local`, When
  `SelectModels` runs, Then the entry is selected. Given the same brain
  with the entry declared `cloud` (and no local sibling), When
  `SelectModels` runs, Then it fails with `ErrNoEligibleModels` — the
  cloud entry is never contacted. And from the REAL ASSEMBLY (H12): Given
  a full config wired through the app (config → `Build` → inbound
  message) whose Private brain declares a compat entry `cloud` pointing
  at a SPY server PLUS a local sibling (so `Build` itself succeeds —
  without one, `ErrNoEligibleModels` fails boot before any message),
  When the message is handled, Then the spy records ZERO contact — the
  privacy filter proven at the assembly level, not only at the selector
  unit.
- **AS-5 (real-model demo — the law)** Given at least ONE real local compat
  server on Chano's iMac (LM Studio or llama.cpp server), When a real
  channel message routes to a brain wired `openai-compatible` at that
  server, Then a chat round-trip completes end-to-end; and IF a cloud
  compat key is at hand, the same demo runs once against a real cloud
  compat endpoint. A green suite over fakes does NOT close this criterion
  (CLAUDE.md law). The demo produces its VERIFIABLE artifact (H12b): a
  REDACTED record in the demo report under `design-drafts/` — server and
  version, `model_id`, date, the path hit, and the per-act result — so
  "witnessed" is paper, not a word.
- **AS-6 (redirect refusal — Stage-16-class e2e, H11)** Given a local
  server that answers 307 pointing at a second SPY server, When
  `Generate` runs against the first, Then the call fails permanent
  (`model.ErrProviderResponse`, "endpoint redirected — refusing to
  follow") AND the spy received NEITHER the request body NOR the
  Authorization header — zero bytes of conversation or credential ever
  reach the redirect target.

## Out of scope — SP-A (explicit)

Streaming; embeddings/images/audio; tool-calling (SP-B); automatic
model/price discovery; retries beyond the existing classification
(ADR-0031 decorator untouched); new native adapters (D4); the
`"developer"` instruction role — o1-family endpoints are expressly
unsupported in SP-A (H2, follow-up `instruction-role-capability`); ANY
evolution of the tool DTOs — adding `tool_call_id` or otherwise touching
`ToolCall`/`Message` for tool correlation is PROHIBITED in SP-A (H13,
deferred to SP-B's RED).

## Success criteria

- Suite green with `-race`; `make quality` full.
- Coverage: new seams ≥ 90% (`internal/model/openaicompat` sits on the
  model seam; the touched `policy`/`config` surfaces keep their existing
  floors), all other new code ≥ 85%.
- Real-model demo per AS-5 executed and witnessed.
- User docs in the SAME batch as the code (public-surfaces checklist,
  permanent rule): CONFIGURATION.md gains the provider with per-provider
  `base_url` examples straight from the verification table.
- Untouched: the headless binary shape, existing adapters (Groq/Ollama
  byte-identical behavior), the retry decorator, the selector.

## Decisions folded in

- Exact-append `/chat/completions` with trailing-slash trim, zero prefix
  magic — the table's `/v1` spread makes guessing hostile to operators.
- No in-package env fallback (unlike GROQ_API_KEY): a generic provider has
  no canonical env name; the wiring owns resolution (D2).
- Empty key valid at the adapter; strictness lives in config/wiring (D2's
  named-must-resolve).
- Warmup rides the existing generic `warmupOne` — no `GET /models` probe
  (supersedes the pre-spec lean; grounded at warmup.go:58-71).
- 404 stays permanent under `ErrProviderResponse` (no new sentinel): the
  retry decorator already treats it as non-transient; the diagnostic text
  carries the operator hint instead.
- Package/provider name asymmetry: the package is
  `internal/model/openaicompat` (Go naming law — package names are
  lowercase, no hyphen or underscore) while the config string is
  `"openai-compatible"` (D1). Same relationship groq/ollama have between
  package name and provider constant; the constant, not the package name,
  is the operator-facing identity.

## `[NEEDS CLARIFICATION]`

None — every open point was adjudicated by the copilot on 2026-08-22. The
former point 1 (Name()/policy-order identity) is resolved as option (a)
plus the collision guard, folded into FR-GW-1, FR-GW-2 and AS-1; the
package-name asymmetry moved to "Decisions folded in". Nothing blocks TDD.
