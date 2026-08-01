import { describe, it, expect } from 'vitest'
import { CHANNEL_MODES_BY_TYPE } from './schema'

// SP3 RED (builder-canvas FR-SCOPE-1): the CHANNEL_MODES gap, fixed HERE as a
// per-type mirror. The flat CHANNEL_MODES = ['polling'] predates discord and
// webhook; the properties panel needs the REAL per-type rule. Espeja
// config.Validate — validateChannels' type switch (config.go:444-452):
//   telegram → validateChannelMode(…, "polling")   (the ONLY telegram mode)
//   discord  → validateChannelMode(…, "gateway")
//   webhook  → NO mode: a non-empty mode is a named field-path error
//              ("webhook takes no mode", ADR-0038 §1 NC-1c)
// NOTE for the copilot: the brief said "telegram polling|webhook" — the Go
// validator says otherwise (polling only). The mirror follows the CODE; if
// telegram is meant to gain a webhook mode, that is a Go change first.
//
// RED today: CHANNEL_MODES_BY_TYPE does not exist in schema.ts.

describe('SP3 schema mirror — channel modes per type', () => {
  it('mirrors the exact mode set the validator accepts per channel type', () => {
    expect(CHANNEL_MODES_BY_TYPE).toEqual({
      telegram: ['polling'],
      discord: ['gateway'],
      webhook: [],
    })
  })
})
