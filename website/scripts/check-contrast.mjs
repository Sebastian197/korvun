// WCAG AA contrast over the site's token pairs (design spec AS-8, the
// ADR-0029 §6 "guards that bite" pattern: the token table is asserted, not
// remembered). Stdlib Node only; run from website/.
//
// Reads the site overrides from .vitepress/theme/custom.css (:root and
// .dark blocks) and the theme defaults it pairs against (--vp-c-bg) from
// the INSTALLED vitepress vars.css — so a token drift on either side goes
// red here before it ships.

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const CUSTOM = fileURLToPath(
  new URL('../.vitepress/theme/custom.css', import.meta.url),
)
const VARS = fileURLToPath(
  new URL(
    '../node_modules/vitepress/dist/client/theme-default/styles/vars.css',
    import.meta.url,
  ),
)

const readVars = (css, selector) => {
  // Every `selector { ... }` block's custom properties, later blocks winning
  // (CSS order semantics for equal specificity).
  const vars = {}
  const block = new RegExp(
    `(?:^|\\n)\\s*${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`,
    'g',
  )
  for (const [, body] of css.matchAll(block)) {
    for (const [, name, value] of body.matchAll(
      /(--[\w-]+)\s*:\s*(#[0-9a-fA-F]{6})/g,
    )) {
      vars[name] = value
    }
  }
  return vars
}

const luminance = (hex) => {
  const lin = (c) => {
    const s = c / 255
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  const n = parseInt(hex.slice(1), 16)
  return (
    0.2126 * lin((n >> 16) & 0xff) +
    0.7152 * lin((n >> 8) & 0xff) +
    0.0722 * lin(n & 0xff)
  )
}
const ratio = (a, b) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

let custom
try {
  custom = readFileSync(CUSTOM, 'utf-8')
} catch {
  console.error(`FAIL: ${CUSTOM} does not exist — the site theme tokens are missing`)
  process.exit(1)
}
const varsCss = readFileSync(VARS, 'utf-8')

const themeLight = readVars(varsCss, ':root')
const themeDark = { ...themeLight, ...readVars(varsCss, '.dark') }
const ourLight = readVars(custom, ':root')
const ourDark = { ...ourLight, ...readVars(custom, '.dark') }
const WHITE = '#FFFFFF'

// [description, foreground, background] — normal-text AA floor (≥ 4.5:1).
const pairs = [
  ['light: brand-1 text on page bg', ourLight['--vp-c-brand-1'], themeLight['--vp-c-bg']],
  ['dark: brand-1 text on page bg', ourDark['--vp-c-brand-1'], themeDark['--vp-c-bg']],
  ['light: button text on brand-3 fill', WHITE, ourLight['--vp-c-brand-3']],
  ['dark: button text on brand-3 fill', WHITE, ourDark['--vp-c-brand-3']],
  ['light: button text on brand-2 hover', WHITE, ourLight['--vp-c-brand-2']],
  ['dark: button text on brand-2 hover', WHITE, ourDark['--vp-c-brand-2']],
]

let failed = false
for (const [name, fg, bg] of pairs) {
  if (!fg || !bg) {
    console.error(`FAIL: ${name} — token missing (fg=${fg}, bg=${bg})`)
    failed = true
    continue
  }
  const r = ratio(fg, bg)
  const ok = r >= 4.5
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}: ${fg} on ${bg} = ${r.toFixed(2)}:1`)
  if (!ok) failed = true
}
if (failed) process.exit(1)
console.log('check-contrast: all token pairs ≥ 4.5:1 (AA) — OK')
