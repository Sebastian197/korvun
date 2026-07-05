# ADR-0030: Stage 14 Phase 2b — builder visual identity + UI architecture

> **Status:** accepted
> **Date:** 2026-07-05
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

ADR-0029 fixes the frontend *toolchain*. This ADR fixes what the builder **is and
looks like**: its own visual identity, the UI architecture over the Phase 2a
control API, and the interaction/state decisions that (a design review proved)
will haunt the implementer if left ambiguous.

The visual direction was chosen the project's way: `/design-shotgun` generated
three original directions (A "Signal Teal", B "Orchestration Violet", C "Corvid
Iridescent"), and `/plan-design-review` (with an independent cross-model design
voice) critiqued them against the real CSS. Two findings were load-bearing and are
baked into this ADR: **(1)** an identity accent must never collide with a fixed
status color (the teal accent sat next to the semantic `sent` green), and **(2)**
the previews only covered the happy path — the states that make a config editor
hard were unspecified.

The requirement is an **original** identity (learn principles from the best
developer tools; copy no brand, color, logo, or asset) coherent with being a
serious open-core product, and coherent with the existing read-only `/ui`.

## Decision

### 1. Visual identity: a synthesis, with a violet functional accent

Not any single shotgun variant, but a **synthesis**:

- **Concept + identity from C** — the *corvid / iridescent* idea (Korvun ≈
  *corvus*, raven) plus a **⌘K command palette**, which ties the name's meaning to
  the terminal heritage instead of decorating with it.
- **Information architecture from A** — a **left rail + breadcrumb + a persistent
  live panel**, so the operator always knows where they are and what the system is
  doing (Krug's scan/trunk test). C's centered single column hid navigation and
  the live pipeline; A's structure wins.
- **Functional accent = violet**, anchored **outside** the fixed semantic palette.
  This is the non-negotiable: the brand color **never** collides with a status
  color. The fixed event palette (shared with `/ui`, ADR-0024) is
  `received #3B82F6` · `sent #22C55E` · `dropped #F59E0B` · `failed #EF4444`.
  Violet is the only accent that does not overlap it. **Teal is rejected as the
  functional accent** precisely because `#2DD4BF` reads as the `sent` green at dot
  size (Rams: one accent, and it means one thing — trust erodes when brand looks
  like status).
- **The iridescent teal→violet gradient is reserved for identity moments only** —
  the glyph/logo, a single hairline, the hero. **Never** on pills, functional
  states, or focus rings: the review proved gradient focus rings and gradient-
  clipped text are low-contrast and do not survive light mode.

### 2. Design tokens + light mode from day one, with an AA contrast floor

- All design decisions are **tokens** (CSS variables in Tailwind v4's `@theme`,
  ADR-0029 §2): `--base` / `--surface` / `--border`, a text ramp
  `--text-1/2/3`, `--accent` (violet), and the fixed `--status-received/sent/
  dropped/failed`. Plus spacing (base 4px: 2xs…3xl), radii (sm/md/lg), and motion
  (easing enter `cubic-bezier(0.16,1,0.3,1)` / exit ease-in; micro 120ms / base
  180ms / panel 220ms; respect `prefers-reduced-motion`).
- **Light mode is defined at the token level on day one**, not retrofitted. The
  gradient (C) and glow (B) effects do **not** port to light and are resolved in
  tokens (identity-only usage keeps that contained). The token swap feeds the
  View Transitions theme change (ADR-0029 §3).
- **A WCAG AA contrast floor is enforced before any component is built, by a test
  that bites** (Phase 2a Principle 1, not a checklist): a **Vitest unit computes
  the contrast ratio of every text/background token pair and asserts ≥ 4.5:1** for
  normal text (ADR-0029 §6). A sub-AA token pair fails CI. This is a gate on the
  design system in 2b.1, not a polish-phase cleanup.

### 3. Typography: OFL, self-hosted, display + neutral UI-sans + mono

- Three roles, all **SIL OFL** and **self-hosted as woff2** (ADR-0029 §5), license
  files in the repo:
  - **display** (expressive, titles) — from the pool Space Grotesk / Bricolage
    Grotesque / Archivo,
  - **UI-sans** (neutral, body/labels) — a separate, more neutral OFL sans,
    **recommended distinct from the display** because dense forms read better with
    a calm body face than an expressive one,
  - **mono** (data/IDs/labels-that-are-values) — JetBrains Mono / IBM Plex Mono.
- The exact faces are locked in **2b.1** against real previews. Satoshi is
  excluded (Fontshare, not OFL — fails the self-host/redistribute requirement).

### 4. UI architecture over the Phase 2a control API

- Mount at **`/builder`** on the admin server, **next to the vanilla read-only
  `/ui`** (ADR-0024), which is **not** rewritten. Its own `go:embed` bundle. The
  Go handler for `/builder` emits **`Content-Security-Policy: default-src 'self'`**
  (ADR-0029 §5) — the no-CDN gate and the XSS mitigation for §6's bearer.
- **Mounting is gated on the same admin token as the mutation surface.** A builder
  whose Save would 404 is a trap: `POST /api/config` is only mounted when a token
  is configured (`RegisterMutation`, ADR-0028). So `/builder` is mounted **only
  when the token is set**; with no token, the operator gets the read-only `/ui`,
  not a builder that cannot save.
- **A new gated read endpoint — `GET /api/config` — is required for round-trip
  editing.** `POST /api/config` takes a **full** `config.Config`; the existing
  read API (`GET /api/brains` / `/api/channels`, ADR-0022) returns **secret-free
  summaries** that deliberately drop the fields needed to reconstruct a valid
  config (`token_env`, `api_key_env`, `base_url`, `locality`, routes, storage,
  admin). Without the raw current config as an editing baseline, the UI cannot
  edit-and-save. So Phase 2b adds **`GET /api/config`, gated by the SAME bearer as
  the write**, returning the raw current config document.
  - **Security reasoning (documented, not incidental):** this is a **conscious,
    scoped reversal** of ADR-0022 §4 ("not even an env-var name") for a **NEW
    gated route**, never a loosening of the open read-only surface. It exposes the
    **names** of env-vars (`token_env`, `api_key_env`) — which the authenticated
    operator already controls in their own config file — and **never their
    values**: `os.Getenv` is not called on this path, so no secret value ever
    leaves the process. The env-only-secret discipline (ADR-0010) holds intact.
- Consumes the control API: `GET /api/config` (gated, the editing baseline),
  `GET /api/brains` + `/api/channels` (the live resolved wiring for display),
  `POST /api/config` (save, gated), `GET /api/reload/{handle}` (poll status).
- **CSRF stays defended by construction** (ADR-0028 §3): the token travels only in
  `Authorization: Bearer`, never a cookie, and `/builder` is **same-origin** with
  the API via `go:embed` (no CORS). This ADR records that invariant explicitly so a
  future dev-proxy or a relaxed CORS policy does not silently void it (also to be
  noted in ADR-0028 §3).
- **Forms only. No canvas / node-graph / React Flow** — the visual canvas stays
  deferred; 2b is polished form editing.

### 5. The reload state machine (the defining interaction)

The face of Phase 2a's reload-and-rebuild. The ADR fixes:

- **Server-reported states are the EXACT `supervisor.State` strings (ADR-0027
  §F4), referenced verbatim, never a parallel enum:** `pending` →
  `cutover-in-progress` → `succeeded | rolled-back | failed`. These are the only
  strings `GET /api/reload/{handle}` returns (`{"state": …}`); the UI must not
  invent `preflighting`/`cutover` names, or the first real reload renders "unknown
  state". Preflight failure surfaces as `failed` — there is no distinct
  `preflighting` server state.
- **Client-synthesized states (pre-submit, never from the server):** `idle`,
  `dirty`. The full UI machine is `idle → dirty →` (POST returns a handle) `→`
  server states above.
- **Where each renders:** the inline chip for a steady state; a banner while
  in-flight (`pending` / `cutover-in-progress`); a modal/inline surface for
  `failed` / `rolled-back` (reason + retry).
- **The form locks from POST until a terminal state** (`succeeded` / `rolled-back`
  / `failed`) — no concurrent edit while the system swaps.
- **Status transport:** **poll `GET /api/reload/{handle}`** by default. The poll
  **must survive the admin server restarting under it:** the cutover shuts down
  the old app (its admin server included) and starts a new one, so `/builder` and
  `/api/*` are briefly **connection-refused** mid-reload. The handle lives in the
  **supervisor** (which outlives the app) and survives the blip (ADR-0027 §F4), so
  the poll retries through the refused window with backoff until the new admin
  server answers `succeeded`. Reusing the `/ui` SSE stream is a later enhancement,
  not day-one.

### 6. Bearer token UX + TLS gating

- A **paste screen**: a token field with **masking**, and clear copy that the
  token is the admin credential (full config-takeover).
- **Default storage is IN-MEMORY (React state), not `sessionStorage`.** `/builder`
  is a new same-origin attack surface (forms + the ⌘K palette) sharing the origin
  with the admin API; anything persisted is readable by any script an XSS runs, and
  this token is the crown jewel. So the default is in-memory: the token lives for
  the tab's lifetime and is **re-pasted per load**. `sessionStorage` is an
  **explicit opt-in** behind a **"remember for this session"** toggle, paired with
  the strict CSP (§4 / ADR-0029 §5) that shrinks the XSS surface. **Never
  `localStorage`, never a cookie.** (This corrects the earlier sessionStorage-by-
  default framing.)
- **401 → re-auth** flow (token cleared, paste screen returns).
- **Cleartext-over-non-loopback is an ADVISORY banner, not a client "block."**
  Honesty first: the browser cannot reliably know the server's real bind address
  (`window.location` can read `localhost` while the process is bound `0.0.0.0`
  behind a tunnel), and any JS check is trivially bypassed (curl). ADR-0028 §2 F10
  deliberately delegates cleartext to the operator, so a JS conditional is **not a
  security control** and must not be sold as one — that would be "guarantee by
  remembering." The UI therefore shows a clear **warning** that a bearer over
  non-loopback HTTP needs a TLS terminator, and does not pretend to enforce it.
  - **If a REAL gate is wanted, it belongs server-side, in ADR-0028** (proposed
    follow-up, not built here): `POST /api/config` refuses when bound non-loopback
    without TLS / `X-Forwarded-Proto: https`. Evaluate and decide there; the UI
    banner stands regardless.
- The shell-shaping pieces (storage default, paste flow, 401) are defined **before
  2b.1**.

### 7. Model-row editing contract (substance of 2b.2)

- **add / edit / remove / reorder** affordances on model rows (today's mockups
  are read-only display).
- **Valid provider list — RESOLVED (was `[NEEDS CLARIFICATION]`): a static known
  set from the config schema, not a runtime endpoint.** The control API exposes no
  provider catalog, and the authoritative set is the `config.Validate` enum
  (`ollama | groq` today). The UI carries it as a build-time constant kept in sync
  with the schema; a drift is caught because `POST /api/config` re-validates
  server-side and returns a 400. Adding a provider stays a Go change (new adapter +
  the `Validate` enum), and the constant follows it — the UI never invents
  providers.
- **`model_id`:** client-side validation is **non-empty only** (matching
  `config.Validate`); real existence is proven by **Preflight / boot**, not by the
  form (a bad id fails the reload's Preflight with a named error, surfaced via §5's
  `failed` state).
- **`locality` is operator-DECLARED, not derived** (segmented `local | cloud`, per
  ADR-0015 §3 — locality is a declaration the privacy selector routes on, never
  inferred from the provider).
- **`api_key_env`** is collected on the row when the provider is cloud (`groq`
  requires it, per `config.Validate`); the UI captures the env-var **name**, never
  a value.
- **Long lists** scroll/collapse (not fixed rows) so a many-model brain does not
  overflow the panel.

### 8. Accessibility floor — part of the design system, NOT deferred to polish

Baked into 2b.1's tokens and components:

- **Form labels (`.lbl`) leave the `faint` tier and leave mono:** minimum AA
  contrast + a legible face. Today `faint` (~`#5E6675`) at 11px uppercase mono
  carries the labels — a Krug scan failure and an AA failure at once.
- **`:focus-visible` on every interactive element** (buttons, segmented controls,
  `<select>`, nav/tabs, the ⌘K bar), not only inputs. A config editor is a
  keyboard surface.
- **Segmented controls with a consistent keyboard model** — native radios, or
  custom buttons with roving-tabindex/arrow-keys; never custom buttons next to
  native selects with an inconsistent keyboard model.
- **Color is never the only channel** in the live feed — an icon/label accompanies
  the status color (colorblind users).
- **Touch/pointer targets ≥ ~32–40px.**
- **Enforced by tests that bite, not a checklist** (Phase 2a Principle 1):
  **axe-core runs inside the Playwright e2e** to assert labels and `:focus-visible`
  on interactive elements, and the Vitest contrast test of §2 guards the AA floor
  (both wired in ADR-0029 §6). A missing focus style or a sub-AA pair fails CI.

## Consequences

- The builder has an **own, defensible identity** (concept-driven, not generic AI
  SaaS, not a clone) that sits coherently beside `/ui`.
- The five hard interactions (accent architecture, light-mode tokens, reload state
  machine, token/TLS UX, model-row contract) and the AA floor are **decided before
  code**, so the implementer is not left to invent them.
- Cost: light mode + AA + full state coverage from day one is more up-front design
  than a happy-path MVP — accepted, because these are the states that make or break
  a config editor, not polish.

## Alternatives considered

- **Ship a single variant as-is** — rejected: A is a generic dev-tool dashboard, B
  is the generic-AI-SaaS look (violet wash + glow + backdrop-blur) and breaks `/ui`
  continuity, C's gradient-as-accent has contrast/light-mode hazards. The synthesis
  takes the best of each.
- **Teal functional accent** — rejected (semantic collision with `sent`, §1).
- **Iridescent gradient on functional states/focus** — rejected (contrast + no
  light-mode port, §1).
- **Canvas/node editor now** — deferred; 2b is forms.
- **Defer accessibility to the polish phase (2b.3)** — rejected: the AA floor and
  focus model are design-system foundations, not polish.
