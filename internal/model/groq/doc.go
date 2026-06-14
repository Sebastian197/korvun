// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package groq implements internal/model.Model against the Groq
// cloud API.
//
// The adapter speaks HTTP+JSON directly to POST
// /openai/v1/chat/completions on the Groq host, using only Go
// stdlib (net/http + encoding/json). The wire format was verified
// against the Groq OpenAI-compatibility docs and a direct probe of
// the live 401 error envelope; only the fields Korvun actually
// reads are modelled as unexported structs in this package.
// ADR-0010 §2 documents why this is hand-rolled rather than pulled
// from a community Go library.
//
// The API key is resolved at construction in the order:
// WithAPIKey > GROQ_API_KEY env > ErrMissingAPIKey. Never argv,
// never a committed config file. The key lives in an unexported
// Adapter field with no accessor and is never logged or surfaced
// in any error. See ADR-0010 §3.
//
// Cloud-shaped failures map to the sentinels added to
// internal/model in commit A:
//
//   - 401 / 403          → model.ErrAuthInvalid
//   - 429                → *model.RateLimitError (wraps model.ErrRateLimited)
//   - 5xx / network / ctx → model.ErrProviderUnavailable
//   - 4xx (other)        → model.ErrProviderResponse
//   - malformed 2xx body → model.ErrProviderResponse
//
// See ADR-0010 §4 for the full table.
package groq

// ProviderName is the canonical provider label emitted on every
// model.Response this adapter produces.
const ProviderName = "groq"

// DefaultBaseURL is the address used when no WithBaseURL option is
// supplied. Matches the Groq OpenAI-compatibility surface; the
// /chat/completions path is appended at request time.
const DefaultBaseURL = "https://api.groq.com/openai/v1"
