import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// SP4 RED (builder-canvas): the properties panel closes SP3's declared gaps —
// (a) the webhook block fields become EDITABLE (SP3 shipped them read-only,
// presence-only red), (b) the MODEL panel edits provider/model_id/locality/
// api_key_env through the existing updateModel action, (c) the type-aware
// mode drag (type change pulls mode to the new type's valid transport) gains
// its pinning test — SP3 implemented it by necessity, untested until now.
// Same surgical React Flow seam as CanvasView.test.tsx (jsdom cannot render
// the real canvas — the SP3-documented cut).

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

function sp4Baseline(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      {
        type: 'webhook',
        mode: '',
        token_env: 'KORVUN_HOOK',
        webhook: { bind: '127.0.0.1:8090', outbound_url: 'https://downstream.example/reply' },
      },
    ],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'public',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [
          { provider: 'groq', model_id: 'llama-3.3-70b', locality: 'cloud', api_key_env: 'GROQ_KEY' },
        ],
      },
    ],
    routes: [],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}

function renderView() {
  return render(<CanvasView baseline={sp4Baseline()} token="secret" reloadDeps={succeedDeps()} />)
}

function selectNode(id: string) {
  act(() => {
    ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id })
  })
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('a. the webhook block is EDITABLE from the panel', () => {
  it('typing in a block field updates the working copy and dirties the bar', () => {
    renderView()
    selectNode('channel:1')
    const bind = screen.getByLabelText('bind') as HTMLInputElement
    fireEvent.change(bind, { target: { value: '0.0.0.0:9000' } })
    expect(bind.value).toBe('0.0.0.0:9000') // the reducer took the edit
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('all four fields are editable, none readOnly', () => {
    renderView()
    selectNode('channel:1')
    for (const label of ['bind', 'path', 'outbound_url', 'outbound_token_env']) {
      const input = screen.getByLabelText(label) as HTMLInputElement
      expect(input.readOnly).toBe(false)
    }
  })

  it('switching a channel to type webhook exposes the editable block and the first edit materializes it', () => {
    renderView()
    selectNode('channel:0') // telegram
    fireEvent.change(screen.getByLabelText('type'), { target: { value: 'webhook' } })
    // The blockless webhook channel still offers the four fields…
    const url = screen.getByLabelText('outbound_url') as HTMLInputElement
    expect(url.value).toBe('')
    // …and the first edit materializes the block (setWebhookField pattern).
    fireEvent.change(url, { target: { value: 'https://x.example/r' } })
    expect(url.value).toBe('https://x.example/r')
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })
})

describe('b. the MODEL panel edits the model entry', () => {
  it('edits model_id via the existing updateModel action', () => {
    renderView()
    selectNode('model:0.0')
    const id = screen.getByLabelText('model_id') as HTMLInputElement
    expect(id.value).toBe('llama-3.3-70b')
    fireEvent.change(id, { target: { value: 'llama-3.3-70b-versatile' } })
    expect(id.value).toBe('llama-3.3-70b-versatile')
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('provider and locality are selects over the mirror enums', () => {
    renderView()
    selectNode('model:0.0')
    const provider = screen.getByLabelText('provider') as HTMLSelectElement
    const locality = screen.getByLabelText('locality') as HTMLSelectElement
    expect(Array.from(provider.options).map((o) => o.value)).toEqual(['ollama', 'groq'])
    expect(Array.from(locality.options).map((o) => o.value)).toEqual(['local', 'cloud'])
  })

  it('api_key_env carries the ADR-0010 microcopy: the env-var NAME, never the value', () => {
    renderView()
    selectNode('model:0.0')
    const key = screen.getByLabelText('api_key_env') as HTMLInputElement
    expect(key.value).toBe('GROQ_KEY')
    // The house microcopy (ModelRow precedent): placeholder says it is a NAME.
    expect(key.placeholder).toBe('env var name')
    fireEvent.change(key, { target: { value: 'GROQ_API_KEY' } })
    expect(key.value).toBe('GROQ_API_KEY')
  })
})

describe('c. type change drags mode to the new type\'s transport (pinned)', () => {
  // SP3 implemented this by necessity (the panel had to keep mode valid);
  // the mirror rule finally gains its regression guard here. Espeja
  // config.Validate: telegram→polling, discord→gateway, webhook→NO mode
  // (config.go:444-452).
  it('telegram → discord pulls mode to gateway', () => {
    renderView()
    selectNode('channel:0')
    fireEvent.change(screen.getByLabelText('type'), { target: { value: 'discord' } })
    const mode = screen.getByLabelText('mode') as HTMLSelectElement
    expect(mode.value).toBe('gateway')
  })

  it('→ webhook drops the mode select entirely (mode emptied)', () => {
    renderView()
    selectNode('channel:0')
    fireEvent.change(screen.getByLabelText('type'), { target: { value: 'webhook' } })
    expect(screen.queryByLabelText('mode')).toBeNull()
  })
})
