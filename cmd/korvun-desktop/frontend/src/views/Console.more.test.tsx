// The completion rider's red suite (FR-DEL-3 / FR-SEARCH / FR-UNREAD /
// FR-POLISH UI side): filter, content search opening at the point, unread
// badges that clear on open, delete flows behind explicit confirmation,
// and the composer's Enter/Shift+Enter contract.
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { Console } from './Console'

type Captured = { url: string; method: string; body: string | null }

const CONVS = [
  {
    key: 'telegram::777',
    active_session: 2,
    session_count: 2,
    turn_count: 5,
    last_activity: '2026-08-08T12:00:00Z',
    last_role: 'assistant',
    taken_over: false,
  },
  {
    key: 'webhook::w1',
    active_session: 1,
    session_count: 1,
    turn_count: 2,
    last_activity: '2026-08-08T11:00:00Z',
    last_role: 'user',
    taken_over: false,
  },
]

const ACTIVE_TURNS = [
  { role: 'user', content: 'algo activo', timestamp: '2026-08-08T12:00:00Z', seq: 0 },
]
const OLD_TURNS = [
  { role: 'user', content: 'la lavadora hace ruido', timestamp: '2026-08-07T12:00:00Z', seq: 0 },
]
const HITS = [
  {
    key: 'telegram::777',
    session: 1,
    seq: 0,
    role: 'user',
    content: 'la lavadora hace ruido',
    timestamp: '2026-08-07T12:00:00Z',
  },
]

function riderFetch(): { fetcher: typeof fetch; calls: Captured[] } {
  const calls: Captured[] = []
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    calls.push({ url, method, body: typeof init?.body === 'string' ? init.body : null })
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.startsWith('/api/search')) return json(HITS)
    if (method === 'DELETE') return json({}, 204)
    if (url === '/api/conversations') return json(CONVS)
    if (url.endsWith('/sessions') && method === 'GET') {
      return json([
        { id: 1, turn_count: 1 },
        { id: 2, turn_count: 4 },
      ])
    }
    if (url.endsWith('/sessions/1')) return json(OLD_TURNS)
    return json(ACTIVE_TURNS)
  }) as typeof fetch
  return { fetcher, calls }
}

beforeEach(() => localStorage.clear())

describe('inbox filter + search', () => {
  it('the filter narrows rows instantly by channel/id', async () => {
    const { fetcher } = riderFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    await screen.findByText(/telegram/i)
    fireEvent.change(screen.getByRole('searchbox', { name: /filter or search/i }), {
      target: { value: 'web' },
    })
    expect(screen.queryByText(/telegram/i)).toBeNull()
    expect(screen.getByText(/webhook/i)).toBeInTheDocument()
  })

  it('Enter searches message content and a hit opens the conversation AT its session', async () => {
    const { fetcher, calls } = riderFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    await screen.findByText(/telegram/i)
    const box = screen.getByRole('searchbox', { name: /filter or search/i })
    fireEvent.change(box, { target: { value: 'lavadora' } })
    fireEvent.keyDown(box, { key: 'Enter' })
    // The hit renders with its content and is clickable.
    const hit = await screen.findByRole('button', { name: /la lavadora hace ruido/i })
    fireEvent.click(hit)
    // Opens the conversation at session 1 (archived) — the exact point.
    await screen.findByText(/archived session — read only/i)
    expect(calls.some((c) => c.url.includes('/sessions/1') && c.method === 'GET')).toBe(true)
    expect(calls.some((c) => c.url.startsWith('/api/search?q=lavadora'))).toBe(true)
  })
})

describe('unread badges', () => {
  it('shows per-conversation counts and clears on open', async () => {
    const { fetcher } = riderFetch()
    const view = render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    await screen.findByText(/telegram/i)
    // Never opened: fully unread.
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    // Opening marks read; the badge for that conversation clears.
    fireEvent.click(screen.getByRole('button', { name: /telegram/i }))
    await screen.findByText('algo activo')
    view.rerender(<Console fetcher={fetcher} feedVersion={1} coreState="running" />)
    await screen.findAllByText(/telegram/i) // inbox row + pane header
    expect(screen.queryByText('5')).toBeNull()
    expect(screen.getByText('2')).toBeInTheDocument() // the unopened one stays lit
  })
})

describe('deletion behind explicit confirmation', () => {
  it('deletes the conversation only after the exact confirm copy', async () => {
    const { fetcher, calls } = riderFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /telegram/i }))
    await screen.findByText('algo activo')
    fireEvent.click(screen.getByRole('button', { name: /delete conversation/i }))
    // Nothing deleted yet; the copy is on stage.
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    expect(
      screen.getByText('This deletes the conversation from disk. No undo.'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    await screen.findByText(/select a conversation/i)
    expect(
      calls.some((c) => c.method === 'DELETE' && c.url === '/api/conversations/telegram%3A%3A777'),
    ).toBe(true)
  })

  it('cancel walks away without deleting', async () => {
    const { fetcher, calls } = riderFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /telegram/i }))
    await screen.findByText('algo activo')
    fireEvent.click(screen.getByRole('button', { name: /delete conversation/i }))
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByText(/no undo/i)).toBeNull()
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
  })

  it('an archived session gets its own delete with its own copy', async () => {
    const { fetcher, calls } = riderFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /telegram/i }))
    await screen.findByText('algo activo')
    fireEvent.click(screen.getByRole('button', { name: /^session 1$/i }))
    await screen.findByText(/archived session — read only/i)
    fireEvent.click(screen.getByRole('button', { name: /delete session/i }))
    expect(
      screen.getByText('This deletes the archived session from disk. No undo.'),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    await screen.findByText('algo activo') // back on the active session
    expect(
      calls.some(
        (c) => c.method === 'DELETE' && c.url === '/api/conversations/telegram%3A%3A777/sessions/1',
      ),
    ).toBe(true)
  })
})

describe('composer contract', () => {
  it('Enter sends; Shift+Enter makes a newline instead', async () => {
    const { fetcher, calls } = riderFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    fireEvent.click(await screen.findByRole('button', { name: /telegram/i }))
    await screen.findByText('algo activo')
    const box = screen.getByRole('textbox', { name: /reply/i })
    fireEvent.change(box, { target: { value: 'línea' } })
    fireEvent.keyDown(box, { key: 'Enter', shiftKey: true })
    expect(calls.some((c) => c.url.endsWith('/reply'))).toBe(false)
    fireEvent.keyDown(box, { key: 'Enter' })
    await screen.findByText(/on its way/i)
    expect(calls.some((c) => c.url.endsWith('/reply') && c.method === 'POST')).toBe(true)
  })
})
