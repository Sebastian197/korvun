// v0.9.2 RED (N6, bug-bash 2026-08-23): chatting at a brain whose models all
// failed their boot probe was a silent void — the warning must appear when
// the open conversation routes to a brain the core observed dead, and must
// NOT appear for a healthy or merely-unprobed brain (never cry wolf).
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { Console } from './Console'
import { brainForChannel, channelOfKey, hasNoLiveModels } from '../console/health'

function healthFetch(modelHealth: string): typeof fetch {
  return (async (input: RequestInfo | URL) => {
    const url = String(input)
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.includes('/api/config')) {
      return json({
        channels: [{ type: 'console', mode: 'attached', token_env: 'X' }],
        brains: [{ name: 'asistente' }],
        routes: [{ channel: 'console', brain: 'asistente' }],
      })
    }
    if (url.includes('/api/brains')) {
      return json([
        {
          name: 'asistente',
          sensitivity: 'public',
          policy: 'priority',
          dispatch: 'fanout',
          models: [{ provider: 'ollama', model_id: 'qwen3', health: modelHealth }],
        },
      ])
    }
    if (url === '/api/conversations') {
      return json([
        {
          key: 'console::chat-1',
          active_session: 1,
          session_count: 1,
          turn_count: 1,
          last_activity: '2026-08-23T09:00:00Z',
          last_role: 'user',
          taken_over: false,
        },
      ])
    }
    if (url.endsWith('/sessions')) return json([{ id: 1, turn_count: 1 }])
    return json([{ role: 'user', content: 'hola', timestamp: '2026-08-23T09:00:00Z', seq: 0 }])
  }) as typeof fetch
}

beforeEach(() => localStorage.clear())

describe('N6 — the chat warns before typing at a dead brain', () => {
  it('shows the warning when every model of the serving brain is unreachable', async () => {
    render(<Console fetcher={healthFetch('unreachable')} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByText(/chat-1/i))
    const warning = await screen.findByTestId('brain-health-warning')
    expect(warning.textContent).toContain('asistente')
    expect(warning.textContent).toMatch(/no live models/i)
  })

  it('stays silent for a healthy brain', async () => {
    render(<Console fetcher={healthFetch('ready')} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByText(/chat-1/i))
    await screen.findByRole('textbox', { name: /message/i })
    await waitFor(() =>
      expect(screen.queryByTestId('brain-health-warning')).not.toBeInTheDocument(),
    )
  })

  it('stays silent for an unprobed brain — absence of evidence is not death', async () => {
    render(<Console fetcher={healthFetch('unknown')} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByText(/chat-1/i))
    await screen.findByRole('textbox', { name: /message/i })
    await waitFor(() =>
      expect(screen.queryByTestId('brain-health-warning')).not.toBeInTheDocument(),
    )
  })
})

describe('N6 — the pure resolution pieces', () => {
  it('channelOfKey extracts the channel type', () => {
    expect(channelOfKey('console::chat-1')).toBe('console')
    expect(channelOfKey('telegram::123')).toBe('telegram')
    expect(channelOfKey('bare')).toBe('bare')
  })

  it('brainForChannel resolves the route defensively', () => {
    expect(brainForChannel([{ channel: 'console', brain: 'b' }], 'console')).toBe('b')
    expect(brainForChannel([], 'console')).toBeNull()
    expect(brainForChannel('garbage', 'console')).toBeNull()
    expect(brainForChannel([{ channel: 'console', brain: 7 }], 'console')).toBeNull()
  })

  it('hasNoLiveModels demands models AND full unreachability', () => {
    const dead = { models: [{ health: 'unreachable' }, { health: 'unreachable' }] }
    const half = { models: [{ health: 'unreachable' }, { health: 'ready' }] }
    const unprobed = { models: [{ health: 'unknown' }] }
    expect(hasNoLiveModels(dead)).toBe(true)
    expect(hasNoLiveModels(half)).toBe(false)
    expect(hasNoLiveModels(unprobed)).toBe(false)
    expect(hasNoLiveModels({ models: [] })).toBe(false)
    expect(hasNoLiveModels(null)).toBe(false)
  })
})
