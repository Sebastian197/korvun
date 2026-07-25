// "Iniciar con la aplicación" (FR-WIN-5): localStorage-driven auto-START of
// the CORE when the window opens — never an OS login item (ADR-0035 §8).
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { isAutostartEnabled, runAutostart, setAutostart } from './autostart'

interface FakeWindow {
  go?: { shell?: { Desktop?: unknown } }
}

beforeEach(() => {
  localStorage.clear()
  delete (window as unknown as FakeWindow).go
})

describe('autostart', () => {
  it('defaults to off and persists the choice', () => {
    expect(isAutostartEnabled()).toBe(false)
    setAutostart(true)
    expect(isAutostartEnabled()).toBe(true)
    expect(localStorage.getItem('korvun.chrome.autostart')).toBe('true')
  })

  it('runAutostart starts the core only when enabled and bound', async () => {
    const start = vi.fn(() => Promise.resolve())
    ;(window as unknown as FakeWindow).go = {
      shell: { Desktop: { Start: start } },
    }
    await runAutostart()
    expect(start).not.toHaveBeenCalled()
    setAutostart(true)
    await runAutostart()
    expect(start).toHaveBeenCalledTimes(1)
  })

  it('a Start rejection is swallowed — the status poll reconciles the truth', async () => {
    setAutostart(true)
    ;(window as unknown as FakeWindow).go = {
      shell: {
        Desktop: { Start: () => Promise.reject(new Error('already running')) },
      },
    }
    await expect(runAutostart()).resolves.toBeUndefined()
  })
})
