# ADR-0029: Stage 14 Phase 2b — builder frontend toolchain

> **Status:** accepted
> **Date:** 2026-07-05
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Phase 2b builds the **visual builder UI**: the operator edits the running config
through polished forms and triggers the reload-and-rebuild that ADR-0027 shipped,
gated by the bearer auth of ADR-0028. This is the first genuine **application UI**
in Korvun. Everything before it was either Go on the hot path or the hand-written
**vanilla read-only `/ui`** of ADR-0024 (a single `go:embed` `index.html`, no
framework, no build step).

That vanilla approach was correct for a read-only tail of events. It does not
scale to a builder: forms with dirty state, client-side validation, an async
reload state machine, keyboard-driven editing, light/dark theming. Hand-rolling
that in vanilla JS would be more code and worse quality than a framework. So Phase
2b **consciously breaks the project's frontend austerity** — but the break is
scoped to the frontend only. The Go core keeps its discipline: **`go.mod` stays at
3 direct dependencies**, because the frontend is a **build-time** toolchain whose
output is static assets, not a Go runtime dependency.

### External-docs note (Context7-verified 2026-07-05)

- Vite current stable line is **v8** (`/vitejs/vite`).
- Tailwind **v4** installs as a Vite plugin: `tailwindcss` + `@tailwindcss/vite`,
  configured in `vite.config.ts` with `plugins: [tailwindcss()]`, and imported
  CSS-first with `@import "tailwindcss";`. It compiles to **local static CSS**
  with zero runtime and zero CDN (`/websites/tailwindcss_installation_using-vite`).
- Vitest current stable line is **v4** (`/vitest-dev/vitest`).

Exact patch versions are pinned and **re-verified via Context7 at scaffold time**
(step 2b.1, the first code that touches these libraries) — this ADR fixes the
*decision*, not the lockfile.

**Innovation-token discipline (boring-by-default).** Versions are pinned to an
**exact patch, never a `^` range** — currency is not stability, and a floating
range lets a churny minor land mid-phase. Of the four majors, only **Tailwind v4**
is genuinely new (a ground-up CSS-first rewrite); React 19 / Vite v8 / Vitest v4
are mature. Tailwind v4 is the **one innovation token** we spend here. If v4 (or
Vite v8) churns disruptively during 2b, the pre-agreed fallback is the mature line
(**Tailwind v3** with the PostCSS plugin, and/or the prior Vite major) — a
reversible, build-time-only change.

## Decision

### 1. Stack: React + TypeScript + Vite

- **React 19** + **TypeScript** (`strict`, no implicit `any`, explicit prop
  types) + **Vite v8**, matching the frontend standards already written in
  CLAUDE.md ("React + TypeScript + Vite + Tailwind"). Package manager: **npm**
  (reconciled to reality: corepack could not activate pnpm in the build environment,
  and the package manager is a convenience, not an architectural, choice — npm works
  and is committed with its lockfile).
- The frontend lives in its own subproject at **`web/builder/`**, isolated from
  the Go module. It has its own `package.json`, `package-lock.json`, `tsconfig.json`,
  and `vite.config.ts`.

### 2. Styling: Tailwind v4, CSS-first, tokens as CSS variables

- **Tailwind v4** via the `@tailwindcss/vite` plugin, CSS-first (`@import
  "tailwindcss";`). No `tailwind.config.js` runtime, no PostCSS chain to maintain,
  no Tailwind CDN.
- The design tokens (ADR-0030) are declared as **CSS custom properties** in an
  `@theme` block — one place for color/spacing/type/radius/motion — so light/dark
  is a variable swap, not a rebuild. Tailwind compiles them to static utility CSS.
- Rejected: **CSS Modules** — viable and more austere, but Tailwind v4 gives
  faster polish iteration and keeps tokens in one enforced place, which the design
  system (ADR-0030) needs. This is the deliberate austerity break, scoped to CSS.

### 3. Animation: CSS + View Transitions first, Motion deferred

- Start with **CSS transitions + the View Transitions API** (theme swap, panel
  enter/exit, reload-status changes). No animation library on day one.
- **Motion (`motion/react`) is deferred** until a concrete interaction genuinely
  needs orchestration that CSS cannot express — the same "no seam without a
  consumer" discipline that governed the event bus (ADR-0023). Adding it later is
  a justified, reversible change; adding it now is speculative bundle weight.

### 4. Single binary preserved: `go:embed` of the built `dist/`

- `vite build` emits `web/builder/dist/`. The Go binary embeds it via
  **`go:embed`** and serves it at **`/builder`** on the existing admin server,
  next to the vanilla read-only `/ui` (ADR-0024), **which is NOT rewritten**.
- The single-binary promise holds: the builder ships *inside* the binary, no
  external asset host, no runtime Node. `go.mod` stays at **3 direct deps**
  (`go-telegram/bot`, `modernc.org/sqlite`, `prometheus/client_golang`) — the
  frontend adds **zero** Go dependencies.

**Build-ordering is load-bearing — `//go:embed dist` is a COMPILE-TIME
requirement.** If `web/builder/dist/` is absent or empty, `go build` fails hard
(`pattern dist: no matching files found`) — not a graceful check, a compiler
error. Left unhandled that breaks a fresh-clone `go build`, the `quality.yml`
matrix (×3 OS), the cross-compile ×6 (`CGO_ENABLED=0`), `golangci-lint`,
`govulncheck`, AND the GoReleaser release (ADR-0025/0026). Two mechanisms make it
safe:

- **A committed placeholder** so the embed pattern always matches on a clean
  clone: `web/builder/dist/.gitkeep` plus a minimal `web/builder/dist/index.html`
  stub (a "run `make build` to build the builder" page). This is the ONLY built
  artifact tracked in git; the real `dist/` is gitignored.
- **The frontend build is an ordered dependency** of every Go build path: the
  `Makefile` `build` (and `quality`) targets, and each Go CI job, run
  `cd web/builder && npm ci && npm run build`
  **before** `go build`. In CI the real `dist/` is **always rebuilt fresh** and a
  committed `dist/` is never trusted (this dissolves the "staleness" question —
  see §6). The stub exists only so a bare local `go build` / clean clone compiles.
- **Local-dev story (explicit):** `make build` builds `dist/` first, so the repo
  always compiles. A developer who runs `go build ./...` directly without the
  frontend build gets the stub page at `/builder` (compiles, serves a "run
  `make build`" placeholder), never a compile failure.

### 5. Zero CDN (non-negotiable, cross-ref ADR-0030 §identity)

Coherent with self-hosted + the privacy promise + the single binary:

- **Fonts self-hosted** as `.woff2`, embedded and served by Korvun, never linked
  to Google Fonts / Fontshare / any CDN. Every chosen face is **SIL OFL**
  (self-host + redistribute OK); its license file ships in the repo.
- **No external resources in the bundle:** no `<script src="https://…">`, no
  `<link href="https://…">` to styles or fonts, no `@import` of a CDN in CSS.
  Tailwind compiles locally; React/Vite compile to `dist/`.
- **npm is build-time only:** the final binary serves pre-compiled statics,
  depending on no registry at runtime.

**Enforced at the NETWORK layer, not by text-matching (guarantee by
construction).** A grep for `https?://` over the built bundle is NOT the gate: a
minified/concatenated Vite bundle defeats a substring match (runtime-assembled
URLs, base64, source-maps), and legitimate content contains the string
(`http://www.w3.org/2000/svg` in every inlined SVG, license/schema URLs) — so grep
both misses smuggled CDNs and forces an eroding allowlist. Instead:

1. **A `Content-Security-Policy: default-src 'self'` header** emitted by the Go
   handler serving `/builder`. The browser then **refuses any external load by
   construction** — script, style, font, image, connect — regardless of what the
   bundle text says. This is the real gate (and it doubles as the XSS mitigation
   ADR-0030 §6 relies on for the in-memory/sessionStorage bearer).
2. **A Playwright assertion in CI** that **fails on any network request to an
   origin other than same-origin** while `/builder` loads and runs. This catches a
   regression that a CSP `report-only` slip would miss.

The `https?://` grep is kept only as an **advisory** pre-scan, never the gate
(Phase 2a Principle 3: guarantee by construction, not a fragile string match).

### 6. CI: a frontend job, guards that bite, isolated from the Go pipeline

- A frontend CI job runs `npm ci`, typecheck, lint,
  `vitest`, and `vite build`. **CI always rebuilds `dist/` fresh and never trusts
  a committed one** — so there is no "staleness" to detect: the artifact that
  ships is the one CI just built from source. (The committed stub of §4 exists
  only for clean-clone compilation, never for release.)
- **The no-CDN gate is the CSP + Playwright same-origin assertion of §5**, not a
  text scan.
- **Guards must bite, not be checklists** (Phase 2a Principle 1 — an executable
  gate that fails when the property is violated): a **Vitest unit computes WCAG
  contrast ratios over the design-token table and asserts ≥ 4.5:1** for normal
  text (ADR-0030 §2/§8), and **axe-core runs inside the Playwright e2e** to assert
  labels and `:focus-visible` on interactive elements (ADR-0030 §8). A smuggled
  CDN, a sub-AA token pair, or a missing focus style fails CI — memory is not the
  gate.
- **Node/Playwright never gates the Go pipeline.** The frontend and e2e jobs are
  **separate** from the cross-compile ×6 and the GoReleaser release, so Node
  flakiness (browser download, e2e timing) cannot block a Go build or a release.
  The Go jobs depend only on the built `dist/` (via the §4 ordered step), not on
  the frontend test job passing.

### 7. Testing

- **Vitest v4 + React Testing Library** for components (form logic, config-document
  assembly, the bearer-header handling, reload-status rendering).
- **Playwright** for the one critical e2e: *paste token → edit a brain → Save →
  watch the reload reach `succeeded`* — and it must **survive the admin server
  restarting mid-cutover** (the poll tolerates connection-refused during the swap,
  ADR-0030 §5), plus the rejection paths (401 no token, 409 self-lock /
  reload-in-progress). The same Playwright run carries the no-CDN same-origin
  assertion (§5) and the axe-core a11y checks (§6).
- **Token/a11y guards that bite** (§6): a Vitest unit asserts WCAG AA contrast over
  the token table; axe-core asserts labels + `:focus-visible`.
- Versions (Vitest v4, Playwright current 1.x, `@testing-library/react` current)
  pinned to an **exact patch** and Context7-verified at scaffold.

## Consequences

- **Cost paid:** a second toolchain (Node + npm + `node_modules`), a CI build
  job, and a `go:embed` wiring step. Accepted: the builder is a real app and the
  cost buys quality the vanilla path cannot reach.
- **Preserved:** single binary, `go.mod` at 3 deps, zero CDN, cross-platform, the
  privacy posture. The frontend is entirely build-time.
- **Reversible everywhere except the dependency choice itself;** Motion, extra
  libraries, and CSS strategy remain add-later decisions.

## Alternatives considered

- **Extend the vanilla `/ui`** — rejected: forms + dirty state + async reload +
  theming is too much hand-rolled JS; worse quality, more code.
- **CSS Modules instead of Tailwind v4** — rejected in favor of token velocity
  (see §2); revisit if the utility approach fights the design system.
- **Motion / an animation lib on day one** — deferred (§3).
- **Preact / Svelte** — rejected: CLAUDE.md already standardizes on React; the
  ecosystem (RTL, Playwright, Tailwind) is the smoothest path.
