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
import { postConfig, HttpError } from './api'
import './edit.css'

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

function BrainForm({ b, index, dispatch }: { b: Config['brains'][number]; index: number; dispatch: D }) {
  const set = (field: 'name' | 'sensitivity' | 'dispatch') => (value: string) =>
    dispatch({ kind: 'setBrainField', brain: index, field, value })
  return (
    <section className="panel">
      <h2>brain · {b.name || '(unnamed)'}</h2>
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

function ChannelForm({ c, index, dispatch }: { c: Config['channels'][number]; index: number; dispatch: D }) {
  const set = (field: 'type' | 'mode' | 'token_env') => (value: string) =>
    dispatch({ kind: 'setChannelField', channel: index, field, value })
  return (
    <section className="panel">
      <h2>channel · {c.type}</h2>
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

export function ConfigEditor({
  baseline,
  token,
  onSaved,
}: {
  baseline: Config
  token: string
  onSaved?: (handle: string) => void
}) {
  const [wc, dispatch] = useReducer(configReducer, baseline, clone)
  const [handle, setHandle] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const dirty = isDirty(wc, baseline)

  async function save() {
    setSaving(true)
    setErr(null)
    try {
      const r = await postConfig(token, wc)
      setHandle(r.handle)
      onSaved?.(r.handle)
    } catch (e) {
      // Full 400/401/409 UI is 2b.2c; here we surface the status honestly.
      setErr(e instanceof HttpError ? `save failed: HTTP ${e.status}` : 'save failed')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="editor">
      {wc.channels.map((c, i) => (
        <ChannelForm key={i} c={c} index={i} dispatch={dispatch} />
      ))}
      {wc.brains.map((b, i) => (
        <BrainForm key={i} b={b} index={i} dispatch={dispatch} />
      ))}
      {wc.routes.map((r, i) => (
        <RouteForm key={i} r={r} index={i} dispatch={dispatch} />
      ))}

      <div className="save-bar">
        <button className="btn primary" type="button" disabled={!dirty || saving} onClick={save}>
          {saving ? 'Saving…' : 'Save and reload'}
        </button>
        <span className="muted">{dirty ? 'unsaved changes' : 'no changes'}</span>
        {handle && (
          <span className="handle" data-testid="reload-handle">
            reload handle: <code>{handle}</code>
          </span>
        )}
        {err && <span className="err">{err}</span>}
      </div>
    </div>
  )
}
