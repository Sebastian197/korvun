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
)

// Config is the root deployment descriptor: the channels to run, the brains to
// orchestrate, the routes binding the two, and optional durable storage.
type Config struct {
	Channels []ChannelConfig `json:"channels"`
	Brains   []BrainConfig   `json:"brains"`
	Routes   []RouteConfig   `json:"routes"`
	// Storage is the optional durable conversation store (ADR-0019). It is a
	// pointer so absence is distinguishable from presence: nil (block omitted)
	// means run stateless (Stage 11 / ADR-0018 behavior, unchanged); a present
	// block means open a durable store at boot. An empty Path defaults to an
	// OS-appropriate data dir, resolved in internal/app.
	Storage *StorageConfig `json:"storage,omitempty"`
}

// StorageConfig declares the durable conversation store. Path is the SQLite
// database file; an empty Path resolves to <os.UserConfigDir>/korvun/korvun.db
// at boot (internal/app). The block is additive over the Stage 11 schema:
// existing configs without it keep their exact stateless behavior.
type StorageConfig struct {
	Path string `json:"path"`
}

// ChannelConfig declares one messaging channel. Type selects the adapter
// (currently "telegram"); Mode selects its transport ("polling"). TokenEnv is
// the NAME of the environment variable holding the bot token — never the token
// itself.
type ChannelConfig struct {
	Type     string `json:"type"`
	Mode     string `json:"mode"`
	TokenEnv string `json:"token_env"`
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
// reaching providers) happens in internal/app.
func (c *Config) Validate() error {
	// Note: Storage is intentionally NOT validated here. storage.path is resolved
	// and checked at boot (internal/app openStore, which returns a named fatal
	// error) because resolving the default path and verifying writability depend
	// on the OS, not on the static schema (ADR-0019 §5).
	channelNames, err := c.validateChannels()
	if err != nil {
		return err
	}
	brainNames, err := c.validateBrains()
	if err != nil {
		return err
	}
	return c.validateRoutes(channelNames, brainNames)
}

func (c *Config) validateChannels() (map[string]bool, error) {
	if len(c.Channels) == 0 {
		return nil, fmt.Errorf("%w: channels: at least one channel is required", ErrInvalidConfig)
	}
	names := make(map[string]bool, len(c.Channels))
	for i, ch := range c.Channels {
		switch ch.Type {
		case "telegram":
		case "":
			return nil, fmt.Errorf("%w: channels[%d].type: required", ErrInvalidConfig, i)
		default:
			return nil, fmt.Errorf("%w: channels[%d].type: unknown channel type %q (supported: telegram)", ErrInvalidConfig, i, ch.Type)
		}
		switch ch.Mode {
		case "polling":
		case "":
			return nil, fmt.Errorf("%w: channels[%d].mode: required", ErrInvalidConfig, i)
		default:
			return nil, fmt.Errorf("%w: channels[%d].mode: unsupported mode %q (this build wires: polling)", ErrInvalidConfig, i, ch.Mode)
		}
		if ch.TokenEnv == "" {
			return nil, fmt.Errorf("%w: channels[%d].token_env: required (name of the env var holding the bot token)", ErrInvalidConfig, i)
		}
		// A telegram channel registers under its type name ("telegram").
		names[ch.Type] = true
	}
	return names, nil
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
	}
	return names, nil
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
	}
	return nil
}

func (c *Config) validateRoutes(channelNames, brainNames map[string]bool) error {
	if len(c.Routes) == 0 {
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
