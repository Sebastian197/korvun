import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

// B14 RED — the panel wiring (spec AS-7): field errors render live in the
// Builder's error mold and Apply is gated while a model block is invalid.
// The 2026-08-23 corruption applied because the panel accepted a glued
// secret name and an emptied api_key_env without a word; these tests pin
// that the same edits now stop at the panel, and that fixing the field
// clears the error and re-enables Apply.

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

// The exact Sunday entry, HEALTHY (the 10:18 backup shape).
const OPENROUTER = {
  provider: 'openai-compatible',
  model_id: 'openrouter/auto',
  locality: 'cloud',
  base_url: 'https://openrouter.ai/api/v1',
  api_key_env: 'OPENROUTER_API_KEY',
} as const

function baseline(): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'public',
        policy: { kind: 'priority' },
        dispatch: 'sequential',
        models: [{ ...OPENROUTER }],
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

const applyButton = () => screen.getByRole('button', { name: /aplicar/i }) as HTMLButtonElement

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('the Sunday base_url is rejected at the panel (AS-3, AS-7)', () => {
  it('shows a base_url field error in the mold and disables Apply', () => {
    renderView()
    selectNode('model:0.0')
    fireEvent.change(screen.getByLabelText('base_url'), {
      target: { value: 'https://openrouter.ai/api/v1OPENROUTER_API_KEY' },
    })
    const err = screen.getByTestId('model-field-error-base_url')
    expect(err.getAttribute('role')).toBe('alert')
    expect(err.textContent).toMatch(/secret|env var/i)
    expect(applyButton().disabled).toBe(true)
  })

  it('fixing the field clears the error and re-enables Apply', () => {
    renderView()
    selectNode('model:0.0')
    const base = screen.getByLabelText('base_url')
    fireEvent.change(base, {
      target: { value: 'https://openrouter.ai/api/v1OPENROUTER_API_KEY' },
    })
    fireEvent.change(base, { target: { value: 'https://openrouter.ai/api/v2' } })
    expect(screen.queryByTestId('model-field-error-base_url')).toBeNull()
    expect(applyButton().disabled).toBe(false) // dirty AND valid
  })
})

describe('a malformed base_url is stopped before any POST (AS-1)', () => {
  it('field error + Apply disabled + zero fetches', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    renderView()
    selectNode('model:0.0')
    fireEvent.change(screen.getByLabelText('base_url'), { target: { value: 'not a url' } })
    expect(screen.getByTestId('model-field-error-base_url')).toBeTruthy()
    expect(applyButton().disabled).toBe(true)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('cloud compat without a key blocks Apply (AS-4, AS-7)', () => {
  it('emptying api_key_env on the cloud entry raises the blocking error', () => {
    renderView()
    selectNode('model:0.0')
    fireEvent.change(screen.getByLabelText('api_key_env'), { target: { value: '' } })
    const err = screen.getByTestId('model-field-error-api_key_env')
    expect(err.getAttribute('role')).toBe('alert')
    expect(applyButton().disabled).toBe(true)
  })

  it('restoring the env var name clears the error', () => {
    renderView()
    selectNode('model:0.0')
    const key = screen.getByLabelText('api_key_env')
    fireEvent.change(key, { target: { value: '' } })
    fireEvent.change(key, { target: { value: 'OPENROUTER_API_KEY' } })
    expect(screen.queryByTestId('model-field-error-api_key_env')).toBeNull()
  })
})

describe('a healthy edit keeps Apply enabled (AS-5)', () => {
  it('no errors, Apply follows dirty as before', () => {
    renderView()
    selectNode('model:0.0')
    fireEvent.change(screen.getByLabelText('model_id'), {
      target: { value: 'openrouter/auto-mini' },
    })
    expect(screen.queryByTestId('model-field-error-base_url')).toBeNull()
    expect(screen.queryByTestId('model-field-error-api_key_env')).toBeNull()
    expect(applyButton().disabled).toBe(false)
  })
})
