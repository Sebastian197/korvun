# ADR-0044: Universal model gateway — the openai-compatible provider

> **Status:** accepted
> **Date:** 2026-08-22
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0010](0010-groq-cloud-provider.md) (the hand-rolled adapter
> mold — options, error grammar, key hygiene), [ADR-0015](0015-pre-dispatch-selector.md) §§3-4 (DECLARED
> locality, the pre-dispatch selector), [ADR-0031](0031-resilience-timeouts-retry-and-degradation.md) (timeout single-owner,
> the retry decorator's law, warmup), [ADR-0042](0042-native-tool-calling-lane.md)
> (the native tool lane SP-B will ride).
> Governing spec: `docs/superpowers/specs/2026-08-22-universal-model-gateway.md`
> (FINAL — Accepted for TDD).

## Context

Korvun ships two native model adapters (Ollama local, Groq cloud). The
post-beta product decision (Chano, 2026-08-22) opens the universal gateway:
most cloud AND local model servers today expose the SAME wire — OpenAI's
`POST {base}/chat/completions` with Bearer auth — verified 2026-08-22 at
the official documentation of DeepSeek, Moonshot/Kimi, Gemini's compat
mode, OpenRouter, LM Studio, and llama.cpp server (the spec's per-provider
table, each row with its source). Adding a native adapter per provider
(D4's rejected path) multiplies code that would be byte-similar, while one
config-driven adapter makes every compat endpoint a first-class model with
the house guarantees intact.

## Decision

One generic provider, `"openai-compatible"`, in a new package
`internal/model/openaicompat` implementing the unchanged `model.Model`
seam. Product decisions sealed by Chano (2026-08-22):

- **D1** — provider name `"openai-compatible"`.
- **D2** — `base_url` REQUIRED, fail-loud naming the field; `api_key_env`
  OPTIONAL but named-must-resolve at boot (`ErrMissingSecret` naming the
  VARIABLE, the `outbound_token_env` mold).
- **D3** — two sub-phases: SP-A (chat) first, SP-B (native tools) second.
- **D4** — ZERO new native adapters: compat endpoints ride this one.

Load-bearing decisions carried by the spec (each with its rationale there):

- **Zero-magic suffix**: Korvun appends EXACTLY `/chat/completions` to the
  operator's full `base_url` prefix — the verified `/v1` / no-`/v1` /
  `/api/v1` / `/v1beta/openai` spread makes guessing hostile.
- **Canonical triplet guard, scoped to compat in SP-A**: duplicate
  (`provider`, normalized `base_url`, `model_id`) within a brain fails
  config load naming both indices; extension to existing providers is the
  named follow-up below.
- **Redirect refusal as a privacy invariant (FR-GW-7)**: `CheckRedirect`
  refuses; any 3xx is a permanent error — a redirect would silently
  re-route conversation body and Authorization to an undeclared host.
- **4 MiB success-body cap, deliberately STRICTER than the mold**: the
  resource-bound invariant weighs more when `base_url` is
  operator-arbitrary; groq/ollama retrofit is a named follow-up.
- **429 partitioned by the CLOSED `quotaExhaustedCodes` set**: quota /
  billing envelopes (verified against OpenAI's error-codes docs) are
  PERMANENT with operator-directing text; unrecognized 429s stay retryable
  rate limits. Providers signaling quota via 402 (DeepSeek, OpenRouter —
  sources in the spec) fall in the permanent default bucket.
- **TLS verification failure is PERMANENT**: retrying does not repair
  trust; detected via `errors.As` on `*tls.CertificateVerificationError`.
- **`role:"system"` only; the o1 family excluded**: endpoints requiring
  the `"developer"` role are expressly out of SP-A.
- **`Name()` = the provider constant** (`"openai-compatible"`), preserved
  verbatim by the retry → `WithModelID` chain, exactly as groq/ollama.
- **No in-package env fallback**: a generic provider has no canonical env
  name; the wiring resolves `api_key_env`.
- **Empty key valid at the adapter**: no-auth local servers are first
  class; strictness lives in config/wiring (D2).

## Adversarial provenance

The spec absorbed two Codex adversarial passes on 2026-08-22: the 13-finding
triage (H1-H13, four shaped by the copilot's adjudication) and the scoped
re-review (H1-H13 closed; its minimal list N2-N5 — compat-scoped guard
positives, the deadline tripwire through adapter+decorator, auth
diagnostics pinned by a Then, the no-Authorization proof exercising a real
request — absorbed). Nothing remains open; the spec is the contract.

## Consequences

- Any compat endpoint — cloud or local — becomes a Korvun model by config
  alone; LM Studio / llama.cpp make private-brain local models trivial.
- The privacy filter covers the new provider BY CONSTRUCTION (declared
  locality, unchanged selector) — no new privacy logic to maintain.
- The adapter is stricter than the mold in bounded reads and redirect
  policy; that asymmetry is deliberate and documented until the retrofit
  follow-up lands.
- `validateModels` grows one provider case; the unknown-provider message's
  valid set becomes `(ollama|groq|openai-compatible)` (a mechanical test
  blast radius, declared in the spec).
- Deferred consequences, registered BY NAME in the spec (all OUT of SP-A):
  1. `model-config-name-override` — optional `name` on `ModelConfig`
     overriding `Name()`, fail-loud on duplicates.
  2. `triplet-guard-existing-providers` — extend the canonical triplet
     guard to ollama/groq, harmonizing the warmup dedup key.
  3. `per-entry-observability` — per-catalog-entry outcome/metric labels.
  4. `instruction-role-capability` — the `"developer"` role (o1 family).
  5. `bounded-success-body-retrofit` — the 4 MiB cap for groq/ollama.

## Alternatives Considered

- **Native adapter per provider** — rejected (D4): N× byte-similar
  packages, each a maintenance and hygiene surface, for endpoints that
  share one wire.
- **Community client SDK (e.g. openai-go)** — rejected: the ADR-0010
  hand-rolled stdlib pattern already fits, keeps the dependency surface at
  zero, and the SDK's retry/error policy would fight the ADR-0031
  decorator's law (the spec's FR-GW-4 matrix is aligned with the decorator
  instead).
- **Guessing/normalizing the URL prefix (`/v1` auto-append)** — rejected:
  the verified provider spread makes any guess wrong somewhere; fail-loud
  explicitness beats magic.
- **Configurable instruction-role knob in SP-A** — rejected by
  adjudication: scoping compatibility (o1 family out) closes the impact
  without new config surface; the capability is the named follow-up.
