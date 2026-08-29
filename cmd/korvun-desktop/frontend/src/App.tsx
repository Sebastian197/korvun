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
import {
  IconActivity,
  IconBuilder,
  IconChannels,
  IconChat,
  IconHome,
  IconSettings,
} from './components/icons'
import { desktop } from './lib/go'
import { useUnreadTotal } from './console/useUnreadTotal'
import { useSnapshot } from './snapshot/store'
import { Activity, ActivityLiveChip } from './views/Activity'
import { BuilderEmbed } from './views/BuilderEmbed'
import { Channels } from './views/Channels'
import { Console } from './views/Console'
import { ChannelWizard } from './views/ChannelWizard'
import { Home } from './views/Home'
import { Onboarding } from './views/Onboarding'
import { Settings } from './views/Settings'

type View = 'inicio' | 'builder' | 'chat' | 'canales' | 'actividad' | 'ajustes'

// Design order (6a review rider a): Builder second, exactly as the sidebar
// mock paints it.
const NAV: ReadonlyArray<{
  id: View
  label: string
  icon: (p: { size?: number }) => React.JSX.Element
}> = [
  { id: 'inicio', label: 'Inicio', icon: IconHome },
  { id: 'builder', label: 'Builder', icon: IconBuilder },
  { id: 'chat', label: 'Chat', icon: IconChat },
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
  // FR-UNREAD: the Chat tab total, alive while the chat is closed. Rendered
  // via data-unread + CSS content so the nav-order text guards stay stable.
  const unread = useUnreadTotal()

  useEffect(() => {
    const d = desktop()
    if (!d) return
    void d
      .Version()
      .then(setVersion)
      .catch(() => undefined)
    // Boot sequence: ensure the default config EXISTS (created=true is the
    // onboarding trigger, ADR-0035 §5), then LOAD it into the Controller so
    // the parado hero's Start actually works — without this the shipping app
    // has no config loaded and Start returns ErrNoConfig (the harness masked
    // it by loading config itself; review finding). LoadConfig is a
    // stopped-state op and boot is stopped, so it is safe here.
    void (async () => {
      let created = false
      try {
        created = await d.EnsureDefaultConfig()
      } catch {
        // No bindings / older shell → stay on the normal chrome.
      }
      setOnboarding(created)
      // Load the config so Start works, best-effort and independent of the
      // onboarding decision above (a LoadConfig failure must not hide the
      // onboarding a fresh install needs).
      try {
        const path = await d.DefaultConfigPath()
        await d.LoadConfig(path)
      } catch {
        // Start will surface its own error if the core cannot boot.
      }
    })()
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
              data-unread={item.id === 'chat' && unread > 0 ? unread : undefined}
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
          {/* N4 (sealed): the volatility chip lives NEXT TO the title. */}
          {view === 'actividad' && <ActivityLiveChip />}
          <HealthzBadge />
        </header>
        {view === 'inicio' && <Home />}
        {view === 'builder' && <BuilderEmbed />}
        {view === 'chat' && <Console />}
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
          onOpenBuilder={() => setView('builder')}
        />
      )}
    </div>
  )
}
