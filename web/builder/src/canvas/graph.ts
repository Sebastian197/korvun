// Pure config→graph projection for the builder canvas (builder-canvas SP2,
// FR-CANVAS/FR-SER). Normative layout: the final-6 mockup
// (design-drafts/final-6-builder-estados.png) — three columns
// canales | cerebros | modelos, model nodes PER BRAIN ENTRY, route edges
// canal→cerebro and composition edges cerebro→modelo. The graph is a derived
// VIEW: positions are computed from config indices and NEVER persisted (NC-6
// resolved) — the config stays the only truth; mutations go through
// configReducer, not through the graph.

import type { Config } from '../config/schema'

/** One canvas node. IDs are the stable keys of the projection:
 *  channel:<i> / brain:<i> / model:<brainIdx>.<modelIdx> (config indices). */
export interface GraphNode {
  id: string
  kind: 'channel' | 'brain' | 'model'
  /** 0 = canales, 1 = cerebros, 2 = modelos (final-6 column order). */
  column: 0 | 1 | 2
  /** Stable row inside the column, in config-index order. */
  row: number
}

/** One canvas edge. `excluded` is UNIFORM across kinds: true only on a
 *  composition edge whose brain is private and whose model is cloud —
 *  the ADR-0015 rule (policy.SelectModels) replicated as DATA; SP3 paints it
 *  (gray dashed), the projection never drops the edge. */
export interface GraphEdge {
  id: string
  kind: 'route' | 'composition'
  source: string
  target: string
  excluded: boolean
}

export interface BuilderGraph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

/** graphFromConfig projects a config onto the canvas graph. PURE and
 *  deterministic: same config in, identical graph out — columns and rows by
 *  config index, IDs from the stable schemes above. Route endpoints resolve by
 *  the REAL registration rule: a channel registers under its TYPE name
 *  (config.go validateChannels, names[ch.Type]) and a route names a configured
 *  channel and brain (validateRoutes); an unresolvable endpoint (invalid
 *  mid-edit config) simply emits no edge — the server 400 is the authority. */
export function graphFromConfig(cfg: Config): BuilderGraph {
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []

  cfg.channels.forEach((_, i) => {
    nodes.push({ id: `channel:${i}`, kind: 'channel', column: 0, row: i })
  })
  cfg.brains.forEach((_, i) => {
    nodes.push({ id: `brain:${i}`, kind: 'brain', column: 1, row: i })
  })

  // Model nodes are PER BRAIN ENTRY (final-6: the same model_id appears once
  // under each brain that carries it), rows sequential over (brain, model).
  let modelRow = 0
  cfg.brains.forEach((b, bi) => {
    b.models.forEach((m, mi) => {
      nodes.push({ id: `model:${bi}.${mi}`, kind: 'model', column: 2, row: modelRow })
      modelRow++
      edges.push({
        id: `comp:${bi}.${mi}`,
        kind: 'composition',
        source: `brain:${bi}`,
        target: `model:${bi}.${mi}`,
        excluded: b.sensitivity === 'private' && m.locality === 'cloud',
      })
    })
  })

  cfg.routes.forEach((r, ri) => {
    const ci = cfg.channels.findIndex((c) => c.type === r.channel)
    const bi = cfg.brains.findIndex((b) => b.name === r.brain)
    if (ci < 0 || bi < 0) return
    edges.push({
      id: `route:${ri}`,
      kind: 'route',
      source: `channel:${ci}`,
      target: `brain:${bi}`,
      excluded: false,
    })
  })

  return { nodes, edges }
}

/** canConnect is the manual-cable validity matrix, mirroring config.Validate:
 *  canal→cerebro is the ONLY drawable connection (a route, validateRoutes —
 *  FR-SCOPE-3: the edge IS the route). Composition cerebro→modelo is born from
 *  the palette drop (brains[i].models, validateModels; NC-6 — never a free
 *  cable), and no other pair (nor a self-connection) has any config shape. */
export function canConnect(source: GraphNode, target: GraphNode): boolean {
  return source.kind === 'channel' && target.kind === 'brain' && source.id !== target.id
}
