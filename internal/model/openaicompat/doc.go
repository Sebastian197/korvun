// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package openaicompat implements internal/model.Model against ANY
// OpenAI-compatible chat-completions endpoint — cloud or local — selected
// by config alone (ADR-0044, the universal model gateway).
//
// Contract: docs/superpowers/specs/2026-08-22-universal-model-gateway.md
// (FINAL — Accepted for TDD). Generalizes the Groq mold (ADR-0010): the
// operator supplies the FULL base_url prefix and Korvun appends exactly
// "/chat/completions" (zero-magic rule); the error grammar is the FR-GW-4
// matrix; redirects are refused (FR-GW-7); success bodies are bounded at
// maxResponseBytes.
//
// The API key is resolved by the WIRING (internal/app) from the config's
// api_key_env and passed via WithAPIKey — there is no env-var fallback in
// this package, and an EMPTY key is valid (no-auth local servers send no
// Authorization header). The key lives in an unexported field with no
// accessor, is never logged, and is literally redacted from every
// body/header-derived diagnostic (the H8 belt).
package openaicompat

// ProviderName is the canonical provider label (D1) emitted on every
// model.Response this adapter produces and returned by Name().
const ProviderName = "openai-compatible"

// maxResponseBytes bounds the SUCCESS (2xx) response body read
// (FR-GW-3/H3): generous for chat, deliberately stricter than the
// groq/ollama mold because base_url is operator-arbitrary.
const maxResponseBytes = 4 << 20 // 4 MiB
