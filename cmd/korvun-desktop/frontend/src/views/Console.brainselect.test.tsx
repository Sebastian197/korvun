// B9 RED — the brain selector on New chat (sealed design ola2-designs §1,
// APROBADO POR CHANO 2026-08-23). Sealed decisions pinned here: the
// dropdown only exists with >1 brain; Esc AND click-outside cancel; the
// microload is «Cargando cerebros…»; the route default comes preselected;
// the born conversation is addressed to the CHOSEN brain (key carries the
// b: prefix) and the badge shows it in header and inbox row.
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { Console } from './Console'

interface Shape {
  brains?: unknown[]
  conversations?: unknown[]
  brainsNeverResolve?: boolean
}

const TWO_BRAINS = [
  { name: 'asistente', sensitivity: 'private', policy: 'priority', dispatch: 'fanout', models: [] },
  { name: 'openrouter', sensitivity: 'public', policy: 'priority', dispatch: 'fanout', models: [] },
]

function consoleFetch(shape: Shape): typeof fetch {
  return (async (input: RequestInfo | URL) => {
    const url = String(input)
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.includes('/api/config')) {
      return json({
        channels: [{ type: 'console', mode: 'attached', token_env: 'X' }],
        brains: (shape.brains ?? TWO_BRAINS).map((b) => ({ name: (b as { name: string }).name })),
        routes: [{ channel: 'console', brain: 'asistente' }],
      })
    }
    if (url.includes('/api/brains')) {
      if (shape.brainsNeverResolve) return new Promise<Response>(() => {})
      return json(shape.brains ?? TWO_BRAINS)
    }
    if (url === '/api/conversations') return json(shape.conversations ?? [])
    if (url.endsWith('/sessions')) return json([{ id: 1, turn_count: 0 }])
    return json([])
  }) as typeof fetch
}

beforeEach(() => localStorage.clear())

function renderConsole(shape: Shape = {}) {
  return render(<Console fetcher={consoleFetch(shape)} feedVersion={0} coreState="running" />)
}

describe('the dropdown (two brains)', () => {
  it('New chat opens «¿Con qué cerebro?» with names, privacy labels and the route default preselected', async () => {
    renderConsole()
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    await screen.findByText('¿Con qué cerebro?')
    const def = screen.getByRole('radio', { name: /asistente/i }) as HTMLInputElement
    expect(def.checked).toBe(true)
    expect((screen.getByRole('radio', { name: /openrouter/i }) as HTMLInputElement).checked).toBe(
      false,
    )
    expect(screen.getByText('Privado')).toBeInTheDocument()
    expect(screen.getByText('Público')).toBeInTheDocument()
  })

  it('choosing the cloud brain births the conversation EXACTLY at it (the case that birthed the piece)', async () => {
    renderConsole()
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    fireEvent.click(await screen.findByRole('radio', { name: /openrouter/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Crear chat' }))
    // The pane header carries the sealed composition: Console · brain · id.
    const title = await screen.findByRole('heading', { name: /console · openrouter · chat-/i })
    expect(title).toBeInTheDocument()
    expect(screen.queryByText('¿Con qué cerebro?')).not.toBeInTheDocument()
  })

  it('keeping the default births an addressed conversation too', async () => {
    renderConsole()
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    fireEvent.click(await screen.findByRole('button', { name: 'Crear chat' }))
    await screen.findByRole('heading', { name: /console · asistente · chat-/i })
  })
})

describe('cancellation (both sealed gestures)', () => {
  it('Escape closes the dropdown leaving no trace', async () => {
    renderConsole()
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    await screen.findByText('¿Con qué cerebro?')
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByText('¿Con qué cerebro?')).not.toBeInTheDocument(),
    )
    // No conversation was created.
    expect(screen.getByText(/select a conversation/i)).toBeInTheDocument()
  })

  it('click outside closes the dropdown leaving no trace', async () => {
    renderConsole()
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    await screen.findByText('¿Con qué cerebro?')
    fireEvent.mouseDown(document.body)
    await waitFor(() =>
      expect(screen.queryByText('¿Con qué cerebro?')).not.toBeInTheDocument(),
    )
    expect(screen.getByText(/select a conversation/i)).toBeInTheDocument()
  })
})

describe('microload and the single-brain case', () => {
  it('shows «Cargando cerebros…» while the list is in flight', async () => {
    renderConsole({ brainsNeverResolve: true })
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    await screen.findByText('Cargando cerebros…')
  })

  it('with ONE brain the dropdown never appears — one click creates directly', async () => {
    renderConsole({ brains: [TWO_BRAINS[0]] })
    fireEvent.click(await screen.findByRole('button', { name: /new chat/i }))
    await screen.findByRole('heading', { name: /console · chat-/i })
    expect(screen.queryByText('¿Con qué cerebro?')).not.toBeInTheDocument()
  })
})

describe('the badge (AS-5)', () => {
  it('an addressed conversation shows its brain in the inbox row', async () => {
    renderConsole({
      conversations: [
        {
          key: 'console::b:openrouter:chat-ab12cd34',
          active_session: 1,
          session_count: 1,
          turn_count: 1,
          last_activity: '2026-08-29T09:00:00Z',
          last_role: 'user',
          taken_over: false,
        },
      ],
    })
    const row = await screen.findByText('chat-ab12cd34')
    expect(row).toBeInTheDocument()
    expect(screen.getByTestId('inbox-brain-badge').textContent).toBe('openrouter')
  })

  it('a legacy conversation renders exactly as today', async () => {
    renderConsole({
      conversations: [
        {
          key: 'console::chat-legacy1',
          active_session: 1,
          session_count: 1,
          turn_count: 1,
          last_activity: '2026-08-29T09:00:00Z',
          last_role: 'user',
          taken_over: false,
        },
      ],
    })
    await screen.findByText('chat-legacy1')
    expect(screen.queryByTestId('inbox-brain-badge')).not.toBeInTheDocument()
  })
})
