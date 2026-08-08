// The operator-console API client (operator-console spec SP4) over the SP4
// proxy: every path is relative and UNAUTHENTICATED HERE on purpose — the
// shell's proxy injects the bearer server-side, so no token ever reaches
// this renderer (the mutate.ts posture). Errors never escape as throws: the
// UI needs states, not stack traces.

export interface ConversationRow {
  key: string
  active_session: number
  session_count: number
  /** Total turns across sessions — the unread arithmetic's anchor. */
  turn_count: number
  last_activity?: string
  last_role?: string
  taken_over: boolean
}

export interface SearchHit {
  key: string
  session: number
  seq: number
  role: string
  content: string
  timestamp?: string
}

export interface SessionRow {
  id: number
  turn_count: number
  first?: string
  last?: string
}

export interface TurnRow {
  role: string
  content: string
  timestamp?: string
  seq: number
}

export type ReplyResult =
  | { ok: true }
  | { ok: false; reason: 'channel-missing' | 'saturated' | 'invalid' | 'failed' }

function convPath(key: string): string {
  return `/api/conversations/${encodeURIComponent(key)}`
}

async function getJSON<T>(url: string, fetcher: typeof fetch, fallback: T): Promise<T> {
  try {
    const res = await fetcher(url)
    if (!res.ok) return fallback
    return (await res.json()) as T
  } catch {
    return fallback
  }
}

export async function listConversations(fetcher: typeof fetch = fetch): Promise<ConversationRow[]> {
  return getJSON('/api/conversations', fetcher, [])
}

export async function conversationDetail(
  key: string,
  fetcher: typeof fetch = fetch,
): Promise<TurnRow[]> {
  return getJSON(convPath(key), fetcher, [])
}

export async function listSessions(
  key: string,
  fetcher: typeof fetch = fetch,
): Promise<SessionRow[]> {
  return getJSON(`${convPath(key)}/sessions`, fetcher, [])
}

export async function sessionDetail(
  key: string,
  id: number,
  fetcher: typeof fetch = fetch,
): Promise<TurnRow[]> {
  return getJSON(`${convPath(key)}/sessions/${id}`, fetcher, [])
}

export async function sendReply(
  key: string,
  text: string,
  fetcher: typeof fetch = fetch,
): Promise<ReplyResult> {
  try {
    const res = await fetcher(`${convPath(key)}/reply`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text }),
    })
    if (res.status === 202) return { ok: true }
    if (res.status === 409) return { ok: false, reason: 'channel-missing' }
    if (res.status === 503) return { ok: false, reason: 'saturated' }
    if (res.status === 400) return { ok: false, reason: 'invalid' }
    return { ok: false, reason: 'failed' }
  } catch {
    return { ok: false, reason: 'failed' }
  }
}

export async function setTakeover(
  key: string,
  on: boolean,
  fetcher: typeof fetch = fetch,
): Promise<boolean> {
  try {
    const res = await fetcher(`${convPath(key)}/${on ? 'takeover' : 'release'}`, {
      method: 'POST',
    })
    return res.status === 204
  } catch {
    return false
  }
}

export async function newSession(
  key: string,
  fetcher: typeof fetch = fetch,
): Promise<number | null> {
  try {
    const res = await fetcher(`${convPath(key)}/sessions`, { method: 'POST' })
    if (!res.ok) return null
    const out = (await res.json()) as { session?: number }
    return typeof out.session === 'number' ? out.session : null
  } catch {
    return null
  }
}

export type DeleteSessionResult = 'ok' | 'active' | 'failed'

export async function deleteConversation(
  key: string,
  fetcher: typeof fetch = fetch,
): Promise<boolean> {
  try {
    const res = await fetcher(convPath(key), { method: 'DELETE' })
    return res.status === 204
  } catch {
    return false
  }
}

export async function deleteSession(
  key: string,
  id: number,
  fetcher: typeof fetch = fetch,
): Promise<DeleteSessionResult> {
  try {
    const res = await fetcher(`${convPath(key)}/sessions/${id}`, { method: 'DELETE' })
    if (res.status === 204) return 'ok'
    if (res.status === 409) return 'active'
    return 'failed'
  } catch {
    return 'failed'
  }
}

export async function searchTurns(
  query: string,
  fetcher: typeof fetch = fetch,
): Promise<SearchHit[]> {
  return getJSON(`/api/search?q=${encodeURIComponent(query)}`, fetcher, [])
}

export type UserMessageResult =
  | { ok: true }
  | { ok: false; reason: 'not-wired' | 'saturated' | 'invalid' | 'failed' }

/** The direct-chat send (console channel): the human speaks as USER and the
 * full pipeline answers. Console keys only — the server enforces it too. */
export async function sendUserMessage(
  key: string,
  text: string,
  fetcher: typeof fetch = fetch,
): Promise<UserMessageResult> {
  try {
    const res = await fetcher(`${convPath(key)}/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text }),
    })
    if (res.status === 202) return { ok: true }
    if (res.status === 409) return { ok: false, reason: 'not-wired' }
    if (res.status === 503) return { ok: false, reason: 'saturated' }
    if (res.status === 400) return { ok: false, reason: 'invalid' }
    return { ok: false, reason: 'failed' }
  } catch {
    return { ok: false, reason: 'failed' }
  }
}
