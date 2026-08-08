// Chat / operator console (operator-console spec SP4 + the 2026-08-08
// completion rider): the inbox with filter, content search, and unread
// badges; the conversation pane where the OPERATOR role is never dressed
// as the AI; read-only archived sessions; deletion behind explicit
// confirmation; and a composer with the Enter/Shift+Enter contract. Real
// time = the SSE feed as a CHANGE SIGNAL ONLY plus a slow catch-up poll —
// content never rides the SSE (ADR-0024 §1); every content read goes
// through the shell proxy's bearer.
//
// UI copy is ENGLISH on purpose (the product's voice — Chano's SP4 rider),
// inside a shell whose chrome is Spanish today; flagged at the SP4 close.
import { useCallback, useEffect, useRef, useState } from 'react'
import { channelLabel } from '../lib/channels'
import { useCoreState, type CoreState } from '../status/store'
import { useFeed } from '../feed/store'
import {
  conversationDetail,
  deleteConversation,
  deleteSession,
  listConversations,
  listSessions,
  newSession,
  searchTurns,
  sendReply,
  sendUserMessage,
  sessionDetail,
  setTakeover,
  type ConversationRow,
  type SearchHit,
  type SessionRow,
  type TurnRow,
} from '../console/api'
import { markRead, readLastSeen, unreadCount } from '../console/unread'
import { relTime } from '../console/time'

export interface ConsoleProps {
  fetcher?: typeof fetch
  /** Feed change signal (defaults to the live feed counters); a change
   * triggers a bearer re-fetch — content NEVER rides the SSE. */
  feedVersion?: number
  /** Core state override for tests; defaults to the live status store. */
  coreState?: CoreState
  /** Catch-up poll (bearer REST, like Channels): the SSE stream can open
   * AFTER events already happened — the poll closes that window. */
  pollIntervalMs?: number
}

const DEFAULT_POLL_MS = 15_000

const CONFIRM_CONVERSATION = 'This deletes the conversation from disk. No undo.'

/** Console-channel keys host the DIRECT chat: the human is the user. */
function isConsoleKey(key: string | null): boolean {
  return key !== null && key.startsWith('console::')
}
const CONFIRM_SESSION = 'This deletes the archived session from disk. No undo.'

type ComposerState = { kind: 'idle' } | { kind: 'inflight' } | { kind: 'error'; message: string }

type ConfirmState = 'none' | 'conversation' | 'session'

function splitKey(key: string): { channel: string; id: string } {
  const i = key.indexOf('::')
  if (i < 0) return { channel: key, id: '' }
  return { channel: key.slice(0, i), id: key.slice(i + 2) }
}

function roleLabel(role: string): string {
  switch (role) {
    case 'operator':
      return 'Operator (you)'
    case 'assistant':
      return 'Korvun'
    case 'system':
      return 'system'
    default:
      return 'User'
  }
}

/** True when el is scrolled close enough to its bottom that new content
 * should keep it pinned there (FR-POLISH: autoscroll respects a human who
 * scrolled up to read). Exported for its unit test. */
export function isNearBottom(el: {
  scrollHeight: number
  scrollTop: number
  clientHeight: number
}): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < 48
}

/** The live feed reduced to a change counter — the console's re-fetch tick.
 * The `live` flip counts as a change on purpose: the stream opening late
 * (core just started) must trigger a catch-up re-fetch for the events it
 * never saw. */
function useLiveFeedVersion(): number {
  const feed = useFeed()
  const c = feed.counters
  return c.received + c.replied + c.dropped + c.failed + (feed.live ? 1 : 0)
}

export function Console(props: ConsoleProps): React.JSX.Element {
  const fetcher = props.fetcher ?? fetch
  const liveVersion = useLiveFeedVersion()
  const feedVersion = props.feedVersion ?? liveVersion
  const liveCore = useCoreState()
  const coreState = props.coreState ?? liveCore

  const [convs, setConvs] = useState<ConversationRow[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  /** null = the ACTIVE session; a number = that session explicitly. */
  const [viewSession, setViewSession] = useState<number | null>(null)
  const [turns, setTurns] = useState<TurnRow[]>([])
  const [composer, setComposer] = useState<ComposerState>({ kind: 'idle' })
  const [draft, setDraft] = useState('')
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<SearchHit[] | null>(null)
  const [confirm, setConfirm] = useState<ConfirmState>('none')
  const [lastSeen, setLastSeen] = useState<Record<string, number>>(() => readLastSeen())
  const [thinking, setThinking] = useState(false)
  const [nowMs, setNowMs] = useState(() => Date.now())

  const turnsRef = useRef<HTMLOListElement | null>(null)
  const pinnedRef = useRef(true)

  const refetchInbox = useCallback(() => {
    let ignore = false
    void listConversations(fetcher).then((rows) => {
      if (!ignore) setConvs(rows)
    })
    return () => {
      ignore = true
    }
  }, [fetcher])

  const refetchConversation = useCallback(() => {
    if (selected === null) return () => undefined
    let ignore = false
    void listSessions(selected, fetcher).then((s) => {
      if (!ignore) setSessions(s)
    })
    const load =
      viewSession === null
        ? conversationDetail(selected, fetcher)
        : sessionDetail(selected, viewSession, fetcher)
    void load.then((t) => {
      if (!ignore) setTurns(t)
    })
    return () => {
      ignore = true
    }
  }, [fetcher, selected, viewSession])

  // The catch-up tick: a slow interval (bearer REST) so nothing is missed
  // even if an SSE frame was dropped or predates the stream. It also
  // refreshes the relative timestamps.
  const [pollTick, setPollTick] = useState(0)
  const pollMs = props.pollIntervalMs ?? DEFAULT_POLL_MS
  useEffect(() => {
    const id = setInterval(() => {
      setPollTick((t) => t + 1)
      setNowMs(Date.now())
    }, pollMs)
    return () => clearInterval(id)
  }, [pollMs])

  // Initial fetch + the SSE change signal + the catch-up tick.
  useEffect(() => refetchInbox(), [refetchInbox, feedVersion, pollTick])
  useEffect(() => refetchConversation(), [refetchConversation, feedVersion, pollTick])

  const listed = convs.find((c) => c.key === selected) ?? null
  // A New-chat draft exists before its first message: synthesize the row so
  // the pane opens (it materializes in the store on first send).
  const current =
    listed ??
    (selected !== null && isConsoleKey(selected)
      ? {
          key: selected,
          active_session: 1,
          session_count: 1,
          turn_count: 0,
          taken_over: false,
        }
      : null)
  const activeSessionID = sessions.length > 0 ? sessions[sessions.length - 1].id : null
  const archived =
    viewSession !== null && activeSessionID !== null && viewSession !== activeSessionID
  const coreDown = coreState !== 'running'

  // Opening (or receiving new turns in) a conversation marks it read — the
  // unread arithmetic anchors on the row's total turn count (FR-UNREAD).
  // Primitive deps + a same-value bail: current is a fresh object every
  // render, and an unconditional setLastSeen would loop the render.
  const currentKey = current?.key ?? null
  const currentTurns = current?.turn_count ?? 0
  useEffect(() => {
    if (currentKey === null) return
    markRead(currentKey, currentTurns)
    setLastSeen((prev) =>
      prev[currentKey] === currentTurns ? prev : { ...prev, [currentKey]: currentTurns },
    )
  }, [currentKey, currentTurns])

  // The brain answered (or the ack landed): thinking ends.
  useEffect(() => {
    if (turns.length === 0) return
    const last = turns[turns.length - 1]
    if (last.role === 'assistant' || last.role === 'system') setThinking(false)
  }, [turns])

  // Autoscroll: pin to the newest turn unless the human scrolled up.
  useEffect(() => {
    const el = turnsRef.current
    if (el !== null && pinnedRef.current) {
      el.scrollTop = el.scrollHeight
      const lastTurn = el.lastElementChild
      // jsdom has no scrollIntoView; browsers do — guard so the tests and
      // the app share one code path.
      if (lastTurn !== null && typeof lastTurn.scrollIntoView === 'function') {
        lastTurn.scrollIntoView({ block: 'end' })
      }
    }
  }, [turns])

  const filter = query.trim().toLowerCase()
  const visibleConvs =
    filter === ''
      ? convs
      : convs.filter((c) => {
          const { channel, id } = splitKey(c.key)
          return (
            channel.toLowerCase().includes(filter) ||
            channelLabel(channel).toLowerCase().includes(filter) ||
            id.toLowerCase().includes(filter)
          )
        })

  function openConversation(key: string, session: number | null): void {
    setSelected(key)
    setViewSession(session)
    setComposer({ kind: 'idle' })
    setConfirm('none')
    pinnedRef.current = true
  }

  async function runSearch(): Promise<void> {
    const q = query.trim()
    if (q === '') {
      setHits(null)
      return
    }
    setHits(await searchTurns(q, fetcher))
  }

  function openHit(hit: SearchHit): void {
    openConversation(hit.key, hit.session)
    setHits(null)
    setQuery('')
  }

  async function submitReply(): Promise<void> {
    if (selected === null || draft.trim() === '') return
    if (isConsoleKey(selected)) {
      // Direct chat: speak as the USER through the full pipeline and show
      // the honest local-model latency while the brain works.
      setComposer({ kind: 'idle' })
      const sent = await sendUserMessage(selected, draft, fetcher)
      if (sent.ok) {
        setDraft('')
        setThinking(true)
        pinnedRef.current = true
        refetchConversation()
        refetchInbox()
        return
      }
      setComposer({
        kind: 'error',
        message:
          sent.reason === 'not-wired'
            ? 'Console channel not wired in the running core.'
            : sent.reason === 'saturated'
              ? 'Brain queue is saturated — try again in a moment.'
              : 'The message could not be sent.',
      })
      return
    }
    setComposer({ kind: 'inflight' })
    const out = await sendReply(selected, draft, fetcher)
    if (out.ok) {
      setDraft('')
      // 202 accepted: the funnel is async — "on its way" is the honest
      // state; a delivery failure surfaces through the feed's failure
      // counters (the same signal that re-fetches this view).
      setComposer({ kind: 'inflight' })
      pinnedRef.current = true
      refetchConversation()
      return
    }
    const message =
      out.reason === 'channel-missing'
        ? 'Channel not registered in the running core.'
        : out.reason === 'saturated'
          ? 'Channel queue is saturated — try again in a moment.'
          : out.reason === 'invalid'
            ? 'The reply was rejected as invalid.'
            : 'The reply could not be sent.'
    setComposer({ kind: 'error', message })
  }

  async function toggleTakeover(on: boolean): Promise<void> {
    if (selected === null) return
    await setTakeover(selected, on, fetcher)
    refetchInbox()
  }

  async function openNewSession(): Promise<void> {
    if (selected === null) return
    await newSession(selected, fetcher)
    setViewSession(null)
    refetchConversation()
    refetchInbox()
  }

  async function confirmDelete(): Promise<void> {
    if (selected === null) return
    if (confirm === 'conversation') {
      await deleteConversation(selected, fetcher)
      setSelected(null)
      setTurns([])
      setSessions([])
    } else if (confirm === 'session' && viewSession !== null) {
      await deleteSession(selected, viewSession, fetcher)
      setViewSession(null)
    }
    setConfirm('none')
    refetchInbox()
    refetchConversation()
  }

  return (
    <div className="console">
      <aside className="console-inbox" aria-label="Conversations">
        <div className="console-inbox-head">
          <h2 className="console-title">Conversations</h2>
          <button
            type="button"
            className="console-newchat"
            onClick={() =>
              openConversation(`console::chat-${crypto.randomUUID().slice(0, 8)}`, null)
            }
          >
            New chat
          </button>
        </div>
        <input
          type="search"
          className="console-search"
          aria-label="Filter or search messages"
          placeholder="Filter — Enter searches messages"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            if (e.target.value.trim() === '') setHits(null)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') void runSearch()
          }}
        />
        {hits !== null && (
          <section className="console-hits" aria-label="Search results">
            <h3 className="console-hits-title">
              {hits.length === 0
                ? 'No messages found.'
                : `${hits.length} message${hits.length === 1 ? '' : 's'}`}
            </h3>
            <ul className="console-list">
              {hits.map((h) => {
                const { channel, id } = splitKey(h.key)
                return (
                  <li key={`${h.key}:${h.session}:${h.seq}`}>
                    <button type="button" className="console-row" onClick={() => openHit(h)}>
                      <span className="console-row-head">
                        <span className="console-row-channel">{channelLabel(channel)}</span>
                        <span className="console-row-id">{id}</span>
                        <span className="console-row-id">s{h.session}</span>
                      </span>
                      <span className="console-row-meta console-hit-content">{h.content}</span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </section>
        )}
        {hits === null && visibleConvs.length === 0 && (
          <p className="console-empty">
            {convs.length === 0 ? 'No conversations yet.' : 'Nothing matches the filter.'}
          </p>
        )}
        {hits === null && (
          <ul className="console-list">
            {visibleConvs.map((c) => {
              const { channel, id } = splitKey(c.key)
              const unread = unreadCount(c, lastSeen)
              return (
                <li key={c.key}>
                  <button
                    type="button"
                    className="console-row"
                    data-unread={unread > 0 ? 'true' : undefined}
                    aria-current={c.key === selected ? 'true' : undefined}
                    onClick={() => openConversation(c.key, null)}
                  >
                    <span className="console-row-head">
                      <span className="console-row-channel">{channelLabel(channel)}</span>
                      <span className="console-row-id">{id}</span>
                      {c.taken_over && <span className="console-badge">TAKEN OVER</span>}
                      {unread > 0 && <span className="console-unread">{unread}</span>}
                    </span>
                    <span className="console-row-meta">
                      <span>
                        {c.session_count} {c.session_count === 1 ? 'session' : 'sessions'}
                      </span>
                      {c.last_role !== undefined && c.last_role !== '' && (
                        <span> · last: {c.last_role}</span>
                      )}
                      {c.last_activity !== undefined && c.last_activity !== '' && (
                        <span> · {relTime(c.last_activity, nowMs)}</span>
                      )}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </aside>

      <section className="console-pane">
        {current === null ? (
          <p className="console-empty">Select a conversation.</p>
        ) : (
          <>
            <header className="console-head">
              <div>
                <h2 className="console-title">
                  {channelLabel(splitKey(current.key).channel)} · {splitKey(current.key).id}
                </h2>
                <p className="console-charge">
                  {isConsoleKey(current.key)
                    ? 'Direct chat — you already are the human here; the brain answers as itself.'
                    : current.taken_over
                      ? 'You are handling this conversation — the brain is silenced.'
                      : 'Korvun is handling this conversation.'}
                </p>
              </div>
              <div className="console-controls">
                {isConsoleKey(current.key) ? (
                  <button type="button" disabled>
                    Take over
                  </button>
                ) : current.taken_over ? (
                  <button type="button" onClick={() => void toggleTakeover(false)}>
                    Release
                  </button>
                ) : (
                  <button type="button" onClick={() => void toggleTakeover(true)}>
                    Take over
                  </button>
                )}
                <button type="button" onClick={() => void openNewSession()}>
                  New session
                </button>
                <button
                  type="button"
                  className="console-danger"
                  onClick={() => setConfirm('conversation')}
                >
                  Delete conversation
                </button>
              </div>
            </header>

            {sessions.length > 1 && (
              <nav className="console-sessions" aria-label="Sessions">
                {sessions.map((s) => (
                  <button
                    type="button"
                    key={s.id}
                    aria-pressed={(viewSession ?? activeSessionID) === s.id ? 'true' : 'false'}
                    onClick={() => {
                      setViewSession(s.id === activeSessionID ? null : s.id)
                      setConfirm('none')
                    }}
                  >
                    Session {s.id}
                    {s.id === activeSessionID ? ' (active)' : ''}
                  </button>
                ))}
              </nav>
            )}

            {archived && (
              <div className="console-archived-row">
                <p className="console-archived">Archived session — read only</p>
                <button
                  type="button"
                  className="console-danger"
                  onClick={() => setConfirm('session')}
                >
                  Delete session
                </button>
              </div>
            )}

            {confirm !== 'none' && (
              <div className="console-confirm" role="alertdialog" aria-label="Confirm deletion">
                <p>{confirm === 'conversation' ? CONFIRM_CONVERSATION : CONFIRM_SESSION}</p>
                <div className="console-confirm-actions">
                  <button
                    type="button"
                    className="console-danger"
                    onClick={() => void confirmDelete()}
                  >
                    Delete
                  </button>
                  <button type="button" onClick={() => setConfirm('none')}>
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {turns.length === 0 && (
              <p className="console-empty">
                {archived ? 'This session is empty.' : 'No messages in this session yet.'}
              </p>
            )}
            <ol
              className="console-turns"
              ref={turnsRef}
              onScroll={() => {
                const el = turnsRef.current
                if (el !== null) pinnedRef.current = isNearBottom(el)
              }}
            >
              {turns.map((t) => (
                <li key={t.seq} className="console-turn" data-role={t.role} data-seq={t.seq}>
                  <span className="console-turn-role">{roleLabel(t.role)}</span>
                  <span className="console-turn-content">{t.content}</span>
                  <span className="console-turn-time">{relTime(t.timestamp, nowMs)}</span>
                </li>
              ))}
            </ol>

            {!archived && (
              <form
                className="console-composer"
                onSubmit={(e) => {
                  e.preventDefault()
                  void submitReply()
                }}
              >
                {coreDown && (
                  <p className="console-composer-reason">
                    Core is stopped — start Korvun to reply.
                  </p>
                )}
                {thinking && isConsoleKey(current.key) && (
                  <p className="console-composer-state">Thinking…</p>
                )}
                {composer.kind === 'inflight' && (
                  <p className="console-composer-state">On its way…</p>
                )}
                {composer.kind === 'error' && (
                  <p className="console-composer-error" role="alert">
                    {composer.message}
                  </p>
                )}
                <div className="console-composer-row">
                  <textarea
                    rows={1}
                    aria-label={isConsoleKey(current.key) ? 'Message Korvun' : 'Reply as operator'}
                    placeholder={
                      isConsoleKey(current.key)
                        ? 'Message Korvun… (Enter sends, Shift+Enter for a new line)'
                        : 'Reply as operator… (Enter sends, Shift+Enter for a new line)'
                    }
                    value={draft}
                    disabled={coreDown}
                    onChange={(e) => {
                      setDraft(e.target.value)
                      if (composer.kind !== 'idle') setComposer({ kind: 'idle' })
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        void submitReply()
                      }
                    }}
                  />
                  <button type="submit" disabled={coreDown || draft.trim() === ''}>
                    Send
                  </button>
                </div>
              </form>
            )}
          </>
        )}
      </section>
    </div>
  )
}
