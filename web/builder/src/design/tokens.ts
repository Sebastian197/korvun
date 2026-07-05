// Korvun builder design tokens (ADR-0030). The SINGLE source of truth for color:
// the CSS variables in styles/theme.css mirror these values, and the WCAG guard
// (tokens.wcag.test.ts) asserts the contrast floor over THIS table so a sub-AA
// pair fails CI (Phase 2a Principle 1 — an executable gate, not a checklist).
//
// Identity (ADR-0030 §1): cool ink neutrals, a VIOLET functional accent anchored
// OUTSIDE the fixed event palette, and the semantic event colors reused verbatim
// from the read-only /ui (ADR-0024). The iridescent teal->violet gradient is an
// identity-only flourish (glyph/hairline), never a functional/text color, so it is
// NOT part of the contrast-checked table.

export interface Theme {
  base: string
  surface: string
  surface2: string
  border: string
  /** Readable text tiers. ALL must pass WCAG AA (>= 4.5:1) on base and surface. */
  text1: string
  text2: string
  text3: string
  /** Violet accent as TEXT (links, active labels); must pass AA. */
  accentText: string
  /** Violet accent as a FILL (primary button bg); paired with onAccent text. */
  accent: string
  onAccent: string
  /** Fixed event palette (ADR-0024), used as small status dots — graphical, so
   *  checked at the 3:1 UI-component floor on surface, not the 4.5 text floor. */
  received: string
  sent: string
  dropped: string
  failed: string
}

export const dark: Theme = {
  base: '#0A0A0C',
  surface: '#131316',
  surface2: '#191920',
  border: '#262633',
  text1: '#E8E8EC',
  text2: '#C2C2CE',
  text3: '#9EA0AC',
  accentText: '#A78BFA',
  accent: '#8B7CF6',
  onAccent: '#0A0A0C',
  received: '#3B82F6',
  sent: '#22C55E',
  dropped: '#F59E0B',
  failed: '#EF4444',
}

export const light: Theme = {
  base: '#F7F7F9',
  surface: '#FFFFFF',
  surface2: '#F1F1F4',
  border: '#E3E3E9',
  text1: '#16161B',
  text2: '#3C3C46',
  text3: '#5A5A66',
  accentText: '#6D5DE0',
  accent: '#6D5DE0',
  onAccent: '#FFFFFF',
  received: '#2563EB',
  sent: '#15803D',
  dropped: '#B45309',
  failed: '#B91C1C',
}

export const themes = { dark, light } as const

// ---- WCAG contrast math (sRGB, per the WCAG 2.x relative-luminance formula) ---

function channel(c: number): number {
  const s = c / 255
  return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
}

/** Relative luminance of a `#rrggbb` color. */
export function luminance(hex: string): number {
  const h = hex.replace('#', '')
  const r = parseInt(h.slice(0, 2), 16)
  const g = parseInt(h.slice(2, 4), 16)
  const b = parseInt(h.slice(4, 6), 16)
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

/** WCAG contrast ratio between two `#rrggbb` colors (1..21). */
export function contrast(a: string, b: string): number {
  const la = luminance(a)
  const lb = luminance(b)
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

/** AA floor for normal text. */
export const AA_TEXT = 4.5
/** Non-text (UI component / graphical) contrast floor. */
export const UI_MIN = 3.0
