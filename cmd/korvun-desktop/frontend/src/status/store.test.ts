// The store's classification is the 503 two-body contract of the SP4 proxy —
// pinned here so a body drift breaks the chrome loudly in CI, not silently in
// the window.
import { describe, expect, it } from 'vitest'
import { getLastOkAt, pollOnce, subscribeCore } from './store'

function fakeFetch(status: number, body: unknown): typeof fetch {
  return (() =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    )) as typeof fetch
}

describe('status store classification', () => {
  it('healthz 200 → running', async () => {
    const state = await pollOnce((() => Promise.resolve(new Response('ok'))) as typeof fetch)
    expect(state).toBe('running')
  })

  it('503 core stopped → stopped', async () => {
    expect(await pollOnce(fakeFetch(503, { error: 'core stopped' }))).toBe('stopped')
  })

  it('503 core unreachable → unreachable', async () => {
    expect(await pollOnce(fakeFetch(503, { error: 'core unreachable' }))).toBe('unreachable')
  })

  it('transport failure → unknown', async () => {
    expect(await pollOnce((() => Promise.reject(new Error('down'))) as typeof fetch)).toBe(
      'unknown',
    )
  })

  it('an off-contract 503 body → unknown, never a guessed state', async () => {
    expect(await pollOnce(fakeFetch(503, { error: 'something else' }))).toBe('unknown')
  })

  it('a healthy poll stamps lastOkAt; a 503 leaves it untouched', async () => {
    const before = Date.now()
    await pollOnce((() => Promise.resolve(new Response('ok'))) as typeof fetch)
    const stamped = getLastOkAt()
    expect(stamped).not.toBeNull()
    expect(stamped ?? 0).toBeGreaterThanOrEqual(before)
    await pollOnce(fakeFetch(503, { error: 'core stopped' }))
    expect(getLastOkAt()).toBe(stamped)
  })

  it('every healthy tick notifies listeners ONCE, even without a state change', async () => {
    await pollOnce((() => Promise.resolve(new Response('ok'))) as typeof fetch)
    let fired = 0
    const unsub = subscribeCore(() => fired++)
    await pollOnce((() => Promise.resolve(new Response('ok'))) as typeof fetch)
    unsub()
    expect(fired).toBe(1) // the "hace Xs" tick — exactly one, never double
  })
})
