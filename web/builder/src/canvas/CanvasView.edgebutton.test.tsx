import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// B8-bis RED (sealed design ola2-designs §5, OPCIÓN (a) — APROBADO POR
// CHANO 2026-08-23): the mouse user gets a deletion gesture. A ✕ button
// floats at the midpoint of the SELECTED route cable (the React Flow
// "Button Edge" pattern); clicking it rides the SAME path as Backspace
// (disconnectRoute + selection cleanup); deselecting hides it; the dashed
// composition edge never offers it. The B8 contract — every created
// connection can be undone — gains the mouse gesture.

const rf = vi.hoisted(() => ({ props: null as Record<string, unknown> | null }))

interface EdgeLike {
  id: string
  type?: string
  selected?: boolean
}

vi.mock('@xyflow/react', () => ({
  ReactFlow: (props: Record<string, unknown>) => {
    rf.props = props
    const edgeTypes = (props.edgeTypes ?? {}) as Record<
      string,
      (p: Record<string, unknown>) => ReactElement
    >
    return (
      <div data-testid="rf-seam">
        {(props.edges as EdgeLike[]).map((e) => {
          const EdgeComp = e.type !== undefined ? edgeTypes[e.type] : undefined
          return EdgeComp ? (
            <EdgeComp
              key={e.id}
              id={e.id}
              selected={e.selected ?? false}
              sourceX={0}
              sourceY={0}
              targetX={100}
              targetY={100}
              sourcePosition="right"
              targetPosition="left"
            />
          ) : null
        })}
      </div>
    )
  },
  Handle: () => null,
  Position: { Left: 'left', Right: 'right', Top: 'top', Bottom: 'bottom' },
  Background: () => null,
  BaseEdge: () => null,
  EdgeLabelRenderer: ({ children }: { children?: unknown }) => <>{children as ReactElement}</>,
  getBezierPath: () => ['M 0 0 L 100 100', 50, 50] as const,
}))

// telegram→asistente is the deletable route; the private brain with a cloud
// model projects the dashed EXCLUDED composition edge (never deletable).
function baseline(): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'private',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
          { provider: 'groq', model_id: 'llama-3.3-70b', locality: 'cloud', api_key_env: 'G' },
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

const edges = () => (rf.props?.edges ?? []) as EdgeLike[]

function selectEdge(id: string, selected = true) {
  act(() => {
    ;(rf.props?.onEdgesChange as (c: unknown[]) => void)([{ id, type: 'select', selected }])
  })
}

function renderView() {
  return render(<CanvasView baseline={baseline()} token="secret" reloadDeps={succeedDeps()} />)
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('the ✕ appears only on the selected route cable', () => {
  it('route edges carry the custom type wired in edgeTypes', () => {
    renderView()
    const route = edges().find((e) => e.id === 'route:0')
    expect(route?.type).toBe('route')
    expect((rf.props?.edgeTypes as Record<string, unknown>)?.route).toBeTypeOf('function')
  })

  it('unselected: no delete button on screen', () => {
    renderView()
    expect(screen.queryByRole('button', { name: 'Eliminar conexión' })).toBeNull()
  })

  it('selecting the cable shows the ✕; deselecting hides it', () => {
    renderView()
    selectEdge('route:0')
    expect(screen.getByRole('button', { name: 'Eliminar conexión' })).toBeTruthy()
    selectEdge('route:0', false)
    expect(screen.queryByRole('button', { name: 'Eliminar conexión' })).toBeNull()
  })
})

describe('clicking the ✕ deletes through the SAME path as Backspace', () => {
  it('removes the route from the working copy and clears the selection', () => {
    renderView()
    selectEdge('route:0')
    fireEvent.click(screen.getByRole('button', { name: 'Eliminar conexión' }))
    expect(edges().some((e) => e.id === 'route:0')).toBe(false)
    expect(screen.queryByRole('button', { name: 'Eliminar conexión' })).toBeNull()
    // The deletion is a working-copy change — Descartar can revert it.
    expect(screen.getByTestId('pending-count').textContent).toMatch(/1 cambio/)
  })
})

describe('the composition edge never offers the gesture', () => {
  it('the excluded dashed edge renders without the ✕ even when marked selected', () => {
    renderView()
    const composition = edges().find((e) => !e.id.startsWith('route:'))
    expect(composition).toBeDefined()
    selectEdge(composition!.id)
    expect(screen.queryByRole('button', { name: 'Eliminar conexión' })).toBeNull()
  })
})
