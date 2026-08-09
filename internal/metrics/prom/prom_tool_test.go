// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package prom

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ObserveToolUse records the governed-tool gate outcomes (ADR-0041 §5): a
// counter by (tool, outcome) plus a latency histogram under the same labels.
// Labels carry tool name and outcome ONLY — never args (the ADR-0024 §1 law
// extended: unbounded label values are both a leak and a cardinality hazard).

func TestObserveToolUse_countsByToolAndOutcome(t *testing.T) {
	m := New()
	m.ObserveToolUse("http_fetch", "ok", 100*time.Millisecond)
	m.ObserveToolUse("http_fetch", "ok", 200*time.Millisecond)
	m.ObserveToolUse("http_fetch", "denied", 0)
	m.ObserveToolUse("read_file", "shadowed", 0)

	const expected = `
# HELP korvun_tool_calls_total Governed tool calls, by tool and outcome (ok|error|denied|shadowed).
# TYPE korvun_tool_calls_total counter
korvun_tool_calls_total{outcome="denied",tool="http_fetch"} 1
korvun_tool_calls_total{outcome="ok",tool="http_fetch"} 2
korvun_tool_calls_total{outcome="shadowed",tool="read_file"} 1
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_tool_calls_total"); err != nil {
		t.Errorf("tool calls counter mismatch:\n%v", err)
	}
}

func TestObserveToolUse_recordsLatencyHistogram(t *testing.T) {
	m := New()
	m.ObserveToolUse("http_fetch", "ok", 100*time.Millisecond)
	m.ObserveToolUse("http_fetch", "ok", 200*time.Millisecond)

	// Two observations totalling 0.3s under (http_fetch, ok).
	count := testutil.CollectAndCount(m.toolDur, "korvun_tool_call_duration_seconds")
	if count == 0 {
		t.Fatal("tool duration histogram collected no series")
	}
}
