import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// v0.9.2 RED (B8, bug-bash 2026-08-23): cables could not be disconnected by
// ANY gesture. Root cause: edges are CONTROLLED but no onEdgesChange was
// passed, so React Flow's selection changes (type:'select') were dropped and
// no edge could ever become selected — onEdgesDelete was unreachable from
// the UI (verified live in WKWebView AND Chromium). The contract pinned
// here: every created connection can be undone — connect → select (via
// onEdgesChange) → the edges prop carries selected:true → delete (via
// onEdgesDelete) → the route leaves the working copy. Selection changes are
// the ONLY thing onEdgesChange applies: structure keeps deriving from the
// working copy alone.

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
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DC' },
    ],
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

type RFEdgeLike = { id: string; selected?: boolean }
const edges = () => (rf.props?.edges ?? []) as RFEdgeLike[]

function renderView() {
  return render(<CanvasView baseline={baseline()} token="secret" reloadDeps={succeedDeps()} />)
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('B8 — every created connection can be undone', () => {
  it('passes onEdgesChange and both delete keys to React Flow', () => {
    renderView()
    expect(typeof rf.props?.onEdgesChange).toBe('function')
    expect(rf.props?.deleteKeyCode).toEqual(['Backspace', 'Delete'])
  })

  it('a select change marks the edge selected in the edges prop', () => {
    renderView()
    act(() => {
      ;(rf.props?.onEdgesChange as (c: unknown[]) => void)([
        { id: 'route:0', type: 'select', selected: true },
      ])
    })
    expect(edges().find((e) => e.id === 'route:0')?.selected).toBe(true)
    act(() => {
      ;(rf.props?.onEdgesChange as (c: unknown[]) => void)([
        { id: 'route:0', type: 'select', selected: false },
      ])
    })
    expect(edges().find((e) => e.id === 'route:0')?.selected).toBe(false)
  })

  it('the full cycle: connect → select → delete leaves the working copy', () => {
    renderView()
    // connect discord → asistente (a NEW route:1)
    act(() => {
      ;(rf.props?.onConnect as (c: { source: string; target: string }) => void)({
        source: 'channel:1',
        target: 'brain:0',
      })
    })
    expect(edges().some((e) => e.id === 'route:1')).toBe(true)
    // select it (the gesture React Flow reports on click)
    act(() => {
      ;(rf.props?.onEdgesChange as (c: unknown[]) => void)([
        { id: 'route:1', type: 'select', selected: true },
      ])
    })
    expect(edges().find((e) => e.id === 'route:1')?.selected).toBe(true)
    // delete it (what React Flow fires on Backspace/Delete over a selection)
    act(() => {
      ;(rf.props?.onEdgesDelete as (d: Array<{ id: string }>) => void)([{ id: 'route:1' }])
    })
    expect(edges().some((e) => e.id === 'route:1')).toBe(false)
    // connect+delete of the SAME route is a net-zero edit: the copy is
    // CLEAN again — the strongest form of "the connection was undone".
    expect(screen.queryByText(/unsaved changes/i)).toBeNull()
    expect(screen.getByText(/no changes/i)).toBeTruthy()
  })

  it('onEdgesChange applies ONLY selection: a remove change never mutates the config', () => {
    renderView()
    const before = edges().length
    act(() => {
      ;(rf.props?.onEdgesChange as (c: unknown[]) => void)([{ id: 'route:0', type: 'remove' }])
    })
    expect(edges().length).toBe(before)
    expect(screen.queryByText(/unsaved changes/i)).toBeNull()
  })
})
