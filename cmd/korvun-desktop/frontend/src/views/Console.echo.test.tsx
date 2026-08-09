// The 2026-08-09 demo pegas 1-3 (red): the user's turn echoes IMMEDIATELY on
// send (before the brain finishes), sits on the RIGHT with the violet
// accent in console conversations, reconciles with the real refetch without
// duplicating, never vanishes silently on failure — and the wait is honest:
// "Thinking…" from the send instant, with a plain-words line after ~10s.
//
// The fake here mirrors PRODUCTION persistence: the POST returns 202 and
// the user+assistant pair lands TOGETHER only when the brain finishes (the
// final-pair-only contract) — exactly the gap the optimistic echo covers.
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Console } from './Console'

type Turn = { role: string; content: string; timestamp: string; seq: number }

function realisticFetch(opts?: { failSend?: boolean; brainDelayMs?: number }) {
  let turns: Turn[] = []
  let pairTimer: ReturnType<typeof setTimeout> | null = null
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.endsWith('/message') && method === 'POST') {
      if (opts?.failSend) return json({ error: 'console channel not wired' }, 409)
      const text = (JSON.parse(String(init?.body)) as { text: string }).text
      // PRODUCTION shape: nothing persists until the brain finishes; then
      // the FINAL PAIR lands together.
      pairTimer = setTimeout(() => {
        turns = [
          ...turns,
          { role: 'user', content: text, timestamp: '2026-08-09T12:00:00Z', seq: turns.length },
          {
            role: 'assistant',
            content: 'aviso enviado',
            timestamp: '2026-08-09T12:00:30Z',
            seq: turns.length + 1,
          },
        ]
      }, opts?.brainDelayMs ?? 40)
      return json({}, 202)
    }
    if (url === '/api/conversations')
      return json(
        turns.length === 0
          ? []
          : [
              {
                key: 'console::chat-1',
                active_session: 1,
                session_count: 1,
                turn_count: turns.length,
                last_activity: '2026-08-09T12:00:00Z',
                last_role: turns[turns.length - 1].role,
                taken_over: false,
              },
            ],
      )
    if (url.endsWith('/sessions') && method === 'GET')
      return json([{ id: 1, turn_count: turns.length }])
    return json(turns)
  }) as typeof fetch
  return { fetcher, cleanup: () => pairTimer !== null && clearTimeout(pairTimer) }
}

async function openDraftAndSend(fetcher: typeof fetch, text: string): Promise<void> {
  render(<Console coreState="running" fetcher={fetcher} feedVersion={0} pollIntervalMs={25} />)
  fireEvent.click(await screen.findByRole('button', { name: 'New chat' }))
  const box = await screen.findByRole('textbox', { name: 'Message Korvun' })
  fireEvent.change(box, { target: { value: text } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
}

describe('immediate echo (pega 2)', () => {
  it('paints the user turn instantly, then reconciles without duplicating', async () => {
    const { fetcher, cleanup } = realisticFetch()
    try {
      await openDraftAndSend(fetcher, 'envía un aviso')
      // IMMEDIATE: the echo is on screen before any refetch can have the pair.
      const echo = await screen.findByText('envía un aviso', { selector: '.console-turn-content' })
      expect(echo).toBeTruthy()
      // Reconciliation: when the real pair lands, exactly ONE user turn.
      await waitFor(() => expect(screen.getByText('aviso enviado', { selector: '.console-turn-content' })).toBeTruthy())
      await waitFor(() =>
        expect(
          screen.getAllByText('envía un aviso', { selector: '.console-turn-content' }),
        ).toHaveLength(1),
      )
    } finally {
      cleanup()
    }
  })

  it('a failed send stays visible, marked as not sent', async () => {
    const { fetcher, cleanup } = realisticFetch({ failSend: true })
    try {
      await openDraftAndSend(fetcher, 'este no sale')
      // The echo never vanishes silently: it stays with a not-sent mark.
      await waitFor(() => {
        const turn = screen
          .getByText('este no sale', { selector: '.console-turn-content' })
          .closest('.console-turn')
        expect(turn?.getAttribute('data-send-state')).toBe('failed')
      })
      expect(screen.getByRole('alert')).toBeTruthy()
    } finally {
      cleanup()
    }
  })
})

describe('own side (pega 3)', () => {
  it('console user turns sit on the right with the violet accent side', async () => {
    const { fetcher, cleanup } = realisticFetch()
    try {
      await openDraftAndSend(fetcher, 'hola')
      const turn = (
        await screen.findByText('hola', { selector: '.console-turn-content' })
      ).closest('.console-turn')
      expect(turn?.getAttribute('data-side')).toBe('own')
      await waitFor(() => expect(screen.getByText('aviso enviado', { selector: '.console-turn-content' })).toBeTruthy())
      const reply = screen
        .getByText('aviso enviado', { selector: '.console-turn-content' })
        .closest('.console-turn')
      expect(reply?.getAttribute('data-side')).toBeNull()
    } finally {
      cleanup()
    }
  })
})

describe('honest wait (pega 4-UI)', () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }))
  afterEach(() => vi.useRealTimers())

  it('Thinking… shows from the send instant and turns honest after 10s', async () => {
    const { fetcher, cleanup } = realisticFetch({ brainDelayMs: 60_000 })
    try {
      await openDraftAndSend(fetcher, 'tarda mucho')
      await waitFor(() => expect(screen.getByText('Thinking…')).toBeTruthy())
      await act(async () => {
        vi.advanceTimersByTime(11_000)
      })
      await waitFor(() =>
        expect(screen.getByText(/local model is thinking/i)).toBeTruthy(),
      )
    } finally {
      cleanup()
    }
  })
})
describe('draft clears on success (sp4 regression guard)', () => {
  it('empties the box after a successful send', async () => {
    const { fetcher, cleanup } = realisticFetch()
    try {
      await openDraftAndSend(fetcher, 'limpia el cajetin')
      const box = screen.getByRole('textbox', { name: 'Message Korvun' }) as HTMLTextAreaElement
      await waitFor(() => expect(box.value).toBe(''))
    } finally {
      cleanup()
    }
  })
})

describe('switching conversations (the duplicated-conversation bug)', () => {
  it('a New chat shows no stale turns and the echo survives same-text history', async () => {
    // Conversation A already holds a "hola" pair; the user opens a NEW chat
    // and sends "hola" again. The echo MUST paint (the old conversation's
    // content must not suppress it) and A's turns must not bleed into B.
    let turns = [
      { role: 'user', content: 'hola', timestamp: '2026-08-09T17:00:00Z', seq: 0 },
      { role: 'assistant', content: 'respuesta vieja', timestamp: '2026-08-09T17:00:30Z', seq: 1 },
    ]
    const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method ?? 'GET'
      const json = (v: unknown, status = 200) =>
        new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
      if (url.endsWith('/message') && method === 'POST') return json({}, 202)
      if (url === '/api/conversations')
        return json([
          {
            key: 'console::chat-a',
            active_session: 1,
            session_count: 1,
            turn_count: turns.length,
            last_activity: '2026-08-09T17:00:00Z',
            last_role: 'assistant',
            taken_over: false,
          },
        ])
      if (url.endsWith('/sessions') && method === 'GET')
        return json([{ id: 1, turn_count: turns.length }])
      // Conversation detail: A has the pair; the NEW chat's detail is SLOW
      // (the real bearer fetch) — the stale window under test.
      if (url.includes('chat-a')) return json(turns)
      await new Promise((r) => setTimeout(r, 150))
      return json([])
    }) as typeof fetch

    render(<Console coreState="running" fetcher={fetcher} feedVersion={0} pollIntervalMs={5000} />)
    // Open A (it has history on screen).
    fireEvent.click(await screen.findByRole('button', { name: /chat-a/i }))
    await screen.findByText('respuesta vieja', { selector: '.console-turn-content' })
    // New chat: the pane MUST be clean IMMEDIATELY (no stale turns), even
    // while the new conversation's detail fetch is still in flight.
    fireEvent.click(screen.getByRole('button', { name: 'New chat' }))
    expect(
      screen.queryByText('respuesta vieja', { selector: '.console-turn-content' }),
    ).toBeNull()
    // Send "hola" INSIDE the stale window — same text as A's history: the
    // echo must still paint.
    const box = await screen.findByRole('textbox', { name: 'Message Korvun' })
    fireEvent.change(box, { target: { value: 'hola' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    await waitFor(() =>
      expect(screen.getByText('hola', { selector: '.console-turn-content' })).toBeTruthy(),
    )
  })
})

describe('system-command echo (the /tools Sending… hang)', () => {
  it('retires the echo when the system ack lands (commands persist no user turn)', async () => {
    let turns: Turn[] = []
    const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method ?? 'GET'
      const json = (v: unknown, status = 200) =>
        new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
      if (url.endsWith('/message') && method === 'POST') {
        // A system command: only the SYSTEM ack persists — never the user turn.
        setTimeout(() => {
          turns = [
            { role: 'system', content: 'Gatekeeper — brain "default"', timestamp: '2026-08-09T18:00:01Z', seq: 0 },
          ]
        }, 30)
        return json({}, 202)
      }
      if (url === '/api/conversations')
        return json(
          turns.length === 0
            ? []
            : [{ key: 'console::chat-1', active_session: 1, session_count: 1, turn_count: turns.length, last_activity: '2026-08-09T18:00:01Z', last_role: 'system', taken_over: false }],
        )
      if (url.endsWith('/sessions') && method === 'GET')
        return json([{ id: 1, turn_count: turns.length }])
      return json(turns)
    }) as typeof fetch

    await openDraftAndSend(fetcher, '/tools')
    await waitFor(() =>
      expect(screen.getByText(/Gatekeeper/, { selector: '.console-turn-content' })).toBeTruthy(),
    )
    // The echo must be GONE once the ack landed — no eternal Sending….
    await waitFor(() =>
      expect(screen.queryByText('/tools', { selector: '.console-turn-content' })).toBeNull(),
    )
  })
})
