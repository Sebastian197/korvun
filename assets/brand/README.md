# Korvun brand — logo sources

The **"K terminal"** mark: the letter **K** knocked out of a rounded tile, a nod to
a shell's `|<` (the terminal Korvun orchestrates).

## Identity palette (ADR-0030)

- **Violet `#7A5AF5`** — the single identity/accent color. Chano's decision
  (2026-07-18) unified the mark on it: use `#7A5AF5` for any flat-ink rendering
  (mono logo, favicon, avatar, the CLI accent).
- **Gradient teal `#2BC8B7` → violet `#7A5AF5`** — reserved for **identity moments
  only** (the hero signature), never for functional UI color.
- **Monochrome-safe by construction** — the shape reads whole in a single color,
  independent of the gradient.

## Files

| File | Use |
|------|-----|
| `korvun-logo-hero.svg` | Hero signature (teal→violet gradient). README header, social preview, splash. |
| `korvun-logo-mono.svg` | Flat violet `#7A5AF5` mark. Favicon, single-ink contexts, embeds where the gradient is out of place. |
| `korvun-avatar-512.png` | 512×512 avatar for the GitHub org/repo and social profiles. |

## Pending (Chano, via web)

- Upload the avatar to the GitHub profile/repo; set the hero as the social preview.
- Derive the CLI header ASCII art from this mark (`internal/cli` ships an honest
  placeholder banner today).
