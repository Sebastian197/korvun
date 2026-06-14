// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package ollama implements internal/model.Model against a local
// Ollama server.
//
// The adapter speaks HTTP+JSON directly to POST /api/chat with
// stream:false, using only Go stdlib (net/http + encoding/json).
// The wire format was verified against the v0.30.8 source of
// github.com/ollama/ollama; only the fields Korvun actually reads
// (model, messages[].role, messages[].content, response.model,
// response.message.role, response.message.content) are modelled
// as unexported structs in this package. ADR-0009 §4 documents
// why this is hand-rolled rather than pulled from the official
// Go client.
//
// The default base URL respects the OLLAMA_HOST environment
// variable (matching the convention of every Ollama tool); see
// DefaultBaseURL.
package ollama

// ProviderName is the canonical provider label emitted on every
// model.Response this adapter produces. Used downstream by the
// fan-out and observability layers for source attribution.
const ProviderName = "ollama"

// DefaultBaseURL is the address used when OLLAMA_HOST is unset
// and no WithBaseURL option was passed. Matches the Ollama
// project's documented default.
const DefaultBaseURL = "http://127.0.0.1:11434"
