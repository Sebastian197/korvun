# N1 — onboarding with the gateway: Design Spec (short)

> **Status:** approved for TDD. UX contract: `design-drafts/ola2-designs.md`
> §3 — APROBADO POR CHANO 2026-08-23 (sealed: the [Comprobar] wait is the
> button itself turning «Comprobando…», disabled until the result). Lote-2
> mandate shape: the compat branch carries the mold's four fields
> (base_url, model_id, api_key_env, locality) and the step VALIDATES the
> chosen model EXISTS before it can close; failures speak on screen with a
> fix at hand. The ollama flow stays intact byte-for-byte. Governing:
> ADR-0044 (compat base_url rules), ADR-0035 §5 (first-run template),
> ADR-0010 (env-var names). External-docs note: the /models probe follows
> the OpenAI-compatible list shape ({"data":[{"id":...}]}) already
> exercised live in the v0.9.0 gateway demo; stdlib only.

## Functional requirements

- **FR-N1-1 (Go)** — `Desktop.CheckCompatModel(baseURL, modelID, apiKeyEnv)`
  probes `{base}/models` (bounded, the CheckOllama deadline class) and
  returns `{reachable, modelFound, needsKey, detail}` — outcomes, never Go
  errors. If apiKeyEnv names a variable present in the environment, its
  value rides as the Bearer; a 401/403 answer sets needsKey. The value
  never appears in detail.
- **FR-N1-2 (Go)** — `Desktop.ApplyCompatFirstRun(baseURL, modelID,
  apiKeyEnv, locality)` rewrites the FIRST-RUN template's brain to a
  single openai-compatible entry (locality declared; api_key_env only
  when non-empty), validates with config.Validate, and persists via
  WriteConfigAtomic to the controller/default path. It governs the
  first-run template only — the onboarding is its only caller.
- **FR-N1-3 (frontend)** — step Modelo gains the two sealed branches:
  Ollama (today's flow untouched) and «Servidor compatible OpenAI» with
  the four mold fields (base_url defaulted to http://localhost:1234/v1,
  model_id, api_key_env optional, locality local|cloud). [Comprobar]
  turns «Comprobando…» disabled (sealed); success names the model in the
  green line; failure speaks on screen with the fix at hand (unreachable
  → retry; model missing → says which id was not found; needsKey → points
  at Ajustes → Secretos, the B10 bridge). Siguiente stays gated on the
  ACTIVE branch's ok; switching branches discards the other's result.
  Closing the step on the compat branch applies FR-N1-2.

## Acceptance scenarios

- AS-1: compat branch, fake binding says model found → green line names
  the model; Siguiente enabled; ApplyCompatFirstRun called with the four
  fields on Siguiente.
- AS-2: model NOT in /models → on-screen failure naming the id, retry at
  hand, Siguiente gated.
- AS-3: needsKey → the failure points at Secretos (B10).
- AS-4: «Comprobando…» disabled while in flight (sealed).
- AS-5: the ollama flow behaves exactly as before (existing tests green).
- AS-6 (Go): /models probe against a deterministic fake server — found,
  missing, 401, unreachable; the env key rides as Bearer when present.
- AS-7 (Go): the applied first-run config validates and carries exactly
  the compat entry.

## Success criteria

`make quality` + `desktop-frontend-check` green, floors intact; zero
changes to the ollama path and to any non-first-run config surface.

## Decisions folded in

- Field-based compat branch (the lote mandate's mold) over the draft's
  /models dropdown — the validation of existence is the sealed essence;
  the picker can layer on later without breaking this shape.
- Apply-on-step-close (Siguiente) so the template is compat BEFORE any
  core boot the later steps trigger.
- The ollama branch's model-exists validation from the UX draft stays
  OUT of this lote (mandate: "el flujo ollama de siempre intacto") —
  recorded as a remainder for the map.

## `[NEEDS CLARIFICATION]`

None.
