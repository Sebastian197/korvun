# Sub-phase 3 — Ollama HTTP error mapping refinement: Design Spec

> **Status:** draft (awaiting co-pilot review — do NOT start tests until the
> Review Checklist is green)
> **Date:** 2026-07-12
> **Relates:** ADR-0031 (Piece 2 — production error handling); ADR-0010 §4
> (adapter error taxonomy). Mirrors `groq.mapHTTPError`, adapted to a local,
> auth-less provider.

## Goal

`ollama.Adapter.Generate` must classify a non-2xx HTTP response into the
**correct `internal/model` sentinel** instead of collapsing every non-2xx into
`ErrProviderResponse` (its single current branch, `ollama.go:121–127`). This
lets the retry decorator (sub-phase 4) distinguish:

- **"provider down" (retryable):** `ErrProviderUnavailable` (5xx, transport).
- **"rate-limited" (retryable, with optional wait):** `*RateLimitError`.
- **"bad response / bad request" (NOT retryable):** `ErrProviderResponse`.
- **"auth rejected" (NOT retryable, operator must fix):** `ErrAuthInvalid`.

The classification must live in an **isolated, table-testable function**
(`mapHTTPError`), exactly like `groq.mapHTTPError`, not inline in `Generate`.

## Context (verified 2026-07-12 — not re-derived from memory)

- **Today:** `ollama.Generate` wraps ALL non-2xx in `ErrProviderResponse` in one
  inline branch (`ollama.go:121–127`); the body is read capped at
  `maxErrorBodyBytes` (1 KiB) and trimmed.
- **groq reference:** `groq.mapHTTPError` (`groq.go:208–230`) switches
  401/403→`ErrAuthInvalid`, 429→`*RateLimitError`, ≥500→`ErrProviderUnavailable`,
  default→`ErrProviderResponse`; helpers `decodeErrorSnippet` and
  `parseRetryAfter` (seconds-only; HTTP-date form explicitly unsupported).
- **Sentinels already exist** (`internal/model/errors.go`):
  `ErrProviderUnavailable`, `ErrProviderResponse`, `ErrAuthInvalid`,
  `ErrRateLimited`, and `RateLimitError{Provider, RetryAfter}` whose `Unwrap`
  returns `ErrRateLimited` (so `errors.Is(err, ErrRateLimited)` and
  `errors.As(err, &rle)` both work).
- **Ollama is local and auth-less:** the adapter sends **no** `Authorization`
  header (contrast groq, which sends `Bearer`). Ollama's native error body is a
  flat `{"error":"<message>"}` string, NOT groq's nested
  `{"error":{message,type,code}}` envelope.

## Functional Requirements — one per mapping rule

| FR | Given HTTP status | Maps to | Retryable? |
|----|-------------------|---------|-----------|
| **FR-1** | `>= 500` (any 5xx) | `model.ErrProviderUnavailable` | yes |
| **FR-2** | `429` | `*model.RateLimitError{Provider:"ollama", RetryAfter: <Retry-After header or 0>}` (wraps `ErrRateLimited`) | yes (after wait) |
| **FR-3** | any 4xx that is NOT 429 and NOT 401/403 (e.g. 400, 404, 422) | `model.ErrProviderResponse` | no |
| **FR-4** | `401` or `403` | `model.ErrAuthInvalid` *(resolved proposal — see D1)* | no |
| **FR-5** | — | The switch lives in an isolated `mapHTTPError(resp *http.Response) error`, called from `Generate` at the `resp.StatusCode < 200 || >= 300` check; NOT inline. | — |

**Unchanged invariants (regression guards, already true today — must stay green):**

| FR | Case | Maps to |
|----|------|---------|
| **FR-6** | Transport failure (`client.Do` error: network down, ctx cancel mid-flight) | `model.ErrProviderUnavailable` (unchanged, `ollama.go:117`) |
| **FR-7** | 2xx with malformed JSON / empty assistant content | `model.ErrProviderResponse` (unchanged, `ollama.go:129–137`) |
| **FR-8** | Request-build / marshal failures | `model.ErrProviderResponse` (unchanged) |

## Design decisions

### D1 — `[NEEDS CLARIFICATION]` 401/403 → **RESOLVED as proposal: `ErrAuthInvalid`**

**Proposal: map 401/403 → `model.ErrAuthInvalid`** (same as groq), NOT
`ErrProviderResponse`.

Rationale:
1. **HTTP semantics are provider-agnostic.** 401/403 mean "credentials
   missing/invalid" regardless of who emits them. Native Ollama never emits
   them, but a reverse proxy / API gateway in front of Ollama (nginx/Caddy basic
   auth, a corporate gateway — a common self-hosted deployment) will.
2. **It matches the sentinel's stated purpose.** `ErrAuthInvalid`'s godoc
   (`errors.go:49–55`) says it is "distinct from `ErrProviderUnavailable`
   because a retry will not help — the operator must fix the credentials". A
   401/403 from a fronting proxy is exactly that: retrying is futile; the
   operator must fix the proxy auth. Folding it into `ErrProviderResponse` would
   mislead the sub-phase 4 retry decorator into a wrong bucket.
3. **Cross-adapter consistency.** Same status code → same sentinel across groq
   and ollama keeps the retry policy uniform and the mental model simple.
4. **Cost is zero:** it is one extra `case` identical to groq's.

Counter-view considered: in a pure single-user Ollama box a 401/403 is so
anomalous it is almost a client-config bug — but even there `ErrAuthInvalid`
("fix credentials/config") is more actionable than `ErrProviderResponse` ("bad
response"). Proposal stands. **Co-pilot to confirm.**

### D2 — `parseRetryAfter`: extract to shared `model.ParseRetryAfter` vs duplicate

**Proposal: extract to `internal/model` as an exported
`model.ParseRetryAfter(raw string) time.Duration`, and have BOTH adapters use
it** (migrate `groq.parseRetryAfter` → `model.ParseRetryAfter` in the same
sub-phase).

Rationale:
- Single source of truth for the parse (seconds today; the one place to extend
  to the HTTP-date form later, which both adapters currently omit identically).
- It is a pure, stateless function that sits naturally beside `RateLimitError`
  in `internal/model`, which already owns the rate-limit vocabulary.
- Tested once instead of twice.

Trade-off / blast radius to flag: extracting means touching `groq.go` +
`groq_test.go` (migrate call site, delete the local copy), slightly widening
this sub-phase beyond "the Ollama adapter". **Bounded alternative:** duplicate an
unexported `parseRetryAfter` inside package `ollama` (zero groq churn, at the
cost of two copies to keep in sync). **Recommend the extraction; co-pilot to
confirm the wider blast radius is acceptable, else fall back to duplication.**

### D3 — Error snippet: keep current raw-trimmed body (out of core scope)

The status-code mapping does **not** depend on the error-body shape (HTTP status
is standard). The current Ollama snippet (raw body, `LimitReader` 1 KiB,
trimmed) is kept as-is for all branches. A structured decoder for Ollama's
`{"error":"<msg>"}` shape (analogous to groq's `decodeErrorSnippet`) is
**optional and out of scope** for this sub-phase; if pursued, the exact body
shape must be **verified via Context7** (Ollama API docs) before coding, per the
project rule — not assumed here.

### D4 — 429 from a local provider

429 from native Ollama is uncommon (local, typically single-user), but is
handled for the same proxy/gateway reason as D1, and because a future Ollama
concurrency limit could introduce it. No native-only assumption is baked in.

## Acceptance Scenarios (Given/When/Then) — one per status class

- **AS-1 (5xx → unavailable):** *Given* an Ollama endpoint returning `503`,
  *when* `Generate` is called, *then* it returns an error satisfying
  `errors.Is(err, model.ErrProviderUnavailable)` and NOT
  `errors.Is(err, model.ErrProviderResponse)`.
- **AS-2 (429 with header → rate limit + hint):** *Given* a `429` with
  `Retry-After: 7`, *when* `Generate` is called, *then* `errors.As(err, &rle)`
  succeeds with `rle.Provider == "ollama"` and `rle.RetryAfter == 7*time.Second`,
  and `errors.Is(err, model.ErrRateLimited)` holds.
- **AS-3 (429 without header → rate limit, zero hint):** *Given* a `429` with no
  `Retry-After`, *then* `errors.As(err, &rle)` succeeds with
  `rle.RetryAfter == 0`.
- **AS-4 (401/403 → auth):** *Given* a `401` (and, as a second case, `403`),
  *then* `errors.Is(err, model.ErrAuthInvalid)` holds and it is NOT
  `ErrProviderResponse`.
- **AS-5 (other 4xx → bad response):** *Given* a `400` (and, as a second case,
  `404`), *then* `errors.Is(err, model.ErrProviderResponse)` holds and it is NOT
  `ErrProviderUnavailable` / `ErrAuthInvalid` / `ErrRateLimited`.
- **AS-6 (transport failure unchanged):** *Given* an unreachable base URL,
  *then* `errors.Is(err, model.ErrProviderUnavailable)` still holds (FR-6
  regression guard).
- **AS-7 (2xx malformed unchanged):** *Given* a `200` with a body that is not
  valid JSON / has empty content, *then* `errors.Is(err,
  model.ErrProviderResponse)` still holds (FR-7 regression guard).
- **AS-8 (snippet never leaks, stays bounded):** *Given* any non-2xx with a
  large body, *then* the error message contains a bounded (≤ 1 KiB) diagnostic
  snippet and the status code, and nothing sensitive.

## Success Criteria

- All FR-1…FR-8 satisfied; all AS-1…AS-8 pass as table-driven tests run with
  `-race`.
- The mapping is an **isolated, directly unit-testable** `mapHTTPError` (tested
  without a full `Generate` round-trip where practical), mirroring groq.
- `internal/model/ollama` package coverage **≥ 90%** (critical-package bar; it
  routes through `model`, and the mapping is pure/branchy so high coverage is
  cheap). Overall `internal/` stays **≥ 85%**; no package regresses.
- `make quality` green `-race` over the WHOLE suite (lint + vet + gosec +
  govulncheck + coverage), not just the new code.
- Zero changes to network/transport behaviour beyond error classification (no
  new requests, headers, or retries added here — retry is sub-phase 4).
- If D2 extraction is taken: `groq` continues to pass unchanged behaviourally
  (same `*RateLimitError` output); the migration is pure refactor.

## Review Checklist (must be green before writing tests)

- [ ] Goal unambiguous and scoped to error classification only.
- [ ] Every FR maps exactly one status class to exactly one sentinel; no gaps,
      no overlaps (5xx / 429 / 401-403 / other-4xx / 2xx-malformed / transport
      all covered).
- [ ] D1 (401/403 → `ErrAuthInvalid`) **confirmed** by co-pilot.
- [ ] D2 (extract `model.ParseRetryAfter` **vs** duplicate) **decided** by
      co-pilot; blast radius on groq accepted or extraction dropped.
- [ ] D3 (keep raw snippet; structured decoder out of scope) confirmed.
- [ ] Acceptance scenarios cover each FR and each regression guard.
- [ ] Success criteria include the ≥90% package bar and whole-suite `make
      quality`.
- [ ] No `[NEEDS CLARIFICATION]` left unresolved.

---

**STOP:** paused here for co-pilot review. No tests, no implementation, no
commit until D1/D2/D3 are confirmed and the checklist is green.
