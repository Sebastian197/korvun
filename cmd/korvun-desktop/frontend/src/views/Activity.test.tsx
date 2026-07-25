// Actividad v1 (FR-WIN-4, ADR-0024 metadata-only): the live feed with
// type/channel filters, the En vivo indicator, and the designed empty state.
// Rows paint ONLY what the frame really carries.
import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { ingestFrame, resetFeedForTests } from '../feed/store'
import { resetIncidentForTests } from '../incident/store'
import { pollOnce } from '../status/store'
import { Activity } from './Activity'

function frame(
  type: string,
  extra: Record<string, string> = {},
  ts = '2026-07-25T14:32:08Z',
): string {
  return JSON.stringify({ type, timestamp: ts, ...extra })
}

async function seedStopped(): Promise<void> {
  await pollOnce(
    (() =>
      Promise.resolve(
        new Response(JSON.stringify({ error: 'core stopped' }), { status: 503 }),
      )) as typeof fetch,
  )
}

beforeEach(async () => {
  resetFeedForTests()
  await seedStopped()
  resetIncidentForTests()
})

describe('Actividad vacía', () => {
  it('paints the designed empty state', () => {
    render(<Activity />)
    expect(screen.getByText('Sin actividad todavía')).toBeInTheDocument()
    expect(screen.getByText(/aquí verás cada mensaje/)).toBeInTheDocument()
  })

  it('stopped and disconnected reads Pausado, honestly', () => {
    render(<Activity />)
    expect(screen.getByText('Pausado — gateway detenido')).toBeInTheDocument()
  })
})

describe('Actividad con frames', () => {
  beforeEach(() => {
    ingestFrame(
      frame('message_received', {
        channel: 'telegram',
        brain: 'asistente',
        envelope_id: 'e-0001',
        direction: 'inbound',
      }),
    )
    ingestFrame(
      frame('reply_sent', {
        channel: 'telegram',
        envelope_id: 'e-0002',
        direction: 'outbound',
      }),
    )
    ingestFrame(
      frame('message_dropped', {
        channel: 'discord',
        envelope_id: 'e-0003',
        direction: 'outbound',
      }),
    )
  })

  it('rows carry the metadata truth: hour, channel tile, event, id, direction', () => {
    render(<Activity />)
    expect(screen.getByText('Mensaje recibido')).toBeInTheDocument()
    expect(screen.getByText('Respuesta enviada')).toBeInTheDocument()
    expect(screen.getByText('Mensaje descartado')).toBeInTheDocument()
    expect(screen.getAllByText('TG').length).toBe(2)
    expect(screen.getByText('DC')).toBeInTheDocument()
    expect(screen.getByText('e-0001')).toBeInTheDocument()
    expect(screen.getAllByText('in').length).toBe(1)
    expect(screen.getAllByText('out').length).toBe(2)
  })

  it('the brain rides the row as the design-idiom chip', () => {
    render(<Activity />)
    expect(screen.getByText('→ asistente')).toBeInTheDocument()
  })

  it('filters by channel', () => {
    render(<Activity />)
    fireEvent.click(screen.getByRole('button', { name: 'discord' }))
    expect(screen.queryByText('Mensaje recibido')).toBeNull()
    expect(screen.getByText('Mensaje descartado')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Todos' }))
    expect(screen.getByText('Mensaje recibido')).toBeInTheDocument()
  })

  it('filters by type', () => {
    render(<Activity />)
    fireEvent.click(screen.getByRole('button', { name: 'Descartados' }))
    expect(screen.queryByText('Respuesta enviada')).toBeNull()
    expect(screen.getByText('Mensaje descartado')).toBeInTheDocument()
  })
})