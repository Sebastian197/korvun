// «¿Qué pasa hoy?» (v0.16.2) — what decides the fate of an irreversible action
// in THIS profile, and the four things the operator can change from here.
//
// The seventh law (CLAUDE.md, 2026-09-23): no core capability is finished if the
// operator cannot see and understand it in the app. Korvun's approval machinery
// has eleven-odd dimensions that decide whether a webhook parks, executes or is
// refused, and until this screen every one of them was a key in a JSON file.
//
// It reads TWO doors and joins them:
//   · GET /api/whats-happening — the contract: every dimension the CODE can
//     decide by, each with where it lives in the profile, and the join from the
//     park gate's condition names to those rows. The join travels from the
//     server so this file never holds a second copy of it.
//   · GET /api/approvals — the gate over THIS profile: which brains can park and,
//     when they cannot, every condition each one fails.
//
// A 404 on the second is not an error: the approvals surface mounts only where
// an action store is open, so its absence IS the first blocker, and the screen
// says so in the contract's own words rather than painting a failure.
import { useCallback, useEffect, useState } from 'react'
import { IconWarning } from '../components/icons'

/** One dimension of the contract, as the read door serves it. */
interface Row {
  rule: string
  label: string
  profile_key: string
  has_button: boolean
  door?: string
  why?: string
  after_approval?: boolean
}

interface Contract {
  rows: Row[]
  /** park-gate condition name -> contract rule name. */
  conditions: Record<string, string>
  /** What the action ledger is to THIS profile (the durable mark): `ok`,
   * `legacy_unfounded` (no mark: older than this version) or
   * `ledger_foreign_profile` (founded or adopted by another profile — every
   * change is refused until this one adopts it); and, when the core could
   * not judge it, `unreadable` (a verdict on the book), `environment` (the
   * storage around it failed) or `unavailable` (it could not be checked now),
   * with the cause in `owner`. `path` is the file the core opened, when the
   * core says it. Absent when there is no ledger, or the core was started
   * without a profile path. */
  ledger?: { standing: string; owner: string; path?: string }
}

interface ParkBlock {
  brain: string
  failed: string[]
  /** The tool the reported condition is ABOUT, when it is about one. Empty
   * means «this condition names no tool», not «no tool» — a button aimed at an
   * empty name is a button aimed at nothing, so the row draws none. */
  tool?: string
}

/** One host allow-list in the profile: the cage that decides AFTER the operator
 * has said yes. Read from the config door the builder already uses, because the
 * park gate does not report cages — they do not stop a request from parking,
 * they refuse it once approved. */
interface Cage {
  brain: string
  tool: string
  hosts: string[]
}

/** The shape of the config this screen reads, narrowed to the cages. Everything
 * else in the document is none of this screen's business. */
interface ConfigShape {
  brains?: {
    name: string
    agent?: {
      webhook_call?: { allow_hosts?: string[] }
      http_fetch?: { allow_hosts?: string[] }
    }
  }[]
}

function cagesOf(cfg: ConfigShape): Cage[] {
  const out: Cage[] = []
  for (const b of cfg.brains ?? []) {
    if (b.agent === undefined) continue
    if (b.agent.webhook_call !== undefined)
      out.push({
        brain: b.name,
        tool: 'webhook_call',
        hosts: b.agent.webhook_call.allow_hosts ?? [],
      })
    if (b.agent.http_fetch !== undefined)
      out.push({ brain: b.name, tool: 'http_fetch', hosts: b.agent.http_fetch.allow_hosts ?? [] })
  }
  return out
}

interface Gate {
  approvals_enabled: boolean
  brains_total: number
  brains_can_park: number
  blocked?: ParkBlock[]
}

/** What a button's POST answered. Every outcome the door can name has its own
 * sentence: a screen that collapsed them would tell an operator «listo» over a
 * cutover that rolled back. */
const OUTCOME_ES: Record<string, string> = {
  applying: 'Aplicando… tu perfil en disco todavía no ha cambiado.',
  applied: 'Hecho. El cambio está en marcha y guardado en tu perfil.',
  not_applied: 'No se aplicó. Tu perfil sigue exactamente como estaba.',
  applied_not_saved:
    'El cambio está en marcha pero NO se pudo guardar: al reiniciar Korvun volverá atrás.',
  another_change_in_flight: 'Ya se está aplicando otro cambio. Vuelve a pulsar cuando termine.',
  would_self_lock:
    'Este cambio dejaría a Korvun sin su token de administración: esta pantalla se quedaría sin botones.',
  needs_confirmation: 'Esta acción abre ejecución real y necesita tu confirmación.',
  no_ledger:
    'No se aplicó nada: este perfil no tiene almacén de acciones, así que el cambio no podría quedar registrado en el libro.',
  act_not_recorded:
    'No se intentó el cambio: el acto del operador no se pudo registrar en el libro, y sin acto no hay cambio.',
  refused: 'No se hizo el cambio.',
  ledger_exists:
    'No se activó: ya hay un libro en la ruta por defecto que este perfil no nombra, y esta pantalla solo adopta un libro que ella misma haya creado en esta sesión.',
  ledger_not_created: 'No se activó: no se pudo crear el libro.',
  ledger_foreign_profile:
    'No se aplicó nada: el libro de acciones lo fundó o adoptó otro perfil. Pulsa «Adoptar libro» en esta pantalla para que este perfil se quede con él.',
  adopted: 'Hecho. Este perfil ha adoptado el libro; queda como acto con su recibo.',
}

/** The two doors that open REAL execution, and the sentence each one asks the
 * operator to agree to. The confirmation is one step, as the director ruled:
 * one sentence, one «sí», no dialog to dismiss. */
const CONFIRMATIONS: Record<string, string> = {
  'lift-shadow': 'Sí, que pueda ejecutarse tras mi aprobación',
  'allow-host': 'Sí, que pueda hablar con este host tras mi aprobación',
  'adopt-ledger': 'Sí, este perfil se queda con este libro',
}

const BUTTON_ES: Record<string, string> = {
  'enable-storage': 'Activar almacén',
  'adopt-ledger': 'Adoptar libro',
  'enable-approvals': 'Encender aprobaciones',
  'set-ceiling': 'Poner el techo',
  'lift-shadow': 'Levantar la sombra',
  'allow-host': 'Añadir host',
}

/** D2 (plan §7), verbatim, with the sentence about the shared file before its
 * replacement instruction. Without the ledger's path, the two sentences that
 * need it are left out rather than guessed. */
function d2(cause: string, path: string | undefined): string {
  const head = `El libro no se puede leer: ${cause}. Korvun no lo repara ni lo recrea. Este fichero contiene el libro de actos y las conversaciones de este perfil.`
  if (path === undefined || path === '') return head
  return `${head} Detén Korvun y sustituye ${path} por una copia tomada antes del fallo; si tu copia es un solo fichero, borra también ${path}-wal y ${path}-shm. Si no tienes copia, aparta esos ficheros y Korvun empezará un libro nuevo, sin el historial de este perfil.`
}

/** D3 for the environment, line by line; its third line needs the path and is
 * left out without it. */
function d3Environment(cause: string, path: string | undefined): string[] {
  const lines = [
    'El libro no se puede usar en esta máquina.',
    `El fichero puede estar bien; lo que falla es el disco o los permisos: ${cause}.`,
  ]
  if (path !== undefined && path !== '')
    lines.push(`Libera espacio o revisa los permisos de ${path} y vuelve a abrir Korvun.`)
  lines.push('Mientras tanto, los botones de esta pantalla no registran nada.')
  return lines
}

/** D3 of the moment (plan §7), verbatim. */
function d3Moment(cause: string): string {
  return `No se pudo comprobar el libro ahora. Otro proceso lo está usando o el sistema no respondió: ${cause}. Vuelve a abrir esta pantalla en un momento; si persiste, cierra la otra ventana o proceso de Korvun.`
}

type Loaded =
  | { kind: 'loading' }
  | { kind: 'no-door' }
  | { kind: 'unreadable'; detail: string }
  | {
      kind: 'ready'
      contract: Contract
      /** The gate, or the failure that stopped it being read. */
      gate: Gate | null
      gateFailure: ReadFailure | null
      cages: Cage[]
      /** The failure that stopped the cages being read, when one did. */
      cageFailure: ReadFailure | null
    }

/** A door's answer while it is being pressed, kept per door so two buttons never
 * share one message. */
interface Pressed {
  door: string
  outcome: string
  detail: string
  /** The operator act this change was recorded as. The seventh law applies to the
   * act itself: a change that is written into the book and never shown is a
   * capability the operator cannot see. */
  actionId: string
  receiptId: string
}

/** A read that failed, with everything the core said about it.
 *
 * `name` is the outcome the core NAMED (`evidence_corrupt`, `unavailable`,
 * `disabled`…), empty when the body carried none. It exists because the first
 * version of this file threw the whole failure away and kept only «no gate»,
 * which the screen then rendered as «the action store is missing» — turning a
 * store that exists and will not read into a store that is not there. Absence
 * and corruption do not share a sentence. */
interface ReadFailure {
  ok: false
  status: number
  /** The core's own outcome name, or '' when it named none. */
  name: string
  detail: string
}

async function getJSON<T>(url: string): Promise<{ ok: true; value: T } | ReadFailure> {
  let res: Response
  try {
    res = await fetch(url, { cache: 'no-store' })
  } catch (e) {
    return { ok: false, status: 0, name: '', detail: e instanceof Error ? e.message : String(e) }
  }
  const text = await res.text().catch(() => '')
  // The core's error bodies are `{error: <name>, message: <text>}`
  // (`writeApprovalError`) or `{error: <text>}` / `{error_code, message}`
  // (`writeError`, `writeErrorCode`). A body it did not write — the plain-text
  // 500 of an unbound error — parses as nothing, and then the name stays empty,
  // which is itself information: the core hit a failure it has no name for.
  let named = ''
  let message = text
  try {
    const body = JSON.parse(text) as { error?: string; error_code?: string; message?: string }
    named = body.error_code ?? body.error ?? ''
    message = body.message ?? body.error ?? text
  } catch {
    /* not our JSON: the raw body is all there is */
  }
  if (!res.ok) return { ok: false, status: res.status, name: named, detail: message }
  try {
    return { ok: true, value: JSON.parse(text) as T }
  } catch {
    return {
      ok: false,
      status: res.status,
      name: '',
      detail: text === '' ? 'cuerpo vacío' : text,
    }
  }
}

/** The status door's answer, trimmed to what the screen reads. */
interface ReloadStatus {
  state?: string
  action_id?: string
  receipt_id?: string
}

/** How the screen waits for a cutover: every POLL_EVERY_MS, at most POLL_BUDGET
 * times (30 s). A real cutover takes well under that; the budget exists so a
 * supervisor that never answers cannot pin the screen forever. */
const POLL_EVERY_MS = 250
const POLL_BUDGET = 120
const TERMINAL_STATES = new Set(['succeeded', 'rolled-back', 'failed', 'persist-failed'])

/** Polls the status door until a terminal state or the budget runs out. One
 * failed poll is not the end of the story: the admin port rotates during the
 * cutover and the proxy answers 503 in that window. */
async function pollUntilTerminal(handle: string): Promise<ReloadStatus | null> {
  for (let i = 0; i < POLL_BUDGET; i++) {
    const polled = await getJSON<ReloadStatus>(`/api/reload/${handle}`)
    if (polled.ok && TERMINAL_STATES.has(polled.value.state ?? '')) return polled.value
    await new Promise((resolve) => setTimeout(resolve, POLL_EVERY_MS))
  }
  return null
}

/** What the core said about the approvals door, when it did not answer. */
const APPROVALS_FAILURE_ES: Record<string, string> = {
  evidence_corrupt:
    'El almacén de acciones existe y su evidencia NO verifica. Esto es permanente: no es que falte el almacén, es que lo que hay no se puede creer.',
  unavailable:
    'El almacén de acciones no se pudo leer en este instante. Es transitorio y no dice nada sobre la evidencia guardada.',
  params_unreadable: 'El almacén de acciones existe y hay parámetros que no se pueden leer.',
  forbidden: 'Esta ventana no está autorizada a leer las aprobaciones.',
}

export function WhatsHappening(): React.JSX.Element {
  const [state, setState] = useState<Loaded>({ kind: 'loading' })
  const [pressed, setPressed] = useState<Pressed | null>(null)
  const [confirming, setConfirming] = useState<string | null>(null)
  const [host, setHost] = useState('')

  const load = useCallback(async (): Promise<void> => {
    const contract = await getJSON<Contract>('/api/whats-happening')
    if (!contract.ok) {
      // 404 here means the mutation surface is not mounted at all: Korvun is
      // running read-only, with no admin token resolved. That is a state with
      // its own sentence, not a failure to report.
      setState(
        contract.status === 404
          ? { kind: 'no-door' }
          : { kind: 'unreadable', detail: contract.detail },
      )
      return
    }
    // The gate is allowed to be absent: no action store, which is the first
    // blocker the contract names.
    const gate = await getJSON<{ gate: Gate }>('/api/approvals')
    // The cages come from the config door the builder already uses. They are
    // NOT in the park gate on purpose: a cage does not stop a request from
    // parking, it refuses it after the operator has approved — which is why
    // this screen shows them in the GREEN state, where they are the only thing
    // still able to say no.
    const cfg = await getJSON<ConfigShape>('/api/config')
    setState({
      kind: 'ready',
      contract: contract.value,
      gate: gate.ok ? gate.value.gate : null,
      // The failure is KEPT. A 404 here is the approvals surface not mounted,
      // which really is the store's absence; a 409 `evidence_corrupt` is a store
      // that exists and will not read. Collapsing both into «no gate» told the
      // operator the wrong one of the two.
      gateFailure: gate.ok ? null : gate,
      cages: cfg.ok ? cagesOf(cfg.value) : [],
      cageFailure: cfg.ok ? null : cfg,
    })
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const press = useCallback(
    async (row: Row, brain: string, tool: string): Promise<void> => {
      const door = row.door
      if (door === undefined) return
      const body: Record<string, unknown> = { confirm: true }
      // The brain travels only for the doors that act on one. The store's row
      // is about the PROFILE, and a body carrying `brain: '—'` would be the
      // screen inventing a target the core never named.
      if (door === 'set-ceiling' || door === 'lift-shadow' || door === 'allow-host')
        body.brain = brain
      // The tool is the one the CORE named, never one this screen assumed. A
      // hardcoded name would be right on the profile the last demo used and
      // wrong on every other.
      if (door === 'lift-shadow' || door === 'allow-host') body.tool = tool
      if (door === 'allow-host') body.host = host
      const res = await fetch(`/api/whats-happening/${door}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const text = await res.text().catch(() => '')
      let outcome = 'unreadable'
      let detail = text
      let actionId = ''
      let receiptId = ''
      let handle = ''
      try {
        const parsed = JSON.parse(text) as {
          outcome?: string
          detail?: string
          handle?: string
          act?: { action_id?: string; receipt_id?: string }
        }
        outcome = parsed.outcome ?? 'unreadable'
        detail = parsed.detail ?? ''
        actionId = parsed.act?.action_id ?? ''
        receiptId = parsed.act?.receipt_id ?? ''
        handle = parsed.handle ?? ''
      } catch {
        /* the raw body is the detail */
      }
      // A cutover still running answers with a handle and NO receipt: the receipt
      // seals the outcome, and the outcome is not known yet. The status door is
      // where it arrives, so the screen polls it until the supervisor reports a
      // terminal state — bounded. The first version asked ONCE, right after the
      // POST, when a real cutover (build, start, persist) is still in flight;
      // the operator stayed on «aplicando…» for good (the internal pass's P3).
      if (outcome === 'applying' && handle !== '') {
        const polled = await pollUntilTerminal(handle)
        if (polled !== null) {
          if (polled.receipt_id !== undefined) receiptId = polled.receipt_id
          if (polled.action_id !== undefined) actionId = polled.action_id
          const state = polled.state ?? ''
          if (state === 'succeeded') outcome = 'applied'
          else if (state === 'rolled-back' || state === 'failed') outcome = 'not_applied'
          else if (state === 'persist-failed') outcome = 'applied_not_saved'
          // One outcome, one sentence. The POST's `detail` described a cutover
          // still in flight; once the poll reports how it ended, that detail is
          // stale, so it does not survive and the row says the terminal
          // outcome's sentence alone. The founding door's path rode in that same
          // detail, so after a poll it is not shown; train G's planned cure moves
          // the path to a field of its own.
          detail = ''
        } else {
          detail =
            'Sigue aplicando; el estado tardó más de lo que esta pantalla espera. Vuelve a mirar en un momento.'
        }
      }
      setPressed({ door, outcome, detail, actionId, receiptId })
      setConfirming(null)
      // Whatever happened, the profile may have moved: re-read rather than
      // guess. A screen that inferred the new state from its own click would be
      // painting what it asked for, not what the core did.
      await load()
    },
    [host, load],
  )

  if (state.kind === 'loading') {
    return <div className="panel-empty">Leyendo tu perfil…</div>
  }
  if (state.kind === 'no-door') {
    return (
      <section className="panel" aria-label="¿Qué pasa hoy?" data-testid="whats-no-door">
        <header className="panel-head">
          <span className="panel-title">¿Qué pasa hoy?</span>
        </header>
        <div className="panel-row">
          <div className="panel-row-body">
            <div className="panel-row-title">Esta pantalla no tiene puerta</div>
            <div className="panel-row-caption">
              Korvun está sirviendo en modo solo lectura: su perfil no nombra un{' '}
              <span className="mono">admin.token_env</span> que resuelva, así que no hay ninguna
              superficie por la que cambiar nada desde aquí. Se arregla en el perfil y reiniciando.
            </div>
          </div>
        </div>
      </section>
    )
  }
  if (state.kind === 'unreadable') {
    return (
      <section className="panel" aria-label="¿Qué pasa hoy?" data-testid="whats-unreadable">
        <header className="panel-head">
          <span className="panel-title">¿Qué pasa hoy?</span>
        </header>
        <div className="panel-row">
          <div className="panel-row-body">
            <div className="panel-row-title">No se pudo leer el estado</div>
            <div className="panel-row-caption">{state.detail}</div>
          </div>
        </div>
      </section>
    )
  }

  const { contract, gate, gateFailure, cages, cageFailure } = state
  const rowFor = (rule: string): Row | undefined => contract.rows.find((r) => r.rule === rule)
  // A 404 is the approvals surface NOT MOUNTED, which the app does exactly when
  // there is no action store — so that one, and only that one, is the contract's
  // `store` row. Every other failure is a store that EXISTS and did not answer,
  // and it gets the core's own name. The two used to share one sentence, and the
  // operator was told a store was missing while holding a corrupt one.
  const storeAbsent = gateFailure !== null && gateFailure.status === 404
  const blocks: ParkBlock[] = storeAbsent
    ? [{ brain: '—', failed: ['store'] }]
    : (gate?.blocked ?? [])
  const gateUnreadable = gateFailure !== null && !storeAbsent
  const approvalsOff = gate !== null && !gate.approvals_enabled
  const ledgerForeign = contract.ledger?.standing === 'ledger_foreign_profile'
  const ledgerLegacy = contract.ledger?.standing === 'legacy_unfounded'
  const ledgerUnreadable = contract.ledger?.standing === 'unreadable'
  const ledgerEnvironment = contract.ledger?.standing === 'environment'
  const ledgerUnavailable = contract.ledger?.standing === 'unavailable'
  const ledgerCause = contract.ledger?.owner ?? ''
  const ledgerPath = contract.ledger?.path
  // A book that cannot be used — a verdict on it, or the storage around it —
  // disables every control of this screen that would write: nothing it wrote
  // could be recorded. «Unavailable» keeps them: it says nothing against the
  // book.
  const blocked = ledgerUnreadable ? 'unreadable' : ledgerEnvironment ? 'environment' : ''
  // An unreadable gate is NOT «nothing blocks». Saying so would be the fail-open
  // this screen exists to prevent: the operator would read a green verdict over
  // a store nobody could question.
  const nothingBlocks = blocks.length === 0 && !approvalsOff && !gateUnreadable
  // «Se aparca y te espera» is a promise about the ledger too: a book that is
  // unreadable, or whose storage fails, records no act, so nothing would park.
  // With either of those two states the green row is not drawn; the cages still
  // are, with their controls disabled. «Unavailable» changes nothing here.
  const parksToday = nothingBlocks && blocked === ''

  return (
    <section className="panel" aria-label="¿Qué pasa hoy?" data-testid="whats-happening">
      <header className="panel-head">
        <span className="panel-title">¿Qué pasa hoy?</span>
        <span className="panel-count">{contract.rows.length}</span>
      </header>

      {gateUnreadable && gateFailure !== null ? (
        // A store that exists and did not answer. It is NOT the `store` row, and
        // it is NOT a green verdict: the operator is told the core's own name for
        // what happened, so «permanente» and «transitorio» stay apart.
        <div className="panel-row" data-testid="whats-gate-unreadable">
          <div className="panel-row-body">
            <div className="panel-row-title">
              <IconWarning size={13} /> No se pudo saber qué pasa con una acción irreversible
            </div>
            <div className="panel-row-caption">
              {APPROVALS_FAILURE_ES[gateFailure.name] ??
                `El núcleo respondió ${gateFailure.status}${
                  gateFailure.name === '' ? ' sin nombrar el motivo' : ` · ${gateFailure.name}`
                }.`}
              {gateFailure.detail !== '' ? ` ${gateFailure.detail}` : ''}
            </div>
            <div className="panel-row-caption">
              Esto NO quiere decir que falte el almacén de acciones. Mientras no se lea, esta
              pantalla no puede decir si algo se aparcaría.
            </div>
          </div>
        </div>
      ) : null}

      {parksToday ? (
        <div className="panel-row" data-testid="whats-green">
          <div className="panel-row-body">
            <div className="panel-row-title">Una acción irreversible se aparca y te espera</div>
            <div className="panel-row-caption">
              Las aprobaciones están encendidas y {gate?.brains_can_park ?? 0} de{' '}
              {gate?.brains_total ?? 0} cerebros pueden aparcar.
              {cageFailure === null ? ' Lo que sigue decidiendo DESPUÉS de que digas que sí:' : ''}
            </div>
          </div>
        </div>
      ) : null}

      {nothingBlocks && cageFailure !== null ? (
        // The cages come from `/api/config`. A failed read leaves the list
        // EMPTY, and an empty list under «lo que sigue decidiendo después» reads
        // as «nothing else decides» — which is the opposite of «we could not
        // look». The promise is withdrawn above and replaced here.
        <div className="panel-row" data-testid="whats-cages-unreadable">
          <div className="panel-row-body">
            <div className="panel-row-title">
              <IconWarning size={13} /> No se pudieron leer las jaulas de red
            </div>
            <div className="panel-row-caption">
              El perfil no se pudo leer ({cageFailure.status}
              {cageFailure.name === '' ? '' : ` · ${cageFailure.name}`}), así que esta pantalla no
              sabe con qué hosts pueden hablar las herramientas. No quiere decir que no haya
              ninguna.
            </div>
          </div>
        </div>
      ) : null}

      {nothingBlocks
        ? cages.map((c) => (
            <BlockedRow
              key={`cage:${c.brain}:${c.tool}`}
              row={rowFor('cage')}
              brain={c.brain}
              tool={c.tool}
              hosts={c.hosts}
              confirming={confirming}
              setConfirming={setConfirming}
              host={host}
              setHost={setHost}
              press={press}
              pressed={pressed}
              blocked={blocked}
            />
          ))
        : null}

      {nothingBlocks ? <AfterApprovalTextRows rows={contract.rows} /> : null}

      {ledgerForeign ? (
        // The ledger belongs to another profile: nothing below can apply until
        // this profile adopts it, so the row comes FIRST, with its one button.
        // It takes no `blocked`: a foreign ledger is one the core could judge.
        <BlockedRow
          key="ledger_foreign_profile"
          row={rowFor('ledger_foreign_profile')}
          brain=""
          tool=""
          confirming={confirming}
          setConfirming={setConfirming}
          host={host}
          setHost={setHost}
          press={press}
          pressed={pressed}
        />
      ) : null}

      {ledgerUnreadable ? (
        // A verdict on the book: D2, its cause and its remedy, the file named.
        <div className="panel-row" data-testid="whats-ledger-unreadable">
          <div className="panel-row-body">
            <div className="panel-row-caption">
              <IconWarning size={13} />{' '}
              <span data-testid="whats-ledger-d2">{d2(ledgerCause, ledgerPath)}</span>
            </div>
          </div>
        </div>
      ) : null}

      {ledgerEnvironment ? (
        // The storage around the book failed: D3 for the environment.
        <div className="panel-row" data-testid="whats-ledger-environment">
          <div className="panel-row-body">
            {d3Environment(ledgerCause, ledgerPath).map((line, i) => (
              <div
                key={line}
                className={i === 0 ? 'panel-row-title' : 'panel-row-caption'}
                data-testid="whats-ledger-environment-line"
              >
                {line}
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {ledgerUnavailable ? (
        // The book could not be checked now: D3 of the moment, the buttons on.
        <div className="panel-row" data-testid="whats-ledger-unavailable-row">
          <div className="panel-row-body">
            <div className="panel-row-caption">
              <span data-testid="whats-ledger-unavailable">{d3Moment(ledgerCause)}</span>
            </div>
          </div>
        </div>
      ) : null}

      {ledgerLegacy ? (
        // No mark at all: a ledger older than this version. Named, not a
        // blocker, not corruption — and no button: this screen offers nothing
        // to mark it with (adoption is offered on a FOREIGN ledger only, and a
        // ledger that exists cannot be founded again).
        <div className="panel-row" data-testid="whats-ledger-legacy">
          <div className="panel-row-body">
            <div className="panel-row-title">Libro anterior a esta versión</div>
            <div className="panel-row-caption">
              El libro de acciones no lleva marca de perfil: es anterior a esta versión. No bloquea
              nada, y esta pantalla no ofrece marcarlo.
            </div>
          </div>
        </div>
      ) : null}

      {approvalsOff ? (
        <BlockedRow
          key="approvals_disabled"
          row={rowFor('approvals_disabled')}
          brain=""
          tool=""
          confirming={confirming}
          setConfirming={setConfirming}
          host={host}
          setHost={setHost}
          press={press}
          pressed={pressed}
          blocked={blocked}
        />
      ) : null}

      {blocks.map((b) =>
        b.failed.map((cond) => {
          const rule = contract.conditions[cond]
          const row = rule === undefined ? undefined : rowFor(rule)
          return (
            <BlockedRow
              key={`${b.brain}:${cond}`}
              row={row}
              cond={cond}
              brain={b.brain}
              tool={b.tool ?? ''}
              confirming={confirming}
              setConfirming={setConfirming}
              host={host}
              setHost={setHost}
              press={press}
              pressed={pressed}
              blocked={blocked}
            />
          )
        }),
      )}

      {/* Everything the screen carries, whether or not it blocks today. The
          text-only rows are the conservative half of the law: an operator who
          cannot press anything must still be told where to go. */}
      <details className="panel-row" data-testid="whats-all-rows">
        <summary className="panel-row-title">Todo lo que decide ({contract.rows.length})</summary>
        {contract.rows.map((r) => (
          <div className="panel-row" key={r.rule}>
            <div className="panel-row-body">
              <div className="panel-row-title">
                {r.label}
                {r.after_approval === true ? (
                  <span className="pill pill-vio">decide después de tu sí</span>
                ) : null}
              </div>
              <div className="panel-row-caption">
                se cambia en el perfil: <span className="mono">{r.profile_key}</span>
                {r.why !== undefined && r.why !== '' ? ` — ${r.why}` : ''}
              </div>
            </div>
            {r.has_button ? <span className="pill pill-ok">tiene botón</span> : null}
          </div>
        ))}
      </details>
    </section>
  )
}

/** The after-approval dimensions that have NO button, in the green state.
 *
 * `AfterApproval` on the Go side says these «belong in the GREEN state too, not
 * only among the blockers». The cage rows above cover one of them; the private
 * network shield is the other, and it was reachable only inside the collapsed
 * «todo lo que decide» list — so the green label enumerated less than the
 * contract declares. It is text, not a button: the shield has no profile key of
 * its own, it arms itself from two others.
 */
function AfterApprovalTextRows({ rows }: { rows: Row[] }): React.JSX.Element {
  const textOnly = rows.filter((r) => r.after_approval === true && !r.has_button)
  return (
    <>
      {textOnly.map((r) => (
        <div className="panel-row" key={r.rule} data-testid={`whats-after-${r.rule}`}>
          <div className="panel-row-body">
            <div className="panel-row-title">
              {r.label}
              <span className="pill pill-vio">decide después de tu sí</span>
            </div>
            <div className="panel-row-caption">
              {r.profile_key}
              {r.why !== undefined && r.why !== '' ? ` — ${r.why}` : ''}
            </div>
          </div>
        </div>
      ))}
    </>
  )
}

export function BlockedRow({
  row,
  cond,
  brain,
  tool,
  hosts,
  confirming,
  setConfirming,
  host,
  setHost,
  press,
  pressed,
  blocked = '',
}: {
  row: Row | undefined
  cond?: string
  brain: string
  /** The tool the core named for this row, or '' when it named none. */
  tool: string
  /** The cage's current allow-list, on the rows that have one. */
  hosts?: string[]
  confirming: string | null
  setConfirming: (v: string | null) => void
  host: string
  setHost: (v: string) => void
  press: (row: Row, brain: string, tool: string) => Promise<void>
  pressed: Pressed | null
  /** Why the screen's writes are off — the ledger `unreadable` or its
   * `environment` failed — or '' when they are on. While it is set, the row's
   * button and the confirmation's confirming button are drawn disabled;
   * «Cancelar» stays, since it only closes the confirmation. */
  blocked?: string
}): React.JSX.Element {
  if (row === undefined) {
    // A condition the contract does not explain. The Go contract mould makes
    // this unreachable by construction; if it ever renders, it must say so
    // rather than draw an empty row the operator cannot act on.
    return (
      <div className="panel-row" data-testid="whats-unexplained">
        <div className="panel-row-body">
          <div className="panel-row-title">
            <IconWarning size={13} /> Un bloqueo que esta versión no sabe explicar
          </div>
          <div className="panel-row-caption">
            El núcleo informa de «{cond ?? '—'}» y esta pantalla no tiene fila para eso.
          </div>
        </div>
      </div>
    )
  }
  const key = `${brain}:${tool}:${row.rule}`
  const needsConfirm = row.door !== undefined && CONFIRMATIONS[row.door] !== undefined
  // A door that acts on a tool cannot be drawn without one. The core leaves the
  // name empty when the condition is not about a tool, and a button carrying an
  // empty name would POST an edit aimed at nothing.
  const needsTool = row.door === 'lift-shadow' || row.door === 'allow-host'
  const door = row.door
  const canPress = row.has_button && door !== undefined && (!needsTool || tool !== '')
  const mine = pressed !== null && pressed.door === row.door
  return (
    <div className="panel-row" data-testid={`whats-block-${row.rule}`}>
      <div className="panel-row-body">
        <div className="panel-row-title">
          {row.label}
          {brain !== '' && brain !== '—' ? <span className="pill pill-off">{brain}</span> : null}
          {tool !== '' ? <span className="pill pill-off">{tool}</span> : null}
        </div>
        <div className="panel-row-caption">
          {hosts !== undefined ? (
            <>
              puede hablar con:{' '}
              <span className="mono">{hosts.length === 0 ? 'ningún host' : hosts.join(', ')}</span>
              {' · '}
            </>
          ) : null}
          se cambia en el perfil: <span className="mono">{row.profile_key}</span>
          {row.why !== undefined && row.why !== '' ? ` — ${row.why}` : ''}
        </div>
        {mine ? (
          <div className="panel-row-caption" role="status" data-testid="whats-outcome">
            {OUTCOME_ES[pressed.outcome] ?? 'Respuesta que esta pantalla no sabe leer.'}
            {pressed.detail !== '' ? ` ${pressed.detail}` : ''}
          </div>
        ) : null}
        {mine && pressed.actionId !== '' ? (
          // The act, SHOWN. Every change through this screen is written into the
          // action ledger, and the seventh law does not exempt the law's own
          // machinery: an operator who cannot see the act cannot check it.
          <div className="panel-row-caption" data-testid="whats-act">
            Queda en el libro como acto <span className="mono">{pressed.actionId}</span>
            {pressed.receiptId !== '' ? (
              <>
                , con recibo <span className="mono">{pressed.receiptId}</span> — se comprueba con{' '}
                <span className="mono">korvun receipt verify</span>
              </>
            ) : (
              <> — el recibo sella el desenlace y llegará cuando el cambio termine</>
            )}
          </div>
        ) : null}
        {confirming === key && door !== undefined ? (
          <div className="panel-row-caption">
            {door === 'allow-host' ? (
              <input
                id={`host-${key}`}
                className="approvals-reason"
                placeholder="hooks.ejemplo.io"
                value={host}
                onChange={(e) => setHost(e.target.value)}
              />
            ) : null}
            <button
              type="button"
              className="btn-primary"
              onClick={() => void press(row, brain, tool)}
              disabled={blocked !== '' || (door === 'allow-host' && host.trim() === '')}
            >
              {CONFIRMATIONS[door]}
            </button>
            <button type="button" className="btn-small" onClick={() => setConfirming(null)}>
              Cancelar
            </button>
          </div>
        ) : null}
      </div>
      {canPress && confirming !== key ? (
        // Secondary, not primary: the house allows ONE identity gradient per
        // view and this row repeats once per blocker, so the accent belongs on
        // the single click that commits — the confirmation — not on each button
        // that merely opens a step.
        <button
          type="button"
          className="btn-secondary"
          disabled={blocked !== ''}
          onClick={() => {
            if (needsConfirm) setConfirming(key)
            else void press(row, brain, tool)
          }}
        >
          {door === undefined ? '' : (BUTTON_ES[door] ?? door)}
        </button>
      ) : null}
    </div>
  )
}
