import { lazy, Suspense, useEffect, useState } from 'react'
import { getBrains, getChannels, getConfig, type BrainSummary, type ChannelSummary } from './api.ts'
import { cleartextRisk } from './cleartext.ts'
import type { Config } from './config/schema.ts'
import './App.css'

// The builder: reads the live wiring (/api/brains + /api/channels, open) and —
// once the operator pastes the admin bearer — the full config (/api/config,
// gated). Since SP4 (FR-SCOPE-1) the post-token FACE is the canvas; the 2b
// ConfigEditor survives as a MODULE (ReloadView/SaveErrorView + the reload
// machine live there), no longer as a view.

// Lazy: the canvas chunk loads only after the token gate opens, so the main
// bundle stays flat.
const CanvasView = lazy(() => import('./canvas/CanvasView.tsx').then((m) => ({ default: m.CanvasView })))

const EVENTS = [
  { key: 'received', label: 'received' },
  { key: 'sent', label: 'sent' },
  { key: 'dropped', label: 'dropped' },
  { key: 'failed', label: 'failed' },
] as const

// Embedded in the desktop chrome the gate is skipped with this sentinel: the
// shell proxy OVERWRITES Authorization with the per-cycle bearer server-side
// (ADR-0035 §4), so the pasted value never reaches the core and the real
// bearer can never be known by the user — a gate here protects nothing and
// reads as broken (app-audit 2026-08-23, symptom 1).
const EMBEDDED_BEARER = 'embedded-proxy'

export function App() {
  // window.top differs from window.self only inside an iframe — the desktop
  // chrome's embed (SP6). Computed once per mount, before any token state.
  const embedded = window.self !== window.top
  const [brains, setBrains] = useState<BrainSummary[] | null>(null)
  const [channels, setChannels] = useState<ChannelSummary[] | null>(null)
  const [config, setConfig] = useState<Config | null>(null)
  // in-memory only (ADR-0030 §6); embedded starts past the gate.
  const [token, setToken] = useState(embedded ? EMBEDDED_BEARER : '')
  const [draft, setDraft] = useState('')
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    getBrains().then(setBrains).catch(() => setBrains([]))
    getChannels().then(setChannels).catch(() => setChannels([]))
  }, [])

  useEffect(() => {
    if (!token) {
      setConfig(null)
      return
    }
    getConfig(token)
      .then((c) => {
        setConfig(c)
        setErr(null)
      })
      .catch((e: unknown) => {
        setConfig(null)
        setErr(e instanceof Error ? e.message : 'failed to load config')
      })
  }, [token])

  // Embedded, the desktop chrome already titles the view → the builder's own
  // top bar is also dropped to avoid a double header (SP6).
  return (
    <div className="app">
      {!embedded && (
        <header className="bar">
          <h1 className="brand">
            <span className="glyph" aria-hidden="true" />
            korvun
          </h1>
          <span className="crumb">builder</span>
          <span className="spacer" />
          <span className="token-state">{token ? 'bearer ✓' : 'no token'}</span>
        </header>
      )}

      {cleartextRisk() && (
        <p className="warn" role="note">
          Not on https or loopback. A bearer token would cross the network in cleartext
          — put a TLS terminator in front (ADR-0028 F10). This is advisory, not enforced.
        </p>
      )}

      {config !== null ? (
        // The canvas face (FR-SCOPE-1): palette + surface + properties panel +
        // the save-bar riding the same reload machine as the 2b editor.
        <main className="canvas-main">
          <Suspense fallback={<p className="muted">loading…</p>}>
            <CanvasView baseline={config} token={token} onAuthError={() => setToken('')} />
          </Suspense>
        </main>
      ) : (
        <>
      <main className="grid">
        <section className="panel">
          <h2>Brains</h2>
          {brains === null ? (
            <p className="muted">loading…</p>
          ) : brains.length === 0 ? (
            <p className="muted">none</p>
          ) : (
            brains.map((b) => (
              <div className="card" key={b.name}>
                <div className="card-head">
                  <span className="name">{b.name}</span>
                  <span className="pill">{b.sensitivity}</span>
                </div>
                <div className="meta">
                  {b.policy} · {b.dispatch} ·{' '}
                  {b.models.map((m) => `${m.provider}/${m.model_id}`).join(', ') || 'no models'}
                </div>
              </div>
            ))
          )}
        </section>

        <section className="panel">
          <h2>Channels</h2>
          {channels === null ? (
            <p className="muted">loading…</p>
          ) : channels.length === 0 ? (
            <p className="muted">none</p>
          ) : (
            channels.map((c) => (
              <div className="card" key={c.name}>
                <div className="card-head">
                  <span className="name">{c.name}</span>
                  <span className="pill">{c.mode}</span>
                </div>
                <div className="meta">
                  {c.type}
                  {c.dropped !== undefined ? ` · dropped ${c.dropped}` : ''}
                </div>
              </div>
            ))
          )}
        </section>
      </main>

      <section className="panel wide">
        <h2>Config</h2>
        {!token ? (
          <form
            className="auth"
            onSubmit={(e) => {
              e.preventDefault()
              setToken(draft.trim()) // paste-safe: strip stray whitespace/newlines
            }}
          >
            <label className="lbl" htmlFor="tok">
              admin bearer token
            </label>
            <div className="auth-row">
              <input
                id="tok"
                className="txt"
                type="password"
                autoComplete="off"
                placeholder="paste to load the raw config"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
              />
              <button className="btn primary" type="submit">
                Load
              </button>
            </div>
            <p className="muted">
              Held in memory only, sent as <code>Authorization: Bearer</code>. Never stored,
              never a cookie.
            </p>
          </form>
        ) : err ? (
          <p className="err">Could not load config: {err}</p>
        ) : (
          <p className="muted">loading…</p>
        )}
      </section>
        </>
      )}

      <footer className="legend">
        {EVENTS.map((e) => (
          <span key={e.key}>
            <span className="dot" style={{ background: `var(--${e.key})` }} />
            {e.label}
          </span>
        ))}
      </footer>
    </div>
  )
}
