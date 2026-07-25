// Korvun desktop-chrome design tokens (SP6 spec FR-WIN-8). EXTRACTED from the
// design-track source of truth (`design-drafts/Korvun Desktop
// (standalone).html` CSS variables) — never re-invented. The CSS variables in
// styles/theme.css mirror these values 1:1, and the WCAG guard
// (tokens.wcag.test.ts) plus the palette scan (palette.scan.test.ts) make the
// system executable law (ADR-0030 §2 pattern, cloned from web/builder).
//
// Identity: violet #7A5AF5 is THE functional accent; teal #2BC8B7 exists only
// inside the identity gradient (one primary action per view — its own scan).
// Status tints (ok/warn/err/off) are pill/badge accents over their -s tinted
// surfaces, held to the 3:1 UI floor; the text tiers and violet-as-text hold
// the 4.5:1 AA floor.
//
// CONSCIOUS DEVIATION — RATIFIED (copilot, 2026-07-25; the design-track
// notice is the copilot's to deliver): the mock's literal --tx3/--capt
// assignments fail the AA gate in their own theme (dark #68687C/#54546A
// ≈ 3.5/2.2:1 on --win) — the themes' values are SWAPPED here so both
// pass. The AA law outranks the mock's literal (ADR-0030 §2); every other
// value is byte-faithful to the source.
//
// SECOND CONSCIOUS DEVIATION (SP6b, same ratified principle, flagged for the
// design track): status tokens carry 11 px text in pills, so they must clear
// the AA TEXT floor (4.5:1) on their tinted pill surfaces — what axe-core
// enforces in e2e. Darkened/lightened MINIMALLY, same hue: light okT
// #0E8B7D→#0B7568, light warnT #96660F→#8F610E, light offT #75758A→#6A6A80,
// dark offT #8A8A9E→#8F8FA4. Everything else stays byte-faithful.

export interface ChromeTheme {
  /** Window/backdrop ramp. */
  desk: string
  win: string
  side: string
  card: string
  card2: string
  /** Hairlines. */
  line: string
  line2: string
  /** Text tiers — ALL must pass AA (>= 4.5:1) on win/side/card/card2. */
  tx1: string
  tx2: string
  tx3: string
  capt: string
  /** Violet accent as TEXT — must pass AA. */
  vioT: string
  /** Violet accent as a FILL, paired with onVio text. */
  vio: string
  onVio: string
  /** Status tints (pill text over tinted surfaces) — 3:1 UI floor on cards. */
  okT: string
  warnT: string
  errT: string
  offT: string
  /** Sidebar hover/active surfaces. */
  navH: string
  navOn: string
}

export const dark: ChromeTheme = {
  desk: '#08080D',
  win: '#0F0F16',
  side: '#12121A',
  card: '#16161F',
  card2: '#1B1B26',
  line: '#232330',
  line2: '#1E1E29',
  tx1: '#EEEEF4',
  tx2: '#A2A2B5',
  tx3: '#8D8D9F',
  capt: '#84849A',
  vioT: '#A28BFF',
  vio: '#7A5AF5',
  onVio: '#FFFFFF',
  okT: '#4CD7C6',
  warnT: '#E7B45D',
  errT: '#F17E86',
  offT: '#8F8FA4',
  navH: '#17171F',
  navOn: '#1D1D29',
}

export const light: ChromeTheme = {
  desk: '#E8E8EF',
  win: '#FAFAFC',
  side: '#F3F3F7',
  card: '#FFFFFF',
  card2: '#F4F4F8',
  line: '#E2E2EB',
  line2: '#ECECF3',
  tx1: '#191922',
  tx2: '#5B5B70',
  tx3: '#68687C',
  capt: '#54546A',
  vioT: '#6644E8',
  vio: '#7A5AF5',
  onVio: '#FFFFFF',
  okT: '#0B7568',
  warnT: '#8F610E',
  errT: '#C13A42',
  offT: '#6A6A80',
  navH: '#EFEFF5',
  navOn: '#E8E8F0',
}

export const themes = { dark, light } as const

/** The identity gradient — ONE primary action per view (its own scan). */
export const IDENTITY_GRADIENT = 'linear-gradient(120deg,#2BC8B7,#7A5AF5)'
export const TEAL = '#2BC8B7'

/** Status/violet tinted pill surfaces as [r, g, b, alpha] — the -s variables
 * in theme.css mirror these (rgba over the card). Pills carry 11 px TEXT, so
 * the paired *T token must clear the AA text floor over the COMPOSITE of the
 * tint on card (the wcag gate computes it). */
export type Tint = readonly [number, number, number, number]

export const tints: Record<'dark' | 'light', Record<'ok' | 'warn' | 'err' | 'off' | 'vio', Tint>> = {
  dark: {
    ok: [43, 200, 183, 0.12],
    warn: [224, 164, 60, 0.14],
    err: [229, 72, 77, 0.14],
    off: [140, 140, 160, 0.13],
    vio: [122, 90, 245, 0.14],
  },
  light: {
    ok: [43, 200, 183, 0.15],
    warn: [224, 164, 60, 0.16],
    err: [229, 72, 77, 0.11],
    off: [120, 120, 145, 0.12],
    vio: [122, 90, 245, 0.11],
  },
}

/** Composite an rgba tint over an opaque `#rrggbb` base. */
export function composite(tint: Tint, baseHex: string): string {
  const h = baseHex.replace('#', '')
  const base = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16))
  const [r, g, b, a] = tint
  const mix = (c: number, bc: number): number => Math.round(c * a + bc * (1 - a))
  return `#${[mix(r, base[0] ?? 0), mix(g, base[1] ?? 0), mix(b, base[2] ?? 0)]
    .map((c) => c.toString(16).padStart(2, '0'))
    .join('')}`
}

// ---- WCAG contrast math (sRGB, WCAG 2.x relative luminance) ----

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
