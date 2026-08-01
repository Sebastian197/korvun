import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// SP5 RED (builder-canvas, console hygiene — the audit §20 / smoke §19
// findings): the builder must load with a CLEAN console. Two source-level
// causes, fixed here as scans (the canvas.tokens.test.ts mechanism: node:fs
// resolved against import.meta.url through an intermediate variable, so vite's
// asset-import-meta-url rewrite doesn't fire and .ts files carry a real
// file:// module URL):
//
//   1. The select chevron rides a `data:image/svg+xml` background — which the
//      real binary's CSP (`default-src 'self'`, no `img-src data:`) BLOCKS
//      (proven in the smoke). The comment in edit.css claiming data: is exempt
//      is WRONG. Green replaces it with pure CSS (or a same-origin asset).
//   2. index.html declares no favicon, so the browser's automatic
//      /favicon.ico request 404s on every load. Green ships and links one.

const here = import.meta.url

describe('c/d. console hygiene — no CSP-blocked data-URI, a served favicon', () => {
  it('edit.css does NOT paint the select chevron with a data: URI', () => {
    const dir = here
    const css = readFileSync(new URL('../edit.css', dir).pathname, 'utf-8')
    // No `url("data:image…")` anywhere — the chevron must be CSS-only or a
    // same-origin asset so `default-src 'self'` never blocks it.
    expect(css).not.toMatch(/url\(\s*["']?data:image/i)
  })

  it('index.html links a favicon so the automatic /favicon.ico never 404s', () => {
    const dir = here
    const html = readFileSync(new URL('../../index.html', dir).pathname, 'utf-8')
    expect(html).toMatch(/<link[^>]+rel=["']icon["']/i)
  })
})