import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { BrainSummary } from '../api'
import { CanvasView } from './CanvasView'

// v0.9.2 RED (N6, bug-bash 2026-08-23): a model that does not answer at boot
// — the real "invalid model name" morning — failed only as a WARN in the log;
// the canvas painted the node as if nothing were wrong and the user found out
// mid-chat. The node now wears the health the core OBSERVED (/api/brains):
// unreachable paints a visible badge with the provider's error as its title,
// ready paints a quiet confirmation, and a never-probed model stays bare — an
// honest absence, never an invented "listo".

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
    channels: [],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'public',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
      },
    ],
    routes: [],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function brains(health: string, detail?: string): BrainSummary[] {
  return [
    {
      name: 'asistente',
      sensitivity: 'public',
      policy: 'priority',
      dispatch: 'fanout',
      models: [
        {
          provider: 'ollama',
          model_id: 'llama3.2:1b',
          health,
          ...(detail ? { health_detail: detail } : {}),
        },
      ],
    },
  ]
}

describe('N6 — the model node wears the observed health', () => {
  it('an unreachable model paints the badge with the provider error as title', async () => {
    render(
      <CanvasView
        baseline={baseline()}
        token="t"
        fetchBrains={() =>
          Promise.resolve(brains('unreachable', 'status 400: invalid model name'))
        }
      />,
    )
    const badge = await screen.findByTestId('badge-health')
    expect(badge.dataset.health).toBe('unreachable')
    expect(badge.textContent).toBe('no responde')
    expect(badge.title).toContain('invalid model name')
  })

  it('a ready model paints the quiet confirmation', async () => {
    render(
      <CanvasView
        baseline={baseline()}
        token="t"
        fetchBrains={() => Promise.resolve(brains('ready'))}
      />,
    )
    const badge = await screen.findByTestId('badge-health')
    expect(badge.dataset.health).toBe('ready')
    expect(badge.textContent).toBe('listo')
  })

  it('a never-probed model stays bare — no invented state', async () => {
    render(
      <CanvasView
        baseline={baseline()}
        token="t"
        fetchBrains={() => Promise.resolve(brains('unknown'))}
      />,
    )
    // The fetch resolves and still no badge: absence of evidence is absence.
    await waitFor(() => expect(rf.props).not.toBeNull())
    await Promise.resolve()
    expect(screen.queryByTestId('badge-health')).toBeNull()
  })

  it('a failing health fetch degrades to bare nodes, never a crash', async () => {
    render(
      <CanvasView
        baseline={baseline()}
        token="t"
        fetchBrains={() => Promise.reject(new Error('core stopped'))}
      />,
    )
    await waitFor(() => expect(rf.props).not.toBeNull())
    expect(screen.queryByTestId('badge-health')).toBeNull()
    expect(screen.getByTestId('model:0.0')).toBeTruthy()
  })
})
