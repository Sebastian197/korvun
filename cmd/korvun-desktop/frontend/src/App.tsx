// The chrome shell (SP6a → SP6c): sidebar with the design's nav + brand mark
// + status chip, the per-view header, and the real views — Home, Canales (+
// the channel wizard overlay), Activity, the Builder same-origin iframe, and
// Settings. On a fresh install (EnsureDefaultConfig created=true) the whole
// chrome is replaced by the onboarding until it finishes.
import { useEffect, useState } from 'react'
import './App.css'
import { BrandMark } from './components/BrandMark'
import { HealthzBadge } from './components/HealthzBadge'
import { StatusChip } from './components/StatusChip'
import { IconActivity, IconBuilder, IconChannels, IconHome, IconSettings } from './components/icons'
import { desktop } from './lib/go'
import { useSnapshot } from './snapshot/store'
import { Activity } from './views/Activity'
import { BuilderEmbed } from './views/BuilderEmbed'
import { Channels } from './views/Channels'
import { ChannelWizard } from './views/ChannelWizard'
import { Home } from './views/Home'
import { Onboarding } from './views/Onboarding'
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

export function App(): React.JSX.Element {
  const [view, setView] = useState<View>('inicio')
  const [version, setVersion] = useState('dev')
  const [onboarding, setOnboarding] = useState(false)
  const [wizardOpen, setWizardOpen] = useState(false)
  const snapshot = useSnapshot()

  useEffect(() => {
    const d = desktop()
    if (!d) return
    void d
      .Version()
      .then(setVersion)
      .catch(() => undefined)
    // Fresh install → run onboarding until it finishes (ADR-0035 §5). A
    // false/absent result (existing config, or no bindings) skips it.
    void d
      .EnsureDefaultConfig()
      .then((created) => setOnboarding(created))
      .catch(() => undefined)
  }, [])

  const active = NAV.find((n) => n.id === view)
  const existingTypes = (snapshot.channels ?? []).map((c) => c.type)

  if (onboarding) {
    return <Onboarding onFinished={() => setOnboarding(false)} />
  }

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
        {view === 'builder' && <BuilderEmbed />}
        {view === 'canales' && (
          <Channels
            onOpenWizard={() => setWizardOpen(true)}
            onOpenBuilder={() => setView('builder')}
          />
        )}
        {view === 'actividad' && <Activity />}
        {view === 'ajustes' && <Settings version={version} />}
      </main>

      {wizardOpen && (
        <ChannelWizard
          existingTypes={existingTypes}
          onClose={() => setWizardOpen(false)}
          onDone={() => setWizardOpen(false)}
        />
      )}
    </div>
  )
}
