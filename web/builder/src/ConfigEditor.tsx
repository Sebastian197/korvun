import { useReducer, useState, type Dispatch } from 'react'
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
      <Select label="provider" value={m.provider} options={PROVIDERS} onChange={(v) => up({ provider: v })} />
      <TextField label="model_id" value={m.model_id} onChange={(v) => up({ model_id: v })} placeholder="required" />
      <Select label="locality" value={m.locality} options={LOCALITIES} onChange={(v) => up({ locality: v })} />
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

function BrainForm({
  b,
  index,
  dispatch,
  error,
  onRemove,
}: {
  b: Config['brains'][number]
  index: number
  dispatch: D
  error?: string | undefined
  onRemove: () => void
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
        <Select label="sensitivity" value={b.sensitivity} options={SENSITIVITIES} onChange={set('sensitivity')} />
        <Select label="dispatch" value={b.dispatch} options={DISPATCHES} onChange={set('dispatch')} />
      </div>
      <Select
        label="policy"
        value={b.policy.kind}
        options={POLICY_KINDS}
        onChange={(v) => dispatch({ kind: 'setPolicyKind', brain: index, value: v })}
      />
      <span className="lbl">models</span>
      {b.models.map((m, j) => (
        <ModelRow key={j} m={m} brain={index} index={j} count={b.models.length} dispatch={dispatch} />
      ))}
      <button className="btn" type="button" onClick={() => dispatch({ kind: 'addModel', brain: index })}>
        + add model
      </button>
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
      <TextField label="token_env" value={c.token_env} onChange={set('token_env')} placeholder="env var name" />
    </section>
  )
}

function RouteForm({ r, index, dispatch }: { r: Config['routes'][number]; index: number; dispatch: D }) {
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

function ReloadView({ status, onRetry }: { status: ReloadStatus; onRetry: () => void }) {
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

function SaveErrorView({ error }: { error: SaveError }) {
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
          This config removes the admin token — you would lock yourself out of the builder. Recover by
          editing the -config file and restarting.
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
}: {
  baseline: Config
  token: string
  onSaved?: (handle: string) => void
  onAuthError?: () => void
  reloadDeps?: PollDeps
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
    saveError?.kind === 'validation' && saveError.field?.startsWith(prefix) ? saveError.message : undefined

  async function save() {
    setSaveError(null)
    try {
      const r = await postConfig(token, wc)
      onSaved?.(r.handle)
      const final = await pollReload(r.handle, token, reloadDeps, setReload)
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
          <ChannelForm key={i} c={c} index={i} dispatch={dispatch} error={errorFor(`channels[${i}]`)} />
        ))}
        {noBrains ? (
          <section className="panel first-run" data-testid="empty-brains">
            <h2>brains</h2>
            <p className="muted">No brains yet. Create your first brain to route messages to a model.</p>
            <button className="btn primary" type="button" onClick={() => dispatch({ kind: 'addBrain' })}>
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
