// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import "errors"

// Sentinel errors returned by Build. Per the golden rule (ADR-0017 §5), a
// configuration or boot failure is FATAL and names what is wrong; a provider
// being unreachable at runtime is NOT fatal (handled by the Brain fallback).
var (
	// ErrMissingSecret is returned when a required secret env var (named by
	// the config via token_env / api_key_env) is not set in the environment.
	// The offending env var NAME is included; the value never is (ADR-0010).
	ErrMissingSecret = errors.New("app: required secret env var is not set")

	// ErrUnknownProvider is returned when a model declares a provider this
	// build cannot construct. config.Validate normally catches this first;
	// this is the app-layer guard for a Config built without Load.
	ErrUnknownProvider = errors.New("app: unknown model provider")

	// ErrUnknownChannelType is returned when a channel declares a type this
	// build cannot construct.
	ErrUnknownChannelType = errors.New("app: unknown channel type")

	// ErrUnknownPolicy is returned when a brain declares a policy kind this
	// build cannot construct.
	ErrUnknownPolicy = errors.New("app: unknown policy kind")

	// ErrUnknownLocality is returned when a model declares a locality that is
	// neither local nor cloud.
	ErrUnknownLocality = errors.New("app: unknown locality")

	// ErrUnknownTool is returned when an agent brain configures a tool name this
	// build cannot resolve. Only the safe built-ins (time, echo, calc) resolve;
	// a dangerous name like "shell" fails loudly at boot (ADR-0021 §8).
	ErrUnknownTool = errors.New("app: unknown agent tool")

	// ErrSensitiveToolUngoverned is returned when an agent brain lists a
	// Sensitive-attr tool on a Cloud-locality model WITHOUT a governance
	// block (estreno E-11). The sensitive×locality rule lives in the gate
	// (policy.SelectTools), which is only mounted with grants — so the
	// ungoverned combination would ship the tool's output (e.g. jail file
	// contents) to the cloud provider with no check anywhere, while the
	// /tools report asserted the protection. Declared, never inferred: the
	// operator adds a governance block or overrides the tool's sensitive
	// attr consciously.
	ErrSensitiveToolUngoverned = errors.New("app: sensitive tool on a cloud brain requires a governance block")

	// ErrMissingToolCage is returned when a caged tool (read_file,
	// http_fetch, webhook_call) is listed in agent.tools without its cage
	// config block (ADR-0041 §4) — a caged tool must not exist without its
	// cage, so the wiring fails loud instead of improvising defaults.
	ErrMissingToolCage = errors.New("app: caged tool requires its cage config block")

	// ErrAgentModelCount is returned when an agent brain is configured with a
	// number of (post-selection) models other than exactly one: the Stage 8 cut
	// is a SINGLE-model tool-use loop (ADR-0021 §1).
	ErrAgentModelCount = errors.New("app: agent brain requires exactly one model")

	// ErrCeilingOverrideTooLow is returned when config sets an explicit
	// brain_handler_timeout below the ceiling the app derives from the brains'
	// per-model timeouts and dispatch shapes. Honoring it would silently
	// guillotine a slow model, so Build fails loud instead (ADR-0031 Decision 2).
	ErrCeilingOverrideTooLow = errors.New("app: brain_handler_timeout override is below the derived ceiling")
)
