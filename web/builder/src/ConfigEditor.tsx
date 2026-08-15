import { useReducer, useState, type Dispatch } from 'react'
import { flushSync } from 'react-dom'
import type { Config } from './config/schema'
import {
  PROVIDERS,
  SENSITIVITIES,
  DISPATCHES,
  POLICY_KINDS,
  LOCALITIES,
  CHANNEL_TYPES,
  CHANNEL_MODES,
  CLOUD_PROVIDERS,
} from './config/schema'
import { clone, isDirty, configReducer, type ConfigAction } from './config/edit'
import {
  TOOL_MODES,
  CAGE_TOOLS,
  effectiveToolAttr,
  shieldShown,
  sensitiveCloudWarning,
  grantMode,
  grantChannels,
  type ToolMode,
} from './config/governance'
import { postConfig, getConfig, getReloadStatus, HttpError } from './api'
import { pollReload, type ReloadStatus, type PollDeps } from './config/reload'
import { parseSaveError, type SaveError } from './config/errors'
import './edit.css'

// Real poll deps (overridable in tests): status from the control API, a timer-based
// sleep, and the wall clock.
const realReloadDeps: PollDeps = {
  getStatus: getReloadStatus,
  sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
  now: () => Date.now(),
}

// Phase 2b.2a — the edit surface. Reads the working copy (cloned from the GET
// /api/config baseline), edits it through the pure reducer, builds the FULL config,
// and POSTs it → shows the raw reload handle. The reload state machine that polls the
// handle is 2b.2b; the error/edge UI is 2b.2c. All enum dropdowns are native <select>
// (accessible by default — the a11y floor, ADR-0030 §8).

type D = Dispatch<ConfigAction>

function Select({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: readonly string[]
  onChange: (v: string) => void
}) {
  return (
    <label className="field">
      <span className="lbl">{label}</span>
      <select className="txt" value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    </label>
  )
}

function TextField({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <label className="field">
      <span className="lbl">{label}</span>
      <input
        className="txt"
        value={value}
        placeholder={placeholder ?? ''}
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  )
}

function ModelRow({
  m,
  brain,
  index,
  count,
  dispatch,
}: {
  m: Config['brains'][number]['models'][number]
  brain: number
  index: number
  count: number
  dispatch: D
}) {
  const isCloud = CLOUD_PROVIDERS.has(m.provider)
  const up = (patch: Partial<typeof m>) =>
    dispatch({ kind: 'updateModel', brain, model: index, patch })
  return (
    <div className="model-row">
      <Select
        label="provider"
        value={m.provider}
        options={PROVIDERS}
        onChange={(v) => up({ provider: v })}
      />
      <TextField
        label="model_id"
        value={m.model_id}
        onChange={(v) => up({ model_id: v })}
        placeholder="required"
      />
      <Select
        label="locality"
        value={m.locality}
        options={LOCALITIES}
        onChange={(v) => up({ locality: v })}
      />
      {isCloud && (
        <TextField
          label="api_key_env"
          value={m.api_key_env ?? ''}
          onChange={(v) => up({ api_key_env: v })}
          placeholder="env var name"
        />
      )}
      <div className="row-actions">
        <button
          className="btn ghost"
          type="button"
          aria-label="move model up"
          disabled={index === 0}
          onClick={() => dispatch({ kind: 'moveModel', brain, from: index, to: index - 1 })}
        >
          ↑
        </button>
        <button
          className="btn ghost"
          type="button"
          aria-label="move model down"
          disabled={index === count - 1}
          onClick={() => dispatch({ kind: 'moveModel', brain, from: index, to: index + 1 })}
        >
          ↓
        </button>
        <button
          className="btn ghost"
          type="button"
          aria-label="remove model"
          onClick={() => dispatch({ kind: 'removeModel', brain, model: index })}
        >
          ✕
        </button>
      </div>
    </div>
  )
}

/** A skill detected in the agent's skills_dir (name + description, read-only). */
export interface DetectedSkill {
  name: string
  description: string
}

/** SP6 governance panel — the agent-brain "Herramientas y skills" section. The
 *  visual contract is design-drafts/governance-panel/: tri-state grant per tool
 *  (violet=allow, amber-dashed=shadow, neutral=deny), attribute chips, channel
 *  chips, the derived non-editable network shield, the three cages with a simple
 *  allow-list editor, and a read-only detected-skills list. Governed by the
 *  existing save-bar (the hot shadow→allow promotion IS the usual Apply). */
function ToolsSection({
  b,
  index,
  dispatch,
  channelNames,
  detectedSkills,
}: {
  b: Config['brains'][number]
  index: number
  dispatch: D
  channelNames: string[]
  detectedSkills: DetectedSkill[]
}) {
  const agent = b.agent
  if (!agent) return null
  const warned = sensitiveCloudWarning(b)
  const toolChannels = [...channelNames, 'console']

  return (
    <div data-testid={`governance-section-${index}`}>
      <hr className="section-divider" />
      <div className="sec-title">Herramientas y skills</div>
      <p className="sec-sub">
        Qué puede ver y ejecutar el agente. La promoción se aplica en caliente con la cabecera.
      </p>

      {warned.length === 0 &&
        agent.tools.map((tool) => {
          const mode: ToolMode = grantMode(agent, tool) ?? 'allow'
          const scoped = grantChannels(agent, tool)
          const sensitive = effectiveToolAttr(agent.tool_attrs, tool, 'sensitive')
          const network = effectiveToolAttr(agent.tool_attrs, tool, 'network')
          const ungoverned = grantMode(agent, tool) === undefined
          return (
            <div className="tool" key={tool}>
              <div className="tool-top">
                <span className="tool-name">{tool}</span>
                <span className="tool-attrs">
                  {sensitive && <span className="attr on">sensible</span>}
                  {network && <span className="attr on">red</span>}
                </span>
              </div>
              <div className="tri" role="group" aria-label={`modo de ${tool}`}>
                {TOOL_MODES.map((m) => {
                  const active = !ungoverned && mode === m
                  const label = m === 'allow' ? 'Permitir' : m === 'shadow' ? 'Ensayo' : 'Denegar'
                  return (
                    <button
                      key={m}
                      type="button"
                      data-testid={`tri-${tool}-${m}`}
                      className={active ? `on-${m}` : ''}
                      aria-pressed={active}
                      onClick={() => dispatch({ kind: 'setToolMode', brain: index, tool, mode: m })}
                    >
                      <span className="sw" />
                      {label}
                    </button>
                  )
                })}
              </div>
              <div className="tool-meta">
                <span className="meta-lead">canales</span>
                {scoped.length === 0 ? (
                  <span className="chip ch">todos</span>
                ) : (
                  scoped.map((c) => (
                    <span className="chip ch" key={c}>
                      {c}
                    </span>
                  ))
                )}
                {shieldShown(b, tool) && (
                  <span className="derived" data-testid={`shield-${tool}`}>
                    <span className="lock">🔒</span>escudo de red · privado
                  </span>
                )}
                <select
                  className="chip-select"
                  aria-label={`canales de ${tool}`}
                  value=""
                  onChange={(e) => {
                    const ch = e.target.value
                    if (!ch) return
                    const next = scoped.includes(ch)
                      ? scoped.filter((x) => x !== ch)
                      : [...scoped, ch]
                    dispatch({ kind: 'setToolChannels', brain: index, tool, channels: next })
                  }}
                >
                  <option value="">± canal</option>
                  {toolChannels.map((c) => (
                    <option key={c} value={c}>
                      {scoped.includes(c) ? `− ${c}` : `+ ${c}`}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          )
        })}

      {warned.length > 0 && (
        <div className="empty-note">
          Este cerebro no tiene reglas de gobierno. Sus herramientas listadas quedan en{' '}
          <b>Permitir</b> en todos los canales.
          <div className="house-warn" data-testid={`sensitive-cloud-warning-${index}`}>
            <span className="ic">▲</span>
            <span className="tx">
              <b>{warned.join(', ')} es una herramienta sensible sobre un modelo en la nube.</b> Sin
              gobierno, su salida puede viajar al proveedor en la nube. Añade un bloque de gobierno,
              o marca <code>sensible: no</code> a conciencia. El arranque lo rechaza hasta entonces.
            </span>
          </div>
        </div>
      )}

      {/* Cages */}
      {CAGE_TOOLS.filter((c) => agent.tools.includes(c)).length > 0 && (
        <span className="lbl govspace">Jaulas</span>
      )}
      {agent.tools.includes('read_file') && (
        <div className="cage">
          <div className="cage-h">
            <span className="t">read_file</span> · raíz y tamaño
          </div>
          <div className="field nomb">
            <span className="lbl">raíz</span>
            <input
              className="txt mono"
              data-testid="cage-read_file-root"
              value={agent.read_file?.root ?? ''}
              onChange={(e) =>
                dispatch({
                  kind: 'setCageField',
                  brain: index,
                  cage: 'read_file',
                  field: 'root',
                  value: e.target.value,
                })
              }
            />
          </div>
          <div className="field nomb">
            <span className="lbl">máx. tamaño</span>
            <input
              className="txt mono"
              data-testid="cage-read_file-max"
              value={agent.read_file?.max_bytes ?? ''}
              onChange={(e) =>
                dispatch({
                  kind: 'setCageField',
                  brain: index,
                  cage: 'read_file',
                  field: 'max_bytes',
                  value: Number(e.target.value) || 0,
                })
              }
            />
          </div>
        </div>
      )}
      {(['http_fetch', 'webhook_call'] as const)
        .filter((c) => agent.tools.includes(c))
        .map((cage) => (
          <div className="cage" key={cage}>
            <div className="cage-h">
              <span className="t">{cage}</span> · hosts permitidos
            </div>
            <div className="hostlist">
              {(agent[cage]?.allow_hosts ?? []).map((h, hi) => (
                <div className="host" key={hi}>
                  <input
                    className="txt mono"
                    data-testid={`cage-${cage}-host-${hi}`}
                    value={h}
                    onChange={(e) =>
                      dispatch({
                        kind: 'setCageHost',
                        brain: index,
                        cage,
                        index: hi,
                        value: e.target.value,
                      })
                    }
                  />
                  <button
                    className="x"
                    type="button"
                    aria-label={`quitar host ${hi} de ${cage}`}
                    onClick={() =>
                      dispatch({ kind: 'removeCageHost', brain: index, cage, index: hi })
                    }
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
            <button
              className="host-add"
              type="button"
              data-testid={`cage-${cage}-add`}
              onClick={() => dispatch({ kind: 'addCageHost', brain: index, cage })}
            >
              + añadir host
            </button>
          </div>
        ))}

      {/* Skills */}
      <span className="lbl govspace">Skills</span>
      <div className="row2">
        <div className="field">
          <span className="lbl">carpeta</span>
          <input
            className="txt mono"
            data-testid="skills-dir"
            value={agent.skills_dir ?? ''}
            onChange={(e) =>
              dispatch({
                kind: 'setSkillsField',
                brain: index,
                field: 'skills_dir',
                value: e.target.value,
              })
            }
          />
        </div>
        <div className="field">
          <span className="lbl">presup. cuerpos</span>
          <input
            className="txt mono"
            data-testid="skills-budget"
            value={agent.skills_body_budget ?? ''}
            onChange={(e) =>
              dispatch({
                kind: 'setSkillsField',
                brain: index,
                field: 'skills_body_budget',
                value: Number(e.target.value) || 0,
              })
            }
          />
        </div>
      </div>
      {detectedSkills.length > 0 && (
        <div className="cage skills-box" data-testid={`skills-list-${index}`}>
          {detectedSkills.map((s) => (
            <div className="skill" key={s.name}>
              <span className="s-name">{s.name}</span>
              <span className="s-desc">{s.description}</span>
              <span className="ro-flag">solo lectura</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function BrainForm({
  b,
  index,
  dispatch,
  error,
  onRemove,
  channelNames,
  detectedSkills,
}: {
  b: Config['brains'][number]
  index: number
  dispatch: D
  error?: string | undefined
  onRemove: () => void
  channelNames: string[]
  detectedSkills: DetectedSkill[]
}) {
  const [confirm, setConfirm] = useState(false)
  const set = (field: 'name' | 'sensitivity' | 'dispatch') => (value: string) =>
    dispatch({ kind: 'setBrainField', brain: index, field, value })
  return (
    <section className="panel">
      <div className="panel-head">
        <h2>brain · {b.name || '(unnamed)'}</h2>
        {confirm ? (
          <span className="confirm" data-testid={`brain-remove-confirm-${index}`}>
            Remove brain?
            <button
              className="btn ghost"
              type="button"
              onClick={() => {
                onRemove()
                setConfirm(false)
              }}
            >
              Yes, remove
            </button>
            <button className="btn ghost" type="button" onClick={() => setConfirm(false)}>
              Cancel
            </button>
          </span>
        ) : (
          <button
            className="btn ghost"
            type="button"
            aria-label={`remove brain ${b.name || index}`}
            onClick={() => setConfirm(true)}
          >
            ✕ remove
          </button>
        )}
      </div>
      {error && (
        <p className="field-err" role="alert" data-testid={`brain-error-${index}`}>
          {error}
        </p>
      )}
      <TextField label="name" value={b.name} onChange={set('name')} />
      <div className="row2">
        <Select
          label="sensitivity"
          value={b.sensitivity}
          options={SENSITIVITIES}
          onChange={set('sensitivity')}
        />
        <Select
          label="dispatch"
          value={b.dispatch}
          options={DISPATCHES}
          onChange={set('dispatch')}
        />
      </div>
      <Select
        label="policy"
        value={b.policy.kind}
        options={POLICY_KINDS}
        onChange={(v) => dispatch({ kind: 'setPolicyKind', brain: index, value: v })}
      />
      <span className="lbl">models</span>
      {b.models.map((m, j) => (
        <ModelRow
          key={j}
          m={m}
          brain={index}
          index={j}
          count={b.models.length}
          dispatch={dispatch}
        />
      ))}
      <button
        className="btn"
        type="button"
        onClick={() => dispatch({ kind: 'addModel', brain: index })}
      >
        + add model
      </button>
      <ToolsSection
        b={b}
        index={index}
        dispatch={dispatch}
        channelNames={channelNames}
        detectedSkills={detectedSkills}
      />
    </section>
  )
}

function ChannelForm({
  c,
  index,
  dispatch,
  error,
}: {
  c: Config['channels'][number]
  index: number
  dispatch: D
  error?: string | undefined
}) {
  const set = (field: 'type' | 'mode' | 'token_env') => (value: string) =>
    dispatch({ kind: 'setChannelField', channel: index, field, value })
  return (
    <section className="panel">
      <h2>channel · {c.type}</h2>
      {error && (
        <p className="field-err" role="alert" data-testid={`channel-error-${index}`}>
          {error}
        </p>
      )}
      <div className="row2">
        <Select label="type" value={c.type} options={CHANNEL_TYPES} onChange={set('type')} />
        <Select label="mode" value={c.mode} options={CHANNEL_MODES} onChange={set('mode')} />
      </div>
      <TextField
        label="token_env"
        value={c.token_env}
        onChange={set('token_env')}
        placeholder="env var name"
      />
    </section>
  )
}

function RouteForm({
  r,
  index,
  dispatch,
}: {
  r: Config['routes'][number]
  index: number
  dispatch: D
}) {
  const set = (field: 'channel' | 'brain') => (value: string) =>
    dispatch({ kind: 'setRouteField', route: index, field, value })
  return (
    <section className="panel">
      <h2>route</h2>
      <div className="row2">
        <TextField label="channel" value={r.channel} onChange={set('channel')} />
        <TextField label="brain" value={r.brain} onChange={set('brain')} />
      </div>
    </section>
  )
}

// Exported since SP3: the canvas save-bar reuses the SAME reload UI (the
// FR-HOT-5 rule — no duplicated reload states).
export function ReloadView({ status, onRetry }: { status: ReloadStatus; onRetry: () => void }) {
  switch (status.phase) {
    case 'idle':
      return null
    case 'polling':
      // In-flight: pending / cutover-in-progress. The form is locked (see fieldset).
      return (
        <div className="reload-banner" role="status" data-testid="reload-inflight">
          <span className="dot" style={{ background: 'var(--accent)' }} /> reloading —{' '}
          <code>{status.server}</code>
          <span className="handle">
            {' '}
            handle <code>{status.handle}</code>
          </span>
        </div>
      )
    case 'succeeded':
      return (
        <span className="reload-chip ok" role="status" data-testid="reload-succeeded">
          <span className="dot" style={{ background: 'var(--sent)' }} /> reload succeeded
        </span>
      )
    case 'rolledBack':
    case 'failed':
      return (
        <div className="reload-panel err" role="alert" data-testid="reload-terminal">
          <span className="dot" style={{ background: 'var(--failed)' }} /> reload{' '}
          {status.phase === 'failed' ? 'failed' : 'rolled back'} — the running config is unchanged.
          <button className="btn" type="button" onClick={onRetry}>
            Retry
          </button>
        </div>
      )
    case 'unknown':
      return (
        <div className="reload-panel warn" role="alert" data-testid="reload-unknown">
          <span className="dot" style={{ background: 'var(--dropped)' }} /> reload status unknown —
          refresh to re-check.
        </div>
      )
  }
}

// Exported since SP3: the canvas reuses the SAME save-error treatments.
export function SaveErrorView({ error }: { error: SaveError }) {
  switch (error.kind) {
    case 'validation':
      return (
        <div className="save-error err" role="alert" data-testid="save-validation">
          validation error{error.field ? ` at ${error.field}` : ''}: {error.message}
        </div>
      )
    case 'selfLock':
      return (
        <div className="save-error err" role="alert" data-testid="save-selflock">
          This config removes the admin token — you would lock yourself out of the builder. Recover
          by editing the -config file and restarting.
        </div>
      )
    case 'reloadInProgress':
      return (
        <div className="save-error warn" role="alert" data-testid="save-reload-in-progress">
          A reload is already in progress — wait for it to finish, then try again.
        </div>
      )
    case 'unauthorized':
      return null // handled by onAuthError (token cleared, paste screen returns)
    case 'other':
      return (
        <div className="save-error err" role="alert" data-testid="save-other">
          save failed (HTTP {error.status}): {error.message}
        </div>
      )
  }
}

export function ConfigEditor({
  baseline,
  token,
  onSaved,
  onAuthError,
  reloadDeps = realReloadDeps,
  detectedSkills = [],
}: {
  baseline: Config
  token: string
  onSaved?: (handle: string) => void
  onAuthError?: () => void
  reloadDeps?: PollDeps
  detectedSkills?: DetectedSkill[]
}) {
  const [base, setBase] = useState<Config>(baseline)
  const [wc, dispatch] = useReducer(configReducer, baseline, clone)
  const [reload, setReload] = useState<ReloadStatus>({ phase: 'idle' })
  const [saveError, setSaveError] = useState<SaveError | null>(null)
  const [confirmDiscard, setConfirmDiscard] = useState(false)

  const locked = reload.phase === 'polling' // full form lock during the swap (§5)
  const dirty = isDirty(wc, base)

  // Map a 400's field path to the section that can act on it (§400 validation inline).
  const errorFor = (prefix: string): string | undefined =>
    saveError?.kind === 'validation' && saveError.field?.startsWith(prefix)
      ? saveError.message
      : undefined

  // View Transitions for the reload state swap (pending → cutover → succeeded /
  // failed): a scoped cross-fade of the reload region only. Checks startViewTransition
  // FIRST so jsdom (no such API, no matchMedia) always takes the plain branch — and
  // reduced-motion falls back too. Wraps the RENDER; the reload state machine
  // (pollReload/reloadReducer) is untouched. (2b.3b)
  const applyReload = (next: ReloadStatus) => {
    const doc = document as Document & { startViewTransition?: (cb: () => void) => void }
    if (doc.startViewTransition && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      doc.startViewTransition(() => flushSync(() => setReload(next)))
    } else {
      setReload(next)
    }
  }

  async function save() {
    setSaveError(null)
    try {
      const r = await postConfig(token, wc)
      onSaved?.(r.handle)
      const final = await pollReload(r.handle, token, reloadDeps, applyReload)
      if (final.phase === 'succeeded') {
        // Re-baseline from the applied config so the working copy reflects reality
        // and dirty clears exactly (§5).
        const applied = await getConfig(token)
        setBase(applied)
        dispatch({ kind: 'reset', config: applied })
      }
    } catch (e) {
      setReload({ phase: 'idle' })
      if (e instanceof HttpError) {
        const se = parseSaveError(e.status, e.body)
        if (se.kind === 'unauthorized') {
          onAuthError?.() // clear the in-memory token, return to the paste screen
          return
        }
        setSaveError(se)
      } else {
        setSaveError({ kind: 'other', status: 0, message: 'save failed' })
      }
    }
  }

  function discard() {
    if (!confirmDiscard) {
      setConfirmDiscard(true)
      return
    }
    dispatch({ kind: 'reset', config: base })
    setConfirmDiscard(false)
    setSaveError(null)
  }

  const noBrains = wc.brains.length === 0

  return (
    <div className="editor">
      <fieldset className="forms" disabled={locked}>
        {wc.channels.map((c, i) => (
          <ChannelForm
            key={i}
            c={c}
            index={i}
            dispatch={dispatch}
            error={errorFor(`channels[${i}]`)}
          />
        ))}
        {noBrains ? (
          <section className="panel first-run" data-testid="empty-brains">
            <h2>brains</h2>
            <p className="muted">
              No brains yet. Create your first brain to route messages to a model.
            </p>
            <button
              className="btn primary"
              type="button"
              onClick={() => dispatch({ kind: 'addBrain' })}
            >
              + Create your first brain
            </button>
          </section>
        ) : (
          <>
            {wc.brains.map((b, i) => (
              <BrainForm
                key={i}
                b={b}
                index={i}
                dispatch={dispatch}
                error={errorFor(`brains[${i}]`)}
                onRemove={() => dispatch({ kind: 'removeBrain', brain: i })}
                channelNames={wc.channels.map((c) => c.type)}
                detectedSkills={detectedSkills}
              />
            ))}
            <button className="btn" type="button" onClick={() => dispatch({ kind: 'addBrain' })}>
              + add brain
            </button>
          </>
        )}
        {wc.routes.map((r, i) => (
          <RouteForm key={i} r={r} index={i} dispatch={dispatch} />
        ))}
      </fieldset>

      <div className="save-bar">
        <button className="btn primary" type="button" disabled={!dirty || locked} onClick={save}>
          {locked ? 'Reloading…' : 'Save and reload'}
        </button>
        {confirmDiscard ? (
          <span className="confirm" data-testid="discard-confirm">
            Discard changes?
            <button className="btn ghost" type="button" onClick={discard}>
              Yes, discard
            </button>
            <button className="btn ghost" type="button" onClick={() => setConfirmDiscard(false)}>
              Cancel
            </button>
          </span>
        ) : (
          <button className="btn" type="button" disabled={!dirty || locked} onClick={discard}>
            Discard
          </button>
        )}
        <span className="muted">{dirty ? 'unsaved changes' : 'no changes'}</span>
        <ReloadView status={reload} onRetry={save} />
        {saveError && <SaveErrorView error={saveError} />}
      </div>
    </div>
  )
}
