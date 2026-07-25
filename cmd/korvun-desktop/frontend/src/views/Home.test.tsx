// Home's three states (FR-WIN-6) against seeded stores — the same stores the
// window feeds from, no view-level mocks.
import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { ingestFrame, resetFeedForTests } from '../feed/store'
import {
  getIncident,
  notifyCoreTransition,
  resetIncidentForTests,
} from '../incident/store'
import { pollSnapshotOnce, resetSnapshotForTests } from '../snapshot/store'
import { pollOnce } from '../status/store'
import { Home } from './Home'

function healthz(status: number, body?: unknown): typeof fetch {
  return (() =>
    Promise.resolve(
      new Response(body === undefined ? 'ok' : JSON.stringify(body), { status }),
    )) as typeof fetch
}

async function seedCore(state: 'running' | 'stopped'): Promise<void> {
  if (state === 'running') await pollOnce(healthz(200))
  else await pollOnce(healthz(503, { error: 'core stopped' }))
}

const SNAPSHOT_FETCH = ((url: string) => {
  const brains = [
    {
      name: 'asistente',
      sensitivity: 'private',
      policy: 'priority',
      dispatch: 'fanout',
      models: [{ provider: 'ollama', model_id: 'llama3.2:1b' }],
    },
  ]
  const channels = [{ type: 'telegram', mode: 'polling', name: 'telegram', dropped: 0 }]
  return Promise.resolve(
    new Response(JSON.stringify(String(url).includes('brains') ? brains : channels), {
      status: 200,
    }),
  )
}) as unknown as typeof fetch

function frame(type: string, extra: Record<string, string> = {}): string {
  return JSON.stringify({ type, timestamp: new Date().toISOString(), ...extra })
}

beforeEach(async () => {
  resetFeedForTests()
  resetIncidentForTests()
  resetSnapshotForTests()
  await seedCore('stopped')
  resetIncidentForTests() // the seeding transition itself is not under test
})

describe('Home parado', () => {
  it('paints the stopped hero with the one gradient Start action', () => {
    render(<Home />)
    expect(screen.getByText('El gateway está detenido')).toBeInTheDocument()
    expect(
      screen.getByText(/no reciben ni responden mensajes mientras esté parado/),
    ).toBeInTheDocument()
    const start = screen.getByRole('button', { name: /Iniciar/ })
    expect(start.className).toContain('btn-primary')
  })

  it('with last-session data, paints it dimmed and stale', async () => {
    await seedCore('running')
    await pollSnapshotOnce(SNAPSHOT_FETCH)
    await seedCore('stopped')
    resetIncidentForTests()
    render(<Home />)
    expect(screen.getByTestId('home-stale-data')).toBeInTheDocument()
    expect(screen.getByText('Telegram')).toBeInTheDocument()
    expect(screen.getAllByText('Detenido').length).toBeGreaterThan(0)
  })
})

describe('Home marcha', () => {
  beforeEach(async () => {
    await seedCore('running')
    await pollSnapshotOnce(SNAPSHOT_FETCH)
  })

  it('paints the running hero with real channel/brain counts', () => {
    render(<Home />)
    expect(screen.getByText('El gateway está en marcha')).toBeInTheDocument()
    expect(screen.getByText(/Sirviendo 1 canal y 1 cerebro/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Detener/ })).toBeInTheDocument()
  })

  it('cards derive from the feed and say the window-scoped truth', () => {
    ingestFrame(frame('message_received', { channel: 'telegram' }))
    ingestFrame(frame('reply_sent', { channel: 'telegram' }))
    ingestFrame(frame('reply_sent', { channel: 'telegram' }))
    render(<Home />)
    expect(screen.getByTestId('card-recibidos')).toHaveTextContent('1')
    expect(screen.getByTestId('card-procesados')).toHaveTextContent('2')
    expect(screen.getByTestId('card-descartados')).toHaveTextContent('0')
    expect(
      screen.getAllByText(/desde que se abrió la ventana/).length,
    ).toBeGreaterThanOrEqual(3)
  })

  it('panels list channels and brains from the control API', () => {
    render(<Home />)
    expect(screen.getByText('Telegram')).toBeInTheDocument()
    expect(screen.getByText('Operativo')).toBeInTheDocument()
    expect(screen.getByText('asistente')).toBeInTheDocument()
    expect(screen.getByText('Privado')).toBeInTheDocument()
    expect(screen.getByText('llama3.2:1b')).toBeInTheDocument()
  })

  it('a failure frame flips the hero to the honest incident banner', () => {
    ingestFrame(frame('message_dropped', { channel: 'telegram' }))
    render(<Home />)
    expect(screen.getByText('En marcha — incidencia')).toBeInTheDocument()
    expect(screen.getByText(/Mensaje descartado en telegram/)).toBeInTheDocument()
  })

  it('Detener marks the stop as user-intended (no reap incident afterwards)', () => {
    render(<Home />)
    fireEvent.click(screen.getByRole('button', { name: /Detener/ }))
    notifyCoreTransition('running', 'stopped')
    expect(getIncident()).toBeNull()
  })
})

describe('Home incidencia por salida inesperada', () => {
  it('a reap paints the red banner without inventing a cause', async () => {
    await seedCore('running')
    notifyCoreTransition('running', 'stopped')
    await seedCore('stopped')
    render(<Home />)
    expect(screen.getByText('El núcleo se detuvo inesperadamente')).toBeInTheDocument()
    expect(screen.getByText(/el motivo queda en el registro del shell/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Iniciar/ })).toBeInTheDocument()
  })
})