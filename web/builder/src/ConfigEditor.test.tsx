import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react'
import { ConfigEditor } from './ConfigEditor'
import type { Config } from './config/schema'
import type { PollDeps } from './config/reload'

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

// Instant, deterministic reload deps that reach `succeeded` on the first poll.
function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}

describe('ConfigEditor — edit → build full config → POST (2b.2a)', () => {
  it('Save is gated on dirty and POSTs the FULL edited config', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ handle: 'r1' }) }) // POST
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => baseline() }) // re-GET after succeeded
    vi.stubGlobal('fetch', fetchMock)

    render(<ConfigEditor baseline={baseline()} token="secret" reloadDeps={succeedDeps()} />)
    const save = screen.getByRole('button', { name: /save and reload/i }) as HTMLButtonElement
    expect(save.disabled).toBe(true) // no changes yet

    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'support-v2' } })
    expect(save.disabled).toBe(false)
    fireEvent.click(save)

    await screen.findByTestId('reload-succeeded')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/config')
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer secret')
    const sent = JSON.parse(init.body as string) as Config
    expect(sent.brains[0].name).toBe('support-v2')
    expect(sent.channels).toEqual(baseline().channels)
    expect(sent.storage).toEqual(baseline().storage)
    expect(sent.admin).toEqual(baseline().admin)
  })

  it('does not POST when there are no changes', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    render(<ConfigEditor baseline={baseline()} token="secret" reloadDeps={succeedDeps()} />)
    fireEvent.click(screen.getByRole('button', { name: /save and reload/i }))
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('ConfigEditor — reload state machine (2b.2b)', () => {
  it('locks the WHOLE form while the reload is in flight (cutover-in-progress)', async () => {
    const gates: Array<() => void> = []
    const reloadDeps: PollDeps = {
      getStatus: async () => 'cutover-in-progress',
      sleep: () => new Promise<void>((res) => gates.push(res)),
      now: () => 0,
    }
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 202, json: async () => ({ handle: 'r1' }) }),
    )

    render(<ConfigEditor baseline={baseline()} token="secret" reloadDeps={reloadDeps} />)
    const nameInput = () => screen.getByLabelText('name') as HTMLInputElement
    fireEvent.change(nameInput(), { target: { value: 'x' } })
    fireEvent.click(screen.getByRole('button', { name: /save and reload/i }))

    // POST resolves → poll starts (pending) → awaiting the first backoff sleep.
    await waitFor(() => expect(gates.length).toBe(1))
    // Release it → getStatus → cutover-in-progress → paused on the next sleep.
    await act(async () => gates.shift()!())
    await waitFor(() => expect(gates.length).toBe(1))

    // The whole form is locked via the wrapping <fieldset disabled>, which natively
    // disables every descendant control during the swap (ADR-0030 §5).
    const fs = nameInput().closest('fieldset') as HTMLFieldSetElement
    expect(fs.disabled).toBe(true)
    expect(screen.getByTestId('reload-inflight').textContent).toContain('cutover-in-progress')
  })

  it('on succeeded, re-fetches the baseline so dirty clears', async () => {
    const applied: Config = { ...baseline(), brains: [{ ...baseline().brains[0], name: 'x' }] }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ handle: 'r1' }) }) // POST
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => applied }) // re-GET
    vi.stubGlobal('fetch', fetchMock)

    render(<ConfigEditor baseline={baseline()} token="secret" reloadDeps={succeedDeps()} />)
    const save = () => screen.getByRole('button', { name: /save and reload/i }) as HTMLButtonElement
    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'x' } })
    fireEvent.click(save())

    await screen.findByTestId('reload-succeeded')
    await waitFor(() => expect(save().disabled).toBe(true)) // dirty cleared by the re-GET
  })
})
