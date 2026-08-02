# ADR-0040: Web track — public website + docs toolchain (VitePress on GitHub Pages)

> **Status:** accepted
> **Date:** 2026-08-02
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

The web track (ROAD-TO-BETA, parallel to the beta queue) needs a public
website: a branded product landing plus user-facing documentation generated
from the repo's existing markdown, served free at
`https://sebastian197.github.io/korvun/` (Chano's decision 2026-08-02:
GitHub project page; the custom domain is a future extension, see below).
The site is bilingual by layers (EN = complete source of truth; ES = landing
+ quickstart, expandable) and must meet the finish bar recorded in
ROAD-TO-BETA: professional, sober native motion (CSS + View Transitions),
`prefers-reduced-motion` always, performance first, no animation library
without an ADR.

Like the builder frontend (ADR-0029), this is a second-toolchain decision —
and the same discipline applies: the toolchain is **build-time only** and
must never touch the Go module.

### External-docs note (Context7 + source, 2026-08-02)

Verified via Context7 (`/vuejs/vitepress`), recorded in full in the design
spec (`docs/superpowers/specs/2026-08-02-website-and-docs-design.md`):

- **Base path**: the `base` site option (`'/korvun/'`) is automatically
  prepended to internal URLs starting with `/` and to static asset paths;
  dynamic paths in theme components use the `withBase` helper. Project
  pages under a subdirectory are a documented first-class deployment.
- **GitHub Pages deploy**: an official Actions workflow shape (build job
  uploading `.vitepress/dist` via the Pages artifact actions + deploy job),
  with `pages: write` / `id-token: write` permissions and a `pages`
  concurrency group.
- **i18n**: a `locales` map (`root` = default locale served at `/`, plus
  per-language keys with their own directory, nav and sidebar) — exactly
  the layered EN/ES shape decided; no auto-redirect magic to fight.
- **Theming/landing**: `layout: home` frontmatter (hero/features), custom
  theme via `extends: DefaultTheme` + `enhanceApp`, hero identity gradient
  expressible through `--vp-home-hero-name-*` CSS variables.
- **Search**: `themeConfig.search.provider: 'local'` — in-browser full-text
  (MiniSearch), per-locale UI translations, **no external service**.

Version verified **at source** (npm registry, 2026-08-02): stable `latest`
= **1.6.4** (2025-08-05). The `next` tag is `2.0.0-alpha.19` — an alpha,
rejected outright (boring-by-default; alphas are not innovation tokens).

## Decision

### 1. Generator: VitePress 1.6.4, pinned exact

- **VitePress `1.6.4`** (exact pin, never a range — ADR-0029 discipline) in
  a dedicated subproject at **`website/`**, with its own `package.json` and
  lockfile regenerated per the CLAUDE.md rule (`npm install
  --include=optional`, `npm ci` clean twice, exact `overrides` for any
  conflicting transitive).
- `website/` is deliberately **top-level, not under `web/`**: `web/` means
  "embedded in the Go binary and served by Korvun" (ADR-0029 §4);
  `website/` means "published to GitHub Pages". The boundary stays visible
  in the tree.

### 2. Build-time only — NEVER in `go.mod`

The website is static output built in CI and published to Pages. It adds
**zero Go dependencies**, embeds nothing in the binary, and is invisible to
`go build ./...`, the cross-compile ×6, GoReleaser and the quality gate.
`go.mod`/`go.sum` byte-identical before and after the piece (asserted as
AS-7 in the spec).

### 3. Zero CDN, zero analytics (posture)

- **Zero CDN**: fonts self-hosted (Geist/Geist Mono via
  `@fontsource/geist{,-mono}`, SIL OFL — the Piece 5 SP6 precedent), the
  launch video committed and served same-origin, search in-browser
  (`provider: 'local'`), no external script/style/font/frame of any kind.
  Same-origin is asserted with the ADR-0029 §5 Playwright pattern, not a
  text grep.
- **Zero analytics/tracking**: nothing to disclose, nothing to consent —
  coherent with the self-hosted privacy promise the site itself advertises.

### 4. Deploy: Actions → Pages, house pinning convention

A dedicated `pages.yml` (SP5): build on pushes to `master` path-filtered to
`website/**`, deploy job only from `master`, non-gating for the Go pipeline
(ADR-0029 §6: Node never blocks a Go build or release). Action tags follow
the house convention — third-party actions pinned to a full commit SHA with
a `# vX.Y.Z` comment; GitHub-owned actions on a major tag with the repo's
standard comment — and are verified at source when SP5 writes the workflow.

### 5. Custom domain: a future extension, not a dependency

Adopting `korvun.dev` (or any custom domain) later requires exactly: a
`CNAME` file in the published site, the DNS records at the registrar, and
flipping `base` from `'/korvun/'` to `'/'`. Nothing else built on this ADR
has to be redone. Until that day, the free `sebastian197.github.io/korvun`
URL is the canonical one.

## Consequences

- **Cost paid:** a third npm subproject (after `web/builder` and the
  desktop frontend) with its own lockfile to keep deterministic, and one
  more CI workflow (SP5).
- **Preserved:** `go.mod` untouched, single-binary promise untouched (the
  site is not the product — it documents it), zero CDN / zero analytics,
  the ADR-0029 finish discipline (exact pins, guards that bite).
- **Reversible:** the site is static output; swapping the generator later
  is a content-porting exercise, not an architecture change.

## Alternatives considered

- **Docusaurus / MkDocs** — not escalated. The spec's mandate was to
  contrast an alternative only if Context7 left something critical
  uncovered; all five critical capabilities (base path, Pages deploy, i18n,
  custom landing/theming, local search) were verified for VitePress with
  evidence. On top of that, VitePress keeps the house Vite-family
  coherence (ADR-0029: Vite/Vitest already in the builder and desktop
  frontends), while MkDocs would introduce a Python toolchain and
  Docusaurus a heavier React runtime for no capability we need.
- **VitePress 2.0.0-alpha** — rejected: alpha line; 1.6.4 is the stable
  `latest`.
- **`web/site/` placement** — rejected: `web/` is reserved for
  binary-embedded frontends; a Pages-published tree there would blur the
  boundary the repo relies on.
- **Algolia DocSearch** — rejected: an external service against the
  zero-CDN/zero-analytics posture; local MiniSearch covers the need.
