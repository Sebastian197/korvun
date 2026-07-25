// The incident store encodes FR-WIN-4's HONEST triggers, nothing more:
//  - the core left 'running' without a UI-initiated Stop → "stopped
//    unexpectedly" (reap-shaped; no invented cause), and
//  - a failure frame (message_dropped / handle_failed) with its REAL channel.
// A clean Start clears it (AS-6).
import { beforeEach, describe, expect, it } from 'vitest'
import {
  getIncident,
  markUserStop,
  notifyCoreTransition,
  notifyFailureFrame,
  resetIncidentForTests,
} from './store'

beforeEach(() => {
  resetIncidentForTests()
})

describe('incident store', () => {
  it('running → stopped WITHOUT a UI stop reads as an unexpected exit', () => {
    notifyCoreTransition('running', 'stopped')
    expect(getIncident()).toMatchObject({ kind: 'reap' })
  })

  it('running → stopped AFTER the UI asked for it is not an incident', () => {
    markUserStop()
    notifyCoreTransition('running', 'stopped')
    expect(getIncident()).toBeNull()
  })

  it('a failure frame becomes an incident carrying its real channel', () => {
    notifyFailureFrame({
      type: 'handle_failed',
      channel: 'telegram',
      timestamp: '2026-07-25T10:00:00Z',
    })
    expect(getIncident()).toMatchObject({
      kind: 'feed',
      frameType: 'handle_failed',
      channel: 'telegram',
    })
  })

  it('a clean Start clears the incident (AS-6 recovery)', () => {
    notifyCoreTransition('running', 'stopped')
    expect(getIncident()).not.toBeNull()
    notifyCoreTransition('stopped', 'running')
    expect(getIncident()).toBeNull()
  })

  it('stopped → stopped transitions never fabricate an incident', () => {
    notifyCoreTransition('unknown', 'stopped')
    notifyCoreTransition('stopped', 'unreachable')
    expect(getIncident()).toBeNull()
  })

  it('a transient blip (running → unknown/unreachable) is NOT a reap', () => {
    // 'unknown' is transport noise and 'unreachable' is a mid-cutover state;
    // only the definitive 503 'core stopped' reading may raise the red
    // banner — a false "se detuvo inesperadamente" would invent a cause.
    notifyCoreTransition('running', 'unknown')
    expect(getIncident()).toBeNull()
    notifyCoreTransition('running', 'unreachable')
    expect(getIncident()).toBeNull()
  })
})
