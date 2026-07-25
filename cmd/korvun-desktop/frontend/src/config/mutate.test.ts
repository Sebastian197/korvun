// The config-mutation pipe (SP6c): GET the current config through the SP4
// proxy (bearer injected server-side — no token here), apply a pure
// transform, POST it, then poll the reload handle to a terminal state,
// tolerating the transient network error of the admin server cycling
// mid-cutover (the SP5-proven pipe, F4). No secret ever crosses this path:
// the config carries only env-var NAMES.
import { describe, expect, it } from 'vitest'
import { addChannel, fetchConfig, mutateConfig, removeChannel, type CoreConfig } from './mutate'

const BASE: CoreConfig = {
  channels: [{ type: 'telegram', mode: 'polling', token_env: 'TELEGRAM_TOKEN' }],
  brains: [{ name: 'asistente' }],
  routes: [{ channel: 'telegram', brain: 'asistente' }],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}

// A scripted fetch: GET config, POST config (captures the body), poll reload.
function scriptedFetch(opts: {
  pollStates: string[]
  onPost?: (cfg: CoreConfig) => void
}): typeof fetch {
  let poll = 0
  return ((url: string, init?: RequestInit) => {
    const u = String(url)
    if (u === '/api/config' && (init?.method ?? 'GET') === 'GET') {
      return Promise.resolve(new Response(JSON.stringify(BASE), { status: 200 }))
    }
    if (u === '/api/config' && init?.method === 'POST') {
      opts.onPost?.(JSON.parse(String(init.body)) as CoreConfig)
      return Promise.resolve(new Response(JSON.stringify({ handle: 'h1' }), { status: 202 }))
    }
    if (u.startsWith('/api/reload/')) {
      const state = opts.pollStates[Math.min(poll, opts.pollStates.length - 1)]
      poll++
      return Promise.resolve(new Response(JSON.stringify({ state }), { status: 200 }))
    }
    return Promise.resolve(new Response('not found', { status: 404 }))
  }) as unknown as typeof fetch
}

describe('addChannel / removeChannel transforms', () => {
  it('addChannel appends a channel and its route, pure', () => {
    const next = addChannel(BASE, {
      type: 'discord',
      mode: 'gateway',
      token_env: 'DISCORD_BOT_TOKEN',
    })
    expect(next.channels).toHaveLength(2)
    expect(next.channels[1]?.type).toBe('discord')
    expect(next.routes).toContainEqual({ channel: 'discord', brain: 'asistente' })
    expect(BASE.channels).toHaveLength(1) // original untouched
  })

  it('removeChannel drops the channel and its dangling routes', () => {
    const next = removeChannel(BASE, 'telegram')
    expect(next.channels).toHaveLength(0)
    expect(next.routes).toHaveLength(0)
    expect(BASE.channels).toHaveLength(1)
  })

  it('addChannel with no brains appends a route-less channel (inert, valid to POST)', () => {
    const next = addChannel(
      { ...BASE, brains: [], routes: [] },
      {
        type: 'discord',
        mode: 'gateway',
        token_env: 'DISCORD_BOT_TOKEN',
      },
    )
    expect(next.channels).toHaveLength(2)
    expect(next.routes).toHaveLength(0) // no brain to wire to
  })
})

describe('mutateConfig pipe', () => {
  it('GET → transform → POST → poll to succeeded', async () => {
    let posted: CoreConfig | null = null
    const fetcher = scriptedFetch({
      pollStates: ['pending', 'cutover-in-progress', 'succeeded'],
      onPost: (c) => (posted = c),
    })
    const res = await mutateConfig(
      (c) =>
        addChannel(c, {
          type: 'discord',
          mode: 'gateway',
          token_env: 'DISCORD_BOT_TOKEN',
        }),
      { fetcher, pollIntervalMs: 0 },
    )
    expect(res.ok).toBe(true)
    expect(posted).not.toBeNull()
    expect((posted as unknown as CoreConfig).channels).toHaveLength(2)
  })

  it('a failed reload resolves ok:false with the terminal state', async () => {
    const fetcher = scriptedFetch({ pollStates: ['failed'] })
    const res = await mutateConfig((c) => c, { fetcher, pollIntervalMs: 0 })
    expect(res.ok).toBe(false)
    expect(res.state).toBe('failed')
  })

  it('a transient reload-poll network error is retried, not fatal (F4)', async () => {
    let poll = 0
    const fetcher = ((url: string, init?: RequestInit) => {
      const u = String(url)
      if (u === '/api/config' && (init?.method ?? 'GET') === 'GET') {
        return Promise.resolve(new Response(JSON.stringify(BASE), { status: 200 }))
      }
      if (u === '/api/config') {
        return Promise.resolve(new Response(JSON.stringify({ handle: 'h1' }), { status: 202 }))
      }
      poll++
      if (poll === 1) return Promise.reject(new Error('ECONNREFUSED')) // admin cycling
      return Promise.resolve(new Response(JSON.stringify({ state: 'succeeded' }), { status: 200 }))
    }) as unknown as typeof fetch
    const res = await mutateConfig((c) => c, { fetcher, pollIntervalMs: 0 })
    expect(res.ok).toBe(true)
  })

  it('a non-202 POST surfaces the error, no polling', async () => {
    const fetcher = ((url: string, init?: RequestInit) => {
      if (String(url) === '/api/config' && (init?.method ?? 'GET') === 'GET') {
        return Promise.resolve(new Response(JSON.stringify(BASE), { status: 200 }))
      }
      return Promise.resolve(new Response(JSON.stringify({ error: 'invalid' }), { status: 400 }))
    }) as unknown as typeof fetch
    const res = await mutateConfig((c) => c, { fetcher, pollIntervalMs: 0 })
    expect(res.ok).toBe(false)
    expect(res.state).toBe('post-failed')
  })

  it('gives up after the poll budget instead of looping forever', async () => {
    const fetcher = scriptedFetch({ pollStates: ['pending'] }) // never terminal
    const res = await mutateConfig((c) => c, {
      fetcher,
      pollIntervalMs: 0,
      maxPolls: 3,
    })
    expect(res.ok).toBe(false)
    expect(res.state).toBe('timeout')
  })

  it('a rolled-back reload resolves ok:false with that terminal state', async () => {
    const fetcher = scriptedFetch({ pollStates: ['pending', 'rolled-back'] })
    const res = await mutateConfig((c) => c, { fetcher, pollIntervalMs: 0 })
    expect(res.ok).toBe(false)
    expect(res.state).toBe('rolled-back')
  })

  it('a 404 on the reload handle short-circuits instead of burning the budget', async () => {
    const fetcher = ((url: string, init?: RequestInit) => {
      const u = String(url)
      if (u === '/api/config' && (init?.method ?? 'GET') === 'GET') {
        return Promise.resolve(new Response(JSON.stringify(BASE), { status: 200 }))
      }
      if (u === '/api/config') {
        return Promise.resolve(new Response(JSON.stringify({ handle: 'h1' }), { status: 202 }))
      }
      return Promise.resolve(new Response('unknown handle', { status: 404 }))
    }) as unknown as typeof fetch
    const res = await mutateConfig((c) => c, { fetcher, pollIntervalMs: 0, maxPolls: 60 })
    expect(res.ok).toBe(false)
    expect(res.state).toBe('unknown-handle')
  })

  it('a GET failure short-circuits before any POST', async () => {
    const fetcher = (() => Promise.resolve(new Response('nope', { status: 503 }))) as typeof fetch
    const res = await mutateConfig((c) => c, { fetcher, pollIntervalMs: 0 })
    expect(res.ok).toBe(false)
    expect(res.state).toBe('get-failed')
  })
})

describe('fetchConfig', () => {
  it('returns the config on 200', async () => {
    const fetcher = (() =>
      Promise.resolve(new Response(JSON.stringify(BASE), { status: 200 }))) as typeof fetch
    expect(await fetchConfig(fetcher)).toEqual(BASE)
  })

  it('returns null on a non-ok response', async () => {
    const fetcher = (() =>
      Promise.resolve(new Response('stopped', { status: 503 }))) as typeof fetch
    expect(await fetchConfig(fetcher)).toBeNull()
  })

  it('returns null on a transport error, never throws', async () => {
    const fetcher = (() => Promise.reject(new Error('down'))) as typeof fetch
    expect(await fetchConfig(fetcher)).toBeNull()
  })
})
