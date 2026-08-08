// FR-CONS-4 red: the direct chat — New chat drafts, user-voiced sends with
// the honest "Thinking…" latency, and takeover disabled with its reason.
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { Console } from './Console'

type Captured = { url: string; method: string; body: string | null }

function chatFetch(): { fetcher: typeof fetch; calls: Captured[] } {
  const calls: Captured[] = []
  let turns: Array<{ role: string; content: string; timestamp: string; seq: number }> = []
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    calls.push({ url, method, body: typeof init?.body === 'string' ? init.body : null })
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.endsWith('/message') && method === 'POST') {
      const text = (JSON.parse(String(init?.body)) as { text: string }).text
      turns = [
        ...turns,
        { role: 'user', content: text, timestamp: '2026-08-08T12:00:00Z', seq: turns.length },
      ]
      // The brain "replies" on the NEXT read after a beat (the refetch).
      setTimeout(() => {
        turns = [
          ...turns,
          {
            role: 'assistant',
            content: 'hola humano',
            timestamp: '2026-08-08T12:00:02Z',
            seq: turns.length,
          },
        ]
      }, 30)
      return json({}, 202)
    }
    if (url === '/api/conversations') {
      return json(
        turns.length === 0
          ? []
          : [
              {
                key: 'console::chat-1',
                active_session: 1,
                session_count: 1,
                turn_count: turns.length,
                last_activity: '2026-08-08T12:00:00Z',
                last_role: turns[turns.length - 1].role,
                taken_over: false,
              },
            ],
      )
    }
    if (url.endsWith('/sessions') && method === 'GET')
      return json([{ id: 1, turn_count: turns.length }])
    return json(turns)
  }) as typeof fetch
  return { fetcher, calls }
}

beforeEach(() => localStorage.clear())

describe('the direct chat (console channel)', () => {
  it('New chat opens a draft console conversation with an enabled composer', async () => {
    const { fetcher } = chatFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    expect(screen.getByText(/direct chat/i)).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /message/i })).toBeEnabled()
  })

  it('sends as USER via /message and shows Thinking… until the reply lands', async () => {
    const { fetcher, calls } = chatFetch()
    const view = render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    const box = screen.getByRole('textbox', { name: /message/i })
    fireEvent.change(box, { target: { value: 'hola' } })
    fireEvent.keyDown(box, { key: 'Enter' })
    await screen.findByText(/thinking/i)
    const msg = calls.find((c) => c.url.endsWith('/message'))
    expect(msg?.method).toBe('POST')
    expect(msg?.url).toMatch(/^\/api\/conversations\/console%3A%3A/)
    expect(JSON.parse(msg?.body ?? '{}')).toEqual({ text: 'hola' })
    // No operator reply path for console sends.
    expect(calls.some((c) => c.url.endsWith('/reply'))).toBe(false)
    // The reply arrives on the next tick: thinking clears, the turn shows.
    await new Promise((r) => setTimeout(r, 60))
    view.rerender(<Console fetcher={fetcher} feedVersion={1} coreState="running" />)
    await screen.findByText('hola humano')
    expect(screen.queryByText(/thinking/i)).toBeNull()
  })

  it('takeover is disabled with its reason in console conversations', async () => {
    const { fetcher } = chatFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    const takeover = screen.getByRole('button', { name: /take over/i })
    expect(takeover).toBeDisabled()
    expect(screen.getByText(/you already are the human/i)).toBeInTheDocument()
  })
})
