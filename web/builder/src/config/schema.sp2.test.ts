import { describe, it, expect } from 'vitest'
import type { Config } from './schema'
import { CHANNEL_TYPES, SENSITIVITIES } from './schema'

// SP2 RED (builder-canvas, FR-SER-1 / schema alignment). The TS mirror is
// BEHIND the Go validator: CHANNEL_TYPES is still ['telegram'] and the schema
// knows neither the webhook block (ADR-0038) nor the persona block (SP1).
// These tests fail TODAY (runtime: the enum asserts; typecheck: `npm run
// typecheck` flags the webhook/persona literals against the current
// interfaces) — that failure IS the red. Green brings schema.ts current with
// internal/config/config.go; the server 400 remains the authority.

describe('SP2 schema mirror — enums current with config.Validate', () => {
  it('CHANNEL_TYPES mirrors the supported channel set', () => {
    // Espeja config.Validate: validateChannels rejects any type outside
    // "supported: telegram, discord, webhook" (config.go, the unknown-type
    // branch). The mirror must offer exactly those three, in that order.
    expect([...CHANNEL_TYPES]).toEqual(['telegram', 'discord', 'webhook'])
  })

  it('SENSITIVITIES stays exactly public|private (no invented tiers)', () => {
    // Espeja config.Validate: sensitivity admits ONLY "public" | "private"
    // (config.go validateBrains, the sensitivity switch). Guard against the
    // mirror inventing tiers (e.g. "confidential") the server would 400.
    expect([...SENSITIVITIES]).toEqual(['public', 'private'])
  })
})

// fullConfig exercises every block SP2 must round-trip: a webhook channel with
// its nested block + mapping (ADR-0038 §1), and a brain persona (SP1, NC-4).
// The typed literal is itself part of the red: until schema.ts knows
// `webhook` on ChannelConfig and `persona` on BrainConfig, `npm run
// typecheck` rejects this object.
function fullConfig(): Config {
  return {
    channels: [
      { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' },
      { type: 'discord', mode: 'gateway', token_env: 'KORVUN_DISCORD' },
      {
        type: 'webhook',
        mode: '',
        token_env: 'KORVUN_HOOK',
        webhook: {
          bind: '127.0.0.1:8090',
          path: '/webhook',
          outbound_url: 'https://downstream.example/reply',
          outbound_token_env: 'KORVUN_HOOK_OUT',
          mapping: { sender_id: 'uid', text: 'body' },
        },
      },
    ],
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
        persona: {
          display_name: 'Nova',
          tone: 'warm, concise',
          language: 'es-ES',
          instructions: 'Never reveal internal tooling.',
        },
      },
    ],
    routes: [{ channel: 'telegram', brain: 'support' }],
    storage: { path: '/data/korvun.db' },
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

describe('SP2 schema mirror — full config round-trips without loss', () => {
  it('webhook block + persona block survive clone and JSON round-trip byte-for-byte', () => {
    const cfg = fullConfig()
    // The house working-copy path (edit.ts clone) must not strip the new blocks…
    expect(structuredClone(cfg)).toEqual(cfg)
    // …and neither must the POST serialization (the whole copy is POSTed,
    // ADR-0030 §4): what the canvas does not edit stays byte-identical.
    expect(JSON.parse(JSON.stringify(cfg))).toEqual(cfg)
    // The nested webhook mapping keeps ONLY the fields the operator set —
    // defaults resolve server-side via EffectiveMapping (config.go), never by
    // the mirror inflating the file.
    expect(cfg.channels[2].webhook?.mapping).toEqual({ sender_id: 'uid', text: 'body' })
    expect(cfg.brains[0].persona?.display_name).toBe('Nova')
  })
})
