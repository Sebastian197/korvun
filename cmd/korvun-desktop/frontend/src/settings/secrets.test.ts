import { describe, expect, it } from 'vitest'
import { secretNamesOfConfig } from './secrets'

// B10 RED — the pure discovery mirror (FR-B10-3).

const FULL = {
  channels: [
    { type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG_TOKEN' },
    {
      type: 'webhook',
      token_env: 'HOOK_IN_TOKEN',
      webhook: { outbound_url: 'http://127.0.0.1:9/x', outbound_token_env: 'HOOK_OUT_TOKEN' },
    },
  ],
  brains: [
    {
      name: 'a',
      models: [
        { provider: 'groq', model_id: 'm1', locality: 'cloud', api_key_env: 'GROQ_API_KEY' },
        { provider: 'ollama', model_id: 'm2', locality: 'local' },
        {
          provider: 'openai-compatible',
          model_id: 'm3',
          locality: 'cloud',
          api_key_env: 'GROQ_API_KEY',
        },
      ],
    },
  ],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}

describe('secretNamesOfConfig', () => {
  it('discovers all four kinds, appearance-ordered, deduplicated', () => {
    expect(secretNamesOfConfig(FULL)).toEqual([
      'KORVUN_TG_TOKEN',
      'HOOK_IN_TOKEN',
      'HOOK_OUT_TOKEN',
      'GROQ_API_KEY',
      'KORVUN_ADMIN_TOKEN',
    ])
  })

  it('unexpected shapes degrade to fewer rows, never a crash', () => {
    expect(secretNamesOfConfig(null)).toEqual([])
    expect(secretNamesOfConfig({ channels: 'nope', brains: [{ models: null }] })).toEqual([])
    expect(secretNamesOfConfig({ admin: { token_env: 'KORVUN_ADMIN_TOKEN' } })).toEqual([
      'KORVUN_ADMIN_TOKEN',
    ])
  })
})
