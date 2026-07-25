// The AA gate that bites (ADR-0030 §2 pattern, cloned for the chrome): a
// sub-AA token pair fails CI, memory is not the gate. Text tiers + violet-as-
// text hold 4.5:1 on every content surface. Status tokens are 11 px TEXT
// (pills, the healthz badge), so — SP6b hardening, what axe-core enforces in
// e2e — they hold the AA TEXT floor both on the content surfaces and on
// their own tinted pill surface (tint composited over card). The gradient is
// identity-only and NOT color-checked (it is never a text color — the
// palette scan enforces where it appears).
import { describe, expect, it } from 'vitest'
import { AA_TEXT, composite, contrast, themes, tints } from './tokens'

const SURFACES = ['win', 'side', 'card', 'card2'] as const
const TEXT_TIERS = ['tx1', 'tx2', 'tx3', 'capt', 'vioT'] as const
const STATUS: ReadonlyArray<
  readonly ['okT' | 'warnT' | 'errT' | 'offT' | 'vioT', 'ok' | 'warn' | 'err' | 'off' | 'vio']
> = [
  ['okT', 'ok'],
  ['warnT', 'warn'],
  ['errT', 'err'],
  ['offT', 'off'],
  ['vioT', 'vio'],
]

describe.each(Object.entries(themes))('theme %s', (name, t) => {
  it.each(TEXT_TIERS)('text token %s passes AA on every surface', (txt) => {
    for (const s of SURFACES) {
      expect(contrast(t[txt], t[s]), `${txt} on ${s}`).toBeGreaterThanOrEqual(AA_TEXT)
    }
  })

  it.each(STATUS)('status text %s passes AA on surfaces AND its %s pill', (txt, tint) => {
    for (const s of SURFACES) {
      expect(contrast(t[txt], t[s]), `${txt} on ${s}`).toBeGreaterThanOrEqual(AA_TEXT)
    }
    const pill = composite(tints[name as 'dark' | 'light'][tint], t.card)
    expect(contrast(t[txt], pill), `${txt} on its ${tint} pill (${pill})`).toBeGreaterThanOrEqual(
      AA_TEXT,
    )
  })

  it('the primary action pair (onVio on vio) passes AA', () => {
    expect(contrast(t.onVio, t.vio)).toBeGreaterThanOrEqual(AA_TEXT)
  })
})
