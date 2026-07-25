// Canales (SP6c, contra final-15/16/17): the real channel list from
// /api/channels, a detail from the config's true fields, the "Añadir canal"
// CTA, and "Quitar canal…" behind a confirmation → the mutation pipe.
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetSnapshotForTests, pollSnapshotOnce } from '../snapshot/store'
import { pollOnce } from '../status/store'
import { Channels } from './Channels'

const CHANNELS = [
  { type: 'telegram', mode: 'polling', name: 'telegram', dropped: 0 },
  { type: 'discord', mode: 'gateway', name: 'discord', dropped: 2 },
]
const BRAINS = [
  {
    name: 'asistente',
    sensitivity: 'private',
    policy: 'priority',
    dispatch: 'fanout',
    models: [{ provider: 'ollama', model_id: 'llama3.2:1b' }],
  },
]
const CONFIG = {
  channels: [
    { type: 'telegram', mode: 'polling', token_env: 'TELEGRAM_TOKEN' },
    { type: 'discord', mode: 'gateway', token_env: 'DISCORD_BOT_TOKEN' },
  ],
  brains: [{ name: 'asistente' }],
  routes: [
    { channel: 'telegram', brain: 'asistente' },
    { channel: 'discord', brain: 'asistente' },
  ],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}

function snapshotFetch(): typeof fetch {
  return ((url: string) => {
    const u = String(url)
    const body = u.includes('brains') ? BRAINS : CHANNELS
    return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))
  }) as unknown as typeof fetch
}

function configFetch(): typeof fetch {
  return (() =>
    Promise.resolve(new Response(JSON.stringify(CONFIG), { status: 200 }))) as typeof fetch
}

beforeEach(async () => {
  resetSnapshotForTests()
  await pollOnce((() => Promise.resolve(new Response('ok'))) as typeof fetch)
  await pollSnapshotOnce(snapshotFetch())
})

describe('Canales', () => {
  it('lists channels with mode, env-var NAME and a health badge', async () => {
    render(
      <Channels
        onOpenWizard={() => undefined}
        onOpenBuilder={() => undefined}
        fetcher={configFetch()}
      />,
    )
    expect(screen.getAllByText('Telegram').length).toBeGreaterThan(0)
    await waitFor(() => {
      expect(screen.getByText(/polling · TELEGRAM_TOKEN/)).toBeInTheDocument()
    })
    expect(screen.getByText('Discord')).toBeInTheDocument()
    expect(screen.getAllByText('Operativo').length).toBeGreaterThan(0)
  })

  it('the detail shows the config route as a chip and the change-in-builder link', async () => {
    render(
      <Channels
        onOpenWizard={() => undefined}
        onOpenBuilder={() => undefined}
        fetcher={configFetch()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Telegram/ }))
    await waitFor(() => {
      expect(screen.getByText(/cambiar en el Builder/)).toBeInTheDocument()
    })
    expect(screen.getAllByText(/TELEGRAM_TOKEN/).length).toBeGreaterThan(0)
    // The value is never shown — only the name.
    expect(screen.queryByText(/solo nombre/)).toBeInTheDocument()
  })

  it('a channel with a token shows the secret honesty note, not a value', async () => {
    render(
      <Channels
        onOpenWizard={() => undefined}
        onOpenBuilder={() => undefined}
        fetcher={configFetch()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    await waitFor(() => {
      expect(screen.getByText(/El valor vive en tu entorno/)).toBeInTheDocument()
    })
  })

  it('Añadir canal opens the wizard', () => {
    const onOpenWizard = vi.fn()
    render(<Channels onOpenWizard={onOpenWizard} onOpenBuilder={() => undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /Añadir canal/ }))
    expect(onOpenWizard).toHaveBeenCalled()
  })

  it('Quitar canal asks for confirmation and only then mutates', async () => {
    const fetcher = vi.fn(((url: string, init?: RequestInit) => {
      const u = String(url)
      if (u === '/api/config' && (init?.method ?? 'GET') === 'GET') {
        return Promise.resolve(new Response(JSON.stringify(CONFIG), { status: 200 }))
      }
      if (u === '/api/config') {
        return Promise.resolve(new Response(JSON.stringify({ handle: 'h1' }), { status: 202 }))
      }
      return Promise.resolve(new Response(JSON.stringify({ state: 'succeeded' }), { status: 200 }))
    }) as unknown as typeof fetch)

    render(
      <Channels
        onOpenWizard={() => undefined}
        onOpenBuilder={() => undefined}
        fetcher={fetcher}
        pollIntervalMs={0}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Discord/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Quitar canal/ }))
    // Confirmation gate: no POST yet.
    const postsBeforeConfirm = (fetcher.mock.calls as unknown[][]).filter(
      (c) => (c[1] as RequestInit | undefined)?.method === 'POST',
    ).length
    expect(postsBeforeConfirm).toBe(0)
    fireEvent.click(screen.getByRole('button', { name: /Sí, quitar/ }))
    await waitFor(() => {
      const posts = (fetcher.mock.calls as unknown[][]).filter(
        (c) => (c[1] as RequestInit | undefined)?.method === 'POST',
      )
      expect(posts.length).toBe(1)
    })
  })
})
