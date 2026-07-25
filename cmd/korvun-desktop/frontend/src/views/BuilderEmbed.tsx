// The Builder view (SP6c, AS-5, ADR-0035 §3c): the EXISTING web/builder
// bundle rendered in a SAME-origin iframe at /builder/ through the SP4 proxy
// (the bearer injection covers it; frame-ancestors 'self' from the commit-0
// CSP change allows exactly this embedding). The chrome's sidebar stays alive
// around it. With the core stopped the proxy answers the stable 503, which
// would render as a broken iframe — so the tab paints the honest stopped
// state instead, never a broken frame.
import { IconBuilder } from '../components/icons'
import { useCoreState } from '../status/store'

export function BuilderEmbed(): React.JSX.Element {
  const core = useCoreState()
  if (core !== 'running') {
    return (
      <div className="builder-stopped" data-testid="builder-stopped">
        <div className="builder-stopped-icon" aria-hidden="true">
          <IconBuilder size={22} />
        </div>
        <div className="builder-stopped-title">El Builder necesita el gateway en marcha</div>
        <p className="builder-stopped-caption">
          Arranca el gateway desde Inicio y el editor visual se cargará aquí, dentro de la ventana.
        </p>
      </div>
    )
  }
  return (
    <div className="builder-embed" data-testid="builder-embed">
      <iframe className="builder-frame" title="Builder" src="/builder/" />
    </div>
  )
}
