// Pure classification of a failed POST /api/config, so the UI shows the RIGHT
// treatment for each failure the design review flagged (ADR-0030 §5, 2b.2c). The
// server bodies are stable (2a): 400 → {"error": "<field-path>: msg"}; 409 →
// {"error_code": "config_would_self_lock" | "reload_in_progress", "message": ...};
// 401 → unauthorized. Pure + Vitest-tested so the mapping cannot silently regress.

export type SaveError =
  | { kind: 'validation'; field: string | null; message: string }
  | { kind: 'selfLock'; message: string }
  | { kind: 'reloadInProgress'; message: string }
  | { kind: 'unauthorized' }
  | { kind: 'other'; status: number; message: string }

function safeJson(body: string): Record<string, unknown> | null {
  try {
    return JSON.parse(body) as Record<string, unknown>
  } catch {
    return null
  }
}

// The leading config-path token in a validate error (mirrors config.Validate paths:
// `brains[0].sensitivity`, `channels[0].token_env`, `admin.token_env`, `routes[0].brain`,
// `brains[0].models[1].provider`).
const FIELD_RE = /(?:channels|brains|routes)(?:\[\d+\])?(?:\.[a-z_]+(?:\[\d+\])?)*|admin\.[a-z_]+/

export function extractField(message: string): string | null {
  const m = message.match(FIELD_RE)
  return m ? m[0] : null
}

export function parseSaveError(status: number, body: string): SaveError {
  if (status === 401) return { kind: 'unauthorized' }
  const j = safeJson(body)
  if (status === 400) {
    const message = (j?.error as string | undefined) ?? body ?? 'invalid config'
    return { kind: 'validation', field: extractField(message), message }
  }
  if (status === 409) {
    const code = j?.error_code as string | undefined
    const message = (j?.message as string | undefined) ?? body
    if (code === 'config_would_self_lock') return { kind: 'selfLock', message }
    if (code === 'reload_in_progress') return { kind: 'reloadInProgress', message }
  }
  const message =
    (j?.error as string | undefined) ?? (j?.message as string | undefined) ?? body ?? `HTTP ${status}`
  return { kind: 'other', status, message }
}
