// The chrome shell (SP6a): sidebar + navigation + theme swap + dynamic
// logo/version, and the MINIMAL stopped Home (hero + wired Start) — the rest
// of the views are honest empty states until their cut (6b/6c) lands them.
import { useEffect, useState } from 'react'
import './App.css'
import { desktop } from './lib/go'
import { useCoreState } from './status/store'
import { applyTheme, storedTheme, type ThemeChoice } from './theme'

type View = 'inicio' | 'canales' | 'actividad' | 'builder' | 'ajustes'

const NAV: ReadonlyArray<{ id: View; label: string }> = [
  { id: 'inicio', label: 'Inicio' },
  { id: 'canales', label: 'Canales' },
  { id: 'actividad', label: 'Actividad' },
  { id: 'builder', label: 'Builder' },
  { id: 'ajustes', label: 'Ajustes' },
]

function HomeParado(): React.JSX.Element {
  // THE LAW's frontend half (FR-WIN-2): one lifecycle call in flight — the
  // button disables while Start runs, and a named error (timeout, busy,
  // boot failure) is painted, never an unhandled rejection.
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const start = (): void => {
    const d = desktop()
    if (!d || busy) return
    setBusy(true)
    setError('')
    d.Start()
      .catch((e: unknown) => {
        setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => setBusy(false))
  }
  return (
    <div className="hero" data-testid="home-parado">
      <span className="hero-badge">
        <span className="dot" aria-hidden="true" />
        Gateway detenido
      </span>
      <h2>Korvun está listo</h2>
      <p>
        El núcleo está parado. Arráncalo para servir tus canales y modelos; la
        ventana sigue viva aunque lo detengas.
      </p>
      <button type="button" className="btn-primary" onClick={start} disabled={busy}>
        {busy ? 'Iniciando…' : 'Iniciar Korvun'}
      </button>
      {error !== '' && (
        <p className="hero-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}

function HomeMarcha(): React.JSX.Element {
  return (
    <div className="empty" data-testid="home-marcha-minimo">
      <span className="pill-running">
        <span className="dot" aria-hidden="true" />
        En marcha
      </span>
      <p>El panel completo de Inicio llega en el siguiente corte.</p>
    </div>
  )
}

function Empty({ label }: { label: string }): React.JSX.Element {
  return (
    <div className="empty">
      <div className="glyph" aria-hidden="true">
        |&lt;
      </div>
      <p>{label} estará disponible próximamente.</p>
    </div>
  )
}

export function App(): React.JSX.Element {
  const [view, setView] = useState<View>('inicio')
  const [theme, setTheme] = useState<ThemeChoice>(() => storedTheme())
  const [version, setVersion] = useState('dev')
  const core = useCoreState()

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  useEffect(() => {
    void desktop()
      ?.Version()
      .then(setVersion)
      .catch(() => undefined)
  }, [])

  const toggleTheme = (): void => {
    setTheme((t) => (t === 'dark' ? 'light' : 'dark'))
  }

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-tile" aria-hidden="true">
            K
          </span>
          <div>
            <div className="brand-name">Korvun</div>
            <div className="brand-version" data-testid="version">
              {version}
            </div>
          </div>
        </div>
        <nav aria-label="Secciones">
          {NAV.map((item) => (
            <button
              key={item.id}
              type="button"
              className="nav-item"
              aria-current={view === item.id ? 'page' : undefined}
              onClick={() => setView(item.id)}
            >
              <span className="nav-dot" aria-hidden="true" />
              {item.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-foot">
          <button
            type="button"
            className="theme-toggle"
            onClick={toggleTheme}
            data-testid="theme-toggle"
          >
            {theme === 'dark' ? 'Tema claro' : 'Tema oscuro'}
          </button>
        </div>
      </aside>
      <main className="main">
        {view === 'inicio' &&
          (core === 'running' ? <HomeMarcha /> : <HomeParado />)}
        {view === 'canales' && <Empty label="Canales" />}
        {view === 'actividad' && <Empty label="Actividad" />}
        {view === 'builder' && <Empty label="El builder" />}
        {view === 'ajustes' && <Empty label="Ajustes" />}
      </main>
    </div>
  )
}
