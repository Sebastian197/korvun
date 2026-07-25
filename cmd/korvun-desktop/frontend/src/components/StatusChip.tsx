// The sidebar-foot status chip (design 02-sidebar / final-3): state dot +
// label, the effective admin port while running, and the loaded config's
// file name. Data: the bindings' Status() via the shell store; without
// bindings (plain browser) it degrades to the core state alone.
import { useShellStatus } from '../status/shell'
import { useCoreState } from '../status/store'

function baseName(p: string): string {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
}

export function StatusChip(): React.JSX.Element {
  const shell = useShellStatus()
  const core = useCoreState()
  const running = core === 'running'

  let label = running ? 'En marcha' : 'Detenido'
  if (shell !== null && shell.ConfigPath === '') label = 'Sin configurar'
  const port =
    running && shell !== null && shell.AdminAddr !== ''
      ? `:${shell.AdminAddr.split(':').pop() ?? ''}`
      : '—'
  const configName = shell !== null && shell.ConfigPath !== '' ? baseName(shell.ConfigPath) : ''

  return (
    <div className="status-chip" data-testid="status-chip">
      <div className="status-chip-row">
        <span className={`chip-dot ${running ? 'chip-dot-ok' : ''}`} aria-hidden="true" />
        <span className="chip-label">{label}</span>
        <span className="chip-port mono">{port}</span>
      </div>
      {configName !== '' && (
        <div className="chip-config mono" title={shell?.ConfigPath}>
          {configName}
        </div>
      )}
    </div>
  )
}
