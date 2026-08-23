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
  /** Boot warmup — only valid for LOCAL models (config.Validate rejects
   *  warmup on cloud entries). The panel does not expose it; the reducer
   *  clears it when a provider/locality change makes it invalid (B6). */
  warmup?: boolean
}

export interface PolicyConfig {
  kind: string
  order?: string[]
}

// ToolGrantConfig mirrors config.ToolGrantConfig (ADR-0041 §1): one tri-state
// grant. mode is allow|shadow|deny; channels, when present, restricts the grant
// to those channels. Absence of a grant for a listed tool is the ungoverned
// advertise+execute default — deny is NOT absence.
export interface ToolGrantConfig {
  tool: string
  mode: string
  channels?: string[]
}

// ToolAttrsConfig mirrors config.ToolAttrsConfig (ADR-0015 declared-not-inferred):
// a per-tool attribute OVERRIDE. An undefined field keeps the house default; the
// Go *bool round-trips as `boolean | undefined`.
export interface ToolAttrsConfig {
  sensitive?: boolean
  network?: boolean
}

// The three tool cages mirror config.{ReadFile,HTTPFetch,WebhookCall}ToolConfig
// (ADR-0041 §4): required when the tool is listed; the caps 0 => tool default,
// so they ride omitempty (undefined here).
export interface ReadFileToolConfig {
  root: string
  max_bytes?: number
}
export interface HTTPFetchToolConfig {
  allow_hosts: string[]
  max_bytes?: number
  max_redirects?: number
}
export interface WebhookCallToolConfig {
  allow_hosts: string[]
  max_bytes?: number
  timeout_seconds?: number
}

// AgentConfig mirrors config.AgentConfig (internal/config/config.go:342-408)
// EXACTLY. The governed-tools fields (ADR-0041) are all optional: an agent block
// written before governance validates byte-for-byte as before (AS-4).
export interface AgentConfig {
  tools: string[]
  max_iterations: number
  system_prompt: string
  governance?: ToolGrantConfig[]
  tool_attrs?: Record<string, ToolAttrsConfig>
  read_file?: ReadFileToolConfig
  http_fetch?: HTTPFetchToolConfig
  webhook_call?: WebhookCallToolConfig
  skills_dir?: string
  skills_body_budget?: number
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

export const PROVIDERS = ['ollama', 'groq', 'openai-compatible'] as const
export const SENSITIVITIES = ['public', 'private'] as const
export const DISPATCHES = ['fanout', 'sequential'] as const
export const POLICY_KINDS = ['priority', 'consensus'] as const
export const LOCALITIES = ['local', 'cloud'] as const
export const CHANNEL_TYPES = ['telegram', 'discord', 'webhook'] as const
export const CHANNEL_MODES = ['polling'] as const

/** Per-type transport modes, mirroring config.Validate's type switch
 *  (config.go:444-452): telegram → polling, discord → gateway, webhook → NO
 *  mode ("webhook takes no mode", ADR-0038 §1 NC-1c). The flat CHANNEL_MODES
 *  above predates discord/webhook and remains only for the legacy form. */
export const CHANNEL_MODES_BY_TYPE: Record<(typeof CHANNEL_TYPES)[number], readonly string[]> = {
  telegram: ['polling'],
  discord: ['gateway'],
  webhook: [],
}

/** Providers that require an api_key_env (cloud). Mirrors config.Validate: groq. */
export const CLOUD_PROVIDERS = new Set<string>(['groq'])
