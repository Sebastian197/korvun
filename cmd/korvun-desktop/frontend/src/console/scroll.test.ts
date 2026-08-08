// isNearBottom: the 48px pin threshold that decides whether new content
// keeps the transcript pinned to the bottom (FR-POLISH autoscroll).
import { describe, expect, it } from 'vitest'
import { isNearBottom } from './scroll'

describe('isNearBottom', () => {
  it('is true exactly at the bottom', () => {
    expect(isNearBottom({ scrollHeight: 1000, scrollTop: 600, clientHeight: 400 })).toBe(true)
  })

  it('is true within the 48px threshold', () => {
    expect(isNearBottom({ scrollHeight: 1000, scrollTop: 553, clientHeight: 400 })).toBe(true)
  })

  it('is false once the reader has scrolled up past the threshold', () => {
    expect(isNearBottom({ scrollHeight: 1000, scrollTop: 552, clientHeight: 400 })).toBe(false)
    expect(isNearBottom({ scrollHeight: 1000, scrollTop: 0, clientHeight: 400 })).toBe(false)
  })
})
