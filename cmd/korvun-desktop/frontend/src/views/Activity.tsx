// Actividad v1 (FR-WIN-4 — ADR-0024 rules): the metadata feed, honestly.
// The frame carries type/channel/brain/timestamp/envelope_id/direction and
// NOTHING else — message content is excluded by the core's construction, so
// the view keeps the design's layout and idiom (filter chips, En vivo, the
// table rhythm) over exactly that metadata. Filters: channel and type.
import { useState } from 'react'
import { IconActivity } from '../components/icons'
import type { FeedFrame } from '../feed/frame'
import { useFeed } from '../feed/store'
import { channelGlyph } from '../lib/channels'
import { hourES } from '../lib/time'
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
      <span className="act-hour mono">{hourES(f.timestamp)}</span>
      <span className="chan-tile act-tile" aria-hidden="true">
        {channelGlyph(f.channel)}
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

// ActivityLiveChip is the N4 state chip the shell mounts NEXT TO the view
// title (sealed design ola2-designs §4) — the permanent, honest declaration
// that the feed is window-scoped. The pill keeps the three honest states
// the view carried before; only the LIVE one earns the pulsing green dot
// (soft pulse, silenced under prefers-reduced-motion in App.css).
export function ActivityLiveChip(): React.JSX.Element {
  const feed = useFeed()
  const core = useCoreState()
  let text = 'Pausado — gateway detenido'
  let on = false
  if (feed.live) {
    text = 'En vivo · desde que se abrió la ventana'
    on = true
  } else if (core === 'running') {
    text = 'Conectando…'
  }
  return (
    <span className="act-live-chip" data-testid="act-live-chip">
      <span className={`chip-dot ${on ? 'chip-dot-ok chip-dot-pulse' : ''}`} aria-hidden="true" />
      {text}
    </span>
  )
}

export function Activity(): React.JSX.Element {
  const feed = useFeed()
  const [channel, setChannel] = useState('all')
  const [type, setType] = useState('all')

  const rows = feed.frames.filter(
    (f) => (channel === 'all' || f.channel === channel) && (type === 'all' || f.type === type),
  )

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
      </div>

      <div className="act-table">
        {rows.length === 0 ? (
          <div className="act-empty">
            <div className="act-empty-icon" aria-hidden="true">
              <IconActivity size={16} />
            </div>
            <div className="act-empty-title">Sin actividad todavía</div>
            <div className="act-empty-caption">
              Cuando fluyan mensajes, aquí verás cada mensaje y su camino por el gateway.
            </div>
            {/* N4 (sealed): the honest volatility line, visible — never fine print. */}
            <div className="act-empty-caption act-empty-volatility">
              El feed vive en esta ventana — al cerrarla se vacía. Solo metadatos, nunca el
              contenido.
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
            {rows.map((f) => (
              // seq is the store's monotonic ingest id — a prepend mounts ONE
              // new row instead of remounting the whole list (review finding).
              <Row key={f.seq ?? f.timestamp} f={f} />
            ))}
          </>
        )}
      </div>
    </div>
  )
}
