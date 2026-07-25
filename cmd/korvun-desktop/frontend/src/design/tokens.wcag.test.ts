// The AA gate that bites (ADR-0030 §2 pattern, cloned for the chrome): a
// sub-AA token pair fails CI, memory is not the gate. Text tiers + violet-as-
// text hold 4.5:1 on every content surface; status pill tints hold the 3:1
// UI floor; the gradient is identity-only and NOT color-checked (it is never
// a text color — the palette scan enforces where it appears).
import { describe, expect, it } from 'vitest'
import { AA_TEXT, UI_MIN, contrast, themes } from './tokens'

const SURFACES = ['win', 'side', 'card', 'card2'] as const
const TEXT_TIERS = ['tx1', 'tx2', 'tx3', 'capt', 'vioT'] as const
const STATUS_TINTS = ['okT', 'warnT', 'errT', 'offT'] as const

describe.each(Object.entries(themes))('theme %s', (_name, t) => {
  it.each(TEXT_TIERS)('text token %s passes AA on every surface', (txt) => {
    for (const s of SURFACES) {
      expect(contrast(t[txt], t[s]), `${txt} on ${s}`).toBeGreaterThanOrEqual(AA_TEXT)
    }
  })

  it.each(STATUS_TINTS)('status tint %s passes the UI floor on every surface', (tint) => {
    for (const s of SURFACES) {
      expect(contrast(t[tint], t[s]), `${tint} on ${s}`).toBeGreaterThanOrEqual(UI_MIN)
    }
  })

  it('the primary action pair (onVio on vio) passes AA', () => {
    expect(contrast(t.onVio, t.vio)).toBeGreaterThanOrEqual(AA_TEXT)
  })
})
