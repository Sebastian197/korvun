// Unread accounting (operator-console spec FR-UNREAD): last-read turn
// counts live in the SHELL's persistent state (localStorage — survives
// restarts, never touches the core), keyed by conversation. A conversation
// is unread when the store's total turn count outruns what the operator
// has seen; deletion shrinking history clamps to zero instead of going
// negative.
import type { ConversationRow } from './api'

const STORE_KEY = 'korvun.console.lastRead'

export function readLastSeen(storage: Storage = localStorage): Record<string, number> {
  try {
    const raw = storage.getItem(STORE_KEY)
    if (raw === null) return {}
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return {}
    const out: Record<string, number> = {}
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof v === 'number' && Number.isFinite(v)) out[k] = v
    }
    return out
  } catch {
    return {}
  }
}

export function markRead(key: string, turnCount: number, storage: Storage = localStorage): void {
  const seen = readLastSeen(storage)
  seen[key] = turnCount
  try {
    storage.setItem(STORE_KEY, JSON.stringify(seen))
  } catch {
    // Quota/private-mode failures degrade to "everything unread" — never a crash.
  }
}

export function unreadCount(row: ConversationRow, lastSeen: Record<string, number>): number {
  return Math.max(0, row.turn_count - (lastSeen[row.key] ?? 0))
}

export function unreadTotal(rows: ConversationRow[], lastSeen: Record<string, number>): number {
  return rows.reduce((sum, r) => sum + unreadCount(r, lastSeen), 0)
}

export { STORE_KEY }
