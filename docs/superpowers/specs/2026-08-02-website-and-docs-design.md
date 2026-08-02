# Web track SP1–SP5 — Public website + docs on GitHub Pages: Design Spec

> **Status:** approved for TDD (2026-08-02).
> Governing ADRs: ADR-0029 (frontend-toolchain precedent: build-time only,
> zero CDN, exact pins, guards that bite), ADR-0030 (visual identity: violet
> `#7A5AF5` accent, teal→violet gradient reserved for identity moments,
> token-first design system), **ADR-0040 (to be authored in SP1** — the
> website toolchain; the number is free on disk, last is 0039).
> External-docs note (Context7 + source, 2026-08-02):
>
> - **Context7 `/vuejs/vitepress`** verified: (1) **base path** — the `base`
>   site option (`'/korvun/'`, must start and end with `/`) is prepended
>   automatically to every internal URL starting with `/` and to static asset
>   paths; dynamic paths inside theme components use the `withBase` helper.
>   (2) **GitHub Pages deploy** — the documented Actions workflow: build job
>   (checkout → setup-node → `configure-pages` → `npm ci` → build → upload
>   `.vitepress/dist` via `upload-pages-artifact`) + deploy job
>   (`deploy-pages`), `permissions: contents:read / pages:write /
>   id-token:write`, `concurrency: group: pages`. (3) **i18n** — a `locales`
>   map with `root` (EN) + per-language keys (`es`), one directory per locale,
>   per-locale `themeConfig` (nav/sidebar), a translations menu in the navbar;
>   VitePress does NOT auto-redirect `/` to a locale — the root locale IS the
>   default. (4) **Theming/landing** — `layout: home` frontmatter with
>   `hero`/`features` blocks; hero name color/gradient via
>   `--vp-home-hero-name-*` CSS variables; custom theme = `extends:
>   DefaultTheme` + `enhanceApp` registering Vue components. (5) **Search** —
>   `themeConfig.search.provider: 'local'`: in-browser full-text via
>   MiniSearch, no external service, per-locale UI translations under
>   `search.options.locales`.
> - **npm registry (source)**: VitePress stable `latest` = **1.6.4**
>   (published 2025-08-05); the `next` tag is `2.0.0-alpha.19` — an alpha,
>   rejected (boring-by-default, ADR-0029 discipline). **Pin 1.6.4 exact.**
> - **Alternatives (Docusaurus / MkDocs)**: NOT escalated — the mandate was
>   to contrast only if something critical was uncovered, and all five
>   critical capabilities above are covered with evidence. ADR-0040 records
>   the rejection rationale (single-paragraph, no re-verification needed:
>   nothing forced the comparison).
> - GitHub Actions tags are verified **at source at implementation time**
>   (SP5), per the CLAUDE.md rule — this spec fixes the *convention*
>   (house style: third-party actions pinned to full commit SHA + `# vX.Y.Z`
>   comment; GitHub-owned actions on major tag + the repo's standard comment),
>   never the tags themselves.

## Goal

After this piece, Korvun has a public website at
**`https://sebastian197.github.io/korvun/`**: a branded product landing
(K terminal mark, ADR-0030 palette, Geist, the v0.6.0 launch clip embedded)
plus user-facing documentation generated from the repo's existing markdown —
navigable, with client-side full-text search, bilingual by layers (EN = the
complete source of truth; ES = landing + quickstart, expandable), deployed
automatically by a SHA-pinned-per-house-convention Actions workflow on every
master push that touches the site. Explicitly out: the custom domain
(`korvun.dev` — documented as a future extension: CNAME + DNS on top of what
is built, plus a single `base` flip to `'/'`; nothing else is redone), any
blog/versioned-docs machinery, operator-console docs (that piece has no spec
yet), and the day-D launch itself (HN / r/selfhosted / Product Hunt stay
reserved for the lienzo+web+beta convergence).

## Functional requirements

### Architecture & toolchain

- **FR-ARCH-1** — The site is a dedicated subproject at **`website/`**
  (own `package.json` + lockfile, VitePress **1.6.4 exact**), top-level and
  deliberately OUTSIDE `web/` — `web/` means "ships inside the Go binary"
  (ADR-0029 §4), `website/` means "published to Pages". Build-time only:
  **`go.mod`/`go.sum` untouched, zero Go dependencies, nothing embedded.**
  Traces to ADR-0040 (SP1 authors it).
- **FR-ARCH-2** — Project-page base path handled correctly: `base:
  '/korvun/'` in the VitePress config; all internal links written
  root-relative (VitePress prepends the base — Context7-verified); any
  dynamic asset path in custom theme components goes through `withBase`.
  The whole site must function when served under the `/korvun/`
  subdirectory, not only at a root dev server.
- **FR-ARCH-3** — Lockfile discipline per CLAUDE.md: regenerate with
  `npm install --include=optional`, verify `npm ci` clean **twice**,
  conflicting transitives pinned with exact `overrides`.

### Landing

- **FR-LAND-1** — `layout: home` extended with the brand: the K terminal
  mark (`assets/brand/korvun-logo-hero.svg`) as hero image; the teal→violet
  gradient appears ONLY as the hero identity moment (ADR-0030 §1 — e.g. via
  `--vp-home-hero-name-background`); violet `#7A5AF5` as the single
  functional accent; **Geist + Geist Mono self-hosted** via
  `@fontsource/geist{,-mono}` (SIL OFL — the Piece 5 SP6 precedent), zero
  CDN, no Google Fonts.
- **FR-LAND-2** — The launch clip embedded **self-hosted**:
  `korvun-v060-clip-1920x1080.mp4` (27 s, 1.3 MB) committed under the site's
  public assets, rendered with a native `<video>` element, poster frame,
  **click-to-play (no autoplay)**; the full demo
  (`korvun-v060-demo-full-1080p.mp4`, 43 s, 2.0 MB) as a secondary
  "watch the full demo" link. No YouTube iframe, no external host.
- **FR-LAND-3** — A features section mapping to the real pillars (gateway /
  multi-model router / multi-brain orchestrator / policy engine / visual
  builder / desktop app / self-hosted single binary), with actions to
  Quickstart and GitHub. **Every claim fact-checked against the repo**
  (the 2026-08-02 lesson: public content is verified like code).

### Documentation content

- **FR-DOCS-1** — EN page map, each page fed by existing markdown,
  rewritten to user language (never pasted verbatim where the source is
  contributor-facing):

  | Site page | Source material | Treatment |
  |---|---|---|
  | `/` (landing) | README pitch + brand | new copy, fact-checked |
  | `/guide/what-is-korvun` | README §What is Korvun + differentiator | adapted |
  | `/guide/install` | `docs/packaging/INSTALL.md` + desktop downloads | rewritten |
  | `/guide/quickstart` | `docs/QUICKSTART.md` | adapted (already user-facing) |
  | `/guide/builder` | `docs/BUILDER.md` | adapted |
  | `/reference/configuration` | `docs/CONFIGURATION.md` | adapted reference |
  | `/channels/telegram` | QUICKSTART steps + CONFIGURATION §telegram | new page |
  | `/channels/discord` | `docs/DISCORD-SETUP.md` | adapted |
  | `/channels/webhook` | `docs/WEBHOOK-SETUP.md` | adapted |
  | `/releases` | `docs/releases/*.md` + GitHub Releases | curated index, links out |

- **FR-DOCS-2** — Ownership rule recorded: repo `docs/` stays canonical for
  contributors; the site is the user-facing rewrite. The release runbook
  gains a "site pages updated?" checklist item so drift is caught at each
  release, not discovered by users.
- **FR-DOCS-3** — Client-side search: `search.provider: 'local'`
  (MiniSearch, in-browser, Context7-verified) — no Algolia or any external
  search service (posture coherence).

### i18n (Chano's decision 2026-08-02 — NOT an open point)

- **FR-I18N-1** — ~~Bilingual by layers (ES = landing + quickstart)~~
  **AMENDED (Chano, 2026-08-02, SP4b): the ES locale is a FULL MIRROR of
  EN** — same ten pages, same nav/sidebar tree, under `/es/`. EN remains
  the source of truth authored first; the technical truth (commands,
  paths, ports, env-var names) is byte-identical between locales and
  MECHANICALLY enforced (the locale-parity gate compares the fenced code
  blocks of every page pair).
- **FR-I18N-2** — The default-theme locale switcher links EN↔ES
  **per page** (every page has its twin; the SP2b-era `i18nRouting:
  false` workaround retired in SP4b). A future page added without its ES
  twin is a permanent harness red (`scripts/check-parity.mjs`). Search UI
  strings translated for `es` via `search.options.locales`.

### Finish & motion (the ROAD-TO-BETA bar)

- **FR-MOT-1** — Transitions are **native only**: CSS transitions +
  the View Transitions API as progressive enhancement. Sober, performance
  first (`transform`/`opacity` only). **Zero animation libraries** — adding
  one requires a new ADR (ADR-0029 §3 discipline).
- **FR-MOT-2** — **`prefers-reduced-motion` ALWAYS respected**: with
  `reduce`, every non-essential animation/transition is disabled and the
  video never auto-plays (it doesn't anyway, per FR-LAND-2).
- **FR-MOT-3** — Accessibility guards that bite (ADR-0029 §6 pattern):
  WCAG AA contrast asserted over the site's token pairs; axe checks on the
  landing's interactive elements.

### Deploy

- **FR-DEP-1** — A new workflow `.github/workflows/pages.yml`: build job on
  every push to `master` **path-filtered** to `website/**` (+ the workflow
  file itself) and on `workflow_dispatch`; deploy job (`deploy-pages`) only
  from `master`. PRs touching `website/**` run the build job as validation,
  never the deploy. Pinning per house convention (see External-docs note);
  current tags verified at source in SP5.
- **FR-DEP-2** — The workflow is **non-gating for the Go pipeline**
  (ADR-0029 §6: Node never blocks a Go build or a release).
- **FR-DEP-3** — Pages source = "GitHub Actions" in repo Settings — a
  one-time flip that is **Chano's** (repo settings), surfaced as an SP5
  prerequisite.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (base path)** Given the site built with `base: '/korvun/'`, When it
  is served under the `/korvun/` subdirectory (e.g. `vitepress preview`),
  Then every internal link, CSS/JS asset, font, image and the video resolve
  under `/korvun/…` — zero requests to root-absolute paths outside the base
  and zero 404s.
- **AS-2 (search)** Given the built site, When the user searches
  "quickstart", Then a local result navigates to `/korvun/guide/quickstart`
  — and the search issues **no network request to any external origin**.
- **AS-3 (zero CDN)** Given the landing fully loaded and interacted with,
  When network requests are recorded (the ADR-0029 §5 Playwright pattern),
  Then every request is same-origin — no fonts, scripts, styles or media
  from any external host.
- **AS-4 (reduced motion)** Given `prefers-reduced-motion: reduce`, When the
  landing loads and the user navigates, Then no non-essential
  animation/transition runs and the video does not play without an explicit
  click.
- **AS-5 (i18n layers)** Given the ES locale, When the user switches
  language on the landing, Then `/es/` renders the ES landing, the ES nav
  reaches the ES quickstart, and no ES nav entry points at a missing page;
  EN keeps the full page map.
- **AS-6 (deploy trigger)** Given a push to `master` touching only Go code,
  When Actions evaluates workflows, Then `pages.yml` does not run; given a
  push touching `website/**`, Then it builds and deploys, and the site
  updates.
- **AS-7 (Go untouched)** Given the piece complete, When `git diff` over
  `go.mod`/`go.sum` and `go build ./...` are checked, Then both are
  byte-identical in behavior to before the piece (nothing embedded, no new
  Go deps).
- **AS-8 (a11y gates)** Given the token pairs and the landing, When the
  contrast unit and axe checks run in CI, Then normal text is ≥ 4.5:1 and
  interactive elements have labels + `:focus-visible` — a violation fails
  the site CI job.
- **AS-9 (lockfile determinism)** Given a fresh clone, When `npm ci` runs
  twice in `website/`, Then both runs are clean with zero lockfile drift.

## Success criteria

- The site is live and fully functional at
  `https://sebastian197.github.io/korvun/` (every AS above green against
  the deployed site, not only locally).
- Site CI green (build under base + link integrity + AS-2/3/4/8 assertions);
  `make quality` stays green over the whole suite — the Go side is untouched
  and proven so (AS-7).
- Performance first: no render-blocking external requests (there are none —
  zero CDN), fonts as self-hosted `.woff2`, media ≤ 2 MB per file,
  animations on `transform`/`opacity` only.
- All public copy fact-checked against repo history/docs before it ships
  (the standing lesson from the 2026-08-02 launch).

## Decisions folded in

- **VitePress over Docusaurus/MkDocs** — all five critical capabilities
  Context7-verified with nothing uncovered (the mandate's escalation trigger
  never fired); Vite-family coherence with ADR-0029; ADR-0040 records the
  one-paragraph rejection of the alternatives.
- **VitePress 1.6.4 exact, not 2.0.0-alpha** — boring-by-default; alphas are
  not innovation tokens.
- **`website/` top-level, not under `web/`** — `web/` = embedded in the
  binary; `website/` = published to Pages; the boundary stays visible.
- **Launch clip committed to the site's public assets** — it is 1.3 MB and
  already public (posted natively on LinkedIn/X on 2026-08-02); self-hosting
  beats any external embed and keeps AS-3 clean.
- **`/releases` links out to GitHub Releases** with a curated index — no
  duplication of release notes to keep in sync.
- **Zero analytics/tracking** — coherent with the privacy posture; nothing
  to disclose, nothing to consent.
- **Social/meta** — `og:` / `twitter:` head tags using
  `assets/brand/korvun-social-preview.png` (copied into site assets).
- **Custom domain as documented future extension** — CNAME + DNS + one
  `base: '/'` flip; recorded in ADR-0040 so nothing built here has to be
  redone.
- **Node version in CI matches the house frontend jobs** (Node 24 line,
  `actions/setup-node` per repo convention).

## SP breakdown (house pattern — each SP: red gate first, then green)

- **SP1 — ADR-0040 + scaffold.** Author ADR-0040 (website toolchain:
  VitePress 1.6.4, build-time only, JAMÁS in `go.mod`, alternatives
  rejected, custom-domain extension path). Scaffold `website/` with base
  `'/korvun/'`, lockfile discipline, stub pages, and the **site CI test
  harness** (build-under-base + link integrity — the red that defines
  AS-1/AS-9).
- **SP2 — Landing.** Brand hero + fonts + video + features + motion;
  reds: AS-3 (same-origin), AS-4 (reduced motion), AS-8 (contrast/axe).
- **SP3 — Docs EN.** The FR-DOCS-1 page map populated (rewritten), nav +
  sidebar + local search; reds: AS-2 + link integrity over the full map.
- **SP4 — ES layer.** `/es/` landing + quickstart, locale switcher, search
  translations; red: AS-5.
- **SP5 — Deploy.** `pages.yml` (tags verified at source, house pinning),
  Pages setting flipped (Chano), live verification of every AS against the
  deployed URL; reds: AS-6 + the live re-run of AS-1/2/3.

## `[NEEDS CLARIFICATION]`

1. **Resolved (2026-08-02):** SP2 proceeds with ADR-0030 + the brand assets
   as the design of record; Claude Design refines afterwards on top of what
   is built (the canvas SP6 precedent).

No other open points: language layering and hosting are Chano's decisions
(2026-08-02, recorded above as law), the generator choice closed with
evidence, and the video/asset/search/analytics calls are folded in for the
review to veto.