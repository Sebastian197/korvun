# Piece 5 SP6 — the real window (desktop chrome): Design Spec + cut plan

> **Status:** draft — awaiting the copilot's review. NO window implementation
> exists yet; this document and the cut plan are this assignment's whole
> deliverable (two-stage mandate).
> Governing ADRs: ADR-0035 §§3(a) (bindings as plain Go, thin Wails
> adapter), 3(b) (same-origin proxy — the SP4 contract in
> `2026-07-25-piece-5-sp4-asset-proxy-design.md` is law), 3(c) (chrome
> survives core shutdown; single builder embed), §4 (secrets/keychain,
> bearer never in the DOM), §5 (first run), §7 (signing) and
> Consequences / framing §8 (v1 exclusions);
> ADR-0029 (frontend toolchain precedent); ADR-0030 (identity + tokens +
> guards that bite). External-docs note: Wails v2 `Bind` /
> lifecycle-context / events surface verified via Context7 TODAY
> (2026-07-25, `/wailsapp/wails`): `options.App.Bind: []interface{}{...}`
> exposes public methods; `OnStartup: func(ctx context.Context)` supplies
> the runtime ctx; generated JS bindings return `Promise`s (Go error →
> rejection); events ride `runtime.EventsEmit(ctx, name, data…)` /
> `window.runtime.EventsOn`. The toolchain versions below are read from
> `web/builder/package.json` (the pinned, CI-green reality). Per SP1 Gate 2
> discipline, the EXACT signatures are re-verified with Context7 again at
> implementation time (cut 6a), never recalled from memory.

## Goal

The native window becomes Korvun's face: a chrome (Home, Channels wizard,
Activity, Settings, onboarding) built to the design-track system, served
from the Wails AssetServer, talking to the in-process core EXCLUSIVELY
through the SP4 same-origin proxy, with the EXISTING `web/builder` embedded
untouched behind `/builder/`. Out of scope: packaging + desktop CI (SP7),
hardware validation (SP8), any core/API change (the §data-honesty list),
and the builder's visual re-styling to the desktop system (documented
future work, not SP6).

## The visual reference pack (design-drafts/, gitignored, on disk)

**Component source:** `Korvun Desktop (standalone).html` — a self-contained
bundle; real HTML/CSS is extracted from it, not re-invented. Its `scenario`
prop drives the states: `'normal'` (default), `'parado'`, `'incidencia'`,
`'recién instalado'` (plus a "sin modelo" substring branch inside the
onboarding path).

**The mini design system is LAW** (`chica31b.png`, `Sistema de
Diseño-selection_31.png`): palette `#7A5AF5` (violet accent) / `#2BC8B7`
(teal, identity-gradient only), surface ramp, amber/red semantics,
Geist/Geist Mono (SIL OFL, EMBEDDED — zero CDN, the ADR-0029 §5 pattern),
the teal→violet gradient on exactly ONE primary action per view, `…`+tooltip
truncation only when content does not fit, and the secrets rule: only
env-var NAMES are ever painted, never a value.

**Screen → file index:**

| Screen / state | Reference files |
|---|---|
| Home, running (normal) | `01-dashboard.png`, `Korvun Desktop-selection_6.png`, detail cards `04/05/06/07-detalle-*.png` (`selection_2/3/4/5`) |
| Home, stopped | `final-1-dashboard-parado.png` |
| Home, incident (channel down) | `Korvun Desktop-incidencia.png` |
| Sidebar / navigation / logo+version | `02-sidebar-navegacion.png` (`selection_7`), `03-detalle-logo-version.png` (`selection`) |
| Channels + wizard steps 1–2 | `Korvun Desktop-selection_8..13.png` |
| Wizard step 3 (keychain, masked field) | `final-4-wizard-paso3.png`, `selection_14/15.png` |
| Builder (dirty / applying states) | `final-6-builder-estados.png`, `final-6b-builder-aplicando.png`, `selection_16..20.png` |
| Activity (+ its 6 decisions, detail) | `final-7-actividad-detalle.png`, `Korvun Desktop-selection_21..30.png` |
| Settings (3 themes) | `final-3-ajustes.png`, `Korvun Desktop-selection_32..35.png` |
| Onboarding (fresh install) | `Korvun Desktop-recien-instalado.png`, `Korvun Desktop-recien-instalado-nav.png` |
| Design system sheets | `chica31b.png`, `Sistema de Diseño-selection_31.png`, `chica-18/19/20/31.png` |

## Functional requirements

- **FR-WIN-1 Stack (copilot proposal VALIDATED):** the chrome is its own
  app at `cmd/korvun-desktop/frontend/` on the SAME toolchain as
  `web/builder` — React 19.2.x + TypeScript strict + Vite 8.1.x +
  Tailwind 4.3.x (`@tailwindcss/vite`) + Vitest 4.1.x + Playwright 1.61.x
  + RTL, npm, exact-patch pins cloned from `web/builder/package.json` and
  re-verified with Context7 at scaffold (ADR-0029 discipline; Tailwind v4
  remains the already-spent innovation token — no new ones). Fonts:
  Geist + Geist Mono via `@fontsource/geist{,-mono}` (SIL OFL,
  self-hosted woff2, license in repo) — the chrome follows the design
  LAW; the builder keeps its ADR-0030 faces until its future re-styling.
  Build output `frontend/dist/`, and the embed directive CHANGES to
  `//go:embed all:frontend/dist` (the current `all:frontend` would sweep
  sources and node_modules into the binary once the app scaffold lands)
  → **the dist/.gitkeep + committed stub pattern (ADR-0029 §4) applies
  inside `frontend/dist/`** so a bare `go build -tags desktop` never
  breaks on a clean clone; `make` gains a desktop-frontend target
  mirroring the builder's ordered build. The SP1 static page is REPLACED
  by the built chrome (its job is done).
- **FR-WIN-2 Bindings (ADR-0035 §3a) + THE DEADLINE LAW (SP4 mandate,
  rider b of this assignment):** a plain-Go `Desktop` bindings struct in
  `internal/shell` (NO Wails import — doc.go contract; `main.go` only
  passes it to `Bind` and hands it the `OnStartup` ctx). Surface:
  `Start()`, `Stop()`, `Status()`, `LoadConfig(path)`,
  `DefaultConfigPath()`, `EnsureDefaultConfig()` (created=true is the
  onboarding trigger, ADR-0035 §5), `SetSecret(name, value)`,
  `DeleteSecret(name)` (wizard step 3), and `CheckOllama(baseURL)` —
  onboarding step 1, a plain Go `GET {baseURL}/api/tags` with a 3 s
  timeout returning `{reachable bool, detail string}` (no new core API;
  the check lives in the shell). Composition note: `DefaultConfigPath` /
  `EnsureDefaultConfig` are package FUNCTIONS today (`firstrun.go`;
  `EnsureDefaultConfig` takes an explicit path) — the arg-less bindings
  compose them (resolve the default path, then call); `CheckOllama` and
  the `Desktop` struct itself are the new shell code of cut 6a.
  **LAW: every binding is bounded — never an unbounded wait from the UI
  into the Controller's mutex.** Mechanism (honest about the mutex):
  `sync.Mutex.Lock` is uncancellable, so a plain `context.WithTimeout`
  inside the Controller cannot bound acquisition; the bindings struct
  therefore runs each lifecycle call in a goroutine and `select`s on
  result-vs-deadline (Start/Stop 30 s; keychain Set/Delete 60 s — a
  Linux Secret Service unlock prompt is a legitimate slow path, its own
  class, not a "state read"; CheckOllama 3 s; state reads 5 s).
  Post-timeout semantics are spec'd, not accidental: a timed-out call's
  goroutine still completes against the Controller (an abandoned Start
  may finish starting the core AFTER the UI error — the status store's
  next poll reconciles the truth), its terminal result is logged, and
  the store serializes lifecycle calls (one in flight) so timed-out
  goroutines cannot pile on the mutex. Enforced where testable: in the
  plain-Go struct with `-race` tests; Wails glue stays a pass-through.
- **FR-WIN-3 Chrome-native vs proxied:** Home / Channels / Activity /
  Settings / onboarding are CHROME views talking to `/api/*`, `/healthz`
  via the SP4 proxy (fetch with relative URLs; the 503 two-body contract
  — `core stopped` / `core unreachable` — is what the stopped/incident
  chrome paints). The BUILDER is the existing `web/builder` embedded
  UNTOUCHED, reached at `/builder/` through the proxy, rendered inside
  the chrome in an `<iframe src="/builder/">` (same origin relative to
  the chrome, so the SP4 bearer injection covers it; the chrome's
  sidebar stays alive around it). **Review finding, needs the copilot's
  call (NC-1):** the builder's own CSP today is `frame-ancestors 'none'`
  (`web/builder/embed.go:26`), which blocks even same-origin framing and
  the proxy passes response headers through verbatim — the iframe cannot
  render without ONE of: (a) a one-line core amendment to
  `frame-ancestors 'self'` (recommended: still blocks all cross-origin
  framing; the only ancestor is the same origin the bearer gate already
  trusts), (b) the SP4 proxy rewriting that response header for
  `/builder/` (amends the SP4 contract), or (c) no iframe — a detached
  view losing the persistent sidebar. Its visual convergence with the
  desktop system is WRITTEN DOWN as future work, not done in SP6.
- **FR-WIN-4 Data honesty — "no-v1" list with graceful degradation** (the
  API is not touched in SP6; the chrome paints what exists):
  - *Per-channel message counters* (design: cards per channel) — the DATA
    exists (`korvun_messages_processed_total{channel}`,
    `korvun_channel_messages_dropped_total{channel}` at `/metrics`,
    proxied), but v1 does not parse Prometheus text exposition → v1
    paints counts derived from the SSE feed, labeled honestly **"desde
    que se abrió la ventana"** (the feed is session-scoped and lossy
    under backpressure — never presented as an all-time total), plus
    `/api/channels`' dropped counters. Metrics-sourced totals: no-v1.
  - *The "Reconexiones" card* (`06-detalle-card-reconexiones.png`) — the
    count exists only as `korvun_channel_reconnects_total` at `/metrics`
    (no SSE type, not in `ChannelSummary`) → no-v1, same reason.
  - *Activity's rich rows* — the mock shows sender, message text, reply,
    decision labels, latency and per-model outcomes; the SSE frame
    carries ONLY `type, channel, brain, timestamp, envelope_id,
    direction`, and message CONTENT is excluded **by construction and
    forever** (ADR-0024 secret-free frames — not a v1 gap, an
    invariant). Activity v1 = the honest metadata feed with type/channel
    filters; the side-by-side screenshot review for Activity therefore
    compares LAYOUT AND IDIOM, not data richness (flagged for the
    copilot/design track explicitly).
  - *"Vaciar memoria"* (clear conversation store) — no API → the row is
    NOT rendered in v1 (a dead button is a lie).
  - *Webhook as a config channel type* — not a core type (SP5 NC-1
    evidence) → the wizard offers telegram/discord only.
  - *Per-channel health for the incident state* — no health API → the
    incident banner triggers on what the shell truly knows: the core
    exited on its own (reap flip of `Status().Running`; the EXIT REASON
    is log-only today, so the banner says "the core stopped
    unexpectedly" without inventing a cause) or `HandleFailed` /
    `MessageDropped` events in the SSE feed (their `channel` field is
    real and is shown); never a per-channel status table.
  - *The wizard's token pre-check state and the memory ON/OFF toggle*
    (`wizPre`, `tgMem` in the mock) — no API behind either → not
    rendered in v1.
- **FR-WIN-5 Settings v1:** theme (dark/light/system) persisted in the
  WebView's `localStorage` (key `korvun.chrome.theme` — chrome-local
  state, not config); real info rows: config path (`Status().ConfigPath`),
  effective admin address (`Status().AdminAddr`) with a Copy button,
  admin token row showing "automatic, per-cycle" and the env-var NAME
  only; "Start the core when the app opens" toggle (localStorage,
  `korvun.chrome.autostart` — this is auto-START-THE-CORE on app launch,
  cheap; an OS login item is a v1 EXCLUSION per ADR-0035 Consequences /
  framing §8 and stays out).
  Everything else the mock shows without an API behind it → FR-WIN-4.
- **FR-WIN-6 Home's three states + the channel wizard:** Home renders
  `marcha` (status cards + live feed), `parado` (the stopped hero with
  the ONE gradient primary action = Start), `incidencia` (amber/red
  banner per FR-WIN-4's honest triggers) — states driven by the Status
  polling store (2 s interval; no Wails events in v1 — polling is the
  simplest thing that works, `EventsEmit` documented as the upgrade path
  if polling ever feels laggy). The wizard, mirroring `config.Validate`
  AS IT IS: step 1 pick type (telegram/discord; a type already
  configured is DISABLED with an honest note — the core registers one
  channel per type) → step 2 the token env-var NAME (`token_env`,
  suggested default per type; `ChannelConfig` has no name field, and
  the mode is fully type-determined — displayed, never chosen) → step 3
  the masked token field writing to the OS keychain via `SetSecret`
  (value never rendered back, never logged, never in the DOM after
  submit) + `POST /api/config` through the proxy + reload-handle polling
  — EXACTLY the pipe the SP5 e2e (`TestFirstRun_builderAddsFirstChannel`)
  already proved; the UI is its first human consumer. Onboarding
  (`EnsureDefaultConfig` created=true): step 1 model check (`CheckOllama`
  — reachability with retry; ADR-0035 §5's "pick provider" half is
  deliberately deferred to builder editing, the template being
  ollama-based) → step 2 first channel (the same wizard) → step 3 Start.
- **FR-WIN-7 Finish ("acabado" — the copilot's SPECTACULAR standard):**
  every view specs its EMPTY state (fresh gateway: no channels, no
  events yet), LOADING state (skeletons, never spinners-on-white),
  `:focus-visible` rings (violet, token-driven), hover states, motion
  (ADR-0030 tokens: enter `cubic-bezier(0.16,1,0.3,1)` / exit ease-in,
  micro 120 ms / base 180 ms / panel 220 ms, `prefers-reduced-motion`
  respected), the truncation law carried concretely (`…`+tooltip only
  when content does not fit — e.g. the long config path in Settings'
  info row, which is also its screenshot-review case), and the embedded
  typography (Geist ramp per the system sheet).
  Verification plan: axe-core inside Playwright (labels +
  focus-visible), the WCAG AA token-contrast Vitest gate (ADR-0030 §2
  pattern, cloned), reduced-motion exercised in one Playwright project,
  and the per-cut screenshot review (below).
- **FR-WIN-8 Fidelity tests (cheap, bite):** a Vitest unit asserts the
  design-token CSS variables exist and match the system sheet's values
  (`--accent: #7A5AF5` etc.); a source-scan test fails on any hardcoded
  hex color outside the token palette in the chrome's `src/` (colors
  live in tokens, nowhere else); and the ONE-gradient-per-view law gets
  its own scan — the gradient utility/token may appear at most once per
  view's source, and "gradient count per screen" is an explicit line in
  the per-cut screenshot review checklist. CSP + same-origin
  (ADR-0029 §5): the chrome's origin cannot get its CSP from the
  builder's Go handler (Wails serves the assets), so the header is
  emitted via **AssetServer middleware** (`assetserver.Options`
  middleware surface, re-verified with Context7 at 6a; `<meta
  http-equiv>` as documented fallback if the middleware cannot cover a
  path), and the Playwright same-origin network assertion runs against
  the chrome regardless — the browser-enforced gate stays the real one.

## Acceptance scenarios (Given / When / Then — the global set; each cut
carries its concrete subset)

- **AS-1** Given a stopped core, When the app opens, Then Home shows the
  `parado` design (single gradient Start action), `/api/*` renders the
  degraded state from the exact 503 `{"error":"core stopped"}` body, and
  the chrome itself is fully alive (ADR-0035 §3c).
- **AS-2** Given Start is clicked, When the core confirms boot, Then the
  status store flips to running within one poll interval, Home paints
  `marcha` with real data (`/healthz`, `/api/brains`, `/api/channels`,
  SSE feed), and no request carries a client-side bearer (proxy-injected
  only — assert no Authorization header leaves the page).
- **AS-3** Given a fresh install (no config), When the app opens, Then
  `EnsureDefaultConfig` reports created=true and the onboarding runs its
  3 steps; on a machine with no Ollama, step 1 paints the honest
  unreachable state ("sin modelo" branch) with a retry, never a fake
  success.
- **AS-4** Given the wizard completes for a discord channel, When step 3
  submits, Then the token lands in the keychain seam (fake store in
  tests), the POSTed config passes the reload to `succeeded` through the
  proxy, `/api/channels` shows the channel, and the token value appears
  in NO DOM node, request URL, or localStorage entry after submit.
- **AS-5** Given the Builder view, When it loads, Then the iframe serves
  the EXISTING `web/builder` bundle through `/builder/` (its own dirty /
  applying states per its own design), and the chrome sidebar stays
  operational around it.
- **AS-6** Given the core dies on its own (reap) or a `HandleFailed`
  arrives on the feed, When Home is visible, Then the `incidencia`
  treatment appears with the honest trigger named, and recovers (banner
  clears) after a clean Start.
- **AS-7 (LAW)** Given a Controller wedged in a long Stop, When any
  binding is called from the UI, Then it returns a named timeout error
  within its deadline — never an unbounded hang (plain-Go `-race` test
  with a deliberately-blocked Controller).
- **AS-8** Given every cut's Playwright run, Then screenshots
  `sp6X-<pantalla>-<estado>.png` (viewport 1440×900, states provoked for
  real against the no-network core) land in `design-drafts/` for the
  copilot's side-by-side review — no screenshots, no review, no
  approval.

## The cut plan (SP6a / SP6b / SP6c — each cut ends with tests green,
`make quality`, screenshots delivered, and the copilot's review gate)

- **SP6a — toolchain + tokens + shell + bindings + state.**
  Scaffold `cmd/korvun-desktop/frontend` (pins cloned + Context7
  re-verified), design tokens + Geist embedding + AA-contrast gate +
  no-hardcoded-hex test, the app shell (sidebar, nav, theme swap,
  logo/version), the `Desktop` bindings struct with THE LAW + `-race`
  tests, the Status polling store, the Playwright harness (built chrome
  dist + `ProxyHandler` + real no-network core on one loopback origin —
  the SP5 pattern; mirrors AssetServer assets-first/handler-on-miss
  semantics), and the screenshot pipeline. 6a INCLUDES a minimal
  parado-Home placeholder (hero + Start action wired) so its review gate
  is satisfiable from its own deliverables. Screenshots:
  `sp6a-shell-navegacion.png`, `sp6a-inicio-parado-minimo.png`,
  `sp6a-tema-claro.png`. Portable 3-OS from day one (the 3a2bd22 lesson:
  GOOS-scoped permission/env assertions; Playwright runs where CI runs
  it — the frontend job is ubuntu, per ADR-0029 §6).
- **SP6b — Home's three states + Settings v1 + Activity feed.**
  The full Home (marcha/parado/incidencia with honest triggers),
  Settings (theme persistence, info rows + Copy, autostart toggle),
  Activity as the SSE live feed with filters + empty state. Screenshots:
  `sp6b-inicio-marcha/parado/incidencia.png`, `sp6b-ajustes-*.png`,
  `sp6b-actividad-*.png`.
- **SP6c — wizard + onboarding + Builder embed + full e2e.**
  The 3-step channel wizard (keychain + proxy mutation pipe), the
  fresh-install onboarding over `EnsureDefaultConfig`, the `/builder/`
  iframe view, and the end-to-end Playwright suite (AS-1..AS-6 complete,
  axe-core, same-origin gate, reduced-motion project). Screenshots:
  `sp6c-asistente-paso1/2/3.png`, `sp6c-onboarding-*.png`,
  `sp6c-builder-embebido.png`.
- Each cut is a reviewable unit: vitest + Playwright against the proxy
  with the real no-network core, `make quality` green with `-race` over
  the whole suite, Conventional commits, file inventory in the report,
  NO advancing to the next cut before the copilot's screenshot review.

## Success criteria

- New Go code (`internal/shell` bindings) ≥ 85% coverage, `-race`, no
  Wails import in `internal/shell`; the chrome's Vitest suite green and
  its Playwright e2e green on the frontend CI job; `make quality` green
  over the whole suite at every cut; headless `cmd/korvun` and its
  pipeline byte-untouched; `go.mod` unchanged (the chrome adds zero Go
  deps).
- The screenshot set of each cut delivered in `design-drafts/` and
  approved side-by-side by the copilot.

## Decisions folded in

- **The builder rides in an iframe** (not navigation that unloads the
  chrome): ADR-0035 §3c's "the chrome survives" extends to survives-
  while-the-builder-is-open; same origin keeps SP4's injection covering
  it and CSP intact.
- **Polling over Wails events for v1 status** — one fewer moving part;
  the events surface is documented (verified above) as the upgrade path.
- **`CheckOllama` lives in the shell as a plain HTTP GET** — onboarding
  needs reachability, not a new core API; 3 s deadline, LAW-compliant.
- **Playwright/Chromium screenshots as the review artifact** — the REAL
  chrome + REAL core + REAL proxy pipeline in a real browser engine;
  what Playwright cannot drive is the WKWebView itself, which is exactly
  SP8's hardware validation. If the copilot wants native-window captures
  earlier, that is a manual step to schedule, not an automatable gate.
- **"Iniciar con la aplicación" = auto-start the CORE on app open** —
  the only reading compatible with ADR-0035 §8 (login-start/tray are v1
  exclusions).
- **The SP1 static page retires in 6a** — replaced by the chrome build;
  the dist-stub pattern keeps clean-clone builds green.

## `[NEEDS CLARIFICATION]`

- **NC-1 — the builder iframe vs its own CSP (spec-review finding,
  blocks FR-WIN-3/AS-5 and cut 6c's builder view).** Evidence:
  `web/builder/embed.go:26` sets `frame-ancestors 'none'`, which blocks
  even same-origin framing, and the SP4 proxy forwards response headers
  verbatim. The options, each touching something this assignment said
  not to touch: **(a)** one-line core amendment to `frame-ancestors
  'self'` (RECOMMENDED — still blocks every cross-origin ancestor; the
  only 'self' ancestor is the origin the bearer gate already trusts;
  needs a tiny builderui commit + test), **(b)** the SP4 proxy rewrites
  that one response header for `/builder/` (amends the SP4 contract),
  **(c)** no iframe — a detached builder view losing the persistent
  sidebar (a design regression). Your call before 6c; 6a/6b do not
  depend on it.

Everything else resolves inside ADR-0035/0029/0030 + the design-track
law + the addendum. Two folded decisions are additionally flagged for
explicit veto in this review (they shape the work): **Playwright/
Chromium as the per-cut screenshot medium** (native WKWebView captures
deferred to SP8), and the **Activity v1 expectation reset** (FR-WIN-4:
metadata-only feed — ADR-0024 bars message content from the frames by
construction, so the side-by-side against `final-7`/`selection_21..30`
compares layout and idiom, not data richness; if the design track wants
richer Activity, that is a future core/API conversation, not SP6).
TDD/implementation does NOT start until this spec and the cut plan pass
the copilot's review — that stop is the assignment's own mandate, not an
NC.
