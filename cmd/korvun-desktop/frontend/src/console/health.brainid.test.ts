import { describe, expect, it } from 'vitest'
import { deadBrainForConversation } from './health'

// B9 FR-B9-5 — N6 coherence: a brain-addressed conversation resolves its
// serving brain from the ID first (route fallback only for legacy ids).
// Fixture: the route points at healthy "asistente"; the id names dead
// "openrouter" — the warning must follow the id.

function fakeFetch(): typeof fetch {
  return (async (input: RequestInfo | URL) => {
    const url = String(input)
    const json = (v: unknown) =>
      new Response(JSON.stringify(v), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    if (url.includes('/api/config')) {
      return json({ routes: [{ channel: 'console', brain: 'asistente' }] })
    }
    return json([
      { name: 'asistente', models: [{ health: 'ready' }] },
      { name: 'openrouter', models: [{ health: 'unreachable' }] },
    ])
  }) as typeof fetch
}

describe('deadBrainForConversation honors the id-addressed brain', () => {
  it('warns for the DEAD id-named brain even when the route brain is healthy', async () => {
    const got = await deadBrainForConversation('console::b:openrouter:chat-1', fakeFetch())
    expect(got).toBe('openrouter')
  })

  it('a legacy id keeps resolving via the route (healthy → no warning)', async () => {
    const got = await deadBrainForConversation('console::chat-1', fakeFetch())
    expect(got).toBeNull()
  })
})
