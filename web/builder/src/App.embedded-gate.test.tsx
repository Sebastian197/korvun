import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { fireEvent } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from './config/schema'
import { App } from './App'

// v0.9.1 RED (app-audit 2026-08-23, symptom 1): embedded in the desktop
// chrome the token gate is THEATRE — the shell proxy overwrites Authorization
// with the per-cycle bearer server-side (internal/shell/proxy.go, ADR-0035
// §4), so any pasted string opens the gate and the real token can never be
// known by the user. Embedded (window.self !== window.top) must SKIP the gate
// and load the config straight away; a direct browser keeps the gate exactly
// as before (there the pasted bearer is the real credential).

vi.mock('@xyflow/react', () => ({
  ReactFlow: (props: Record<string, unknown>) => {
    const nodeTypes = (props.nodeTypes ?? {}) as Record<
      string,
      (p: { id: string; data: unknown; selected: boolean }) => ReactElement
    >
    return (
      <div data-testid="rf-seam">
        {(props.nodes as Array<{ id: string; type: string; data: unknown }>).map((n) => {
          const C = nodeTypes[n.type]
          return C ? <C key={n.id} id={n.id} data={n.data} selected={false} /> : null
        })}
      </div>
    )
  },
  Handle: () => null,
  Position: { Left: 'left', Right: 'right', Top: 'top', Bottom: 'bottom' },
  Background: () => null,
}))

const CONFIG: Config = {
  channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
  brains: [
    {
      name: 'asistente',
      sensitivity: 'public',
      policy: { kind: 'priority' },
      dispatch: 'fanout',
      models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
    },
  ],
  routes: [{ channel: 'telegram', brain: 'asistente' }],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}

function stubApi() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      const json = (b: unknown) => ({ ok: true, status: 200, json: async () => b })
      if (url.includes('/api/brains')) return json([])
      if (url.includes('/api/channels')) return json([])
      if (url.includes('/api/config')) return json(CONFIG)
      return { ok: false, status: 404, json: async () => ({}) }
    }),
  )
}

function embed() {
  Object.defineProperty(window, 'top', { value: {}, configurable: true })
}
function unembed() {
  Object.defineProperty(window, 'top', { value: window, configurable: true })
}

beforeEach(() => {
  stubApi()
  document.documentElement.dataset.theme = 'dark'
})
afterEach(() => {
  unembed()
  vi.unstubAllGlobals()
})

describe('embedded desktop mode skips the token gate (v0.9.1, symptom 1)', () => {
  it('embedded: the canvas loads with NO token pasted and the gate never renders', async () => {
    embed()
    render(<App />)
    await screen.findByTestId('canvas-surface')
    expect(screen.queryByLabelText('admin bearer token')).toBeNull()
  })

  it('direct browser: the gate stays and the canvas waits for the bearer', async () => {
    render(<App />)
    expect(screen.queryByTestId('canvas-surface')).toBeNull()
    const input = screen.getByLabelText('admin bearer token')
    fireEvent.change(input, { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: 'Load' }))
    await screen.findByTestId('canvas-surface')
  })
})
