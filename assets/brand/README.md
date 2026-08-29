# Korvun brand — the governed K (A1)

The **A1 governed K** (adopted 2026-08-29, director-approved over the rendered
korvun-brand-motion mockup): the letter **K** knocked out of a rounded tile,
now telling the routing story — three model signals enter, policy decides
inside the K, and one controlled response exits. Design sources live in
`design-drafts/korvun-brand-motion/logos/`; these files are the promoted
production assets, validated by `website/scripts/check-brand.mjs`
(canonical/public parity, layer names, gradient endpoints, GIF budget).

## Identity palette (ADR-0030)

- **Gradient `#2BC8B7 → #7A5AF5`** — identity-only, locked: diagonal
  top-left to bottom-right, no intermediate stops. Never used for
  functional UI states.
- **Violet `#7A5AF5`** — the single-ink identity color; the mono mark's fill
  and the accent the product UIs already use.

## Assets

| File | What it is |
|------|------------|
| `korvun-logo-hero.svg` | The cinematic hero mark (560×440): governed K, two orbits, three input signals, decision node, output signal, four deterministic particles — the named layers the website masthead animates. Mirrored byte-for-byte at `website/static/brand/`. |
| `korvun-logo-mono.svg` | The compact K (64×64) in flat violet — the ADR-0030 single-ink form for functional contexts. Mirrored at `website/static/brand/`. |
| `korvun-readme-masthead.gif` | The controlled README masthead: one-shot 1280×480 @16fps, 5.2s (quiet → three-signal convergence → decision → one output → final hold), ≤2.5 MiB, no infinite loop — it settles on the exact static source frame. Rendered deterministically (Playwright frame captures over a paused master clock) and assembled with the pinned imageio-ffmpeg 0.6.0 static ffmpeg 7.1 (the v0.6.0 clip precedent). |
| `korvun-avatar-512.png` | 512×512 transparent avatar from the compact A1 K (also the source of the desktop app icon at `cmd/korvun-desktop/build/appicon.png`, from which Wails derives the macOS/Windows/Linux icons). |
| `korvun-social-preview.png` | 1280×640 social card: the sealed README-masthead final frame on the house background. |
| `korvun-logo.svg` / other legacy files | Pre-A1 "K terminal" era; superseded — kept only where still referenced. |
