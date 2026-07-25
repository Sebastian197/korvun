// The header's /healthz caption (design: top-right, mono): OK + freshness
// while running, an honest "sin respuesta" when the proxy answers 503, and
// a dash while nothing is known yet.
import { agoSeconds } from '../lib/time'
import { useCoreState, useLastOkAt } from '../status/store'

export function HealthzBadge(): React.JSX.Element {
  const core = useCoreState()
  const lastOk = useLastOkAt()
  let text = '/healthz · —'
  let ok = false
  if (core === 'running') {
    text = `/healthz · OK · ${agoSeconds(lastOk) || 'hace 0 s'}`
    ok = true
  } else if (core === 'stopped' || core === 'unreachable') {
    text = '/healthz · sin respuesta'
  }
  return (
    <span className={`healthz ${ok ? 'healthz-ok' : ''}`} data-testid="healthz-badge">
      {text}
    </span>
  )
}
