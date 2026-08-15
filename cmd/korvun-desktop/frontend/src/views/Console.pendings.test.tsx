// Estreno E-14 (adversarial H11): the optimistic echo was a SINGLE slot — a
// second rapid send overwrote the first pending before its real pair
// landed, so the first echo vanished silently: exactly the bug the echo
// piece promises cannot happen. The echo is now a LIST: every in-flight
// send stays visible until ITS pair reconciles it, including two sends with
// identical content.
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Console } from './Console'

type Turn = { role: string; content: string; timestamp: string; seq: number }

// A fetch fake where the brain pair for each send lands only when the test
// RELEASES it — full control of the reconciliation order, no timers.
function gatedFetch(opts?: { failTexts?: Set<string> }) {
  let turns: Turn[] = []
  const gates: Array<() => void> = []
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.endsWith('/message') && method === 'POST') {
      const text = (JSON.parse(String(init?.body)) as { text: string }).text
      if (opts?.failTexts?.has(text)) return json({ error: 'not wired' }, 409)
      gates.push(() => {
        turns = text.startsWith('/')
          ? [
              ...turns,
              {
                role: 'system',
                content: `ack ${text}`,
                timestamp: '2026-08-15T12:00:00Z',
                seq: turns.length,
              },
            ]
          : [
              ...turns,
              { role: 'user', content: text, timestamp: '2026-08-15T12:00:00Z', seq: turns.length },
              {
                role: 'assistant',
                content: `eco de ${text}`,
                timestamp: '2026-08-15T12:00:30Z',
                seq: turns.length + 1,
              },
            ]
      })
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
                last_activity: '2026-08-15T12:00:00Z',
                last_role: turns[turns.length - 1].role,
                taken_over: false,
              },
            ],
      )
    if (url.endsWith('/sessions') && method === 'GET')
      return json([{ id: 1, turn_count: turns.length }])
    return json(turns)
  }) as typeof fetch
  return { fetcher, releaseNext: () => gates.shift()?.() }
}

async function openChat(fetcher: typeof fetch): Promise<void> {
  render(<Console coreState="running" fetcher={fetcher} feedVersion={0} pollIntervalMs={25} />)
  fireEvent.click(await screen.findByRole('button', { name: 'New chat' }))
  await screen.findByRole('textbox', { name: 'Message Korvun' })
}

function send(text: string): void {
  const box = screen.getByRole('textbox', { name: 'Message Korvun' })
  fireEvent.change(box, { target: { value: text } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
}

describe('pending echoes are a list (E-14)', () => {
  it('two rapid sends both stay visible; each retires with ITS pair', async () => {
    const { fetcher, releaseNext } = gatedFetch()
    await openChat(fetcher)

    send('uno')
    await screen.findByText('uno', { selector: '.console-turn-content' })
    send('dos')
    // BOTH echoes on screen while both are in flight — the single-slot bug
    // dropped 'uno' here.
    await screen.findByText('dos', { selector: '.console-turn-content' })
    expect(screen.getByText('uno', { selector: '.console-turn-content' })).toBeTruthy()
    expect(screen.getAllByText('Sending…')).toHaveLength(2)

    // First pair lands: 'uno' reconciles to the store turn, 'dos' still pending.
    releaseNext()
    await waitFor(() =>
      expect(screen.getByText('eco de uno', { selector: '.console-turn-content' })).toBeTruthy(),
    )
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(1))
    expect(screen.getAllByText('uno', { selector: '.console-turn-content' })).toHaveLength(1)
    expect(screen.getByText('dos', { selector: '.console-turn-content' })).toBeTruthy()

    // Second pair lands: nothing pending, no duplicates.
    releaseNext()
    await waitFor(() =>
      expect(screen.getByText('eco de dos', { selector: '.console-turn-content' })).toBeTruthy(),
    )
    await waitFor(() => expect(screen.queryAllByText('Sending…')).toHaveLength(0))
    expect(screen.getAllByText('dos', { selector: '.console-turn-content' })).toHaveLength(1)
  })

  it('two identical rapid sends reconcile one-to-one with their store turns', async () => {
    const { fetcher, releaseNext } = gatedFetch()
    await openChat(fetcher)

    send('hola')
    await screen.findByText('hola', { selector: '.console-turn-content' })
    send('hola')
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(2))

    releaseNext()
    // One store turn covers ONE pending: exactly one 'Sending…' survives
    // (the old content-equality reconcile retired both).
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(1))

    releaseNext()
    await waitFor(() => expect(screen.queryAllByText('Sending…')).toHaveLength(0))
    // Both real turns visible: two identical user turns from the store.
    await waitFor(() =>
      expect(screen.getAllByText('hola', { selector: '.console-turn-content' })).toHaveLength(2),
    )
  })
})

describe('re-review candidates (E-14 follow-up)', () => {
  it('one system ack retires exactly ONE pending command, oldest first', async () => {
    const { fetcher, releaseNext } = gatedFetch()
    await openChat(fetcher)

    send('/tools')
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(1))
    send('/tools')
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(2))

    // ONE system ack lands: exactly one command pending retires — the old
    // last-turn-is-system heuristic dropped BOTH here.
    releaseNext()
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(1))

    releaseNext()
    await waitFor(() => expect(screen.queryAllByText('Sending…')).toHaveLength(0))
  })

  it('an identical turn already in history does not swallow a new echo', async () => {
    const { fetcher, releaseNext } = gatedFetch()
    await openChat(fetcher)

    // First 'hola' completes fully: the store now HOLDS a user turn 'hola'.
    send('hola')
    releaseNext()
    await waitFor(() => expect(screen.queryAllByText('Sending…')).toHaveLength(0))

    // A NEW 'hola': its echo must survive until ITS OWN pair lands — the
    // historical turn must not count as its reconciliation.
    send('hola')
    await waitFor(() => expect(screen.getAllByText('Sending…')).toHaveLength(1))
    // Give the reconcile effect a tick to (wrongly) retire it.
    await new Promise((r) => setTimeout(r, 100))
    expect(screen.getAllByText('Sending…')).toHaveLength(1)

    releaseNext()
    await waitFor(() => expect(screen.queryAllByText('Sending…')).toHaveLength(0))
  })
})

describe('failed-echo retention (re-review F6)', () => {
  it('a successful retry supersedes the failed copy of the same text', async () => {
    const failTexts = new Set(['hola'])
    const { fetcher, releaseNext } = gatedFetch({ failTexts })
    await openChat(fetcher)

    send('hola')
    await waitFor(() => {
      const turn = screen
        .getByText('hola', { selector: '.console-turn-content' })
        .closest('.console-turn')
      expect(turn?.getAttribute('data-send-state')).toBe('failed')
    })

    // The retry succeeds: its pair lands and the stale failed copy retires.
    failTexts.delete('hola')
    send('hola')
    releaseNext()
    await waitFor(() => expect(screen.queryAllByText('Not sent')).toHaveLength(0))
    await waitFor(() => expect(screen.queryAllByText('Sending…')).toHaveLength(0))
    expect(screen.getAllByText('hola', { selector: '.console-turn-content' })).toHaveLength(1)
  })

  it('a long outage cannot grow the echo list without bound', async () => {
    const failTexts = new Set<string>()
    const { fetcher } = gatedFetch({ failTexts })
    await openChat(fetcher)

    for (let i = 0; i < 25; i++) {
      const text = `intento ${i}`
      failTexts.add(text)
      send(text)
    }
    await waitFor(() => {
      const marks = screen.queryAllByText('Not sent')
      expect(marks.length).toBeGreaterThan(0)
      expect(marks.length).toBeLessThanOrEqual(20)
    })
  })
})
