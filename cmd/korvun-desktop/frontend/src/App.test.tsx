// Shell smoke: navigation renders every section in the design order with
// real icons, the brand tile carries the canonical mark, and the sidebar
// foot carries the status chip.
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { App } from './App'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}

afterEach(() => {
  delete (window as unknown as FakeWindow).go
})

describe('chrome shell', () => {
  it('renders the sidebar with all five sections and the version row', () => {
    render(<App />)
    for (const label of ['Inicio', 'Canales', 'Actividad', 'Builder', 'Ajustes']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
    expect(screen.getByTestId('version').textContent).toBe('dev')
  })

  it('nav follows the design order: Inicio, Builder, Canales, Actividad, Ajustes', () => {
    render(<App />)
    const nav = screen.getByRole('navigation', { name: 'Secciones' })
    const labels = Array.from(nav.querySelectorAll('button')).map((b) =>
      (b.textContent ?? '').trim(),
    )
    expect(labels).toEqual(['Inicio', 'Builder', 'Canales', 'Actividad', 'Ajustes'])
  })

  it('every nav item carries its real design icon (svg), not a placeholder dot', () => {
    render(<App />)
    const nav = screen.getByRole('navigation', { name: 'Secciones' })
    for (const button of Array.from(nav.querySelectorAll('button'))) {
      expect(button.querySelector('svg'), `${button.textContent} lacks an icon`).not.toBeNull()
      expect(button.querySelector('.nav-dot')).toBeNull()
    }
  })

  it('the brand tile renders the canonical K mark with the identity gradient', () => {
    render(<App />)
    const tile = screen.getByTestId('brand-tile')
    const svg = tile.querySelector('svg')
    expect(svg).not.toBeNull()
    const stops = Array.from(svg?.querySelectorAll('stop') ?? []).map((s) =>
      s.getAttribute('stop-color'),
    )
    expect(stops).toEqual(['#2BC8B7', '#7A5AF5'])
  })

  it('the stopped Home shows the hero with the one primary action', () => {
    render(<App />)
    expect(screen.getByTestId('home-parado')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Iniciar/ })).toBeInTheDocument()
  })

  it('the sidebar foot carries the status chip and the header the healthz badge', () => {
    render(<App />)
    expect(screen.getByTestId('status-chip')).toBeInTheDocument()
    expect(screen.getByTestId('healthz-badge')).toBeInTheDocument()
  })

  it('navigating to Canales renders the real channels view', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'Canales' }))
    expect(screen.getByTestId('canales')).toBeInTheDocument()
  })

  it('Añadir canal opens the wizard overlay from Canales', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'Canales' }))
    fireEvent.click(screen.getByRole('button', { name: /Añadir canal/ }))
    expect(screen.getByRole('dialog', { name: 'Añadir un canal' })).toBeInTheDocument()
  })

  it('a fresh install (EnsureDefaultConfig created=true) replaces the chrome with onboarding', async () => {
    ;(window as unknown as FakeWindow).go = {
      shell: {
        Desktop: {
          Version: () => Promise.resolve('v0.4.0'),
          EnsureDefaultConfig: () => Promise.resolve(true),
        },
      },
    }
    render(<App />)
    await waitFor(() => {
      expect(screen.getByTestId('onboarding')).toBeInTheDocument()
    })
    // The dashboard nav is not shown during onboarding.
    expect(screen.queryByRole('navigation', { name: 'Secciones' })).toBeNull()
  })

  it('an existing install (created=false) shows the normal chrome, no onboarding', async () => {
    ;(window as unknown as FakeWindow).go = {
      shell: {
        Desktop: {
          Version: () => Promise.resolve('v0.4.0'),
          EnsureDefaultConfig: () => Promise.resolve(false),
        },
      },
    }
    render(<App />)
    await waitFor(() => {
      expect(screen.getByTestId('version').textContent).toBe('v0.4.0')
    })
    expect(screen.queryByTestId('onboarding')).toBeNull()
    expect(screen.getByRole('navigation', { name: 'Secciones' })).toBeInTheDocument()
  })
})
