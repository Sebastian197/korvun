import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// SP5 RED (builder-canvas): DELETE from the canvas + the minimal-usability
// touches the audit demanded — the "Eliminar nodo…" panel control (with
// confirmation, final-6), edge deletion by selection+Delete (routes only), the
// palette hint literal, and console hygiene. Same surgical React Flow seam as
// the other CanvasView suites (jsdom can't render the real canvas — SP3 cut).

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

function sp5Baseline(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DISCORD' },
    ],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'public',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
      },
      { name: 'general', sensitivity: 'public', policy: { kind: 'consensus' }, dispatch: 'fanout', models: [] },
    ],
    routes: [
      { channel: 'telegram', brain: 'asistente' },
      { channel: 'discord', brain: 'general' },
    ],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}
function renderView() {
  return render(<CanvasView baseline={sp5Baseline()} token="secret" reloadDeps={succeedDeps()} />)
}
function selectNode(id: string) {
  act(() => {
    ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id })
  })
}
// React Flow's standard delete hook: onEdgesDelete(edges[]). CanvasView must
// route it to disconnectRoute for route edges and ignore composition edges.
function deleteEdges(edges: Array<{ id: string }>) {
  act(() => {
    ;(rf.props?.onEdgesDelete as (e: Array<{ id: string }>) => void)?.(edges)
  })
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('a. "Eliminar nodo…" from the panel (final-6), with confirmation', () => {
  it('deleting a BRAIN cascades its routes and models out', () => {
    renderView()
    selectNode('brain:0') // asistente, routed from telegram
    fireEvent.click(screen.getByRole('button', { name: /eliminar nodo/i }))
    // Confirmation gate first (destructive; final-6 pattern).
    fireEvent.click(screen.getByRole('button', { name: /sí, eliminar|confirmar/i }))
    // asistente gone → only general remains → its brain node is brain:0 now.
    expect(screen.queryByText('asistente')).toBeNull()
    // The route telegram→asistente cascaded: only discord→general survives.
    const edges = (rf.props?.edges ?? []) as Array<{ id: string; kind?: string }>
    const routeEdges = edges.filter((e) => e.id.startsWith('route:'))
    expect(routeEdges).toHaveLength(1)
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('deleting a CHANNEL cascades its routes out', () => {
    renderView()
    selectNode('channel:1') // discord, routes discord→general
    fireEvent.click(screen.getByRole('button', { name: /eliminar nodo/i }))
    fireEvent.click(screen.getByRole('button', { name: /sí, eliminar|confirmar/i }))
    const channels = (rf.props?.nodes as Array<{ id: string; type: string }>).filter((n) => n.type === 'channel')
    expect(channels).toHaveLength(1)
    const edges = (rf.props?.edges ?? []) as Array<{ id: string }>
    expect(edges.filter((e) => e.id.startsWith('route:'))).toHaveLength(1)
  })

  it('deleting a MODEL removes only that entry', () => {
    renderView()
    selectNode('model:0.0')
    fireEvent.click(screen.getByRole('button', { name: /eliminar nodo/i }))
    fireEvent.click(screen.getByRole('button', { name: /sí, eliminar|confirmar/i }))
    expect(screen.queryByTestId('model:0.0')).toBeNull()
    // The brain and its route survive — a model is not a route.
    expect(screen.getByTestId('brain:0')).toBeTruthy()
  })

  it('cancelling the confirmation deletes NOTHING', () => {
    renderView()
    selectNode('brain:0')
    fireEvent.click(screen.getByRole('button', { name: /eliminar nodo/i }))
    fireEvent.click(screen.getByRole('button', { name: /cancelar/i }))
    expect(screen.getByText('asistente')).toBeTruthy()
    expect(screen.getByText(/no changes/i)).toBeTruthy()
  })
})

describe('b. edge deletion: route edges only (composition is not cable-deleted)', () => {
  it('deleting a ROUTE edge dispatches disconnectRoute', () => {
    renderView()
    const before = (rf.props?.edges as Array<{ id: string }>).filter((e) => e.id.startsWith('route:')).length
    deleteEdges([{ id: 'route:0' }])
    const after = (rf.props?.edges as Array<{ id: string }>).filter((e) => e.id.startsWith('route:')).length
    expect(after).toBe(before - 1)
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('deleting a COMPOSITION edge does NOTHING (model leaves via its panel)', () => {
    renderView()
    const before = (rf.props?.edges as Array<{ id: string }>).length
    deleteEdges([{ id: 'comp:0.0' }])
    const after = (rf.props?.edges as Array<{ id: string }>).length
    expect(after).toBe(before)
    expect(screen.getByText(/no changes/i)).toBeTruthy()
  })
})

describe('c. the palette carries the final-6 drag hint', () => {
  it('shows the literal palette hint (v0.9.1 wording: models land ON brains)', () => {
    renderView()
    expect(screen.getByText(/Arrastra canales y cerebros al lienzo; un modelo se suelta sobre un cerebro\./)).toBeTruthy()
  })
})