import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigEditor } from './ConfigEditor'
import type { Config } from './config/schema'

function baseline(): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'support',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama'] },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
      },
    ],
    routes: [{ channel: 'telegram', brain: 'support' }],
    storage: { path: '/data/korvun.db' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('ConfigEditor (2b.2a: edit → build full config → POST → handle)', () => {
  it('Save is gated on dirty, POSTs the FULL edited config, and shows the reload handle', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 202,
      json: async () => ({ handle: 'reload-1' }),
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<ConfigEditor baseline={baseline()} token="secret" />)

    const save = screen.getByRole('button', { name: /save and reload/i }) as HTMLButtonElement
    expect(save.disabled).toBe(true) // no changes yet

    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'support-v2' } })
    expect(save.disabled).toBe(false) // dirty

    fireEvent.click(save)

    const handle = await screen.findByTestId('reload-handle')
    expect(handle.textContent).toContain('reload-1')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/config')
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer secret')

    // The FULL config went out: the edit applied AND every untouched field preserved.
    const sent = JSON.parse(init.body as string) as Config
    expect(sent.brains[0].name).toBe('support-v2')
    expect(sent.channels).toEqual(baseline().channels)
    expect(sent.routes).toEqual(baseline().routes)
    expect(sent.storage).toEqual(baseline().storage)
    expect(sent.admin).toEqual(baseline().admin)
    expect(sent.brains[0].models).toEqual(baseline().brains[0].models)
  })

  it('does not POST when there are no changes', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    render(<ConfigEditor baseline={baseline()} token="secret" />)
    const save = screen.getByRole('button', { name: /save and reload/i }) as HTMLButtonElement
    fireEvent.click(save)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
