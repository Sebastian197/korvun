// SP6 — the governance panel through the SHIPPED canvas properties panel
// (config→UI→config): select the brain node, interact with the "Herramientas y
// skills" section, Save, assert the POSTed config. The visual contract is
// design-drafts/governance-panel/; this pins the behavior on the real panel.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import type { ReactElement } from 'react'
import type { Config } from '../config/schema'
import type { PollDeps } from '../config/reload'
import { CanvasView } from './CanvasView'

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

function succeedDeps(): PollDeps {
  return { getStatus: async () => 'succeeded', sleep: async () => {}, now: () => 0 }
}

function agentBaseline(over?: Partial<Config['brains'][number]>): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'soporte',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama'] },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2', locality: 'local' }],
        agent: {
          tools: ['read_file', 'http_fetch'],
          max_iterations: 5,
          system_prompt: '',
          http_fetch: { allow_hosts: ['api.github.com'] },
          read_file: { root: '/docs' },
        },
        ...over,
      },
    ],
    routes: [{ channel: 'telegram', brain: 'soporte' }],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

function captureSaveFetch(): { fetchMock: ReturnType<typeof vi.fn>; posted: () => Config } {
  let body: Config | null = null
  const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      body = JSON.parse(String(init.body)) as Config
      return { ok: true, status: 202, json: async () => ({ handle: 'r1' }) }
    }
    return { ok: true, status: 200, json: async () => body ?? agentBaseline() }
  })
  return { fetchMock, posted: () => body as Config }
}

function selectBrain() {
  act(() => {
    ;(rf.props?.onNodeClick as (e: unknown, n: { id: string }) => void)(null, { id: 'brain:0' })
  })
}

async function apply() {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: /aplicar/i }))
  })
}

beforeEach(() => {
  rf.props = null
  document.documentElement.dataset.theme = 'dark'
})

describe('SP6 governance panel on the canvas — presence (AS-8)', () => {
  it('renders the section for an agent brain', () => {
    render(<CanvasView baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    expect(screen.getByTestId('governance-section-0')).toBeTruthy()
  })

  it('does NOT render for a non-agent brain', () => {
    const nonAgent = agentBaseline()
    delete nonAgent.brains[0].agent
    render(<CanvasView baseline={nonAgent} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    expect(screen.queryByTestId('governance-section-0')).toBeNull()
  })
})

describe('SP6 tri-state (AS-1/2) — config→UI→config through the canvas', () => {
  it('Ensayo → Aplicar POSTs mode:shadow', async () => {
    const { fetchMock, posted } = captureSaveFetch()
    vi.stubGlobal('fetch', fetchMock)
    render(<CanvasView baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    fireEvent.click(screen.getByTestId('tri-read_file-shadow'))
    await apply()
    expect(posted().brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'shadow' }])
  })

  it('promote shadow→allow POSTs mode:allow', async () => {
    const { fetchMock, posted } = captureSaveFetch()
    vi.stubGlobal('fetch', fetchMock)
    const withShadow = agentBaseline()
    withShadow.brains[0].agent!.governance = [{ tool: 'read_file', mode: 'shadow' }]
    render(<CanvasView baseline={withShadow} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    fireEvent.click(screen.getByTestId('tri-read_file-allow'))
    await apply()
    expect(posted().brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'allow' }])
  })
})

describe('SP6 derivations (AS-5/6)', () => {
  it('shows the shield for a private network tool, hides for read_file', () => {
    render(<CanvasView baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    expect(screen.getByTestId('shield-http_fetch')).toBeTruthy()
    expect(screen.queryByTestId('shield-read_file')).toBeNull()
  })

  it('hides the shield when public', () => {
    render(
      <CanvasView
        baseline={agentBaseline({ sensitivity: 'public' })}
        token="secret"
        reloadDeps={succeedDeps()}
      />,
    )
    selectBrain()
    expect(screen.queryByTestId('shield-http_fetch')).toBeNull()
  })

  it('shows the house warning for ungoverned sensitive×cloud, hides when governed', () => {
    const cloud = agentBaseline({
      sensitivity: 'public',
      models: [{ provider: 'groq', model_id: 'm', locality: 'cloud' }],
      agent: {
        tools: ['read_file'],
        max_iterations: 5,
        system_prompt: '',
        read_file: { root: '/docs' },
      },
    })
    render(<CanvasView baseline={cloud} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    expect(screen.getByTestId('sensitive-cloud-warning-0').textContent).toContain('read_file')
  })
})

describe('SP6 cage editor (AS-7) — config→UI→config', () => {
  it('editing an allow-list host and Aplicar POSTs the new host', async () => {
    const { fetchMock, posted } = captureSaveFetch()
    vi.stubGlobal('fetch', fetchMock)
    render(<CanvasView baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)
    selectBrain()
    fireEvent.change(screen.getByTestId('cage-http_fetch-host-0'), {
      target: { value: 'docs.korvun.dev' },
    })
    await apply()
    expect(posted().brains[0].agent!.http_fetch!.allow_hosts).toEqual(['docs.korvun.dev'])
  })
})

describe('SP6 skills (AS-10)', () => {
  it('renders detected skills read-only (no toggle)', () => {
    const withSkills = agentBaseline()
    withSkills.brains[0].agent!.skills_dir = '~/korvun-skills'
    render(
      <CanvasView
        baseline={withSkills}
        token="secret"
        reloadDeps={succeedDeps()}
        detectedSkills={[
          { name: 'web-research', description: 'fetch + summarize an allow-listed URL' },
        ]}
      />,
    )
    selectBrain()
    const list = screen.getByTestId('skills-list-0')
    expect(list.textContent).toContain('web-research')
    expect(list.querySelector('button')).toBeNull()
    expect(list.querySelector('input')).toBeNull()
  })
})
