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

export interface BrainConfig {
  name: string
  sensitivity: string
  policy: PolicyConfig
  dispatch: string
  models: ModelConfig[]
  agent?: AgentConfig
}

export interface ChannelConfig {
  type: string
  mode: string
  token_env: string
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
export const CHANNEL_TYPES = ['telegram'] as const
export const CHANNEL_MODES = ['polling'] as const

/** Providers that require an api_key_env (cloud). Mirrors config.Validate: groq. */
export const CLOUD_PROVIDERS = new Set<string>(['groq'])
