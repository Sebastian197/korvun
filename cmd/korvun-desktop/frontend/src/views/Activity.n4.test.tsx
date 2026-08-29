// N4 RED (sealed design ola2-designs §4, APROBADO POR CHANO 2026-08-23):
// Actividad declares its volatility. Sealed decisions pinned here: the
// declaration lives in a STATE CHIP next to the view title (thin-border
// pill, green dot with a soft pulse, no animation under
// prefers-reduced-motion); the empty state is a composition whose second
// line says the feed lives in this window; the honest non-live states are
// preserved inside the chip.
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { resetFeedForTests, setLiveForTests } from '../feed/store'
import { resetIncidentForTests } from '../incident/store'
import { pollOnce } from '../status/store'
import { Activity, ActivityLiveChip } from './Activity'

async function seedCore(status: number, body: unknown): Promise<void> {
  await pollOnce((() =>
    Promise.resolve(
      new Response(JSON.stringify(body), { status }),
    )) as typeof fetch)
}

beforeEach(async () => {
  resetFeedForTests()
  await seedCore(503, { error: 'core stopped' })
  resetIncidentForTests()
})

describe('the state chip (sealed: pill + pulsing dot next to the title)', () => {
  it('live: the pill declares «En vivo · desde que se abrió la ventana» with the pulsing dot', () => {
    setLiveForTests(true)
    render(<ActivityLiveChip />)
    const chip = screen.getByTestId('act-live-chip')
    expect(chip.textContent).toContain('En vivo · desde que se abrió la ventana')
    expect(chip.querySelector('.chip-dot-pulse')).not.toBeNull()
  })

  it('stopped: the chip keeps the honest Pausado state, dot unpulsed', () => {
    render(<ActivityLiveChip />)
    const chip = screen.getByTestId('act-live-chip')
    expect(chip.textContent).toContain('Pausado — gateway detenido')
    expect(chip.querySelector('.chip-dot-pulse')).toBeNull()
  })

  it('running but SSE not yet open: Conectando…', async () => {
    await seedCore(200, { state: 'running' })
    render(<ActivityLiveChip />)
    expect(screen.getByTestId('act-live-chip').textContent).toContain('Conectando…')
  })
})

describe('the view defers to the header chip', () => {
  it('the filter row no longer carries its own live text', () => {
    setLiveForTests(true)
    render(<Activity />)
    expect(screen.queryByText(/en vivo/i)).toBeNull()
  })
})

describe('the empty state declares the volatility (sealed second line)', () => {
  it('fresh start: icon + title + the sealed line, privacy included', () => {
    render(<Activity />)
    expect(screen.getByText('Sin actividad todavía')).toBeTruthy()
    expect(
      screen.getByText(
        'El feed vive en esta ventana — al cerrarla se vacía. Solo metadatos, nunca el contenido.',
      ),
    ).toBeTruthy()
  })
})

describe('reduced motion (sealed: no animation under the media query)', () => {
  it('App.css silences the pulse under prefers-reduced-motion', () => {
    const css = readFileSync(join(__dirname, '..', 'App.css'), 'utf-8')
    const idx = css.indexOf('@media (prefers-reduced-motion: reduce)')
    expect(idx, 'a prefers-reduced-motion block must exist').toBeGreaterThan(-1)
    expect(css.slice(idx, idx + 400)).toContain('chip-dot-pulse')
  })
})
