// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config

import "errors"

// Sentinel errors returned by Load and Validate. Per ADR-0017 §5, a
// malformed configuration is a FATAL boot error: the binary must refuse to
// start and name the offending field, never panic. Callers match these with
// errors.Is; every error wraps one of them with a field path and reason.
var (
	// ErrConfigRead is returned when the config file cannot be read from
	// disk (missing, unreadable). The path is included.
	ErrConfigRead = errors.New("config: cannot read file")

	// ErrConfigParse is returned when the file is not valid JSON. The path
	// and the decoder's own message are included.
	ErrConfigParse = errors.New("config: invalid JSON")

	// ErrInvalidConfig is returned when the parsed config violates a schema
	// invariant (missing required field, unknown enum value, dangling
	// route). The wrapping message names the offending field path.
	ErrInvalidConfig = errors.New("config: invalid")
)
