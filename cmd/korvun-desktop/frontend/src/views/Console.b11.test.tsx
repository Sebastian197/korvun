// B11 RED — la espera veraz (sealed design b11-b12-honest-chat.md,
// APROBADO POR CHANO 2026-08-29). Pinned here: the sealed mold
// «{brain} está pensando — {model_id} · {local|nube}…» fed by the B9
// conversation id + N6 /api/brains; the long-wait fork local/nube; honest
// degradation (drop detail, NEVER invent — the old hardcoded line only
// survives when it is literally true).
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetFeedForTests } from '../feed/store'
import { Console } from './Console'

interface Shape {
  brains?: unknown[]
  routes?: unknown[]
  conversations?: unknown[]
}

const OPENROUTER = {
  name: 'openrouter',
  sensitivity: 'public',
  policy: 'priority',
  dispatch: 'fanout',
  models: [{ provider: 'openai-compatible', model_id: 'openrouter/auto', locality: 'cloud' }],
}
const ASISTENTE = {
  name: 'asistente',
  sensitivity: 'private',
  policy: 'priority',
  dispatch: 'fanout',
  models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
}

function conv(key: string) {
  return {
    key,
    active_session: 1,
    session_count: 1,
    turn_count: 0,
    last_activity: '2026-08-29T09:00:00Z',
    last_role: 'user',
    taken_over: false,
  }
}

function consoleFetch(shape: Shape): typeof fetch {
  return (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (url.endsWith('/message') && init?.method === 'POST') return json({}, 202)
    if (url.includes('/api/config')) {
      return json({
        channels: [{ type: 'console', mode: 'attached', token_env: 'X' }],
        brains: [{ name: 'asistente' }, { name: 'openrouter' }],
        routes: shape.routes ?? [{ channel: 'console', brain: 'asistente' }],
      })
    }
    if (url.includes('/api/brains')) return json(shape.brains ?? [ASISTENTE, OPENROUTER])
    if (url === '/api/conversations') return json(shape.conversations ?? [])
    if (url.endsWith('/sessions')) return json([{ id: 1, turn_count: 0 }])
    return json([])
  }) as typeof fetch
}

async function openAndSend(shape: Shape, rowText: string): Promise<void> {
  render(<Console fetcher={consoleFetch(shape)} feedVersion={0} coreState="running" />)
  fireEvent.click(await screen.findByText(rowText))
  const box = await screen.findByRole('textbox', { name: /message/i })
  fireEvent.change(box, { target: { value: 'hola' } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
}

beforeEach(() => {
  localStorage.clear()
  resetFeedForTests()
  vi.useFakeTimers({ shouldAdvanceTime: true })
})
afterEach(() => vi.useRealTimers())

describe('estado 1 — la espera veraz (el caso del domingo)', () => {
  it('brain de nube: la pantalla nombra brain, modelo y NUBE', async () => {
    await openAndSend({ conversations: [conv('console::b:openrouter:chat-1')] }, 'chat-1')
    await screen.findByText('openrouter está pensando — openrouter/auto · nube…')
  })

  it('conversación legacy: el brain de la ruta, con su modelo local', async () => {
    await openAndSend({ conversations: [conv('console::chat-2')] }, 'chat-2')
    await screen.findByText('asistente está pensando — llama3.2:1b · local…')
  })
})

describe('estado 2 — la espera larga bifurca por la verdad', () => {
  it('local: el letrero de hoy, solo cuando ES verdad', async () => {
    await openAndSend({ conversations: [conv('console::chat-2')] }, 'chat-2')
    await screen.findByText(/asistente está pensando/)
    vi.advanceTimersByTime(11_000)
    await screen.findByText(
      'llama3.2:1b sigue pensando — un modelo local puede tardar en esta máquina…',
    )
  })

  it('nube: jamás el letrero local — dice que la petición está en la nube', async () => {
    await openAndSend({ conversations: [conv('console::b:openrouter:chat-1')] }, 'chat-1')
    await screen.findByText(/openrouter está pensando/)
    vi.advanceTimersByTime(11_000)
    await screen.findByText('openrouter sigue sin responder — la petición está en la nube…')
    expect(screen.queryByText(/local model is thinking/i)).toBeNull()
    expect(screen.queryByText(/en esta máquina/)).toBeNull()
  })
})

describe('degradación honesta — quitar detalle, jamás inventar', () => {
  it('sin datos del modelo: nombra el brain y calla el resto', async () => {
    await openAndSend(
      { brains: [], conversations: [conv('console::b:openrouter:chat-1')] },
      'chat-1',
    )
    await screen.findByText('openrouter está pensando…')
  })

  it('sin brain resoluble (ruta vacía, id legacy): «Pensando…» a secas', async () => {
    await openAndSend(
      { brains: [], routes: [], conversations: [conv('console::chat-9')] },
      'chat-9',
    )
    await screen.findByText('Pensando…')
    await waitFor(() => expect(screen.queryByText(/está pensando —/)).toBeNull())
  })
})
