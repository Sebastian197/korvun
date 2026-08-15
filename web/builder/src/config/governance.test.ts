// SP6 RED — the governance reducer transitions (config→config, no DOM). The
// house criterion since v0.6.0: every panel control maps to a pure action that
// mutates ONLY brains[i].agent and preserves every untouched field. The
// serialization matrix is the exact espejo of config.AgentConfig
// (internal/config/config.go), so these tests double as the schema contract.
import { describe, it, expect } from 'vitest'
import { configReducer } from './edit'
import type { Config, AgentConfig } from './schema'
import { effectiveToolAttr, shieldShown, sensitiveCloudWarning } from './governance'

function agentConfig(agent: AgentConfig): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'T' }],
    brains: [
      {
        name: 'soporte',
        sensitivity: 'private',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2', locality: 'local' }],
        agent,
      },
    ],
    routes: [{ channel: 'telegram', brain: 'soporte' }],
  }
}

const baseAgent = { tools: ['read_file', 'http_fetch'], max_iterations: 5, system_prompt: '' }

describe('setToolMode (AS-1/2/3)', () => {
  it('creates a shadow grant on a previously ungoverned tool, preserving the rest', () => {
    const c = agentConfig(baseAgent)
    const next = configReducer(c, {
      kind: 'setToolMode',
      brain: 0,
      tool: 'read_file',
      mode: 'shadow',
    })
    expect(next.brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'shadow' }])
    // Everything else byte-identical.
    expect(next.brains[0].agent!.tools).toEqual(['read_file', 'http_fetch'])
    expect(next.channels).toEqual(c.channels)
    expect(next.routes).toEqual(c.routes)
  })

  it('promotes shadow→allow in place (the hot promotion)', () => {
    const c = agentConfig({ ...baseAgent, governance: [{ tool: 'read_file', mode: 'shadow' }] })
    const next = configReducer(c, {
      kind: 'setToolMode',
      brain: 0,
      tool: 'read_file',
      mode: 'allow',
    })
    expect(next.brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'allow' }])
  })

  it('stores deny explicitly (deny != absence)', () => {
    const c = agentConfig(baseAgent)
    const next = configReducer(c, {
      kind: 'setToolMode',
      brain: 0,
      tool: 'http_fetch',
      mode: 'deny',
    })
    expect(next.brains[0].agent!.governance).toEqual([{ tool: 'http_fetch', mode: 'deny' }])
  })

  it('updates an existing grant without duplicating it', () => {
    const c = agentConfig({
      ...baseAgent,
      governance: [{ tool: 'read_file', mode: 'allow', channels: ['telegram'] }],
    })
    const next = configReducer(c, {
      kind: 'setToolMode',
      brain: 0,
      tool: 'read_file',
      mode: 'shadow',
    })
    expect(next.brains[0].agent!.governance).toEqual([
      { tool: 'read_file', mode: 'shadow', channels: ['telegram'] },
    ])
  })
})

describe('setToolChannels', () => {
  it('scopes a grant to channels; empty clears the restriction', () => {
    const c = agentConfig({ ...baseAgent, governance: [{ tool: 'read_file', mode: 'allow' }] })
    const scoped = configReducer(c, {
      kind: 'setToolChannels',
      brain: 0,
      tool: 'read_file',
      channels: ['telegram', 'console'],
    })
    expect(scoped.brains[0].agent!.governance).toEqual([
      { tool: 'read_file', mode: 'allow', channels: ['telegram', 'console'] },
    ])
    const cleared = configReducer(scoped, {
      kind: 'setToolChannels',
      brain: 0,
      tool: 'read_file',
      channels: [],
    })
    expect(cleared.brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'allow' }])
  })
})

describe('setToolAttrOverride (AS-4)', () => {
  it('sets an override then clears it back to the house default', () => {
    const c = agentConfig(baseAgent)
    const set = configReducer(c, {
      kind: 'setToolAttrOverride',
      brain: 0,
      tool: 'http_fetch',
      attr: 'network',
      value: false,
    })
    expect(set.brains[0].agent!.tool_attrs).toEqual({ http_fetch: { network: false } })
    const cleared = configReducer(set, {
      kind: 'setToolAttrOverride',
      brain: 0,
      tool: 'http_fetch',
      attr: 'network',
      value: undefined,
    })
    // Key removed, and tool_attrs removed entirely when empty.
    expect(cleared.brains[0].agent!.tool_attrs).toBeUndefined()
  })

  it('keeps other attr overrides when clearing one', () => {
    const c = agentConfig({
      ...baseAgent,
      tool_attrs: { read_file: { sensitive: false, network: true } },
    })
    const next = configReducer(c, {
      kind: 'setToolAttrOverride',
      brain: 0,
      tool: 'read_file',
      attr: 'sensitive',
      value: undefined,
    })
    expect(next.brains[0].agent!.tool_attrs).toEqual({ read_file: { network: true } })
  })
})

describe('cage editors (AS-7)', () => {
  it('adds, edits and removes a host in an allow-list', () => {
    const c = agentConfig({ ...baseAgent, http_fetch: { allow_hosts: ['a'] } })
    const added = configReducer(c, { kind: 'addCageHost', brain: 0, cage: 'http_fetch' })
    expect(added.brains[0].agent!.http_fetch!.allow_hosts).toEqual(['a', ''])
    const edited = configReducer(added, {
      kind: 'setCageHost',
      brain: 0,
      cage: 'http_fetch',
      index: 1,
      value: 'b',
    })
    expect(edited.brains[0].agent!.http_fetch!.allow_hosts).toEqual(['a', 'b'])
    const removed = configReducer(edited, {
      kind: 'removeCageHost',
      brain: 0,
      cage: 'http_fetch',
      index: 0,
    })
    expect(removed.brains[0].agent!.http_fetch!.allow_hosts).toEqual(['b'])
  })

  it('edits read_file root and cap', () => {
    const c = agentConfig({ ...baseAgent, read_file: { root: '/x' } })
    const root = configReducer(c, {
      kind: 'setCageField',
      brain: 0,
      cage: 'read_file',
      field: 'root',
      value: '/y',
    })
    expect(root.brains[0].agent!.read_file).toEqual({ root: '/y' })
    const cap = configReducer(root, {
      kind: 'setCageField',
      brain: 0,
      cage: 'read_file',
      field: 'max_bytes',
      value: 65536,
    })
    expect(cap.brains[0].agent!.read_file).toEqual({ root: '/y', max_bytes: 65536 })
  })
})

describe('skills fields (AS-10)', () => {
  it('edits skills dir and budget', () => {
    const c = agentConfig(baseAgent)
    const dir = configReducer(c, {
      kind: 'setSkillsField',
      brain: 0,
      field: 'skills_dir',
      value: '~/s',
    })
    expect(dir.brains[0].agent!.skills_dir).toBe('~/s')
    const budget = configReducer(dir, {
      kind: 'setSkillsField',
      brain: 0,
      field: 'skills_body_budget',
      value: 8000,
    })
    expect(budget.brains[0].agent!.skills_body_budget).toBe(8000)
  })
})

describe('derivations (FR-DERIVE-1 / FR-WARN-1)', () => {
  it('effectiveToolAttr honors override over house default', () => {
    expect(effectiveToolAttr(undefined, 'http_fetch', 'network')).toBe(true) // house default
    expect(effectiveToolAttr({ http_fetch: { network: false } }, 'http_fetch', 'network')).toBe(
      false,
    )
    expect(effectiveToolAttr(undefined, 'read_file', 'sensitive')).toBe(true)
    expect(effectiveToolAttr(undefined, 'calc', 'network')).toBe(false)
  })

  it('shieldShown truth table (AS-5)', () => {
    const priv = agentConfig(baseAgent).brains[0]
    expect(shieldShown(priv, 'http_fetch')).toBe(true) // private + network
    const pub = { ...priv, sensitivity: 'public' }
    expect(shieldShown(pub, 'http_fetch')).toBe(false) // public
    const overridden = {
      ...priv,
      agent: { ...baseAgent, tool_attrs: { http_fetch: { network: false } } },
    }
    expect(shieldShown(overridden, 'http_fetch')).toBe(false) // network:false override
    expect(shieldShown(priv, 'read_file')).toBe(false) // not a network tool
  })

  it('sensitiveCloudWarning fires only ungoverned + sensitive + cloud (AS-6)', () => {
    const cloudBrain = {
      name: 'a',
      sensitivity: 'public',
      policy: { kind: 'priority' },
      dispatch: 'fanout',
      models: [{ provider: 'groq', model_id: 'm', locality: 'cloud' }],
      agent: { tools: ['read_file'], max_iterations: 5, system_prompt: '' },
    } as Config['brains'][number]
    expect(sensitiveCloudWarning(cloudBrain)).toEqual(['read_file'])
    // Governed → no warning.
    const governed = {
      ...cloudBrain,
      agent: { ...cloudBrain.agent!, governance: [{ tool: 'read_file', mode: 'allow' }] },
    }
    expect(sensitiveCloudWarning(governed)).toEqual([])
    // Local model → no warning.
    const local = {
      ...cloudBrain,
      models: [{ provider: 'ollama', model_id: 'm', locality: 'local' }],
    }
    expect(sensitiveCloudWarning(local)).toEqual([])
  })
})
