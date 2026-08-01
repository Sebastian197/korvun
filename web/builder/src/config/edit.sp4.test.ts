import { describe, it, expect } from 'vitest'
import type { Config } from './schema'
import { clone, configReducer } from './edit'

// SP4 RED (builder-canvas, closes SP3's declared gap): the webhook block is
// EDITABLE from the properties panel via a new reducer action, with the house
// structuredClone round-trip (untouched fields byte-identical, baseline never
// mutated). Espeja config.Validate: the webhook nested block belongs to a
// webhook channel (ADR-0038 §1); its four editable fields are
// bind/path/outbound_url/outbound_token_env (mapping stays out of v1 panels).
//
// RED note (house precedent): the 'setWebhookField' action kind does not
// exist in ConfigAction yet — typecheck rejects it and the reducer has no
// branch, so every assert fails. That failure IS the red.

function baseline(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      {
        type: 'webhook',
        mode: '',
        token_env: 'KORVUN_HOOK',
        webhook: { bind: '127.0.0.1:8090', outbound_url: 'https://downstream.example/reply' },
      },
      { type: 'webhook', mode: '', token_env: 'KORVUN_HOOK_2' }, // block ABSENT (mid-edit shape)
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
    routes: [],
    storage: { path: '/data/korvun.db' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('setWebhookField — the panel edits the webhook block', () => {
  it('patches one field of an existing block, preserving its siblings', () => {
    const base = baseline()
    const wc = configReducer(clone(base), {
      kind: 'setWebhookField',
      channel: 1,
      field: 'outbound_token_env',
      value: 'KORVUN_HOOK_OUT',
    })
    expect(wc.channels[1].webhook).toEqual({
      bind: '127.0.0.1:8090',
      outbound_url: 'https://downstream.example/reply',
      outbound_token_env: 'KORVUN_HOOK_OUT',
    })
    // Round-trip: everything the action does not edit stays byte-identical.
    expect(wc.channels[0]).toEqual(base.channels[0])
    expect(wc.channels[2]).toEqual(base.channels[2])
    expect(wc.brains).toEqual(base.brains)
    expect(wc.storage).toEqual(base.storage)
    expect(wc.admin).toEqual(base.admin)
    // …and the baseline itself was never mutated (structuredClone pattern).
    expect(base.channels[1].webhook?.outbound_token_env).toBeUndefined()
  })

  it('materializes the block on a webhook channel that has none (mid-edit)', () => {
    // A channel switched to type webhook starts blockless; the first panel
    // edit materializes the block with only the edited field (the persona
    // pattern) — the server 400 remains the authority on completeness
    // (ADR-0038: outbound_url required at Validate).
    const base = baseline()
    const wc = configReducer(clone(base), {
      kind: 'setWebhookField',
      channel: 2,
      field: 'outbound_url',
      value: 'https://other.example/hook',
    })
    expect(wc.channels[2].webhook).toEqual({ outbound_url: 'https://other.example/hook' })
    expect(base.channels[2].webhook).toBeUndefined() // baseline intact
  })

  it('edits every panel field: bind, path, outbound_url, outbound_token_env', () => {
    const base = baseline()
    let wc = clone(base)
    const fields = [
      ['bind', '0.0.0.0:9000'],
      ['path', '/hooks/in'],
      ['outbound_url', 'https://x.example/r'],
      ['outbound_token_env', 'HOOK_OUT'],
    ] as const
    for (const [field, value] of fields) {
      wc = configReducer(wc, { kind: 'setWebhookField', channel: 1, field, value })
    }
    expect(wc.channels[1].webhook).toEqual({
      bind: '0.0.0.0:9000',
      path: '/hooks/in',
      outbound_url: 'https://x.example/r',
      outbound_token_env: 'HOOK_OUT',
    })
  })
})
