// Pure editing model for the builder (ADR-0030 §4). The working copy is a deep clone
// of the GET /api/config baseline; edits are immutable transitions; the whole copy is
// POSTed. Everything here is a pure function so the guarantees (round-trip preserves
// untouched fields, dirty detection) are Vitest-testable without a DOM.

import type { Config, ModelConfig, BrainConfig, PersonaConfig, WebhookConfig } from './schema'

/** Deep clone of a config baseline. */
export function clone(c: Config): Config {
  return structuredClone(c)
}

/** True when the working copy differs from its baseline. Both come from the same
 *  source (baseline + structuredClone), so a stable serialization compares safely. */
export function isDirty(working: Config, baseline: Config): boolean {
  return JSON.stringify(working) !== JSON.stringify(baseline)
}

/** A fresh model row (the "add model" default). model_id empty until the operator
 *  fills it (client checks non-empty; real existence is proven by Preflight). */
export function newModel(): ModelConfig {
  return { provider: 'ollama', model_id: '', locality: 'local' }
}

/** A fresh brain (the empty/first-run "create your first brain" default). */
export function newBrain(): BrainConfig {
  return { name: '', sensitivity: 'public', policy: { kind: 'priority' }, dispatch: 'fanout', models: [] }
}

/** A fresh channel (the palette-drop default): telegram/polling with an empty
 *  token_env — the properties panel completes it; the server 400 is the
 *  authority on a still-empty value (builder-canvas SP3). */
export function newChannel(): Config['channels'][number] {
  return { type: 'telegram', mode: 'polling', token_env: '' }
}

export type ConfigAction =
  | { kind: 'setBrainField'; brain: number; field: 'name' | 'sensitivity' | 'dispatch'; value: string }
  | { kind: 'setPolicyKind'; brain: number; value: string }
  | { kind: 'addModel'; brain: number }
  | { kind: 'updateModel'; brain: number; model: number; patch: Partial<ModelConfig> }
  | { kind: 'removeModel'; brain: number; model: number }
  | { kind: 'moveModel'; brain: number; from: number; to: number }
  | { kind: 'setChannelField'; channel: number; field: 'type' | 'mode' | 'token_env'; value: string }
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

// Immutable helpers: replace one element of an array without touching the rest.
function replaceAt<T>(arr: T[], i: number, next: T): T[] {
  return arr.map((v, j) => (j === i ? next : v))
}

function editBrain(c: Config, i: number, next: (b: Config['brains'][number]) => Config['brains'][number]): Config {
  return { ...c, brains: replaceAt(c.brains, i, next(c.brains[i])) }
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
      return editBrain(state, action.brain, (b) => ({ ...b, policy: { ...b.policy, kind: action.value } }))
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
      return editBrain(state, action.brain, (b) => ({ ...b, models: move(b.models, action.from, action.to) }))
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
  }
}

function move<T>(arr: T[], from: number, to: number): T[] {
  if (to < 0 || to >= arr.length || from === to) return arr
  const next = arr.slice()
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}
