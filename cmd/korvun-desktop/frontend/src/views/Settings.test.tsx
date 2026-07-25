// Ajustes v1 (FR-WIN-5, contra final-3): theme choice persisted in
// localStorage, REAL info rows from the bindings' Status, the autostart
// toggle, and the security row that paints only the env-var NAME. What has
// no API behind it is NOT rendered (a dead button is a lie).
import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { pollShellOnce, resetShellForTests } from '../status/shell'
import { pollOnce } from '../status/store'
import { Settings } from './Settings'

interface FakeWindow {
  go?: { shell?: { Desktop?: unknown } }
}

function installBindings(status: {
  Running: boolean
  ConfigPath: string
  AdminAddr: string
  TokenEnv: string
}): void {
  ;(window as unknown as FakeWindow).go = {
    shell: { Desktop: { Status: () => Promise.resolve(status) } },
  }
}

const RUNNING = {
  Running: true,
  ConfigPath: '/Users/chano/Library/Application Support/korvun/korvun.json',
  AdminAddr: '127.0.0.1:52814',
  TokenEnv: 'KORVUN_ADMIN_TOKEN',
}

async function seedRunning(): Promise<void> {
  installBindings(RUNNING)
  await pollShellOnce()
  await pollOnce((() => Promise.resolve(new Response('ok'))) as typeof fetch)
}

async function seedStopped(): Promise<void> {
  installBindings({ ...RUNNING, Running: false, AdminAddr: '' })
  await pollShellOnce()
  await pollOnce((() =>
    Promise.resolve(
      new Response(JSON.stringify({ error: 'core stopped' }), { status: 503 }),
    )) as typeof fetch)
}

beforeEach(() => {
  resetShellForTests()
  localStorage.clear()
  document.documentElement.dataset.theme = 'dark'
  delete (window as unknown as FakeWindow).go
})

describe('Ajustes · apariencia', () => {
  it('offers Oscuro/Claro/Sistema and persists the choice', () => {
    render(<Settings version="dev" />)
    fireEvent.click(screen.getByRole('button', { name: 'Claro' }))
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(localStorage.getItem('korvun.chrome.theme')).toBe('light')
    fireEvent.click(screen.getByRole('button', { name: 'Sistema' }))
    expect(localStorage.getItem('korvun.chrome.theme')).toBe('system')
  })
})

describe('Ajustes · gateway', () => {
  it('paints the real config path and the effective admin address', async () => {
    await seedRunning()
    render(<Settings version="dev" />)
    expect(screen.getByText(RUNNING.ConfigPath)).toBeInTheDocument()
    expect(screen.getByText(/127\.0\.0\.1:52814/)).toBeInTheDocument()
    expect(screen.getByText(/asignado al arrancar/)).toBeInTheDocument()
  })

  it('Copiar writes the effective address and acknowledges', async () => {
    await seedRunning()
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    render(<Settings version="dev" />)
    fireEvent.click(screen.getByRole('button', { name: /Copiar/ }))
    expect(writeText).toHaveBeenCalledWith('127.0.0.1:52814')
    expect(await screen.findByText('Copiado')).toBeInTheDocument()
  })

  it('stopped: the address row is honest and Copiar is disabled', async () => {
    await seedStopped()
    render(<Settings version="dev" />)
    expect(screen.getByText(/se asigna al arrancar/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Copiar/ })).toBeDisabled()
  })

  it('the autostart toggle persists korvun.chrome.autostart', () => {
    render(<Settings version="dev" />)
    const toggle = screen.getByRole('switch', {
      name: /Iniciar con la aplicación/,
    })
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(localStorage.getItem('korvun.chrome.autostart')).toBe('true')
  })
})

describe('Ajustes · seguridad y honestidad', () => {
  it('the token row paints the env-var NAME and the rotation pill', async () => {
    await seedRunning()
    render(<Settings version="dev" />)
    expect(screen.getByText(/KORVUN_ADMIN_TOKEN/)).toBeInTheDocument()
    expect(screen.getByText('automático · se rota al arrancar')).toBeInTheDocument()
  })

  it('rows without an API are NOT rendered', () => {
    render(<Settings version="dev" />)
    expect(screen.queryByText(/Vaciar memoria/)).toBeNull()
    expect(screen.queryByText(/Mostrar en carpeta/)).toBeNull()
    expect(screen.queryByText(/Memoria de conversaciones/)).toBeNull()
  })

  it('the footer states the version line', () => {
    render(<Settings version="v0.4.0" />)
    expect(screen.getByText(/Korvun v0\.4\.0 · un solo binario · Apache-2\.0/)).toBeInTheDocument()
  })
})
