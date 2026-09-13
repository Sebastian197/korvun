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
import { useCallback, useEffect, useRef, useState } from 'react'
import type { JSX } from 'react'
import { desktop } from '../lib/go'
import {
  ESC_LINE,
  OUTCOME_TEXT,
  PARAMS_STATE_TEXT,
  PERMANENT_LINE,
  SURFACE_NOT_MOUNTED,
  UNKNOWN_NAME_TITLE,
  UNREADABLE_TITLE,
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
  | { kind: 'ok'; value: T }
  | { kind: 'named'; name: string; message: string; currentLaw: string; status: number }
  | { kind: 'unreadable'; detail: string }

// ---------------------------------------------------------------------------
// Rendering of untrusted bytes (§6.2)
// ---------------------------------------------------------------------------

/** Controls, invisibles and bidi marks become visible escapes. Without this a
 * U+202E lets the document read "hooks.acme.io" while the digest seals
 * something else: the page would say one thing and the digest seal another.
 * Applied to EVERY field of uncontrolled origin, not to one block. */
function escapeUntrusted(s: string): string {
  let out = ''
  for (const ch of s) {
    const c = ch.codePointAt(0) ?? 0
    const invisible =
      c < 0x20 ||
      c === 0x7f ||
      c === 0x200b ||
      c === 0x200c ||
      c === 0x200d ||
      c === 0xfeff ||
      (c >= 0x202a && c <= 0x202e) ||
      (c >= 0x2066 && c <= 0x2069)
    out += invisible ? `<U+${c.toString(16).toUpperCase().padStart(4, '0')}>` : ch
  }
  return out
}

/** sha256: + 64 lowercase hex, the shape action.Digest produces. Anything else
 * is "digest ilegible": it is not printed as a digest, not shortened, and not
 * fed to the tail-collision detector. */
const DIGEST_RE = /^sha256:[0-9a-f]{64}$/
function isDigest(d: string): boolean {
  return DIGEST_RE.test(d)
}
const tailOf = (d: string): string => d.slice(-6)

/** The 64 hex in eight groups of eight, the ONE grouping of the document. */
function digestGroups(d: string): string[] {
  const hex = d.slice('sha256:'.length)
  return Array.from({ length: 8 }, (_, i) => hex.slice(i * 8, i * 8 + 8))
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
  return { kind: 'ok', value: body as T }
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
        <State
          title="APROBACIONES APAGADAS EN ESTE PERFIL"
          lines={[
            'Con las aprobaciones apagadas ya no se retiene nada nuevo',
            'Una acción irreversible se ejecuta ahora en el momento en que el agente la llama. Esto no es una bandeja vacía: es un hueco en la garantía.',
            'Y lo que se aparcó ANTES de apagar el interruptor sigue vivo en el almacén: esta pantalla no puede enseñártelo con las aprobaciones apagadas, y solo se decide desde la CLI hasta que el barrendero lo cierre.',
            'Se enciende en el perfil: approvals.enabled',
          ]}
        >
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
          return (
            <li key={r.id}>
              <button type="button" className="approvals-row" onClick={() => onOpen(r.id)}>
                <span className="approvals-row-op">{escapeUntrusted(r.operation)}</span>
                <span className="approvals-row-class">{banner.label}</span>
                <span className="approvals-row-origin">{escapeUntrusted(r.origin)}</span>
                <span className="approvals-row-id">{r.id}</span>
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
                <span className="approvals-row-expiry">
                  {r.expires_at === '' ? 'no caduca' : r.expires_at}
                </span>
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
  const scroller = useRef<HTMLDivElement>(null)

  const load = useCallback(() => {
    setAnswer(null)
    setTyped('')
    setPasteRefused(false)
    setDecision(null)
    if (scroller.current !== null) scroller.current.scrollTop = 0
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
  const expiresAt =
    detail !== null && detail.expires_at !== '' ? Date.parse(detail.expires_at) : NaN
  const clockSaysExpired = !Number.isNaN(expiresAt) && now >= expiresAt
  const armed = detail !== null && isDigest(detail.digest) && typed === tailOf(detail.digest)

  const send = useCallback(
    (verb: 'approve' | 'reject', body: string) => {
      setDecision({ kind: 'sending' })
      void ask<{ outcome: string; digest?: string; result?: string; receipt_id: string }>(
        `/api/approvals/${id}/${verb}`,
        { method: 'POST', headers: { 'content-type': 'application/json' }, body },
      ).then((a) => {
        if (a.kind === 'ok') {
          setDecision(
            verb === 'approve'
              ? a.value.outcome === 'failed'
                ? {
                    // The tool ran and said no. It is a KNOWN outcome with its
                    // receipt: calling it executed would be a lie, and calling
                    // it unknown would be a second one.
                    kind: 'failed',
                    digest: a.value.digest ?? '',
                    detail: a.value.result ?? '',
                    receipt: a.value.receipt_id,
                  }
                : {
                    kind: 'executed',
                    digest: a.value.digest ?? '',
                    result: a.value.result ?? '',
                    receipt: a.value.receipt_id,
                  }
              : { kind: 'rejected', receipt: a.value.receipt_id },
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

  const reject = useCallback(() => send('reject', JSON.stringify({ comment })), [send, comment])

  // Esc rejects while the request is open and undecided, typing included — and
  // is inert everywhere else (FR-UI-53), which is what gives it one meaning.
  const escActive = detail !== null && decision === null
  useEffect(() => {
    if (!escActive) return undefined
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') reject()
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
  const canApprove = paramsPresent && d.brain_gone !== true && !clockSaysExpired
  const groups = isDigest(d.digest) ? digestGroups(d.digest) : []

  return (
    <>
      <BackBar onBack={onBack} digest={d.digest} expires={d.expires_at} />
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
      <div className="approvals-doc" ref={scroller}>
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
            {d.parameters_state === 'too_large' && <p>korvun approvals show {d.id}</p>}
          </section>

          <section>
            <h2>ORIGEN</h2>
            <p data-testid="approval-origin">{escapeUntrusted(d.purpose)}</p>
            <p>{escapeUntrusted(d.principal_id)}</p>
          </section>

          <section>
            <h2>LA LEY QUE LO EXIGIÓ</h2>
            <p>{d.law_digest}</p>
            <p>{escapeUntrusted(d.required_rule)}</p>
            <p>{escapeUntrusted(d.tool_cage)}</p>
          </section>

          <section>
            <h2>CADUCIDAD</h2>
            <p>{d.expires_at === '' ? 'no caduca' : d.expires_at}</p>
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

            <label htmlFor="approvals-comment">Motivo del rechazo (opcional)</label>
            <input
              id="approvals-comment"
              type="text"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
            />

            <Actions>
              <button
                type="button"
                className="approvals-reject"
                disabled={sending}
                onClick={reject}
              >
                Rechazar
              </button>
              {/* The arming gate sits BETWEEN the two doors, in the DOM and on
                  the screen. That is what puts 320 px between them without a
                  stretch of dead space, and it is why Tab from the reason
                  reaches Rechazar, then the gate, then Aprobar — the hand has
                  to cross the gate to get to the expensive control. */}
              {canApprove && (
                <ArmingField
                  typed={typed}
                  target={isDigest(d.digest) ? tailOf(d.digest) : ''}
                  pasteRefused={pasteRefused}
                  onKey={(ch) => setTyped((t) => (t.length >= 6 ? t : t + ch))}
                  onBackspace={() => setTyped((t) => t.slice(0, -1))}
                  onPaste={() => {
                    setPasteRefused(true)
                    setTyped('')
                  }}
                />
              )}
              {canApprove && (
                <button
                  type="button"
                  className="approvals-approve"
                  disabled={!armed || sending}
                  onClick={() => send('approve', JSON.stringify({ digest: d.digest }))}
                >
                  Aprobar y ejecutar
                </button>
              )}
            </Actions>
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
      {expires !== undefined && expires !== '' && <span>{expires}</span>}
    </div>
  )
}

/** One input drawn as six cells. Typing arms it; pasting, dropping and
 * autofill do not — six characters are not a cryptographic proof and this
 * never sells them as one: what they prove is that whoever approves looked at
 * THIS digest. */
function ArmingField({
  typed,
  target,
  pasteRefused,
  onKey,
  onBackspace,
  onPaste,
}: {
  typed: string
  target: string
  pasteRefused: boolean
  onKey: (ch: string) => void
  onBackspace: () => void
  onPaste: () => void
}): JSX.Element {
  const status =
    typed.length < 6
      ? `faltan ${String(6 - typed.length)}`
      : typed === target
        ? '✓ coincide'
        : 'no coincide'
  return (
    <div className="approvals-arming">
      <label htmlFor="approvals-arming-input">
        Para armar Aprobar, reteclea los seis últimos caracteres del digest
      </label>
      <input
        id="approvals-arming-input"
        type="text"
        maxLength={6}
        value={typed}
        aria-describedby="approvals-arming-status"
        onKeyDown={(e) => {
          if (e.key === 'Backspace') {
            onBackspace()
            return
          }
          if (e.key.length !== 1) return
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
      <span id="approvals-arming-status" aria-live="polite">
        {status}
      </span>
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
    case 'expired':
      return (
        <State
          title="Esta petición caducó y ya no se puede decidir"
          lines={[
            `Caducó a las ${escapeUntrusted(answer.message)}. La acción aparcada se cierra con su recibo; no se ha ejecutado nada.`,
            'DIGEST — YA NO ACCIONABLE',
          ]}
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
        lines={[detail.digest, escapeUntrusted(decision.result), `Recibo ${decision.receipt}`]}
        alert
      >
        <Actions>{back}</Actions>
      </State>
    )
  }
  if (decision.kind === 'failed') {
    return (
      <State
        title="La acción se ejecutó y falló"
        lines={[
          detail.digest,
          'La ejecución se intentó y no salió bien. El registro se cerró con su recibo, así que esto no es una duda sobre la DECISIÓN.',
          'Lo que esta pantalla no puede decirte es si el efecto llegó a salir: la herramienta puede haber rechazado sus argumentos sin tocar nada, o haber expirado con la llamada ya entregada. Mira el libro antes de repetir nada.',
          escapeUntrusted(decision.detail),
          `Recibo ${decision.receipt}`,
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
        lines={[`Recibo ${decision.receipt}`]}
        alert
      >
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
            `Caducó a las ${detail.expires_at}. La acción aparcada se cierra con su recibo; no se ha ejecutado nada.`,
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
