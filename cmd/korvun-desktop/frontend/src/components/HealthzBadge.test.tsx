// v0.16.2 · «¿Qué pasa hoy?» — cure 3: THE INDICATOR, measured then redesigned.
//
// RED. What the measurement found, before touching anything:
//
//   · the store polls every POLL_INTERVAL_MS = 2000 ms;
//   · the counter DOES advance — `pollOnce` rewrites `lastOkAt` on every OK and
//     notifies even when the state did not change, with a comment saying why —
//     so «hace X s» is not a code fault, it is a label that can only ever read
//     0, 1 or 2 while everything works. True, and carrying no information;
//   · nothing measures the round trip. `pollOnce` never times its `fetch`;
//   · `lastOkAt` is NEVER cleared in the store. It is `HealthzBadge` that stops
//     showing it — so when the core goes quiet the operator loses the one datum
//     that matters then: when it last worked.
//
// These three moulds pin the redesign the director approved: «en vivo · HH:MM:SS
// · N ms» while healthy, the last hour KEPT when the answer is lost, and no
// «en vivo» over a poll that is frozen.
//
// Evidence level, honest: in-process, jsdom, with an injected fetcher and a
// forced clock. Nothing here proves a real core or a real network.
//
// Plan: docs/superpowers/specs/2026-09-23-v0162-que-pasa-hoy-pretest.md,
// guarantee G7, attacks A12, A13 and A13-bis.
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { POLL_STALE_AFTER_MS, pollOnce, resetCoreForTests } from '../status/store'
import { HealthzBadge } from './HealthzBadge'

/** An answer that is OK after `delayMs` of measurable round trip. */
function slowOk(delayMs: number): typeof fetch {
  return (() =>
    new Promise((resolve) => {
      vi.advanceTimersByTime(delayMs)
      resolve(new Response('{}', { status: 200 }))
    })) as unknown as typeof fetch
}

const stopped: typeof fetch = (() =>
  Promise.resolve(
    new Response(JSON.stringify({ error: 'core stopped' }), { status: 503 }),
  )) as unknown as typeof fetch

/** A request that never resolves: the wedged proxy `pollOnce` guards against
 * with `if (polling) return current`. */
const neverResolves: typeof fetch = (() => new Promise(() => {})) as unknown as typeof fetch

// The instant the clock is frozen at, and its LOCAL 24h rendering. The caption
// shows wall-clock time, which is what an operator reads off their own machine,
// so the expectation is derived rather than hard-coded: a test that pinned the
// UTC string would pass only in one timezone, and it failed here on the first
// run for exactly that reason.
const FROZEN = new Date('2026-09-24T18:30:04Z')
const LOCAL_HOUR = FROZEN.toLocaleTimeString('es-ES', { hour12: false })

describe('HealthzBadge', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(FROZEN)
    resetCoreForTests()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  // A13 · the round trip is MEASURED, not invented. The forced clock advances
  // by a known amount inside the fetch, and the badge must show that amount.
  //
  // PROBING MUTATION: show a constant instead of the measurement. The badge
  // then prints the same number whatever the clock does, and this reddens
  // while its siblings stay green.
  it('shows the hour of the last answer and the measured round trip', async () => {
    await pollOnce(slowOk(7))
    render(<HealthzBadge />)
    const text = screen.getByTestId('healthz-badge').textContent ?? ''

    expect(text).toContain('en vivo')
    // The HOUR, not a countdown: «hace X s» could only ever read 0, 1 or 2.
    expect(text).toMatch(/\d{2}:\d{2}:\d{2}/)
    expect(text).toContain('7 ms')
    expect(text).not.toContain('hace')
  })

  // A12 · losing the answer must not lose the hour. The store never clears
  // `lastOkAt`; today the badge stops reading it, and that is the branch this
  // mould watches.
  //
  // PROBING MUTATION: return the badge to `'/healthz · sin respuesta'` without
  // the hour — which is exactly what it prints today. The mutation lives in the
  // LABEL, not in the store: putting it in the store would redden this for a
  // reason that has nothing to do with the guarantee.
  it('keeps the last good hour when the core stops answering', async () => {
    await pollOnce(slowOk(4))
    await pollOnce(stopped)
    render(<HealthzBadge />)
    const text = screen.getByTestId('healthz-badge').textContent ?? ''

    expect(text).toContain('sin respuesta')
    expect(text).toContain(LOCAL_HOUR)
    // And it must not go on claiming to be live over a core that is not.
    expect(text).not.toContain('en vivo')
  })

  // A13-bis · a poll that never resolves. `pollOnce` opens with
  // `if (polling) return current`, so every later tick is skipped and the
  // measurement goes stale. A badge that keeps saying «en vivo» over a frozen
  // poll is lying about the one thing it exists to report.
  //
  // PROBING MUTATION (executed): leave «en vivo» while the poll is wedged. This
  // reddens and the other two stay green.
  it('does not claim to be live while the poll is wedged', async () => {
    await pollOnce(slowOk(3))
    void pollOnce(neverResolves)
    vi.advanceTimersByTime(POLL_STALE_AFTER_MS + 1000)
    render(<HealthzBadge />)
    const text = screen.getByTestId('healthz-badge').textContent ?? ''

    expect(text).not.toContain('en vivo')
    expect(text).toContain('sin confirmar')
    // The last good hour survives here too: it is the only fact still true.
    expect(text).toContain(LOCAL_HOUR)
  })

  // A13-ter · THE MOUNT ORDER OF THE REAL APP. The mould above renders AFTER
  // the poll has already wedged, so it only ever observed a first render — and
  // the internal pass proved that is not the shape the app has. `HealthzBadge`
  // mounts once, while the core is healthy, and stays mounted; `isPollStale()`
  // read during that render answers "no", and under a wedged poll NOTHING
  // notifies afterwards, because `pollOnce` returns early and no state changes.
  // The badge measured «en vivo · 19:30:04 · 3 ms» unchanged a full minute past
  // the window.
  //
  // So this one mounts FIRST, healthy, then wedges the poll and lets the clock
  // run. The badge must change on its own.
  //
  // PROBING MUTATION (executed): remove the `armStaleWatchdog()` call from
  // pollOnce. The badge then keeps printing «en vivo» for as long as the clock
  // is advanced, and this reddens while the three moulds above stay green —
  // which is exactly how the defect survived the first round.
  it('stops claiming to be live on its own when the poll wedges under it', async () => {
    await pollOnce(slowOk(3))
    render(<HealthzBadge />)
    // The starting point: healthy, and saying so.
    expect(screen.getByTestId('healthz-badge').textContent ?? '').toContain('en vivo')

    await act(async () => {
      void pollOnce(neverResolves)
      vi.advanceTimersByTime(POLL_STALE_AFTER_MS + 60_000)
    })

    const text = screen.getByTestId('healthz-badge').textContent ?? ''
    expect(text).not.toContain('en vivo')
    expect(text).toContain('sin confirmar')
    expect(text).toContain(LOCAL_HOUR)
  })
})
