// Aprobaciones (v0.15.0, spec v36) — a parked irreversible action is read and
// decided inside the window.
//
// Three rules govern every line below, and each one is a scenario in §12:
//
//  1. RENDER BY NAME. A refusal is routed by its `error` name, never by the
//     English text that rides with it. An unknown name prints raw and never
//     degrades to an empty list or a generic shrug (FR-UI-37).
//  2. NEVER OFFER WHAT THE ANSWER IN HAND SAYS THE STORE WOULD REFUSE. A
//     coherent but stale snapshot is not knowledge, so the button stays; an
//     answer that says the row is no longer PENDING removes it.
//  3. THE SCREEN NEVER CLAIMS AN EFFECT HAPPENED OR DID NOT more strongly than
//     its evidence allows. Several outcomes answer "no se sabe" on purpose.
//
// The document is read-through: it is not painted until the store has returned
// parameters that re-derive the digest (FR-UI-62). That check runs on the
// server; this screen shows the confirmation and refuses to render without it.
import './approvals.css'
import { Fragment, useCallback, useEffect, useRef, useState } from 'react'
import type { JSX } from 'react'
import { desktop } from '../lib/go'
import {
  ESC_LINE,
  LOOPBACK_ONLY,
  MALFORMED_ROW_ID,
  OUTCOME_TEXT,
  PARAMS_STATE_TEXT,
  PERMANENT_LINE,
  POST_PARAMS_MISMATCH,
  SURFACE_NOT_MOUNTED,
  UNKNOWN_NAME_TITLE,
  UNREADABLE_TITLE,
  UNRECOGNISED_SUCCESS,
  classBanner,
} from './approvalsText'

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

interface Row {
  id: string
  action_id: string
  operation: string
  effect_class: string
  expires_at: string
  digest: string
  origin: string
}
interface Detail extends Row {
  purpose: string
  principal_id: string
  reversibility: string
  tool_cage: string
  required_rule: string
  law_digest: string
  parameters: string
  parameters_state: string
  brain_gone?: boolean
  authority?: unknown
}
interface ApprovalAuthority {
  requester_principal_id: string
  intent_id: string
  intent_purpose: string
  principal_chain: string[]
  budget: { kind: 'finite'; remaining: number } | { kind: 'unlimited' }
}
interface Gate {
  approvals_enabled: boolean
  brains_total: number
  brains_can_park: number
  /** Parked requests this page could NOT serve whole. The store skips a row it
   * cannot scan instead of failing the page — and if nobody paints the count,
   * the operator loses that request in silence, which is the exact failure
   * skipping it was meant to avoid. */
  rows_skipped?: number
}

/** What one request produced: a document, a named refusal, or an answer the
 * screen could not read. The three are disjoint on purpose — collapsing the
 * third into "empty" is the fail-open FR-UI-37 forbids. */
type Answer<T> =
  | { kind: 'ok'; value: T; status: number }
  | { kind: 'named'; name: string; message: string; currentLaw: string; status: number }
  | { kind: 'unreadable'; detail: string }

// ---------------------------------------------------------------------------
// Rendering of untrusted bytes (§6.2)
// ---------------------------------------------------------------------------

/** Code points a reader cannot see for what they are become visible escapes.
 * Without this a U+202E lets the document read "hooks.acme.io" while the digest
 * seals something else: the page would say one thing and the digest another.
 *
 * It judges by Unicode CLASS, not by a list: controls (Cc), format characters
 * (Cf — the bidi marks, the zero-width joiners, U+2060, the tags), lone
 * surrogates (Cs), every separator except the ASCII space (Zs, Zl, Zp), and
 * whatever Unicode declares Default_Ignorable_Code_Point (U+034F, the variation
 * selectors, U+3164). The list it replaced was cured once after the v0.15.0
 * external review ("pagar100 EUR" and "pagar<U+2060>100 EUR" sealed different
 * digests and read the same) and still missed eight members of the class.
 *
 * What it does NOT cover: characters that have a glyph, even a blank one
 * (U+2800), and characters that look like other characters. Applied to EVERY
 * field of uncontrolled origin, not to one block. */
const UNSEEN = /^[\p{Cc}\p{Cf}\p{Cs}\p{Zs}\p{Zl}\p{Zp}\p{Default_Ignorable_Code_Point}]$/u
function escapeUntrusted(s: string): string {
  let out = ''
  for (const ch of s) {
    const c = ch.codePointAt(0) ?? 0
    // The alphabet reserves its own opening bracket. Every escape printed here
    // reads `<U+XXXX>`, so untrusted bytes containing '<' can spell one: the
    // literal text `<U+2060>` would read exactly like the escape of a real
    // U+2060 and the operator could not tell the sealed bytes from the
    // rendering. Escaping '<' itself leaves no such pair.
    if (ch === '<') {
      out += '<U+003C>'
      continue
    }
    const invisible = ch !== ' ' && UNSEEN.test(ch)
    out += invisible ? `<U+${c.toString(16).toUpperCase().padStart(4, '0')}>` : ch
  }
  return out
}

type AuthorityRead =
  { ok: true; value: ApprovalAuthority | undefined } | { ok: false; detail: string }

function readAuthority(value: unknown): AuthorityRead {
  if (value === undefined) return { ok: true, value: undefined }
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return { ok: false, detail: 'authority no es un objeto' }
  }
  const a = value as Record<string, unknown>
  for (const field of ['requester_principal_id', 'intent_id', 'intent_purpose'] as const) {
    if (typeof a[field] !== 'string' || a[field] === '') {
      return { ok: false, detail: `authority.${field} ilegible` }
    }
  }
  if (
    !Array.isArray(a.principal_chain) ||
    a.principal_chain.length === 0 ||
    a.principal_chain.some((principal) => typeof principal !== 'string' || principal === '')
  ) {
    return { ok: false, detail: 'authority.principal_chain ilegible' }
  }
  if (typeof a.budget !== 'object' || a.budget === null || Array.isArray(a.budget)) {
    return { ok: false, detail: 'authority.budget ilegible' }
  }
  const budget = a.budget as Record<string, unknown>
  if (budget.kind === 'finite') {
    if (
      typeof budget.remaining !== 'number' ||
      !Number.isSafeInteger(budget.remaining) ||
      budget.remaining < 0
    ) {
      return { ok: false, detail: 'authority.budget.remaining ilegible' }
    }
  } else if (budget.kind === 'unlimited') {
    if ('remaining' in budget) {
      return { ok: false, detail: 'authority.budget.remaining no pertenece a unlimited' }
    }
  } else {
    return { ok: false, detail: 'authority.budget.kind ilegible' }
  }
  return { ok: true, value: a as unknown as ApprovalAuthority }
}

/** sha256: + 64 lowercase hex, the shape action.Digest produces. Anything else
 * is "digest ilegible": it is not printed as a digest, not shortened, and not
 * fed to the tail-collision detector. */
const DIGEST_RE = /^sha256:[0-9a-f]{64}$/
function isDigest(d: string): boolean {
  return DIGEST_RE.test(d)
}
const tailOf = (d: string): string => d.slice(-6)

/** Compatibility apr_ and strict apr3_ ids, each followed by 32 lowercase
 * hex. An id is untrusted bytes from the core: a row outside these two shapes
 * is listed and never opened, and no URL is ever built from one. */
const APPROVAL_ID_RE = /^(?:apr|apr3)_[0-9a-f]{32}$/
function isApprovalID(id: string): boolean {
  return APPROVAL_ID_RE.test(id)
}

/** rcpt_ + 32 lowercase hex, the shape action.NewReceiptID mints. */
const RECEIPT_ID_RE = /^rcpt_[0-9a-f]{32}$/
function isReceiptID(id: string): boolean {
  return RECEIPT_ID_RE.test(id)
}

/** The committed outcome of a POST, judged by protocol (P2-10): only a 200
 * with the required fields for its outcome paints that outcome. `executed` and
 * `failed` require a minted receipt shape, a `result` that is a non-empty
 * string, and a `digest` that is not merely well shaped but IDENTICAL to the
 * one this screen sent with the request, so an answer about another action is
 * never painted as this one's. `rejected` keeps the contract it had: its
 * receipt is taken as the string it is — the screen escapes it where it prints
 * it — and a `digest` or `result` of the wrong TYPE still refuses the whole
 * answer. Nothing else paints anything, for either verb. */
function judgeDecision(
  verb: 'approve' | 'reject',
  status: number,
  body: unknown,
  sentDigest: string,
): Exclude<Decision, null | { kind: 'sending' } | { kind: 'named' } | { kind: 'lost' }> | null {
  if (status !== 200 || typeof body !== 'object' || body === null) return null
  const b = body as Record<string, unknown>
  if (typeof b.outcome !== 'string' || typeof b.receipt_id !== 'string') return null
  const receipt = b.receipt_id
  const optional = (v: unknown): v is string | undefined => v === undefined || typeof v === 'string'
  if (verb === 'reject') {
    if (!optional(b.digest) || !optional(b.result)) return null
    return b.outcome === 'rejected' ? { kind: 'rejected', receipt } : null
  }
  if (!isReceiptID(receipt)) return null
  if (typeof b.digest !== 'string' || !isDigest(b.digest)) return null
  if (b.digest !== sentDigest) return null
  if (typeof b.result !== 'string' || b.result === '') return null
  const digest = b.digest
  const result = b.result
  if (b.outcome === 'executed') return { kind: 'executed', digest, result, receipt }
  // The tool ran and said no. It is a KNOWN outcome with its receipt: calling
  // it executed would be a lie, and calling it unknown would be a second one.
  if (b.outcome === 'failed') return { kind: 'failed', digest, detail: result, receipt }
  return null
}

/** The 64 hex in eight groups of eight, the ONE grouping of the document. */
function digestGroups(d: string): string[] {
  const hex = d.slice('sha256:'.length)
  return Array.from({ length: 8 }, (_, i) => hex.slice(i * 8, i * 8 + 8))
}

// ---------------------------------------------------------------------------
// Expiry (G7 of docs/superpowers/specs/2026-09-13-approvals-screen-to-mockup-pretest.md)
// ---------------------------------------------------------------------------

/** The one shape a stored expiry may have: UTC to the second, an optional
 * fraction of up to nine digits, and Z. A value that fails it is illegible and
 * is not handed to Date.parse, whose laxity differs between engines. */
const EXPIRY_RE = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/
const ILLEGIBLE_EXPIRY = 'caducidad ilegible'
/** setTimeout fires at once for a delay above this many milliseconds. */
const MAX_TIMER_MS = 2147483647

interface Expiry {
  ms: number
  date: string
  time: string
}

/** The shape, then the calendar: month 01–12, a day that exists in that month
 * (29 February in Gregorian leap years only), hour 00–23, minute and second
 * 00–59. `null` is illegible. */
function parseExpiry(v: string): Expiry | null {
  const m = EXPIRY_RE.exec(v)
  if (m === null) return null
  const [y, mo, d, h, mi, sec] = m.slice(1, 7).map(Number)
  const leap = (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]
  if (mo < 1 || mo > 12 || d < 1 || d > days[mo - 1] || h > 23 || mi > 59 || sec > 59) return null
  const frac = m[7] === undefined ? 0 : Number(m[7].padEnd(3, '0').slice(0, 3))
  const t = new Date(0)
  t.setUTCFullYear(y, mo - 1, d)
  t.setUTCHours(h, mi, sec, frac)
  return { ms: t.getTime(), date: v.slice(0, 10), time: v.slice(11, 19) }
}

const pad2 = (n: number): string => String(n).padStart(2, '0')

/** «HH:MM:SSZ» sliced from the stored string, with the stored date before it
 * when that UTC date is not the window clock's UTC date. */
function expiryStamp(e: Expiry, now: number): string {
  const today = new Date(now).toISOString().slice(0, 10)
  return `${e.date === today ? '' : `${e.date} `}${e.time}Z`
}

/** The label of the bar and of the CADUCIDAD card. Seconds are ceiled, so a
 * request that is still live does not read «0m 00s». */
function expiryLabel(e: Expiry, now: number): string {
  const s = Math.ceil((e.ms - now) / 1000)
  const at = expiryStamp(e, now)
  if (s <= 0) return `el reloj de la ventana dice 00:00 · ${at}`
  if (s < 3600) return `caduca en ${Math.floor(s / 60)}m ${pad2(s % 60)}s · ${at}`
  return `caduca en ${Math.floor(s / 3600)}h ${pad2(Math.floor((s % 3600) / 60))}m · ${at}`
}

/** The list row has no clock (FR-UI-11), so it shows the instant and no
 * countdown that would freeze. */
function RowExpiry({ value }: { value: string }): JSX.Element {
  if (value === '') return <span className="approvals-row-expiry">no caduca</span>
  const e = parseExpiry(value)
  if (e === null)
    return (
      <span className="approvals-row-expiry">
        <span>{ILLEGIBLE_EXPIRY}</span> <span>{escapeUntrusted(value)}</span>
      </span>
    )
  return <span className="approvals-row-expiry">{`caduca · ${expiryStamp(e, Date.now())}`}</span>
}

// ---------------------------------------------------------------------------
// Fetch
// ---------------------------------------------------------------------------

async function ask<T>(url: string, init?: RequestInit): Promise<Answer<T>> {
  let res: Response
  try {
    res = await fetch(url, init)
  } catch (e) {
    return { kind: 'unreadable', detail: e instanceof Error ? e.message : String(e) }
  }
  const text = await res.text().catch(() => '')
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    // A 404 whose body is not ours is the core without the approvals surface
    // mounted, which has its own literal and its own fix.
    if (res.status === 404)
      return {
        kind: 'named',
        name: 'surface_not_mounted',
        message: '',
        currentLaw: '',
        status: 404,
      }
    return { kind: 'unreadable', detail: text === '' ? 'cuerpo vacío' : text }
  }
  if (typeof body !== 'object' || body === null) {
    return { kind: 'unreadable', detail: text }
  }
  const rec = body as Record<string, unknown>
  if (typeof rec.error === 'string') {
    if (res.status === 404 && !('message' in rec)) {
      return {
        kind: 'named',
        name: 'surface_not_mounted',
        message: '',
        currentLaw: '',
        status: 404,
      }
    }
    return {
      kind: 'named',
      name: rec.error,
      message: typeof rec.message === 'string' ? rec.message : '',
      currentLaw: typeof rec.current_law_digest === 'string' ? rec.current_law_digest : '',
      status: res.status,
    }
  }
  if (!res.ok) return { kind: 'unreadable', detail: text }
  return { kind: 'ok', value: body as T, status: res.status }
}

// ---------------------------------------------------------------------------
// Shared bits
// ---------------------------------------------------------------------------

function Actions({ children }: { children: React.ReactNode }): JSX.Element {
  return <div className="approvals-actions">{children}</div>
}

/** A terminal state: a condition is role="status", an event role="alert". */
function State({
  title,
  lines,
  alert,
  children,
}: {
  title: string
  lines: string[]
  alert?: boolean
  children?: React.ReactNode
}): JSX.Element {
  return (
    <div className="approvals-state" role={alert === true ? 'alert' : 'status'}>
      <div className="approvals-state-title">{title}</div>
      {lines.map((l) => (
        <p key={l} className="approvals-state-line">
          {l}
        </p>
      ))}
      {children}
    </div>
  )
}

function ConfigFolderButton(): JSX.Element {
  const [failed, setFailed] = useState('')
  const path = useRef('')
  useEffect(() => {
    const d = desktop()
    if (d === undefined) return
    void d
      .Status()
      .then((s) => {
        path.current = s.ConfigPath
      })
      .catch(() => undefined)
  }, [])
  return (
    <>
      <button
        type="button"
        className="btn-secondary"
        onClick={() => {
          const d = desktop()
          const open = d?.OpenConfigFolder
          if (d === undefined || open === undefined) {
            setFailed(path.current)
            return
          }
          void open.call(d).catch(() => setFailed(path.current))
        }}
      >
        Abrir la carpeta de configuración
      </button>
      {failed !== '' && (
        <p role="alert">
          No se ha podido abrir la carpeta. La ruta es <span>{failed}</span>
        </p>
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// Named-answer rendering, shared by list and detail
// ---------------------------------------------------------------------------

interface NamedProps {
  answer: Extract<Answer<unknown>, { kind: 'named' }>
  onRetry: () => void
  onGoHome: () => void
  onBack?: () => void
}

/** E3's lines, one text wherever `disabled` arrives: the list, the detail, or
 * a POST. */
const DISABLED_LINES = [
  'Con las aprobaciones apagadas ya no se retiene nada nuevo',
  'Una acción irreversible se ejecuta ahora en el momento en que el agente la llama. Esto no es una bandeja vacía: es un hueco en la garantía.',
  'Y lo que se aparcó ANTES de apagar el interruptor sigue vivo en el almacén: esta pantalla no puede enseñártelo con las aprobaciones apagadas, y solo se decide desde la CLI hasta que el barrendero lo cierre.',
  'Se enciende en el perfil: approvals.enabled',
]

/** The names both surfaces share. Returns null when the caller must handle the
 * name itself (the detail's decision states). */
function SharedNamed({ answer, onRetry, onGoHome }: NamedProps): JSX.Element | null {
  switch (answer.name) {
    case 'core stopped':
      return <CoreStopped onRetry={onRetry} onGoHome={onGoHome} />
    case 'core unreachable':
      return (
        <State
          title="El núcleo no responde. El proceso figura en marcha pero no contesta. Ninguna decisión ha salido de esta ventana."
          lines={[]}
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onRetry}>
              Reintentar
            </button>
          </Actions>
        </State>
      )
    case 'disabled':
      return (
        <State title="APROBACIONES APAGADAS EN ESTE PERFIL" lines={DISABLED_LINES}>
          <Actions>
            <ConfigFolderButton />
          </Actions>
        </State>
      )
    case 'forbidden':
      return (
        <State
          title="La ventana no ha podido autenticarse contra el núcleo. No ha salido ninguna decisión."
          lines={[]}
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onGoHome}>
              Ir a Inicio
            </button>
          </Actions>
        </State>
      )
    case 'surface_not_mounted':
      return (
        <State title={SURFACE_NOT_MOUNTED} lines={[]}>
          <Actions>
            <ConfigFolderButton />
          </Actions>
        </State>
      )
    case 'unavailable':
      return (
        <State
          title="El almacén no se pudo leer en este instante. Es transitorio y no dice nada sobre la evidencia."
          lines={[]}
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onRetry}>
              Reintentar
            </button>
          </Actions>
        </State>
      )
    default:
      return null
  }
}

/** E1 crossed with Status() (FR-UI-63): a 503 "core stopped" has THREE causes,
 * so the screen asks the shell before asserting any of them. */
function CoreStopped({
  onRetry,
  onGoHome,
}: {
  onRetry: () => void
  onGoHome: () => void
}): JSX.Element {
  const [running, setRunning] = useState<boolean | 'unknown'>('unknown')
  const [startError, setStartError] = useState('')
  useEffect(() => {
    const d = desktop()
    if (d === undefined) {
      setRunning('unknown')
      return
    }
    void d
      .Status()
      .then((s) => setRunning(s.Running))
      .catch(() => setRunning('unknown'))
  }, [])
  if (running === true) {
    return (
      <State
        title="Esta ventana no alcanza la puerta de aprobaciones del núcleo. El proceso está en marcha; puede ser una recarga en curso o la observabilidad apagada en el perfil."
        lines={[]}
      >
        <Actions>
          <button type="button" className="btn-secondary" onClick={onRetry}>
            Reintentar
          </button>
          <ConfigFolderButton />
        </Actions>
      </State>
    )
  }
  if (running === 'unknown') {
    return (
      <State
        title="Esta ventana no alcanza la puerta de aprobaciones, y tampoco ha podido preguntar al núcleo en qué estado está."
        lines={[]}
      >
        <Actions>
          <button type="button" className="btn-secondary" onClick={onRetry}>
            Reintentar
          </button>
        </Actions>
      </State>
    )
  }
  return (
    <State
      title="El núcleo está parado"
      lines={[
        'No se puede registrar ninguna decisión ni ejecutar ninguna acción con el gateway detenido. Lo que hubiera aparcado sigue intacto: sus digests y sus relojes de caducidad los guarda el almacén, no esta ventana.',
      ]}
    >
      <Actions>
        <button
          type="button"
          className="btn-primary"
          onClick={() => {
            const d = desktop()
            if (d === undefined) return
            void d
              .Start()
              .catch((e: unknown) => setStartError(e instanceof Error ? e.message : String(e)))
          }}
        >
          Arrancar el núcleo
        </button>
        <button type="button" className="btn-secondary" onClick={onGoHome}>
          Ir a Inicio
        </button>
      </Actions>
      {startError !== '' && <p role="alert">{startError}</p>}
    </State>
  )
}

function Unreadable({ detail, onRetry }: { detail: string; onRetry: () => void }): JSX.Element {
  return (
    <State title={UNREADABLE_TITLE} lines={[escapeUntrusted(detail)]}>
      <Actions>
        <button type="button" className="btn-secondary" onClick={onRetry}>
          Reintentar
        </button>
      </Actions>
    </State>
  )
}

function UnknownName({
  answer,
  onRetry,
}: {
  answer: Extract<Answer<unknown>, { kind: 'named' }>
  onRetry?: () => void
}): JSX.Element {
  return (
    <State
      title={UNKNOWN_NAME_TITLE}
      lines={[`${answer.status}`, escapeUntrusted(answer.name), escapeUntrusted(answer.message)]}
    >
      {onRetry !== undefined && (
        <Actions>
          <button type="button" className="btn-secondary" onClick={onRetry}>
            Reintentar
          </button>
        </Actions>
      )}
    </State>
  )
}

// ---------------------------------------------------------------------------
// The screen
// ---------------------------------------------------------------------------

export function Approvals({ onGoHome }: { onGoHome: () => void }): JSX.Element {
  const [openID, setOpenID] = useState<string | null>(null)
  return openID === null ? (
    <PendingList onOpen={setOpenID} onGoHome={onGoHome} />
  ) : (
    <RequestDetail id={openID} onBack={() => setOpenID(null)} onGoHome={onGoHome} />
  )
}

// --- P1 --------------------------------------------------------------------

function PendingList({
  onOpen,
  onGoHome,
}: {
  onOpen: (id: string) => void
  onGoHome: () => void
}): JSX.Element {
  const [answer, setAnswer] = useState<Answer<{ gate: Gate; rows: Row[] }> | null>(null)
  // The list does not refresh itself (FR-UI-11): it is asked on entering and
  // with [Actualizar]. A late answer to a superseded request never paints.
  const seq = useRef(0)
  const load = useCallback(() => {
    // The page in hand STAYS while the new one is asked. Blanking it would
    // make [Actualizar] destroy what the operator is reading, and a late
    // answer to a superseded request is dropped by the sequence number rather
    // than by luck.
    const mine = ++seq.current
    void ask<{ gate: Gate; rows: Row[] }>('/api/approvals').then((a) => {
      if (mine === seq.current) setAnswer(a)
    })
  }, [])
  useEffect(load, [load])
  useEffect(() => {
    // Esc is inert in the list (FR-UI-53) — it is bound here to NOTHING on
    // purpose, so the binding lives in one place and its absence is visible.
    return undefined
  }, [])

  if (answer === null) return <State title="Consultando el almacén…" lines={[]} />
  if (answer.kind === 'unreadable') return <Unreadable detail={answer.detail} onRetry={load} />
  if (answer.kind === 'named') {
    // SharedNamed is CALLED, not rendered: an element is always truthy, so
    // `<SharedNamed/> ?? fallback` would never reach the fallback and an
    // unknown name would paint nothing at all — the silent degradation
    // FR-UI-37 forbids, arriving through a null check that reads fine.
    const shared = SharedNamed({ answer, onRetry: load, onGoHome })
    return shared ?? <UnknownName answer={answer} onRetry={load} />
  }

  const { gate, rows } = answer.value
  const skipped = gate.rows_skipped ?? 0
  if (rows.length === 0) {
    if (!gate.approvals_enabled) {
      // The gate said off while still answering 200: E3's text, never V1's.
      // Prohibited here is any assertion of emptiness — the switch only forks
      // the RECORDER, so requests parked before it was flipped are still
      // PENDING in the store and still decidable from the CLI.
      const off: Answer<never> = {
        kind: 'named',
        name: 'disabled',
        message: '',
        currentLaw: '',
        status: 409,
      }
      return SharedNamed({ answer: off, onRetry: load, onGoHome }) ?? <></>
    }
    if (gate.brains_can_park === 0) {
      return (
        <State
          title="APROBACIONES ENCENDIDAS · NINGÚN CEREBRO PUEDE APARCAR"
          lines={[
            'Nada puede llegar a esta lista',
            'Las aprobaciones están encendidas, pero ningún cerebro reúne las condiciones para aparcar. Falta al menos una de estas cinco: el almacén de acciones abierto, un cerebro agente, un techo de efecto en write_irreversible o critical, una herramienta de esa clase en su jaula, y esa herramienta permitida por la gobernanza. Con un techo por debajo la acción se deniega; sin techo se ejecuta al instante. Esto tampoco es una bandeja vacía.',
          ]}
        >
          <Actions>
            <ConfigFolderButton />
          </Actions>
        </State>
      )
    }
    return (
      <State
        title="No hay nada aparcado."
        lines={[
          `Las aprobaciones están encendidas y ${gate.brains_can_park} de ${gate.brains_total} cerebros pueden aparcar acciones irreversibles: si uno de ellos lo intenta, aparecerá aquí.`,
        ]}
      >
        <Actions>
          <button type="button" className="btn-secondary" onClick={load}>
            Actualizar
          </button>
        </Actions>
      </State>
    )
  }

  // Tail collision (FR-UI-12), over THIS page only — declared, not sold as a
  // global guarantee. Unreadable digests stay out of the detector.
  const tails = new Map<string, number>()
  for (const r of rows)
    if (isDigest(r.digest)) tails.set(tailOf(r.digest), (tails.get(tailOf(r.digest)) ?? 0) + 1)
  const collided = new Set([...tails].filter(([, n]) => n > 1).map(([t]) => t))

  return (
    <div className="approvals-list">
      {skipped > 0 && (
        <p role="alert">
          {skipped === 1
            ? '1 petición aparcada no se ha podido leer y no sale en esta lista. Está en el almacén: míralo con la CLI.'
            : `${String(skipped)} peticiones aparcadas no se han podido leer y no salen en esta lista. Están en el almacén: míralo con la CLI.`}
        </p>
      )}
      <Actions>
        <button type="button" className="btn-secondary" onClick={load}>
          Actualizar
        </button>
      </Actions>
      {collided.size > 0 && (
        <p role="status">Dos peticiones muestran la misma cola de digest; aquí van enteros</p>
      )}
      <ul className="approvals-rows">
        {rows.map((r) => {
          const banner = classBanner(r.effect_class)
          const whole = !isDigest(r.digest) || collided.has(tailOf(r.digest))
          if (!isApprovalID(r.id)) {
            // P2-1: listed, so the operator sees it exists; not a button, so it
            // is neither opened nor decided, and no URL is built from its id.
            return (
              <li key={r.id}>
                <div className="approvals-row">
                  <span className="approvals-row-op">{escapeUntrusted(r.operation)}</span>
                  <span className="approvals-row-class">{banner.label}</span>
                  <span className="approvals-row-origin">{escapeUntrusted(r.origin)}</span>
                  <span className="approvals-row-id">{escapeUntrusted(r.id)}</span>
                  <p role="status">{MALFORMED_ROW_ID}</p>
                </div>
              </li>
            )
          }
          return (
            <li key={r.id}>
              <button type="button" className="approvals-row" onClick={() => onOpen(r.id)}>
                <span className="approvals-row-op">{escapeUntrusted(r.operation)}</span>
                <span className="approvals-row-class">{banner.label}</span>
                <span className="approvals-row-origin">{escapeUntrusted(r.origin)}</span>
                <span className="approvals-row-id">{escapeUntrusted(r.id)}</span>
                <span className="approvals-row-digest">
                  {!isDigest(r.digest) ? (
                    <>
                      <span>digest ilegible</span>
                      <span>{escapeUntrusted(r.digest)}</span>
                    </>
                  ) : whole ? (
                    r.digest
                  ) : (
                    `${r.digest.slice(7, 15)}…${tailOf(r.digest)}`
                  )}
                </span>
                <RowExpiry value={r.expires_at} />
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

// --- P2 / P3 / P4 / P5 -----------------------------------------------------

/** What the decision produced. `null` while nothing has been sent. */
type Decision =
  | null
  | { kind: 'sending' }
  | { kind: 'executed'; digest: string; result: string; receipt: string }
  | { kind: 'failed'; digest: string; detail: string; receipt: string }
  | { kind: 'rejected'; receipt: string }
  | { kind: 'named'; name: string; message: string; currentLaw: string; status: number }
  | { kind: 'lost'; verb: 'approve' | 'reject' }
  | { kind: 'unrecognised' }

function RequestDetail({
  id,
  onBack,
  onGoHome,
}: {
  id: string
  onBack: () => void
  onGoHome: () => void
}): JSX.Element {
  const [answer, setAnswer] = useState<Answer<Detail> | null>(null)
  const [typed, setTyped] = useState('')
  const [pasteRefused, setPasteRefused] = useState(false)
  const [decision, setDecision] = useState<Decision>(null)
  const [comment, setComment] = useState('')
  const [now, setNow] = useState(() => Date.now())
  // One programmatic focus per load (G5a). The re-read's scroll to the top is
  // carried by the loading state, which replaces the document in .main — the
  // only scroll container of the view (G9, G10).
  const autofocused = useRef(false)

  const load = useCallback(() => {
    setAnswer(null)
    setTyped('')
    setPasteRefused(false)
    setDecision(null)
    autofocused.current = false
    // P2-1: the list never opens a malformed id; this refuses to build the URL
    // even if something else ever does.
    if (!isApprovalID(id)) {
      setAnswer({ kind: 'named', name: 'malformed id', message: '', currentLaw: '', status: 0 })
      return
    }
    void ask<Detail>(`/api/approvals/${id}`).then(setAnswer)
  }, [id])
  // Asked ONCE on opening; refreshing is [Volver a leer], which clears the
  // arming (FR-UI-14). No interval fetches here — AS-42 pins it.
  useEffect(load, [load])

  // The window's clock only withdraws Aprobar; it never asserts expiry. Who
  // judges that is the server, at the touch of the decision (FR-UI-29).
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])

  const detail = answer?.kind === 'ok' ? answer.value : null
  const authorityRead = readAuthority(detail?.authority)
  // G7: the stored expiry is judged by shape first. An illegible one withdraws
  // Aprobar exactly like a window clock past a legible one.
  const expiry = detail !== null && detail.expires_at !== '' ? parseExpiry(detail.expires_at) : null
  const expiryIllegible = detail !== null && detail.expires_at !== '' && expiry === null
  const clockSaysExpired = expiry !== null && now >= expiry.ms
  const armed = detail !== null && isDigest(detail.digest) && typed === tailOf(detail.digest)
  // The live document is painted: a 200 whose parameters the screen can show.
  // `present` with an empty body and an unknown state paint Unreadable (E9).
  const documentPainted =
    detail !== null &&
    authorityRead.ok &&
    (detail.parameters_state === 'present'
      ? detail.parameters !== ''
      : PARAMS_STATE_TEXT[detail.parameters_state] !== undefined)
  // FR-UI-15: an illegible digest offers no Aprobar and no arming row.
  const canApprove =
    documentPainted &&
    isDigest(detail.digest) &&
    detail.parameters_state === 'present' &&
    detail.brain_gone !== true &&
    !clockSaysExpired &&
    !expiryIllegible

  // G8: the arming dies with every presentational withdrawal. The typed tail is
  // cleared, not only hidden, so a clock going back shows an empty gate.
  useEffect(() => {
    if (!canApprove) setTyped('')
  }, [canApprove])

  // G7: a one-shot timer at the expiry instant, so the withdrawal does not wait
  // for the next tick. Its delay is clamped to setTimeout's range and the timer
  // is re-armed by the render it causes; the 1 s tick stays the net for a
  // wall-clock jump after arming.
  const expiryMs = expiry === null ? null : expiry.ms
  useEffect(() => {
    if (expiryMs === null) return undefined
    const left = expiryMs - Date.now()
    if (left <= 0) return undefined
    const t = setTimeout(() => setNow(Date.now()), Math.min(left, MAX_TIMER_MS))
    return () => clearTimeout(t)
  }, [expiryMs, now])

  // G5: the arming row fully in view moves focus to the gate, once per load, and
  // only when nobody holds the focus.
  const reachEnd = useCallback((input: HTMLInputElement) => {
    if (autofocused.current) return
    const active = document.activeElement
    if (active !== null && active !== document.body && !active.classList.contains('main')) return
    autofocused.current = true
    input.focus({ preventScroll: true })
  }, [])

  const send = useCallback(
    (verb: 'approve' | 'reject', body: string, sentDigest: string) => {
      if (!isApprovalID(id)) return
      setDecision({ kind: 'sending' })
      void ask<unknown>(`/api/approvals/${id}/${verb}`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body,
      }).then((a) => {
        if (a.kind === 'ok') {
          setDecision(
            judgeDecision(verb, a.status, a.value, sentDigest) ?? { kind: 'unrecognised' },
          )
          return
        }
        if (a.kind === 'unreadable') {
          setDecision({ kind: 'lost', verb })
          return
        }
        if (a.name === 'core unreachable' || a.name === 'core stopped') {
          setDecision({ kind: 'lost', verb })
          return
        }
        // `a` is narrowed to the named variant here, and that variant IS the
        // shape Decision's own 'named' carries: passing it whole lets the
        // compiler check the two stay equal instead of re-stating `kind`.
        setDecision(a)
      })
    },
    [id],
  )

  const reject = useCallback(() => send('reject', JSON.stringify({ comment }), ''), [send, comment])

  // Esc rejects while the request is open and undecided, typing included — and
  // is inert everywhere else (FR-UI-53), which is what gives it one meaning.
  // E9 (a 200 painted as unreadable) offers no decision, so Esc is inert there;
  // an Esc that belongs to an IME composition cancels the IME and decides nothing.
  const escActive = documentPainted && decision === null
  useEffect(() => {
    if (!escActive) return undefined
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape' && !e.isComposing && e.keyCode !== 229) reject()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [escActive, reject])

  if (answer === null) return <State title="Consultando el almacén…" lines={[]} />

  if (answer.kind === 'unreadable') {
    return (
      <>
        <BackBar onBack={onBack} />
        <Unreadable detail={answer.detail} onRetry={load} />
      </>
    )
  }

  if (answer.kind === 'named') {
    return (
      <>
        <BackBar onBack={onBack} />
        <ReadRefusal
          answer={answer}
          onRetry={load}
          onGoHome={onGoHome}
          onBack={onBack}
          onReject={reject}
        />
      </>
    )
  }

  if (!authorityRead.ok) {
    return (
      <>
        <BackBar onBack={onBack} />
        <Unreadable detail={authorityRead.detail} onRetry={load} />
      </>
    )
  }

  // Decision states replace the document once a decision has been sent.
  if (decision !== null && decision.kind !== 'sending') {
    return (
      <>
        <BackBar onBack={onBack} />
        <DecisionState
          decision={decision}
          detail={answer.value}
          onBack={onBack}
          onReload={load}
          onGoHome={onGoHome}
          onReject={reject}
        />
      </>
    )
  }
  const d = answer.value
  const authority = authorityRead.value
  const paramsText = PARAMS_STATE_TEXT[d.parameters_state]
  const paramsPresent = d.parameters_state === 'present'
  // `present` with an empty body is an unreadable answer, never an offer: a row
  // born without arguments is `empty`, which has its own literal (AS-37).
  if (paramsPresent && d.parameters === '') {
    return (
      <>
        <BackBar onBack={onBack} />
        <Unreadable detail="parameters_state present con cuerpo vacío" onRetry={load} />
      </>
    )
  }
  if (!paramsPresent && paramsText === undefined) {
    return (
      <>
        <BackBar onBack={onBack} />
        <Unreadable
          detail={`parameters_state ${escapeUntrusted(d.parameters_state)}`}
          onRetry={load}
        />
      </>
    )
  }

  const sending = decision !== null && decision.kind === 'sending'
  const banner = classBanner(d.effect_class)
  const groups = isDigest(d.digest) ? digestGroups(d.digest) : []

  return (
    <>
      <BackBar
        onBack={onBack}
        digest={d.digest}
        expires={
          d.expires_at === ''
            ? ''
            : expiry === null
              ? `${ILLEGIBLE_EXPIRY} ${escapeUntrusted(d.expires_at)}`
              : expiryLabel(expiry, now)
        }
      />
      {/* The live document gets its re-read too, not only the refusal states.
          An operator reading a request he no longer trusts had no way to ask
          for it again, and `load` is what clears the arming and puts the
          reader back at the top — a new document must never be handed over at
          an offset that meant something in the old one. */}
      {!sending && (
        <Actions>
          <button type="button" className="btn-secondary" onClick={load}>
            Volver a leer la petición
          </button>
        </Actions>
      )}
      <div className="approvals-doc">
        <article role="article">
          <section className="approvals-digest">
            <h2>EL DIGEST — EXACTAMENTE ESTO SE EJECUTARÁ</h2>
            {isDigest(d.digest) ? (
              <p className="approvals-digest-hex" aria-label={groups.join(' ')}>
                {groups.map((g, i) => (
                  <span key={`${g}-${String(i)}`}>{g} </span>
                ))}
              </p>
            ) : (
              <p>digest ilegible {escapeUntrusted(d.digest)}</p>
            )}
            <p>Cambia un solo carácter de lo que se ve abajo y este digest es otro digest</p>
            <p>✓ el almacén devolvió parámetros que re-derivan este digest</p>
          </section>

          <section>
            <h2>OPERACIÓN</h2>
            <p>{escapeUntrusted(d.operation)}</p>
            <p>{escapeUntrusted(d.origin)}</p>
          </section>

          <section>
            <h2>CLASE DE EFECTO</h2>
            <p>{banner.label}</p>
            <p>{banner.phrase}</p>
            <p>
              {d.reversibility === ''
                ? 'el registro no declara reversibilidad'
                : escapeUntrusted(d.reversibility)}
            </p>
          </section>

          <section>
            <h2>PARÁMETROS — LITERALES</h2>
            <p data-testid="approval-parameters">
              {paramsPresent ? escapeUntrusted(d.parameters) : paramsText}
            </p>
            {d.parameters_state === 'too_large' && (
              <p>korvun approvals show {escapeUntrusted(d.id)}</p>
            )}
          </section>

          <section>
            <h2>ORIGEN</h2>
            <p data-testid="approval-origin">{escapeUntrusted(d.purpose)}</p>
            <p>{escapeUntrusted(d.principal_id)}</p>
          </section>

          {authority !== undefined && (
            <section data-testid="approval-authority">
              <h2>AUTORIDAD</h2>
              <h3>QUIÉN PIDIÓ</h3>
              <p>{escapeUntrusted(authority.requester_principal_id)}</p>
              <h3>BAJO QUÉ CONTRATO</h3>
              <p>{escapeUntrusted(authority.intent_id)}</p>
              <p>{escapeUntrusted(authority.intent_purpose)}</p>
              <h3>CADENA</h3>
              <p>
                {authority.principal_chain.map((principal, index) => (
                  <Fragment key={`${principal}-${String(index)}`}>
                    {index > 0 && ' → '}
                    {escapeUntrusted(principal)}
                  </Fragment>
                ))}
              </p>
              <h3>PRESUPUESTO ANTES DE ESTE INTENTO</h3>
              <p>
                {authority.budget.kind === 'finite'
                  ? `máximo ${String(authority.budget.remaining)} inicios`
                  : 'máximo sin límite declarado'}
              </p>
            </section>
          )}

          <section>
            <h2>LA LEY QUE LO EXIGIÓ</h2>
            <p data-testid="approval-law">{escapeUntrusted(d.law_digest)}</p>
            <p>{escapeUntrusted(d.required_rule)}</p>
            <p>{escapeUntrusted(d.tool_cage)}</p>
          </section>

          <section>
            <h2>CADUCIDAD</h2>
            {d.expires_at === '' ? (
              <p>no caduca</p>
            ) : expiry === null ? (
              <>
                <p>{ILLEGIBLE_EXPIRY}</p>
                <p>{escapeUntrusted(d.expires_at)}</p>
              </>
            ) : (
              <>
                <p>{expiryLabel(expiry, now)}</p>
                <p data-testid="approval-expiry">{escapeUntrusted(d.expires_at)}</p>
              </>
            )}
          </section>

          <p>↓ la decisión está al final de la petición</p>

          <section className="approvals-decision">
            <h2>FIN DE LA PETICIÓN</h2>
            <p>
              Aprobar ejecuta {escapeUntrusted(d.operation)} exactamente como lo sella el digest de
              arriba. No hay deshacer y no hay compensación conocida.
            </p>

            {clockSaysExpired && (
              <p role="status">
                El reloj de esta ventana dice que esta petición ya ha caducado. Quien lo juzga es el
                servidor, en el toque de la decisión.
              </p>
            )}

            {sending && <p role="status">Ejecutando la acción aprobada. No cierres la ventana.</p>}

            {/* FR-UI-57 (director, decision 3 of 2026-09-13): the arming is a
                full-width row ABOVE the reason field, as plates 03/03b draw it, so
                Tab walks document → arming → reason → Rechazar → Aprobar. */}
            {canApprove && (
              <ArmingField
                typed={typed}
                target={isDigest(d.digest) ? tailOf(d.digest) : ''}
                prefix={isDigest(d.digest) ? d.digest.slice(7).slice(48, 58) : ''}
                pasteRefused={pasteRefused}
                onKey={(ch) => setTyped((t) => (t.length >= 6 ? t : t + ch))}
                onBackspace={() => setTyped((t) => t.slice(0, -1))}
                onPaste={() => {
                  setPasteRefused(true)
                  setTyped('')
                }}
                onReachEnd={reachEnd}
              />
            )}

            <label htmlFor="approvals-comment">Motivo del rechazo (opcional)</label>
            <input
              id="approvals-comment"
              className="approvals-reason"
              type="text"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
            />

            <div className="approvals-actions approvals-doors">
              <button
                type="button"
                className="approvals-reject"
                disabled={sending}
                onClick={reject}
              >
                Rechazar
              </button>
              {canApprove && (
                <button
                  type="button"
                  className="approvals-approve"
                  disabled={!armed || sending}
                  onClick={() => send('approve', JSON.stringify({ digest: d.digest }), d.digest)}
                >
                  Aprobar y ejecutar
                </button>
              )}
            </div>
            <p>{ESC_LINE}</p>
          </section>
        </article>
      </div>
    </>
  )
}

function BackBar({
  onBack,
  digest,
  expires,
}: {
  onBack: () => void
  digest?: string
  expires?: string
}): JSX.Element {
  return (
    <div className="approvals-bar">
      <button type="button" className="btn-secondary" onClick={onBack}>
        ← Pendientes
      </button>
      {digest !== undefined && isDigest(digest) && (
        <span>
          {digest.slice(7, 15)}…{tailOf(digest)} · fijado mientras decides
        </span>
      )}
      {expires !== undefined && expires !== '' && (
        <span className="approvals-bar-expiry">{expires}</span>
      )}
    </div>
  )
}

/** The arming gate: one `<input maxlength=6>` drawn as six cells (FR-UI-39).
 * Typing arms it; pasting, dropping and autofill do not, nor does a repeated
 * key or a keydown reported with keyCode 229. Six characters are not a
 * cryptographic proof and this does not sell them as one: what they prove is
 * that whoever approves looked at THIS digest. */
function ArmingField({
  typed,
  target,
  prefix,
  pasteRefused,
  onKey,
  onBackspace,
  onPaste,
  onReachEnd,
}: {
  typed: string
  target: string
  prefix: string
  pasteRefused: boolean
  onKey: (ch: string) => void
  onBackspace: () => void
  onPaste: () => void
  onReachEnd: (input: HTMLInputElement) => void
}): JSX.Element {
  const row = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const el = row.current
    const field = input.current
    if (el === null || field === null || typeof IntersectionObserver === 'undefined') {
      return undefined
    }
    const io = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) if (entry.intersectionRatio >= 1) onReachEnd(field)
      },
      { root: el.closest('.main'), threshold: 1 },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [onReachEnd])
  const status =
    typed.length < 6
      ? `faltan ${String(6 - typed.length)}`
      : typed === target
        ? '✓ coincide'
        : 'no coincide'
  return (
    <div className="approvals-arming" data-testid="arming-row" ref={row}>
      <label htmlFor="approvals-arming-input">
        Para armar Aprobar, reteclea los seis últimos caracteres del digest
      </label>
      <div className="approvals-arming-line">
        <span className="approvals-arming-prefix" data-testid="arming-prefix">
          {`…${prefix.slice(0, 8)} ${prefix.slice(8)}`}
        </span>
        <div className="approvals-arming-cells" data-testid="arming-cells">
          <input
            ref={input}
            id="approvals-arming-input"
            className="approvals-arming-input"
            type="text"
            maxLength={6}
            value={typed}
            autoComplete="off"
            spellCheck={false}
            aria-describedby="approvals-arming-status"
            onKeyDown={(e) => {
              if (e.key === 'Backspace') {
                onBackspace()
                return
              }
              if (e.key.length !== 1) return
              // G5e: a repeat or a keyCode 229 does not count as a keystroke.
              if (e.repeat || e.keyCode === 229 || e.nativeEvent.isComposing) return
              const ch = e.key.toLowerCase()
              // Alphabet [0-9a-f]; uppercase folds down; everything else is
              // ignored WITHOUT painting an error (FR-UI-40).
              if (/^[0-9a-f]$/.test(ch)) onKey(ch)
            }}
            onChange={() => undefined}
            onPaste={(e) => {
              e.preventDefault()
              onPaste()
            }}
            onDrop={(e) => {
              e.preventDefault()
              onPaste()
            }}
          />
          {Array.from({ length: 6 }, (_, i) => (
            <span
              key={i}
              className={
                i < typed.length ? 'approvals-arming-cell is-typed' : 'approvals-arming-cell'
              }
              data-testid="arming-cell"
            >
              {i < typed.length ? typed[i] : '–'}
            </span>
          ))}
        </div>
        <span id="approvals-arming-status" className="approvals-arming-status" aria-live="polite">
          {status}
        </span>
      </div>
      {pasteRefused && <p>Pegar no arma: teclea los seis caracteres</p>}
    </div>
  )
}

/** The refusals that arrive from a READ of the detail. E7 and E9-bis keep
 * Rechazar; E8 does not, because the rejection runs the same belts inside its
 * own transaction and the button would die with the same name. */
function ReadRefusal({
  answer,
  onRetry,
  onGoHome,
  onBack,
  onReject,
}: NamedProps & { onReject: () => void }): JSX.Element {
  const shared = SharedNamed({ answer, onRetry, onGoHome })
  if (shared !== null) return shared
  const back = (
    <button type="button" className="btn-secondary" onClick={onBack}>
      Volver a pendientes
    </button>
  )
  switch (answer.name) {
    // A synthetic name, spelled with a space like 'core stopped', so no name
    // the server registers can collide with it.
    case 'malformed id':
      return (
        <State title={MALFORMED_ROW_ID} lines={[]}>
          <Actions>{back}</Actions>
        </State>
      )
    case 'expired':
      return (
        <State
          title="Esta petición caducó y ya no se puede decidir"
          // The READ door carries no stored instant, and no digest to declare
          // no longer actionable: only what is known is said.
          lines={['La acción aparcada se cierra con su recibo; no se ha ejecutado nada.']}
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'already_decided':
      return (
        <State
          title="Esta petición ya no está esperando decisión, y esta ventana no la ha decidido."
          lines={[
            'Puede haberla decidido otro operador, o el almacén guardar para ella un estado que esta ventana no puede juzgar; desde aquí no se distingue. Mira el libro.',
          ]}
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'not_found':
      return (
        <State title="No hay ninguna petición con ese identificador." lines={[]}>
          <Actions>{back}</Actions>
        </State>
      )
    case 'params_digest_mismatch':
      return (
        <State
          title="Los parámetros guardados no reproducen el digest de esta petición"
          lines={[
            'Esta ventana no te enseña un documento que no cuadra consigo mismo. No se ha ejecutado nada y no se ofrece ninguna decisión.',
            PERMANENT_LINE,
          ]}
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'invalidated':
      return (
        <State
          title="La ley bajo la que se aparcó esta petición ya no existe"
          lines={[
            'No se ha ejecutado nada, y esto NO es transitorio: mientras el perfil siga como está, esta petición no se puede aprobar nunca.',
            answer.currentLaw === '' ? 'Ahora rige otra ley.' : `Ahora rige ${answer.currentLaw}`,
            'Rechazarla sí funciona: retirar autoridad es seguro bajo cualquier ley.',
          ]}
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onReject}>
              Rechazar esta petición
            </button>
            {back}
          </Actions>
        </State>
      )
    case 'evidence_corrupt':
      return (
        <State
          title="La evidencia de esta petición no cuadra consigo misma"
          lines={[
            `El almacén guarda esta petición, pero su historia no verifica: ${escapeUntrusted(answer.message)}. No se ha leído nada como bueno y no se ofrece ninguna decisión.`,
            PERMANENT_LINE,
          ]}
        >
          <Actions>{back}</Actions>
        </State>
      )
    default:
      return <UnknownName answer={answer} onRetry={onRetry} />
  }
}

/** After a decision that is already committed. The screen never offers another
 * decision here — what changes is what is KNOWN about the effect. */
function DecisionState({
  decision,
  detail,
  onBack,
  onReload,
  onGoHome,
  onReject,
}: {
  decision: Exclude<Decision, null | { kind: 'sending' }>
  detail: Detail
  onBack: () => void
  onReload: () => void
  onGoHome: () => void
  onReject: () => void
}): JSX.Element {
  const back = (
    <button type="button" className="btn-secondary" onClick={onBack}>
      Volver a pendientes
    </button>
  )
  if (decision.kind === 'executed') {
    return (
      <State
        title="Ejecutada"
        lines={[
          decision.digest,
          escapeUntrusted(decision.result),
          `Recibo ${escapeUntrusted(decision.receipt)}`,
        ]}
        alert
      >
        <Actions>{back}</Actions>
      </State>
    )
  }
  if (decision.kind === 'failed') {
    return (
      <State
        title="El intento falló"
        lines={[
          decision.digest,
          'La ejecución se intentó y no salió bien. El registro se cerró con su recibo, así que esto no es una duda sobre la DECISIÓN.',
          'La herramienta se negó antes de que nada saliera, o falló sin llegar a entregar nada. Cuando el binario NO puede afirmar eso, esta pantalla no dice «falló»: dice que no se sabe. Mira el libro antes de repetir nada.',
          escapeUntrusted(decision.detail),
          `Recibo ${escapeUntrusted(decision.receipt)}`,
        ]}
        alert
      >
        <Actions>{back}</Actions>
      </State>
    )
  }
  if (decision.kind === 'rejected') {
    return (
      <State
        title="Rechazada. La acción aparcada se cierra con su recibo sellado."
        lines={[`Recibo ${escapeUntrusted(decision.receipt)}`]}
        alert
      >
        <Actions>{back}</Actions>
      </State>
    )
  }
  if (decision.kind === 'unrecognised') {
    return (
      <State title={UNRECOGNISED_SUCCESS} lines={[]} alert>
        <Actions>{back}</Actions>
      </State>
    )
  }
  if (decision.kind === 'lost') {
    return (
      <State
        title={
          decision.verb === 'reject'
            ? 'El rechazo salió de esta ventana y no hemos recibido su desenlace.'
            : 'La decisión salió de esta ventana. No sabemos si el efecto llegó a ocurrir.'
        }
        lines={[]}
        alert
      >
        <Actions>{back}</Actions>
      </State>
    )
  }

  // A named refusal of a POST.
  const outcome = OUTCOME_TEXT[decision.name]
  if (outcome !== undefined) {
    return (
      <State title={outcome} lines={[escapeUntrusted(decision.message)]} alert>
        <Actions>{back}</Actions>
      </State>
    )
  }
  switch (decision.name) {
    // P2-10: the registered names a POST can carry, each under its own literal
    // (UX v36 for not_found, unavailable and disabled; the 2026-09-19 approved
    // literals for params_digest_mismatch and loopback_only). None affirms an
    // outcome.
    case 'not_found':
      return (
        <State title="No hay ninguna petición con ese identificador." lines={[]} alert>
          <Actions>{back}</Actions>
        </State>
      )
    case 'unavailable':
      return (
        <State
          title="El almacén no se pudo leer en este instante. Es transitorio y no dice nada sobre la evidencia."
          lines={[]}
          alert
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'disabled':
      return (
        <State title="APROBACIONES APAGADAS EN ESTE PERFIL" lines={DISABLED_LINES} alert>
          <Actions>
            <ConfigFolderButton />
            {back}
          </Actions>
        </State>
      )
    case 'params_digest_mismatch':
      return (
        <State title={POST_PARAMS_MISMATCH} lines={[]} alert>
          <Actions>
            <button type="button" className="btn-secondary" onClick={onReload}>
              Volver a leer la petición
            </button>
            {back}
          </Actions>
        </State>
      )
    case 'loopback_only':
      return (
        <State title={LOOPBACK_ONLY} lines={[]} alert>
          <Actions>{back}</Actions>
        </State>
      )
    case 'digest_mismatch':
      return (
        <State
          title="La petición cambió entre que la leíste y que la aprobaste"
          lines={[
            'No se ha ejecutado nada y nada se ha consumido.',
            `Aprobaste ${detail.digest}`,
            'El servidor comparó ese digest con el de la petición guardada y no coinciden, así que paró antes de decidir. Eso es exactamente lo que tiene que pasar.',
            'Lo que hay que hacer: leerla otra vez, entera, desde el principio.',
          ]}
          alert
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onReload}>
              Volver a leer la petición
            </button>
            {back}
          </Actions>
        </State>
      )
    case 'expired':
      return (
        <State
          title="Esta petición caducó y ya no se puede decidir"
          lines={[
            parseExpiry(detail.expires_at) === null
              ? `Caducó en un instante que esta ventana no puede leer: ${ILLEGIBLE_EXPIRY} ${escapeUntrusted(detail.expires_at)}. La acción aparcada se cierra con su recibo; no se ha ejecutado nada.`
              : `Caducó a las ${detail.expires_at}. La acción aparcada se cierra con su recibo; no se ha ejecutado nada.`,
            'DIGEST — YA NO ACCIONABLE',
          ]}
          alert
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'brain_gone':
      return (
        <State
          title="El cerebro que pidió esta acción ya no está en el perfil"
          lines={[
            'No se ha decidido nada. Aprobar necesita la ley de ese cerebro y esa ley ya no existe; rechazar no la necesita, así que sigue disponible.',
          ]}
          alert
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onReject}>
              Rechazar esta petición
            </button>
            {back}
          </Actions>
        </State>
      )
    case 'invalidated':
      return (
        <State
          title="La ley bajo la que se aparcó esta petición ya no existe"
          lines={[
            'No se ha ejecutado nada, y esto NO es transitorio: mientras el perfil siga como está, esta petición no se puede aprobar nunca.',
            decision.currentLaw === ''
              ? 'Ahora rige otra ley.'
              : `Ahora rige ${decision.currentLaw}`,
            'Rechazarla sí funciona: retirar autoridad es seguro bajo cualquier ley.',
          ]}
          alert
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onReject}>
              Rechazar esta petición
            </button>
            {back}
          </Actions>
        </State>
      )
    case 'already_decided':
      return (
        <State
          title="Esta petición ya no está esperando decisión, y esta ventana no la ha decidido."
          lines={[
            'Puede haberla decidido otro operador, o el almacén guardar para ella un estado que esta ventana no puede juzgar; desde aquí no se distingue. Mira el libro.',
          ]}
          alert
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'evidence_corrupt':
      return (
        <State
          title="La evidencia de esta petición no cuadra consigo misma"
          lines={[
            `El almacén guarda esta petición, pero su historia no verifica: ${escapeUntrusted(decision.message)}. No se ha leído nada como bueno y no se ofrece ninguna decisión.`,
            PERMANENT_LINE,
          ]}
          alert
        >
          <Actions>{back}</Actions>
        </State>
      )
    case 'forbidden':
      return (
        <State
          title="La ventana no ha podido autenticarse contra el núcleo. No ha salido ninguna decisión."
          lines={[]}
          alert
        >
          <Actions>
            <button type="button" className="btn-secondary" onClick={onGoHome}>
              Ir a Inicio
            </button>
            {back}
          </Actions>
        </State>
      )
    default:
      // An unknown name in a POST inherits the POST rule: unknown outcome and
      // NO [Reintentar] — retry repeats the last GET and never a POST.
      return (
        <State
          title={UNKNOWN_NAME_TITLE}
          lines={[
            `${decision.status}`,
            escapeUntrusted(decision.name),
            escapeUntrusted(decision.message),
          ]}
          alert
        >
          <Actions>{back}</Actions>
        </State>
      )
  }
}
