# Governed-tools piece · SP6 — the builder governance panel: Design Spec

> **Status:** approved for TDD (director's visual OK on the
> `design-drafts/governance-panel/` mockups, 2026-08-15 — those PNGs are the
> visual contract, as final-6 was for the canvas).
> Governing ADRs: ADR-0041 (governed tools, tri-state grants, cages, shield,
> skills), ADR-0030 (builder visual identity + config→UI→config projection,
> §4 the working-copy-POST model), ADR-0015 (declared-not-inferred
> attributes). External-docs note: **no new external API** — the panel reuses
> the builder's existing stack exactly (React `useReducer`, `@testing-library/react`
> + Vitest, Playwright e2e, the `config/edit.ts` pure-reducer pattern). The
> serialization matrix is an EXACT mirror of the Go `config.AgentConfig`
> (`internal/config/config.go:342-408`), derived from the real validator, not
> invented.

## Goal

Deliver the **"Herramientas y skills"** section inside the builder's agent-brain
properties panel: the operator edits tool governance visually — tri-state grant
per tool, channel scoping, attribute overrides with the derived network shield,
the three cages, and skills — and the section serializes back into
`brain.agent` exactly as the Go schema expects, governed by the existing
Apply/Discard/counter header (the hot shadow→allow promotion IS the usual
Apply). Afterwards: a non-technical operator promotes a rehearsed (shadow) tool
to live (allow) with one click, sees the network shield as a derived fact, and
edits cage allow-lists without hand-writing JSON. Explicitly OUT: no new tool
types, no per-skill toggles (skills stay read-only detected list), no cage
validation beyond what the server already enforces (the 400 is the backstop),
no builder support for non-agent brains growing this section.

## Functional requirements

- **FR-SCHEMA-1** — The TS `AgentConfig` mirror
  (`web/builder/src/config/schema.ts`) is extended to the EXACT Go espejo:
  `governance?: ToolGrantConfig[]`, `tool_attrs?: Record<string, ToolAttrsConfig>`,
  `read_file?/http_fetch?/webhook_call?` cage configs, `skills_dir?`,
  `skills_body_budget?` — every field with the Go `omitempty`/optionality and
  the `*bool` overrides as `boolean | undefined`. Additive to a shared file;
  blast radius = the mirror only (the server re-validates every POST).
  Traces to `config.AgentConfig`. New enum `TOOL_MODES = ['allow','shadow','deny']`
  and `CATALOG_TOOLS` (the built-in names: `time,echo,calc,read_file,http_fetch,
  webhook_call`) mirroring `tool.BuiltinAttrs` — a UX convenience, server is
  truth.

- **FR-EDIT-1** — Pure reducer actions in `config/edit.ts` (the config→config
  transitions, testable without a DOM — the house criterion since v0.6.0), one
  per panel control, each mutating ONLY `brains[i].agent`: `setToolMode`
  (allow/shadow/deny — creates/updates the grant, deletes it on the ungoverned
  default), `setToolChannels`, `setToolAttrOverride` (sensitive/network `*bool`,
  clearing back to house default), `setCageField` (read_file root/max_bytes;
  http_fetch/webhook_call caps), `addCageHost`/`setCageHost`/`removeCageHost`
  (the allow-list editor), `setSkillsField` (dir/budget). Every action preserves
  untouched fields (the round-trip guarantee) and is a no-op-safe pure function.

- **FR-DERIVE-1** — The network **shield** is a DERIVED, non-editable UI state:
  shown (amber-dashed pill + lock) when `brain.sensitivity === 'private'` AND
  the tool's effective `network` attribute is true (house default OR override).
  It is computed, never stored — mirrors `ToolDecision.Shield` (`caged.go`); no
  reducer action writes it.

- **FR-WARN-1** — The safe-default warning: when an agent brain has NO
  `governance` block AND lists a tool whose effective `sensitive` attribute is
  true AND its single model's `locality === 'cloud'`, the panel renders the
  house warning (mirrors the boot guard `ErrSensitiveToolUngoverned`, estreno
  E-11). Presentation only — the server still rejects the boot; the panel warns
  before Apply.

- **FR-UI-1** — `ConfigEditor.tsx` renders the section ONLY inside a brain whose
  `agent` block is present (`b.agent !== undefined`). A non-agent brain (no
  `agent`) never shows it. The section uses the mockup's tokens and classes
  verbatim (the final-6 contract): the tri-state segmented control
  (violet=allow, amber-dashed=shadow, neutral=deny), attribute chips, channel
  chips, the derived shield pill, the cage editors, the read-only skills list.

- **FR-GOV-1** — The section is governed by the EXISTING header: edits flow
  through the same working-copy reducer, so `pendingChangeCount` counts them and
  Apply POSTs the whole config. Promotion shadow→allow is a `setToolMode` edit
  applied by the ordinary Apply — no separate promote endpoint (the per-cycle
  bearer path stays sealed, ADR-0035 §4).

## Acceptance scenarios (Given / When / Then)

- **AS-1** Given an agent brain with `agent.tools=['read_file']` and no
  governance, When `setToolMode(brain,'read_file','shadow')`, Then
  `agent.governance` becomes `[{tool:'read_file',mode:'shadow'}]` and every other
  field is byte-identical (round-trip Vitest).
- **AS-2** Given the grant above at `shadow`, When `setToolMode(...,'allow')`,
  Then the grant's mode is `'allow'` (the promotion), and `pendingChangeCount`
  reports the brain as one change.
- **AS-3** Given a grant at some mode, When `setToolMode(...,'deny')` is the
  ungoverned-equivalent reset the operator chooses, Then deny is stored
  explicitly as `{mode:'deny'}` (deny is NOT the same as absent — absent means
  advertise+execute; deny means neither).
- **AS-4** Given `tool_attrs` absent, When `setToolAttrOverride(brain,'http_fetch',
  'network',false)`, Then `agent.tool_attrs={'http_fetch':{network:false}}`; When
  the override is cleared, Then the key (and `tool_attrs` if now empty) is removed
  — back to the house default (declared-not-inferred).
- **AS-5** Given a private brain listing `http_fetch` (network house-default true)
  with no `network:false` override, When the panel renders, Then the shield pill
  shows for `http_fetch`; When sensitivity is `public`, Then it does not; When an
  override sets `network:false`, Then it does not (FR-DERIVE-1 truth table).
- **AS-6** Given a public agent brain, cloud model, `tools=['read_file']`
  (sensitive house-default true), no governance, When the panel renders, Then the
  house warning is visible naming the tool; When a governance block exists, Then
  it is not.
- **AS-7** Given `http_fetch` cage with `allow_hosts=['a']`, When `addCageHost`
  then `setCageHost(...,1,'b')` then `removeCageHost(...,0)`, Then
  `allow_hosts=['b']`.
- **AS-8** Given a NON-agent brain (no `agent` block), When the properties panel
  renders, Then no "Herramientas y skills" section exists (queryable-absent test).
- **AS-9 (e2e, Playwright)** Given the real built UI with an agent brain, When the
  operator opens it, sets a grant to Ensayo, Apply → the POSTed config carries
  `mode:'shadow'` and the subsequent GET returns it; When they promote to
  Permitir, Apply → the config carries `mode:'allow'` — the full hot-promotion
  round trip, verified end to end.
- **AS-10** Given the skills fields, When the panel renders a config with
  `skills_dir` and a detected list, Then the dir + budget are editable and the
  skill names/descriptions are read-only (no toggle control emitted).

## Success criteria

- Vitest: the `config/edit.ts` reducer additions at 100% of the new branches;
  the component projection tests (config→UI→config) green; existing builder
  suites unregressed.
- `tsc --noEmit`, ESLint, Prettier all clean on the touched files.
- Playwright e2e (AS-9) green on its own port (the zombie-preview lesson: a
  dedicated port, killed after).
- `make quality` green with `-race` over the WHOLE suite (the Go core is
  UNCHANGED — proven by no diff under `internal/`; the frontend gate is the
  builder's four jobs).
- Visual fidelity: the built UI's three states match the mockup PNGs
  side-by-side (captures to `design-drafts/governance-panel/built/`); any
  deviation stops and reports, never improvises.
- Untouched: the headless binary and all Go packages (`git diff --stat internal/`
  empty), `web/builder/dist` rebuilt fresh by the existing CI job.

## Decisions folded in

- **Deny is explicit, not absence.** The mockup's tri-state has three positions;
  absence of a grant is the ungoverned advertise+execute default, so choosing
  "Denegar" stores `{mode:'deny'}` — matching `policy.ToolDeny` (neither
  advertise nor execute). The operator reaches the ungoverned state by removing
  the grant, a separate (future) affordance, not by picking deny.
- **The shield and the warning are pure derivations** in the component (FR-DERIVE-1,
  FR-WARN-1), never reducer state — they can never drift from sensitivity/attrs
  because they are recomputed each render from the same source the server reads.
- **Skills stay read-only.** The mockup shows a detected list; per-skill toggles
  are out of scope (no schema field for them) — dir + budget are the only
  editable skill controls.
- **No new promote endpoint.** Hot promotion is a `setToolMode` edit + the
  existing Apply (ADR-0035 §4 keeps the in-app control path sealed).

## `[NEEDS CLARIFICATION]`

None. The Go schema is the authority for every field, the mockups are the
authority for every pixel, and both were verified in source before writing this
spec.
