import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import axe from 'axe-core'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
// SP3 RED: this component does not exist yet — the import failure IS the red.
import { CanvasView } from './CanvasView'

// SP3 RED (builder-canvas FR-CANVAS/FR-SCOPE-1, NC-6/NC-7). THE HONEST CUT
// (declared per the brief's technical notice, verified empirically): real
// React Flow does NOT render under jsdom — a probe render throws
// `ReferenceError: ResizeObserver is not defined`, and with a polyfill the
// canvas still measures 0×0 (React Flow's documented jsdom limitation). So
// this suite tests OUR OWN layer with the canvas surgically isolated:
//
//   - `@xyflow/react` is mocked with a seam that CAPTURES the props CanvasView
//     hands it (nodes/edges/isValidConnection/onConnect/onNodeClick) and
//     renders each node through OUR nodeTypes components — so node testids,
//     kinds and marks are REAL production DOM, not mock output.
//   - What the mock cannot honestly cover goes to the SP4 e2e against the real
//     binary: real drag/pointer interaction, real edge SVG painting (the gray
//     dashed excluded stroke), color-contrast, and the desktop iframe.
//
// The palette, the properties panel and the save-bar are real DOM throughout.

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

// Fixture in the final-6 shape: three channels (webhook WITH its block), a
// private brain carrying local+cloud (the exclusion case) and an empty public
// brain (the drop target).
function canvasBaseline(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DISCORD' },
      {
        type: 'webhook',
        mode: '',
        token_env: 'KORVUN_HOOK',
        webhook: { outbound_url: 'https://downstream.example/reply' },
      },
    ],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama', 'groq'] },
        dispatch: 'sequential',
        models: [
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
          { provider: 'groq', model_id: 'llama-3.3-70b', locality: 'cloud', api_key_env: 'GROQ_KEY' },
        ],
      },
      { name: 'general', sensitivity: 'public', policy: { kind: 'consensus' }, dispatch: 'fanout', models: [] },
    ],
    routes: [{ channel: 'telegram', brain: 'asistente' }],
    storage: { path: '/data/korvun.db' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}

function renderView() {
  return render(<CanvasView baseline={canvasBaseline()} token="secret" reloadDeps={succeedDeps()} />)
}

// A DataTransfer double: jsdom has no real DnD, but our handlers only read
// getData('application/korvun-block') — the payload contract of the palette.
function dt(block: string) {
  return { dataTransfer: { getData: (k: string) => (k === 'application/korvun-block' ? block : '') } }
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('a. CanvasView renders the SP2 projection', () => {
  it('one node per channel/brain/model, testid = the stable graph id', () => {
    renderView()
    for (const id of [
      'channel:0',
      'channel:1',
      'channel:2',
      'brain:0',
      'brain:1',
      'model:0.0',
      'model:0.1',
    ]) {
      expect(screen.getByTestId(id)).toBeTruthy()
    }
  })

  it('the three kinds are distinguishable (data-kind on the node DOM)', () => {
    renderView()
    expect(screen.getByTestId('channel:2').dataset.kind).toBe('channel')
    expect(screen.getByTestId('brain:0').dataset.kind).toBe('brain')
    expect(screen.getByTestId('model:0.1').dataset.kind).toBe('model')
  })
})

describe('b. the excluded edge carries an observable mark', () => {
  it("comp edges private+cloud get the 'edge-excluded' class; the rest do not", () => {
    renderView()
    // The mark rides the edge objects CanvasView hands React Flow — SP3 CSS
    // paints it gray dashed over this class (visual check goes to SP4 e2e).
    // Data rule per SP2 graphFromConfig (ADR-0015: private brain ⛔ cloud model).
    const edges = (rf.props?.edges ?? []) as Array<{ id: string; className?: string }>
    const cls = (id: string) => edges.find((e) => e.id === id)?.className ?? ''
    expect(cls('comp:0.1')).toContain('edge-excluded') // private + cloud
    expect(cls('comp:0.0')).not.toContain('edge-excluded') // private + local
    expect(cls('route:0')).not.toContain('edge-excluded')
  })
})

describe('c. isValidConnection = canConnect wired; onConnect dispatches exactly', () => {
  it('exposes canConnect through isValidConnection', () => {
    renderView()
    const isValid = rf.props?.isValidConnection as (c: {
      source: string
      target: string
    }) => boolean
    // Mirror matrix (SP2 canConnect): canal→cerebro the only manual cable.
    expect(isValid({ source: 'channel:1', target: 'brain:1' })).toBe(true)
    expect(isValid({ source: 'brain:0', target: 'model:0.0' })).toBe(false)
    expect(isValid({ source: 'channel:0', target: 'channel:1' })).toBe(false)
  })

  it('a valid connect dispatches connectRoute with the right indices', () => {
    renderView()
    act(() => {
      ;(rf.props?.onConnect as (c: { source: string; target: string }) => void)({
        source: 'channel:1',
        target: 'brain:1',
      })
    })
    // Observable through the projection: the new route edge appears…
    const edges = (rf.props?.edges ?? []) as Array<{ id: string; source: string; target: string }>
    expect(edges).toContainEqual(
      expect.objectContaining({ id: 'route:1', source: 'channel:1', target: 'brain:1' }),
    )
    // …and the working copy is dirty (save-bar reflects it).
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('an invalid connect attempt dispatches NOTHING', () => {
    renderView()
    const before = (rf.props?.edges as unknown[]).length
    act(() => {
      ;(rf.props?.onConnect as (c: { source: string; target: string }) => void)({
        source: 'brain:0',
        target: 'model:0.0',
      })
    })
    expect((rf.props?.edges as unknown[]).length).toBe(before)
    expect(screen.getByText(/no changes/i)).toBeTruthy()
  })
})

describe('d. palette drops (NC-6: no orphans)', () => {
  it('model dropped ONTO a brain → that brain gains the model', () => {
    renderView()
    fireEvent.drop(screen.getByTestId('brain:1'), dt('model'))
    // brains[1].models was empty; the projection now emits model:1.0.
    expect(screen.getByTestId('model:1.0')).toBeTruthy()
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })

  it('model dropped on empty canvas → nothing (a model exists only inside a brain)', () => {
    renderView()
    const before = (rf.props?.nodes as unknown[]).length
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('model'))
    expect((rf.props?.nodes as unknown[]).length).toBe(before)
    expect(screen.getByText(/no changes/i)).toBeTruthy()
  })

  it('channel and brain palette blocks dropped on the canvas create new entries via the reducer', () => {
    renderView()
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('channel'))
    expect(screen.getByTestId('channel:3')).toBeTruthy()
    fireEvent.drop(screen.getByTestId('canvas-surface'), dt('brain'))
    expect(screen.getByTestId('brain:2')).toBeTruthy()
  })

  it('palette blocks are draggable and carry their testids', () => {
    renderView()
    for (const id of ['palette:channel', 'palette:brain', 'palette:model']) {
      expect(screen.getByTestId(id).getAttribute('draggable')).toBe('true')
    }
  })
})

describe('e. selection → properties panel (re-homed forms, modes mirror)', () => {
  it('selecting the webhook channel shows its block fields and NO mode select', () => {
    renderView()
    act(() => {
      ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id: 'channel:2' })
    })
    const panel = screen.getByTestId('properties-panel')
    expect(panel).toBeTruthy()
    // The webhook nested block (ADR-0038 §1) surfaces in the panel.
    for (const label of ['bind', 'path', 'outbound_url', 'outbound_token_env']) {
      expect(screen.getByLabelText(label)).toBeTruthy()
    }
    // Espeja config.Validate: "webhook takes no mode" (config.go, NC-1c).
    expect(screen.queryByLabelText('mode')).toBeNull()
  })

  it('mode options mirror the validator per type: telegram=polling, discord=gateway', () => {
    renderView()
    const optionsFor = (id: string) => {
      act(() => {
        ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id })
      })
      const sel = screen.getByLabelText('mode') as HTMLSelectElement
      return Array.from(sel.options).map((o) => o.value)
    }
    // Espeja config.Validate: validateChannelMode(…, "polling") for telegram
    // (config.go:445) and (…, "gateway") for discord (config.go:449).
    expect(optionsFor('channel:0')).toEqual(['polling'])
    expect(optionsFor('channel:1')).toEqual(['gateway'])
  })
})

describe('f. persona in the brain panel', () => {
  it('the four persona fields edit the block through the reducer', () => {
    renderView()
    act(() => {
      ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id: 'brain:0' })
    })
    for (const label of ['display_name', 'tone', 'language', 'instructions']) {
      expect(screen.getByLabelText(label)).toBeTruthy()
    }
    const name = screen.getByLabelText('display_name') as HTMLInputElement
    fireEvent.change(name, { target: { value: 'Nova' } })
    expect(name.value).toBe('Nova') // the working copy took the setPersonaField
    expect(screen.getByText(/unsaved changes/i)).toBeTruthy()
  })
})

describe('g. dirty + save-bar reuse the REAL reload machine', () => {
  it('Aplicar posts the full config and drives the real pollReload to succeeded', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ handle: 'r1' }) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => canvasBaseline() })
    vi.stubGlobal('fetch', fetchMock)

    renderView()
    const apply = screen.getByRole('button', { name: /aplicar/i }) as HTMLButtonElement
    expect(apply.disabled).toBe(true) // clean → gated

    act(() => {
      ;(rf.props?.onConnect as (c: { source: string; target: string }) => void)({
        source: 'channel:1',
        target: 'brain:1',
      })
    })
    expect(apply.disabled).toBe(false)
    fireEvent.click(apply)

    // The REAL machine's testid (ConfigEditor's ReloadView, reused not
    // duplicated): reloadDeps here is the real PollDeps shape from
    // src/config/reload — importing the real types/machinery is the contract.
    await screen.findByTestId('reload-succeeded')

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/config')
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer secret')
    const sent = JSON.parse(init.body as string) as Config
    expect(sent.routes).toContainEqual({ channel: 'discord', brain: 'general' })
    expect(sent.storage).toEqual(canvasBaseline().storage) // untouched fields ride whole
  })
})

describe('h. the violet accent is a TOKEN, never a loose hex (NC-7)', () => {
  it('the Aplicar button rides the tokenized primary class', () => {
    renderView()
    const apply = screen.getByRole('button', { name: /aplicar/i })
    // .btn.primary is var(--accent)-based (App.css:150; ADR-0030 tokens).
    expect(apply.className).toContain('primary')
  })

  // The stylesheet token-scan half of this criterion lives in
  // canvas.tokens.test.ts: vitest stubs every css import (even `?raw`) to an
  // empty module, and .tsx files get web-transform URLs (http://localhost/…),
  // so the fs+import.meta.url mechanism (approved 2026-08-01) needs a .ts
  // sibling with node-transform file:// URLs. Assertions unchanged there.
})

describe('i. axe on the view (structural; color-contrast → SP4 e2e)', () => {
  // jsdom cannot compute color contrast (no layout/paint) — that rule is
  // DISABLED here and covered against the real binary in SP4 (SP0 precedent:
  // axe ran clean in both themes over the real page). This checks structure:
  // labels, roles, names of the palette/panel/save-bar DOM.
  const structuralAxe = (el: HTMLElement) =>
    axe.run(el, { rules: { 'color-contrast': { enabled: false } } })

  it('no structural violations in dark', async () => {
    document.documentElement.dataset.theme = 'dark'
    const { container } = renderView()
    const results = await structuralAxe(container)
    expect(results.violations).toEqual([])
  })

  it('no structural violations in light', async () => {
    document.documentElement.dataset.theme = 'light'
    const { container } = renderView()
    const results = await structuralAxe(container)
    expect(results.violations).toEqual([])
  })
})
