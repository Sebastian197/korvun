// The 3-step channel assistant (SP6c, contra chica-18/19/20 + final-4),
// mirroring config.Validate AS IT IS. Step 1: pick a type (telegram/discord;
// a type already configured is DISABLED — the core registers one channel per
// type). Step 2: the token env-var NAME (suggested per type; the mode is
// type-determined, displayed never chosen). Step 3: the OS-keychain step — a
// masked once-only field → SetSecret (value never rendered back, never
// logged, never in the DOM after submit) + "Comprobar entorno" via
// CheckSecretPresence (presence, never value) + the terminal fold. Finishing
// runs the SP5-proven pipe: POST /api/config through the proxy + reload poll.
import { useState } from 'react'
import { WizardSteps } from '../components/WizardSteps'
import { CHANNEL_TYPES, channelGlyph } from '../lib/channels'
import { desktop } from '../lib/go'
import { addChannel, mutateConfig, mutateOpts, type ChannelConfig } from '../config/mutate'

type KsState = 'idle' | 'saving' | 'done'
type PreState = 'idle' | 'checking' | 'ok' | 'fail'

interface WizardProps {
  existingTypes: string[]
  onClose: () => void
  onDone: () => void
  /** Lands the user on the Builder — the webhook card's breadcrumb
   *  (v0.9.1, app-audit añadido 2). Optional: absent in harnesses. */
  onOpenBuilder?: () => void
  fetcher?: typeof fetch
  pollIntervalMs?: number
}

export function ChannelWizard({
  existingTypes,
  onClose,
  onDone,
  onOpenBuilder,
  fetcher,
  pollIntervalMs,
}: WizardProps): React.JSX.Element {
  const [step, setStep] = useState(1)
  const [typeId, setTypeId] = useState('')
  // The webhook card never advances the token flow (its shape — bind, path,
  // outbound_url — is Builder territory until the v0.10 wizard step): picking
  // it shows the breadcrumb instead.
  const [webhookPicked, setWebhookPicked] = useState(false)
  const [secret, setSecret] = useState('')
  const [ks, setKs] = useState<KsState>('idle')
  const [pre, setPre] = useState<PreState>('idle')
  const [preDetail, setPreDetail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const chosen = CHANNEL_TYPES.find((t) => t.id === typeId) ?? null

  const saveSecret = (): void => {
    if (!chosen || secret === '' || ks === 'saving') return
    const d = desktop()
    if (!d) {
      // No bindings (plain browser / the e2e harness installs its own): the
      // keychain step cannot store anything, so stay honest — the flow does
      // not claim a secret was persisted. In the packaged Wails app the
      // binding is always present; this path is dev/harness-only.
      setSecret('')
      setError('Sin llavero en este entorno — exporta la variable en tu terminal.')
      return
    }
    setKs('saving')
    setError('')
    d.SetSecret(chosen.tokenEnv, secret)
      .then(() => {
        setKs('done')
        setSecret('') // drop the value from state the instant it is stored
      })
      .catch(() => {
        // Generic message: the OS keyring backend's error strings are outside
        // the audited boundary — never render them (project rule: nothing
        // secret-adjacent in an error message). Detail is logged Go-side.
        setKs('idle')
        setError(
          'No se pudo guardar en el llavero. Inténtalo de nuevo o exporta la variable en tu terminal.',
        )
      })
  }

  const checkEnv = (): void => {
    const d = desktop()
    if (!chosen || !d) return
    setPre('checking')
    d.CheckSecretPresence(chosen.tokenEnv)
      .then((p) => {
        if (p.inEnv || p.inKeychain) {
          setPre('ok')
          setPreDetail(
            p.inEnv ? `${chosen.tokenEnv} detectada · entorno` : `${chosen.tokenEnv} en el llavero`,
          )
        } else {
          setPre('fail')
        }
      })
      .catch(() => setPre('fail'))
  }

  const finish = (): void => {
    if (!chosen || submitting) return
    setSubmitting(true)
    setError('')
    const ch: ChannelConfig = { type: chosen.id, mode: chosen.mode, token_env: chosen.tokenEnv }
    void mutateConfig((c) => addChannel(c, ch), mutateOpts(fetcher, pollIntervalMs))
      .then((res) => {
        if (res.ok) onDone()
        else setError(`No se pudo conectar el canal (${res.state}).`)
      })
      .finally(() => setSubmitting(false))
  }

  const next = (): void => {
    if (step < 3) setStep((s) => s + 1)
  }
  const back = (): void => {
    if (step > 1) setStep((s) => s - 1)
  }
  const canNext = step === 1 ? chosen !== null : true

  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label="Añadir un canal">
      <div className="modal modal-wide" data-testid="channel-wizard">
        <WizardSteps step={step} labels={['Canal', 'Preparar', 'El token']} />

        <div className="wiz-body">
          {step === 1 && (
            <>
              <div className="wiz-heading">¿Qué canal quieres conectar?</div>
              <div className="wiz-caption">Podrás añadir más cuando quieras.</div>
              <div className="wiz-type-list">
                {CHANNEL_TYPES.map((t) => {
                  const taken = existingTypes.includes(t.id)
                  return (
                    <button
                      key={t.id}
                      type="button"
                      className={`wiz-type ${typeId === t.id ? 'wiz-type-on' : ''}`}
                      disabled={taken}
                      onClick={() => {
                        setWebhookPicked(false)
                        setTypeId(t.id)
                      }}
                    >
                      <span className="chan-tile" aria-hidden="true">
                        {channelGlyph(t.id)}
                      </span>
                      <span className="wiz-type-body">
                        <span className="wiz-type-title">{t.label}</span>
                        <span className="wiz-type-blurb">
                          {taken ? 'ya está conectado — un canal por tipo' : t.blurb}
                        </span>
                      </span>
                    </button>
                  )
                })}
                {/* The webhook breadcrumb (v0.9.1, app-audit añadido 2): the
                    channel exists since ADR-0038, but with no card here a
                    user concluded it did not exist at all. */}
                <button
                  type="button"
                  className={`wiz-type ${webhookPicked ? 'wiz-type-on' : ''}`}
                  disabled={existingTypes.includes('webhook')}
                  onClick={() => {
                    setTypeId('')
                    setWebhookPicked(true)
                  }}
                >
                  <span className="chan-tile" aria-hidden="true">
                    {channelGlyph('webhook')}
                  </span>
                  <span className="wiz-type-body">
                    <span className="wiz-type-title">Webhook</span>
                    <span className="wiz-type-blurb">
                      {existingTypes.includes('webhook')
                        ? 'ya está conectado — un canal por tipo'
                        : 'endpoint HTTP genérico · se monta en el Builder'}
                    </span>
                  </span>
                </button>
              </div>
              {webhookPicked && (
                <div className="wiz-webhook-note" role="note">
                  <p className="wiz-caption">
                    El canal webhook se configura en el Builder: suelta un bloque de canal en el
                    lienzo y cámbiale el tipo a <span className="mono">webhook</span> en su panel.
                    Ahí viven sus campos (<span className="mono">bind</span>,{' '}
                    <span className="mono">path</span>, <span className="mono">outbound_url</span>).
                  </p>
                  <button
                    type="button"
                    className="btn primary"
                    onClick={() => {
                      onOpenBuilder?.()
                      onClose()
                    }}
                  >
                    Ir al Builder
                  </button>
                </div>
              )}
            </>
          )}

          {step === 2 && chosen !== null && (
            <>
              <div className="wiz-heading">El bot, por su nombre</div>
              <div className="wiz-caption">
                Korvun guardará el nombre de la variable; el valor lo pones en el paso 3.
              </div>
              <div className="chan-field">
                <span className="chan-field-label mono">NOMBRE DE LA VARIABLE DE ENTORNO</span>
                <div className="chan-chip">
                  <span className="mono">{chosen.tokenEnv}</span>
                  <span className="chan-spacer" />
                  <span className="chan-only-name">sugerido</span>
                </div>
              </div>
              <div className="chan-field">
                <span className="chan-field-label mono">MODO</span>
                <div className="chan-chip mono">{chosen.mode} · determinado por el tipo</div>
              </div>
            </>
          )}

          {step === 3 && chosen !== null && (
            <>
              <div className="wiz-heading">El token, por su nombre</div>
              <div className="wiz-caption">
                Korvun guarda solo el nombre de la variable — el valor queda en tu entorno o tu
                llavero.
              </div>
              <div className="chan-field">
                <span className="chan-field-label mono">VALOR DEL TOKEN</span>
                {ks !== 'done' ? (
                  <div className="wiz-secret-row">
                    <input
                      type="password"
                      className="wiz-secret-input mono"
                      placeholder="Pega aquí el token (una sola vez)"
                      value={secret}
                      onChange={(e) => setSecret(e.target.value)}
                      autoComplete="off"
                    />
                    <button
                      type="button"
                      className="btn-primary btn-primary-sm"
                      onClick={saveSecret}
                      disabled={secret === '' || ks === 'saving'}
                    >
                      {ks === 'saving' ? 'Guardando…' : 'Guardar en el llavero'}
                    </button>
                  </div>
                ) : (
                  <div className="wiz-secret-done">
                    <span className="mono">guardado en el llavero del sistema</span>
                    <span className="chan-spacer" />
                    <button
                      type="button"
                      className="chan-link"
                      onClick={() => {
                        setKs('idle')
                        setPre('idle')
                      }}
                    >
                      Sustituir…
                    </button>
                  </div>
                )}
                <div className="chan-secret-note">
                  Se guarda en el llavero de tu sistema (Keychain / Credential Manager) — nunca en
                  Korvun, sus archivos ni sus logs.
                </div>
              </div>

              <details className="wiz-terminal">
                <summary>¿Usas terminal?</summary>
                <div className="wiz-terminal-body mono">
                  <div>$ export {chosen.tokenEnv}=···</div>
                  <div>$ korvun config check --preflight</div>
                  <div className="wiz-terminal-note">tu entorno siempre gana sobre el llavero</div>
                </div>
              </details>

              <div className="wiz-precheck">
                {pre === 'idle' && (
                  <>
                    <button type="button" className="btn-secondary btn-sm" onClick={checkEnv}>
                      Comprobar entorno
                    </button>
                    <span className="wiz-precheck-hint">
                      resuelve la variable sin leer su valor
                    </span>
                  </>
                )}
                {pre === 'checking' && <span className="wiz-precheck-hint">Comprobando…</span>}
                {pre === 'ok' && <span className="wiz-precheck-ok mono">✓ {preDetail}</span>}
                {pre === 'fail' && (
                  <span className="wiz-precheck-fail">
                    Aún no aparece — guárdala en el llavero o expórtala en tu terminal.
                  </span>
                )}
              </div>
            </>
          )}

          {error !== '' && (
            <div className="hero-strip-error" role="alert">
              {error}
            </div>
          )}
        </div>

        <div className="wiz-foot">
          <button type="button" className="chan-link" onClick={onClose}>
            Cancelar
          </button>
          <span className="chan-spacer" />
          {step > 1 && (
            <button type="button" className="btn-secondary btn-sm" onClick={back}>
              Atrás
            </button>
          )}
          {step < 3 ? (
            <button
              type="button"
              className="btn-primary btn-primary-sm"
              onClick={next}
              disabled={!canNext}
            >
              Siguiente
            </button>
          ) : (
            <button
              type="button"
              className="btn-primary btn-primary-sm"
              onClick={finish}
              disabled={submitting}
            >
              {submitting ? 'Conectando…' : 'Conectar canal'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
