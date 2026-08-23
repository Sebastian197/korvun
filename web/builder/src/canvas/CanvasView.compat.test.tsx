import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// v0.9.1 RED (app-audit 2026-08-23, finding A1): the builder lied about the
// openai-compatible provider (ADR-0044, the v0.9.0 gateway). PROVIDERS did
// not list it, so the panel's select displayed the FIRST option ("ollama")
// for a compat entry, and the panel had no base_url field at all — reading
// was a lie and editing a corruption vector. This suite pins: the provider
// shows its true name, base_url is visible and editable, and a compat entry
// survives a save round-trip SEMANTICALLY INTACT — both when another node is
// edited and when the compat entry itself is.

const rf = vi.hoisted(() => ({ props: null as Record<string, unknown> | null }))

vi.mock('@xyflow/react', () => ({
  ReactFlow: (props: Record<string, unknown>) => {
    rf.props = props
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

// The compat entry rides FIRST (model:0.0) — the audit's exact shape: a local
// llama.cpp endpoint through the openai-compatible provider.
const COMPAT = {
  provider: 'openai-compatible',
  model_id: 'qwen3-4b-instruct-2507',
  locality: 'local',
  base_url: 'http://127.0.0.1:8189/v1',
} as const

function compatBaseline(): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'public',
        policy: { kind: 'priority', order: ['openai-compatible', 'ollama'] },
        dispatch: 'sequential',
        models: [
          { ...COMPAT },
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
        ],
      },
    ],
    routes: [{ channel: 'telegram', brain: 'asistente' }],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}

function renderView() {
  return render(<CanvasView baseline={compatBaseline()} token="secret" reloadDeps={succeedDeps()} />)
}

function selectNode(id: string) {
  act(() => {
    ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id })
  })
}

function stubSave() {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ handle: 'r1' }) })
    .mockResolvedValueOnce({ ok: true, status: 200, json: async () => compatBaseline() })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function apply(fetchMock: ReturnType<typeof vi.fn>): Promise<Config> {
  fireEvent.click(screen.getByRole('button', { name: /aplicar/i }))
  await screen.findByTestId('reload-succeeded')
  const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
  return JSON.parse(init.body as string) as Config
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('a. the compat entry is displayed truthfully', () => {
  it('provider select shows "openai-compatible", never "ollama"', () => {
    renderView()
    selectNode('model:0.0')
    const provider = screen.getByLabelText('provider') as HTMLSelectElement
    expect(provider.value).toBe('openai-compatible')
  })

  it('the panel exposes base_url with the entry value', () => {
    renderView()
    selectNode('model:0.0')
    const base = screen.getByLabelText('base_url') as HTMLInputElement
    expect(base.value).toBe(COMPAT.base_url)
  })
})

describe('b. the compat entry is editable through its real fields', () => {
  it('base_url edits ride updateModel and mark the copy dirty', () => {
    renderView()
    selectNode('model:0.0')
    const base = screen.getByLabelText('base_url') as HTMLInputElement
    fireEvent.change(base, { target: { value: 'http://127.0.0.1:9999/v1' } })
    expect(base.value).toBe('http://127.0.0.1:9999/v1')
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('editing the compat entry itself keeps provider and base_url intact on save', async () => {
    const fetchMock = stubSave()
    renderView()
    selectNode('model:0.0')
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'qwen3-8b' } })
    const sent = await apply(fetchMock)
    expect(sent.brains[0].models[0]).toEqual({ ...COMPAT, model_id: 'qwen3-8b' })
  })
})

describe('c. round-trip: editing ANOTHER node leaves the compat entry intact', () => {
  it('the posted config carries the compat entry byte-for-byte', async () => {
    const fetchMock = stubSave()
    renderView()
    selectNode('model:0.1') // the ollama sibling
    fireEvent.change(screen.getByLabelText('model_id'), { target: { value: 'llama3.3:8b' } })
    const sent = await apply(fetchMock)
    expect(sent.brains[0].models[0]).toEqual(COMPAT)
    expect(sent.brains[0].models[1].model_id).toBe('llama3.3:8b')
  })
})
