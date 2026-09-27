// The header's /healthz caption, redesigned in v0.16.2 after measuring it.
//
// What the measurement found: the store polls every 2 s, so «hace X s» could
// only ever read 0, 1 or 2 — true, and carrying no information. Nothing timed
// the round trip. And when the core went quiet the caption dropped `lastOkAt`
// entirely, losing the one datum that matters then: when it last worked. The
// store never cleared that value; this label simply stopped reading it.
//
// So: the HOUR of the last answer and the round trip in ms while healthy; the
// hour KEPT when the answer is lost; and no «en vivo» over a poll that is
// wedged, which `pollOnce`'s own `if (polling) return` makes possible.
import { hourES } from '../lib/time'
import { useCoreState, useLastOkAt, useLastRoundTripMs, usePollStale } from '../status/store'

function hourOf(ms: number | null): string {
  return ms === null ? '' : hourES(new Date(ms).toISOString())
}

export function HealthzBadge(): React.JSX.Element {
  const core = useCoreState()
  const lastOk = useLastOkAt()
  const roundTrip = useLastRoundTripMs()
  const stale = usePollStale()

  let text = '/healthz · —'
  let ok = false
  if (stale && lastOk !== null) {
    // A poll that never answered. Saying «en vivo» here would be the one lie
    // this caption exists to prevent, so it names the doubt and keeps the last
    // hour, which is the only fact still true.
    text = `sin confirmar · última ${hourOf(lastOk)}`
  } else if (core === 'running' && lastOk !== null) {
    text = `en vivo · ${hourOf(lastOk)}`
    if (roundTrip !== null) text += ` · ${roundTrip} ms`
    ok = true
  } else if (core === 'stopped' || core === 'unreachable') {
    // The hour survives the loss. An operator staring at a quiet core needs to
    // know how long it has been quiet, and the store has always known.
    text = lastOk === null ? 'sin respuesta' : `sin respuesta · última ${hourOf(lastOk)}`
  }
  return (
    <span className={`healthz ${ok ? 'healthz-ok' : ''}`} data-testid="healthz-badge">
      {text}
    </span>
  )
}
