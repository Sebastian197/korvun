// The ADR-0024 metadata frame, exactly as the SSE wire carries it — no field
// can hold message content, by the core's construction.
export interface FeedFrame {
  type: string
  channel?: string
  brain?: string
  timestamp: string
  envelope_id?: string
  direction?: string
  /** Client-side monotonic ingest sequence — the stable React key (never on
   * the wire; the store assigns it). */
  seq?: number
}

/** Parse one SSE data payload; null when it is not a well-formed frame. */
export function parseFrame(data: string): FeedFrame | null {
  try {
    const raw: unknown = JSON.parse(data)
    if (typeof raw !== 'object' || raw === null) return null
    const f = raw as Record<string, unknown>
    if (typeof f.type !== 'string' || typeof f.timestamp !== 'string') return null
    const out: FeedFrame = { type: f.type, timestamp: f.timestamp }
    if (typeof f.channel === 'string') out.channel = f.channel
    if (typeof f.brain === 'string') out.brain = f.brain
    if (typeof f.envelope_id === 'string') out.envelope_id = f.envelope_id
    if (typeof f.direction === 'string') out.direction = f.direction
    return out
  } catch {
    return null
  }
}
