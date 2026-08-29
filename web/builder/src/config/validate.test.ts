import { describe, it, expect } from 'vitest'
import { validateModel } from './validate'
import type { ModelConfig } from './schema'

// B14 RED (spec 2026-08-29-b14-model-panel-preapply-validation.md): the
// pre-Apply validator of the model panel. The 2026-08-23 corruption applied
// cleanly because every corrupt field was individually plausible to the
// core (a glued secret name IS a valid URL path; api_key_env is only
// required for groq; locality "local" asks nothing) — the panel is the
// first line that sees the whole block as the operator meant it.

const healthyCompat = (): ModelConfig => ({
  provider: 'openai-compatible',
  model_id: 'openrouter/auto',
  locality: 'cloud',
  base_url: 'https://openrouter.ai/api/v1',
  api_key_env: 'OPENROUTER_API_KEY',
})

const fieldsOf = (m: ModelConfig) => validateModel(m).map((e) => e.field)

describe('base_url must be an absolute http(s) URL (AS-1, AS-2)', () => {
  it('rejects garbage that does not parse as a URL', () => {
    const errs = validateModel({ ...healthyCompat(), base_url: 'not a url' })
    expect(errs.some((e) => e.field === 'base_url')).toBe(true)
  })

  it('rejects a scheme-less host (relative URL)', () => {
    const errs = validateModel({ ...healthyCompat(), base_url: 'openrouter.ai/api/v1' })
    expect(errs.some((e) => e.field === 'base_url')).toBe(true)
  })

  it('rejects a non-http(s) scheme', () => {
    const errs = validateModel({ ...healthyCompat(), base_url: 'ftp://openrouter.ai/v1' })
    expect(errs.some((e) => e.field === 'base_url')).toBe(true)
  })

  it('requires base_url for openai-compatible', () => {
    const m = { ...healthyCompat() }
    delete m.base_url
    expect(fieldsOf(m)).toContain('base_url')
  })
})

describe('the Sunday shape: a secret NAME glued into the URL (AS-3)', () => {
  it('rejects the exact corrupted base_url of 2026-08-23', () => {
    const errs = validateModel({
      ...healthyCompat(),
      base_url: 'https://openrouter.ai/api/v1OPENROUTER_API_KEY',
    })
    const err = errs.find((e) => e.field === 'base_url')
    expect(err).toBeDefined()
    expect(err!.message).toMatch(/secret|env var/i)
  })

  it('rejects a glued name in the middle of the path too', () => {
    const errs = validateModel({
      ...healthyCompat(),
      base_url: 'https://host.example/MY_API_KEY/v1',
    })
    expect(errs.some((e) => e.field === 'base_url')).toBe(true)
  })
})

describe('cloud without a key is blocking (AS-4)', () => {
  it('openai-compatible + cloud + empty api_key_env → api_key_env error', () => {
    const errs = validateModel({ ...healthyCompat(), api_key_env: '' })
    expect(errs.some((e) => e.field === 'api_key_env')).toBe(true)
  })

  it('openai-compatible + cloud + absent api_key_env → api_key_env error', () => {
    const m = { ...healthyCompat() }
    delete m.api_key_env
    expect(fieldsOf(m)).toContain('api_key_env')
  })

  it('groq + empty api_key_env → api_key_env error (mirrors the core)', () => {
    const errs = validateModel({
      provider: 'groq',
      model_id: 'llama-3.3-70b',
      locality: 'cloud',
    })
    expect(errs.some((e) => e.field === 'api_key_env')).toBe(true)
  })
})

describe('healthy shapes validate clean (AS-5, AS-6)', () => {
  it('the healthy openrouter block has zero errors', () => {
    expect(validateModel(healthyCompat())).toEqual([])
  })

  it('a local compat endpoint without a key is fine (the llama.cpp shape)', () => {
    expect(
      validateModel({
        provider: 'openai-compatible',
        model_id: 'qwen3-4b',
        locality: 'local',
        base_url: 'http://127.0.0.1:8189/v1',
      }),
    ).toEqual([])
  })

  it('a local ollama entry with no base_url is fine (base_url optional off-compat)', () => {
    expect(
      validateModel({ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }),
    ).toEqual([])
  })

  it('an ollama entry with a well-formed override base_url is fine', () => {
    expect(
      validateModel({
        provider: 'ollama',
        model_id: 'llama3.2:1b',
        locality: 'local',
        base_url: 'http://192.168.1.20:11434/v1',
      }),
    ).toEqual([])
  })
})
