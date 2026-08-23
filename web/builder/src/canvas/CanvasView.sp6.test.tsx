import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act, within } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// SP6 RED (builder-canvas): dressing the canvas against final-6 — rich nodes,
// a sectioned palette, the header counter + Descartar, the editable webhook
// mapping, and sane fit-view scale. Same surgical seam; the node DOM is real
// (nodeTypes render through it), so node content is a genuine assertion.

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

function sp6Baseline(): Config {
  return {
    channels: [
      {
        type: 'webhook',
        mode: '',
        token_env: 'KORVUN_HOOK',
        webhook: { outbound_url: 'https://x.example/r' },
      },
    ],
    brains: [
      {
        name: 'privado',
        sensitivity: 'private',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
          { provider: 'groq', model_id: 'nube', locality: 'cloud', api_key_env: 'K' },
        ],
      },
      { name: 'publico', sensitivity: 'public', policy: { kind: 'consensus' }, dispatch: 'fanout', models: [] },
    ],
    routes: [{ channel: 'webhook', brain: 'privado' }],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}
function renderView() {
  return render(<CanvasView baseline={sp6Baseline()} token="secret" reloadDeps={succeedDeps()} />)
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

describe('a. rich nodes (final-6): icon, badges, policy line', () => {
  it('every node carries a type icon', () => {
    renderView()
    for (const id of ['channel:0', 'brain:0', 'model:0.0']) {
      expect(within(screen.getByTestId(id)).getByTestId('node-icon')).toBeTruthy()
    }
  })
  it('the brain node shows its sensitivity badge', () => {
    renderView()
    expect(within(screen.getByTestId('brain:0')).getByTestId('badge-sensitivity').textContent).toMatch(/privado|private/i)
    expect(within(screen.getByTestId('brain:1')).getByTestId('badge-sensitivity').textContent).toMatch(/público|public/i)
  })
  it('the model node shows its locality badge', () => {
    renderView()
    expect(within(screen.getByTestId('model:0.0')).getByTestId('badge-locality').textContent).toMatch(/local/i)
    expect(within(screen.getByTestId('model:0.1')).getByTestId('badge-locality').textContent).toMatch(/nube|cloud/i)
  })
  it('the brain node shows its policy', () => {
    renderView()
    expect(within(screen.getByTestId('brain:0')).getByTestId('node-policy').textContent).toMatch(/priority|prioridad/i)
  })
})

describe('b. the palette is sectioned with icons and drag-dots; SP5 hint stays', () => {
  it('has the three titled sections', () => {
    renderView()
    expect(screen.getByText(/^canales$/i)).toBeTruthy()
    expect(screen.getByText(/^cerebros$/i)).toBeTruthy()
    expect(screen.getByText(/^modelos$/i)).toBeTruthy()
  })
  it('each palette block carries an icon and a drag-dot affordance', () => {
    renderView()
    for (const block of ['channel', 'brain', 'model']) {
      const el = screen.getByTestId(`palette:${block}`)
      expect(within(el).getByTestId('block-icon')).toBeTruthy()
      expect(within(el).getByTestId('drag-dots')).toBeTruthy()
    }
  })
  it('keeps the SP5 drag hint', () => {
    renderView()
    expect(screen.getByText(/Arrastra canales y cerebros al lienzo; un modelo se suelta sobre un cerebro\./)).toBeTruthy()
  })
})

describe('c. canvas header: pending counter + functional Descartar', () => {
  it('with zero changes: Descartar is disabled, no counter', () => {
    renderView()
    expect((screen.getByRole('button', { name: /descartar/i }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryByText(/cambios? sin aplicar/i)).toBeNull()
  })
  it('one edit → "1 cambio sin aplicar"; Descartar enabled', () => {
    renderView()
    selectNode('brain:0')
    fireEvent.change(screen.getByLabelText('name', { exact: true }), { target: { value: 'privado-v2' } })
    expect(screen.getByText(/1 cambio sin aplicar/i)).toBeTruthy()
    expect((screen.getByRole('button', { name: /descartar/i }) as HTMLButtonElement).disabled).toBe(false)
  })
  it('Descartar reverts to the applied config and re-renders the projection', () => {
    renderView()
    selectNode('brain:0')
    fireEvent.change(screen.getByLabelText('name', { exact: true }), { target: { value: 'privado-v2' } })
    // The node label changed in the projection…
    expect((rf.props?.nodes as Array<{ id: string; data: { label: string } }>).find((n) => n.id === 'brain:0')?.data.label).toBe('privado-v2')
    fireEvent.click(screen.getByRole('button', { name: /descartar/i }))
    // …and reverts on Descartar; the counter clears.
    expect((rf.props?.nodes as Array<{ id: string; data: { label: string } }>).find((n) => n.id === 'brain:0')?.data.label).toBe('privado')
    expect(screen.queryByText(/cambios? sin aplicar/i)).toBeNull()
    expect(screen.getByText(/no changes/i)).toBeTruthy()
  })
})

describe('d. the webhook mapping is editable from the panel (six fields)', () => {
  it('shows all six mapping fields and mutation flows through the reducer', () => {
    renderView()
    selectNode('channel:0')
    for (const label of ['sender_id', 'sender_name', 'text', 'media_url', 'media_type', 'conversation_id']) {
      expect(screen.getByLabelText(label)).toBeTruthy()
    }
    const sid = screen.getByLabelText('sender_id') as HTMLInputElement
    fireEvent.change(sid, { target: { value: 'uid' } })
    expect(sid.value).toBe('uid')
    expect(screen.getByText(/1 cambio sin aplicar/i)).toBeTruthy()
  })
})

describe('g. fit-view scale keeps nodes reachable (not lost in the void)', () => {
  it('passes fitViewOptions with padding and a sane maxZoom', () => {
    renderView()
    const opts = rf.props?.fitViewOptions as { padding?: number; maxZoom?: number } | undefined
    expect(opts, 'CanvasView must pass fitViewOptions').toBeTruthy()
    expect(opts?.padding).toBeGreaterThan(0)
    expect(opts?.maxZoom).toBeGreaterThan(0)
    expect(opts?.maxZoom).toBeLessThanOrEqual(1.2) // never zoom a two-node graph huge
  })
})
