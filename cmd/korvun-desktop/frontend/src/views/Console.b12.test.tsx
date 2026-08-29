// B12 RED — el fallo visible (sealed design b11-b12-honest-chat.md,
// APROBADO POR CHANO 2026-08-29, decisiones selladas: la tardía SE
// MUESTRA — cancelar = dejar de esperar, no borrar; el fallo vive SOLO
// como banda — el store guarda turnos reales, el registro es Actividad).
// Pinned here: handle_failed corta la espera y pinta la banda sellada con
// [Reintentar]; la segunda línea con los DOS arreglos en reincidencia; el
// umbral de 60 s AVISA y jamás corta solo; la banda se cierra al
// reintentar/cambiar; y NINGÚN estado de error se persiste como turno.
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ingestFrame, resetFeedForTests } from '../feed/store'
import { Console } from './Console'

const OPENROUTER = {
  name: 'openrouter',
  sensitivity: 'public',
  policy: 'priority',
  dispatch: 'fanout',
  models: [{ provider: 'openai-compatible', model_id: 'openrouter/auto', locality: 'cloud' }],
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

interface Harness {
  fetcher: typeof fetch
  posts: () => string[]
  setTurns: (t: Array<{ role: string; content: string; timestamp: string; seq: number }>) => void
}

function makeHarness(): Harness {
  const posts: string[] = []
  let turns: Array<{ role: string; content: string; timestamp: string; seq: number }> = []
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } })
    if (init?.method === 'POST') {
      posts.push(url)
      if (url.endsWith('/message')) return json({}, 202)
      return json({}, 404) // any OTHER write is a contamination tripwire
    }
    if (url.includes('/api/config')) {
      return json({
        channels: [{ type: 'console', mode: 'attached', token_env: 'X' }],
        brains: [{ name: 'openrouter' }],
        routes: [{ channel: 'console', brain: 'openrouter' }],
      })
    }
    if (url.includes('/api/brains')) return json([OPENROUTER])
    if (url === '/api/conversations')
      return json([conv('console::b:openrouter:chat-1'), conv('console::b:openrouter:chat-2')])
    if (url.endsWith('/sessions')) return json([{ id: 1, turn_count: turns.length }])
    return json(turns)
  }) as typeof fetch
  return { fetcher, posts: () => posts, setTurns: (t) => (turns = t) }
}

const FAIL_TEXT = 'openrouter no pudo responder esta vez — fallo del proveedor o del modelo.'
const REPEAT_TEXT =
  'Se repite. Revisa el modelo de openrouter en el Builder o su clave en Ajustes → Secretos.'
const WAIT_TEXT =
  'Sin respuesta de openrouter tras un minuto. Puedes seguir esperando o reintentar.'

function failFrame(): string {
  return JSON.stringify({
    type: 'handle_failed',
    channel: 'console',
    brain: 'openrouter',
    timestamp: new Date().toISOString(),
    direction: 'inbound',
  })
}

async function openAndSend(h: Harness, rowText = 'chat-1'): Promise<void> {
  render(<Console fetcher={h.fetcher} feedVersion={0} coreState="running" />)
  fireEvent.click(await screen.findByText(rowText))
  const box = await screen.findByRole('textbox', { name: /message/i })
  fireEvent.change(box, { target: { value: 'resume el estado' } })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
  await screen.findByText(/está pensando/)
}

beforeEach(() => {
  localStorage.clear()
  resetFeedForTests()
  vi.useFakeTimers({ shouldAdvanceTime: true })
})
afterEach(() => vi.useRealTimers())

describe('estado 3 — el fallo corta la espera y se dice en pantalla', () => {
  it('handle_failed → banda sellada con [Reintentar]; la espera fuera', async () => {
    const h = makeHarness()
    await openAndSend(h)
    ingestFrame(failFrame())
    const band = await screen.findByRole('alert')
    expect(band.textContent).toContain(FAIL_TEXT)
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeTruthy()
    await waitFor(() => expect(screen.queryByText(/está pensando/)).toBeNull())
    // Primera vez: la segunda línea NO aparece.
    expect(screen.queryByText(REPEAT_TEXT)).toBeNull()
  })

  it('[Reintentar] cierra la banda y reenvía el MISMO texto por el camino normal', async () => {
    const h = makeHarness()
    await openAndSend(h)
    const sendsBefore = h.posts().filter((u) => u.endsWith('/message')).length
    ingestFrame(failFrame())
    await screen.findByRole('alert')
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))
    await waitFor(() =>
      expect(h.posts().filter((u) => u.endsWith('/message')).length).toBe(sendsBefore + 1),
    )
    expect(screen.queryByText(FAIL_TEXT)).toBeNull()
    await screen.findByText(/está pensando/) // la espera veraz vuelve
  })

  it('reincidencia: la segunda línea con los DOS arreglos (Builder / Secretos)', async () => {
    const h = makeHarness()
    await openAndSend(h)
    ingestFrame(failFrame())
    await screen.findByRole('alert')
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))
    await screen.findByText(/está pensando/)
    ingestFrame(failFrame())
    await screen.findByText(REPEAT_TEXT)
  })

  it('cambiar de conversación cierra la banda', async () => {
    const h = makeHarness()
    await openAndSend(h)
    ingestFrame(failFrame())
    await screen.findByRole('alert')
    fireEvent.click(screen.getByText('chat-2'))
    await waitFor(() => expect(screen.queryByText(FAIL_TEXT)).toBeNull())
  })
})

describe('estado 4 — el umbral de 60 s avisa, jamás corta solo', () => {
  it('a los 61 s: el aviso con [Seguir esperando] y [Cancelar y reintentar]', async () => {
    const h = makeHarness()
    await openAndSend(h)
    await act(async () => {
      vi.advanceTimersByTime(61_000)
    })
    await screen.findByText(WAIT_TEXT)
    expect(screen.getByRole('button', { name: 'Seguir esperando' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Cancelar y reintentar' })).toBeTruthy()
  })

  it('[Seguir esperando] cierra el aviso y NO re-avisa para esa petición', async () => {
    const h = makeHarness()
    await openAndSend(h)
    await act(async () => {
      vi.advanceTimersByTime(61_000)
    })
    await screen.findByText(WAIT_TEXT)
    fireEvent.click(screen.getByRole('button', { name: 'Seguir esperando' }))
    await waitFor(() => expect(screen.queryByText(WAIT_TEXT)).toBeNull())
    await act(async () => {
      vi.advanceTimersByTime(61_000)
    })
    expect(screen.queryByText(WAIT_TEXT)).toBeNull()
    // La espera sigue viva — nadie cortó nada.
    expect(screen.getByText(/sigue sin responder|está pensando/)).toBeTruthy()
  })

  it('[Cancelar y reintentar] reenvía; la respuesta TARDÍA de la cancelada se pinta (decisión 1)', async () => {
    const h = makeHarness()
    await openAndSend(h)
    await act(async () => {
      vi.advanceTimersByTime(61_000)
    })
    await screen.findByText(WAIT_TEXT)
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar y reintentar' }))
    await waitFor(() => expect(h.posts().filter((u) => u.endsWith('/message')).length).toBe(2))
    // La respuesta de la petición CANCELADA llega tarde al store…
    h.setTurns([
      { role: 'user', content: 'resume el estado', timestamp: '2026-08-29T09:01:00Z', seq: 0 },
      {
        role: 'assistant',
        content: 'la respuesta tardía',
        timestamp: '2026-08-29T09:02:00Z',
        seq: 1,
      },
    ])
    await act(async () => {
      vi.advanceTimersByTime(16_000) // el catch-up poll la trae
    })
    // …y SE PINTA como turno normal: el chat nunca oculta.
    await screen.findByText('la respuesta tardía')
  })
})

describe('el invariante — nada de estados de error se persiste (decisión 2)', () => {
  it('en todo el flujo, los únicos POST son /message y la banda jamás es un turno', async () => {
    const h = makeHarness()
    await openAndSend(h)
    ingestFrame(failFrame())
    await screen.findByRole('alert')
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }))
    await screen.findByText(/está pensando/)
    const nonMessagePosts = h.posts().filter((u) => !u.endsWith('/message'))
    expect(nonMessagePosts).toEqual([])
    // El texto de la banda no existe como contenido de turno.
    const turnTexts = Array.from(document.querySelectorAll('.console-turn-content')).map(
      (n) => n.textContent,
    )
    expect(turnTexts.some((t) => t?.includes('no pudo responder'))).toBe(false)
  })
})
