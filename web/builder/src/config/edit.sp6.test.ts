import { describe, it, expect } from 'vitest'
import type { Config } from './schema'
import { clone, configReducer, pendingChangeCount } from './edit'

// SP6 RED (builder-canvas): the webhook MAPPING becomes editable (its six
// fields), and a deterministic pendingChangeCount powers the canvas header's
// "N cambios sin aplicar" counter (final-6). Both keep the house structuredClone
// round-trip. RED note: `setWebhookMappingField` and `pendingChangeCount` do
// not exist yet — typecheck + missing branch are the red.

function baseline(): Config {
  return {
    channels: [
      {
        type: 'webhook',
        mode: '',
        token_env: 'KORVUN_HOOK',
        webhook: { outbound_url: 'https://x.example/r', mapping: { text: 'body' } },
      },
    ],
    brains: [
      {
        name: 'support',
        sensitivity: 'public',
        policy: { kind: 'priority' },
        dispatch: 'fanout',
        models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
      },
    ],
    routes: [{ channel: 'webhook', brain: 'support' }],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('setWebhookMappingField — the six mapping fields are editable', () => {
  const fields = ['sender_id', 'sender_name', 'text', 'media_url', 'media_type', 'conversation_id'] as const

  it('patches one mapping field, preserving the sibling mapping + block fields', () => {
    const base = baseline()
    const wc = configReducer(clone(base), {
      kind: 'setWebhookMappingField',
      channel: 0,
      field: 'sender_id',
      value: 'uid',
    })
    expect(wc.channels[0].webhook?.mapping).toEqual({ text: 'body', sender_id: 'uid' })
    // The block's own fields (outbound_url) and the rest of the config stand.
    expect(wc.channels[0].webhook?.outbound_url).toBe('https://x.example/r')
    expect(wc.brains).toEqual(base.brains)
    expect(base.channels[0].webhook?.mapping).toEqual({ text: 'body' }) // baseline intact
  })

  it('materializes the mapping object on a block that has none', () => {
    const base = baseline()
    base.channels[0].webhook = { outbound_url: 'https://x.example/r' } // no mapping
    const wc = configReducer(clone(base), {
      kind: 'setWebhookMappingField',
      channel: 0,
      field: 'conversation_id',
      value: 'cid',
    })
    expect(wc.channels[0].webhook?.mapping).toEqual({ conversation_id: 'cid' })
  })

  it('every one of the six fields round-trips', () => {
    const base = baseline()
    let wc = clone(base)
    for (const field of fields) {
      wc = configReducer(wc, { kind: 'setWebhookMappingField', channel: 0, field, value: `v_${field}` })
    }
    // The loop edits ALL six fields, so `text` is overwritten from its
    // baseline 'body' to 'v_text' — every field round-trips its edited value.
    expect(wc.channels[0].webhook?.mapping).toEqual({
      text: 'v_text',
      sender_id: 'v_sender_id',
      sender_name: 'v_sender_name',
      media_url: 'v_media_url',
      media_type: 'v_media_type',
      conversation_id: 'v_conversation_id',
    })
  })
})

describe('pendingChangeCount — the header counter (N cambios sin aplicar)', () => {
  it('is 0 for an unedited working copy', () => {
    const base = baseline()
    expect(pendingChangeCount(base, clone(base))).toBe(0)
  })

  it('counts one changed entity as 1', () => {
    const base = baseline()
    const wc = configReducer(clone(base), { kind: 'setBrainField', brain: 0, field: 'name', value: 'support-v2' })
    expect(pendingChangeCount(base, wc)).toBe(1)
  })

  it('counts across sections (a brain edit + a new channel = 2)', () => {
    const base = baseline()
    let wc = configReducer(clone(base), { kind: 'setBrainField', brain: 0, field: 'sensitivity', value: 'private' })
    wc = configReducer(wc, { kind: 'addChannel' })
    expect(pendingChangeCount(base, wc)).toBe(2)
  })

  it('counts an added AND a removed entity', () => {
    const base = baseline()
    let wc = configReducer(clone(base), { kind: 'addBrain' }) // +1 brain
    wc = configReducer(wc, { kind: 'removeChannel', channel: 0 }) // -1 channel (and its route)
    // brains: +1, channels: -1, routes: -1 (cascaded) → 3 distinct changes.
    expect(pendingChangeCount(base, wc)).toBe(3)
  })
})
