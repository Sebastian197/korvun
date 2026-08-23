import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// v0.9.1 RED (app-audit 2026-08-23, symptoms 2 and 3): the canvas answers.
// (a) Dropping a MODEL on the empty surface was a silent no-op — NC-6 (a
// model exists only inside a brain) is kept, but the rule becomes VISIBLE:
// the drop paints an honest hint instead of nothing. (b) The properties
// panel had no close path at all (no X, no Escape, no click-outside): the
// only exits were deleting the node or selecting another. It gains all
// three affordances.

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

function baseline(): Config {
  return {
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
}

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}

function renderView() {
  return render(<CanvasView baseline={baseline()} token="secret" reloadDeps={succeedDeps()} />)
}

function selectNode(id: string) {
  act(() => {
    ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id })
  })
}

function dt(block: string) {
  return { dataTransfer: { getData: (k: string) => (k === 'application/korvun-block' ? block : '') } }
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('a. a model dropped on the empty surface answers (no silent no-op)', () => {
  it('paints the honest hint as a status region', () => {
    renderView()
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('model'))
    const hint = screen.getByTestId('surface-hint')
    expect(hint.getAttribute('role')).toBe('status')
    expect(hint.textContent).toMatch(/cerebro/i)
  })

  it('NC-6 semantics unchanged: no node is created by the surface drop', () => {
    renderView()
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('model'))
    expect(screen.queryByTestId('model:0.1')).toBeNull()
    expect(screen.queryByText(/unsaved changes/i)).toBeNull()
  })

  it('a successful brain drop clears the hint', () => {
    renderView()
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('model'))
    expect(screen.getByTestId('surface-hint')).toBeTruthy()
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('brain'))
    expect(screen.queryByTestId('surface-hint')).toBeNull()
    expect(screen.getByTestId('brain:1')).toBeTruthy()
  })
})

describe('b. the properties panel closes (X, Escape, click-outside)', () => {
  it('the X button closes the panel', () => {
    renderView()
    selectNode('brain:0')
    expect(screen.getByTestId('properties-panel')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Cerrar panel' }))
    expect(screen.queryByTestId('properties-panel')).toBeNull()
  })

  it('Escape closes the panel', () => {
    renderView()
    selectNode('model:0.0')
    expect(screen.getByTestId('properties-panel')).toBeTruthy()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.queryByTestId('properties-panel')).toBeNull()
  })

  it('clicking the pane (outside any node) closes the panel', () => {
    renderView()
    selectNode('channel:0')
    expect(screen.getByTestId('properties-panel')).toBeTruthy()
    act(() => {
      ;(rf.props?.onPaneClick as () => void)()
    })
    expect(screen.queryByTestId('properties-panel')).toBeNull()
  })
})
