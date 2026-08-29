// B10 RED — the SECRETOS card (sealed design ola2-designs §2). Pinned here:
// discovery from the live config (proxy when running, the name-only binding
// when stopped), row skeleton while loading, presence WITHOUT values (env
// wins over keychain and the row says so), write-only in-row editing (the
// field empties the instant SetSecret fires), confirmed delete, the sealed
// keychain-error text, the sealed unreadable-config notice with
// [Abrir carpeta], the sealed empty state — and the NEGATIVE invariant: a
// typed value exists NOWHERE after saving.
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SecretsCard } from './SecretsCard'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}

function mountDesktop(bindings: Record<string, unknown>): void {
  ;(window as unknown as FakeWindow).go = { shell: { Desktop: bindings } }
}

afterEach(() => {
  delete (window as unknown as FakeWindow).go
})

beforeEach(() => localStorage.clear())

const NAMES = ['KORVUN_TG_TOKEN', 'OPENROUTER_API_KEY']

function presenceOf(map: Record<string, { inEnv: boolean; inKeychain: boolean }>) {
  return (name: string) => {
    const p = map[name]
    return p !== undefined ? Promise.resolve(p) : Promise.reject(new Error('not a secret name'))
  }
}

function stoppedCard(bindings: Record<string, unknown>) {
  mountDesktop(bindings)
  return render(<SecretsCard coreState="stopped" />)
}

describe('discovery (AS-1)', () => {
  it('core stopped: rows come from ListSecretNames, after a skeleton', async () => {
    let resolve!: (v: string[]) => void
    const listing = new Promise<string[]>((r) => {
      resolve = r
    })
    stoppedCard({
      ListSecretNames: () => listing,
      CheckSecretPresence: presenceOf({}),
    })
    expect(screen.getByTestId('secrets-skeleton')).toBeInTheDocument()
    resolve(NAMES)
    expect(await screen.findByText('KORVUN_TG_TOKEN')).toBeInTheDocument()
    expect(screen.getByText('OPENROUTER_API_KEY')).toBeInTheDocument()
    expect(screen.queryByTestId('secrets-skeleton')).not.toBeInTheDocument()
  })

  it('core running: rows come from /api/config through the proxy', async () => {
    mountDesktop({ CheckSecretPresence: presenceOf({}) })
    const fetcher = (async () =>
      new Response(
        JSON.stringify({
          channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG_TOKEN' }],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )) as typeof fetch
    render(<SecretsCard coreState="running" fetcher={fetcher} />)
    expect(await screen.findByText('KORVUN_TG_TOKEN')).toBeInTheDocument()
  })
})

describe('presence (AS-3)', () => {
  it('keychain-only, env-wins, absent and uncheckable states render from booleans', async () => {
    stoppedCard({
      ListSecretNames: () =>
        Promise.resolve(['KEYCHAIN_ONLY_TOKEN', 'BOTH_PLACES_KEY', 'ABSENT_TOKEN', 'WEIRD_NAME']),
      CheckSecretPresence: presenceOf({
        KEYCHAIN_ONLY_TOKEN: { inEnv: false, inKeychain: true },
        BOTH_PLACES_KEY: { inEnv: true, inKeychain: true },
        ABSENT_TOKEN: { inEnv: false, inKeychain: false },
      }),
    })
    const rowOf = async (name: string) =>
      (await screen.findByText(name)).closest('[data-testid="secret-row"]') as HTMLElement
    // findByText (async) throughout: each row's presence resolves on its own
    // CheckSecretPresence promise — the CI runner exposed the missing awaits.
    expect(
      await within(await rowOf('KEYCHAIN_ONLY_TOKEN')).findByText(/en el llavero/),
    ).toBeTruthy()
    // The wizard nuance, sealed: the environment WINS over the keychain.
    expect(await within(await rowOf('BOTH_PLACES_KEY')).findByText(/entorno.*gana/i)).toBeTruthy()
    expect(await within(await rowOf('ABSENT_TOKEN')).findByText(/ausente/)).toBeTruthy()
    expect(await within(await rowOf('WEIRD_NAME')).findByText(/no comprobable/)).toBeTruthy()
  })
})

describe('write-only editing (AS-2) and the negative invariant (AS-6)', () => {
  it('saving calls SetSecret(name, value), empties the field INSTANTLY, and the value exists nowhere', async () => {
    const setSecret = vi.fn(() => Promise.resolve())
    const check = vi.fn(presenceOf({ OPENROUTER_API_KEY: { inEnv: false, inKeychain: true } }))
    stoppedCard({
      ListSecretNames: () => Promise.resolve(['OPENROUTER_API_KEY']),
      CheckSecretPresence: check,
      SetSecret: setSecret,
    })
    fireEvent.click(await screen.findByRole('button', { name: /actualizar/i }))
    const field = screen.getByLabelText(/valor/i) as HTMLInputElement
    expect(field.type).toBe('password')
    fireEvent.change(field, { target: { value: 's3cr3t-valor' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))
    await waitFor(() =>
      expect(setSecret).toHaveBeenCalledWith('OPENROUTER_API_KEY', 's3cr3t-valor'),
    )
    // The wizard discipline: the field empties the instant of the call.
    expect((screen.queryByLabelText(/valor/i) as HTMLInputElement | null)?.value ?? '').toBe('')
    // THE invariant: the value exists nowhere in the document.
    expect(document.body.innerHTML).not.toContain('s3cr3t-valor')
    // And no read binding ever saw it.
    for (const call of check.mock.calls) {
      expect(call[0]).not.toContain('s3cr3t')
    }
  })

  it('a keychain failure shows the SEALED error text and offers Reintentar', async () => {
    stoppedCard({
      ListSecretNames: () => Promise.resolve(['OPENROUTER_API_KEY']),
      CheckSecretPresence: presenceOf({ OPENROUTER_API_KEY: { inEnv: false, inKeychain: false } }),
      SetSecret: () => Promise.reject(new Error('keychain: item locked')),
    })
    fireEvent.click(await screen.findByRole('button', { name: /guardar…/i }))
    fireEvent.change(screen.getByLabelText(/valor/i), { target: { value: 'v' } })
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }))
    const err = await screen.findByRole('alert')
    expect(err.textContent).toContain(
      'El llavero del sistema rechazó la operación. Reintenta; si persiste, desbloquéalo en Acceso a Llaveros.',
    )
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
    // The raw Go error (which could theoretically carry paths) stays out.
    expect(err.textContent).not.toContain('keychain: item locked')
  })
})

describe('delete with confirmation (AS-4)', () => {
  it('asks in-row; cancel touches nothing; confirm calls DeleteSecret', async () => {
    const del = vi.fn(() => Promise.resolve())
    stoppedCard({
      ListSecretNames: () => Promise.resolve(['OPENROUTER_API_KEY']),
      CheckSecretPresence: presenceOf({ OPENROUTER_API_KEY: { inEnv: false, inKeychain: true } }),
      DeleteSecret: del,
    })
    fireEvent.click(await screen.findByRole('button', { name: /eliminar del llavero/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }))
    expect(del).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: /eliminar del llavero/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Sí, eliminar' }))
    await waitFor(() => expect(del).toHaveBeenCalledWith('OPENROUTER_API_KEY'))
  })
})

describe('the sealed notice and empty states (AS-5)', () => {
  it('unreadable config: names korvun.json, offers Abrir carpeta and Reintentar', async () => {
    const open = vi.fn(() => Promise.resolve())
    stoppedCard({
      ListSecretNames: () => Promise.reject(new Error('shell: parse config: bad')),
      OpenConfigFolder: open,
      CheckSecretPresence: presenceOf({}),
    })
    const notice = await screen.findByRole('alert')
    expect(notice.textContent).toContain('korvun.json')
    fireEvent.click(screen.getByRole('button', { name: 'Abrir carpeta' }))
    await waitFor(() => expect(open).toHaveBeenCalled())
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
  })

  it('a config with no referenced secrets paints the sealed empty state', async () => {
    stoppedCard({
      ListSecretNames: () => Promise.resolve([]),
      CheckSecretPresence: presenceOf({}),
    })
    expect(await screen.findByText(/no referencia ningún secreto todavía/)).toBeInTheDocument()
  })

  it('the permanent footer note: values are never shown', async () => {
    stoppedCard({
      ListSecretNames: () => Promise.resolve(NAMES),
      CheckSecretPresence: presenceOf({}),
    })
    await screen.findByText('KORVUN_TG_TOKEN')
    expect(screen.getByText(/Los valores nunca se muestran/)).toBeInTheDocument()
  })
})
