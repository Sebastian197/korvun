# Piece BUILDER-CANVAS — visual node editor + personality per brain: Design Spec

> **Status:** implemented (2026-08-01) — SP0–SP4 code-complete on the local
> batch (`722a20e` → `f7b00f2`), ADR-0039 `accepted`. All clarifications were
> resolved before TDD (see "Clarifications resolved" below). Pending: Chano's
> visual smoke → batch rehearsal → push.
>
> **Governing ADRs (inherited as law):** ADR-0015 (pre-dispatch privacy
> selector — Public keeps all models, Private keeps Local only), ADR-0017
> (dispatch shapes: fan-out / sequential), ADR-0021 (AgentBrain + system
> prompt), ADR-0022 (read-only control API: `/api/brains`, `/api/channels`
> secret-free summaries), ADR-0024 (fixed event palette, SSE `/ui`), ADR-0027
> (config mutation → supervisor reload/rebuild; 202 + status handle; the two
> `409` codes `config_would_self_lock` / `reload_in_progress`; single-flight
> lock), ADR-0028 (admin bearer, CSRF-by-construction, F10 cleartext heuristic),
> ADR-0029 (frontend toolchain: React 19 + TS strict + Vite + Tailwind v4;
> Vitest + Playwright; zero CDN), ADR-0030 (builder visual identity + UI
> architecture: violet functional accent, teal-gradient identity-only, AA
> contrast gate, working-copy edit model, reload state machine), ADR-0010
> (secrets env-only), ADR-0033 (Discord channel), ADR-0035 (desktop app,
> same-origin `/builder/` iframe), ADR-0038 (Webhook channel).
>
> **New ADR this piece delivers:** **ADR-0039** — frontend canvas dependency
> (React Flow). Next free number verified on disk (last is `0038-…`).
>
> **External-docs note — READ THIS FIRST.** This piece adds one production
> frontend library (a node-graph canvas; MASTER §4.2 names **React Flow**).
> Per CLAUDE.md the exact npm package identity, current version, public API, and
> **React 19 compatibility** MUST be verified through **Context7** BEFORE any
> canvas code. **Context7 was NOT connected in the session that authored this
> spec** — so this document does not state React Flow's package name, version,
> or API from memory, and it must not be treated as verification. That
> verification is the first gate of SP0 (the integration spike) and a blocking
> input to ADR-0039 (NC-1, resolved by process: Context7-only verification as the
> SP0 gate; if the spike fails on CSP/incompat/iframe, STOP and the copilot
> decides on the spike data). Everything else in this piece uses only stdlib,
> existing `internal/` packages, and the existing `web/builder` sources — **no**
> other external library is proposed (NC-2, resolved: zero second frontend
> dependencies).

---

## Goal

After this piece the Korvun builder is a **visual node editor**: a draggable
palette of the three channel blocks (Telegram / Discord / Webhook), a canvas of
channel / brain / model nodes with connections, the **privacy exclusion made
visible** (a cloud model excluded by a private brain rendered as a dimmed,
dashed, non-connectable edge/node), and a **properties panel** that edits the
selected node — including a new **personality per brain** (display name, tone,
language, system instructions) that composes into the brain's system prompt.
Saving the graph produces exactly the `config.Config` JSON the core already
reads, applied through the **existing** Phase-2a hot-apply reload machine (no new
reload path). This **extends** the existing `web/builder` (React 19 form editor
over a working-copy reducer + reload state machine); it does not rewrite it. The
reducer (`config/edit.ts`), the API client (`api.ts`), the reload machine
(`config/reload.ts`), the error mapping (`config/errors.ts`), and the design
tokens (`design/tokens.ts`) are **reused as-is** or extended additively.

Explicitly OUT of this piece (deferred): the universal model gate (provider set
stays `ollama` / `groq` as today); tools/skills governance and minimal memory
(separate ROAD-TO-BETA pieces); operator manual-reply/takeover; any change to
the headless message hot-path; auto-layout of an existing config beyond the
deterministic column placement fixed by NC-6 (canals | brains | models, stable by
config index; positions are never persisted in v1 — not in the core config, not
in `localStorage`).

---

## Current state (verified first-hand, 2026-08-01)

- **The builder is a single-page form editor, not a canvas.** `web/builder/src/App.tsx`
  renders: a header, the F10 cleartext warning (`cleartext.ts`), two read-only
  summary panels (Brains / Channels from `getBrains()` / `getChannels()`,
  **unauthenticated**), and a bearer-gated `Config` panel that mounts
  `ConfigEditor` once the operator pastes the admin token (held in React state
  only, ADR-0030 §6).
- **The edit surface is `ConfigEditor.tsx` (481 lines).** It edits a deep-cloned
  working copy through the pure reducer `configReducer` (`config/edit.ts`),
  tracks `dirty = isDirty(wc, base)`, and on Save calls `postConfig` →
  `pollReload` → on `succeeded` re-baselines via `getConfig`. `locked =
  reload.phase === 'polling'` disables the whole `<fieldset>` during cutover.
- **The reload state machine already exists and is pure** (`config/reload.ts`):
  server states verbatim from the supervisor (`pending`, `cutover-in-progress`,
  `succeeded`, `rolled-back`, `failed`); UI phases `idle | polling | succeeded |
  rolledBack | failed | unknown`; **a transient net error during cutover keeps
  polling, never maps to `failed`** (ECONNREFUSED-as-retry, ADR-0027 F4);
  `timeout → unknown`.
- **Serialization is already lossless.** `clone` is `structuredClone(baseline)`;
  every reducer branch returns a new `Config` preserving untouched fields
  (`storage`, `observability`, `admin`, per-brain `retry`, per-brain `agent`,
  per-model `base_url`, etc.). `getConfig` → edit → `postConfig(wc)` round-trips.
- **Backend `BrainConfig`** (`internal/config/config.go`): `Name`, `Sensitivity`
  (`public|private`), `Policy{Kind, Order}`, `Dispatch` (`fanout|sequential`),
  `Models[]`, `Retry *bool`, `Agent *AgentConfig`. **`system_prompt` exists
  today only under `AgentConfig`** (`Tools`, `MaxIterations`, `SystemPrompt`),
  and `app.go:677` wires it via `WithAgentSystemPrompt` **only for agent
  brains**. A plain Orchestrator brain currently receives **no** system prompt
  (`brain.WithSystemPrompt` exists but is not wired from config). This is the
  gap the persona sub-phase fills.
- **`/api/brains` publishes survivors only.** `brainSummary` (`app.go:582`)
  appends a model to the summary **iff** `sens == Public || loc == Local`
  (ADR-0015). For a private brain, cloud models are **absent** from the summary —
  the API does not expose the excluded model. `TestBrainSummary_matchesSelector`
  cross-checks this against the real selector.
- **Frontend schema is stale vs. backend channels.** `config/schema.ts`
  `CHANNEL_TYPES = ['telegram']` and `CHANNEL_MODES = ['polling']`, but the
  backend now has Discord (ADR-0033) and Webhook (ADR-0038). The 3-channel
  palette in the mockup requires bringing these enums current.
- **CSP is strict, no `style-src` override.** `web/builder/embed.go`:
  `default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'self'`.
  React applies inline styles via the CSSOM (`element.style`), which CSP does not
  govern, so the current builder's inline styles work; a bundled stylesheet is
  same-origin and fine. Any library that injects a runtime `<style>` element, an
  `@import`, or loads a font/asset from a CDN would violate this CSP. That is a
  spike acceptance criterion, not an assumption (see SP0 and `NC-1`).
- **Desktop integration is an iframe** (`cmd/korvun-desktop/frontend/src/views/BuilderEmbed.tsx`):
  when the core is running it renders `<iframe src="/builder/">` (same-origin via
  the SP4 proxy; `frame-ancestors 'self'` allows exactly this); otherwise it
  paints an honest stopped state. No desktop-side change is needed for the
  canvas to appear — but the canvas must behave identically inside the iframe
  (see FR-CTX and `NC-5`).
- **Current deps** (`web/builder/package.json`): only `react` / `react-dom`
  `19.2.7`, Tailwind `4.3.2`, Vite `8.1.3`. No node-graph, drag-and-drop, state,
  or layout library is present — React Flow is a genuinely new dependency.
- **Tooling for the success criteria already exists:** Playwright
  (`e2e/builder.spec.ts`, `playwright.config.ts`) and `@axe-core/playwright`.

### Normative mockups (cited by name)

- **`design-drafts/final-6-builder-estados.png`** — the reference layout: desktop
  shell (sidebar Inicio / Builder / Canales / Actividad / Ajustes) around the
  builder; a three-column builder = left palette (CANALES: Telegram / Discord /
  Webhook · CEREBROS: Cerebro · MODELOS: Ollama `local` / Groq `nube`), center
  canvas (dotted grid; channel, brain, model nodes; teal/green connections),
  right **Propiedades** panel bound to the selected `cerebro` node (NOMBRE,
  SENSIBILIDAD Privado/Público, DESPACHO Fanout/Secuencial, POLÍTICA
  `prioridad: ollama → groq`, MODELOS list + `+ Añadir modelo`, `Eliminar
  nodo…`). Hot-apply bar: `1 cambio sin aplicar` · `Descartar` · `Aplicar
  cambios`. This mockup fixes: **channel / brain / model are the node kinds;
  agent and policy are brain PROPERTIES, not nodes.**
- **`design-drafts/final-6b-builder-aplicando.png`** — the same view in the
  **applying** state: top-right `Aplicando — el gateway sigue en marcha…` with a
  spinner; the panel stays visible.
- **`design-drafts/Korvun Desktop (standalone).html`** — the interactive builder
  prototype. Its "Builder · momentos" enum is `— | cambios sin aplicar |
  aplicando | conexión inválida`. It is the **normative source** for two states
  the PNGs do not show:
  - **Privacy exclusion:** a private brain renders its cloud model with chip
    `nube · excluido (privado)` and node/edge style `opacity:.6;
    border-style:dashed`; microcopy *"Privado: los modelos en la nube se excluyen
    antes de llamar — lo sensible no sale de tu máquina."* The `excl` flag is
    derived from the brain's sensitivity, not from the API.
  - **Invalid connection:** dragging a connection from a channel port onto a
    model triggers `kvReject` animation + toast *"Un canal no puede ir directo a
    un modelo — conéctalo a través de un cerebro"*; dropping on a brain input is
    valid.
- **`design-drafts/sp6c-builder.png`** — the current desktop embed placeholder
  ("Korvun builder not built. Run `make build`"), i.e. the state this piece
  replaces once the real canvas is bundled.

### States the design track left UNPRODUCED (flagged per the brief)

The mockups cover: clean (`—`), dirty (`cambios sin aplicar`), applying
(`aplicando`), applied-OK toast, and invalid-connection. They do **not** provide
a canvas visual for the following existing behaviors, which this piece must still
render (adapting copy to the canvas visual language) — this is a design gap, not
a licence to drop them:

1. **`rolled-back`** and **`failed`** terminal reload states (`ReloadView` err
   panel today).
2. **`unknown`** reload state (total-timeout / unrecognized server string).
3. **`reload_in_progress` `409`** (a reload already running) and
   **`config_would_self_lock` `409`** (config removes the admin token).
4. **`400` validation** surfaced inline on the offending node, and **`401`**
   (token cleared → re-auth).
5. **Discard confirmation** ("Discard changes?") and **remove-node
   confirmation** ("Remove brain?").

NC-3 (resolved): these reuse the 2b error components (`ReloadView` /
`SaveErrorView` / confirmations) with canvas-adapted copy — no new error-state
mockups; the design system is the law.

---

## Functional requirements

Grouped by area. Additive changes to shared packages are marked with blast
radius.

### A. Scope & relation to the existing builder (brief item a)

- **FR-SCOPE-1** — The canvas becomes the **primary and only** builder view.
  `ConfigEditor`'s form fields are **re-homed** into the right-hand **Propiedades**
  panel, bound to the currently selected node. The current read-only Brains /
  Channels summary cards (`App.tsx` `<main class="grid">`) are subsumed by the
  canvas nodes. Decision rationale: `final-6` shows one integrated
  palette+canvas+panel surface with a single hot-apply bar and **no tab toggle**
  between "forms" and "canvas". Seam: `App.tsx` restructured; `ConfigEditor.tsx`
  decomposed so the field groups (`BrainForm`, `ChannelForm`, `RouteForm`,
  `ModelRow`) become panel content, while its state wiring (`useReducer` over
  `configReducer`, `save`/`discard`, `pollReload`, `ReloadView`, `SaveErrorView`)
  is **lifted and reused unchanged**.
- **FR-SCOPE-2** — The bearer gate and F10 cleartext warning are preserved
  verbatim: the canvas mounts only after the operator pastes the admin token; the
  token stays in memory only (ADR-0030 §6, ADR-0028).
- **FR-SCOPE-3** — Routes, which have no dedicated node in the mockup, are
  represented by **channel→brain edges** (see FR-CANVAS-3). `RouteForm` as a
  standalone panel is removed; a route is created/removed by drawing/deleting the
  channel→brain edge. Fields not represented by any node or edge (`storage`,
  `observability`, `admin`) are **not** surfaced by the canvas and are preserved
  untouched through the round trip (FR-SER-2).

### B. Personality per brain — Go first (brief item b)

- **FR-PERSONA-1 (additive `internal/config`)** — Add an additive personality
  block to `BrainConfig` (field set fixed by NC-4, resolved):
  `persona` with `display_name`, `tone`, `language`, `instructions` — **all
  optional free-text with length caps** (the exact caps are set by the SP1 red).
  `display_name` is **presentation only**; `name` remains the routing key and is
  untouched. Omitted `persona` = today's behavior byte-for-byte. Blast
  radius: `config.Config` is shared by `internal/app`, `internal/router` wiring,
  the TS mirror (`schema.ts`), and every existing `korvun.json`. Backward compat
  is a hard requirement: existing configs (no `persona`) MUST validate and behave
  exactly as before.
- **FR-PERSONA-2 (composition, `internal/app` + `internal/brain`)** — A pure
  `composePersona(persona) → string` assembles the persona fields into a system
  prompt fragment. Composition is fixed by NC-4 (resolved): **the persona is a
  PREFIX of the system prompt.** For an Orchestrator brain, via
  `WithSystemPrompt(composePersona(p))` (this newly exercises the config wiring
  absent today). For an AgentBrain, `composePersona(p)` is **prepended to the
  existing `buildSystemPrompt` WITHOUT altering its internal order** — the
  ADR-0021 tool protocol block stays intact, both contributions are present, and
  nothing is overwritten.
- **FR-PERSONA-3 (validation)** — `config.Validate` gains persona validation
  (bounds/enum for `tone`/`language` if constrained; length caps on
  `instructions`) consistent with the house error style (`%w` wrap of
  `ErrInvalidConfig`, indexed `brains[i].persona.…` field path so the builder's
  `400`-inline mapping can target it, mirroring `errorFor`).
- **FR-PERSONA-4** — This is a **standalone Go sub-phase (SP1)** with TDD
  (red→green) **before** any frontend renders it. Coverage floor ≥90%
  (`brain`/`config` are house-critical packages).

### C. Canvas dependency — React Flow (brief item c)

- **FR-DEP-1** — Adopt one node-graph canvas library (MASTER §4.2: React Flow),
  self-hosted, bundled by Vite, **zero CDN** (ADR-0029). Its stylesheet is
  imported and bundled same-origin. Exact package identity, version, and API are
  **verified EXCLUSIVELY via Context7** as the first action of SP0 and recorded in
  ADR-0039 (NC-1, resolved by process). No version or symbol is asserted in this
  spec.
- **FR-DEP-2** — The library MUST run clean under the existing CSP
  (`default-src 'self'`, no `unsafe-inline`, no CDN): **zero CSP violations** in
  the console for a mounted graph, with all fonts/assets bundled. If it cannot
  (e.g. it injects a runtime `<style>` or requires `unsafe-inline`), that is a
  spike failure — do **not** silently relax the CSP; STOP and let the copilot
  decide on the spike data (NC-1, resolved by process).
- **FR-DEP-3** — The canvas uses **only** this one library plus what
  `web/builder` already has (React 19, Tailwind v4). Drag-and-drop from the
  palette, node/edge state, and layout are implemented with the canvas library's
  own primitives and/or plain React + the existing reducer — **no** additional
  drag-and-drop, global-state, or layout library (NC-2, resolved: **zero** second
  frontend dependencies in this piece). Valve: any temptation STOPS and earns its
  own NC/ADR rather than being added in passing.
- **FR-DEP-4** — The root `go.mod` and the nested `web/builder` `go.mod` are
  untouched (React Flow is an npm dependency). Proven by `go version -m` /
  `go.mod` diff in SP4.

### D. The canvas — nodes, connections, validity, privacy (brief item d)

- **FR-CANVAS-1 (node kinds)** — Three node kinds only: **channel**, **brain**,
  **model**. Evidence: `final-6` renders exactly these; the brain node carries
  agent/policy/dispatch/sensitivity as **inline tags and panel properties**, not
  as separate nodes (`asistente` shows `público`, `secuencial`, `prioridad:
  ollama → groq`). Decision folded in: **agent and policy are brain properties,
  not node kinds.**
- **FR-CANVAS-2 (palette)** — A left palette groups CANALES (telegram / discord /
  webhook), CEREBROS (a new brain), MODELOS (ollama `local` / groq `nube`).
  Dragging a palette block onto the canvas dispatches the matching reducer add
  action (`addBrain`; new `addChannel`; and `addModel`-at-brain — a model block
  is always dropped **onto a brain**, never as an orphan node, NC-6 resolved).
  Empty-canvas copy: *"Arrastra un canal desde la paleta para empezar. Después
  conéctalo a un cerebro y a sus modelos."*
- **FR-CANVAS-3 (connection validity, domain-derived)** — Valid edges:
  **channel → brain** (creates a `route`) and **brain → model** (adds the model
  to the brain's `models`). Every other drag is **invalid and visibly rejected**:
  a channel dropped on a model shows the `kvReject` animation + toast *"Un canal
  no puede ir directo a un modelo — conéctalo a través de un cerebro"* (standalone
  prototype, verbatim), and no config mutation occurs. Per NC-6 (resolved) the
  validity matrix is an **exact mirror of `config.Validate`** — no more permissive,
  no more restrictive — and the SP2/SP3 tests **derive it from the real validator,
  never invent it**. Two brains with the same provider are two config entries and
  therefore two nodes (schema-mirrored). The domain rules come from
  `internal/config` (routes are channel→brain; a brain owns models).
- **FR-CANVAS-4 (privacy exclusion visible)** — For a brain whose `sensitivity`
  is `private`, every non-`local` model connected to it is rendered **excluded**:
  dimmed + dashed node/edge (`opacity:.6; border-style:dashed`) with chip `nube ·
  excluido (privado)` and the microcopy from the prototype. **Source of the
  exclusion:** the frontend **replicates the ADR-0015 selector rule locally**
  (Public keeps all; Private keeps Local only) over the working-copy config —
  because `/api/brains` **omits** excluded models entirely and so cannot drive the
  edge, and the config the builder edits is where full model localities live.
  Decision folded in: **the frontend derives the exclusion; `/api/brains` is at
  most a cross-check, never the source.** The rule is implemented as a pure
  function mirroring `brainSummary`, and an SP2 test pins it against the same
  cases as `TestBrainSummary_matchesSelector` so the two cannot diverge.

### E. Serialization — graph ⇄ config round trip (brief item e)

- **FR-SER-1** — Two pure functions, `configToGraph(config) → {nodes, edges}`
  and `graphToConfig(graph, base) → Config`, are the only bridge between the
  canvas and the config. `graphToConfig` produces **exactly** the `config.Config`
  shape the core reads (POSTed whole via the existing `postConfig`).
- **FR-SER-2 (lossless round trip)** — `configToGraph` then `graphToConfig`
  preserves every field the canvas does not edit (`storage`, `observability`,
  `admin`, `retry`, `agent` internals not surfaced, `base_url`, and — until the
  panel edits them — persona fields), byte-identical under `JSON.stringify`. This
  extends the existing guarantee (`config/edit.ts` already preserves untouched
  fields; the canvas must not regress it). SP2 red tests assert round-trip on the
  real `korvun.example.json` and on a config carrying every currently-unedited
  field.
- **FR-SER-3** — The canvas continues to edit through `configReducer`
  (`config/edit.ts`), extended additively with the actions the panel needs:
  `addChannel`, `addModel`-at-brain (a model is always added onto a brain — **no
  orphan model nodes**, NC-6 resolved), `setPolicyOrder` (the mockup's `prioridad:
  ollama → groq` — not editable today), `addRoute`/`removeRoute` (edges), and
  persona setters. Each is a pure branch with the round-trip guarantee; no
  existing branch changes behavior.

### F. Hot-apply — reuse the existing reload machine (brief item f)

- **FR-HOT-1** — The canvas reuses the **existing** save/reload path verbatim:
  `postConfig` (202 → handle) → `pollReload` → re-baseline on `succeeded`. **No
  new reload endpoint, no duplicated state machine.**
- **FR-HOT-2 (state mapping)** — The mockup hot-apply bar maps onto the existing
  machine and dirty tracking:
  - `1 cambio sin aplicar` / `N cambios` ← `dirty = isDirty(wc, base)` plus a
    **diff count** (new; today the UI shows only `unsaved changes` text).
  - `Descartar` ← `discard()` (with the existing confirm step).
  - `Aplicar cambios` ← `save()`.
  - `Aplicando — el gateway sigue en marcha…` ← `reload.phase === 'polling'`
    (`pending` / `cutover-in-progress`) + `locked` fieldset. Copy changes; state
    is the existing one.
  - applied-OK toast ← `succeeded`.
- **FR-HOT-3 (lock during cutover)** — While `reload.phase === 'polling'` the
  canvas is **locked** (no drag, connect, delete, or panel edit), the exact
  analogue of today's `<fieldset disabled={locked}>`.
- **FR-HOT-4 (transient net error)** — The ECONNREFUSED-as-retry rule is
  inherited untouched from `config/reload.ts`; the canvas never surfaces a
  transient cutover-restart error as `failed`.
- **FR-HOT-5 (unproduced states)** — `rolled-back`, `failed`, `unknown`, the two
  `409`s, `400`-inline, `401`, and the confirmations (see "unproduced states"
  above) MUST still render on the canvas. Per NC-3 (resolved) they **reuse the 2b
  error components already designed** (`ReloadView` / `SaveErrorView` /
  confirmations) with copy adapted to the canvas — **no new error-state mockups**:
  the design system is the law (the AA-over-mock precedent).
- **FR-HOT-6 (invalid connection ≠ reload state)** — Invalid-connection
  (FR-CANVAS-3) is a **client-side pre-apply** validity state, independent of the
  reload machine; it never blocks or interacts with an in-flight reload beyond the
  general lock.

### G. Execution contexts (brief item g)

- **FR-CTX-1** — The canvas works identically in the **direct browser** context
  (`/builder` with a pasted bearer) and **embedded** in the desktop iframe
  (`/builder/` via the SP4 proxy; `frame-ancestors 'self'`; same-origin;
  `cleartext.ts` already recognizes the Wails origins so the F10 warning does not
  false-positive there).
- **FR-CTX-2 (iframe risks to check)** — Per NC-5 (resolved), drag-and-drop,
  pointer-capture, and focus behavior of the canvas library **inside the WebView
  iframe** (not only a browser tab) is a **SP0 acceptance criterion**. No
  desktop-side (`BuilderEmbed.tsx`) change is expected; if one proves necessary
  that is a finding to raise, not to absorb.

### H. Visual identity — reconcile tokens (brief item h)

- **FR-ID-1 (ADR-0030 governs)** — The canvas conforms to the ADR-0030 token
  system (`design/tokens.ts` + `styles/theme.css`), the executable
  WCAG-AA-gated source of truth (`tokens.wcag.test.ts`). Violet is the functional
  accent for primary actions and node selection; the fixed event palette drives
  status; the teal→violet gradient stays **identity-only** (glyph / hairline).
- **FR-ID-2 (reconcile mockup functional teal)** — The mockups use teal
  functionally (the `Aplicar cambios` gradient button; the armed-port highlight
  `border-color: var(--teal)`), which **conflicts** with ADR-0030 §1 (teal
  rejected as a functional accent because it collides with the `sent` green; the
  review rejected gradient-filled controls). **NC-7 — Chano's decision
  (2026-08-01): VIOLET functional.** ADR-0030 §1 stays intact; the functional teal
  usages in the mockups (the `Aplicar` button, the armed port) are substituted by
  the violet accent or a **new functional token** added to `tokens.ts` under the
  same WCAG gate; teal remains reserved for identity. The copilot delivers the
  notice of this substitution to the design track.
- **FR-ID-3** — Node/edge/panel styling maps to existing tokens; any new token
  passes `tokens.wcag.test.ts` (AA text ≥4.5, UI/graphical ≥3.0) in both light
  and dark. Both themes are delivered day one (ADR-0030 §2).

### I. Security (brief item i)

- **FR-SEC-1** — Secrets remain **env-var name only**. The channel node's token
  field and the model node's `api_key_env` field carry the **name** of an env var
  (`TELEGRAM_TOKEN`, `DISCORD_BOT_TOKEN`), never a value; webhook renders `sin
  secreto` when it has no `token_env` (ADR-0010, ADR-0030 §7 microcopy). No secret
  value is ever entered, displayed, stored, or logged.
- **FR-SEC-2** — No new endpoint is exposed without the bearer. `getBrains` /
  `getChannels` stay read-only/open; `getConfig` / `postConfig` /
  `getReloadStatus` stay bearer-gated. The CSP is **unchanged** (FR-DEP-2).
- **FR-SEC-3** — Long values (env-var names, model ids) are truncated visually per
  the mockup, with the full value available on hover/`title`, never dropped from
  the config.

---

## Acceptance scenarios (Given / When / Then)

- **AS-PERSONA-1** Given a `korvun.json` with **no** `persona` on any brain, When
  it is loaded and validated, Then it validates and every brain's system prompt is
  byte-identical to today (backward compat; assert on the composed prompt).
- **AS-PERSONA-2** Given a brain with `persona.display_name`, `tone`, `language`,
  `instructions`, When the Orchestrator (non-agent) brain is wired, Then
  `WithSystemPrompt` receives the `composePersona` output (the previously-unwired
  path is now exercised).
- **AS-PERSONA-3** Given an **agent** brain with both `Agent.SystemPrompt` and a
  `persona`, When wired, Then `composePersona(p)` is prepended to the existing
  `buildSystemPrompt` (NC-4 order), the ADR-0021 protocol block is intact, and
  both contributions are present (no silent drop).
- **AS-PERSONA-4** Given a `persona` violating a validation bound, When validated,
  Then `Validate` returns `%w`-wrapped `ErrInvalidConfig` with field path
  `brains[i].persona.<field>` (targetable by the builder's inline `400` mapping).
- **AS-DEP-1** Given the canvas library bundled by Vite and mounted under the
  real CSP, When a two-node graph renders, Then the console shows **zero** CSP
  violations and **zero** external network requests (all assets same-origin).
- **AS-CANVAS-1** Given the palette, When a channel block is dragged onto the
  canvas, Then a channel node appears and the working copy gains the channel
  (dirty = true), with no reload triggered.
- **AS-CANVAS-2** Given a channel node and a brain node, When the operator draws
  channel→brain, Then a `route` is added and the edge renders; When they draw
  channel→model, Then the `kvReject` animation + the exact toast fire and **no**
  config change occurs.
- **AS-CANVAS-3** Given a **private** brain with a `local` and a `cloud` model
  connected, When rendered, Then the cloud model shows `nube · excluido (privado)`
  dimmed+dashed and the privacy microcopy; When the brain is switched to
  `public`, Then the cloud model renders normally. The exclusion set equals
  `brainSummary`'s survivor rule for the same input.
- **AS-SER-1** Given `korvun.example.json`, When `configToGraph` then
  `graphToConfig`, Then the result is `JSON.stringify`-identical to the input
  (round-trip preserves `storage`, `observability`, `admin`, `retry`, `agent`,
  `base_url`).
- **AS-HOT-1** Given a dirty canvas, When `Aplicar cambios` is pressed and the
  server reports `pending` → `cutover-in-progress` → `succeeded`, Then the bar
  shows `Aplicando — el gateway sigue en marcha…`, the canvas is locked during
  polling, and on success the working copy re-baselines (dirty clears).
- **AS-HOT-2** Given an in-flight reload, When the poll gets a network error
  (cutover restart), Then the machine keeps polling and never shows `failed`
  (inherited from `reload.ts`).
- **AS-HOT-3** Given a config that removes the admin token, When `Aplicar
  cambios` is pressed, Then the `409 config_would_self_lock` renders its distinct
  message (not confused with `reload_in_progress`).
- **AS-HOT-4** Given a `400` validation error referencing `brains[2].persona…`,
  When Save fails, Then the error surfaces inline on the corresponding node/panel.
- **AS-CTX-1** Given the desktop app with the core running, When the Builder tab
  opens, Then the canvas loads in the same-origin iframe and drag/connect/select
  all work inside the WebView (no CSP or `frame-ancestors` error).
- **AS-ID-1** Given the new tokens, When `tokens.wcag.test.ts` runs, Then every
  text pair ≥4.5:1 and every graphical token ≥3.0:1 in both themes.
- **AS-E2E-1** Given a running gateway and a pasted bearer, When the operator
  creates a brain, connects a model, and saves (the MASTER §14.2 flow), Then the
  config persists and a subsequent `getConfig` reflects it; axe reports no
  violations on the new views.

---

## Success criteria

- Coverage floors: ≥90% for `internal/config` + `internal/brain` (persona);
  ≥85% for new frontend modules; the new pure functions (`composePersona`,
  `configToGraph`, `graphToConfig`, the exclusion rule, new reducer branches)
  are fully unit-tested without a DOM.
- `make quality` green with `-race` over the **whole** suite (Go), and the
  frontend gates (`tsc --noEmit`, ESLint, Vitest, `tokens.wcag.test.ts`) green.
- **Playwright e2e** of the MASTER §14.2 flow ("crear cerebro → conectar modelo
  → guardar") passes in both the browser and (where harnessed) the iframe
  context; **axe** clean on every new view.
- **Side-by-side captures** against the cited mockups (`final-6`, `final-6b`, and
  the standalone prototype's exclusion + invalid states) attached at SP4.
- **`go.mod` (root and `web/builder`) intact** — proven by diff; the headless
  binary and pipelines untouched (no `web/builder`/`frontend`/`e2e` path changes
  outside this piece's scope trigger unrelated CI lanes).

---

## Sub-phase breakdown (brief item j — house pattern: red → green → quality per SP)

- **SP0 — Integration spike + ADR-0039 (the early gate; the SP1-of-Piece-5
  pattern).** Context7-verify React Flow (NC-1 gate: Context7 connected in the
  session, pending Chano's confirmation); bundle it under Vite with zero
  CDN; mount a trivial 2-node graph under the real CSP and confirm **zero CSP
  violations**, light+dark, axe-clean, and correct behavior **inside the desktop
  iframe**. Deliverable: ADR-0039 with the verified package/version/API and a
  go/no-go. **If the spike fails (CSP, React 19 incompat, iframe), STOP** — do not
  build on unproven ground.
- **SP1 — Personality per brain (Go, TDD).** FR-PERSONA-1..4. Pure Go: additive
  `BrainConfig` persona, `composePersona`, wiring for both brain kinds,
  validation, backward-compat. No frontend. Per NC-4 (resolved); the SP1 red fixes
  the exact length caps.
- **SP2 — Schema/state alignment + serialization (frontend, TDD).** Bring
  `schema.ts` enums current (telegram/discord/webhook + modes); add persona to
  the TS mirror; add reducer branches (FR-SER-3); implement `configToGraph` /
  `graphToConfig` + the pure exclusion rule with round-trip and
  selector-parity tests. No canvas rendering yet.
- **SP3 — Canvas view (frontend, component TDD + axe).** The React Flow canvas:
  palette drag, node kinds, connection validity + invalid-connection reject,
  privacy-exclusion rendering, selection → Propiedades panel (re-homed forms),
  reuse of the save-bar + reload machine + error views. Token reconciliation
  (FR-ID-*).
- **SP4 — Integration, e2e, quality.** Playwright master-flow e2e + axe on new
  views; both execution contexts; side-by-side captures; `make quality` green
  `-race`; `go.mod` diff clean; close docs (stage doc + ADR-0039 + master doc +
  `/graphify --update` for the doc changes).

---

## Decisions folded in

- **Canvas is the primary/only builder view; forms become the properties panel**
  (FR-SCOPE-1) — `final-6` shows one integrated surface, no tab toggle.
- **channel / brain / model are the only node kinds; agent + policy are brain
  properties** (FR-CANVAS-1) — the brain node's inline tags and the Propiedades
  panel in `final-6`.
- **Routes are channel→brain edges; `RouteForm` is removed as a panel**
  (FR-SCOPE-3, FR-CANVAS-3) — routes are exactly channel→brain in the domain.
- **The frontend derives the privacy exclusion by replicating ADR-0015 locally;
  `/api/brains` is not the source** (FR-CANVAS-4) — the API omits excluded
  models; the config the builder edits carries full localities.
- **Reuse the existing reload machine and reducer wholesale; extend additively**
  (FR-HOT-1, FR-SER-3) — they are already pure and tested; duplicating them would
  be the anti-pattern the brief warns against.
- **ADR-0030 governs the canvas palette; functional teal from the mockup is
  substituted with the violet accent** (FR-ID-1/2) — Chano's NC-7 decision
  (2026-08-01): violet functional, teal identity-only.
- **No dependency beyond the one canvas library** (FR-DEP-3) — NC-2 resolved: zero
  second frontend dependencies; any temptation earns its own NC/ADR.

---

## Clarifications resolved (2026-08-01)

All seven open points are resolved; the spec is approved. SP0 remains gated on
Context7 being connected in the session (NC-1 gate below).

- **NC-1 (React Flow) — RESOLVED BY PROCESS.** The package identity, version,
  API, and React-19 compatibility are verified **EXCLUSIVELY via Context7** as the
  first action of SP0 and recorded in ADR-0039. If the spike fails (CSP, incompat,
  iframe), it **STOPS** and the copilot decides on the spike's own data. Pre-gate:
  Context7 connected in the session (pending Chano's confirmation).
- **NC-2 (second dependency) — RESOLVED.** **Zero** second frontend dependencies
  in this piece: drag, state, and layout are done with the canvas library + React
  + the existing reducer. Valve: any temptation stops and earns its own NC/ADR.
- **NC-3 (unproduced states) — RESOLVED.** The states without a mockup **reuse the
  2b error components already designed** (`ReloadView` / `SaveErrorView` /
  confirmations) with copy adapted to the canvas. **No new error-state mockups**:
  the design system is the law (the AA-over-mock precedent).
- **NC-4 (persona) — RESOLVED.** Fields `display_name` / `tone` / `language` /
  `instructions`, **all optional free-text with length caps** (the exact caps are
  fixed by the SP1 red). `display_name` is **presentation only** — `name` stays
  the routing key, untouched. Composition: the persona is a **PREFIX** of the
  system prompt. Orchestrator: `WithSystemPrompt(composePersona(p))` (this newly
  exercises the wiring absent today). AgentBrain: `composePersona(p)` **prepended
  to the existing `buildSystemPrompt` WITHOUT altering its internal order** — the
  ADR-0021 tool protocol block stays intact, both contributions present, nothing
  overwritten.
- **NC-5 (iframe) — RESOLVED.** Drag / pointer-capture / focus **inside the
  WebView iframe** is an **SP0 acceptance criterion**.
- **NC-6 (multiplicity + layout) — RESOLVED.** The valid-connection matrix is an
  **exact mirror of `config.Validate`** — no more permissive, no more restrictive;
  the SP2/SP3 tests **derive it from the real validator, never invent it**. **No
  orphan model nodes** — a palette model is dropped **onto a brain**. Two brains
  with the same provider are two config entries, hence two nodes (schema-mirrored).
  First-load placement is a **deterministic column layout** (canals | brains |
  models, stable by config index); **positions are never persisted in v1** —
  neither in the core config nor in `localStorage`.
- **NC-7 (teal) — RESOLVED (Chano, 2026-08-01): violet functional.** ADR-0030 §1
  stays intact; the mockups' functional teal usages (the `Aplicar` button, the
  armed port) are substituted by the violet accent or a **new functional token**
  under the same WCAG gate; teal is reserved for identity. The copilot delivers
  the notice of this substitution to the design track.
