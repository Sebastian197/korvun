// Fidelity scans (SP6 spec FR-WIN-8), cheap and biting:
//  1. NO hardcoded color (hex/rgb/hsl) anywhere in src/ outside the token
//     table — colors live in tokens (and their theme.css mirror), nowhere
//     else.
//  2. The identity GRADIENT appears at most ONCE per view source — the
//     design law "one gradient primary action per view", executable.
// Sources are read from DISK (a Vite `?raw` import of CSS comes back EMPTY
// under the Tailwind plugin — found in review), so CSS is genuinely scanned.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, sep } from 'node:path'
import { describe, expect, it } from 'vitest'

const SRC = join(__dirname, '..')
const TOKEN_FILES = ['tokens.ts', 'theme.css']

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
  return p.slice(SRC.length + 1).split(sep).join('/')
}

const sources = walk(SRC)
  .map((p) => [rel(p), readFileSync(p, 'utf8')] as const)
  .filter(
    ([name]) =>
      !name.includes('.test.') && !name.includes('.spec.') && name !== 'test.setup.ts',
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

  it('the identity gradient appears at most once per view source', () => {
    for (const [name, body] of sources) {
      if (isTokenFile(name)) continue
      const count = (body.match(/--grad|IDENTITY_GRADIENT/g) ?? []).length
      expect(count, `${name} uses the gradient ${count} times`).toBeLessThanOrEqual(1)
    }
  })
})
