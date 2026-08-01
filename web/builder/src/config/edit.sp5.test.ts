import { describe, it, expect } from 'vitest'
import type { Config } from './schema'
import { clone, configReducer } from './edit'

// SP5 RED (builder-canvas): the builder learns to DELETE. Node deletion is a
// domain CASCADE mirrored from config.Validate's referential rules — deleting
// a brain or a channel must also drop the routes that reference it, or the
// next Apply would 400 on a dangling routes[i] (validateRoutes: the route's
// channel/brain must name a configured entry). Every branch keeps the house
// structuredClone round-trip: untouched fields byte-identical, baseline intact.
//
// RED note (house precedent): `removeChannel` does not exist in ConfigAction
// yet (typecheck rejects it); `removeBrain` exists but does NOT cascade routes
// today, so its cascade assertions fail. Both are the red.

function baseline(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DISCORD' },
    ],
    brains: [
      {
        name: 'asistente',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama', 'groq'] },
        dispatch: 'fanout',
        models: [
          { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
          { provider: 'groq', model_id: 'llama-3.3-70b', locality: 'cloud', api_key_env: 'GROQ_KEY' },
        ],
      },
      { name: 'general', sensitivity: 'public', policy: { kind: 'consensus' }, dispatch: 'fanout', models: [] },
    ],
    routes: [
      { channel: 'telegram', brain: 'asistente' },
      { channel: 'discord', brain: 'general' },
    ],
    storage: { path: '/data/korvun.db' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('removeBrain — cascade: the brain, its models (inside it) and its routes', () => {
  it('drops the brain AND every route that names it; the rest byte-identical', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'removeBrain', brain: 0 })
    // The brain (with its two models, which live inside it) is gone…
    expect(wc.brains.map((b) => b.name)).toEqual(['general'])
    // …and its route (telegram→asistente) cascaded out — the dangling-route
    // 400 the domain would raise is prevented at the source (validateRoutes).
    expect(wc.routes).toEqual([{ channel: 'discord', brain: 'general' }])
    // Round-trip: channels, storage, admin, the surviving brain untouched.
    expect(wc.channels).toEqual(base.channels)
    expect(wc.storage).toEqual(base.storage)
    expect(wc.admin).toEqual(base.admin)
    expect(base.brains).toHaveLength(2) // baseline intact
  })
})

describe('removeChannel — cascade: the channel and its routes', () => {
  it('drops the channel AND every route that names it (by its TYPE)', () => {
    const base = baseline()
    // A channel is named in routes by its TYPE (config.go validateChannels,
    // names[ch.Type]); removing channels[1] (discord) must drop discord→general.
    const wc = configReducer(clone(base), { kind: 'removeChannel', channel: 1 })
    expect(wc.channels.map((c) => c.type)).toEqual(['telegram'])
    expect(wc.routes).toEqual([{ channel: 'telegram', brain: 'asistente' }])
    expect(wc.brains).toEqual(base.brains)
    expect(wc.admin).toEqual(base.admin)
    expect(base.channels).toHaveLength(2) // baseline intact
  })
})

describe('removeModel — only that one entry (no route touch)', () => {
  it('drops brains[i].models[j] and nothing else', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'removeModel', brain: 0, model: 1 })
    expect(wc.brains[0].models.map((m) => m.model_id)).toEqual(['llama3.2:1b'])
    expect(wc.routes).toEqual(base.routes) // models are not routes
    expect(wc.channels).toEqual(base.channels)
    expect(base.brains[0].models).toHaveLength(2) // baseline intact
  })
})

describe('disconnectRoute — removing one route by index (the cable delete)', () => {
  it('drops exactly routes[i], everything else byte-identical', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'disconnectRoute', route: 0 })
    expect(wc.routes).toEqual([{ channel: 'discord', brain: 'general' }])
    expect(wc.brains).toEqual(base.brains)
    expect(wc.channels).toEqual(base.channels)
    expect(base.routes).toHaveLength(2) // baseline intact
  })
})