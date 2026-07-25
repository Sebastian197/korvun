// Home (FR-WIN-6): the three honest states over the real stores —
//  marcha      hero strip + window-scoped cards + control-API panels,
//  parado      the stopped hero, with the last session's data dimmed,
//  incidencia  the FR-WIN-4 honest triggers only (reap-shaped exit, or a
//              failure frame with its real channel), recovering on Start.
import { useState } from 'react'
import { IconActivity, IconChannels, IconPlay, IconStop, IconWarning } from '../components/icons'
import { channelGlyph } from '../lib/channels'
import { desktop } from '../lib/go'
import { agoSeconds, hourES } from '../lib/time'
import { minuteSeries, SPARK_MINUTES, useFeed } from '../feed/store'
import {
  clearUserStop,
  dismissIncident,
  markUserStop,
  useIncident,
  type Incident,
} from '../incident/store'
import { useSnapshot, type BrainSummary, type ChannelSummary } from '../snapshot/store'
import { useCoreState, useLastOkAt } from '../status/store'

function useLifecycleAction(): {
  busy: boolean
  error: string
  start: () => void
  stop: () => void
} {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const call = (op: 'Start' | 'Stop'): void => {
    const d = desktop()
    if (!d || busy) return
    // Intent is marked only when a Stop is actually DISPATCHED, and unmarked
    // if the binding rejects — otherwise a no-op or failed Stop would arm a
    // sticky flag that swallows the next real unexpected exit (review).
    if (op === 'Stop') markUserStop()
    setBusy(true)
    setError('')
    d[op]()
      .catch((e: unknown) => {
        if (op === 'Stop') clearUserStop()
        setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => setBusy(false))
  }
  return { busy, error, start: () => call('Start'), stop: () => call('Stop') }
}

function StartButton({ busy, onClick }: { busy: boolean; onClick: () => void }): React.JSX.Element {
  return (
    <button type="button" className="btn-primary" onClick={onClick} disabled={busy}>
      <IconPlay size={15} />
      {busy ? 'Iniciando…' : 'Iniciar'}
    </button>
  )
}

function StopButton({ busy, onClick }: { busy: boolean; onClick: () => void }): React.JSX.Element {
  return (
    <button type="button" className="btn-secondary" onClick={onClick} disabled={busy}>
      <IconStop size={13} />
      {busy ? 'Deteniendo…' : 'Detener'}
    </button>
  )
}

function feedIncidentCaption(inc: Extract<Incident, { kind: 'feed' }>): string {
  const what = inc.frameType === 'message_dropped' ? 'Mensaje descartado' : 'Fallo al responder'
  const hora = hourES(inc.at)
  return `${what} en ${inc.channel} · ${hora} — el detalle vive en Actividad`
}

// Hero: one strip, state-driven (the design's heroOk/heroWarn/heroStop).
function Hero(): React.JSX.Element {
  const core = useCoreState()
  const incident = useIncident()
  const snapshot = useSnapshot()
  const { busy, error, start, stop } = useLifecycleAction()
  const running = core === 'running'

  let icon: React.JSX.Element
  let title: string
  let caption: string
  let tone: 'ok' | 'warn' | 'err' | 'off'
  if (running && incident?.kind === 'feed') {
    tone = 'warn'
    icon = <IconWarning size={17} />
    title = 'En marcha — incidencia'
    caption = feedIncidentCaption(incident)
  } else if (running) {
    tone = 'ok'
    icon = <span className="dot-pulse" />
    const nc = snapshot.channels?.length ?? 0
    const nb = snapshot.brains?.length ?? 0
    title = 'El gateway está en marcha'
    caption = `Sirviendo ${nc} ${nc === 1 ? 'canal' : 'canales'} y ${nb} ${
      nb === 1 ? 'cerebro' : 'cerebros'
    } · sin incidencias en esta ventana`
  } else if (incident?.kind === 'reap') {
    tone = 'err'
    icon = <IconWarning size={17} />
    title = 'El núcleo se detuvo inesperadamente'
    caption =
      'El proceso terminó sin que la ventana lo pidiera; el motivo queda en el registro del shell. Un arranque limpio recupera el estado.'
  } else {
    tone = 'off'
    icon = <IconStop size={15} />
    title = 'El gateway está detenido'
    caption = 'Los canales no reciben ni responden mensajes mientras esté parado'
  }

  return (
    <div className={`hero-strip tone-${tone}`} data-testid="home-hero">
      <div className="hero-strip-icon" aria-hidden="true">
        {icon}
      </div>
      <div className="hero-strip-body">
        <div className="hero-strip-title">{title}</div>
        <div className="hero-strip-caption">{caption}</div>
        {error !== '' && (
          <div className="hero-strip-error" role="alert">
            {error}
          </div>
        )}
      </div>
      {incident?.kind === 'feed' && (
        // EVENT incidents are occurrences, not conditions (spec amendment):
        // "Entendido" acknowledges and clears the banner. A reap never gets
        // this button — a dead core is a live condition.
        <button type="button" className="btn-small" onClick={dismissIncident}>
          Entendido
        </button>
      )}
      {running ? (
        <StopButton busy={busy} onClick={stop} />
      ) : (
        <StartButton busy={busy} onClick={start} />
      )}
    </div>
  )
}

// Bar geometry from the design's sparkline: a visible floor plus the scaled
// range, current minute emphasized.
const SPARK_BAR_MIN_PX = 4
const SPARK_BAR_RANGE_PX = 22

function Sparkline({ series }: { series: number[] }): React.JSX.Element {
  const max = Math.max(1, ...series)
  return (
    <div className="spark" aria-hidden="true">
      {series.map((v, i) => (
        <div
          // A fixed-length trailing window: the index IS the identity.
          key={i}
          className="spark-bar"
          style={{
            height: SPARK_BAR_MIN_PX + Math.round((v / max) * SPARK_BAR_RANGE_PX),
            opacity: i === series.length - 1 ? 0.95 : 0.35,
          }}
        />
      ))}
    </div>
  )
}

function Cards(): React.JSX.Element {
  const feed = useFeed()
  useLastOkAt() // re-render each healthy poll so "hace Xs" ticks
  const dropdInfo =
    feed.counters.dropped === 0
      ? 'ninguno desde que se abrió la ventana'
      : 'desde que se abrió la ventana'
  const lastReply =
    feed.lastReplyAt === null ? 'aún ninguno' : `último ${agoSeconds(feed.lastReplyAt)}`
  return (
    <div className="cards">
      <div className="card" data-testid="card-recibidos">
        <div className="card-title">Mensajes recibidos</div>
        <div className="card-big">{feed.counters.received}</div>
        <div className="card-caption">desde que se abrió la ventana</div>
      </div>
      <div className="card" data-testid="card-procesados">
        <div className="card-title">Mensajes procesados</div>
        <div className="card-row">
          <div className="card-big">{feed.counters.replied}</div>
          <Sparkline series={minuteSeries(Date.now(), SPARK_MINUTES)} />
        </div>
        <div className="card-caption">desde que se abrió la ventana · {lastReply}</div>
      </div>
      <div className="card" data-testid="card-descartados">
        <div className="card-title">Mensajes descartados</div>
        <div className="card-big">{feed.counters.dropped}</div>
        <div className="card-caption">{dropdInfo}</div>
      </div>
    </div>
  )
}

function ChannelRow({
  ch,
  running,
  incidentChannel,
}: {
  ch: ChannelSummary
  running: boolean
  incidentChannel: string | null
}): React.JSX.Element {
  const label = ch.name.charAt(0).toUpperCase() + ch.name.slice(1)
  let pill = <span className="pill pill-off">Detenido</span>
  if (running) {
    pill =
      incidentChannel === ch.name ? (
        <span className="pill pill-warn">Incidencia</span>
      ) : (
        <span className="pill pill-ok">Operativo</span>
      )
  }
  const dropped = ch.dropped ?? 0
  return (
    <div className="panel-row">
      <span className="chan-tile" aria-hidden="true">
        {channelGlyph(ch.type)}
      </span>
      <div className="panel-row-body">
        <div className="panel-row-title">{label}</div>
        <div className="panel-row-caption">
          {ch.mode}
          {dropped > 0 ? ` · ${dropped} descartados desde el arranque` : ''}
        </div>
      </div>
      {pill}
    </div>
  )
}

const SENSITIVITY_ES: Record<string, string> = {
  private: 'Privado',
  public: 'Público',
  internal: 'Interno',
}

function BrainRow({ brain }: { brain: BrainSummary }): React.JSX.Element {
  return (
    <div className="panel-row panel-row-brain">
      <div className="panel-row-body">
        <div className="brain-head">
          <span className="panel-row-title">{brain.name}</span>
          <span className={`pill ${brain.sensitivity === 'private' ? 'pill-vio' : 'pill-ok'}`}>
            {SENSITIVITY_ES[brain.sensitivity] ?? brain.sensitivity}
          </span>
          <span className="pill pill-off">{brain.dispatch}</span>
        </div>
        {brain.models.map((m) => (
          <div className="brain-model" key={`${m.provider}/${m.model_id}`}>
            <span className="dot-ok" aria-hidden="true" />
            <span className="mono">{m.model_id}</span>
            <span className="brain-provider">· {m.provider}</span>
          </div>
        ))}
        <div className="panel-row-caption">política · {brain.policy}</div>
      </div>
    </div>
  )
}

function Panels(): React.JSX.Element {
  const snapshot = useSnapshot()
  const core = useCoreState()
  const incident = useIncident()
  const running = core === 'running'
  const incidentChannel = incident?.kind === 'feed' ? incident.channel : null
  const channels = snapshot.channels ?? []
  const brains = snapshot.brains ?? []
  return (
    <div className="panels">
      <section className="panel" aria-label="Canales">
        <header className="panel-head">
          <span className="panel-title">Canales</span>
          <span className="panel-count">{channels.length}</span>
        </header>
        {channels.length === 0 ? (
          <div className="panel-empty">Sin canales configurados</div>
        ) : (
          channels.map((ch) => (
            <ChannelRow key={ch.name} ch={ch} running={running} incidentChannel={incidentChannel} />
          ))
        )}
      </section>
      <section className="panel" aria-label="Cerebros">
        <header className="panel-head">
          <span className="panel-title">Cerebros</span>
          <span className="panel-count">{brains.length}</span>
        </header>
        {brains.length === 0 ? (
          <div className="panel-empty">Sin cerebros configurados</div>
        ) : (
          brains.map((b) => <BrainRow key={b.name} brain={b} />)
        )}
      </section>
    </div>
  )
}

export function Home(): React.JSX.Element {
  const core = useCoreState()
  const snapshot = useSnapshot()
  const running = core === 'running'
  const hasLastData = (snapshot.channels?.length ?? 0) > 0 || (snapshot.brains?.length ?? 0) > 0

  return (
    <div className="home" data-testid={running ? 'home-marcha' : 'home-parado'}>
      <Hero />
      {running ? (
        <>
          <Cards />
          {/* Until the FIRST fresh poll of this cycle answers, the retained
              snapshot is still last session's — dim it, never sell it as
              live (review finding). */}
          {snapshot.stale ? (
            <div className="home-stale">
              <Panels />
            </div>
          ) : (
            <Panels />
          )}
        </>
      ) : (
        hasLastData && (
          // The last session's answer, dimmed and honest — never live data.
          <div className="home-stale" data-testid="home-stale-data">
            <div className="stale-note">
              <IconActivity size={13} /> Último dato de la sesión — el gateway está parado
            </div>
            <Cards />
            <Panels />
          </div>
        )
      )}
      {!running && !hasLastData && (
        <p className="home-hint">
          <IconChannels size={13} /> Arranca el gateway para ver canales, cerebros y el feed en
          vivo.
        </p>
      )}
    </div>
  )
}
