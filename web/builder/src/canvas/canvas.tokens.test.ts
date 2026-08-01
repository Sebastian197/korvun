import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// SP3 criterion h (NC-7), stylesheet half: canvas.css must ride the house
// accent TOKEN and never smuggle the violet as a loose hex. Mechanism note
// (change approved 2026-08-01, assertions byte-identical to the red): vitest
// stubs every css IMPORT — even `?raw` — to an empty module, and .tsx test
// files get web-transform URLs, so this scan lives in a .ts file where
// import.meta.url is a real file:// URL and node:fs reads the source directly.
// (.pathname: node's fs rejects the jsdom URL brand, so it gets the resolved
// string — the resolution itself rides import.meta.url, never the cwd.)

describe('h. the violet accent is a TOKEN in canvas.css (NC-7)', () => {
  it('references var(--accent) for the connection affordance and bans loose violet hex', () => {
    // The intermediate variable is load-bearing: vite's asset-import-meta-url
    // plugin statically rewrites the literal `new URL('…', import.meta.url)`
    // pattern into an http asset URL (self.location base) — indirection keeps
    // the genuine file:// module URL while the resolution still rides
    // import.meta.url, never the cwd.
    const here = import.meta.url
    const css = readFileSync(new URL('canvas.css', here).pathname, 'utf-8')
    expect(css).toMatch(/var\(--accent\)/)
    expect(css).not.toMatch(/#8b7cf6|#7a5af5|#a78bfa/i)
  })
})
