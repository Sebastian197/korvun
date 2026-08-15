// Pure editing model for the builder (ADR-0030 §4). The working copy is a deep clone
// of the GET /api/config baseline; edits are immutable transitions; the whole copy is
// POSTed. Everything here is a pure function so the guarantees (round-trip preserves
// untouched fields, dirty detection) are Vitest-testable without a DOM.

import type {
  Config,
  ModelConfig,
  BrainConfig,
  PersonaConfig,
  WebhookConfig,
  WebhookMapping,
  AgentConfig,
  ToolGrantConfig,
} from './schema'

/** Deep clone of a config baseline. */
export function clone(c: Config): Config {
  return structuredClone(c)
}

/** True when the working copy differs from its baseline. Both come from the same
 *  source (baseline + structuredClone), so a stable serialization compares safely. */
export function isDirty(working: Config, baseline: Config): boolean {
  return JSON.stringify(working) !== JSON.stringify(baseline)
}

/** pendingChangeCount powers the canvas header's "N cambios sin aplicar" (SP6,
 *  final-6). It counts DISTINCT entity changes across the three top-level lists
 *  (channels, brains, routes): each index that differs by value is one change,
 *  and each added/removed index (length delta) is one change. 0 for a clean
 *  working copy. Deterministic and stable-serialization based (same as isDirty). */
export function pendingChangeCount(baseline: Config, working: Config): number {
  const countList = <T>(base: T[], work: T[]): number => {
    let n = Math.abs(base.length - work.length) // added or removed entities
    for (let i = 0; i < Math.min(base.length, work.length); i++) {
      if (JSON.stringify(base[i]) !== JSON.stringify(work[i])) n++ // modified in place
    }
    return n
  }
  return (
    countList(baseline.channels, working.channels) +
    countList(baseline.brains, working.brains) +
    countList(baseline.routes, working.routes)
  )
}

/** A fresh model row (the "add model" default). model_id empty until the operator
 *  fills it (client checks non-empty; real existence is proven by Preflight). */
export function newModel(): ModelConfig {
  return { provider: 'ollama', model_id: '', locality: 'local' }
}

/** A fresh brain (the empty/first-run "create your first brain" default). */
export function newBrain(): BrainConfig {
  return {
    name: '',
    sensitivity: 'public',
    policy: { kind: 'priority' },
    dispatch: 'fanout',
    models: [],
  }
}

/** A fresh channel (the palette-drop default): telegram/polling with an empty
 *  token_env — the properties panel completes it; the server 400 is the
 *  authority on a still-empty value (builder-canvas SP3). */
export function newChannel(): Config['channels'][number] {
  return { type: 'telegram', mode: 'polling', token_env: '' }
}

export type ConfigAction =
  | {
      kind: 'setBrainField'
      brain: number
      field: 'name' | 'sensitivity' | 'dispatch'
      value: string
    }
  | { kind: 'setPolicyKind'; brain: number; value: string }
  | { kind: 'addModel'; brain: number }
  | { kind: 'updateModel'; brain: number; model: number; patch: Partial<ModelConfig> }
  | { kind: 'removeModel'; brain: number; model: number }
  | { kind: 'moveModel'; brain: number; from: number; to: number }
  | {
      kind: 'setChannelField'
      channel: number
      field: 'type' | 'mode' | 'token_env'
      value: string
    }
  | { kind: 'setRouteField'; route: number; field: 'channel' | 'brain'; value: string }
  | { kind: 'addBrain' }
  | { kind: 'removeBrain'; brain: number }
  | { kind: 'reset'; config: Config }
  // Canvas actions (builder-canvas SP2, FR-SER-3). The canvas mutates the SAME
  // working copy through these branches — the graph is a projection, never a
  // second state (NC-6).
  | { kind: 'addChannel' }
  | { kind: 'removeChannel'; channel: number }
  | { kind: 'connectRoute'; channel: number; brain: number }
  | { kind: 'disconnectRoute'; route: number }
  | { kind: 'dropModel'; brain: number; model: ModelConfig }
  | { kind: 'setPersonaField'; brain: number; field: keyof PersonaConfig; value: string }
  // SP4: the webhook block is editable from the properties panel. mapping is
  // deliberately NOT a panel field in v1 (server defaults rule, ADR-0038 §1).
  | {
      kind: 'setWebhookField'
      channel: number
      field: 'bind' | 'path' | 'outbound_url' | 'outbound_token_env'
      value: string
    }
  // SP6: the webhook mapping's six fields, editable from the panel.
  | { kind: 'setWebhookMappingField'; channel: number; field: keyof WebhookMapping; value: string }
  // SP6 governance panel — the agent-brain "Herramientas y skills" section
  // (ADR-0041). Every branch mutates ONLY brains[i].agent and preserves the
  // rest (the round-trip guarantee); the serialization matrix is the exact
  // espejo of config.AgentConfig.
  | { kind: 'setToolMode'; brain: number; tool: string; mode: 'allow' | 'shadow' | 'deny' }
  | { kind: 'setToolChannels'; brain: number; tool: string; channels: string[] }
  | {
      kind: 'setToolAttrOverride'
      brain: number
      tool: string
      attr: 'sensitive' | 'network'
      value: boolean | undefined
    }
  | {
      kind: 'setCageField'
      brain: number
      cage: 'read_file' | 'http_fetch' | 'webhook_call'
      field: string
      value: string | number
    }
  | { kind: 'addCageHost'; brain: number; cage: 'http_fetch' | 'webhook_call' }
  | {
      kind: 'setCageHost'
      brain: number
      cage: 'http_fetch' | 'webhook_call'
      index: number
      value: string
    }
  | { kind: 'removeCageHost'; brain: number; cage: 'http_fetch' | 'webhook_call'; index: number }
  | {
      kind: 'setSkillsField'
      brain: number
      field: 'skills_dir' | 'skills_body_budget'
      value: string | number
    }

// Immutable helpers: replace one element of an array without touching the rest.
function replaceAt<T>(arr: T[], i: number, next: T): T[] {
  return arr.map((v, j) => (j === i ? next : v))
}

function editBrain(
  c: Config,
  i: number,
  next: (b: Config['brains'][number]) => Config['brains'][number],
): Config {
  return { ...c, brains: replaceAt(c.brains, i, next(c.brains[i])) }
}

/** Edit a brain's agent block (SP6). A no-op when the brain has no agent block —
 *  the panel only mounts these actions for agent brains, so this guard is
 *  defensive. */
function editAgent(c: Config, i: number, next: (a: AgentConfig) => AgentConfig): Config {
  return editBrain(c, i, (b) => (b.agent ? { ...b, agent: next(b.agent) } : b))
}

/** Upsert a tri-state grant, preserving its channels; setting a mode never
 *  drops an existing channel scope. */
function upsertGrant(
  agent: AgentConfig,
  tool: string,
  patch: Partial<ToolGrantConfig>,
): AgentConfig {
  const grants = agent.governance ?? []
  const idx = grants.findIndex((g) => g.tool === tool)
  const nextGrants =
    idx >= 0
      ? replaceAt(grants, idx, { ...grants[idx], ...patch })
      : [...grants, { tool, mode: 'allow', ...patch } as ToolGrantConfig]
  return { ...agent, governance: nextGrants }
}

/** The one pure reducer for the whole edit surface. Every branch returns a NEW config
 *  and preserves every field it does not touch (the round-trip guarantee). */
export function configReducer(state: Config, action: ConfigAction): Config {
  switch (action.kind) {
    case 'reset':
      // Re-baseline the working copy to a freshly fetched config (after a succeeded
      // reload) so dirty clears exactly.
      return clone(action.config)
    case 'addBrain':
      return { ...state, brains: [...state.brains, newBrain()] }
    case 'removeBrain': {
      // Delete one brain AND cascade the routes that name it (SP5): a dangling
      // routes[i].brain would 400 on the next Apply (validateRoutes). The
      // brain's models live inside it, so they go with it. Everything else is
      // byte-identical. Removing the last brain leaves an empty list → the UI
      // shows the empty/first-run state.
      const gone = state.brains[action.brain]?.name
      return {
        ...state,
        brains: state.brains.filter((_, j) => j !== action.brain),
        routes: state.routes.filter((r) => r.brain !== gone),
      }
    }
    case 'setBrainField':
      return editBrain(state, action.brain, (b) => ({ ...b, [action.field]: action.value }))
    case 'setPolicyKind':
      return editBrain(state, action.brain, (b) => ({
        ...b,
        policy: { ...b.policy, kind: action.value },
      }))
    case 'addModel':
      return editBrain(state, action.brain, (b) => ({ ...b, models: [...b.models, newModel()] }))
    case 'updateModel':
      return editBrain(state, action.brain, (b) => ({
        ...b,
        models: replaceAt(b.models, action.model, { ...b.models[action.model], ...action.patch }),
      }))
    case 'removeModel':
      return editBrain(state, action.brain, (b) => ({
        ...b,
        models: b.models.filter((_, j) => j !== action.model),
      }))
    case 'moveModel':
      return editBrain(state, action.brain, (b) => ({
        ...b,
        models: move(b.models, action.from, action.to),
      }))
    case 'setChannelField':
      return {
        ...state,
        channels: replaceAt(state.channels, action.channel, {
          ...state.channels[action.channel],
          [action.field]: action.value,
        }),
      }
    case 'setRouteField':
      return {
        ...state,
        routes: replaceAt(state.routes, action.route, {
          ...state.routes[action.route],
          [action.field]: action.value,
        }),
      }
    case 'addChannel':
      // A channel palette block dropped on the canvas (SP3): the fresh
      // telegram/polling default; the panel completes token_env.
      return { ...state, channels: [...state.channels, newChannel()] }
    case 'removeChannel': {
      // Delete one channel AND cascade the routes that name it (SP5). A route
      // names a channel by its TYPE (config.go validateChannels, names[ch.Type]),
      // so the cascade matches on type — same dangling-route 400 prevention as
      // removeBrain.
      const gone = state.channels[action.channel]?.type
      return {
        ...state,
        channels: state.channels.filter((_, j) => j !== action.channel),
        routes: state.routes.filter((r) => r.channel !== gone),
      }
    }
    case 'connectRoute':
      // Drawing the canal→cerebro edge IS creating the route (FR-SCOPE-3). A
      // route names the channel by its TYPE — the registration rule of
      // config.go validateChannels (names[ch.Type]) — and the brain by name.
      return {
        ...state,
        routes: [
          ...state.routes,
          { channel: state.channels[action.channel].type, brain: state.brains[action.brain].name },
        ],
      }
    case 'disconnectRoute':
      // Deleting the edge removes exactly that route. Zero routes with
      // channels present will 400 on Apply (validateRoutes) — the honest
      // mid-edit state the error mapping surfaces, never masked here.
      return { ...state, routes: state.routes.filter((_, j) => j !== action.route) }
    case 'dropModel':
      // The palette drop targets a BRAIN (NC-6: no orphan models — a model
      // exists only as brains[i].models[j], config.go validateModels). Cloned
      // so a caller-held reference cannot alias the working copy.
      return editBrain(state, action.brain, (b) => ({
        ...b,
        models: [...b.models, structuredClone(action.model)],
      }))
    case 'setPersonaField':
      // First edit materializes the block with only the edited field; later
      // edits patch that field, preserving siblings (SP1 validatePersona:
      // optional, partial, additive).
      return editBrain(state, action.brain, (b) => ({
        ...b,
        persona: { ...(b.persona ?? {}), [action.field]: action.value },
      }))
    case 'setWebhookField': {
      // The persona pattern for the webhook block (ADR-0038 §1): the first
      // edit on a blockless webhook channel materializes it; later edits
      // patch one field preserving siblings. Completeness (outbound_url
      // required) stays the server 400's call.
      const webhook: WebhookConfig = {
        ...(state.channels[action.channel].webhook ?? {}),
        [action.field]: action.value,
      }
      return {
        ...state,
        channels: replaceAt(state.channels, action.channel, {
          ...state.channels[action.channel],
          webhook,
        }),
      }
    }
    case 'setWebhookMappingField': {
      // SP6: patch one of the six mapping fields, materializing the mapping
      // object (and the block) on first edit; server defaults fill the rest
      // (EffectiveMapping) so the mirror never inflates the file.
      const current = state.channels[action.channel].webhook ?? {}
      const mapping: WebhookMapping = { ...(current.mapping ?? {}), [action.field]: action.value }
      return {
        ...state,
        channels: replaceAt(state.channels, action.channel, {
          ...state.channels[action.channel],
          webhook: { ...current, mapping },
        }),
      }
    }
    case 'setToolMode':
      return editAgent(state, action.brain, (a) =>
        upsertGrant(a, action.tool, { mode: action.mode }),
      )
    case 'setToolChannels':
      return editAgent(state, action.brain, (a) => {
        // Upsert the grant with the new scope; an EMPTY list clears the
        // restriction (the channels key rides omitempty, so it is dropped).
        const grants = a.governance ?? []
        const idx = grants.findIndex((g) => g.tool === action.tool)
        const base: ToolGrantConfig = idx >= 0 ? grants[idx] : { tool: action.tool, mode: 'allow' }
        const next: ToolGrantConfig =
          action.channels.length > 0
            ? { ...base, channels: action.channels }
            : { tool: base.tool, mode: base.mode }
        return { ...a, governance: idx >= 0 ? replaceAt(grants, idx, next) : [...grants, next] }
      })
    case 'setToolAttrOverride':
      return editAgent(state, action.brain, (a) => {
        const attrs = { ...(a.tool_attrs ?? {}) }
        const cur = { ...(attrs[action.tool] ?? {}) }
        if (action.value === undefined) delete cur[action.attr]
        else cur[action.attr] = action.value
        // Clearing the last field of a tool removes its entry; clearing the
        // last entry removes tool_attrs entirely — back to the house default.
        if (Object.keys(cur).length === 0) delete attrs[action.tool]
        else attrs[action.tool] = cur
        if (Object.keys(attrs).length === 0) {
          const rest = { ...a }
          delete rest.tool_attrs
          return rest
        }
        return { ...a, tool_attrs: attrs }
      })
    case 'setCageField':
      return editAgent(state, action.brain, (a) => {
        const cage = { ...((a[action.cage] as Record<string, unknown> | undefined) ?? {}) }
        cage[action.field] = action.value
        return { ...a, [action.cage]: cage }
      })
    case 'addCageHost':
      return editAgent(state, action.brain, (a) => {
        const cur = a[action.cage] ?? { allow_hosts: [] }
        return { ...a, [action.cage]: { ...cur, allow_hosts: [...cur.allow_hosts, ''] } }
      })
    case 'setCageHost':
      return editAgent(state, action.brain, (a) => {
        const cur = a[action.cage] ?? { allow_hosts: [] }
        return {
          ...a,
          [action.cage]: {
            ...cur,
            allow_hosts: replaceAt(cur.allow_hosts, action.index, action.value),
          },
        }
      })
    case 'removeCageHost':
      return editAgent(state, action.brain, (a) => {
        const cur = a[action.cage] ?? { allow_hosts: [] }
        return {
          ...a,
          [action.cage]: {
            ...cur,
            allow_hosts: cur.allow_hosts.filter((_, j) => j !== action.index),
          },
        }
      })
    case 'setSkillsField':
      return editAgent(state, action.brain, (a) => ({ ...a, [action.field]: action.value }))
  }
}

function move<T>(arr: T[], from: number, to: number): T[] {
  if (to < 0 || to >= arr.length || from === to) return arr
  const next = arr.slice()
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}
