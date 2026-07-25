// Actividad v1 (FR-WIN-4 — ADR-0024 rules): the metadata feed, honestly.
// The frame carries type/channel/brain/timestamp/envelope_id/direction and
// NOTHING else — message content is excluded by the core's construction, so
// the view keeps the design's layout and idiom (filter chips, En vivo, the
// table rhythm) over exactly that metadata. Filters: channel and type.
import { useState } from 'react'
import { IconActivity } from '../components/icons'
import type { FeedFrame } from '../feed/frame'
import { useFeed } from '../feed/store'
import { useCoreState } from '../status/store'

const TYPE_LABEL: Record<string, string> = {
  message_received: 'Mensaje recibido',
  reply_sent: 'Respuesta enviada',
  message_dropped: 'Mensaje descartado',
  handle_failed: 'Fallo al responder',
}

const TYPE_FILTERS: ReadonlyArray<{ id: string; label: string }> = [
  { id: 'all', label: 'Todos los eventos' },
  { id: 'message_received', label: 'Recibidos' },
  { id: 'reply_sent', label: 'Respuestas' },
  { id: 'message_dropped', label: 'Descartados' },
  { id: 'handle_failed', label: 'Fallos' },
]

const TILE: Record<string, string> = { telegram: 'TG', discord: 'DC' }

function hourOf(ts: string): string {
  const d = new Date(ts)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleTimeString('es-ES', { hour12: false })
}

function dirOf(direction?: string): string {
  if (direction === 'inbound') return 'in'
  if (direction === 'outbound') return 'out'
  return '—'
}

function rowChip(f: FeedFrame): React.JSX.Element {
  if (f.type === 'message_dropped') return <span className="pill pill-warn">descartado</span>
  if (f.type === 'handle_failed') return <span className="pill pill-err">fallo</span>
  if (f.brain !== undefined && f.brain !== '')
    return <span className="pill pill-vio">→ {f.brain}</span>
  return <span className="pill pill-ok">ok</span>
}

function Row({ f }: { f: FeedFrame }): React.JSX.Element {
  return (
    <div className="act-row">
      <span className="act-hour mono">{hourOf(f.timestamp)}</span>
      <span className="chan-tile act-tile" aria-hidden="true">
        {TILE[f.channel ?? ''] ?? (f.channel ?? '??').slice(0, 2).toUpperCase()}
      </span>
      <span className="act-event">
        {TYPE_LABEL[f.type] ?? f.type}
        {f.envelope_id !== undefined && f.envelope_id !== '' && (
          <span className="act-id mono">{f.envelope_id}</span>
        )}
      </span>
      {rowChip(f)}
      <span className="act-dir mono">{dirOf(f.direction)}</span>
    </div>
  )
}

export function Activity(): React.JSX.Element {
  const feed = useFeed()
  const core = useCoreState()
  const [channel, setChannel] = useState('all')
  const [type, setType] = useState('all')

  const rows = feed.frames.filter(
    (f) =>
      (channel === 'all' || f.channel === channel) && (type === 'all' || f.type === type),
  )

  let liveText = 'Pausado — gateway detenido'
  let liveOn = false
  if (feed.live) {
    liveText = 'En vivo'
    liveOn = true
  } else if (core === 'running') {
    liveText = 'Conectando…'
  }

  return (
    <div className="activity" data-testid="actividad">
      <div className="act-filters">
        <button
          type="button"
          className={channel === 'all' ? 'chip-btn chip-on' : 'chip-btn'}
          onClick={() => setChannel('all')}
        >
          Todos
        </button>
        {feed.channels.map((c) => (
          <button
            key={c}
            type="button"
            className={channel === c ? 'chip-btn chip-on' : 'chip-btn'}
            onClick={() => setChannel(c)}
          >
            {c}
          </button>
        ))}
        <span className="act-sep" aria-hidden="true" />
        {TYPE_FILTERS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={type === t.id ? 'chip-btn chip-on' : 'chip-btn'}
            onClick={() => setType(t.id)}
          >
            {t.label}
          </button>
        ))}
        <span className="act-spacer" />
        <span className="act-live">
          <span className={`chip-dot ${liveOn ? 'chip-dot-ok' : ''}`} aria-hidden="true" />
          {liveText}
        </span>
      </div>

      <div className="act-table">
        {rows.length === 0 ? (
          <div className="act-empty">
            <div className="act-empty-icon" aria-hidden="true">
              <IconActivity size={16} />
            </div>
            <div className="act-empty-title">Sin actividad todavía</div>
            <div className="act-empty-caption">
              Cuando fluyan mensajes, aquí verás cada mensaje y su camino por el gateway —
              solo metadatos, nunca el contenido.
            </div>
          </div>
        ) : (
          <>
            <div className="act-head">
              <span className="act-hour">HORA</span>
              <span className="act-tile-space" />
              <span className="act-event">EVENTO</span>
              <span>DESTINO</span>
              <span className="act-dir">DIR</span>
            </div>
            {rows.map((f, i) => (
              <Row key={`${f.envelope_id ?? ''}·${f.timestamp}·${f.type}·${String(i)}`} f={f} />
            ))}
          </>
        )}
      </div>
    </div>
  )
}