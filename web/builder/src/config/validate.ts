// Pre-Apply validation of one model block (B14, spec
// 2026-08-29-b14-model-panel-preapply-validation.md). FIRST LINE only: the
// server's config.Validate re-checks every POST and remains the final judge
// (ADR-0030 §4) — this module exists so the panel stops the 2026-08-23
// corruption shapes BEFORE a reload can apply them. Pure + DOM-free (the
// errors.ts precedent).

import type { ModelConfig } from './schema'

/** One field-scoped validation error for the model panel. */
export interface ModelFieldError {
  field: 'base_url' | 'api_key_env' | 'model_id'
  message: string
}

/** An env-var-NAME-shaped token: an ALL-CAPS run with at least one
 *  underscore (`OPENROUTER_API_KEY`, `MY_API_KEY`). Legitimate endpoint
 *  prefixes do not carry one; the 2026-08-23 corruption did — the secret's
 *  NAME glued into the URL by a stray gesture the panel never questioned. */
const ENV_NAME_RE = /[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+/

/** Parse an absolute http(s) URL with a host, or null. */
function parseHttpUrl(raw: string): URL | null {
  let u: URL
  try {
    u = new URL(raw)
  } catch {
    return null
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
  if (u.hostname === '') return null
  return u
}

/** Validate one model block for the panel (pure). Empty array = applyable.
 *  Deliberately NARROWER than config.Validate: only the shapes the panel
 *  must stop before an Apply can make them live (FR-B14-2..4). */
export function validateModel(m: ModelConfig): ModelFieldError[] {
  const errs: ModelFieldError[] = []
  const isCompat = m.provider === 'openai-compatible'
  const base = (m.base_url ?? '').trim()
  if (base === '') {
    if (isCompat) {
      errs.push({
        field: 'base_url',
        message: 'required for openai-compatible — the full endpoint prefix, e.g. https://host/v1',
      })
    }
  } else if (parseHttpUrl(base) === null) {
    errs.push({
      field: 'base_url',
      message: 'must be an absolute http(s) URL, e.g. https://host/v1',
    })
  } else {
    const glued = base.match(ENV_NAME_RE)
    if (glued) {
      errs.push({
        field: 'base_url',
        message: `"${glued[0]}" looks like a secret env var name glued into the URL — the key name belongs in api_key_env, never in the URL`,
      })
    }
  }
  // Cloud-without-key (FR-B14-4): groq mirrors the core rule; compat+cloud
  // is the panel's own first line — the core stays permissive (ADR-0044),
  // but a cloud endpoint without a key is the mute console of 2026-08-23.
  const needsKey = m.provider === 'groq' || (isCompat && m.locality === 'cloud')
  if (needsKey && (m.api_key_env ?? '').trim() === '') {
    errs.push({
      field: 'api_key_env',
      message: 'required for a cloud model — the NAME of the env var holding the API key',
    })
  }
  return errs
}
