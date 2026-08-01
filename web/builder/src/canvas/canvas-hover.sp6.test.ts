import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// SP6 RED (builder-canvas, f): the connection handles get a violet hover halo
// — the affordance the final-6 mockup shows. Fixed as a token scan (the
// canvas.tokens mechanism: node:fs against import.meta.url through an
// intermediate variable so vite's asset-import-meta-url rewrite doesn't fire).
// RED today: canvas.css has no :hover rule on the handles.

const here = import.meta.url

describe('f. handles get a violet hover halo (token, never a loose hex)', () => {
  it('canvas.css styles .react-flow__handle:hover with var(--accent) and no violet hex', () => {
    const css = readFileSync(new URL('canvas.css', here).pathname, 'utf-8')
    // A :hover rule targeting the handle must exist and reference the accent
    // token (the halo). Match a handle+hover selector followed (in its block)
    // by var(--accent).
    const hasHandleHover = /\.react-flow__handle[^{]*:hover[^{]*\{[^}]*var\(--accent\)/s.test(css)
    expect(hasHandleHover, 'a .react-flow__handle:hover rule using var(--accent) is required').toBe(true)
    // No loose violet hex anywhere in the stylesheet (NC-7).
    expect(css).not.toMatch(/#8b7cf6|#7a5af5|#a78bfa/i)
  })
})