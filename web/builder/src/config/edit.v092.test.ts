import { describe, it, expect } from 'vitest'
import type { Config } from './schema'
import { configReducer } from './edit'

// v0.9.2 RED (B6, app bug-bash 2026-08-23): the model panel does not expose
// `warmup`, but a node inherited from a local template can carry warmup:true
// underneath — and the validator rejects warmup on cloud entries ("only
// valid for local models"), so a provider switch to the gateway walked the
// user into a rejected reload over a field they could neither see nor
// touch. The reducer now follows the channel panel's mode-follows-type
// pattern: changing provider to openai-compatible/groq, or locality to
// cloud, CLEARS warmup.

function baseline(): Config {
  return {
    channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
    brains: [
      {
        name: 'default',
        sensitivity: 'private',
        policy: { kind: 'priority', order: ['ollama'] },
        dispatch: 'fanout',
        models: [
          {
            provider: 'ollama',
            model_id: 'llama3.2',
            locality: 'local',
            base_url: 'http://localhost:11434',
            warmup: true,
          },
        ],
      },
    ],
    routes: [{ channel: 'telegram', brain: 'default' }],
    admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
  }
}

const model = (c: Config) => c.brains[0].models[0]

describe('B6 — provider/locality changes clear the orphan warmup', () => {
  it("today's exact case: local ollama node switched to openai-compatible loses warmup", () => {
    const after = configReducer(baseline(), {
      kind: 'updateModel',
      brain: 0,
      model: 0,
      patch: { provider: 'openai-compatible' },
    })
    expect('warmup' in model(after)).toBe(false)
  })

  it('locality switched to cloud loses warmup (the validator mirror)', () => {
    const after = configReducer(baseline(), {
      kind: 'updateModel',
      brain: 0,
      model: 0,
      patch: { locality: 'cloud' },
    })
    expect('warmup' in model(after)).toBe(false)
  })

  it('provider switched to groq (always cloud) loses warmup', () => {
    const after = configReducer(baseline(), {
      kind: 'updateModel',
      brain: 0,
      model: 0,
      patch: { provider: 'groq' },
    })
    expect('warmup' in model(after)).toBe(false)
  })

  it('unrelated edits keep the warmup of a local node', () => {
    const after = configReducer(baseline(), {
      kind: 'updateModel',
      brain: 0,
      model: 0,
      patch: { model_id: 'llama3.2:1b' },
    })
    expect(model(after).warmup).toBe(true)
  })

  it('staying on ollama/local keeps warmup', () => {
    const after = configReducer(baseline(), {
      kind: 'updateModel',
      brain: 0,
      model: 0,
      patch: { provider: 'ollama', locality: 'local' },
    })
    expect(model(after).warmup).toBe(true)
  })
})
