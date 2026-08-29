# B13 — protocol JSON never reaches a channel: Design Spec

> **Status:** approved for TDD (mandate: Chano's lote prompt, 2026-08-29;
> behavior, error-surface language and conservative detection pinned there —
> UX debt map row 22, cause hunted in the 2026-08-23 bug bash).
> Governing ADRs: ADR-0021 (agent loop), ADR-0041 §5 (metadata-only audit),
> ADR-0042 §5 (native lane + text rescue). External-docs note: stdlib +
> existing `internal/` packages only — no external API is touched.

## Goal

After this phase, tool-call-shaped protocol JSON can no longer leave the
AgentBrain toward any channel. Today the text lane returns any non-`TOOL:`
content verbatim as the final answer (`agent.go` runLoop), and the native
lane's `rescueTextToolCall` deliberately passes through a tool-call-shaped
JSON whose name is NOT registered ("is an ordinary answer and passes
through untouched" — the documented fail-open that put
`{"name":"avisos-caseros",…}` raw into Chano's Telegram, with zero log
trace). B13 closes the fail-open at the single point both lanes share:
`Handle`'s final-answer seam, before `decisionToEnvelopes`. Out of scope:
rescuing/EXECUTING registered tool calls on the text lane (the guard only
blocks the leak), the B12 request-time failure surface, and a legitimate
answer that deliberately QUOTES an exact whole-body tool call (declared
out-of-scope in the debt map row).

## Functional requirements

- **FR-B13-1** — A registry-free shape detector recognises content whose
  ENTIRE meaningful body is one tool-call-shaped JSON object: after
  trimming whitespace and pure code-fence delimiter lines (the
  `firstMeaningfulLine` noise classes), the body parses as a single JSON
  object carrying a string `"name"` and at least one of
  `"arguments"`/`"parameters"`/`"args"`. Seam: `internal/brain`
  (refactor of `rescueTextToolCall`'s shape half; the rescue keeps its
  registered-name check on top — additive, no behavior change for
  registered rescues).
- **FR-B13-2** — In `Handle`, a final answer matching FR-B13-1 is NEVER
  sent to the channel. The outbound reply is replaced by a brief honest
  error in the house language of the channel surface (English, the
  `defaultFallback` register): the user learns the model produced an
  internal tool request instead of an answer.
- **FR-B13-3** — The block is observable: one structured WARN in the local
  log carrying the phantom tool name (bounded, `boundedArgs` — the name is
  model-controlled), brain and channel + one metadata-only audit event
  with FINITE labels (`Tool: "unknown"`, `Rule: "protocol_leak"`,
  `Outcome: "denied"` — the `unknown_tool` grammar).
- **FR-B13-4** — The leaked JSON is NOT persisted to conversation memory
  (the poisoned-context lesson, 2026-08-09 `96d0e5a`): a guarded reply
  persists nothing, like the fallback path.
- **FR-B13-5** — A message that merely CONTAINS JSON amid normal text, or
  a JSON object without the tool-call key pair, passes through untouched —
  the guard inspects the whole-body shape only.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (the exact incident)** Given an AgentBrain whose model answers
  `{"name": "avisos-caseros", "parameters": {…}}` as its final text (a
  tool name registered NOWHERE), When `Handle` processes a telegram
  message, Then the outbound text does NOT contain the JSON (neither
  `avisos-caseros` nor `"name"`), the reply is the honest-error text, and
  the log carries a WARN naming the phantom tool.
- **AS-2 (native lane, same leak)** Same content arriving as the native
  lane's final text (no ToolCalls, unregistered name) → same block.
- **AS-3 (the pinned edge)** Given a final answer that CONTAINS a
  tool-call-shaped JSON between normal prose (a user asking for help with
  code), When handled, Then it reaches the channel untouched.
- **AS-4 (plain JSON answers survive)** A whole-body JSON object WITHOUT
  the name+args pair (`{"name": "Chano", "city": "Sevilla"}`) passes
  untouched (existing test stays green unmodified).
- **AS-5 (fenced variant)** The same tool-call JSON wrapped in a pure
  ```-fence is still blocked — fence delimiters are formatting noise, the
  body shape is unchanged.
- **AS-6 (no persistence)** With a conversation store mounted, a guarded
  reply appends NO turns.
- **AS-7 (registered rescue intact)** The native lane's registered-name
  rescue still executes through the governed path (existing tests green).

## Success criteria

- `internal/brain` coverage stays ≥ 90 (house floor for brain).
- `make quality` green with `-race` over the whole suite.
- Zero changes outside `internal/brain`; the only existing-test change is
  the documented reversal of the fail-open comment's contract (none of the
  three existing rescue tests asserts the unregistered-name LEAK reaching
  the user, so no approved test needs weakening — verified before RED).

## Decisions folded in

- Guard placement at `Handle` (not inside each lane): one seam covers text
  + native + the RT-3 degrade path; rationale = the lote mandate ("punto
  común de salida").
- The honest error does not echo the phantom tool name (model-controlled
  content stays out of the user surface; the name lives in the bounded
  local WARN only).
- The guarded reply reuses the fallback non-persistence semantics.
- Fence-stripped detection (AS-5) included: same mechanism, real leak
  vector, still whole-body-shape conservative.

## `[NEEDS CLARIFICATION]`

None — behavior, error surface and edge case are pinned by the lote
prompt and debt-map row 22.
