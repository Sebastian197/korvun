// FR-POLISH red: relative timestamps.
import { describe, expect, it } from 'vitest'
import { relTime } from './time'

const NOW = Date.parse('2026-08-08T12:00:00Z')

describe('relTime', () => {
  it('renders the honest ladder', () => {
    expect(relTime('2026-08-08T11:59:30Z', NOW)).toBe('just now')
    expect(relTime('2026-08-08T11:58:00Z', NOW)).toBe('2m ago')
    expect(relTime('2026-08-08T09:00:00Z', NOW)).toBe('3h ago')
    expect(relTime('2026-08-06T12:00:00Z', NOW)).toBe('2d ago')
  })
  it('old dates fall back to the date; junk renders empty', () => {
    expect(relTime('2026-06-01T12:00:00Z', NOW)).toMatch(/2026/)
    expect(relTime(undefined, NOW)).toBe('')
    expect(relTime('nope', NOW)).toBe('')
  })
})
