// The chrome shell (SP6a, completed through SP6b): sidebar with the design's
// nav + brand mark + status chip, the per-view header (title + /healthz), and
// the real views — Home lands in 6b; the wizard/onboarding/builder embed are
// 6c. Theme control lives in Ajustes (FR-WIN-5).
import { useEffect, useState } from 'react'
import './App.css'
import { BrandMark } from './components/BrandMark'
import { HealthzBadge } from './components/HealthzBadge'
import { StatusChip } from './components/StatusChip'
import {
  IconActivity,
  IconBuilder,
  IconChannels,
  IconHome,
  IconSettings,
} from './components/icons'
import { desktop } from './lib/go'
import { Activity } from './views/Activity'
import { Home } from './views/Home'
import { Settings } from './views/Settings'

type View = 'inicio' | 'builder' | 'canales' | 'actividad' | 'ajustes'

// Design order (6a review rider a): Builder second, exactly as the sidebar
// mock paints it.
const NAV: ReadonlyArray<{
  id: View
  label: string
  icon: (p: { size?: number }) => React.JSX.Element
}> = [
  { id: 'inicio', label: 'Inicio', icon: IconHome },
  { id: 'builder', label: 'Builder', icon: IconBuilder },
  { id: 'canales', label: 'Canales', icon: IconChannels },
  { id: 'actividad', label: 'Actividad', icon: IconActivity },
  { id: 'ajustes', label: 'Ajustes', icon: IconSettings },
]

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
  const [version, setVersion] = useState('dev')

  useEffect(() => {
    void desktop()
      ?.Version()
      .then(setVersion)
      .catch(() => undefined)
  }, [])

  const active = NAV.find((n) => n.id === view)

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-tile" data-testid="brand-tile">
            <BrandMark size={34} />
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
              <span className="nav-icon" aria-hidden="true">
                <item.icon />
              </span>
              {item.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-foot">
          <StatusChip />
        </div>
      </aside>
      <main className="main">
        <header className="main-head">
          <h1 className="view-title">{active?.label ?? 'Inicio'}</h1>
          <HealthzBadge />
        </header>
        {view === 'inicio' && <Home />}
        {view === 'builder' && <Empty label="El builder" />}
        {view === 'canales' && <Empty label="Canales" />}
        {view === 'actividad' && <Activity />}
        {view === 'ajustes' && <Settings version={version} />}
      </main>
    </div>
  )
}