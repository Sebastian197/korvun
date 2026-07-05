import { describe, it, expect } from 'vitest'
import { parseSaveError, extractField } from './errors'

describe('extractField — maps a validate message to a form field path', () => {
  it('pulls the config path out of a config.Validate message', () => {
    expect(extractField('config: invalid config: brains[0].sensitivity: unknown sensitivity "x"')).toBe(
      'brains[0].sensitivity',
    )
    expect(extractField('...: channels[0].token_env: required')).toBe('channels[0].token_env')
    expect(extractField('...: admin.token_env: required')).toBe('admin.token_env')
    expect(extractField('...: brains[1].models[2].provider: unknown')).toBe('brains[1].models[2].provider')
    expect(extractField('...: routes[0].brain: no brain named "x"')).toBe('routes[0].brain')
  })
  it('returns null when there is no field path', () => {
    expect(extractField('something generic went wrong')).toBeNull()
  })
})

describe('parseSaveError — each failure gets its distinct treatment', () => {
  it('400 → validation with the field extracted', () => {
    const e = parseSaveError(400, JSON.stringify({ error: 'brains[0].sensitivity: unknown' }))
    expect(e).toEqual({ kind: 'validation', field: 'brains[0].sensitivity', message: 'brains[0].sensitivity: unknown' })
  })

  it('the TWO 409 codes are distinguished (not a generic 409)', () => {
    const lock = parseSaveError(409, JSON.stringify({ error_code: 'config_would_self_lock', message: 'locked out' }))
    const busy = parseSaveError(409, JSON.stringify({ error_code: 'reload_in_progress', message: 'busy' }))
    expect(lock.kind).toBe('selfLock')
    expect(busy.kind).toBe('reloadInProgress')
    expect(lock.kind).not.toBe(busy.kind) // never collapsed into one generic 409
  })

  it('401 → unauthorized (drives token clear + re-auth)', () => {
    expect(parseSaveError(401, JSON.stringify({ error: 'unauthorized' }))).toEqual({ kind: 'unauthorized' })
  })

  it('anything else → other, carrying the status and message', () => {
    const e = parseSaveError(500, JSON.stringify({ error: 'reload could not be started' }))
    expect(e).toEqual({ kind: 'other', status: 500, message: 'reload could not be started' })
  })
})
