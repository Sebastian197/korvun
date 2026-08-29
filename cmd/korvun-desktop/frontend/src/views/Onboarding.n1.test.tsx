// N1 RED — the gateway branch of the onboarding's Modelo step (sealed
// design ola2-designs §3 + the lote-2 mandate shape). Pinned here: the two
// branch radios (ollama stays the default and untouched), the compat
// branch's four mold fields, the sealed «Comprobando…» disabled wait, the
// green line NAMING the validated model, on-screen failures with the fix
// at hand (missing model names the id; needsKey points at Secretos), the
// Siguiente gate on the ACTIVE branch, and the first-run rewrite riding
// Siguiente with the four fields.
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Onboarding } from './Onboarding'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}

function mountDesktop(bindings: Record<string, unknown>): void {
  ;(window as unknown as FakeWindow).go = { shell: { Desktop: bindings } }
}

afterEach(() => {
  delete (window as unknown as FakeWindow).go
})

const FOUND = { reachable: true, modelFound: true, needsKey: false, detail: 'model present' }
const MISSING = { reachable: true, modelFound: false, needsKey: false, detail: 'not in /models' }
const NEEDS_KEY = { reachable: true, modelFound: false, needsKey: true, detail: 'auth required' }

function renderStep1(bindings: Record<string, unknown>) {
  mountDesktop(bindings)
  return render(<Onboarding onFinished={() => undefined} />)
}

function switchToCompat(): void {
  fireEvent.click(screen.getByRole('radio', { name: /servidor compatible openai/i }))
}

describe('the two branches (AS-5 pin: ollama stays the default)', () => {
  it('renders both radios with ollama selected and its untouched flow', () => {
    renderStep1({})
    expect((screen.getByRole('radio', { name: /ollama/i }) as HTMLInputElement).checked).toBe(true)
    expect(
      (screen.getByRole('radio', { name: /servidor compatible openai/i }) as HTMLInputElement)
        .checked,
    ).toBe(false)
    expect(screen.getByRole('button', { name: 'Comprobar modelo' })).toBeInTheDocument()
  })

  it('the compat branch shows the four mold fields with the sealed default URL', () => {
    renderStep1({})
    switchToCompat()
    expect((screen.getByLabelText('base_url') as HTMLInputElement).value).toBe(
      'http://localhost:1234/v1',
    )
    expect(screen.getByLabelText('model_id')).toBeInTheDocument()
    expect(screen.getByLabelText('api_key_env')).toBeInTheDocument()
    expect(screen.getByLabelText('locality')).toBeInTheDocument()
  })
})

describe('the sealed check flow (AS-1, AS-4)', () => {
  it('«Comprobando…» disabled while in flight, then the green line NAMES the model', async () => {
    let resolve!: (v: typeof FOUND) => void
    const pending = new Promise<typeof FOUND>((r) => {
      resolve = r
    })
    const check = vi.fn(() => pending)
    renderStep1({ CheckCompatModel: check })
    switchToCompat()
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'qwen3-4b' } })
    fireEvent.click(screen.getByRole('button', { name: 'Comprobar' }))
    const checking = screen.getByRole('button', { name: 'Comprobando…' }) as HTMLButtonElement
    expect(checking.disabled).toBe(true)
    resolve(FOUND)
    const ok = await screen.findByText(/✓ .*qwen3-4b.*listo/)
    expect(ok).toBeInTheDocument()
    expect(check).toHaveBeenCalledWith('http://localhost:1234/v1', 'qwen3-4b', '')
    expect((screen.getByRole('button', { name: 'Siguiente' }) as HTMLButtonElement).disabled).toBe(
      false,
    )
  })

  it('Siguiente applies the first-run rewrite with the four fields (AS-1)', async () => {
    const apply = vi.fn(() => Promise.resolve())
    renderStep1({ CheckCompatModel: () => Promise.resolve(FOUND), ApplyCompatFirstRun: apply })
    switchToCompat()
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'qwen3-4b' } })
    fireEvent.change(screen.getByLabelText('api_key_env'), { target: { value: 'MY_API_KEY' } })
    fireEvent.change(screen.getByLabelText('locality'), { target: { value: 'cloud' } })
    fireEvent.click(screen.getByRole('button', { name: 'Comprobar' }))
    await screen.findByText(/listo/)
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente' }))
    await waitFor(() =>
      expect(apply).toHaveBeenCalledWith(
        'http://localhost:1234/v1',
        'qwen3-4b',
        'MY_API_KEY',
        'cloud',
      ),
    )
  })
})

describe('on-screen failures with the fix at hand (AS-2, AS-3)', () => {
  it('a missing model names the id and offers Reintentar; Siguiente stays gated', async () => {
    renderStep1({ CheckCompatModel: () => Promise.resolve(MISSING) })
    switchToCompat()
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'no-such' } })
    fireEvent.click(screen.getByRole('button', { name: 'Comprobar' }))
    const fail = await screen.findByRole('alert')
    expect(fail.textContent).toContain('no-such')
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
    expect((screen.getByRole('button', { name: 'Siguiente' }) as HTMLButtonElement).disabled).toBe(
      true,
    )
  })

  it('needsKey points at Ajustes → Secretos (the B10 bridge)', async () => {
    renderStep1({ CheckCompatModel: () => Promise.resolve(NEEDS_KEY) })
    switchToCompat()
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'qwen3-4b' } })
    fireEvent.click(screen.getByRole('button', { name: 'Comprobar' }))
    const fail = await screen.findByRole('alert')
    expect(fail.textContent).toMatch(/Secretos/)
  })

  it('switching branches discards the other result — the gate follows the ACTIVE branch', async () => {
    renderStep1({ CheckCompatModel: () => Promise.resolve(FOUND) })
    switchToCompat()
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'qwen3-4b' } })
    fireEvent.click(screen.getByRole('button', { name: 'Comprobar' }))
    await screen.findByText(/listo/)
    // Back to ollama: its check never ran → Siguiente gated again.
    fireEvent.click(screen.getByRole('radio', { name: /ollama/i }))
    expect((screen.getByRole('button', { name: 'Siguiente' }) as HTMLButtonElement).disabled).toBe(
      true,
    )
  })
})
