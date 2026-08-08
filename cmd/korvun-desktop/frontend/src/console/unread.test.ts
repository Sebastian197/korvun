// FR-UNREAD red: the last-read arithmetic and its persistence.
import { beforeEach, describe, expect, it } from 'vitest'
import type { ConversationRow } from './api'
import { markRead, readLastSeen, STORE_KEY, unreadCount, unreadTotal } from './unread'

const row = (key: string, turns: number): ConversationRow => ({
  key,
  active_session: 1,
  session_count: 1,
  turn_count: turns,
  taken_over: false,
})

beforeEach(() => localStorage.clear())

describe('unread accounting', () => {
  it('counts turns beyond the last-read mark, clamped at zero', () => {
    const seen = { 'tg::a': 3 }
    expect(unreadCount(row('tg::a', 5), seen)).toBe(2)
    expect(unreadCount(row('tg::a', 3), seen)).toBe(0)
    expect(unreadCount(row('tg::a', 2), seen)).toBe(0) // deletion shrank history
  })

  it('a never-opened conversation is fully unread', () => {
    expect(unreadCount(row('tg::new', 4), {})).toBe(4)
  })

  it('markRead persists and readLastSeen round-trips', () => {
    markRead('tg::a', 7)
    expect(readLastSeen()['tg::a']).toBe(7)
    expect(JSON.parse(localStorage.getItem(STORE_KEY) ?? '{}')['tg::a']).toBe(7)
  })

  it('corrupt storage reads as empty, never throws', () => {
    localStorage.setItem(STORE_KEY, '{nope')
    expect(readLastSeen()).toEqual({})
  })

  it('totals across rows', () => {
    markRead('tg::a', 1)
    const rows = [row('tg::a', 3), row('dc::b', 2)]
    expect(unreadTotal(rows, readLastSeen())).toBe(4) // 2 + 2
  })
})
