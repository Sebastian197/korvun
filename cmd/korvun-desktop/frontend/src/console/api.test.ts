// SP4 red (operator-console spec): the console API client — relative proxy
// paths (the bearer is the proxy's business, NEVER this renderer's), exact
// methods and bodies, honest error mapping.
import { describe, expect, it, vi } from 'vitest'
import {
  conversationDetail,
  listConversations,
  listSessions,
  newSession,
  sendReply,
  sessionDetail,
  setTakeover,
} from './api'

interface Captured {
  url: string
  method: string
  body: string | null
  headers: Headers
}

function fakeFetch(
  status: number,
  json: unknown,
): { fetcher: typeof fetch; calls: Captured[] } {
  const calls: Captured[] = []
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({
      url: String(input),
      method: init?.method ?? 'GET',
      body: typeof init?.body === 'string' ? init.body : null,
      headers: new Headers(init?.headers),
    })
    return new Response(JSON.stringify(json), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  }) as typeof fetch
  return { fetcher, calls }
}

describe('console api client', () => {
  it('lists conversations from the proxy path without any auth header', async () => {
    const { fetcher, calls } = fakeFetch(200, [
      { key: 'telegram::c', active_session: 2, session_count: 2, taken_over: false },
    ])
    const rows = await listConversations(fetcher)
    expect(rows).toHaveLength(1)
    expect(rows[0].key).toBe('telegram::c')
    expect(calls[0].url).toBe('/api/conversations')
    expect(calls[0].method).toBe('GET')
    expect(calls[0].headers.get('Authorization')).toBeNull()
  })

  it('reads detail, sessions and archived-session turns from their paths', async () => {
    const { fetcher, calls } = fakeFetch(200, [])
    await conversationDetail('telegram::c', fetcher)
    await listSessions('telegram::c', fetcher)
    await sessionDetail('telegram::c', 1, fetcher)
    expect(calls.map((c) => c.url)).toEqual([
      '/api/conversations/telegram%3A%3Ac',
      '/api/conversations/telegram%3A%3Ac/sessions',
      '/api/conversations/telegram%3A%3Ac/sessions/1',
    ])
  })

  it('sends a reply as POST {text} and reads 202 as accepted', async () => {
    const { fetcher, calls } = fakeFetch(202, {})
    const out = await sendReply('telegram::c', 'aquí Chano', fetcher)
    expect(out.ok).toBe(true)
    expect(calls[0].method).toBe('POST')
    expect(calls[0].url).toBe('/api/conversations/telegram%3A%3Ac/reply')
    expect(JSON.parse(calls[0].body ?? '{}')).toEqual({ text: 'aquí Chano' })
  })

  it('maps reply failures honestly: 409 channel, 503 saturated, 400 invalid, 500 failed', async () => {
    for (const [status, reason] of [
      [409, 'channel-missing'],
      [503, 'saturated'],
      [400, 'invalid'],
      [500, 'failed'],
    ] as const) {
      const { fetcher } = fakeFetch(status, { error: 'x' })
      const out = await sendReply('telegram::c', 'x', fetcher)
      expect(out).toEqual({ ok: false, reason })
    }
  })

  it('flips takeover and release via their POST endpoints', async () => {
    const { fetcher, calls } = fakeFetch(204, {})
    await setTakeover('telegram::c', true, fetcher)
    await setTakeover('telegram::c', false, fetcher)
    expect(calls.map((c) => `${c.method} ${c.url}`)).toEqual([
      'POST /api/conversations/telegram%3A%3Ac/takeover',
      'POST /api/conversations/telegram%3A%3Ac/release',
    ])
  })

  it('opens a new session via POST and returns the id', async () => {
    const { fetcher, calls } = fakeFetch(200, { session: 3 })
    const id = await newSession('telegram::c', fetcher)
    expect(id).toBe(3)
    expect(calls[0].method).toBe('POST')
    expect(calls[0].url).toBe('/api/conversations/telegram%3A%3Ac/sessions')
  })

  it('a network failure never throws out of the client', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('down')) as unknown as typeof fetch
    const rows = await listConversations(fetcher)
    expect(rows).toEqual([])
    const out = await sendReply('telegram::c', 'x', fetcher)
    expect(out).toEqual({ ok: false, reason: 'failed' })
  })
})
