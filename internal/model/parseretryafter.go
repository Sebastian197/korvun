// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter reads a Retry-After header value as an integer
// number of seconds and returns it as a time.Duration. Empty or
// unparseable values return zero — the consumer interprets zero as
// "no hint given" (e.g. HTTP 429 without a usable retry-after).
//
// It parses the seconds form only. The HTTP spec also allows an
// HTTP-date form, which is deliberately NOT handled here: neither the
// Ollama nor the Groq adapter — the two current consumers — emits it
// today, so an HTTP-date value yields zero. A future provider that
// uses the date form would require extending this function.
//
// This is the single source of truth shared by internal/model/ollama
// and internal/model/groq (ADR-0031 sub-phase 3, decision D2); the
// body is the former groq.parseRetryAfter verbatim.
func ParseRetryAfter(raw string) time.Duration {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}
