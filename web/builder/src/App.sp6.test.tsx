import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from './config/schema'
import { App } from './App'

// SP6 RED (builder-canvas, e): the builder's OWN top bar ("korvun · builder")
// is redundant when embedded in the desktop chrome (which already titles the
// view). When window.self !== window.top the own bar must NOT render; in a
// direct browser (self === top) it stays. RED today: App always renders the
// header.bar.

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

async function landOnCanvas() {
  fireEvent.change(screen.getByLabelText('admin bearer token'), { target: { value: 'secret' } })
  fireEvent.click(screen.getByRole('button', { name: 'Load' }))
  await screen.findByTestId('canvas-surface')
}

beforeEach(() => {
  stubApi()
  document.documentElement.dataset.theme = 'dark'
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('e. own top bar vs the desktop chrome (no double header)', () => {
  it('direct browser (self === top): the own bar is present', async () => {
    // jsdom: window.top === window.self by default.
    render(<App />)
    await landOnCanvas()
    expect(screen.getByText('builder')).toBeTruthy() // the crumb in the own bar
  })

  it('embedded (self !== top): the own bar is NOT rendered', async () => {
    // Simulate the desktop iframe: make top a different object than self.
    Object.defineProperty(window, 'top', { value: {}, configurable: true })
    try {
      render(<App />)
      await landOnCanvas()
      expect(screen.queryByText('builder')).toBeNull()
    } finally {
      Object.defineProperty(window, 'top', { value: window, configurable: true })
    }
  })
})