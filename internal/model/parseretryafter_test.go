// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"testing"
	"time"
)

// TestParseRetryAfter pins the contract of the shared, exported
// ParseRetryAfter helper (ADR-0031 sub-phase 3, decision D2). The
// contract is copied verbatim from the current groq.parseRetryAfter
// so the move preserves behaviour exactly:
//
//   - a positive integer number of seconds → that many seconds;
//   - empty / whitespace / non-numeric / zero / negative → 0
//     (the "no hint given" sentinel value);
//   - the HTTP-date form is deliberately NOT parsed and yields 0.
//
// The honesty note about the HTTP-date form (Principle 1: do not lose
// that honesty when moving the helper) must survive on ParseRetryAfter's
// godoc: it parses seconds only, neither Ollama nor Groq emits the
// HTTP-date form today, and a future provider that does would require
// extending this function.
func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", 0},
		{"   ", 0},
		{"30", 30 * time.Second},
		{" 7 ", 7 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"nonsense", 0},
		{"Mon, 14 Jun 2026 18:00:00 GMT", 0}, // HTTP-date form, not supported
	}
	for _, tc := range cases {
		if got := ParseRetryAfter(tc.raw); got != tc.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
