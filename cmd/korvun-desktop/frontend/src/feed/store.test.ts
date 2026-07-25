// The live-feed store: ingests ADR-0024 metadata frames and derives the
// window-scoped counters Home paints (FR-WIN-4: "desde que se abrió la
// ventana", never presented as an all-time total) plus the Activity rows.
import { beforeEach, describe, expect, it } from 'vitest'
import { getFeed, ingestFrame, minuteSeries, resetFeedForTests } from './store'

function frame(type: string, extra: Record<string, string> = {}): string {
  return JSON.stringify({ type, timestamp: '2026-07-25T10:00:00Z', ...extra })
}

beforeEach(() => {
  resetFeedForTests()
})

describe('feed store', () => {
  it('counts each frame type into its window-scoped counter', () => {
    ingestFrame(frame('message_received', { channel: 'telegram' }))
    ingestFrame(frame('reply_sent', { channel: 'telegram' }))
    ingestFrame(frame('reply_sent', { channel: 'telegram' }))
    ingestFrame(frame('message_dropped', { channel: 'telegram' }))
    ingestFrame(frame('handle_failed', { channel: 'telegram' }))
    expect(getFeed().counters).toEqual({ received: 1, replied: 2, dropped: 1, failed: 1 })
  })

  it('keeps frames newest-first and caps the buffer', () => {
    for (let i = 0; i < 300; i++) {
      ingestFrame(frame('reply_sent', { envelope_id: `e${i}` }))
    }
    const { frames } = getFeed()
    expect(frames.length).toBe(250)
    expect(frames[0]?.envelope_id).toBe('e299')
  })

  it('collects the channel names it has seen (the Activity filter source)', () => {
    ingestFrame(frame('message_received', { channel: 'telegram' }))
    ingestFrame(frame('message_received', { channel: 'discord' }))
    ingestFrame(frame('message_received', { channel: 'telegram' }))
    expect(getFeed().channels).toEqual(['telegram', 'discord'])
  })

  it('ignores a malformed frame without corrupting state', () => {
    ingestFrame('{not json')
    ingestFrame(JSON.stringify({ nada: true }))
    expect(getFeed().counters.received).toBe(0)
    expect(getFeed().frames).toEqual([])
  })

  it('minuteSeries buckets replies per minute for the sparkline', () => {
    const now = Date.parse('2026-07-25T10:14:30Z')
    const at = (min: number): string => new Date(now - min * 60_000).toISOString()
    ingestFrame(JSON.stringify({ type: 'reply_sent', timestamp: at(0) }))
    ingestFrame(JSON.stringify({ type: 'reply_sent', timestamp: at(0) }))
    ingestFrame(JSON.stringify({ type: 'reply_sent', timestamp: at(2) }))
    ingestFrame(JSON.stringify({ type: 'reply_sent', timestamp: at(20) })) // out of window
    const series = minuteSeries(now, 14)
    expect(series.length).toBe(14)
    expect(series[13]).toBe(2) // current minute
    expect(series[11]).toBe(1) // two minutes ago
    expect(series.reduce((a, b) => a + b, 0)).toBe(3)
  })
})