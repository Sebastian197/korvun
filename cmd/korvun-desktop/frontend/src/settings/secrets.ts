// B10 — the Secrets panel's pure half (spec
// 2026-08-29-b10-secrets-panel.md FR-B10-3): NAME discovery from a parsed
// config object, the TS mirror of the shell's collectSecretNames. Defensive
// on shape — any unexpected input yields fewer rows, never a crash. Names
// only, ever: nothing here can carry a value.

function str(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

/** Deduplicated, appearance-ordered secret NAMES a config references. */
export function secretNamesOfConfig(cfg: unknown): string[] {
  if (cfg === null || typeof cfg !== 'object') return []
  const c = cfg as {
    channels?: unknown
    brains?: unknown
    admin?: unknown
  }
  const seen = new Set<string>()
  const names: string[] = []
  const add = (n: string): void => {
    if (n !== '' && !seen.has(n)) {
      seen.add(n)
      names.push(n)
    }
  }
  if (Array.isArray(c.channels)) {
    for (const ch of c.channels as Array<{ token_env?: unknown; webhook?: unknown }>) {
      if (ch === null || typeof ch !== 'object') continue
      add(str(ch.token_env))
      const wh = ch.webhook as { outbound_token_env?: unknown } | undefined
      if (wh !== null && typeof wh === 'object') add(str(wh?.outbound_token_env))
    }
  }
  if (Array.isArray(c.brains)) {
    for (const b of c.brains as Array<{ models?: unknown }>) {
      if (b === null || typeof b !== 'object' || !Array.isArray(b.models)) continue
      for (const m of b.models as Array<{ api_key_env?: unknown }>) {
        if (m === null || typeof m !== 'object') continue
        add(str(m.api_key_env))
      }
    }
  }
  const admin = c.admin as { token_env?: unknown } | undefined
  if (admin !== null && typeof admin === 'object') add(str(admin?.token_env))
  return names
}
