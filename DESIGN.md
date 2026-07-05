# Design System — Korvun builder

The visual identity + token system for the Korvun builder UI (`/builder`), fixed by
**ADR-0030**. The machine-checked source of truth for color is
`web/builder/src/design/tokens.ts` (guarded by `tokens.wcag.test.ts`); this document
is the human-readable companion. Everything here is **self-hosted, zero CDN**
(ADR-0029 §5).

## Product context

- **What:** the config builder for Korvun, a self-hosted open-core Go binary
  (messaging gateway + multi-model router). Edited by **forms**, not a canvas.
- **Who:** the single operator running their own Korvun.
- **Peers (principles, not imitation):** Linear, Raycast, Warp, Zed. Korvun has its
  **own** identity; no brand/color/logo is copied.

## Aesthetic direction

- **Concept:** corvid / iridescent (Korvun ≈ *corvus*, raven), with a **⌘K command
  palette** — the name's meaning made into the identity, terminal heritage as
  personality, on top of a dense, always-oriented information architecture.
- **Mood:** serious, precise, ink-dark, high-craft. Not generic AI-SaaS.

## Color

One meaningful accent, kept OUT of the fixed status palette (trust: brand ≠ status).

- **Functional accent = violet** — `#8B7CF6` (dark) / `#6D5DE0` (light). **Teal is
  rejected as the accent** because it reads as the `sent` green at dot size.
- **Iridescent teal→violet** (`linear-gradient(135deg,#2DD4BF,#8B7CF6)`) is
  **identity-only** (glyph, one hairline, hero) — never on states, pills, or focus.
- **Neutrals (dark):** base `#0A0A0C` · surface `#131316` · surface-2 `#191920` ·
  border `#262633`. **Text tiers (all pass WCAG AA):** text-1 `#E8E8EC` · text-2
  `#C2C2CE` · text-3 `#9EA0AC` (labels live here, never sub-AA).
- **Neutrals (light):** base `#F7F7F9` · surface `#FFFFFF` · border `#E3E3E9` ·
  text-1 `#16161B` · text-2 `#3C3C46` · text-3 `#5A5A66` · accent `#6D5DE0`.
- **Fixed event palette (verbatim from `/ui`, ADR-0024):** received `#3B82F6` · sent
  `#22C55E` · dropped `#F59E0B` · failed `#EF4444`. Used as status dots (graphical,
  ≥ 3:1), never as the brand accent, and never as the ONLY channel (icon/label too).
- **Light + dark are defined from day one**; the theme is a CSS-variable swap on
  `[data-theme]`, which feeds the View Transitions theme change.

## Typography (all SIL OFL, self-hosted woff2)

- **Display:** Archivo (600/700) — sharp grotesque, titles/wordmark.
- **UI / body:** IBM Plex Sans (400/500/600) — a NEUTRAL sans, calmer than the
  display, for dense forms.
- **Mono / data:** IBM Plex Mono (400/500) — IDs, values, labels-that-are-values.
- Loaded via Fontsource and bundled locally by Vite; license files in
  `web/builder/fonts/LICENSES/`. Satoshi was excluded (Fontshare, not OFL).

## Spacing · radii · motion

- **Spacing (base 4px):** 2xs 2 · xs 4 · sm 8 · md 16 · lg 24 · xl 32 · 2xl 48.
- **Radii:** sm 6 · md 9 · lg 13.
- **Motion:** easing enter `cubic-bezier(0.16,1,0.3,1)` / exit `ease-in`; durations
  micro 120ms · base 180ms · panel 220ms; `prefers-reduced-motion` respected. Start
  with CSS transitions + the View Transitions API; Motion is deferred.

## Accessibility floor (enforced, not aspirational)

- **AA contrast** over the whole token table — an executable Vitest guard
  (`tokens.wcag.test.ts`) fails CI on a sub-AA pair (Phase 2a Principle 1).
- **`:focus-visible` on every interactive element**, not just inputs.
- Color is never the sole channel; touch targets ≥ ~32–40px. (axe-core in the
  Playwright e2e lands with the edit forms in 2b.2.)
