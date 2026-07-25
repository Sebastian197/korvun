// First-run onboarding (SP6c, contra recien-instalado): 3 steps —
// CheckOllama honest result → first channel (reuses the wizard) → Start. On
// completion the chrome shows normal Home.
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Onboarding } from './Onboarding'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}

beforeEach(() => {
  delete (window as unknown as FakeWindow).go
})

describe('Onboarding', () => {
  it('step 1: CheckOllama reachable paints the honest ready state', async () => {
    const check = vi.fn(() => Promise.resolve({ reachable: true, detail: 'ollama is responding' }))
    ;(window as unknown as FakeWindow).go = { shell: { Desktop: { CheckOllama: check } } }
    render(<Onboarding onFinished={() => undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /Comprobar modelo/ }))
    await waitFor(() => {
      expect(check).toHaveBeenCalled()
    })
    expect(await screen.findByText(/ollama accesible · .* — listo/)).toBeInTheDocument()
  })

  it('step 1: an unreachable Ollama paints the honest failure, never a fake success', async () => {
    const check = vi.fn(() => Promise.resolve({ reachable: false, detail: 'not reachable' }))
    ;(window as unknown as FakeWindow).go = { shell: { Desktop: { CheckOllama: check } } }
    render(<Onboarding onFinished={() => undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /Comprobar modelo/ }))
    expect(await screen.findByText(/Ollama no responde/)).toBeInTheDocument()
    // The step-2 gate stays closed on failure (Siguiente disabled).
    expect(screen.getByRole('button', { name: /Siguiente/ })).toBeDisabled()
  })

  it('step 3: Start finishes onboarding', async () => {
    const start = vi.fn(() => Promise.resolve())
    ;(window as unknown as FakeWindow).go = {
      shell: {
        Desktop: {
          CheckOllama: () => Promise.resolve({ reachable: true, detail: 'ok' }),
          Start: start,
        },
      },
    }
    const onFinished = vi.fn()
    render(<Onboarding onFinished={onFinished} initialStep={3} />)
    fireEvent.click(screen.getByRole('button', { name: /Arrancar Korvun/ }))
    await waitFor(() => {
      expect(start).toHaveBeenCalled()
      expect(onFinished).toHaveBeenCalled()
    })
  })
})
