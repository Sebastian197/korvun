// Canales (SP6c, contra final-15/16/17): a list of the connected channels
// with real health from /api/channels, a detail panel showing only what
// exists for real (the config route as a chip, health, the secret honesty
// note — env-var NAME only), the "Añadir canal" CTA into the wizard, and
// "Quitar canal…" behind a confirmation → the config-mutation pipe. Nothing
// without an API is painted (FR-WIN-4).
import { useEffect, useState } from 'react'
import { IconChannels, IconWarning } from '../components/icons'
import { channelGlyph } from '../lib/channels'
import {
  fetchConfig,
  mutateConfig,
  removeChannel,
  type CoreConfig,
  type MutateOptions,
} from '../config/mutate'
import { useSnapshot, type ChannelSummary } from '../snapshot/store'
import { useCoreState } from '../status/store'

// Build a MutateOptions with only the defined keys (exactOptionalPropertyTypes).
function mutateOpts(fetcher?: typeof fetch, pollIntervalMs?: number): MutateOptions {
  const o: MutateOptions = {}
  if (fetcher !== undefined) o.fetcher = fetcher
  if (pollIntervalMs !== undefined) o.pollIntervalMs = pollIntervalMs
  return o
}

const CHANNEL_LABEL = (type: string): string => type.charAt(0).toUpperCase() + type.slice(1)

const TOKEN_DEFAULT: Record<string, string> = {
  telegram: 'TELEGRAM_TOKEN',
  discord: 'DISCORD_BOT_TOKEN',
}

interface ChannelsProps {
  onOpenWizard: () => void
  onOpenBuilder: () => void
  fetcher?: typeof fetch
  pollIntervalMs?: number
}

function healthPill(running: boolean): React.JSX.Element {
  return running ? (
    <span className="pill pill-ok">Operativo</span>
  ) : (
    <span className="pill pill-off">Detenido</span>
  )
}

export function Channels({
  onOpenWizard,
  onOpenBuilder,
  fetcher,
  pollIntervalMs,
}: ChannelsProps): React.JSX.Element {
  const snapshot = useSnapshot()
  const core = useCoreState()
  const running = core === 'running'
  const channels = snapshot.channels ?? []
  const [selected, setSelected] = useState<string | null>(null)
  const [config, setConfig] = useState<CoreConfig | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // The route + token env-var name live in the config, not /api/channels.
  useEffect(() => {
    let alive = true
    void fetchConfig(fetcher).then((c) => {
      if (alive) setConfig(c)
    })
    return () => {
      alive = false
    }
  }, [fetcher])

  const active = selected ?? channels[0]?.name ?? null
  const activeSummary = channels.find((c) => c.name === active) ?? null
  const activeCfg = config?.channels.find((c) => c.type === active) ?? null
  const activeRoute = config?.routes.find((r) => r.channel === active) ?? null

  const remove = (): void => {
    if (active === null || busy) return
    setBusy(true)
    setError('')
    void mutateConfig((c) => removeChannel(c, active), mutateOpts(fetcher, pollIntervalMs))
      .then((res) => {
        if (!res.ok) setError(`No se pudo quitar el canal (${res.state}).`)
        else setSelected(null)
      })
      .finally(() => {
        setBusy(false)
        setConfirming(false)
      })
  }

  return (
    <div className="channels" data-testid="canales">
      <div className="chan-list">
        <div className="chan-list-head">
          <span className="panel-title">Conectados</span>
          <span className="panel-count">{channels.length}</span>
          <span className="chan-spacer" />
          <button type="button" className="btn-primary btn-primary-sm" onClick={onOpenWizard}>
            + Añadir canal
          </button>
        </div>
        {channels.length === 0 ? (
          <div className="chan-empty">
            <IconChannels size={20} />
            <p>Aún no hay canales. Conecta el primero con “Añadir canal”.</p>
          </div>
        ) : (
          channels.map((ch) => (
            <button
              key={ch.name}
              type="button"
              className={`chan-row ${ch.name === active ? 'chan-row-on' : ''}`}
              onClick={() => setSelected(ch.name)}
            >
              <span className="chan-tile" aria-hidden="true">
                {channelGlyph(ch.type)}
              </span>
              <span className="chan-row-body">
                <span className="chan-row-title">{CHANNEL_LABEL(ch.type)}</span>
                <span className="chan-row-caption mono">
                  {ch.mode} ·{' '}
                  {config?.channels.find((c) => c.type === ch.type)?.token_env ??
                    TOKEN_DEFAULT[ch.type] ??
                    'sin secreto'}
                </span>
              </span>
              <span className={`chan-dot ${running ? 'chan-dot-ok' : ''}`} aria-hidden="true" />
            </button>
          ))
        )}
      </div>

      {activeSummary !== null && (
        <ChannelDetail
          summary={activeSummary}
          cfg={activeCfg}
          route={activeRoute}
          running={running}
          onOpenBuilder={onOpenBuilder}
          onRemove={() => setConfirming(true)}
          busy={busy}
          error={error}
        />
      )}

      {confirming && (
        <ConfirmRemove
          name={active ?? ''}
          busy={busy}
          onCancel={() => setConfirming(false)}
          onConfirm={remove}
        />
      )}
    </div>
  )
}

function ChannelDetail({
  summary,
  cfg,
  route,
  running,
  onOpenBuilder,
  onRemove,
  busy,
  error,
}: {
  summary: ChannelSummary
  cfg: { type: string; mode: string; token_env: string } | null
  route: { channel: string; brain: string } | null
  running: boolean
  onOpenBuilder: () => void
  onRemove: () => void
  busy: boolean
  error: string
}): React.JSX.Element {
  const tokenEnv = cfg?.token_env ?? ''
  const dropped = summary.dropped ?? 0
  return (
    <div className="chan-detail" data-testid="canal-detalle">
      <div className="chan-detail-head">
        <span className="chan-tile" aria-hidden="true">
          {channelGlyph(summary.type)}
        </span>
        <div className="chan-detail-title">
          <div className="panel-row-title">{CHANNEL_LABEL(summary.type)}</div>
          <div className="panel-row-caption">
            {summary.type} · {summary.mode}
          </div>
        </div>
        <span className="chan-spacer" />
        {healthPill(running)}
      </div>

      <div className="chan-detail-body">
        <div className="chan-field">
          <span className="chan-field-label mono">RUTA</span>
          <div className="chan-chip">
            <span className="mono">
              {route !== null ? `${route.channel} → ${route.brain}` : 'sin ruta'}
            </span>
            <span className="chan-spacer" />
            <button type="button" className="chan-link" onClick={onOpenBuilder}>
              cambiar en el Builder →
            </button>
          </div>
        </div>

        {dropped > 0 && (
          <div className="chan-field">
            <span className="chan-field-label mono">DESCARTADOS (VENTANA DEL CORE)</span>
            <div className="chan-chip mono">{dropped}</div>
          </div>
        )}

        {tokenEnv !== '' ? (
          <div className="chan-field">
            <span className="chan-field-label mono">TOKEN DEL BOT</span>
            <div className="chan-chip">
              <span className="mono">{tokenEnv}</span>
              <span className="chan-spacer" />
              <span className="chan-only-name">solo nombre</span>
            </div>
            <div className="chan-secret-note">
              <IconWarning size={13} />
              <span>
                El valor vive en tu entorno ({tokenEnv}). Korvun no lo guarda, no lo registra y no
                lo muestra — si se filtra, revócalo y exporta el nuevo.
              </span>
            </div>
          </div>
        ) : (
          <div className="chan-field">
            <span className="chan-field-caption">
              Este canal no usa secretos: recibe peticiones HTTP en tu propia máquina.
            </span>
          </div>
        )}

        {error !== '' && (
          <div className="hero-strip-error" role="alert">
            {error}
          </div>
        )}

        <div className="chan-detail-foot">
          <button type="button" className="btn-danger" onClick={onRemove} disabled={busy}>
            Quitar canal…
          </button>
        </div>
      </div>
    </div>
  )
}

function ConfirmRemove({
  name,
  busy,
  onCancel,
  onConfirm,
}: {
  name: string
  busy: boolean
  onCancel: () => void
  onConfirm: () => void
}): React.JSX.Element {
  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label="Quitar canal">
      <div className="modal">
        <div className="modal-title">¿Quitar {CHANNEL_LABEL(name)}?</div>
        <p className="modal-body">
          El canal dejará de recibir y responder mensajes. El token en tu entorno o tu llavero no se
          toca — puedes volver a añadirlo cuando quieras.
        </p>
        <div className="modal-actions">
          <button type="button" className="btn-secondary" onClick={onCancel} disabled={busy}>
            Cancelar
          </button>
          <button type="button" className="btn-danger" onClick={onConfirm} disabled={busy}>
            {busy ? 'Quitando…' : 'Sí, quitar'}
          </button>
        </div>
      </div>
    </div>
  )
}
