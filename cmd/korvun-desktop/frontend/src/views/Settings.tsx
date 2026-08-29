// Ajustes v1 (FR-WIN-5, contra final-3): apariencia (tema), gateway (config
// path, autostart, effective admin address + Copy), seguridad (token row —
// env-var NAME only, never a value). Rows without an API behind them are NOT
// rendered (FR-WIN-4: a dead button is a lie) — no Datos section, no
// "Mostrar en carpeta".
import { useEffect, useRef, useState } from 'react'
import { isAutostartEnabled, setAutostart } from '../autostart'
import { IconCopy } from '../components/icons'
import { useShellStatus } from '../status/shell'
import { useCoreState } from '../status/store'
import { applyTheme, storedTheme, type ThemeChoice } from '../theme'
import { SecretsCard } from './SecretsCard'

const THEME_OPTIONS: ReadonlyArray<{ id: ThemeChoice; label: string }> = [
  { id: 'dark', label: 'Oscuro' },
  { id: 'light', label: 'Claro' },
  { id: 'system', label: 'Sistema' },
]

function ThemeRow(): React.JSX.Element {
  const [choice, setChoice] = useState<ThemeChoice>(() => storedTheme())
  const pick = (t: ThemeChoice): void => {
    setChoice(t)
    applyTheme(t)
  }
  return (
    <div className="set-row">
      <div className="set-row-body">
        <div className="set-row-title">Tema</div>
        <div className="set-row-caption">el violeta y el teal rinden mejor en oscuro</div>
      </div>
      <div className="segmented" role="group" aria-label="Tema">
        {THEME_OPTIONS.map((o) => (
          <button
            key={o.id}
            type="button"
            className={choice === o.id ? 'seg-on' : 'seg-off'}
            aria-pressed={choice === o.id}
            onClick={() => pick(o.id)}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  )
}

function AutostartRow(): React.JSX.Element {
  const [on, setOn] = useState(() => isAutostartEnabled())
  const flip = (): void => {
    const next = !on
    setOn(next)
    setAutostart(next)
  }
  return (
    <div className="set-row">
      <div className="set-row-body">
        <div className="set-row-title" id="autostart-label">
          Iniciar con la aplicación
        </div>
        <div className="set-row-caption">arranca el gateway al abrir Korvun Desktop</div>
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={on}
        aria-labelledby="autostart-label"
        className={`toggle ${on ? 'toggle-on' : ''}`}
        onClick={flip}
      >
        <span className="toggle-knob" />
      </button>
    </div>
  )
}

function CopyButton({ value, disabled }: { value: string; disabled: boolean }): React.JSX.Element {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  const copy = (): void => {
    void navigator.clipboard
      ?.writeText(value)
      .then(() => {
        setCopied(true)
        clearTimeout(timer.current)
        timer.current = setTimeout(() => setCopied(false), 1500)
      })
      .catch(() => undefined)
  }
  return (
    <button type="button" className="btn-small" onClick={copy} disabled={disabled}>
      <IconCopy size={11} />
      {copied ? 'Copiado' : 'Copiar'}
    </button>
  )
}

export function Settings({ version }: { version: string }): React.JSX.Element {
  const shell = useShellStatus()
  const core = useCoreState()
  const running = core === 'running'
  const configPath = shell?.ConfigPath ?? ''
  const adminAddr = running && shell !== null ? shell.AdminAddr : ''
  const tokenEnv = shell?.TokenEnv ?? ''

  return (
    <div className="settings" data-testid="ajustes">
      <section className="set-card">
        <div className="set-card-head">APARIENCIA</div>
        <ThemeRow />
      </section>

      <section className="set-card">
        <div className="set-card-head">GATEWAY</div>
        <div className="set-row">
          <div className="set-row-body">
            <div className="set-row-title">Fichero de configuración</div>
            <div className="set-row-caption mono truncate" title={configPath}>
              {configPath !== '' ? configPath : 'sin configuración cargada'}
            </div>
          </div>
        </div>
        <AutostartRow />
        <div className="set-row">
          <div className="set-row-body">
            <div className="set-row-title">Panel de administración</div>
            <div className="set-row-caption mono truncate">
              {adminAddr !== ''
                ? `${adminAddr} · asignado al arrancar`
                : '— · se asigna al arrancar'}
            </div>
          </div>
          <CopyButton value={adminAddr} disabled={adminAddr === ''} />
          <span className="pill pill-ok">solo local</span>
        </div>
      </section>

      <section className="set-card">
        <div className="set-card-head">SEGURIDAD</div>
        <div className="set-row">
          <div className="set-row-body">
            <div className="set-row-title">Token de administración</div>
            <div className="set-row-caption">
              generado automáticamente en cada arranque
              {tokenEnv !== '' ? (
                <>
                  {' · variable '}
                  <span className="mono">{tokenEnv}</span>
                </>
              ) : null}
            </div>
          </div>
          <span className="pill pill-ok">
            <span className="dot-ok" aria-hidden="true" />
            automático · se rota al arrancar
          </span>
        </div>
      </section>

      {/* B10 (sealed design ola2-designs §2): the Secrets card. */}
      <SecretsCard coreState={core} />

      <p className="set-footer mono">Korvun {version} · un solo binario · Apache-2.0</p>
    </div>
  )
}
