import { describe, expect, it } from 'vitest'
import { brainOfConversationId, displayIdOf, newDirectChatKey } from './brainid'

// B9 RED — the id contract (spec FR-B9-3/4). The format must round-trip
// any legal brain name (config only requires non-empty + unique) and leave
// ordinary ids untouched.

describe('newDirectChatKey', () => {
  it('brain-addressed keys carry the encoded prefix', () => {
    const key = newDirectChatKey('openrouter')
    expect(key).toMatch(/^console::b:openrouter:chat-[0-9a-f-]{8}$/)
  })

  it('a null brain keeps today’s shape byte-for-byte', () => {
    expect(newDirectChatKey(null)).toMatch(/^console::chat-[0-9a-f-]{8}$/)
  })

  it('a brain name with a colon percent-encodes unambiguously', () => {
    const key = newDirectChatKey('mi:brain')
    expect(key).toMatch(/^console::b:mi%3Abrain:chat-/)
  })
})

describe('brainOfConversationId', () => {
  it('decodes the prefixed brain', () => {
    expect(brainOfConversationId('b:openrouter:chat-ab12cd34')).toBe('openrouter')
    expect(brainOfConversationId('b:mi%3Abrain:chat-1')).toBe('mi:brain')
  })

  it('ordinary ids resolve to null', () => {
    expect(brainOfConversationId('chat-ab12cd34')).toBeNull()
    expect(brainOfConversationId('1000234')).toBeNull()
    expect(brainOfConversationId('b:solo-un-segmento')).toBeNull()
    expect(brainOfConversationId('b::chat-1')).toBeNull()
  })
})

describe('displayIdOf', () => {
  it('strips the brain prefix for display', () => {
    expect(displayIdOf('b:openrouter:chat-ab12cd34')).toBe('chat-ab12cd34')
  })

  it('leaves ordinary ids untouched', () => {
    expect(displayIdOf('chat-ab12cd34')).toBe('chat-ab12cd34')
  })
})
