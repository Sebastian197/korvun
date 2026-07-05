import { describe, it, expect } from 'vitest'
import {
  reloadReducer,
  mapServerState,
  pollReload,
  nextInterval,
  POLL,
  type ReloadStatus,
} from './reload'

describe('reloadReducer — verbatim server states, no invented ones', () => {
  const polling: ReloadStatus = { phase: 'polling', handle: 'r1', server: 'pending' }

  it('maps only the exact supervisor.State strings', () => {
    expect(mapServerState('pending')).toBe('pending')
    expect(mapServerState('cutover-in-progress')).toBe('cutover-in-progress')
    expect(mapServerState('succeeded')).toBe('succeeded')
    expect(mapServerState('rolled-back')).toBe('rolled-back')
    expect(mapServerState('failed')).toBe('failed')
  })

  it('an UNKNOWN server string is surfaced as `unknown`, never mapped to a valid state', () => {
    // The bite: a parallel/renamed enum (e.g. "cutover", "preflighting") must NOT
    // silently become a valid phase.
    for (const bad of ['cutover', 'preflighting', 'done', '', 'SUCCEEDED']) {
      const next = reloadReducer(polling, { kind: 'serverState', state: bad })
      expect(next.phase).toBe('unknown')
    }
  })

  it('transitions to the matching terminal phase', () => {
    expect(reloadReducer(polling, { kind: 'serverState', state: 'succeeded' }).phase).toBe('succeeded')
    expect(reloadReducer(polling, { kind: 'serverState', state: 'rolled-back' }).phase).toBe('rolledBack')
    expect(reloadReducer(polling, { kind: 'serverState', state: 'failed' }).phase).toBe('failed')
    expect(reloadReducer(polling, { kind: 'serverState', state: 'cutover-in-progress' })).toEqual({
      phase: 'polling',
      handle: 'r1',
      server: 'cutover-in-progress',
    })
  })

  it('a net error keeps the state unchanged (stay polling), never failed', () => {
    const cutover: ReloadStatus = { phase: 'polling', handle: 'r1', server: 'cutover-in-progress' }
    expect(reloadReducer(cutover, { kind: 'netError' })).toEqual(cutover)
  })

  it('timeout while polling → unknown', () => {
    expect(reloadReducer(polling, { kind: 'timeout' }).phase).toBe('unknown')
  })
})

describe('nextInterval — bounded exponential backoff', () => {
  it('grows from the initial interval and caps at maxMs', () => {
    expect(nextInterval(0)).toBe(POLL.initialMs)
    expect(nextInterval(1)).toBeGreaterThan(POLL.initialMs)
    expect(nextInterval(50)).toBe(POLL.maxMs) // capped
  })
})

describe('pollReload — ECONNREFUSED during cutover is RETRY, not failed', () => {
  it('rides through transient net errors and reaches succeeded', async () => {
    // pending → cutover-in-progress → (net err) → (net err) → succeeded.
    const seq = ['pending', 'cutover-in-progress', '__neterr__', '__neterr__', 'succeeded']
    let i = 0
    const deps = {
      getStatus: async () => {
        const v = seq[i++]
        if (v === '__neterr__') throw new Error('ECONNREFUSED') // the admin server is restarting
        return v
      },
      sleep: async () => {},
      now: () => 0, // never times out
    }
    const seen: ReloadStatus[] = []
    const final = await pollReload('r1', 'tok', deps, (s) => seen.push(s))

    expect(final.phase).toBe('succeeded')
    // It passed through cutover and NEVER went to failed despite the net errors.
    expect(seen.some((s) => s.phase === 'polling' && s.server === 'cutover-in-progress')).toBe(true)
    expect(seen.some((s) => s.phase === 'failed')).toBe(false)
  })

  it('a server-reported failed IS terminal (distinguished from a net error)', async () => {
    const seq = ['cutover-in-progress', 'failed']
    let i = 0
    const deps = { getStatus: async () => seq[i++], sleep: async () => {}, now: () => 0 }
    const final = await pollReload('r1', 'tok', deps, () => {})
    expect(final.phase).toBe('failed')
  })

  it('exhausting the total budget yields unknown', async () => {
    let t = 0
    const deps = {
      getStatus: async () => 'pending',
      sleep: async () => {},
      now: () => (t += POLL.totalMs), // jump past the budget on the first check
    }
    const final = await pollReload('r1', 'tok', deps, () => {})
    expect(final.phase).toBe('unknown')
  })
})
