import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from './config/schema'
import { App } from './App'

// SP4 RED (builder-canvas FR-SCOPE-1, the SWITCH): once the operator pastes
// the admin bearer, the CANVAS is the builder's main face — palette, surface
// and the canvas save-bar — and the legacy form editor is no longer the view.
// The token gate itself is unchanged (paste screen, in-memory only, ADR-0030
// §6). React Flow rides the same surgical seam as the CanvasView suites
// (jsdom cannot render the real canvas — SP3's documented cut).
//
// RED today: App still mounts ConfigEditor after the paste, so the canvas
// testids never appear and 'Save and reload' does.

vi.mock('@xyflow/react', () => ({
  ReactFlow: (props: Record<string, unknown>) => {
    const nodeTypes = (props.nodeTypes ?? {}) as Record<
      string,
      (p: { id: string; data: unknown; selected: boolean }) => ReactElement
    >
    return (
      <div data-testid="rf-seam">
        {(props.nodes as Array<{ id: string; type: string; data: unknown }>).map((n) => {
          const NodeComp = nodeTypes[n.type]
          return NodeComp ? <NodeComp key={n.id} id={n.id} data={n.data} selected={false} /> : null
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
      const json = (body: unknown) => ({ ok: true, status: 200, json: async () => body })
      if (url.includes('/api/brains')) return json([])
      if (url.includes('/api/channels')) return json([])
      if (url.includes('/api/config')) return json(CONFIG)
      return { ok: false, status: 404, json: async () => ({}) }
    }),
  )
}

async function pasteToken() {
  fireEvent.change(screen.getByLabelText('admin bearer token'), { target: { value: 'secret' } })
  fireEvent.click(screen.getByRole('button', { name: 'Load' }))
  // The canvas face: the drop surface is the view's anchor testid.
  await screen.findByTestId('canvas-surface')
}

beforeEach(() => {
  stubApi()
  document.documentElement.dataset.theme = 'dark'
})

describe('the switch: canvas is the builder\'s main view (FR-SCOPE-1)', () => {
  it('pasting the token lands on the canvas: surface, palette and Aplicar', async () => {
    render(<App />)
    await pasteToken()
    expect(screen.getByTestId('canvas-surface')).toBeTruthy()
    for (const id of ['palette:channel', 'palette:brain', 'palette:model']) {
      expect(screen.getByTestId(id)).toBeTruthy()
    }
    expect(screen.getByRole('button', { name: /aplicar/i })).toBeTruthy()
  })

  it('the legacy form editor is no longer the face', async () => {
    render(<App />)
    await pasteToken()
    // The 2b editor's anchors must be gone from the main view: its save
    // button and its editor container.
    expect(screen.queryByRole('button', { name: /save and reload/i })).toBeNull()
    expect(document.querySelector('.editor')).toBeNull()
  })

  it('the canvas face renders the loaded config as nodes', async () => {
    render(<App />)
    await pasteToken()
    expect(screen.getByTestId('channel:0')).toBeTruthy()
    expect(screen.getByTestId('brain:0')).toBeTruthy()
    expect(screen.getByTestId('model:0.0')).toBeTruthy()
  })

  it('the token gate itself is unchanged: no canvas before the paste', () => {
    render(<App />)
    expect(screen.getByLabelText('admin bearer token')).toBeTruthy()
    expect(screen.queryByTestId('canvas-surface')).toBeNull()
  })
})
