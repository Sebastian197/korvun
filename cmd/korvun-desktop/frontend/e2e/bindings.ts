// Playwright shim for the shell.Desktop bindings: installs
// window.go.shell.Desktop over the harness's /__test/bindings bridge, so the
// chrome exercises the SAME surface the native Wails window binds (Go error
// → Promise rejection, exactly like the generated bindings).
import type { Page } from '@playwright/test'

export async function installBindings(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const call = async (method: string, args: unknown[] = []): Promise<unknown> => {
      const resp = await fetch(`/__test/bindings/${method}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(args),
      })
      const body = (await resp.json()) as { result?: unknown; error?: string }
      if (body.error !== undefined) throw new Error(body.error)
      return body.result
    }
    ;(window as unknown as { go: unknown }).go = {
      shell: {
        Desktop: {
          Start: () => call('Start'),
          Stop: () => call('Stop'),
          Status: () => call('Status'),
          Version: () => call('Version'),
          LoadConfig: (p: string) => call('LoadConfig', [p]),
          DefaultConfigPath: () => call('DefaultConfigPath'),
          EnsureDefaultConfig: () => call('EnsureDefaultConfig'),
          SetSecret: (n: string, v: string) => call('SetSecret', [n, v]),
          DeleteSecret: (n: string) => call('DeleteSecret', [n]),
          CheckOllama: (b: string) => call('CheckOllama', [b]),
          CheckSecretPresence: (n: string) => call('CheckSecretPresence', [n]),
        },
      },
    }
  })
}
