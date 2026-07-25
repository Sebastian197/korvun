// First-run onboarding (SP6c, contra recien-instalado; ADR-0035 §5): three
// steps over EnsureDefaultConfig's created=true — model check (CheckOllama,
// honest reachability with retry; the "pick provider" half is deferred to
// builder editing, the template being ollama-based) → first channel (reuses
// the wizard) → Start. On completion the chrome shows normal Home.
import { useState } from 'react'
import { BrandMark } from '../components/BrandMark'
import { IconPlay, IconWarning } from '../components/icons'
import { desktop } from '../lib/go'
import { ChannelWizard } from './ChannelWizard'

const OLLAMA_BASE = 'http://127.0.0.1:11434'

type ChkState = 'idle' | 'checking' | 'ok' | 'fail'

interface OnboardingProps {
  onFinished: () => void
  /** Test seam: jump straight to a step. */
  initialStep?: number
}

export function Onboarding({ onFinished, initialStep }: OnboardingProps): React.JSX.Element {
  const [step, setStep] = useState(initialStep ?? 1)
  const [chk, setChk] = useState<ChkState>('idle')
  const [wizardOpen, setWizardOpen] = useState(false)
  const [channelAdded, setChannelAdded] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')

  const checkModel = (): void => {
    const d = desktop()
    if (!d) {
      setChk('fail')
      return
    }
    setChk('checking')
    d.CheckOllama(OLLAMA_BASE)
      .then((r) => {
        if (r.reachable) {
          setChk('ok')
        } else {
          setChk('fail')
        }
      })
      .catch(() => setChk('fail'))
  }

  const start = (): void => {
    const d = desktop()
    if (starting) return
    setStarting(true)
    setError('')
    if (!d) {
      onFinished()
      return
    }
    d.Start()
      .then(() => onFinished())
      .catch((e: unknown) => {
        setError(e instanceof Error ? e.message : String(e))
        setStarting(false)
      })
  }

  return (
    <div className="onboarding" data-testid="onboarding">
      <div className="ob-card">
        <div className="ob-head">
          <span className="brand-tile ob-tile" aria-hidden="true">
            <BrandMark size={34} />
          </span>
          <div>
            <div className="ob-title">Bienvenido a Korvun</div>
            <div className="ob-caption">
              Tu gateway de IA, en tu propia máquina. Tres pasos y funcionando.
            </div>
          </div>
        </div>

        <div className="wiz-steps">
          <span className={`wiz-step ${step === 1 ? 'wiz-step-on' : ''}`}>1 · Modelo</span>
          <span className="wiz-sep" />
          <span className={`wiz-step ${step === 2 ? 'wiz-step-on' : ''}`}>2 · Canal</span>
          <span className="wiz-sep" />
          <span className={`wiz-step ${step === 3 ? 'wiz-step-on' : ''}`}>3 · Arranque</span>
        </div>

        <div className="ob-body">
          {step === 1 && (
            <>
              <div className="wiz-heading">Primero: ¿hay un modelo que responda?</div>
              <div className="wiz-caption">
                Korvun no trae modelos — usa los que ya tienes. Comprobemos que hay uno accesible
                antes de conectar nada.
              </div>
              <div className="ob-check">
                {chk === 'idle' && (
                  <button type="button" className="btn-primary btn-primary-sm" onClick={checkModel}>
                    Comprobar modelo
                  </button>
                )}
                {chk === 'checking' && (
                  <span className="wiz-precheck-hint">Comprobando {OLLAMA_BASE}…</span>
                )}
                {chk === 'ok' && (
                  <span className="wiz-precheck-ok mono">
                    ✓ ollama accesible · {OLLAMA_BASE} — listo
                  </span>
                )}
                {chk === 'fail' && (
                  <div className="ob-check-fail">
                    <IconWarning size={14} />
                    <span>Ollama no responde — arráncalo y reintenta.</span>
                    <button type="button" className="btn-secondary btn-sm" onClick={checkModel}>
                      Reintentar
                    </button>
                  </div>
                )}
              </div>
            </>
          )}

          {step === 2 && (
            <>
              <div className="wiz-heading">Conecta tu primer canal</div>
              <div className="wiz-caption">
                Un canal es por dónde te llegan y respondes los mensajes.
              </div>
              {channelAdded ? (
                <div className="wiz-secret-done">
                  <span className="mono">canal conectado</span>
                </div>
              ) : (
                <button
                  type="button"
                  className="btn-primary btn-primary-sm"
                  onClick={() => setWizardOpen(true)}
                >
                  Conectar un canal
                </button>
              )}
            </>
          )}

          {step === 3 && (
            <>
              <div className="wiz-heading">Todo listo — arranca el gateway</div>
              <div className="wiz-caption">
                Korvun empezará a servir tus canales y modelos. Podrás detenerlo cuando quieras.
              </div>
              <button type="button" className="btn-primary" onClick={start} disabled={starting}>
                <IconPlay size={15} />
                {starting ? 'Arrancando…' : 'Arrancar Korvun'}
              </button>
              {error !== '' && (
                <div className="hero-strip-error" role="alert">
                  {error}
                </div>
              )}
            </>
          )}
        </div>

        {step < 3 && (
          <div className="ob-foot">
            <span className="chan-spacer" />
            <button
              type="button"
              className="btn-primary btn-primary-sm"
              onClick={() => setStep((s) => s + 1)}
              disabled={step === 1 ? chk !== 'ok' : !channelAdded}
            >
              Siguiente
            </button>
          </div>
        )}
      </div>

      {wizardOpen && (
        <ChannelWizard
          existingTypes={[]}
          onClose={() => setWizardOpen(false)}
          onDone={() => {
            setWizardOpen(false)
            setChannelAdded(true)
          }}
        />
      )}
    </div>
  )
}
