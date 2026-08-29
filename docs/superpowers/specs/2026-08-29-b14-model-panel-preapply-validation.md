# B14 — model panel pre-Apply validation: Design Spec

> **Status:** approved for TDD (mandate: Chano's lote prompt, 2026-08-29;
> UX debt map row 23 — the 2026-08-23 live corruption, primary evidence in
> the map row). Governing ADRs: ADR-0030 §4/§5 (builder edit surface,
> server is the authority), ADR-0044 (compat base_url rules), ADR-0010
> (env-var NAME, never a value). External-docs note: no external API —
> TypeScript + existing builder modules + the WHATWG `URL` constructor
> (browser built-in) only.

## Goal

After this phase, the Builder's model panel validates its block BEFORE
Apply: a malformed `base_url`, a compat+cloud entry without `api_key_env`,
and the exact Sunday shape (a secret NAME glued into the URL) are stopped
at the panel with field-level messages in the Builder's existing error
mold — the first line of defense. The core stays the final judge
(`config.Validate` re-validates every POST); the panel validator is a UX
pre-check, deliberately NARROWER than the core, never a replacement. Out
of scope: fixing the corruption's input mechanism itself (reported
separately per the lote mandate), the legacy `ConfigEditor` form (it does
not expose `base_url`), and any server-side change.

## The Sunday mechanism (verified against source, hypothesis for the gesture)

Why the reload cut to `succeeded` with the corrupt block — verified in
`internal/config/config.go`:

- `https://openrouter.ai/api/v1OPENROUTER_API_KEY` IS a well-formed
  absolute https URL (the glued name rides as path); `validateCompatBaseURL`
  passes it.
- `api_key_env` is required by the core ONLY for `groq`; an
  openai-compatible entry with it emptied validates.
- with `locality` flipped to `local`, nothing else objects.

Each field individually plausible ⇒ the core validated the corrupt whole.
The INPUT gesture that produced it remains a hypothesis (candidate: a
native text drag moving the selected `OPENROUTER_API_KEY` from the
api_key_env input into base_url — a move both glues the text at the drop
point AND empties the source, two of the three symptoms in one gesture,
uncaught by React controlled inputs in WKWebView). Not reproducible in
jsdom; needs a manual WKWebView repro → reported as its own map row, NOT
fixed here (lote mandate c).

## Functional requirements

- **FR-B14-1** — A pure validator module (`web/builder/src/config/validate.ts`)
  exports `validateModel(m: ModelConfig): ModelFieldError[]` where
  `ModelFieldError = { field: 'base_url' | 'api_key_env' | 'model_id', message: string }`.
  Pure + Vitest-testable without a DOM (the `errors.ts` precedent).
- **FR-B14-2** — `base_url` rules: when present (or required —
  openai-compatible requires it), it must parse as an ABSOLUTE http(s)
  URL with a host (WHATWG `URL` + protocol/host check). Violation →
  a field error naming `base_url`.
- **FR-B14-3** — The Sunday shape: a `base_url` that embeds an
  env-var-NAME-shaped token (an ALL-CAPS run with at least one
  underscore, e.g. `OPENROUTER_API_KEY`) is rejected with a message
  saying a secret name appears glued inside the URL. Conservative by
  construction: legitimate endpoint prefixes do not carry
  `[A-Z0-9]+(_[A-Z0-9]+)+` tokens.
- **FR-B14-4** — Cloud-without-key: provider `openai-compatible` +
  locality `cloud` + empty `api_key_env` → a BLOCKING error on
  `api_key_env` (first line only — the core deliberately does not enforce
  this; the panel does, because a cloud endpoint without a key produced
  the mute console). `groq` + empty `api_key_env` mirrors the core rule.
- **FR-B14-5** — Panel wiring (`CanvasView` model panel): field errors
  render under their fields in the Builder's existing error mold
  (`field-err`, `role="alert"`), live as the operator edits.
- **FR-B14-6** — Apply gating: while any model in the working copy has a
  blocking error, the canvas save action does not POST — the button is
  disabled (with the panel showing why); `save()` refuses as the belt.
  The server 400 remains the backstop for everything else.

## Acceptance scenarios (Given / When / Then)

- **AS-1** Given a compat model whose `base_url` is `not a url`, When the
  panel validates, Then a `base_url` field error appears BEFORE any POST.
- **AS-2** Given `base_url` = `openrouter.ai/api/v1` (no scheme), Then a
  `base_url` field error (absolute http(s) required).
- **AS-3 (the Sunday case)** Given `base_url` =
  `https://openrouter.ai/api/v1OPENROUTER_API_KEY`, Then a `base_url`
  field error naming the glued secret name — rejected at the panel, not
  at the reload.
- **AS-4** Given provider `openai-compatible`, locality `cloud`,
  `api_key_env` empty, Then a blocking `api_key_env` error.
- **AS-5** Given a healthy compat entry
  (`https://openrouter.ai/api/v1`, cloud, `OPENROUTER_API_KEY` in
  api_key_env), Then zero errors — Apply stays enabled.
- **AS-6** Given a local ollama entry with empty `base_url`, Then zero
  errors (base_url optional off-compat; the core's own rules untouched).
- **AS-7 (component)** Given the model panel over an invalid entry, Then
  the field error renders in the mold (`role="alert"`) and the save
  button is disabled; fixing the field clears the error and re-enables.

## Success criteria

- Builder coverage gate intact (83/74/72/83 thresholds).
- `make quality` + `make desktop-frontend-check` green; e2e-binary suite
  green.
- Zero Go changes; zero changes to `config.Validate` (the judge is
  untouched).

## Decisions folded in

- Validator lives beside `errors.ts` as a pure module — the house pattern
  for testable UI logic.
- The secret-name heuristic is panel-only (never added to the core): a
  false positive there would block a weird-but-legal config at the UX
  layer only, where the operator can read the message; the core stays
  permissive per ADR-0044.
- Error copy reuses the Builder's bilingual-neutral technical register
  (English, the existing molde), matching the panel's current microcopy.
- The legacy ConfigEditor form is untouched: it has no base_url field —
  adding one is B-map work, not this guard.

## `[NEEDS CLARIFICATION]`

None — rules and cases pinned by the lote prompt and map row 23.
