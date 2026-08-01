// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package config parses and validates a Korvun deployment descriptor from a
// JSON file into a typed Config (ADR-0017 §1).
//
// The SCHEMA (the field shape below) is the one-way door: once an operator
// writes a config file, the field names and structure are a contract. The
// FORMAT is deliberately the standard library's encoding/json — zero new
// dependencies on the first binary's critical path. YAML is deferred to a
// later stage (ADR-0017 §1), reusing this same schema; when it lands it is a
// new decode path, not a new schema.
//
// Secrets are referenced by env-var NAME, never by value: a channel carries
// token_env and a cloud model carries api_key_env. The actual secret is
// resolved from the environment at boot (in internal/app), never read from
// this file (ADR-0010, ADR-0017 §1).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"
)

// DefaultRequestTimeout is the per-attempt provider timeout used when neither a
// per-model nor a top-level request_timeout is set (ADR-0031 Decision 3). It
// errs generous — the safe floor for a cold local model load — because a slow
// success beats a fast false-failure; cloud models are tightened per-model.
const DefaultRequestTimeout = 120 * time.Second

// Config is the root deployment descriptor: the channels to run, the brains to
// orchestrate, the routes binding the two, and optional durable storage.
type Config struct {
	// RequestTimeout is the top-level default per-attempt provider timeout, a
	// duration string (e.g. "120s"). A per-model ModelConfig.RequestTimeout
	// overrides it; when both are empty, DefaultRequestTimeout applies
	// (ADR-0031 Decision 3). Resolve the effective value via
	// EffectiveRequestTimeout.
	RequestTimeout string `json:"request_timeout,omitempty"`
	// BrainHandlerTimeout is the OPTIONAL explicit override of the router's
	// derived per-Handle ceiling, a duration string. It is honored only if it is
	// >= the ceiling the app derives from the brains' per-model timeouts and
	// dispatch shapes; a lower value fails loud at boot (ADR-0031 Decision 2 —
	// never silently guillotine a slow model). Empty means "derive it".
	BrainHandlerTimeout string          `json:"brain_handler_timeout,omitempty"`
	Channels            []ChannelConfig `json:"channels"`
	Brains              []BrainConfig   `json:"brains"`
	Routes              []RouteConfig   `json:"routes"`
	// Storage is the optional durable conversation store (ADR-0019). It is a
	// pointer so absence is distinguishable from presence: nil (block omitted)
	// means run stateless (Stage 11 / ADR-0018 behavior, unchanged); a present
	// block means open a durable store at boot. An empty Path defaults to an
	// OS-appropriate data dir, resolved in internal/app.
	Storage *StorageConfig `json:"storage,omitempty"`
	// Observability is the optional admin HTTP server (ADR-0020). It is a
	// pointer for parse-time presence detection, but note the DELIBERATE
	// asymmetry with Storage: an ABSENT block means the server is ON with safe
	// loopback defaults (observability is safe on loopback and always useful),
	// whereas an absent Storage block means OFF. The operator disables the
	// server explicitly with observability.enabled = false. Resolve via
	// ObservabilitySettings.
	Observability *ObservabilityConfig `json:"observability,omitempty"`
	// Admin is the optional admin-mutation auth block (ADR-0028 §1). It is a
	// pointer for parse-time presence detection: ABSENT means no mutation surface
	// (the read-only default, exactly today's behavior); PRESENT names the env-var
	// holding the bearer token. The token VALUE is never in the file — only the
	// env-var name — resolved via os.Getenv in internal/app, so a config that names
	// an unset var mounts nothing (no token => mutation not mounted, ADR-0028 §1).
	Admin *AdminConfig `json:"admin,omitempty"`
}

// AdminConfig names the env-var holding the admin bearer token (ADR-0028 §1). The
// value lives only in the environment (ADR-0010 discipline), never in the file.
type AdminConfig struct {
	TokenEnv string `json:"token_env"`
}

// StorageConfig declares the durable conversation store. Path is the SQLite
// database file; an empty Path resolves to <os.UserConfigDir>/korvun/korvun.db
// at boot (internal/app). The block is additive over the Stage 11 schema:
// existing configs without it keep their exact stateless behavior.
type StorageConfig struct {
	Path string `json:"path"`
}

// DefaultObservabilityAddr is the admin server's default bind address: loopback
// so a fresh boot exposes nothing to the network, on port 2112 (the conventional
// client_golang exporter port, distinct from 9090 the Prometheus server port).
// An operator who wants external access sets observability.addr to 0.0.0.0:PORT
// consciously and owns the auth/TLS/firewall that go with it (ADR-0020 §4).
const DefaultObservabilityAddr = "127.0.0.1:2112"

// ObservabilityConfig declares the admin HTTP server (/metrics + /healthz,
// ADR-0020). Enabled is a *bool so an unset value (block present, "enabled"
// omitted) is distinguishable from an explicit false and defaults to true. An
// empty Addr resolves to DefaultObservabilityAddr. The block is additive over
// the prior schema. Resolve both fields via Config.ObservabilitySettings.
type ObservabilityConfig struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Addr    string `json:"addr,omitempty"`
}

// ObservabilitySettings resolves the effective admin-server settings, applying
// the absent-is-on asymmetry and the default address. It is the single place the
// defaulting rules live, so internal/app stays thin.
func (c *Config) ObservabilitySettings() (enabled bool, addr string) {
	o := c.Observability
	if o == nil {
		return true, DefaultObservabilityAddr
	}
	enabled = o.Enabled == nil || *o.Enabled
	addr = o.Addr
	if addr == "" {
		addr = DefaultObservabilityAddr
	}
	return enabled, addr
}

// EffectiveRequestTimeout resolves the per-attempt timeout that applies to m:
// the per-model RequestTimeout if set, else the top-level Config.RequestTimeout,
// else DefaultRequestTimeout (ADR-0031 Decision 3). It assumes the strings have
// passed Validate (parseable, positive); a malformed or non-positive value is
// treated as unset so the next tier applies, never as a zero deadline.
func (c *Config) EffectiveRequestTimeout(m ModelConfig) time.Duration {
	if d := parsePositiveDuration(m.RequestTimeout); d > 0 {
		return d
	}
	if d := parsePositiveDuration(c.RequestTimeout); d > 0 {
		return d
	}
	return DefaultRequestTimeout
}

// parsePositiveDuration returns the parsed duration if s is a valid, strictly
// positive duration string, and 0 otherwise (empty, malformed, or non-positive).
func parsePositiveDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// ChannelConfig declares one messaging channel. Type selects the adapter
// ("telegram", "discord", or "webhook"); Mode selects its transport, which is
// type-aware (telegram="polling", discord="gateway", and webhook takes NO mode).
// TokenEnv is the NAME of the environment variable holding the channel's inbound
// secret — never the value itself (ADR-0010). For webhook, TokenEnv is the inbound
// shared secret checked on every request (ADR-0038 §1, NC-1a — it REUSES token_env
// rather than a dedicated field). Webhook is the optional nested block that a
// "webhook" channel requires (ADR-0038 §1).
type ChannelConfig struct {
	Type     string         `json:"type"`
	Mode     string         `json:"mode"`
	TokenEnv string         `json:"token_env"`
	Webhook  *WebhookConfig `json:"webhook,omitempty"`
}

// DefaultWebhookBind is the webhook channel's default listen address: loopback so a
// fresh boot exposes nothing to the network (ADR-0038 §1, ADR-0020 §4). An operator
// who binds a non-loopback address owns the fronting TLS-terminating reverse proxy.
const DefaultWebhookBind = "127.0.0.1:8090"

// DefaultWebhookPath is the webhook channel's default inbound POST path (ADR-0038 §1).
const DefaultWebhookPath = "/webhook"

// WebhookConfig is the nested block a "webhook" channel carries (ADR-0038 §1). It is
// a pointer on ChannelConfig for presence detection: required when type=="webhook",
// and rejected under any other type. Bind/Path default via EffectiveBind/EffectivePath
// when empty; OutboundURL is REQUIRED; OutboundTokenEnv is the OPTIONAL env-var NAME
// for the outbound downstream Bearer secret (env-only, ADR-0010); Mapping is optional
// and resolved through EffectiveMapping. The inbound secret is NOT here — it is the
// channel-level TokenEnv (NC-1a).
type WebhookConfig struct {
	Bind             string          `json:"bind,omitempty"`
	Path             string          `json:"path,omitempty"`
	OutboundURL      string          `json:"outbound_url,omitempty"`
	OutboundTokenEnv string          `json:"outbound_token_env,omitempty"`
	Mapping          *WebhookMapping `json:"mapping,omitempty"`
}

// WebhookMapping names which JSON fields in an inbound payload map to Envelope fields
// (ADR-0038 §1). Every field is optional; an empty field resolves to its canonical
// default (the field's own conventional name) via EffectiveMapping.
type WebhookMapping struct {
	SenderID       string `json:"sender_id,omitempty"`
	SenderName     string `json:"sender_name,omitempty"`
	Text           string `json:"text,omitempty"`
	MediaURL       string `json:"media_url,omitempty"`
	MediaType      string `json:"media_type,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

// EffectiveBind resolves the webhook listen address: the configured Bind if set, else
// DefaultWebhookBind (ADR-0038 §1). Mirrors the EffectiveRequestTimeout pattern —
// the single place the defaulting rule lives, so downstream wiring stays thin.
func (w *WebhookConfig) EffectiveBind() string {
	if w.Bind != "" {
		return w.Bind
	}
	return DefaultWebhookBind
}

// EffectivePath resolves the webhook inbound path: the configured Path if set, else
// DefaultWebhookPath (ADR-0038 §1).
func (w *WebhookConfig) EffectivePath() string {
	if w.Path != "" {
		return w.Path
	}
	return DefaultWebhookPath
}

// EffectiveMapping resolves the field mapping, filling every empty field with its
// canonical default name (ADR-0038 §1). It merges field-by-field so an operator may
// override only the fields they need; an absent Mapping block resolves to all six
// canonical names.
func (w *WebhookConfig) EffectiveMapping() WebhookMapping {
	m := WebhookMapping{}
	if w.Mapping != nil {
		m = *w.Mapping
	}
	if m.SenderID == "" {
		m.SenderID = "sender_id"
	}
	if m.SenderName == "" {
		m.SenderName = "sender_name"
	}
	if m.Text == "" {
		m.Text = "text"
	}
	if m.MediaURL == "" {
		m.MediaURL = "media_url"
	}
	if m.MediaType == "" {
		m.MediaType = "media_type"
	}
	if m.ConversationID == "" {
		m.ConversationID = "conversation_id"
	}
	return m
}

// BrainConfig declares one orchestrating brain: its declared data Sensitivity
// (the pre-dispatch privacy constraint, ADR-0015), the Policy that reduces the
// outcomes, the Dispatch shape (fan-out or sequential fail-over, ADR-0017 §3),
// and the Models it dispatches to.
type BrainConfig struct {
	Name        string        `json:"name"`
	Sensitivity string        `json:"sensitivity"`
	Policy      PolicyConfig  `json:"policy"`
	Dispatch    string        `json:"dispatch"`
	Models      []ModelConfig `json:"models"`
	// Retry toggles per-model retry for this brain (ADR-0031 Decision 3). It is a
	// *bool so an absent value (default: on) is distinguishable from an explicit
	// false. It MUST NOT be true on a sequential brain: the serial fail-over IS
	// the retry story, so enabling retry there would multiply the serial worst
	// case (SV2) — Validate rejects that combination.
	Retry *bool `json:"retry,omitempty"`
	// Agent, when present, mounts a tool-use AgentBrain instead of the default
	// fan-out Orchestrator (ADR-0021). Both satisfy brain.Brain, so the router and
	// cmd/korvun are agnostic to which one a brain wires. nil = the Orchestrator.
	Agent *AgentConfig `json:"agent,omitempty"`
	// Persona, when present, gives this brain a personality composed as a PREFIX
	// of its system prompt (builder-canvas spec FR-PERSONA-1, NC-4 resolved).
	// nil = today's behavior byte-for-byte: no system-prompt contribution at all.
	Persona *PersonaConfig `json:"persona,omitempty"`
}

// PersonaConfig is the optional per-brain personality block (builder-canvas
// spec FR-PERSONA-1, NC-4 resolved): all fields are free text and optional.
// DisplayName is PRESENTATION ONLY — BrainConfig.Name remains the routing key.
// Validate caps each field in RUNES (user-visible characters, not bytes):
// display_name 80, tone 200, language 60, instructions 4000 (FR-PERSONA-3).
type PersonaConfig struct {
	DisplayName  string `json:"display_name,omitempty"`
	Tone         string `json:"tone,omitempty"`
	Language     string `json:"language,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// AgentConfig configures a tool-use AgentBrain (ADR-0021). Tools names the
// built-in tools to register (time, echo, calc — the safe, pure set; resolution
// and the dangerous-tool boundary live in internal/tool.Builtin, ADR-0021 §8).
// MaxIterations is the hard loop cap (0 => the AgentBrain default). SystemPrompt
// is the operator prompt appended after the protocol block.
type AgentConfig struct {
	Tools         []string `json:"tools"`
	MaxIterations int      `json:"max_iterations"`
	SystemPrompt  string   `json:"system_prompt"`
}

// PolicyConfig selects the reducer. Kind is "priority" or "consensus"; Order is
// the provider priority list both reducers use.
type PolicyConfig struct {
	Kind  string   `json:"kind"`
	Order []string `json:"order"`
}

// ModelConfig declares one provider in a brain's catalog. Locality is DECLARED
// here (not derived from the adapter, ADR-0015 §3) so the privacy selector can
// route on it. BaseURL is optional (defaults per adapter). APIKeyEnv is the
// NAME of the env var holding a cloud provider's API key.
type ModelConfig struct {
	Provider  string `json:"provider"`
	ModelID   string `json:"model_id"`
	Locality  string `json:"locality"`
	BaseURL   string `json:"base_url"`
	APIKeyEnv string `json:"api_key_env"`
	// RequestTimeout is this model's per-attempt timeout, a duration string (e.g.
	// "15s"). It overrides the top-level Config.RequestTimeout; empty inherits it
	// (ADR-0031 Decision 3). Timeout is a property of the provider/model — a cold
	// local model wants a generous window while a cloud endpoint wants a tight
	// one. Resolve via Config.EffectiveRequestTimeout.
	RequestTimeout string `json:"request_timeout,omitempty"`
	// MaxRetries is the per-model retry count for transient post-load errors
	// (ADR-0031 Decision 3). 0 disables retry for this model; the retry mechanism
	// itself lands with the decorator (a later sub-phase). Must be >= 0.
	MaxRetries int `json:"max_retries,omitempty"`
	// Warmup marks this model for a best-effort boot warmup (ADR-0031 sub-phase 6,
	// Decision 1b): a trivial Generate at Start so the first real message does not
	// pay the cold-load latency. Only valid for local models — a warmup to a cloud
	// model bills the user for no cold-load benefit, so warmup:true on a cloud
	// model is rejected at config load. Default false.
	Warmup bool `json:"warmup,omitempty"`
}

// RouteConfig binds an inbound channel (by its registered name) to a brain (by
// name).
type RouteConfig struct {
	Channel string `json:"channel"`
	Brain   string `json:"brain"`
}

// Load reads, parses, and validates a config file. Every failure is fatal and
// names what is wrong (ADR-0017 §5): a missing file (ErrConfigRead), malformed
// JSON (ErrConfigParse), or a schema violation naming the offending field
// (ErrInvalidConfig). On success the returned Config has passed Validate.
func Load(path string) (*Config, error) {
	// #nosec G304 -- path is the operator-supplied config location (a CLI
	// argument); reading the file they point at is the entire purpose of Load.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrConfigRead, path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrConfigParse, path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate enforces the schema invariants, returning the first violation as an
// error wrapping ErrInvalidConfig with the offending field path. It checks
// structure and enum membership only; semantic wiring (resolving secrets,
// reaching providers) happens in internal/app. One normalization rides it
// (NC-1 companion): a valid config never carries nil Channels/Routes — an
// OMITTED key must behave, re-serve, and re-persist exactly like an empty
// list ("channels": null through GET /api/config would crash the builder).
func (c *Config) Validate() error {
	// Note: Storage is intentionally NOT validated here. storage.path is resolved
	// and checked at boot (internal/app openStore, which returns a named fatal
	// error) because resolving the default path and verifying writability depend
	// on the OS, not on the static schema (ADR-0019 §5).
	if err := c.validateTimeouts(); err != nil {
		return err
	}
	channelNames, err := c.validateChannels()
	if err != nil {
		return err
	}
	brainNames, err := c.validateBrains()
	if err != nil {
		return err
	}
	if err := c.validateAdmin(); err != nil {
		return err
	}
	if err := c.validateRoutes(channelNames, brainNames); err != nil {
		return err
	}
	if c.Channels == nil {
		c.Channels = []ChannelConfig{}
	}
	if c.Routes == nil {
		c.Routes = []RouteConfig{}
	}
	return nil
}

// validateTimeouts checks the top-level resilience durations (ADR-0031). A
// request_timeout that is present must parse AND be strictly positive (a zero or
// negative default is a guillotine, ADR-0031 Decision 2); a brain_handler_timeout
// that is present must parse (its >= derived check is a boot concern in
// internal/app, not a static-schema one).
func (c *Config) validateTimeouts() error {
	if c.RequestTimeout != "" {
		d, err := time.ParseDuration(c.RequestTimeout)
		if err != nil {
			return fmt.Errorf("%w: request_timeout: invalid duration %q: %w", ErrInvalidConfig, c.RequestTimeout, err)
		}
		if d <= 0 {
			return fmt.Errorf("%w: request_timeout: must be a positive duration, got %q", ErrInvalidConfig, c.RequestTimeout)
		}
	}
	if c.BrainHandlerTimeout != "" {
		if _, err := time.ParseDuration(c.BrainHandlerTimeout); err != nil {
			return fmt.Errorf("%w: brain_handler_timeout: invalid duration %q: %w", ErrInvalidConfig, c.BrainHandlerTimeout, err)
		}
	}
	return nil
}

// validateAdmin checks the optional admin block: if present, it must name a
// non-empty token_env (the env-var NAME). The value's presence in the environment
// is resolved at boot (internal/app), not here (ADR-0028 §1).
func (c *Config) validateAdmin() error {
	if c.Admin != nil && c.Admin.TokenEnv == "" {
		return fmt.Errorf("%w: admin.token_env: required when an admin block is present (name of the env var holding the bearer token)", ErrInvalidConfig)
	}
	return nil
}

// validateChannels checks every channel that IS present, strictly. ZERO
// channels is a VALID state (NC-1 of the SP5 spec, option B, 2026-07-25): the
// desktop first-run template boots channel-less — admin server, builder and
// brains alive — and the builder adds the first channel (ADR-0035 §5). The
// boot warns loudly about it (internal/app.Start).
func (c *Config) validateChannels() (map[string]bool, error) {
	names := make(map[string]bool, len(c.Channels))
	for i, ch := range c.Channels {
		// The transport mode is TYPE-AWARE: telegram=polling, discord=gateway, and
		// webhook takes NO mode (ADR-0038 §1). A mode that belongs to another channel
		// — or any mode on a webhook — is rejected with its field path.
		switch ch.Type {
		case "telegram":
			if err := validateChannelMode(i, ch.Mode, "polling"); err != nil {
				return nil, err
			}
		case "discord":
			if err := validateChannelMode(i, ch.Mode, "gateway"); err != nil {
				return nil, err
			}
		case "webhook":
			// Webhook takes NO transport mode (ADR-0038 §1, NC-1c): a non-empty mode is
			// a named field-path error. The nested block is REQUIRED (NC-1b); its
			// outbound_url is validated after the common token_env check below.
			if ch.Mode != "" {
				return nil, fmt.Errorf("%w: channels[%d].mode: webhook takes no mode", ErrInvalidConfig, i)
			}
			if ch.Webhook == nil {
				return nil, fmt.Errorf("%w: channels[%d].webhook: required for type %q", ErrInvalidConfig, i, ch.Type)
			}
		case "":
			return nil, fmt.Errorf("%w: channels[%d].type: required", ErrInvalidConfig, i)
		default:
			return nil, fmt.Errorf("%w: channels[%d].type: unknown channel type %q (supported: telegram, discord, webhook)", ErrInvalidConfig, i, ch.Type)
		}
		// A webhook block is only valid on a webhook channel (ADR-0038 §1, NC-1b).
		if ch.Type != "webhook" && ch.Webhook != nil {
			return nil, fmt.Errorf("%w: channels[%d].webhook: only valid for type %q", ErrInvalidConfig, i, "webhook")
		}
		if ch.TokenEnv == "" {
			return nil, fmt.Errorf("%w: channels[%d].token_env: required (name of the env var holding the channel secret)", ErrInvalidConfig, i)
		}
		// Webhook-only field checks that depend on the block, AFTER the common
		// token_env check so an empty token_env surfaces first (ADR-0038 §1).
		if ch.Type == "webhook" && ch.Webhook.OutboundURL == "" {
			return nil, fmt.Errorf("%w: channels[%d].webhook.outbound_url: required for type %q", ErrInvalidConfig, i, ch.Type)
		}
		// A channel registers under its type name ("telegram" / "discord" / "webhook").
		names[ch.Type] = true
	}
	return names, nil
}

// validateChannelMode checks a channel's mode against the single mode its type
// supports in this build, returning a field-path error (ADR-0017 §5) for a missing
// or wrong mode.
func validateChannelMode(i int, mode, want string) error {
	switch mode {
	case want:
		return nil
	case "":
		return fmt.Errorf("%w: channels[%d].mode: required", ErrInvalidConfig, i)
	default:
		return fmt.Errorf("%w: channels[%d].mode: unsupported mode %q (this build wires: %s)", ErrInvalidConfig, i, mode, want)
	}
}

func (c *Config) validateBrains() (map[string]bool, error) {
	if len(c.Brains) == 0 {
		return nil, fmt.Errorf("%w: brains: at least one brain is required", ErrInvalidConfig)
	}
	names := make(map[string]bool, len(c.Brains))
	for i, b := range c.Brains {
		if b.Name == "" {
			return nil, fmt.Errorf("%w: brains[%d].name: required", ErrInvalidConfig, i)
		}
		if names[b.Name] {
			return nil, fmt.Errorf("%w: brains[%d].name: duplicate brain name %q", ErrInvalidConfig, i, b.Name)
		}
		names[b.Name] = true

		switch b.Sensitivity {
		case "public", "private":
		case "":
			return nil, fmt.Errorf("%w: brains[%d].sensitivity: required (public|private)", ErrInvalidConfig, i)
		default:
			return nil, fmt.Errorf("%w: brains[%d].sensitivity: unknown sensitivity %q (public|private)", ErrInvalidConfig, i, b.Sensitivity)
		}
		switch b.Dispatch {
		case "", "fanout", "sequential": // empty defaults to fanout in app
		default:
			return nil, fmt.Errorf("%w: brains[%d].dispatch: unknown dispatch %q (fanout|sequential)", ErrInvalidConfig, i, b.Dispatch)
		}
		// SV2 (ADR-0031 Decision 3): retry is off by construction for sequential —
		// the fail-over IS the retry story, so an explicit retry:true would multiply
		// the serial worst case. Reject it loudly rather than silently ignore it.
		if b.Dispatch == "sequential" && b.Retry != nil && *b.Retry {
			return nil, fmt.Errorf("%w: brains[%d].retry: retry:true is not allowed on a sequential brain (the serial fail-over is the retry story; enabling it multiplies the serial worst case)", ErrInvalidConfig, i)
		}
		switch b.Policy.Kind {
		case "priority", "consensus":
		case "":
			return nil, fmt.Errorf("%w: brains[%d].policy.kind: required (priority|consensus)", ErrInvalidConfig, i)
		default:
			return nil, fmt.Errorf("%w: brains[%d].policy.kind: unknown policy %q (priority|consensus)", ErrInvalidConfig, i, b.Policy.Kind)
		}
		if err := validateModels(i, b.Models); err != nil {
			return nil, err
		}
		if err := validateAgent(i, b.Agent); err != nil {
			return nil, err
		}
		if err := validatePersona(i, b.Persona); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// validatePersona checks the optional persona block's length caps
// (FR-PERSONA-3, NC-4 resolved). Caps count RUNES, not bytes, so multi-byte
// text is capped by what the operator actually typed. Every violation names
// the indexed field path (brains[i].persona.<field>) so the builder's inline
// 400 mapping can target the offending input.
func validatePersona(brainIdx int, p *PersonaConfig) error {
	if p == nil {
		return nil
	}
	caps := []struct {
		field string
		value string
		max   int
	}{
		{"display_name", p.DisplayName, 80},
		{"tone", p.Tone, 200},
		{"language", p.Language, 60},
		{"instructions", p.Instructions, 4000},
	}
	for _, c := range caps {
		if n := utf8.RuneCountInString(c.value); n > c.max {
			return fmt.Errorf("%w: brains[%d].persona.%s: exceeds %d characters (got %d)",
				ErrInvalidConfig, brainIdx, c.field, c.max, n)
		}
	}
	return nil
}

// validateAgent checks the optional agent block's structure (ADR-0021). Tool-name
// resolution (the safe-toolset boundary) is a semantic concern handled in
// internal/app, mirroring how unknown provider names surface there, not here.
func validateAgent(brainIdx int, a *AgentConfig) error {
	if a == nil {
		return nil
	}
	if len(a.Tools) == 0 {
		return fmt.Errorf("%w: brains[%d].agent.tools: at least one tool is required", ErrInvalidConfig, brainIdx)
	}
	for j, name := range a.Tools {
		if name == "" {
			return fmt.Errorf("%w: brains[%d].agent.tools[%d]: tool name is required", ErrInvalidConfig, brainIdx, j)
		}
	}
	if a.MaxIterations < 0 {
		return fmt.Errorf("%w: brains[%d].agent.max_iterations: must be >= 0 (0 => default)", ErrInvalidConfig, brainIdx)
	}
	return nil
}

func validateModels(brainIdx int, models []ModelConfig) error {
	if len(models) == 0 {
		return fmt.Errorf("%w: brains[%d].models: at least one model is required", ErrInvalidConfig, brainIdx)
	}
	for j, m := range models {
		switch m.Provider {
		case "ollama", "groq":
		case "":
			return fmt.Errorf("%w: brains[%d].models[%d].provider: required", ErrInvalidConfig, brainIdx, j)
		default:
			return fmt.Errorf("%w: brains[%d].models[%d].provider: unknown provider %q (ollama|groq)", ErrInvalidConfig, brainIdx, j, m.Provider)
		}
		if m.ModelID == "" {
			return fmt.Errorf("%w: brains[%d].models[%d].model_id: required", ErrInvalidConfig, brainIdx, j)
		}
		switch m.Locality {
		case "local", "cloud":
		case "":
			return fmt.Errorf("%w: brains[%d].models[%d].locality: required (local|cloud)", ErrInvalidConfig, brainIdx, j)
		default:
			return fmt.Errorf("%w: brains[%d].models[%d].locality: unknown locality %q (local|cloud)", ErrInvalidConfig, brainIdx, j, m.Locality)
		}
		// Cloud providers must declare where their API key comes from. The
		// value is resolved from the environment at boot, never stored here.
		if m.Provider == "groq" && m.APIKeyEnv == "" {
			return fmt.Errorf("%w: brains[%d].models[%d].api_key_env: required for cloud provider %q (name of the env var holding the API key)", ErrInvalidConfig, brainIdx, j, m.Provider)
		}
		// Per-model resilience fields (ADR-0031 Decision 3): a present
		// request_timeout must parse and be strictly positive; max_retries must be
		// >= 0 (0 disables). A zero/negative timeout is a guillotine, rejected like
		// an unparseable one.
		if m.RequestTimeout != "" {
			d, err := time.ParseDuration(m.RequestTimeout)
			if err != nil {
				return fmt.Errorf("%w: brains[%d].models[%d].request_timeout: invalid duration %q: %w", ErrInvalidConfig, brainIdx, j, m.RequestTimeout, err)
			}
			if d <= 0 {
				return fmt.Errorf("%w: brains[%d].models[%d].request_timeout: must be a positive duration, got %q", ErrInvalidConfig, brainIdx, j, m.RequestTimeout)
			}
		}
		if m.MaxRetries < 0 {
			return fmt.Errorf("%w: brains[%d].models[%d].max_retries: must be >= 0 (0 disables), got %d", ErrInvalidConfig, brainIdx, j, m.MaxRetries)
		}
		// Boot warmup (ADR-0031 sub-phase 6) is only meaningful for local models:
		// a cloud model has no cold load to hide and a warmup call bills the user
		// real money, so warmup:true on a non-local model is a fail-loud config
		// error (never silently spend).
		if m.Warmup && m.Locality != "local" {
			return fmt.Errorf("%w: brains[%d].models[%d].warmup: only valid for local models, got locality %q", ErrInvalidConfig, brainIdx, j, m.Locality)
		}
	}
	return nil
}

// validateRoutes keeps the strict rule for channel-bearing configs: with at
// least one channel, at least one route is still required (a deaf channel is
// a misconfiguration). The relaxation is scoped to the CHANNEL-LESS state
// (NC-1, option B): zero channels + zero routes is the valid first-run shape.
func (c *Config) validateRoutes(channelNames, brainNames map[string]bool) error {
	if len(c.Routes) == 0 && len(c.Channels) > 0 {
		return fmt.Errorf("%w: routes: at least one route is required", ErrInvalidConfig)
	}
	for i, r := range c.Routes {
		if !channelNames[r.Channel] {
			return fmt.Errorf("%w: routes[%d].channel: no channel named %q is configured", ErrInvalidConfig, i, r.Channel)
		}
		if !brainNames[r.Brain] {
			return fmt.Errorf("%w: routes[%d].brain: no brain named %q is configured", ErrInvalidConfig, i, r.Brain)
		}
	}
	return nil
}
