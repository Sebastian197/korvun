// SP4 red (operator-console spec, FR-UI-1): the chat view — inbox with
// takeover badges, the conversation pane where the OPERATOR role is never
// dressed as the AI, the composer with honest states, keyboard-reachable
// controls, and read-only archived sessions.
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Console } from './Console'

type Captured = { url: string; method: string; body: string | null }

const CONVS = [
  {
    key: 'telegram::777',
    active_session: 2,
    session_count: 2,
    turn_count: 6,
    last_activity: '2026-08-08T12:00:00Z',
    last_role: 'operator',
    taken_over: true,
  },
  {
    key: 'webhook::w1',
    active_session: 1,
    session_count: 1,
    turn_count: 1,
    last_activity: '2026-08-08T11:00:00Z',
    last_role: 'assistant',
    taken_over: false,
  },
]

const ACTIVE_TURNS = [
  { role: 'user', content: 'necesito ayuda humana', timestamp: '2026-08-08T11:58:00Z', seq: 0 },
  { role: 'assistant', content: 'Puedo intentarlo yo', timestamp: '2026-08-08T11:59:00Z', seq: 1 },
  {
    role: 'operator',
    content: 'aquí Chano, te atiendo',
    timestamp: '2026-08-08T12:00:00Z',
    seq: 2,
  },
  { role: 'system', content: 'nota del sistema', timestamp: '2026-08-08T12:00:30Z', seq: 3 },
]

const OLD_TURNS = [{ role: 'user', content: 'histórico viejo', timestamp: '', seq: 0 }]

// A STATEFUL fake of the console API: takeover and new-session mutate what
// later reads return, so the view's refetch-after-mutation shows real state
// transitions (the same shape the live core produces).
function consoleFetch(overrides?: { replyStatus?: number }): {
  fetcher: typeof fetch
  calls: Captured[]
} {
  const calls: Captured[] = []
  let takenOver = true
  let sessionIDs = [1, 2]
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    calls.push({ url, method, body: typeof init?.body === 'string' ? init.body : null })
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url === '/api/conversations') {
      const rows = [
        { ...CONVS[0], taken_over: takenOver, session_count: sessionIDs.length },
        CONVS[1],
      ]
      return json(rows)
    }
    if (url.endsWith('/reply')) return json({}, overrides?.replyStatus ?? 202)
    if (url.endsWith('/takeover')) {
      takenOver = true
      return json({}, 204)
    }
    if (url.endsWith('/release')) {
      takenOver = false
      return json({}, 204)
    }
    if (url.endsWith('/sessions') && method === 'POST') {
      const next = sessionIDs[sessionIDs.length - 1] + 1
      sessionIDs = [...sessionIDs, next]
      return json({ session: next })
    }
    if (url.endsWith('/sessions')) {
      return json(sessionIDs.map((id) => ({ id, turn_count: 1, first: '', last: '' })))
    }
    if (url.endsWith('/sessions/1')) return json(OLD_TURNS)
    return json(ACTIVE_TURNS) // conversation detail (active session)
  }) as typeof fetch
  return { fetcher, calls }
}

async function openFirstConversation(fetcher: typeof fetch, feedVersion = 0) {
  const view = render(<Console fetcher={fetcher} feedVersion={feedVersion} coreState="running" />)
  fireEvent.click(await screen.findByText(/telegram/i))
  await screen.findByText('aquí Chano, te atiendo')
  return view
}

describe('Console — inbox', () => {
  it('lists conversations with readable channel + id, session count, last role and TAKEOVER badge', async () => {
    const { fetcher } = consoleFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    await screen.findByText(/telegram/i)
    // Readable identity, not the raw key.
    expect(screen.queryByText('telegram::777')).toBeNull()
    expect(screen.getByText('777')).toBeInTheDocument()
    // Session count + last role + takeover badge.
    expect(screen.getByText(/2 sessions/i)).toBeInTheDocument()
    expect(screen.getByText(/taken over/i)).toBeInTheDocument()
    // Order: the fixture order (newest activity first) is preserved.
    const rows = screen.getAllByRole('button', { name: /telegram|webhook/i })
    expect(rows[0].textContent).toMatch(/telegram/i)
    expect(rows[1].textContent).toMatch(/webhook/i)
  })

  it('re-fetches when the SSE change signal ticks — content never rides the SSE itself', async () => {
    const { fetcher, calls } = consoleFetch()
    const view = render(<Console fetcher={fetcher} feedVersion={0} coreState="running" />)
    await screen.findByText(/telegram/i)
    const before = calls.filter((c) => c.url === '/api/conversations').length
    view.rerender(<Console fetcher={fetcher} feedVersion={1} coreState="running" />)
    await screen.findByText(/telegram/i)
    const after = calls.filter((c) => c.url === '/api/conversations').length
    expect(after).toBeGreaterThan(before)
  })
})

describe('Console — conversation pane', () => {
  it('renders the active session with the four roles visually distinct — the operator is NEVER dressed as the AI', async () => {
    const { fetcher } = consoleFetch()
    await openFirstConversation(fetcher)

    const operatorTurn = screen.getByText('aquí Chano, te atiendo').closest('[data-role]')
    const assistantTurn = screen.getByText('Puedo intentarlo yo').closest('[data-role]')
    const userTurn = screen.getByText('necesito ayuda humana').closest('[data-role]')
    const systemTurn = screen.getByText('nota del sistema').closest('[data-role]')
    expect(operatorTurn?.getAttribute('data-role')).toBe('operator')
    expect(assistantTurn?.getAttribute('data-role')).toBe('assistant')
    expect(userTurn?.getAttribute('data-role')).toBe('user')
    expect(systemTurn?.getAttribute('data-role')).toBe('system')
    // The honesty label: the operator turn is visibly the human's.
    expect(screen.getByText(/operator \(you\)/i)).toBeInTheDocument()
  })

  it('navigates an old session read-only, clearly separated as archived', async () => {
    const { fetcher } = consoleFetch()
    await openFirstConversation(fetcher)
    fireEvent.click(screen.getByRole('button', { name: /session 1/i }))
    await screen.findByText('histórico viejo')
    expect(screen.getByText(/archived session — read only/i)).toBeInTheDocument()
    // The composer is gone in an archived session.
    expect(screen.queryByRole('textbox', { name: /reply/i })).toBeNull()
  })
})

describe('Console — composer', () => {
  it('sends the reply and shows the honest in-flight state on 202', async () => {
    const { fetcher, calls } = consoleFetch()
    await openFirstConversation(fetcher)
    const box = screen.getByRole('textbox', { name: /reply/i })
    fireEvent.change(box, { target: { value: 'voy yo' } })
    fireEvent.click(screen.getByRole('button', { name: /send/i }))
    await screen.findByText(/on its way/i)
    const reply = calls.find((c) => c.url.endsWith('/reply'))
    expect(reply?.method).toBe('POST')
    expect(JSON.parse(reply?.body ?? '{}')).toEqual({ text: 'voy yo' })
  })

  it('surfaces a channel-not-registered failure instead of pretending', async () => {
    const { fetcher } = consoleFetch({ replyStatus: 409 })
    await openFirstConversation(fetcher)
    fireEvent.change(screen.getByRole('textbox', { name: /reply/i }), {
      target: { value: 'x' },
    })
    fireEvent.click(screen.getByRole('button', { name: /send/i }))
    await screen.findByText(/channel not registered/i)
  })

  it('is disabled with a clear reason when the core is stopped', async () => {
    const { fetcher } = consoleFetch()
    render(<Console fetcher={fetcher} feedVersion={0} coreState="stopped" />)
    fireEvent.click(await screen.findByText(/telegram/i))
    await screen.findByText('aquí Chano, te atiendo')
    const box = screen.getByRole('textbox', { name: /reply/i })
    expect(box).toBeDisabled()
    expect(screen.getByText(/core is stopped/i)).toBeInTheDocument()
  })
})

describe('Console — controls', () => {
  it('takeover/release POST their endpoints and show who is in charge', async () => {
    const { fetcher, calls } = consoleFetch()
    await openFirstConversation(fetcher)
    // The fixture conversation is taken over: the state is visible and the
    // control offers the release.
    expect(screen.getByText(/you are handling this conversation/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /release/i }))
    await screen.findByText(/korvun is handling this conversation/i)
    expect(calls.some((c) => c.url.endsWith('/release') && c.method === 'POST')).toBe(true)
  })

  it('new session POSTs the console reset — and never a reply', async () => {
    const { fetcher, calls } = consoleFetch()
    await openFirstConversation(fetcher)
    fireEvent.click(screen.getByRole('button', { name: /new session/i }))
    await screen.findByText(/session 3/i)
    expect(calls.some((c) => c.url.endsWith('/sessions') && c.method === 'POST')).toBe(true)
    expect(calls.some((c) => c.url.endsWith('/reply'))).toBe(false)
  })

  it('every control is reachable by keyboard (real buttons, real textbox)', async () => {
    const { fetcher } = consoleFetch()
    await openFirstConversation(fetcher)
    const box = screen.getByRole('textbox', { name: /reply/i })
    // Send only arms with a draft (an empty reply is not a thing) — type
    // first, then walk every control by focus.
    fireEvent.change(box, { target: { value: 'algo' } })
    for (const name of [/release|take over/i, /new session/i, /send/i]) {
      const el = screen.getByRole('button', { name })
      el.focus()
      expect(document.activeElement).toBe(el)
    }
    box.focus()
    expect(document.activeElement).toBe(box)
  })
})
