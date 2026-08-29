# Website brand-motion system — governed K and routing journey: Design Spec

> **Status: VERIFIED — implemented, corrected to the literal mockup and
> approved at the visual curtain (telón 2) by Chano, 2026-08-29.** Shipped
> with v0.10.0; deviations from the mockup on record: the A1 gradient K
> (the director's call over the mockup's flat-teal rendering artifact), the
> gradient navbar mark, and the no-cuts/no-dim uniformity laws superseding
> the mockup's separators and pending dim.
> Original approval: **approved for TDD — APROBADO POR CHANO 2026-08-29** (the
> design-drafts/korvun-brand-motion mockup seen and approved by LOOKING at
> it — the Sixth Law's gate). Two conditions recorded with the yes:
> 1. **The landing's CURRENT transitions and animations are UNTOUCHABLE.**
>    They were already this package's own contract; the approval seals it
>    twice — the brand lote adds, it never rewrites existing motion.
> 2. **The desktop APP ICON (macOS/Windows/Linux) enters the brand lote's
>    scope**, regenerated from the governed A1 K.
> The lote (IMPLEMENTATION-PLAN.md + app icon + ADR-0030 updated to the A1
> identity) is queued for RIGHT AFTER v0.10.0 (see HANDOFF).
> Governing sources: ADR-0030 (website identity palette),
> `docs/superpowers/specs/2026-08-02-website-and-docs-design.md`, and the
> existing landing motion contract in `website/src/theme/storytelling.ts`.
> External-docs note: this piece adds no runtime dependency and no new package
> API. It uses inline SVG, CSS, existing React/Docusaurus component seams, and
> browser-platform APIs already available to the website. `package.json` and
> lockfiles must remain unchanged.

## Goal

Replace the current Korvun mark with the approved A1 "governed K" identity and
turn the public landing page into a continuous routing story. The website hero
will use the cinematic Vite-scale masthead; the GitHub README will use the
controlled one-shot masthead; and a scroll-driven circuit will connect the existing
six landing sections from the hero to the final CTA. The current copy,
section order, links, release facts, terminal, capability cards, privacy diagram,
video, locale parity, and below-fold reveal transitions stay intact. This piece
does not redesign documentation pages, change product behavior, add a JavaScript
animation library, or alter the Go binary.

## Functional requirements

- **FR-BRAND-1** — The canonical A1 identity must depict three colored input
  signals entering a rounded gradient tile, a negative-space K acting as the
  policy boundary, and one neutral response leaving it. The identity gradient
  remains `#2BC8B7` to `#7A5AF5`, as governed by ADR-0030.
- **FR-BRAND-2** — The brand asset family must provide a compact core mark for
  navigation, favicon, avatar, and single-ink contexts, plus an extended routing
  signature for large identity moments. Both forms must remain recognizable in
  monochrome and at their intended minimum sizes.
- **FR-BRAND-3** — The primary wordmark remains `Korvun` in the website's Geist
  Sans voice, semibold and compact. `Korvun.dev` remains a domain reference, not
  part of the product name.
- **FR-BRAND-4** — Canonical assets under `assets/brand/` and their public mirrors
  under `website/static/brand/` must be updated together. Avatar and social
  preview derivatives must be regenerated from the approved mark. The existing
  public asset paths must remain valid where possible to avoid broken embeds.

- **FR-HERO-1** — `website/src/components/landing/Hero.tsx` must replace the
  current static hero `<img>` with a dedicated inline-SVG masthead component. It
  must preserve all hero copy, actions, release facts, rail labels, semantic
  heading structure, and locale behavior.
- **FR-HERO-2** — The website masthead must use the approved cinematic motion:
  restrained dimensional tilt, two slow counter-rotating orbits, exactly four
  deterministic particles, three input signals, one decision pulse, and one
  outgoing response. Every animated layer must use `transform`, `opacity`, or
  `filter`, preserving the existing motion-quality gate.
- **FR-HERO-3** — Masthead animation must run only while the hero is visible and
  the user has not requested reduced motion. When paused, the mark must retain a
  deliberate, readable static frame.
- **FR-HERO-4** — The compact mark in the website navigation must use the new A1
  core geometry without the extended routes, so it remains legible at 24–32 px.

- **FR-JOURNEY-1** — `LandingPage.tsx` must retain the exact ordered section
  sequence `hero`, `install`, `capabilities`, `privacy`, `demo`, `final`. Each
  section must expose a visual route port on its meaningful product block:
  masthead, install terminal, capability grid, privacy diagram, demo frame, and
  final CTA respectively.
- **FR-JOURNEY-2** — A dedicated routing-journey layer must connect those ports
  with one continuous technical circuit. Desktop geometry may alternate through
  the available gutters; mobile geometry must collapse to a single side rail
  with short branches into the content. The route must not overlap readable text,
  controls, or video controls.
- **FR-JOURNEY-3** — Scroll progress must progressively reveal the active circuit,
  move a single signal head along it, and assign each section one of `pending`,
  `active`, or `complete`. Arrival must activate the target block before the
  signal leaves for the next section.
- **FR-JOURNEY-4** — Scroll work must be coalesced through
  `requestAnimationFrame`. A scroll frame may read the current scroll position and
  write only route progress, signal transform, and section state. Port geometry
  may be measured at initialization and resize, but not repeatedly on every
  scroll frame.
- **FR-JOURNEY-5** — The active route must be revealed by a transformed SVG clip
  or equivalent transform-based mask. CSS keyframes and transitions must continue
  to animate only `transform`, `opacity`, `filter`, or `none`; the approved
  `website/scripts/check-motion.mjs` contract must not be weakened.
- **FR-JOURNEY-6** — The capability section may temporarily branch the signal
  across its eight existing cards, then rejoin the main circuit. The privacy
  section must emphasize the local eligible path while leaving the excluded cloud
  path visibly blocked. These effects must not change content or link behavior.
- **FR-JOURNEY-7** — The journey controller must clean up scroll/resize listeners,
  observers, animation-frame work, inline state, and diagnostic attributes on
  Docusaurus route changes or React unmount.

- **FR-MOTION-1** — The existing `armStorytelling` reveal behavior remains a
  separate contract: the hero is never hidden, below-fold `[data-motion]` targets
  receive their current bounded stagger, reveal once, and remain visible.
- **FR-MOTION-2** — The new route states must complement, not replace, the current
  reveal transitions, hero float, and demo/video treatment. A block cannot be
  hidden solely because the routing controller failed or has not initialized.
- **FR-MOTION-3** — With `prefers-reduced-motion: reduce`, all content must be
  immediately visible, the route must appear as a complete static circuit, the
  signal head must be absent, and no meaningful masthead or section animation may
  run.
- **FR-MOTION-4** — Without `IntersectionObserver`, `ResizeObserver`, or active
  JavaScript, the landing content and actions must remain usable. The fallback may
  show a static low-contrast circuit or omit the active overlay, but it must never
  dim or conceal content.

- **FR-README-1** — The README header must replace its current 150 px static mark
  with the approved controlled V1 masthead: frontal A1 mark, restrained halo,
  three input signals, one decision pulse, and one outgoing response. It must not
  contain orbit, particle, or dimensional-tilt layers.
- **FR-README-2** — The README animation must be a repository-hosted, one-shot
  raster animation that settles on a clean final frame instead of looping
  indefinitely. It must be no wider than 1280 px, no taller than 520 px, and no
  larger than 2.5 MiB.
- **FR-README-3** — The README's product name, expansion, description, website
  links, badges, and following desktop screenshot must remain unchanged. The new
  masthead must have useful alternative text.

- **FR-TEST-1** — Brand asset checks must assert required SVG view boxes, the A1
  compact K mask, gradient endpoints, and synchronized canonical/public copies.
- **FR-TEST-2** — Unit tests for the routing controller must cover ordered state
  changes, monotonic progress, `requestAnimationFrame` coalescing, reduced-motion
  fallback, missing-observer fallback, resize recomputation, and complete cleanup.
- **FR-TEST-3** — Playwright coverage must exercise both locales, desktop and
  390 px mobile layouts, the route from hero to final, preservation of the
  existing reveal-once behavior, no horizontal overflow, and reduced motion.
- **FR-TEST-4** — The existing accessibility, release-fact, same-origin media,
  link-destination, locale-parity, and motion-property checks must remain green
  without weakening their assertions.

## Acceptance scenarios (Given / When / Then)

- **AS-BRAND-1** Given the canonical hero and mono SVG sources, When the brand
  asset test parses them, Then the files expose the approved gradient endpoints,
  A1 K mask, compact/extended variants, and matching public mirrors.
- **AS-BRAND-2** Given the website at either supported locale, When the landing
  renders, Then the navigation shows the compact A1 mark and the hero shows the
  extended governed-K masthead without changing the H1 text or actions.
- **AS-HERO-1** Given normal motion and a visible hero, When the masthead is
  observed, Then its running state enables the input, decision, output, float,
  tilt, two-orbit, four-particle, and glow layers with non-zero durations authored
  only on approved motion properties.
- **AS-HERO-2** Given the masthead has left the viewport, When its visibility
  observer reports it hidden, Then the running state is removed or paused and no
  continuous masthead work remains active.
- **AS-JOURNEY-1** Given a desktop viewport and normal motion, When the user
  scrolls from the hero through the final CTA, Then route progress never moves
  backwards for forward scrolling, the active state visits all six section names
  in document order, and the final section reaches `complete` at the end.
- **AS-JOURNEY-2** Given the capabilities section is active, When the signal
  enters its port, Then all eight existing cards remain present and usable while
  the temporary branch treatment visits them before rejoining the main route.
- **AS-JOURNEY-3** Given the privacy section is active, When the signal reaches
  its diagram, Then `local model` is visually eligible and `cloud model` remains
  visibly excluded; the authored privacy copy is unchanged.
- **AS-JOURNEY-4** Given a 390 px viewport, When the user scrolls the landing,
  Then the route uses the mobile side rail, every port remains inside the viewport,
  the terminal and video stay usable, and document width does not exceed viewport
  width.
- **AS-MOTION-1** Given normal motion, When a below-fold target first enters the
  viewport, Then the existing `k-reveal`/`k-in` contract reveals it once and the
  route activation does not reset that transition.
- **AS-MOTION-2** Given reduced motion, When either locale loads, Then every
  section is visible, the circuit is complete and static, the signal head is not
  rendered, and computed meaningful animation/transition durations are at most
  0.001 seconds.
- **AS-FALLBACK-1** Given a runtime without `IntersectionObserver` or
  `ResizeObserver`, When the landing loads, Then every heading, link, terminal,
  capability card, privacy node, video, and CTA remains visible and usable with no
  routing-state dimming.
- **AS-LIFECYCLE-1** Given the route controller is armed, When Docusaurus rerenders
  or unmounts the landing, Then observers disconnect, pending animation frames are
  cancelled, listeners are removed, and route state/styles are cleared.
- **AS-README-1** Given GitHub renders `README.md`, When the header asset loads,
  Then it plays the controlled A1 routing sequence once without orbit, particles,
  or tilt, settles on a readable final frame, stays within the file-size/dimension
  budget, and leaves the rest of the header content unchanged.
- **AS-REGRESSION-1** Given a production build served under `/korvun/`, When the
  complete Playwright and Axe suites run for EN and ES, Then the six-section order,
  headings, links, release facts, same-origin website media, WCAG A/AA scan, and
  existing desktop/mobile contracts all remain green.

## Success criteria

- All new routing and asset unit contracts pass; frontend coverage does not fall
  below the current project gate.
- `npm run check` is green, including unit tests, TypeScript, locale parity,
  production build, distribution checks, documentation checks, contrast, and the
  unmodified motion-property gate.
- Playwright is green against the built site under the real `/korvun/` base path
  for EN, ES, desktop, 390 px mobile, and reduced motion.
- `make quality` is green over the whole repository, including `-race` where the
  project quality target applies it.
- No new runtime or development dependency is added; `website/package.json` and
  lockfiles are byte-for-byte unchanged.
- The README masthead is at most 1280×520 and 2.5 MiB, plays once, and settles on
  its final frame.
- Scroll handling is rAF-coalesced, performs no per-frame geometry measurement,
  and leaves no active work after cleanup or while the masthead is off-screen.
- The headless Go binary, desktop application, release pipelines, public routes,
  landing copy, and product behavior remain untouched. Git diff and the complete
  quality gate provide the proof.

## Decisions folded in

- **A1 governed K is canonical.** It preserves the current K recognition while
  making the product's routing and policy role visible.
- **Korvun remains the product name.** The `.dev` suffix is reserved for domain
  references.
- **Cinematic web, controlled README.** The website carries the stronger living
  brand moment while it is visible; the README uses a cleaner frontal sequence
  that plays once and settles.
- **No Rive or animation dependency.** Inline SVG, CSS, and a small native
  controller deliver the approved behavior with less payload and no new runtime
  trust surface.
- **Ports belong to meaningful blocks.** The route connects product evidence,
  rather than acting as a detached progress indicator.
- **One main circuit, local branches.** The page retains a readable primary path;
  capability and privacy detail appear only while their section is active.
- **Transform-based reveal.** An SVG clip/mask and transformed signal head preserve
  the existing motion-property contract instead of weakening it.
- **Current reveals are additive.** `armStorytelling` remains independently
  testable; routing state cannot hide content.
- **Static reduced-motion state.** Accessibility keeps the explanatory topology
  while removing travel, pulsing, floating, and staggered reveal.
- **Responsive geometry is explicit.** Desktop uses content gutters; mobile uses
  a side rail. The system does not attempt one fragile path for every viewport.
- **README animation plays once.** This retains the controlled routing explanation
  without forcing an endless loop on GitHub readers.

## `[NEEDS CLARIFICATION]`

None. The brand direction, web masthead, README masthead, scroll journey,
transition preservation, responsive fallback, and reduced-motion behavior were
approved during the visual design review.
