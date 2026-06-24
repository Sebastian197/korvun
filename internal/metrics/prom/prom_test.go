// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package prom

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestNew_satisfiesMetricsInterface pins that the Prometheus impl is a
// metrics.Metrics (the domain depends only on the interface, ADR-0020 §2).
func TestNew_satisfiesMetricsInterface(t *testing.T) {
	var _ metrics.Metrics = New()
}

func TestIncMessages_countsByChannel(t *testing.T) {
	m := New()
	m.IncMessages("telegram")
	m.IncMessages("telegram")

	const expected = `
# HELP korvun_messages_processed_total Inbound messages handed to a brain, by channel.
# TYPE korvun_messages_processed_total counter
korvun_messages_processed_total{channel="telegram"} 2
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_messages_processed_total"); err != nil {
		t.Errorf("messages counter mismatch:\n%v", err)
	}
}

func TestIncRouterError_countsByKind(t *testing.T) {
	m := New()
	m.IncRouterError("handle")

	const expected = `
# HELP korvun_router_errors_total Asynchronous router failures, by kind.
# TYPE korvun_router_errors_total counter
korvun_router_errors_total{kind="handle"} 1
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_router_errors_total"); err != nil {
		t.Errorf("router errors counter mismatch:\n%v", err)
	}
}

func TestIncProviderFailure_countsByProvider(t *testing.T) {
	m := New()
	m.IncProviderFailure("groq")
	m.IncProviderFailure("groq")

	const expected = `
# HELP korvun_provider_failures_total Failed provider calls, by provider.
# TYPE korvun_provider_failures_total counter
korvun_provider_failures_total{provider="groq"} 2
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_provider_failures_total"); err != nil {
		t.Errorf("provider failures counter mismatch:\n%v", err)
	}
}

func TestObserveTurnsPersisted_sumsGroups(t *testing.T) {
	m := New()
	m.ObserveTurnsPersisted(2)
	m.ObserveTurnsPersisted(3)

	const expected = `
# HELP korvun_conversation_turns_persisted_total Turns durably appended on a successful reply.
# TYPE korvun_conversation_turns_persisted_total counter
korvun_conversation_turns_persisted_total 5
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_conversation_turns_persisted_total"); err != nil {
		t.Errorf("turns persisted counter mismatch:\n%v", err)
	}
}

// TestObserveProviderDuration_recordsOkOutcome asserts one ok observation lands
// under outcome="ok" with the right bucket math (250ms <= the 0.25s bucket).
// GatherAndCompare parses the expected text into the metric model and compares
// semantically, so label order is irrelevant; only the fixed buckets and the
// observed value are pinned.
func TestObserveProviderDuration_recordsOkOutcome(t *testing.T) {
	m := New()
	m.ObserveProviderDuration("groq", true, 250*time.Millisecond)

	const expected = `
# HELP korvun_provider_request_duration_seconds Provider call latency, by provider and outcome.
# TYPE korvun_provider_request_duration_seconds histogram
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="0.05"} 0
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="0.1"} 0
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="0.25"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="0.5"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="1"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="2.5"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="5"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="10"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="20"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="30"} 1
korvun_provider_request_duration_seconds_bucket{outcome="ok",provider="groq",le="+Inf"} 1
korvun_provider_request_duration_seconds_sum{outcome="ok",provider="groq"} 0.25
korvun_provider_request_duration_seconds_count{outcome="ok",provider="groq"} 1
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_provider_request_duration_seconds"); err != nil {
		t.Errorf("provider duration histogram mismatch:\n%v", err)
	}
}

// TestObserveProviderDuration_outcomeSplitsSeries asserts ok and error map to
// DISTINCT outcome labels for the same provider, so failure latency is
// separable from success latency: two observations on one provider yield two
// histogram series.
func TestObserveProviderDuration_outcomeSplitsSeries(t *testing.T) {
	m := New()
	m.ObserveProviderDuration("groq", true, 250*time.Millisecond)
	m.ObserveProviderDuration("groq", false, 5*time.Second)

	got, err := testutil.GatherAndCount(m.Gatherer(), "korvun_provider_request_duration_seconds")
	if err != nil {
		t.Fatalf("GatherAndCount: %v", err)
	}
	if got != 2 {
		t.Errorf("series count = %d, want 2 (ok and error split into distinct series)", got)
	}
}

// TestHandler_servesMetricsExposition asserts Handler() returns an http.Handler
// that serves the registry's metrics in Prometheus text format, so the admin
// server can mount it at /metrics without importing promhttp itself (ADR-0020
// §2, §4).
func TestHandler_servesMetricsExposition(t *testing.T) {
	m := New()
	m.IncMessages("telegram")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler(slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "korvun_messages_processed_total") {
		t.Errorf("exposition missing korvun_messages_processed_total:\n%s", body)
	}
}

// TestRegisterDroppedSource_isPull asserts the dropped counter reflects the
// source AT SCRAPE TIME: changing the source after registration changes the
// gathered value, proving it is pull-collected, not a snapshot taken at
// registration (ADR-0020 §3).
func TestRegisterDroppedSource_isPull(t *testing.T) {
	m := New()
	var dropped atomic.Uint64
	if err := m.RegisterDroppedSource("telegram", dropped.Load); err != nil {
		t.Fatalf("RegisterDroppedSource: %v", err)
	}

	const before = `
# HELP korvun_channel_messages_dropped_total Inbound messages dropped after enqueue timeout, by channel.
# TYPE korvun_channel_messages_dropped_total counter
korvun_channel_messages_dropped_total{channel="telegram"} 0
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(before), "korvun_channel_messages_dropped_total"); err != nil {
		t.Errorf("dropped before any drop:\n%v", err)
	}

	dropped.Add(3)

	const after = `
# HELP korvun_channel_messages_dropped_total Inbound messages dropped after enqueue timeout, by channel.
# TYPE korvun_channel_messages_dropped_total counter
korvun_channel_messages_dropped_total{channel="telegram"} 3
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(after), "korvun_channel_messages_dropped_total"); err != nil {
		t.Errorf("dropped after 3 (pull semantics):\n%v", err)
	}
}

// TestRegisterDroppedSource_duplicateReturnsErrorNoPanic asserts a second
// registration for the same channel returns an error instead of panicking
// (MustRegister would panic). Defensive: today the router rejects duplicate
// channel names, but a future second channel type must not turn a config into a
// hard boot panic (review F2).
func TestRegisterDroppedSource_duplicateReturnsErrorNoPanic(t *testing.T) {
	m := New()
	var dropped atomic.Uint64
	if err := m.RegisterDroppedSource("telegram", dropped.Load); err != nil {
		t.Fatalf("first RegisterDroppedSource: %v", err)
	}
	if err := m.RegisterDroppedSource("telegram", dropped.Load); err == nil {
		t.Error("second RegisterDroppedSource for the same channel returned nil, want a registration error (not a panic)")
	}
}
