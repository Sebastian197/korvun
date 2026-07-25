// First-run onboarding (SP6c, contra recien-instalado; ADR-0035 §5): three
// steps over EnsureDefaultConfig's created=true — model check (CheckOllama,
// honest reachability with retry; the "pick provider" half is deferred to
// builder editing, the template being ollama-based) → first channel (reuses
// the assistant) → arranque. On completion the chrome shows normal Home.
//
// DESIGN RESOLUTION (SP6c review, flagged for the copilot): the assistant's
// mutation pipe (POST /api/config + reload) needs a RUNNING core, but the
// design's step order is model → channel → start. To make the flow actually
// work on a real fresh install (the core is stopped until the user starts it),
// connecting a channel FIRST boots the ollama-only template (which boots
// channel-less — proven by TestFirstRun_templateBoots), so the assistant runs
// against a live core exactly like Canales. The channel is OPTIONAL: you can
// skip it and add one later from Canales. Step 3 then adapts honestly — the
// core is already up if you connected a channel ("Entrar a Korvun"), or it
// boots the template now if you skipped ("Arrancar Korvun").
import { useState } from 'react'
import { BrandMark } from '../components/BrandMark'
import { IconPlay, IconWarning } from '../components/icons'
import { WizardSteps } from '../components/WizardSteps'
import { desktop } from '../lib/go'
import { useCoreState } from '../status/store'
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
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const core = useCoreState()
  const running = core === 'running'

  const checkModel = (): void => {
    const d = desktop()
    if (!d) {
      setChk('fail')
      return
    }
    setChk('checking')
    d.CheckOllama(OLLAMA_BASE)
      .then((r) => setChk(r.reachable ? 'ok' : 'fail'))
      .catch(() => setChk('fail'))
  }

  // Boot the template core so the assistant's POST/reload pipe has a live
  // target. Idempotent: an already-running core (ErrLifecycleBusy / already
  // running) is success. Opens the wizard only once the core is up.
  const openWizard = (): void => {
    const d = desktop()
    if (busy) return
    if (!d) {
      setWizardOpen(true)
      return
    }
    setBusy(true)
    setError('')
    d.Start()
      .then(() => setWizardOpen(true))
      .catch((e: unknown) => {
        // Already running is fine — the core is live, open the wizard.
        const msg = e instanceof Error ? e.message : String(e)
        if (msg.includes('already running')) setWizardOpen(true)
        else setError('No se pudo arrancar el núcleo para conectar el canal: ' + msg)
      })
      .finally(() => setBusy(false))
  }

  // Step 3: enter if the core is already up (a channel was connected), else
  // boot the template now.
  const finish = (): void => {
    const d = desktop()
    if (busy) return
    if (!d || running) {
      onFinished()
      return
    }
    setBusy(true)
    setError('')
    d.Start()
      .then(() => onFinished())
      .catch((e: unknown) => {
        const msg = e instanceof Error ? e.message : String(e)
        if (msg.includes('already running')) onFinished()
        else {
          setError(msg)
          setBusy(false)
        }
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

        <WizardSteps step={step} labels={['Modelo', 'Canal', 'Arranque']} />

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
                Un canal es por dónde te llegan y respondes los mensajes. Puedes conectarlo ahora o
                más tarde desde Canales.
              </div>
              {channelAdded ? (
                <div className="wiz-secret-done">
                  <span className="mono">canal conectado</span>
                </div>
              ) : (
                <button
                  type="button"
                  className="btn-primary btn-primary-sm"
                  onClick={openWizard}
                  disabled={busy}
                >
                  {busy ? 'Preparando…' : 'Conectar un canal'}
                </button>
              )}
              {error !== '' && (
                <div className="hero-strip-error" role="alert">
                  {error}
                </div>
              )}
            </>
          )}

          {step === 3 && (
            <>
              <div className="wiz-heading">
                {running ? 'Todo listo — entra a Korvun' : 'Todo listo — arranca el gateway'}
              </div>
              <div className="wiz-caption">
                Korvun {running ? 'ya está sirviendo' : 'empezará a servir'} tus canales y modelos.
                Podrás detenerlo cuando quieras.
              </div>
              <button type="button" className="btn-primary" onClick={finish} disabled={busy}>
                <IconPlay size={15} />
                {busy
                  ? running
                    ? 'Entrando…'
                    : 'Arrancando…'
                  : running
                    ? 'Entrar a Korvun'
                    : 'Arrancar Korvun'}
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
              // Step 1 gates on the model check; step 2's channel is OPTIONAL
              // (the template runs channel-less), so Siguiente is always open.
              disabled={step === 1 && chk !== 'ok'}
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
