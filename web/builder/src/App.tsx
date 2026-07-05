import { useEffect, useState } from 'react'
import { getBrains, getChannels, getConfig, type BrainSummary, type ChannelSummary } from './api.ts'
import type { Config } from './config/schema.ts'
import { ConfigEditor } from './ConfigEditor.tsx'
import './App.css'

// Phase 2b.1 minimal builder: reads the live wiring (/api/brains + /api/channels,
// open) and — once the operator pastes the admin bearer — the raw config
// (/api/config, gated). READ-ONLY: no edit forms yet (that is 2b.2). The point of
// this cut is "the builder loads and shows the live state, wearing Korvun's face."

// Non-loopback + non-https means a pasted bearer would cross the network in the
// clear (ADR-0028 F10). We WARN — we do not pretend to block (ADR-0030 §6): the
// server does not enforce it and a JS check is not a security control.
function cleartextRisk(): boolean {
  const { protocol, hostname } = window.location
  if (protocol === 'https:') return false
  return hostname !== 'localhost' && hostname !== '127.0.0.1' && hostname !== '[::1]'
}

const EVENTS = [
  { key: 'received', label: 'received' },
  { key: 'sent', label: 'sent' },
  { key: 'dropped', label: 'dropped' },
  { key: 'failed', label: 'failed' },
] as const

export function App() {
  const [brains, setBrains] = useState<BrainSummary[] | null>(null)
  const [channels, setChannels] = useState<ChannelSummary[] | null>(null)
  const [config, setConfig] = useState<Config | null>(null)
  const [token, setToken] = useState('') // in-memory only (ADR-0030 §6)
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

  return (
    <div className="app">
      <header className="bar">
        <span className="brand">
          <span className="glyph" aria-hidden="true" />
          korvun
        </span>
        <span className="crumb">builder · read-only</span>
        <span className="spacer" />
        <span className="token-state">{token ? 'bearer ✓' : 'no token'}</span>
      </header>

      {cleartextRisk() && (
        <p className="warn" role="note">
          Not on https or loopback. A bearer token would cross the network in cleartext
          — put a TLS terminator in front (ADR-0028 F10). This is advisory, not enforced.
        </p>
      )}

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
        ) : config === null ? (
          <p className="muted">loading…</p>
        ) : (
          <ConfigEditor baseline={config} token={token} onAuthError={() => setToken('')} />
        )}
      </section>

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
