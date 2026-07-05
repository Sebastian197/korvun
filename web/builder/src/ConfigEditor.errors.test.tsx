import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'm', locality: 'local' }],
      },
    ],
    routes: [{ channel: 'telegram', brain: 'support' }],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}
const deps: PollDeps = { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }

function stubFetchError(status: number, body: string) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: false, status, text: async () => body }),
  )
}

function editAndSave() {
  fireEvent.change(screen.getByLabelText('name'), { target: { value: 'x' } })
  fireEvent.click(screen.getByRole('button', { name: /save and reload/i }))
}

describe('ConfigEditor — error/edge states (2b.2c)', () => {
  it('400 validation is shown AT the field the server named', async () => {
    stubFetchError(400, JSON.stringify({ error: 'brains[0].sensitivity: unknown sensitivity' }))
    render(<ConfigEditor baseline={baseline()} token="t" reloadDeps={deps} />)
    editAndSave()
    const at = await screen.findByTestId('brain-error-0') // inline, on brain 0
    expect(at.textContent).toContain('sensitivity')
  })

  it('the TWO 409 render DISTINCT treatments (not one generic 409)', async () => {
    stubFetchError(409, JSON.stringify({ error_code: 'config_would_self_lock', message: 'x' }))
    const a = render(<ConfigEditor baseline={baseline()} token="t" reloadDeps={deps} />)
    editAndSave()
    expect(await screen.findByTestId('save-selflock')).toBeTruthy()
    expect(screen.queryByTestId('save-reload-in-progress')).toBeNull()
    a.unmount()

    stubFetchError(409, JSON.stringify({ error_code: 'reload_in_progress', message: 'x' }))
    render(<ConfigEditor baseline={baseline()} token="t" reloadDeps={deps} />)
    editAndSave()
    expect(await screen.findByTestId('save-reload-in-progress')).toBeTruthy()
    expect(screen.queryByTestId('save-selflock')).toBeNull()
  })

  it('401 clears the token (re-auth): onAuthError fires', async () => {
    stubFetchError(401, JSON.stringify({ error: 'unauthorized' }))
    const onAuthError = vi.fn()
    render(<ConfigEditor baseline={baseline()} token="t" reloadDeps={deps} onAuthError={onAuthError} />)
    editAndSave()
    await vi.waitFor(() => expect(onAuthError).toHaveBeenCalledTimes(1))
  })

  it('empty/first-run: a config with no brains shows the create-your-first-brain state', () => {
    const empty: Config = { ...baseline(), brains: [] }
    render(<ConfigEditor baseline={empty} token="t" reloadDeps={deps} />)
    expect(screen.getByTestId('empty-brains').textContent).toMatch(/create your first brain/i)
    expect(screen.queryByTestId('brain-error-0')).toBeNull()
  })

  it('remove brain: confirm removes it; removing the LAST brain falls into the empty/first-run state', () => {
    render(<ConfigEditor baseline={baseline()} token="t" reloadDeps={deps} />)
    // baseline() has one brain, "support". Click its remove control.
    fireEvent.click(screen.getByRole('button', { name: /remove brain support/i }))
    // it asks to confirm — not an accidental destructive click
    expect(screen.getByTestId('brain-remove-confirm-0')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /yes, remove/i }))
    // last brain gone → the empty/first-run state, not a broken empty list
    expect(screen.getByTestId('empty-brains').textContent).toMatch(/create your first brain/i)
  })

  it('dirty + Discard asks for confirmation and reverts to the baseline', () => {
    render(<ConfigEditor baseline={baseline()} token="t" reloadDeps={deps} />)
    const save = screen.getByRole('button', { name: /save and reload/i }) as HTMLButtonElement
    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'support-v2' } })
    expect(save.disabled).toBe(false) // dirty

    fireEvent.click(screen.getByRole('button', { name: /^discard$/i }))
    // confirmation, not an immediate revert
    expect(screen.getByTestId('discard-confirm')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /yes, discard/i }))

    expect(save.disabled).toBe(true) // reverted → not dirty
    expect((screen.getByLabelText('name') as HTMLInputElement).value).toBe('support')
  })
})
