import { describe, it, expect } from 'vitest'
import type { Config } from './schema'
import { clone, isDirty, configReducer, newModel } from './edit'

// A realistic baseline with fields the edit surface does NOT touch (storage,
// observability, admin, a second brain, routes). The round-trip guard asserts these
// survive every edit — it BITES if the reducer ever drops a field while editing one.
function baseline(): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'support',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama', 'groq'] },
        dispatch: 'fanout',
        models: [
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
          { provider: 'groq', model_id: 'llama-3.3-70b', locality: 'cloud', api_key_env: 'GROQ_KEY' },
        ],
      },
      { name: 'other', sensitivity: 'public', policy: { kind: 'consensus' }, dispatch: 'sequential', models: [] },
    ],
    routes: [{ channel: 'telegram', brain: 'support' }],
    storage: { path: '/data/korvun.db' },
    observability: { enabled: true, addr: '127.0.0.1:2112' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('clone + dirty', () => {
  it('clone is deep: mutating the copy does not touch the baseline', () => {
    const base = baseline()
    const wc = clone(base)
    wc.brains[0].name = 'changed'
    expect(base.brains[0].name).toBe('support')
  })

  it('isDirty: false after clone, true after an edit, false when reverted', () => {
    const base = baseline()
    let wc = clone(base)
    expect(isDirty(wc, base)).toBe(false)
    wc = configReducer(wc, { kind: 'setBrainField', brain: 0, field: 'sensitivity', value: 'public' })
    expect(isDirty(wc, base)).toBe(true)
    wc = configReducer(wc, { kind: 'setBrainField', brain: 0, field: 'sensitivity', value: 'private' })
    expect(isDirty(wc, base)).toBe(false)
  })
})

describe('configReducer — round-trip preserves untouched fields', () => {
  it('editing one brain field keeps channels, routes, storage, admin, and the other brain intact', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'setBrainField', brain: 0, field: 'sensitivity', value: 'public' })
    expect(wc.brains[0].sensitivity).toBe('public') // the edit applied
    // everything else is byte-identical to the baseline
    expect(wc.channels).toEqual(base.channels)
    expect(wc.routes).toEqual(base.routes)
    expect(wc.storage).toEqual(base.storage)
    expect(wc.observability).toEqual(base.observability)
    expect(wc.admin).toEqual(base.admin)
    expect(wc.brains[1]).toEqual(base.brains[1])
    expect(wc.brains[0].models).toEqual(base.brains[0].models) // unrelated field preserved
    expect(wc.brains[0].policy).toEqual(base.brains[0].policy)
  })

  it('a full edit sequence yields a config with exactly the edits and nothing dropped', () => {
    const base = baseline()
    let wc = clone(base)
    wc = configReducer(wc, { kind: 'setBrainField', brain: 0, field: 'name', value: 'support-v2' })
    wc = configReducer(wc, { kind: 'setPolicyKind', brain: 0, value: 'consensus' })
    wc = configReducer(wc, { kind: 'setChannelField', channel: 0, field: 'token_env', value: 'NEW_TG' })
    wc = configReducer(wc, { kind: 'setRouteField', route: 0, field: 'brain', value: 'support-v2' })

    expect(wc.brains[0].name).toBe('support-v2')
    expect(wc.brains[0].policy.kind).toBe('consensus')
    expect(wc.brains[0].policy.order).toEqual(['ollama', 'groq']) // order NOT dropped by the kind edit
    expect(wc.channels[0].token_env).toBe('NEW_TG')
    expect(wc.routes[0].brain).toBe('support-v2')
    // untouched
    expect(wc.storage).toEqual(base.storage)
    expect(wc.admin).toEqual(base.admin)
    expect(wc.brains[1]).toEqual(base.brains[1])
  })
})

describe('configReducer — model rows (ADR-0030 §7)', () => {
  it('addModel appends a default row without touching existing models', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'addModel', brain: 0 })
    expect(wc.brains[0].models).toHaveLength(3)
    expect(wc.brains[0].models[2]).toEqual(newModel())
    expect(wc.brains[0].models.slice(0, 2)).toEqual(base.brains[0].models)
  })

  it('updateModel patches one row only', () => {
    const wc = configReducer(clone(baseline()), {
      kind: 'updateModel',
      brain: 0,
      model: 1,
      patch: { model_id: 'llama-3.1-8b', api_key_env: 'GROQ_KEY_2' },
    })
    expect(wc.brains[0].models[1].model_id).toBe('llama-3.1-8b')
    expect(wc.brains[0].models[1].api_key_env).toBe('GROQ_KEY_2')
    expect(wc.brains[0].models[1].provider).toBe('groq') // untouched
    expect(wc.brains[0].models[0].model_id).toBe('llama3.2:1b') // other row untouched
  })

  it('removeModel drops one row', () => {
    const wc = configReducer(clone(baseline()), { kind: 'removeModel', brain: 0, model: 0 })
    expect(wc.brains[0].models).toHaveLength(1)
    expect(wc.brains[0].models[0].provider).toBe('groq')
  })

  it('moveModel reorders; an out-of-range move is a no-op', () => {
    const base = baseline()
    const up = configReducer(clone(base), { kind: 'moveModel', brain: 0, from: 1, to: 0 })
    expect(up.brains[0].models.map((m) => m.provider)).toEqual(['groq', 'ollama'])
    const noop = configReducer(clone(base), { kind: 'moveModel', brain: 0, from: 0, to: 5 })
    expect(noop.brains[0].models).toEqual(base.brains[0].models)
  })
})

describe('configReducer — removeBrain (functional symmetry with removeModel)', () => {
  it('removes the named brain and preserves EVERYTHING else', () => {
    const base = baseline() // two brains: support, other
    const wc = configReducer(clone(base), { kind: 'removeBrain', brain: 0 })
    expect(wc.brains).toHaveLength(1)
    expect(wc.brains[0]).toEqual(base.brains[1]) // the surviving brain is intact
    // the round-trip guarantee: nothing else dropped
    expect(wc.channels).toEqual(base.channels)
    expect(wc.routes).toEqual(base.routes)
    expect(wc.storage).toEqual(base.storage)
    expect(wc.observability).toEqual(base.observability)
    expect(wc.admin).toEqual(base.admin)
  })

  it('marks the config dirty after a remove', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'removeBrain', brain: 1 })
    expect(isDirty(wc, base)).toBe(true)
  })

  it('removing the last brain yields an empty list (→ the UI empty/first-run state)', () => {
    const one: Config = { ...baseline(), brains: [baseline().brains[0]] }
    const wc = configReducer(clone(one), { kind: 'removeBrain', brain: 0 })
    expect(wc.brains).toEqual([])
    expect(wc.channels).toEqual(one.channels) // rest still intact
  })
})
