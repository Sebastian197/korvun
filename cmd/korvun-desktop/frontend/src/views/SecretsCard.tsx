// B10 — the SECRETOS card in Ajustes (sealed design ola2-designs §2,
// APROBADO POR CHANO 2026-08-23 after two visual-finish rounds): a Settings
// card with in-row editing and confirmation, house-finish buttons, presence
// dots, a permanent values-are-never-shown note, the sealed keychain error,
// the sealed unreadable-config notice with [Abrir carpeta], and a row
// skeleton while discovery runs. Discovery: /api/config through the proxy
// when the core runs; the name-only ListSecretNames binding when stopped —
// the star case, a boot broken by an absent secret. THE INVARIANT: no value
// is ever shown, returned, logged, or carried by any surface here — the
// editor field is write-only and empties the instant SetSecret fires.
import { useCallback, useEffect, useState } from 'react'
import { desktop } from '../lib/go'
import { secretNamesOfConfig } from '../settings/secrets'
import type { CoreState } from '../status/store'

export interface SecretsCardProps {
  coreState: CoreState
  fetcher?: typeof fetch
}

/** Sealed keychain-failure copy (ola2-designs §2). The raw error NEVER
 * rides along — it could carry paths, and the message must stay fixed. */
const KEYCHAIN_ERROR =
  'El llavero del sistema rechazó la operación. Reintenta; si persiste, desbloquéalo en Acceso a Llaveros.'

type Presence =
  | { kind: 'checking' }
  | { kind: 'env-wins' }
  | { kind: 'keychain' }
  | { kind: 'env' }
  | { kind: 'absent' }
  | { kind: 'uncheckable' }

function presenceLabel(p: Presence): { dot: string; text: string } {
  switch (p.kind) {
    case 'checking':
      return { dot: '', text: '…' }
    case 'env-wins':
      return { dot: 'dot-ok', text: 'en el entorno · el entorno gana al llavero' }
    case 'env':
      return { dot: 'dot-ok', text: 'en el entorno' }
    case 'keychain':
      return { dot: 'dot-ok', text: 'en el llavero' }
    case 'absent':
      return { dot: 'dot-off', text: 'ausente' }
    case 'uncheckable':
      return { dot: 'dot-off', text: 'no comprobable' }
  }
}

function SecretRow({ name }: { name: string }): React.JSX.Element {
  const [presence, setPresence] = useState<Presence>({ kind: 'checking' })
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<'save' | 'delete' | null>(null)
  const [busy, setBusy] = useState(false)

  const check = useCallback((): void => {
    const d = desktop()
    if (!d) {
      setPresence({ kind: 'uncheckable' })
      return
    }
    d.CheckSecretPresence(name)
      .then((p) => {
        if (p.inEnv && p.inKeychain) setPresence({ kind: 'env-wins' })
        else if (p.inEnv) setPresence({ kind: 'env' })
        else if (p.inKeychain) setPresence({ kind: 'keychain' })
        else setPresence({ kind: 'absent' })
      })
      .catch(() => setPresence({ kind: 'uncheckable' }))
  }, [name])

  useEffect(() => {
    check()
  }, [check])

  const save = (): void => {
    const d = desktop()
    if (!d || busy) return
    const v = value
    // Write-only discipline (the wizard's): the field empties the INSTANT
    // of the call — the value lives nowhere in this component afterward.
    setValue('')
    setBusy(true)
    setError(null)
    d.SetSecret(name, v)
      .then(() => {
        setEditing(false)
        check()
      })
      .catch(() => setError('save'))
      .finally(() => setBusy(false))
  }

  const doDelete = (): void => {
    const d = desktop()
    if (!d || busy) return
    setBusy(true)
    setError(null)
    setConfirming(false)
    d.DeleteSecret(name)
      .then(() => check())
      .catch(() => setError('delete'))
      .finally(() => setBusy(false))
  }

  const pl = presenceLabel(presence)
  const inKeychain = presence.kind === 'keychain' || presence.kind === 'env-wins'
  return (
    <div className="set-row" data-testid="secret-row">
      <div className="set-row-body">
        <div className="set-row-title mono">{name}</div>
        <div className="set-row-caption">
          {pl.dot !== '' && <span className={pl.dot} aria-hidden="true" />} {pl.text}
        </div>
        {error !== null && (
          <div className="secret-error" role="alert">
            {KEYCHAIN_ERROR}{' '}
            <button
              type="button"
              className="btn-small"
              onClick={() => (error === 'save' ? setEditing(true) : setConfirming(true))}
            >
              Reintentar
            </button>
          </div>
        )}
      </div>
      {editing ? (
        <span className="secret-editor">
          <input
            type="password"
            className="secret-input"
            aria-label={`valor de ${name}`}
            value={value}
            autoComplete="off"
            onChange={(e) => setValue(e.target.value)}
          />
          <button type="button" className="btn-small btn-accent" onClick={save} disabled={busy}>
            Guardar
          </button>
          <button
            type="button"
            className="btn-small"
            onClick={() => {
              setValue('')
              setEditing(false)
            }}
          >
            Cancelar
          </button>
        </span>
      ) : confirming ? (
        <span className="secret-confirm">
          ¿Eliminar del llavero?
          <button type="button" className="btn-small btn-danger" onClick={doDelete}>
            Sí, eliminar
          </button>
          <button type="button" className="btn-small" onClick={() => setConfirming(false)}>
            Cancelar
          </button>
        </span>
      ) : (
        <span className="secret-actions">
          <button
            type="button"
            className="btn-small"
            onClick={() => {
              setError(null)
              setEditing(true)
            }}
          >
            {inKeychain ? 'Actualizar…' : 'Guardar…'}
          </button>
          {inKeychain && (
            <button
              type="button"
              className="btn-small btn-danger"
              aria-label={`Eliminar del llavero ${name}`}
              onClick={() => {
                setError(null)
                setConfirming(true)
              }}
            >
              ×
            </button>
          )}
        </span>
      )}
    </div>
  )
}

type Discovery = { kind: 'loading' } | { kind: 'ready'; names: string[] } | { kind: 'unreadable' }

export function SecretsCard(props: SecretsCardProps): React.JSX.Element {
  const fetcher = props.fetcher ?? fetch
  const [state, setState] = useState<Discovery>({ kind: 'loading' })

  const discover = useCallback((): void => {
    setState({ kind: 'loading' })
    const viaBinding = (): void => {
      const d = desktop()
      if (!d?.ListSecretNames) {
        setState({ kind: 'unreadable' })
        return
      }
      d.ListSecretNames()
        .then((names) => setState({ kind: 'ready', names }))
        .catch(() => setState({ kind: 'unreadable' }))
    }
    if (props.coreState === 'running') {
      // Live config through the proxy; the binding is the fallback.
      fetcher('/api/config', { cache: 'no-store' })
        .then(async (r) => {
          if (!r.ok) throw new Error(`config ${r.status}`)
          setState({ kind: 'ready', names: secretNamesOfConfig(await r.json()) })
        })
        .catch(viaBinding)
      return
    }
    viaBinding()
  }, [fetcher, props.coreState])

  useEffect(() => {
    discover()
  }, [discover])

  const openFolder = (): void => {
    void desktop()?.OpenConfigFolder?.()
  }

  return (
    <section className="set-card">
      <div className="set-card-head">SECRETOS</div>
      {state.kind === 'loading' && (
        <div data-testid="secrets-skeleton" className="secrets-skeleton" aria-hidden="true">
          <div className="skeleton-row" />
          <div className="skeleton-row" />
        </div>
      )}
      {state.kind === 'unreadable' && (
        <div className="set-row">
          <div className="set-row-body">
            <div className="secret-error" role="alert">
              No se pudo leer korvun.json — revísalo a mano y reintenta.
            </div>
          </div>
          <button type="button" className="btn-small" onClick={openFolder}>
            Abrir carpeta
          </button>
          <button type="button" className="btn-small" onClick={discover}>
            Reintentar
          </button>
        </div>
      )}
      {state.kind === 'ready' &&
        (state.names.length === 0 ? (
          <div className="set-row">
            <div className="set-row-body">
              <div className="set-row-caption">
                Tu configuración no referencia ningún secreto todavía. Las filas aparecen al
                configurar canales o modelos que usen claves.
              </div>
            </div>
          </div>
        ) : (
          state.names.map((n) => <SecretRow key={n} name={n} />)
        ))}
      <p className="set-row-caption secrets-note">
        Los valores nunca se muestran. Korvun guarda solo nombres; el valor vive en el llavero del
        sistema.
      </p>
    </section>
  )
}
