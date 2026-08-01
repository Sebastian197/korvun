import { describe, it, expect } from 'vitest'
import { readFileSync, existsSync } from 'node:fs'

// SP4 RED (builder-canvas, the switch's second half): with the canvas as the
// builder's MAIN view, the spike page (FlowSpike, ?spike=flow, its css) and
// the ?spike=canvas harness gate are RETIRED — the view needs no gate. Fixed
// as a source scan (the canvas.tokens.test.ts mechanism: node:fs resolved
// against import.meta.url — vitest gives .ts files genuine file:// URLs; the
// intermediate variable dodges vite's asset-import-meta-url rewrite).
//
// RED today: main.tsx still carries both gates and the spike files exist.

const here = import.meta.url

describe('spike retirement (fixed by the switch tests)', () => {
  it('main.tsx has no spike gates left', () => {
    const main = readFileSync(new URL('main.tsx', here).pathname, 'utf-8')
    expect(main).not.toMatch(/spike/i)
    expect(main).not.toMatch(/FlowSpike|CanvasHarness/)
  })

  it('the SP0 spike files are gone', () => {
    expect(existsSync(new URL('spike/FlowSpike.tsx', here).pathname)).toBe(false)
    expect(existsSync(new URL('spike/flow-spike.css', here).pathname)).toBe(false)
  })

  it('the ?spike=canvas harness wrapper is gone (the view needs no gate)', () => {
    expect(existsSync(new URL('canvas/CanvasHarness.tsx', here).pathname)).toBe(false)
  })
})
