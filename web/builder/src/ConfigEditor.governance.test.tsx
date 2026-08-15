// SP6 RED — the governance panel component (config→UI→config through the real
// ConfigEditor: interact, Save, assert the POSTed config carries the change).
// The visual contract is design-drafts/governance-panel/; this file pins the
// BEHAVIOR (the pixels are the mockup's job, the classes are asserted for
// fidelity).
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { ConfigEditor } from './ConfigEditor'
import type { Config } from './config/schema'
import type { PollDeps } from './config/reload'

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

/** Captures the config POSTed by Save. */
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

async function save(): Promise<void> {
  const btn = screen.getByRole('button', { name: /save and reload/i })
  await act(async () => {
    fireEvent.click(btn)
  })
}

describe('SP6 governance panel — presence (AS-8, FR-UI-1)', () => {
  it('renders the "Herramientas y skills" section for an agent brain', () => {
    render(<ConfigEditor baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)
    expect(screen.getByTestId('governance-section-0')).toBeTruthy()
  })

  it('does NOT render the section for a non-agent brain', () => {
    const nonAgent = agentBaseline()
    delete nonAgent.brains[0].agent
    render(<ConfigEditor baseline={nonAgent} token="secret" reloadDeps={succeedDeps()} />)
    expect(screen.queryByTestId('governance-section-0')).toBeNull()
  })
})

describe('SP6 tri-state (AS-1/2, FR-GOV-1) — config→UI→config', () => {
  it('setting a grant to Ensayo and Saving POSTs mode:shadow', async () => {
    const { fetchMock, posted } = captureSaveFetch()
    vi.stubGlobal('fetch', fetchMock)
    render(<ConfigEditor baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)

    // The tri-state control for read_file, "Ensayo" segment.
    fireEvent.click(screen.getByTestId('tri-read_file-shadow'))
    await save()

    const g = posted().brains[0].agent!.governance
    expect(g).toEqual([{ tool: 'read_file', mode: 'shadow' }])
  })

  it('promoting shadow→allow POSTs mode:allow (hot promotion)', async () => {
    const { fetchMock, posted } = captureSaveFetch()
    vi.stubGlobal('fetch', fetchMock)
    const withShadow = agentBaseline()
    withShadow.brains[0].agent!.governance = [{ tool: 'read_file', mode: 'shadow' }]
    render(<ConfigEditor baseline={withShadow} token="secret" reloadDeps={succeedDeps()} />)

    fireEvent.click(screen.getByTestId('tri-read_file-allow'))
    await save()

    expect(posted().brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'allow' }])
  })
})

describe('SP6 derivations (AS-5/6, FR-DERIVE-1/FR-WARN-1)', () => {
  it('shows the network shield pill for a private brain network tool', () => {
    render(<ConfigEditor baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)
    expect(screen.getByTestId('shield-http_fetch')).toBeTruthy()
    // read_file is not a network tool → no shield.
    expect(screen.queryByTestId('shield-read_file')).toBeNull()
  })

  it('hides the shield when the brain is public', () => {
    render(
      <ConfigEditor
        baseline={agentBaseline({ sensitivity: 'public' })}
        token="secret"
        reloadDeps={succeedDeps()}
      />,
    )
    expect(screen.queryByTestId('shield-http_fetch')).toBeNull()
  })

  it('shows the house warning for ungoverned sensitive×cloud', () => {
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
    render(<ConfigEditor baseline={cloud} token="secret" reloadDeps={succeedDeps()} />)
    const warn = screen.getByTestId('sensitive-cloud-warning-0')
    expect(warn.textContent).toContain('read_file')
  })

  it('no warning when governed', () => {
    const cloud = agentBaseline({
      sensitivity: 'public',
      models: [{ provider: 'groq', model_id: 'm', locality: 'cloud' }],
      agent: {
        tools: ['read_file'],
        max_iterations: 5,
        system_prompt: '',
        read_file: { root: '/docs' },
        governance: [{ tool: 'read_file', mode: 'allow' }],
      },
    })
    render(<ConfigEditor baseline={cloud} token="secret" reloadDeps={succeedDeps()} />)
    expect(screen.queryByTestId('sensitive-cloud-warning-0')).toBeNull()
  })
})

describe('SP6 cage editor (AS-7) — config→UI→config', () => {
  it('editing an allow-list host and Saving POSTs the new host', async () => {
    const { fetchMock, posted } = captureSaveFetch()
    vi.stubGlobal('fetch', fetchMock)
    render(<ConfigEditor baseline={agentBaseline()} token="secret" reloadDeps={succeedDeps()} />)

    const host0 = screen.getByTestId('cage-http_fetch-host-0') as HTMLInputElement
    fireEvent.change(host0, { target: { value: 'docs.korvun.dev' } })
    await save()

    expect(posted().brains[0].agent!.http_fetch!.allow_hosts).toEqual(['docs.korvun.dev'])
  })
})

describe('SP6 skills (AS-10)', () => {
  it('renders detected skills read-only (no toggle emitted)', () => {
    const withSkills = agentBaseline()
    withSkills.brains[0].agent!.skills_dir = '~/korvun-skills'
    render(
      <ConfigEditor
        baseline={withSkills}
        token="secret"
        reloadDeps={succeedDeps()}
        detectedSkills={[
          { name: 'web-research', description: 'fetch + summarize an allow-listed URL' },
        ]}
      />,
    )
    const list = screen.getByTestId('skills-list-0')
    expect(list.textContent).toContain('web-research')
    // No interactive control inside the skills list (read-only).
    expect(list.querySelector('button')).toBeNull()
    expect(list.querySelector('input')).toBeNull()
  })
})
