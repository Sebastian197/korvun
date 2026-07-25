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
import { desktop } from '../lib/go'
import { addChannel, mutateConfig, type ChannelConfig, type MutateOptions } from '../config/mutate'

interface ChannelType {
  id: string
  label: string
  mode: string
  tokenEnv: string
  blurb: string
  hasSecret: boolean
}

const TYPES: ChannelType[] = [
  {
    id: 'telegram',
    label: 'Telegram',
    mode: 'polling',
    tokenEnv: 'TELEGRAM_TOKEN',
    blurb: 'bot con @BotFather · polling',
    hasSecret: true,
  },
  {
    id: 'discord',
    label: 'Discord',
    mode: 'gateway',
    tokenEnv: 'DISCORD_BOT_TOKEN',
    blurb: 'bot del Portal de desarrolladores · gateway',
    hasSecret: true,
  },
]

type KsState = 'idle' | 'saving' | 'done'
type PreState = 'idle' | 'checking' | 'ok' | 'fail'

interface WizardProps {
  existingTypes: string[]
  onClose: () => void
  onDone: () => void
  fetcher?: typeof fetch
  pollIntervalMs?: number
}

export function ChannelWizard({
  existingTypes,
  onClose,
  onDone,
  fetcher,
  pollIntervalMs,
}: WizardProps): React.JSX.Element {
  const [step, setStep] = useState(1)
  const [typeId, setTypeId] = useState('')
  const [secret, setSecret] = useState('')
  const [ks, setKs] = useState<KsState>('idle')
  const [pre, setPre] = useState<PreState>('idle')
  const [preDetail, setPreDetail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const chosen = TYPES.find((t) => t.id === typeId) ?? null

  const saveSecret = (): void => {
    const d = desktop()
    if (!chosen || secret === '' || ks === 'saving') return
    if (!d) {
      // No bindings (plain browser): the keychain step is inert but the flow
      // proceeds — the harness bridge provides SetSecret in e2e.
      setKs('done')
      setSecret('')
      return
    }
    setKs('saving')
    setError('')
    d.SetSecret(chosen.tokenEnv, secret)
      .then(() => {
        setKs('done')
        setSecret('') // drop the value from state the instant it is stored
      })
      .catch((e: unknown) => {
        setKs('idle')
        setError(e instanceof Error ? e.message : String(e))
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
    const opts: MutateOptions = {}
    if (fetcher !== undefined) opts.fetcher = fetcher
    if (pollIntervalMs !== undefined) opts.pollIntervalMs = pollIntervalMs
    void mutateConfig((c) => addChannel(c, ch), opts)
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
        <div className="wiz-steps">
          <span className={`wiz-step ${step === 1 ? 'wiz-step-on' : ''}`}>1 · Canal</span>
          <span className="wiz-sep" />
          <span className={`wiz-step ${step === 2 ? 'wiz-step-on' : ''}`}>2 · Preparar</span>
          <span className="wiz-sep" />
          <span className={`wiz-step ${step === 3 ? 'wiz-step-on' : ''}`}>3 · El token</span>
        </div>

        <div className="wiz-body">
          {step === 1 && (
            <>
              <div className="wiz-heading">¿Qué canal quieres conectar?</div>
              <div className="wiz-caption">Podrás añadir más cuando quieras.</div>
              <div className="wiz-type-list">
                {TYPES.map((t) => {
                  const taken = existingTypes.includes(t.id)
                  return (
                    <button
                      key={t.id}
                      type="button"
                      className={`wiz-type ${typeId === t.id ? 'wiz-type-on' : ''}`}
                      disabled={taken}
                      onClick={() => setTypeId(t.id)}
                    >
                      <span className="chan-tile" aria-hidden="true">
                        {t.label.slice(0, 2).toUpperCase()}
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
              </div>
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
