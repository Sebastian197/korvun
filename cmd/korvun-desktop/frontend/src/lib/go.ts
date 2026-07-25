// Typed access to the Wails-bound Desktop surface (internal/shell.Desktop).
// Inside the real window, Wails injects window.go.shell.Desktop; in the e2e
// harness (a plain browser) it is absent and every accessor degrades honestly
// — the chrome renders, actions that need the shell are inert.
export interface DesktopBindings {
  Start(): Promise<void>
  Stop(): Promise<void>
  LoadConfig(path: string): Promise<void>
  DefaultConfigPath(): Promise<string>
  EnsureDefaultConfig(): Promise<boolean>
  SetSecret(name: string, value: string): Promise<void>
  DeleteSecret(name: string): Promise<void>
  CheckOllama(baseURL: string): Promise<{ reachable: boolean; detail: string }>
  Version(): Promise<string>
}

interface WailsWindow {
  go?: { shell?: { Desktop?: DesktopBindings } }
}

/** The bound surface, or undefined outside the Wails window. */
export function desktop(): DesktopBindings | undefined {
  return (window as unknown as WailsWindow).go?.shell?.Desktop
}
