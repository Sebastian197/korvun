// TypeScript mirror of internal/config (the Go `config.Config`). The builder edits
// a working copy of this shape and POSTs it whole (ADR-0030 §4).
//
// The enum constants below mirror `config.Validate`. THE SERVER IS THE SOURCE OF
// TRUTH — it re-validates every POST and returns 400 on any violation — so these are
// a UX convenience (populate dropdowns, cheap client checks), not the authority.
// Keep in sync with internal/config/config.go `Validate`; drift is caught by the
// server 400.

export interface ModelConfig {
  provider: string
  model_id: string
  locality: string
  base_url?: string
  api_key_env?: string
}

export interface PolicyConfig {
  kind: string
  order?: string[]
}

export interface AgentConfig {
  tools: string[]
  max_iterations: number
  system_prompt: string
}

// PersonaConfig mirrors config.PersonaConfig (builder-canvas SP1, NC-4): the
// optional per-brain personality, all free text. display_name is presentation
// only — name stays the routing key. Rune caps (80/200/60/4000) are enforced
// by the server (validatePersona); the 400 field path targets the inline form.
export interface PersonaConfig {
  display_name?: string
  tone?: string
  language?: string
  instructions?: string
}

export interface BrainConfig {
  name: string
  sensitivity: string
  policy: PolicyConfig
  dispatch: string
  models: ModelConfig[]
  agent?: AgentConfig
  persona?: PersonaConfig
}

// WebhookMapping mirrors config.WebhookMapping (ADR-0038 §1): which inbound
// JSON fields map to Envelope fields. Every field optional; defaults resolve
// SERVER-side (EffectiveMapping) — the mirror never inflates the file.
export interface WebhookMapping {
  sender_id?: string
  sender_name?: string
  text?: string
  media_url?: string
  media_type?: string
  conversation_id?: string
}

// WebhookConfig mirrors config.WebhookConfig (ADR-0038 §1): the nested block a
// "webhook" channel requires (and only that type may carry — server 400
// otherwise). bind/path default server-side; outbound_token_env is an env-var
// NAME, never a value (ADR-0010).
export interface WebhookConfig {
  bind?: string
  path?: string
  outbound_url?: string
  outbound_token_env?: string
  mapping?: WebhookMapping
}

export interface ChannelConfig {
  type: string
  mode: string
  token_env: string
  webhook?: WebhookConfig
}

export interface RouteConfig {
  channel: string
  brain: string
}

export interface StorageConfig {
  path: string
}

export interface ObservabilityConfig {
  enabled?: boolean
  addr?: string
}

export interface AdminConfig {
  token_env: string
}

export interface Config {
  channels: ChannelConfig[]
  brains: BrainConfig[]
  routes: RouteConfig[]
  storage?: StorageConfig
  observability?: ObservabilityConfig
  admin?: AdminConfig
}

// ---- enums (mirror config.Validate; server 400 is the backstop) --------------

export const PROVIDERS = ['ollama', 'groq'] as const
export const SENSITIVITIES = ['public', 'private'] as const
export const DISPATCHES = ['fanout', 'sequential'] as const
export const POLICY_KINDS = ['priority', 'consensus'] as const
export const LOCALITIES = ['local', 'cloud'] as const
export const CHANNEL_TYPES = ['telegram', 'discord', 'webhook'] as const
export const CHANNEL_MODES = ['polling'] as const

/** Providers that require an api_key_env (cloud). Mirrors config.Validate: groq. */
export const CLOUD_PROVIDERS = new Set<string>(['groq'])
