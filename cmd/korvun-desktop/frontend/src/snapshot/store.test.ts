// The snapshot store mirrors the read-only control API (/api/brains,
// /api/channels) while the core runs, and RETAINS the last session's answer
// when it stops — the stopped Home paints it dimmed ("métricas atenuadas si
// hay último dato de la sesión"), marked stale, never as live data.
import { beforeEach, describe, expect, it } from 'vitest'
import { getSnapshot, pollSnapshotOnce, resetSnapshotForTests } from './store'

const CHANNELS = [{ type: 'telegram', mode: 'polling', name: 'telegram', dropped: 2 }]
const BRAINS = [
  {
    name: 'asistente',
    sensitivity: 'private',
    policy: 'priority',
    dispatch: 'fanout',
    models: [{ provider: 'ollama', model_id: 'llama3.2:1b' }],
  },
]

function okFetch(): typeof fetch {
  return ((url: string) => {
    const body = String(url).includes('brains') ? BRAINS : CHANNELS
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
  }) as unknown as typeof fetch
}

function stoppedFetch(): typeof fetch {
  return (() =>
    Promise.resolve(
      new Response(JSON.stringify({ error: 'core stopped' }), { status: 503 }),
    )) as typeof fetch
}

beforeEach(() => {
  resetSnapshotForTests()
})

describe('snapshot store', () => {
  it('mirrors channels and brains from the control API', async () => {
    await pollSnapshotOnce(okFetch())
    const s = getSnapshot()
    expect(s.channels).toEqual(CHANNELS)
    expect(s.brains).toEqual(BRAINS)
    expect(s.stale).toBe(false)
  })

  it('a 503 keeps the last session data and marks it stale', async () => {
    await pollSnapshotOnce(okFetch())
    await pollSnapshotOnce(stoppedFetch())
    const s = getSnapshot()
    expect(s.channels).toEqual(CHANNELS)
    expect(s.stale).toBe(true)
  })

  it('before any successful poll there is no data (and it is stale)', () => {
    const s = getSnapshot()
    expect(s.channels).toBeNull()
    expect(s.brains).toBeNull()
    expect(s.stale).toBe(true)
  })

  it('the lifecycle follows the core: data goes stale (but survives) on stop', async () => {
    await pollSnapshotOnce(okFetch())
    expect(getSnapshot().stale).toBe(false)
    const { startSnapshot } = await import('./store')
    const { pollOnce } = await import('../status/store')
    startSnapshot()
    await pollOnce((() =>
      Promise.resolve(
        new Response(JSON.stringify({ error: 'core stopped' }), {
          status: 503,
        }),
      )) as typeof fetch)
    const s = getSnapshot()
    expect(s.stale).toBe(true) // reconcile marked it on the transition
    expect(s.channels).toEqual(CHANNELS) // …but the last session's data survives
  })
})
