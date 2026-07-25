// 24h clock, everywhere (6b review rider a): the banner painted "2:57:43 PM"
// while Actividad painted "14:57:42" — one formatter, es-ES, hour12 never.
import { describe, expect, it } from 'vitest'
import { hourES } from './time'

describe('hourES', () => {
  it('formats a local afternoon time in 24h es-ES', () => {
    // Built from LOCAL components so the assertion is timezone-independent.
    const ts = new Date(2026, 6, 25, 15, 7, 9).toISOString()
    expect(hourES(ts)).toBe('15:07:09')
  })

  it('never emits a 12h AM/PM marker', () => {
    const ts = new Date(2026, 6, 25, 14, 57, 43).toISOString()
    expect(hourES(ts)).not.toMatch(/AM|PM|a\.\s?m\.|p\.\s?m\./i)
  })

  it('an unparseable timestamp reads as a dash, never NaN', () => {
    expect(hourES('no-es-fecha')).toBe('—')
  })
})
