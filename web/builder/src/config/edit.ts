// Pure editing model for the builder (ADR-0030 §4). The working copy is a deep clone
// of the GET /api/config baseline; edits are immutable transitions; the whole copy is
// POSTed. Everything here is a pure function so the guarantees (round-trip preserves
// untouched fields, dirty detection) are Vitest-testable without a DOM.

import type { Config, ModelConfig, BrainConfig } from './schema'

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
    case 'removeBrain':
      // Symmetric with removeModel: drop one brain, preserve everything else
      // (channels, routes, storage, admin, the other brains). Removing the last
      // brain leaves an empty list → the UI shows the empty/first-run state.
      return { ...state, brains: state.brains.filter((_, j) => j !== action.brain) }
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
  }
}

function move<T>(arr: T[], from: number, to: number): T[] {
  if (to < 0 || to >= arr.length || from === to) return arr
  const next = arr.slice()
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}
