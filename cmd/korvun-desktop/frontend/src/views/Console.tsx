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
import { brainForChannel, deadBrainForConversation } from '../console/health'
import { brainOfConversationId, displayIdOf, newDirectChatKey } from '../console/brainid'
import { markRead, readLastSeen, unreadCount } from '../console/unread'
import { relTime } from '../console/time'
import { isNearBottom } from '../console/scroll'

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

/** The live feed reduced to a change counter — the console's re-fetch tick.
 * The `live` flip counts as a change on purpose: the stream opening late
 * (core just started) must trigger a catch-up re-fetch for the events it
 * never saw. */
/** Hard cap on optimistic echoes kept in state/DOM (re-review F6). */
const MAX_PENDING_ECHOES = 20

type PendingEcho = {
  id: number
  key: string
  content: string
  state: 'sending' | 'failed'
  /** User turns with this content ALREADY in the store at send time: only
   * turns beyond this baseline reconcile this echo — a historical identical
   * message must never swallow a new send (re-review follow-up). */
  baseline: number
  /** System turns in the store at send time; one NEW system ack retires ONE
   * pending command, oldest first — never all of them at once. */
  sysBaseline: number
}
type TurnLike = { role: string; content: string }

// retiredPendingIds is the single reconciliation rule, shared by the effect
// and the render filter (estreno E-14 + its re-review follow-up): each NEW
// store turn beyond a pending's own baseline retires exactly ONE pending,
// oldest first — per content for messages, per system ack for commands.
function retiredPendingIds(pendings: PendingEcho[], turns: TurnLike[], key: string): Set<number> {
  const out = new Set<number>()
  const claimed = new Map<string, number>()
  let claimedSys = 0
  const sysCount = turns.filter((t) => t.role === 'system').length
  for (const p of pendings) {
    if (p.key !== key) continue
    if (p.state === 'failed') {
      // A failed echo is SUPERSEDED once any turn with its content lands
      // beyond its own baseline (the retry succeeded): keeping a "Not sent"
      // next to the identical delivered message is stale, and failed
      // entries otherwise never retire at all (re-review follow-up F6).
      const covered = turns.filter((t) => t.role === 'user' && t.content === p.content).length
      if (covered > p.baseline) out.add(p.id)
      continue
    }
    if (p.state !== 'sending') continue
    if (p.content.startsWith('/')) {
      // A system command never persists a user turn — its SYSTEM ack is the
      // reconciliation signal, one ack per command.
      if (sysCount >= p.sysBaseline + claimedSys + 1) {
        out.add(p.id)
        claimedSys++
      }
      continue
    }
    const covered = turns.filter((t) => t.role === 'user' && t.content === p.content).length
    const used = claimed.get(p.content) ?? 0
    if (covered >= p.baseline + used + 1) {
      out.add(p.id)
      claimed.set(p.content, used + 1)
    }
  }
  return out
}

function visiblePendings(pendings: PendingEcho[], turns: TurnLike[], key: string): PendingEcho[] {
  const retired = retiredPendingIds(pendings, turns, key)
  return pendings.filter((p) => p.key === key && !retired.has(p.id))
}

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
  // Honest-wait clock (pega 4-UI): when the thinking started, so the caption
  // can turn plain-words after ~10s of a slow local model.
  const [thinkingSince, setThinkingSince] = useState<number | null>(null)
  const [thinkingLong, setThinkingLong] = useState(false)
  // B12 (sealed design b11-b12-honest-chat.md): the visible failure. The
  // band is UI state ONLY — decision 2: the store keeps real turns, the
  // permanent record is Actividad's feed. handle_failed frames arrive
  // through the live feed store; the seq baseline scopes them to the
  // in-flight request.
  const feed = useFeed()
  const [failBand, setFailBand] = useState<{ brain: string; repeat: boolean } | null>(null)
  const [waitNotice, setWaitNotice] = useState<'none' | 'shown' | 'dismissed'>('none')
  const failSeqBaseline = useRef(0)
  const lastSentRef = useRef<{ key: string; text: string } | null>(null)
  const consecutiveFails = useRef(0)
  // Optimistic echoes (pega 2, list since estreno E-14): EVERY in-flight
  // send paints the instant Send is pressed and reconciles with ITS real
  // pair — a second rapid send must not overwrite the first (the
  // single-slot bug); a failed send stays visible, marked — never a
  // silent vanish.
  const [pendings, setPendings] = useState<PendingEcho[]>([])
  const nextPendingId = useRef(0)
  const [nowMs, setNowMs] = useState(() => Date.now())

  const turnsRef = useRef<HTMLOListElement | null>(null)
  const pinnedRef = useRef(true)

  // B9 (sealed design ola2-designs §1): the New chat brain selector.
  // null = closed; brains null = the «Cargando cerebros…» flash while the
  // /api/brains + /api/config pair is in flight.
  const [brainMenu, setBrainMenu] = useState<{
    brains: { name: string; sensitivity: string }[] | null
    chosen: string | null
  } | null>(null)
  const brainMenuRef = useRef<HTMLDivElement | null>(null)
  const brainMenuOpen = brainMenu !== null

  // Sealed cancellation gestures: Esc AND click-outside, both leaving no
  // trace. Bound only while the menu is open, removed on close/unmount.
  useEffect(() => {
    if (!brainMenuOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeBrainMenu()
    }
    const onDown = (e: MouseEvent) => {
      if (brainMenuRef.current && !brainMenuRef.current.contains(e.target as Node)) {
        closeBrainMenu()
      }
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('mousedown', onDown)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('mousedown', onDown)
    }
    // closeBrainMenu is a stable-by-shape function declaration below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [brainMenuOpen])

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

  // N6: the serving brain of the OPEN conversation, when the core observed
  // every one of its models unreachable — the warning paints before the user
  // types into the void. Refreshed with the catch-up tick, so a fixed model
  // clears the banner within one poll.
  const [deadBrain, setDeadBrain] = useState<string | null>(null)
  useEffect(() => {
    if (selected === null) {
      setDeadBrain(null)
      return
    }
    let ignore = false
    void deadBrainForConversation(selected, fetcher).then((name) => {
      if (!ignore) setDeadBrain(name)
    })
    return () => {
      ignore = true
    }
  }, [fetcher, selected, pollTick])

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

  // The brain answered (or the ack landed): thinking ends. A real answer
  // also resets the B12 failure streak — the repeat line is for
  // CONSECUTIVE failures only.
  useEffect(() => {
    if (turns.length === 0) return
    const last = turns[turns.length - 1]
    if (last.role === 'assistant' || last.role === 'system') {
      setThinking(false)
      setThinkingSince(null)
      if (last.role === 'assistant') consecutiveFails.current = 0
    }
  }, [turns])

  // B12 — the failure CUTS the wait and speaks (sealed estado 3): a NEW
  // handle_failed frame on the console channel, while this conversation
  // waits, ends the thinking and paints the band. Correlation: the seq
  // baseline captured at send time (one operator, one in-flight wait).
  useEffect(() => {
    if (!thinking || selected === null || !isConsoleKey(selected)) return
    const hit = feed.frames.find(
      (f) =>
        f.type === 'handle_failed' &&
        f.channel === 'console' &&
        (f.seq ?? 0) > failSeqBaseline.current,
    )
    if (hit === undefined) return
    failSeqBaseline.current = hit.seq ?? failSeqBaseline.current
    setThinking(false)
    setThinkingSince(null)
    setWaitNotice('none')
    const fallback = servingBrainInfo(selected).brain ?? ''
    setFailBand({
      brain: hit.brain !== undefined && hit.brain !== '' ? hit.brain : fallback,
      repeat: consecutiveFails.current > 0,
    })
    consecutiveFails.current++
    // servingBrainInfo reads refs only — stable by construction.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [feed.frames, thinking, selected])

  // Reconcile the optimistic echoes with the shared per-baseline rule
  // (retiredPendingIds): each NEW store turn retires exactly one pending.
  useEffect(() => {
    setPendings((prev) => {
      if (prev.length === 0 || selected === null) return prev
      const retired = retiredPendingIds(prev, turns, selected)
      if (retired.size === 0) return prev
      return prev.filter((p) => !retired.has(p.id))
    })
  }, [turns, selected])

  // The honest-wait tick: after ~10s of thinking, say it in plain words —
  // and after 60s, the B12 threshold NOTICE (sealed estado 4: it warns,
  // it never cuts alone; 'dismissed' means the user chose to keep waiting
  // and this request never re-warns).
  useEffect(() => {
    if (thinkingSince === null) {
      setThinkingLong(false)
      return
    }
    const tick = () => {
      const elapsed = Date.now() - thinkingSince
      setThinkingLong(elapsed > 10_000)
      if (elapsed > 60_000) {
        setWaitNotice((w) => (w === 'none' ? 'shown' : w))
      }
    }
    tick()
    const id = setInterval(tick, 1_000)
    return () => clearInterval(id)
  }, [thinkingSince])

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

  // B9 — the sealed selector lifecycle. The brain list PREFETCHES on mount
  // and is cached, so the click decides synchronously: ≤1 brain keeps
  // today's one-click create byte-for-byte; >1 opens the menu with the
  // rows already painted. «Cargando cerebros…» is the honest flash for
  // the click that arrives before the prefetch resolved. Esc and
  // click-outside cancel leaving no trace; the sequence token kills an
  // in-flight open the user cancelled first.
  const brainMenuSeq = useRef(0)
  // The pane clears the INSTANT New chat is pressed (the approved
  // no-stale-turns contract); a cancelled menu restores what was open —
  // "no deja rastro" in both directions.
  const brainMenuPrev = useRef<string | null>(null)
  const brainChoices = useRef<{
    brains: { name: string; sensitivity: string; model: { id: string; locality: string } | null }[]
    def: string | null
  } | null>(null)
  const brainChoicesInFlight = useRef<Promise<{
    brains: { name: string; sensitivity: string; model: { id: string; locality: string } | null }[]
    def: string | null
  }> | null>(null)

  const loadBrainChoices = useCallback(() => {
    const p = (async () => {
      let brains: {
        name: string
        sensitivity: string
        model: { id: string; locality: string } | null
      }[] = []
      let routes: unknown
      try {
        const [brainsResp, cfgResp] = await Promise.all([
          fetcher('/api/brains', { cache: 'no-store' }),
          fetcher('/api/config', { cache: 'no-store' }),
        ])
        if (brainsResp.ok) {
          const raw = (await brainsResp.json()) as unknown
          if (Array.isArray(raw)) {
            brains = raw
              .filter(
                (b): b is { name: string; sensitivity?: unknown; models?: unknown } =>
                  b !== null &&
                  typeof b === 'object' &&
                  typeof (b as { name?: unknown }).name === 'string',
              )
              .map((b) => {
                // B11: the FIRST model's identity feeds the truthful wait
                // (the sealed «{model_id} · {local|nube}» detail). Shape is
                // defensive — anything unexpected degrades to null detail.
                let model: { id: string; locality: string } | null = null
                if (Array.isArray(b.models) && b.models.length > 0) {
                  const m = b.models[0] as { model_id?: unknown; locality?: unknown }
                  if (m !== null && typeof m === 'object' && typeof m.model_id === 'string') {
                    model = {
                      id: m.model_id,
                      locality: typeof m.locality === 'string' ? m.locality : '',
                    }
                  }
                }
                return {
                  name: b.name,
                  sensitivity: typeof b.sensitivity === 'string' ? b.sensitivity : '',
                  model,
                }
              })
          }
        }
        if (cfgResp.ok) routes = ((await cfgResp.json()) as { routes?: unknown })?.routes
      } catch {
        brains = []
      }
      const out = { brains, def: brainForChannel(routes, 'console') }
      brainChoices.current = out
      return out
    })()
    brainChoicesInFlight.current = p
    return p
  }, [fetcher])

  // B11 — who is actually serving the open conversation: the B9 id first,
  // the console route as fallback; the model detail from the cached
  // /api/brains list. Dropping detail is honest; inventing it never is.
  const servingBrainInfo = (
    key: string | null,
  ): { brain: string | null; model: { id: string; locality: string } | null } => {
    if (key === null || !isConsoleKey(key)) return { brain: null, model: null }
    const cached = brainChoices.current
    const brain = brainOfConversationId(splitKey(key).id) ?? cached?.def ?? null
    if (brain === null) return { brain: null, model: null }
    const row = cached?.brains.find((b) => b.name === brain)
    return { brain, model: row?.model ?? null }
  }

  // B11 — the sealed truthful-wait mold (b11-b12-honest-chat.md, estado
  // 1/2): «{brain} está pensando — {model_id} · {local|nube}…»; the long
  // wait forks by the TRUTH (the old hardcoded local line survives only
  // when it is literally true); degradation drops detail, never invents.
  const thinkingCaption = (long: boolean): string => {
    const { brain, model } = servingBrainInfo(selected)
    if (brain === null) return long ? 'Sigue pensando…' : 'Pensando…'
    if (model === null || model.id === '') {
      return long ? `${brain} sigue pensando…` : `${brain} está pensando…`
    }
    if (!long) {
      const where = model.locality === 'cloud' ? 'nube' : 'local'
      return `${brain} está pensando — ${model.id} · ${where}…`
    }
    return model.locality === 'cloud'
      ? `${brain} sigue sin responder — la petición está en la nube…`
      : `${model.id} sigue pensando — un modelo local puede tardar en esta máquina…`
  }

  useEffect(() => {
    void loadBrainChoices()
  }, [loadBrainChoices])

  function closeBrainMenu(): void {
    brainMenuSeq.current++
    setBrainMenu(null)
    if (brainMenuPrev.current !== null) {
      openConversation(brainMenuPrev.current, null)
      brainMenuPrev.current = null
    }
  }

  function decideFromChoices(c: {
    brains: { name: string; sensitivity: string }[]
    def: string | null
  }): void {
    if (c.brains.length <= 1) {
      // Today's behavior byte-for-byte: one click, straight to the chat.
      brainMenuPrev.current = null
      setBrainMenu(null)
      openConversation(newDirectChatKey(null), null)
      return
    }
    const chosen =
      c.def !== null && c.brains.some((b) => b.name === c.def) ? c.def : c.brains[0].name
    setBrainMenu({ brains: c.brains, chosen })
  }

  function openBrainMenu(): void {
    // The pane goes clean NOW (the approved no-stale-turns contract) —
    // whatever was open is remembered and restored on cancel.
    brainMenuPrev.current = selected
    setSelected(null)
    setTurns([])
    setSessions([])
    setViewSession(null)
    const cached = brainChoices.current
    if (cached !== null) {
      decideFromChoices(cached)
      void loadBrainChoices() // background freshness for the next open
      return
    }
    const seq = ++brainMenuSeq.current
    setBrainMenu({ brains: null, chosen: null })
    void (brainChoicesInFlight.current ?? loadBrainChoices()).then((c) => {
      if (seq !== brainMenuSeq.current) return // cancelled while loading
      decideFromChoices(c)
    })
  }

  function createFromBrainMenu(): void {
    if (brainMenu === null || brainMenu.chosen === null) return
    const key = newDirectChatKey(brainMenu.chosen)
    brainMenuPrev.current = null // creating IS the outcome — nothing to restore
    closeBrainMenu()
    openConversation(key, null)
  }

  function openConversation(key: string, session: number | null): void {
    setSelected(key)
    setViewSession(session)
    setComposer({ kind: 'idle' })
    setConfirm('none')
    // B12: switching conversations closes the failure band and the wait
    // notice — they belong to the conversation that produced them.
    setFailBand(null)
    setWaitNotice('none')
    pinnedRef.current = true
    // The pane belongs to the NEW conversation from this instant: stale
    // turns from the previous one must never show under its title (the
    // 2026-08-09 duplicated-conversation bug — they also suppressed the
    // optimistic echo via the same-content reconciliation).
    setTurns([])
    setSessions([])
    setPendings([])
    setThinking(false)
    setThinkingSince(null)
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

  // sendDirectText is the ONE direct-chat send path (extracted for B12: the
  // [Reintentar] gestures ride the same rail as the composer — echo, honest
  // wait, reconciliation, band resets included).
  async function sendDirectText(text: string): Promise<boolean> {
    if (selected === null) return false
    setComposer({ kind: 'idle' })
    // B12 resets: a fresh send opens a fresh honest wait.
    setFailBand(null)
    setWaitNotice('none')
    failSeqBaseline.current = feed.frames.reduce((m, f) => Math.max(m, f.seq ?? 0), 0)
    lastSentRef.current = { key: selected, text }
    const pendingId = ++nextPendingId.current
    const baseline = turns.filter((t) => t.role === 'user' && t.content === text).length
    const sysBaseline = turns.filter((t) => t.role === 'system').length
    setPendings((p) => {
      // Hard retention cap (re-review follow-up F6): a long outage with
      // retries must not grow state and DOM without bound. Oldest FAILED
      // entries evict first; sending ones only under a pathological pile.
      const next = [
        ...p,
        {
          id: pendingId,
          key: selected,
          content: text,
          state: 'sending' as const,
          baseline,
          sysBaseline,
        },
      ]
      if (next.length > MAX_PENDING_ECHOES) {
        const failedIdx = next.findIndex((x) => x.state === 'failed')
        next.splice(failedIdx >= 0 ? failedIdx : 0, 1)
      }
      return next
    })
    setThinking(true)
    setThinkingSince(Date.now())
    pinnedRef.current = true
    const sent = await sendUserMessage(selected, text, fetcher)
    if (sent.ok) {
      refetchConversation()
      refetchInbox()
      return true
    }
    // The echo NEVER vanishes silently: it stays, marked as not sent.
    setPendings((p) => p.map((x) => (x.id === pendingId ? { ...x, state: 'failed' } : x)))
    setThinking(false)
    setThinkingSince(null)
    setComposer({
      kind: 'error',
      message:
        sent.reason === 'not-wired'
          ? 'Console channel not wired in the running core.'
          : sent.reason === 'saturated'
            ? 'Brain queue is saturated — try again in a moment.'
            : 'The message could not be sent.',
    })
    return false
  }

  // B12 — [Reintentar]: resend the LAST user message through the same rail.
  function retryLastSend(): void {
    const last = lastSentRef.current
    if (last === null || last.key !== selected) {
      setFailBand(null)
      return
    }
    void sendDirectText(last.text)
  }

  // B12 — [Cancelar y reintentar]: stop waiting on THIS request (decision
  // 1: a late answer still paints as a normal turn — cancel means stop
  // waiting, never hide) and resend.
  function cancelAndRetry(): void {
    setThinking(false)
    setThinkingSince(null)
    setWaitNotice('none')
    retryLastSend()
  }

  async function submitReply(): Promise<void> {
    if (selected === null || draft.trim() === '') return
    if (isConsoleKey(selected)) {
      // Direct chat: the user's turn ECHOES INSTANTLY (pega 2) and the
      // honest wait starts from the send instant (pega 4-UI) — the store's
      // final pair reconciles both later. The send itself is the shared
      // sendDirectText rail (B12's retry rides it too).
      const ok = await sendDirectText(draft)
      if (ok) setDraft('')
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
          <div className="console-newchat-wrap" ref={brainMenuRef}>
            <button type="button" className="console-newchat" onClick={openBrainMenu}>
              New chat <span aria-hidden="true">▾</span>
            </button>
            {brainMenu !== null && (
              <div className="console-brainmenu" role="dialog" aria-label="¿Con qué cerebro?">
                <p className="console-brainmenu-title">¿Con qué cerebro?</p>
                {brainMenu.brains === null ? (
                  <p className="console-brainmenu-loading">Cargando cerebros…</p>
                ) : (
                  <>
                    {brainMenu.brains.map((b) => (
                      <label key={b.name} className="console-brainmenu-row">
                        <input
                          type="radio"
                          name="newchat-brain"
                          checked={brainMenu.chosen === b.name}
                          onChange={() =>
                            setBrainMenu({ brains: brainMenu.brains, chosen: b.name })
                          }
                        />
                        <span className="console-brainmenu-name">{b.name}</span>
                        <span className="console-brainmenu-privacy">
                          {b.sensitivity === 'private' ? 'Privado' : 'Público'}
                        </span>
                      </label>
                    ))}
                    <div className="console-brainmenu-actions">
                      <button
                        type="button"
                        className="console-brainmenu-create"
                        onClick={createFromBrainMenu}
                      >
                        Crear chat
                      </button>
                    </div>
                  </>
                )}
              </div>
            )}
          </div>
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
                      {brainOfConversationId(id) !== null && (
                        <span className="console-brain-badge" data-testid="inbox-brain-badge">
                          {brainOfConversationId(id)}
                        </span>
                      )}
                      <span className="console-row-id">{displayIdOf(id)}</span>
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
                  {(() => {
                    // B9 badge: an addressed conversation composes the
                    // sealed «Console · brain · id» header; legacy ids
                    // render exactly as before.
                    const { channel, id } = splitKey(current.key)
                    const brain = brainOfConversationId(id)
                    return brain !== null
                      ? `${channelLabel(channel)} · ${brain} · ${displayIdOf(id)}`
                      : `${channelLabel(channel)} · ${id}`
                  })()}
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

            {turns.length === 0 && (archived || pendings.length === 0) && (
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
                <li
                  key={t.seq}
                  className="console-turn"
                  data-role={t.role}
                  data-seq={t.seq}
                  data-side={isConsoleKey(current.key) && t.role === 'user' ? 'own' : undefined}
                >
                  <span className="console-turn-role">{roleLabel(t.role)}</span>
                  <span className="console-turn-content">{t.content}</span>
                  <span className="console-turn-time">{relTime(t.timestamp, nowMs)}</span>
                </li>
              ))}
              {visiblePendings(pendings, turns, current.key).map((p) => (
                <li
                  key={`pending-${p.id}`}
                  className="console-turn"
                  data-role="user"
                  data-side="own"
                  data-send-state={p.state}
                >
                  <span className="console-turn-role">{roleLabel('user')}</span>
                  <span className="console-turn-content">{p.content}</span>
                  <span className="console-turn-time">
                    {p.state === 'failed' ? 'Not sent' : 'Sending…'}
                  </span>
                </li>
              ))}
            </ol>

            {!archived && deadBrain !== null && (
              <p className="console-brain-warning" data-testid="brain-health-warning" role="alert">
                Brain “{deadBrain}” has no live models — every boot probe failed. Check its model in
                the Builder before chatting.
              </p>
            )}
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
                {/* B12 (sealed): the failure lives ON SCREEN as a band —
                    never an eternal wait, never a persisted turn. */}
                {failBand !== null && (
                  <div className="console-fail-band" role="alert">
                    <span className="band-icon" aria-hidden="true">
                      ⚠
                    </span>
                    <span>
                      {failBand.brain !== '' ? failBand.brain : 'El brain'} no pudo responder esta
                      vez — fallo del proveedor o del modelo.
                      {failBand.repeat && (
                        <span className="console-fail-second">
                          Se repite. Revisa el modelo de{' '}
                          {failBand.brain !== '' ? failBand.brain : 'ese brain'} en el Builder o su
                          clave en Ajustes → Secretos.
                        </span>
                      )}
                    </span>
                    <button type="button" className="btn-small" onClick={retryLastSend}>
                      Reintentar
                    </button>
                  </div>
                )}
                {/* B12 (sealed): the 60 s threshold WARNS, never cuts alone. */}
                {thinking && waitNotice === 'shown' && isConsoleKey(current.key) && (
                  <div className="console-wait-band" role="alert">
                    <span className="band-icon" aria-hidden="true">
                      ⚠
                    </span>
                    <span>
                      Sin respuesta de {servingBrainInfo(current.key).brain ?? 'ese brain'} tras un
                      minuto. Puedes seguir esperando o reintentar.
                    </span>
                    <span className="console-wait-actions">
                      <button
                        type="button"
                        className="btn-small"
                        onClick={() => setWaitNotice('dismissed')}
                      >
                        Seguir esperando
                      </button>
                      <button type="button" className="btn-small" onClick={cancelAndRetry}>
                        Cancelar y reintentar
                      </button>
                    </span>
                  </div>
                )}
                {thinking && waitNotice !== 'shown' && isConsoleKey(current.key) && (
                  <p className="console-composer-state">{thinkingCaption(thinkingLong)}</p>
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
