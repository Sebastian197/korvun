// The live-feed store: ingests ADR-0024 metadata frames and derives the
// window-scoped counters Home paints (FR-WIN-4: "desde que se abrió la
// ventana", never presented as an all-time total) plus the Activity rows.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { pollOnce } from '../status/store'
import { getFeed, ingestFrame, minuteSeries, resetFeedForTests, startFeed } from './store'

class FakeES {
  static instances: FakeES[] = []
  url: string
  onopen: (() => void) | null = null
  onmessage: ((ev: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(url: string) {
    this.url = url
    FakeES.instances.push(this)
  }
  close(): void {
    this.closed = true
  }
}

const esFactory = (url: string): EventSource => new FakeES(url) as unknown as EventSource

function okFetch(): typeof fetch {
  return (() => Promise.resolve(new Response('ok'))) as typeof fetch
}

function stoppedFetch(): typeof fetch {
  return (() =>
    Promise.resolve(
      new Response(JSON.stringify({ error: 'core stopped' }), { status: 503 }),
    )) as typeof fetch
}

function frame(type: string, extra: Record<string, string> = {}): string {
  return JSON.stringify({ type, timestamp: '2026-07-25T10:00:00Z', ...extra })
}

beforeEach(() => {
  resetFeedForTests()
  FakeES.instances = []
})

afterEach(() => {
  vi.useRealTimers()
})

describe('feed store', () => {
  it('counts each frame type into its window-scoped counter', () => {
    ingestFrame(frame('message_received', { channel: 'telegram' }))
    ingestFrame(frame('reply_sent', { channel: 'telegram' }))
    ingestFrame(frame('reply_sent', { channel: 'telegram' }))
    ingestFrame(frame('message_dropped', { channel: 'telegram' }))
    ingestFrame(frame('handle_failed', { channel: 'telegram' }))
    expect(getFeed().counters).toEqual({
      received: 1,
      replied: 2,
      dropped: 1,
      failed: 1,
    })
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

  it('the SSE lifecycle follows the core: open on running, live on open, closed on stop', async () => {
    await pollOnce(okFetch())
    startFeed(esFactory)
    expect(FakeES.instances.length).toBe(1)
    expect(FakeES.instances[0]?.url).toBe('/api/events')
    FakeES.instances[0]?.onopen?.()
    expect(getFeed().live).toBe(true)
    FakeES.instances[0]?.onmessage?.({ data: frame('reply_sent') })
    expect(getFeed().counters.replied).toBe(1)
    await pollOnce(stoppedFetch())
    expect(FakeES.instances[0]?.closed).toBe(true)
    expect(getFeed().live).toBe(false)
    // The window-scoped data SURVIVES the stop — only the stream closes.
    expect(getFeed().counters.replied).toBe(1)
  })

  it('a stream error retries after RETRY_MS while the core still runs', async () => {
    await pollOnce(okFetch())
    vi.useFakeTimers()
    startFeed(esFactory)
    FakeES.instances[0]?.onopen?.()
    FakeES.instances[0]?.onerror?.()
    expect(getFeed().live).toBe(false)
    expect(FakeES.instances.length).toBe(1)
    vi.advanceTimersByTime(3000)
    expect(FakeES.instances.length).toBe(2) // reconnected
  })

  it('a stream error does NOT reconnect once the core stopped', async () => {
    await pollOnce(okFetch())
    vi.useFakeTimers()
    startFeed(esFactory)
    FakeES.instances[0]?.onerror?.()
    vi.useRealTimers()
    await pollOnce(stoppedFetch())
    vi.useFakeTimers()
    vi.advanceTimersByTime(10_000)
    expect(FakeES.instances.length).toBe(1) // no second instance
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
