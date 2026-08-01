import { describe, it, expect } from 'vitest'
import type { Config } from '../config/schema'
// SP2 RED: this module does not exist yet — the import failure IS the red
// (house precedent: the compile-failure red of coordinator_carveout_test.go on
// the Go side). Green creates src/canvas/graph.ts.
import { graphFromConfig, canConnect, type GraphNode } from './graph'

// SP2 RED (builder-canvas FR-CANVAS/FR-SER, NC-6 resolved): graphFromConfig is
// the PURE, deterministic config→graph projection. Normative reference: the
// final-6 mockup (design-drafts/final-6-builder-estados.png) — three columns
// canales | cerebros | modelos, model nodes PER BRAIN ENTRY (llama3.2:1b
// appears once per brain that carries it), route edges canal→cerebro and
// composition edges cerebro→modelo. Positions are derived and NEVER persisted
// (NC-6): the graph is a projection, the config stays the only truth.

// Fixture mirroring the final-6 shape: 3 channels, 2 brains, per-brain models.
function canvasConfig(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DISCORD' },
      { type: 'webhook', mode: '', token_env: 'KORVUN_HOOK' },
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
      {
        name: 'general',
        sensitivity: 'public',
        policy: { kind: 'consensus' },
        dispatch: 'fanout',
        models: [
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
          { provider: 'groq', model_id: 'llama-3.3-70b', locality: 'cloud', api_key_env: 'GROQ_KEY' },
        ],
      },
    ],
    routes: [
      { channel: 'telegram', brain: 'asistente' },
      { channel: 'discord', brain: 'general' },
      { channel: 'webhook', brain: 'general' },
    ],
    storage: { path: '/data/korvun.db' },
  }
}

describe('graphFromConfig — columns, stable order, stable IDs (final-6 layout)', () => {
  it('projects the three columns in config-index order with stable IDs', () => {
    const g = graphFromConfig(canvasConfig())

    // Column 0: channels, one node per channels[i], row = i (config order —
    // the mockup lists Telegram, Discord, Webhook top-down).
    const channels = g.nodes.filter((n) => n.kind === 'channel')
    expect(channels.map((n) => [n.id, n.column, n.row])).toEqual([
      ['channel:0', 0, 0],
      ['channel:1', 0, 1],
      ['channel:2', 0, 2],
    ])

    // Column 1: brains by config index.
    const brains = g.nodes.filter((n) => n.kind === 'brain')
    expect(brains.map((n) => [n.id, n.column, n.row])).toEqual([
      ['brain:0', 1, 0],
      ['brain:1', 1, 1],
    ])

    // Column 2: model nodes PER BRAIN ENTRY (final-6: llama3.2:1b appears
    // twice — once under each brain), rows sequential over (brain, model)
    // config order. ID scheme model:<brainIdx>.<modelIdx> is the stable key.
    const models = g.nodes.filter((n) => n.kind === 'model')
    expect(models.map((n) => [n.id, n.column, n.row])).toEqual([
      ['model:0.0', 2, 0],
      ['model:0.1', 2, 1],
      ['model:1.0', 2, 2],
      ['model:1.1', 2, 3],
    ])
  })

  it('projects route edges (canal→cerebro) and composition edges (cerebro→modelo)', () => {
    const g = graphFromConfig(canvasConfig())

    // Route edges: one per routes[i]. Route names resolve against the REAL
    // registration rule — a channel registers under its TYPE name
    // (config.go validateChannels: names[ch.Type] = true) and a route must
    // name a configured channel and brain (validateRoutes, routes[i].channel /
    // routes[i].brain checks).
    const routes = g.edges.filter((e) => e.kind === 'route')
    expect(routes.map((e) => [e.id, e.source, e.target])).toEqual([
      ['route:0', 'channel:0', 'brain:0'],
      ['route:1', 'channel:1', 'brain:1'],
      ['route:2', 'channel:2', 'brain:1'],
    ])

    // Composition edges: one per brains[i].models[j] — composition lives IN
    // the brain (config.go validateModels: models belong to brains[i].models),
    // it is not a routes[] concern.
    const comps = g.edges.filter((e) => e.kind === 'composition')
    expect(comps.map((e) => [e.id, e.source, e.target])).toEqual([
      ['comp:0.0', 'brain:0', 'model:0.0'],
      ['comp:0.1', 'brain:0', 'model:0.1'],
      ['comp:1.0', 'brain:1', 'model:1.0'],
      ['comp:1.1', 'brain:1', 'model:1.1'],
    ])
  })

  it('is deterministic: two calls over the same config are identical', () => {
    const cfg = canvasConfig()
    const a = graphFromConfig(cfg)
    const b = graphFromConfig(cfg)
    expect(b).toEqual(a)
    expect(JSON.stringify(b)).toBe(JSON.stringify(a))
  })
})

describe('privacy exclusion — the DATA on composition edges (SP3 paints it)', () => {
  it('private brain + cloud model → excluded; everything else → not', () => {
    // Espeja ADR-0015 as enforced at boot: policy.SelectModels never lets a
    // private brain dispatch to a cloud-locality model (the app fails loud on
    // an all-cloud private brain). Locality is DECLARED config data
    // (ModelConfig.locality, ADR-0015 §3), so the exclusion is derivable
    // client-side — /api/brains omits excluded models, the canvas must still
    // SHOW them, marked. Sensitivity has exactly two tiers (public|private,
    // config.go validateBrains) — no "confidential" tier exists to mirror.
    const g = graphFromConfig(canvasConfig())
    const byId = new Map(g.edges.map((e) => [e.id, e]))

    expect(byId.get('comp:0.0')?.excluded).toBe(false) // private + local → dispatches
    expect(byId.get('comp:0.1')?.excluded).toBe(true) // private + cloud → excluded
    expect(byId.get('comp:1.0')?.excluded).toBe(false) // public + local
    expect(byId.get('comp:1.1')?.excluded).toBe(false) // public + cloud → allowed

    // Route edges carry the flag too (uniform shape), always false.
    expect(byId.get('route:0')?.excluded).toBe(false)
  })
})

describe('canConnect — the mirror validity matrix (manual cables)', () => {
  const node = (kind: GraphNode['kind'], id: string): GraphNode =>
    ({ id, kind }) as GraphNode

  const channel = node('channel', 'channel:0')
  const channel2 = node('channel', 'channel:1')
  const brain = node('brain', 'brain:0')
  const brain2 = node('brain', 'brain:1')
  const model = node('model', 'model:0.0')
  const model2 = node('model', 'model:1.0')

  it('canal→cerebro is the ONLY valid manual connection', () => {
    // Espeja config.Validate: routes[] is the one channel↔brain link the
    // schema has (validateRoutes: routes[i].channel must name a channel,
    // routes[i].brain must name a brain). Drawing the cable IS creating the
    // route (FR-SCOPE-3: RouteForm is replaced by the edge).
    expect(canConnect(channel, brain)).toBe(true)
  })

  it('cerebro→modelo is NOT cable-made: composition is born from the drop', () => {
    // Espeja config.Validate: a model exists only INSIDE brains[i].models
    // (validateModels) — there is no config shape for a free-standing
    // brain→model link, so the palette drop mutates brains[i].models
    // (NC-6 resolved: no orphan addModel) and the cable is rejected.
    expect(canConnect(brain, model)).toBe(false)
  })

  it('every other pair is rejected', () => {
    // canal→modelo: no config shape relates a channel to a model directly —
    // routes bind channel→brain only (validateRoutes).
    expect(canConnect(channel, model)).toBe(false)
    // cerebro→cerebro: brains do not reference brains anywhere in the schema
    // (validateBrains: name/sensitivity/policy/dispatch/models — no peers).
    expect(canConnect(brain, brain2)).toBe(false)
    // modelo→cualquiera: models are leaf entries of brains[i].models
    // (validateModels) — they source nothing.
    expect(canConnect(model, brain)).toBe(false)
    expect(canConnect(model, channel)).toBe(false)
    expect(canConnect(model, model2)).toBe(false)
    // canal→canal: channels only appear as routes[i].channel (validateRoutes)
    // — never as a route target.
    expect(canConnect(channel, channel2)).toBe(false)
  })

  it('self-connections are rejected', () => {
    // Same node on both ends can express nothing in the config schema; also
    // the SP0 spike's isValidConnection precedent (source !== target).
    expect(canConnect(channel, channel)).toBe(false)
    expect(canConnect(brain, brain)).toBe(false)
    expect(canConnect(model, model)).toBe(false)
  })
})
