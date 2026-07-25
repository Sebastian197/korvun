// The mirror gate (review P2): the app renders EXCLUSIVELY from theme.css's
// variables, so the "1:1 mirror" claim between tokens.ts and theme.css must
// be executable — a typo'd hex in either file fails here, not in the window.
// theme.css is read from disk (a Vite `?raw` import of CSS comes back EMPTY
// under the Tailwind plugin — found in review, do not "simplify" back).
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { themes, type ChromeTheme } from './tokens'

const themeCss = readFileSync(join(__dirname, '..', 'styles', 'theme.css'), 'utf8')

// Map tokens.ts fields to their CSS variable names.
const FIELD_TO_VAR: Record<keyof ChromeTheme, string> = {
  desk: '--desk',
  win: '--win',
  side: '--side',
  card: '--card',
  card2: '--card2',
  line: '--line',
  line2: '--line2',
  tx1: '--tx1',
  tx2: '--tx2',
  tx3: '--tx3',
  capt: '--capt',
  vioT: '--vio-t',
  vio: '--vio',
  onVio: '--on-vio',
  okT: '--ok-t',
  warnT: '--warn-t',
  errT: '--err-t',
  offT: '--off-t',
  navH: '--nav-h',
  navOn: '--nav-on',
}

function cssBlock(theme: 'dark' | 'light'): string {
  const marker = `:root[data-theme='${theme}']`
  const start = themeCss.indexOf(marker)
  expect(start, `missing block for ${theme}`).toBeGreaterThanOrEqual(0)
  const open = themeCss.indexOf('{', start)
  const close = themeCss.indexOf('}', open)
  return themeCss.slice(open, close)
}

describe.each(['dark', 'light'] as const)('theme.css mirrors tokens.ts (%s)', (name) => {
  const block = cssBlock(name)
  const table = themes[name]
  it.each(Object.entries(FIELD_TO_VAR))('%s ↔ %s', (field, cssVar) => {
    const m = block.match(new RegExp(`${cssVar}:\\s*([^;]+);`))
    expect(m, `${cssVar} missing from the ${name} block`).not.toBeNull()
    const cssValue = (m as RegExpMatchArray)[1].trim().toLowerCase()
    expect(cssValue).toBe(table[field as keyof ChromeTheme].toLowerCase())
  })
})
