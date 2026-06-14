// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package groq

import "errors"

// ErrMissingAPIKey is returned by New when neither WithAPIKey was
// used nor the GROQ_API_KEY environment variable is set. The error
// is provider-specific (lives in this package, not in
// internal/model) because the env-var name is part of the Groq
// adapter's contract, not the universal Model contract. ADR-0010
// §3.
var ErrMissingAPIKey = errors.New("groq: missing API key (set GROQ_API_KEY or use WithAPIKey)")
