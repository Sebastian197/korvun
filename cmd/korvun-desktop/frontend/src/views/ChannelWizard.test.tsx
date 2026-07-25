// The 3-step channel assistant (SP6c, contra chica-18/19/20 + final-4),
// mirroring config.Validate as it is: type (one channel per type; occupied
// types disabled with an honest note) → env-var NAME → keychain (masked
// once-only field → SetSecret; "Comprobar entorno" via CheckSecretPresence,
// presence never value) → POST + reload. The secret value never appears in a
// DOM node, request URL, or storage after submit.
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ChannelWizard } from './ChannelWizard'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}

const CONFIG = {
  channels: [{ type: 'telegram', mode: 'polling', token_env: 'TELEGRAM_TOKEN' }],
  brains: [{ name: 'asistente' }],
  routes: [{ channel: 'telegram', brain: 'asistente' }],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}

function pipeFetch(capture: { posted?: unknown }): typeof fetch {
  return ((url: string, init?: RequestInit) => {
    const u = String(url)
    if (u === '/api/config' && (init?.method ?? 'GET') === 'GET') {
      return Promise.resolve(new Response(JSON.stringify(CONFIG), { status: 200 }))
    }
    if (u === '/api/config') {
      capture.posted = JSON.parse(String(init?.body))
      return Promise.resolve(new Response(JSON.stringify({ handle: 'h1' }), { status: 202 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ state: 'succeeded' }), { status: 200 }))
  }) as unknown as typeof fetch
}

beforeEach(() => {
  delete (window as unknown as FakeWindow).go
  localStorage.clear()
})

describe('ChannelWizard step 1 — type', () => {
  it('a type already configured is DISABLED with an honest note', () => {
    render(
      <ChannelWizard
        existingTypes={['telegram']}
        onClose={() => undefined}
        onDone={() => undefined}
      />,
    )
    const tg = screen.getByRole('button', { name: /Telegram/ })
    expect(tg).toBeDisabled()
    expect(screen.getByText(/ya está conectado/)).toBeInTheDocument()
    // Discord is free.
    expect(screen.getByRole('button', { name: /Discord/ })).toBeEnabled()
  })
})

describe('ChannelWizard step 3 — keychain', () => {
  function advanceToStep3(): void {
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
  }

  it('Atrás steps back through the flow', () => {
    render(<ChannelWizard existingTypes={[]} onClose={() => undefined} onDone={() => undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    expect(screen.getByText(/El bot, por su nombre/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Atrás/ }))
    expect(screen.getByText(/¿Qué canal quieres conectar/)).toBeInTheDocument()
  })

  it('with no bindings the keychain step is HONEST (no fake success)', () => {
    render(<ChannelWizard existingTypes={[]} onClose={() => undefined} onDone={() => undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    fireEvent.change(screen.getByPlaceholderText(/una sola vez/), { target: { value: 'v' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar en el llavero/ }))
    // No bindings → never claim it was stored (security review finding).
    expect(screen.queryByText(/guardado en el llavero/)).toBeNull()
    expect(screen.getByRole('alert')).toHaveTextContent(/Sin llavero en este entorno/)
  })

  it('a rejected SetSecret paints a GENERIC error and lets the field be retried', async () => {
    ;(window as unknown as FakeWindow).go = {
      shell: {
        Desktop: { SetSecret: () => Promise.reject(new Error('keychain locked: /Users/x')) },
      },
    }
    render(
      <ChannelWizard
        existingTypes={['telegram']}
        onClose={() => undefined}
        onDone={() => undefined}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    const secret = 'top-secret-token'
    fireEvent.change(screen.getByPlaceholderText(/una sola vez/), { target: { value: secret } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar en el llavero/ }))
    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/No se pudo guardar en el llavero/)
    })
    // The raw backend error is NEVER shown, and the field is editable again.
    expect(screen.queryByText(/keychain locked/)).toBeNull()
    expect(screen.getByPlaceholderText(/una sola vez/)).toBeInTheDocument()
  })

  it('SetSecret is called with the pasted value; the value leaves no DOM/storage trace', async () => {
    const setSecret = vi.fn(() => Promise.resolve())
    ;(window as unknown as FakeWindow).go = { shell: { Desktop: { SetSecret: setSecret } } }
    render(
      <ChannelWizard
        existingTypes={['telegram']}
        onClose={() => undefined}
        onDone={() => undefined}
      />,
    )
    advanceToStep3()

    const secret = 'super-secret-bot-token-value'
    const field = screen.getByPlaceholderText(/una sola vez/)
    fireEvent.change(field, { target: { value: secret } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar en el llavero/ }))

    await waitFor(() => {
      expect(setSecret).toHaveBeenCalledWith('DISCORD_BOT_TOKEN', secret)
    })
    await screen.findByText(/guardado en el llavero/)
    // The value is nowhere in the DOM after saving, and never in localStorage.
    expect(document.body.innerHTML).not.toContain(secret)
    expect(JSON.stringify(localStorage)).not.toContain(secret)
  })

  it('Comprobar entorno uses CheckSecretPresence (presence, never value)', async () => {
    const check = vi.fn(() => Promise.resolve({ inEnv: true, inKeychain: false }))
    ;(window as unknown as FakeWindow).go = { shell: { Desktop: { CheckSecretPresence: check } } }
    render(
      <ChannelWizard
        existingTypes={['telegram']}
        onClose={() => undefined}
        onDone={() => undefined}
      />,
    )
    advanceToStep3()
    fireEvent.click(screen.getByRole('button', { name: /Comprobar entorno/ }))
    await waitFor(() => {
      expect(check).toHaveBeenCalledWith('DISCORD_BOT_TOKEN')
    })
    expect(await screen.findByText(/detectada/)).toBeInTheDocument()
  })

  it('a failed reload paints the error and does not report done', async () => {
    ;(window as unknown as FakeWindow).go = {
      shell: { Desktop: { SetSecret: () => Promise.resolve() } },
    }
    const failFetch = ((url: string, init?: RequestInit) => {
      const u = String(url)
      if (u === '/api/config' && (init?.method ?? 'GET') === 'GET') {
        return Promise.resolve(new Response(JSON.stringify(CONFIG), { status: 200 }))
      }
      if (u === '/api/config') {
        return Promise.resolve(new Response(JSON.stringify({ handle: 'h1' }), { status: 202 }))
      }
      return Promise.resolve(new Response(JSON.stringify({ state: 'failed' }), { status: 200 }))
    }) as unknown as typeof fetch
    const onDone = vi.fn()
    render(
      <ChannelWizard
        existingTypes={['telegram']}
        onClose={() => undefined}
        onDone={onDone}
        fetcher={failFetch}
        pollIntervalMs={0}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    fireEvent.click(screen.getByRole('button', { name: /Siguiente/ }))
    fireEvent.change(screen.getByPlaceholderText(/una sola vez/), { target: { value: 'v' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar en el llavero/ }))
    await screen.findByText(/guardado en el llavero/)
    fireEvent.click(screen.getByRole('button', { name: /Conectar canal/ }))
    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/No se pudo conectar/)
    })
    expect(onDone).not.toHaveBeenCalled()
  })

  it('completing the wizard POSTs the new channel and reports done', async () => {
    const cap: { posted?: unknown } = {}
    ;(window as unknown as FakeWindow).go = {
      shell: { Desktop: { SetSecret: () => Promise.resolve() } },
    }
    const onDone = vi.fn()
    render(
      <ChannelWizard
        existingTypes={['telegram']}
        onClose={() => undefined}
        onDone={onDone}
        fetcher={pipeFetch(cap)}
        pollIntervalMs={0}
      />,
    )
    advanceToStep3()
    fireEvent.change(screen.getByPlaceholderText(/una sola vez/), {
      target: { value: 'v' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Guardar en el llavero/ }))
    await screen.findByText(/guardado en el llavero/)
    fireEvent.click(screen.getByRole('button', { name: /Conectar canal|Finalizar|Añadir/ }))
    await waitFor(() => {
      expect(onDone).toHaveBeenCalled()
    })
    const posted = cap.posted as { channels: Array<{ type: string }> }
    expect(posted.channels.map((c) => c.type)).toContain('discord')
  })
})
