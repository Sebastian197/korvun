// Fidelity scans (SP6 spec FR-WIN-8), cheap and biting:
//  1. NO hardcoded color (hex/rgb/hsl) anywhere in src/ outside the token
//     table and the brand source — colors live in tokens (their theme.css
//     mirror) and in the canonical BrandMark copy, nowhere else.
//  2. The identity GRADIENT law, PER VIEW (6a review rider d — the test says
//     what the law says): each view source (src/views/*.tsx, and App.tsx
//     while it still hosts a view) may carry at most ONE gradient-bearing
//     marker (`btn-primary` / `--grad` / `IDENTITY_GRADIENT`); every other
//     non-allowlisted file carries NONE, and in CSS `--grad` may be
//     REFERENCED only by the sanctioned utility selectors below. The brand
//     tile is the identity moment and lives inside BrandMark (allowlisted).
// Sources are read from DISK (a Vite `?raw` import of CSS comes back EMPTY
// under the Tailwind plugin — found in review), so CSS is genuinely scanned.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, sep } from 'node:path'
import { describe, expect, it } from 'vitest'

const SRC = join(__dirname, '..')
// tokens.ts + theme.css ARE the token table; BrandMark.tsx is the byte-faithful
// copy of assets/brand/korvun-logo-hero.svg (the brand source of truth).
const TOKEN_FILES = ['tokens.ts', 'theme.css', 'components/BrandMark.tsx']
// The only CSS selectors allowed to reference the identity gradient.
const GRADIENT_UTILITIES = ['.btn-primary']

function walk(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...walk(p))
    else if (/\.(ts|tsx|css)$/.test(p)) out.push(p)
  }
  return out
}

function rel(p: string): string {
  return p
    .slice(SRC.length + 1)
    .split(sep)
    .join('/')
}

const sources = walk(SRC)
  .map((p) => [rel(p), readFileSync(p, 'utf8')] as const)
  .filter(
    ([name]) => !name.includes('.test.') && !name.includes('.spec.') && name !== 'test.setup.ts',
  )

function isTokenFile(name: string): boolean {
  return TOKEN_FILES.some((f) => name.endsWith(f))
}

describe('palette fidelity', () => {
  it('scans real file contents (the CSS is not empty)', () => {
    const appCss = sources.find(([name]) => name === 'App.css')
    expect(appCss, 'App.css missing from the scan set').toBeDefined()
    expect((appCss as readonly [string, string])[1].length).toBeGreaterThan(100)
  })

  it('no hardcoded colors (hex/rgb/hsl) outside the token table', () => {
    const offenders: string[] = []
    for (const [name, body] of sources) {
      if (isTokenFile(name)) continue
      for (const hit of body.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/g) ?? []) {
        offenders.push(`${name}: ${hit}`)
      }
    }
    expect(offenders, offenders.join('\n')).toEqual([])
  })

  // Multi-step overlay FLOWS (the channel wizard, the onboarding): the
  // design (final-4, chica-18/19/20, recién-instalado) applies the identity
  // gradient to EACH step's single primary action, and only one step renders
  // at a time — so a static per-file count over-counts a law the runtime
  // honors ("one gradient primary VISIBLE per screen"). They are exempt from
  // the per-view cap; the 5 dashboard views + App.tsx still carry at most one.
  const FLOW_FILES = ['views/ChannelWizard.tsx', 'views/Onboarding.tsx']

  it('the identity gradient appears at most once PER VIEW, nowhere else', () => {
    for (const [name, body] of sources) {
      if (isTokenFile(name)) continue
      if (name.endsWith('.css')) continue // CSS is held to the selector law below
      if (FLOW_FILES.includes(name)) continue // multi-step overlay, one primary per step
      // `btn-primary(?!-)` counts the gradient BASE class only: `btn-primary-sm`
      // is a size modifier that reuses the base's gradient (CSS: only
      // `.btn-primary` sets var(--grad)), so `className="btn-primary btn-primary-sm"`
      // is ONE gradient action, not two.
      const count = (body.match(/btn-primary(?!-)|--grad|IDENTITY_GRADIENT/g) ?? []).length
      const isView = name.startsWith('views/') || name === 'App.tsx'
      const max = isView ? 1 : 0
      expect(
        count,
        `${name} uses the gradient ${count} times (allowed ${max})`,
      ).toBeLessThanOrEqual(max)
    }
  })

  it('in CSS, --grad is referenced only by the sanctioned utility selectors', () => {
    for (const [name, body] of sources) {
      if (isTokenFile(name) || !name.endsWith('.css')) continue
      // Split into rule blocks and attribute every var(--grad) use to its selector.
      for (const block of body.split('}')) {
        if (!block.includes('var(--grad)')) continue
        const selector = (block.split('{')[0] ?? '').trim()
        expect(
          GRADIENT_UTILITIES.some((u) => selector === u),
          `${name}: selector "${selector}" uses var(--grad) — only ${GRADIENT_UTILITIES.join(', ')} may`,
        ).toBe(true)
      }
    }
  })
})
