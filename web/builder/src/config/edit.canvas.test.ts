import { describe, it, expect } from 'vitest'
import type { Config, ModelConfig } from './schema'
import { clone, configReducer } from './edit'

// SP2 RED (builder-canvas FR-SER-3, NC-4/NC-6 resolved): the canvas mutations
// as NEW pure reducer branches over the SAME working copy — connect/disconnect
// a route by drawing/deleting the canal→cerebro edge, drop a model ONTO a
// brain (never orphan, NC-6), edit the persona from the properties panel. In
// every branch: round-trip without loss — fields the canvas does not edit stay
// byte-identical (the house structuredClone pattern of edit.test.ts), and the
// baseline is never mutated.
//
// RED note: these action kinds do not exist in ConfigAction yet — `npm run
// typecheck` rejects them and at runtime the reducer's switch has no branch,
// so every assert below fails. That failure IS the red.

function baseline(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DISCORD' },
    ],
    brains: [
      {
        name: 'support',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama', 'groq'] },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
      },
      { name: 'general', sensitivity: 'public', policy: { kind: 'consensus' }, dispatch: 'sequential', models: [] },
    ],
    routes: [{ channel: 'telegram', brain: 'support' }],
    storage: { path: '/data/korvun.db' },
    observability: { enabled: true, addr: '127.0.0.1:2112' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('connectRoute — drawing the canal→cerebro edge adds the exact route', () => {
  it('appends {channel: <type>, brain: <name>} and touches nothing else', () => {
    const base = baseline()
    // Espeja config.Validate: a channel registers under its TYPE name
    // (validateChannels: names[ch.Type] = true) and routes[i].channel /
    // routes[i].brain must name configured entries (validateRoutes) — so the
    // route the edge creates references channels[1].type and brains[1].name.
    const wc = configReducer(clone(base), { kind: 'connectRoute', channel: 1, brain: 1 })
    expect(wc.routes).toEqual([
      { channel: 'telegram', brain: 'support' },
      { channel: 'discord', brain: 'general' },
    ])
    // Round-trip: everything the edge does not create stays byte-identical.
    expect(wc.channels).toEqual(base.channels)
    expect(wc.brains).toEqual(base.brains)
    expect(wc.storage).toEqual(base.storage)
    expect(wc.observability).toEqual(base.observability)
    expect(wc.admin).toEqual(base.admin)
    // …and the baseline itself was never mutated (structuredClone pattern).
    expect(base.routes).toEqual([{ channel: 'telegram', brain: 'support' }])
  })
})

describe('disconnectRoute — deleting the edge removes exactly that route', () => {
  it('removes routes[i] and preserves every other field', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'disconnectRoute', route: 0 })
    // NOTE the honest consequence (espeja validateRoutes): with channels
    // present and zero routes the server rejects on Apply — the canvas may
    // reach that state mid-edit; the 400 mapping surfaces it (FR-HOT).
    expect(wc.routes).toEqual([])
    expect(wc.channels).toEqual(base.channels)
    expect(wc.brains).toEqual(base.brains)
    expect(wc.storage).toEqual(base.storage)
    expect(wc.admin).toEqual(base.admin)
    expect(base.routes).toHaveLength(1) // baseline intact
  })
})

describe('dropModel — dropping a palette model ONTO a brain (NC-6: never orphan)', () => {
  it('appends the exact entry to that brain.models only', () => {
    const base = baseline()
    // Espeja config.Validate: a model exists only as brains[i].models[j]
    // (validateModels) — the drop TARGET is the brain, there is no
    // free-standing model in the schema (NC-6 killed the orphan addModel).
    const dropped: ModelConfig = {
      provider: 'groq',
      model_id: 'llama-3.3-70b',
      locality: 'cloud',
      api_key_env: 'GROQ_KEY',
    }
    const wc = configReducer(clone(base), { kind: 'dropModel', brain: 1, model: dropped })
    expect(wc.brains[1].models).toEqual([dropped])
    // The OTHER brain's models and the rest of the config: byte-identical.
    expect(wc.brains[0]).toEqual(base.brains[0])
    expect(wc.channels).toEqual(base.channels)
    expect(wc.routes).toEqual(base.routes)
    expect(wc.storage).toEqual(base.storage)
    expect(base.brains[1].models).toEqual([]) // baseline intact
  })
})

describe('setPersonaField — the properties panel mutates the persona block', () => {
  it('creates the block on first edit with only the edited field', () => {
    const base = baseline()
    // Espeja config.Validate: persona is OPTIONAL and additive
    // (validatePersona accepts a partial block; absent fields are simply
    // empty). First edit materializes it; nothing else appears.
    const wc = configReducer(clone(base), {
      kind: 'setPersonaField',
      brain: 0,
      field: 'display_name',
      value: 'Nova',
    })
    expect(wc.brains[0].persona).toEqual({ display_name: 'Nova' })
    expect(wc.brains[1].persona).toBeUndefined()
    expect(base.brains[0].persona).toBeUndefined() // baseline intact
  })

  it('updates one field of an existing block, preserving the others', () => {
    const base = baseline()
    base.brains[0].persona = { display_name: 'Nova', tone: 'warm', language: 'es-ES' }
    const wc = configReducer(clone(base), {
      kind: 'setPersonaField',
      brain: 0,
      field: 'instructions',
      value: 'Cite sources.',
    })
    expect(wc.brains[0].persona).toEqual({
      display_name: 'Nova',
      tone: 'warm',
      language: 'es-ES',
      instructions: 'Cite sources.',
    })
    // Round-trip: the rest of the brain and the config are byte-identical.
    expect(wc.brains[0].models).toEqual(base.brains[0].models)
    expect(wc.brains[0].policy).toEqual(base.brains[0].policy)
    expect(wc.channels).toEqual(base.channels)
    expect(wc.routes).toEqual(base.routes)
  })
})
