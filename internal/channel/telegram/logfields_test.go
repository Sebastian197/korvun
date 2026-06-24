// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// capturingHandler is a test slog.Handler that records every Record it
// handles, so a test can assert which structured fields a log call carried.
// It is concurrency-safe (the adapter logs from the dispatch goroutine).
type capturingHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
	attrs   []slog.Attr
}

func newCapturingLogger() (*slog.Logger, *[]slog.Record, *sync.Mutex) {
	mu := &sync.Mutex{}
	recs := &[]slog.Record{}
	return slog.New(capturingHandler{mu: mu, records: recs}), recs, mu
}

func (h capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := r.Clone()
	for _, a := range h.attrs {
		rec.AddAttrs(a)
	}
	*h.records = append(*h.records, rec)
	return nil
}

func (h capturingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	merged := append(append([]slog.Attr{}, h.attrs...), as...)
	return capturingHandler{mu: h.mu, records: h.records, attrs: merged}
}

func (h capturingHandler) WithGroup(string) slog.Handler { return h }

// findRecord returns the first captured record whose message equals msg.
func findRecord(t *testing.T, recs *[]slog.Record, mu *sync.Mutex, msg string) slog.Record {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	for _, r := range *recs {
		if r.Message == msg {
			return r
		}
	}
	t.Fatalf("no log record with message %q (captured %d records)", msg, len(*recs))
	return slog.Record{}
}

// attrString returns the string value of the named attr on the record, and
// whether it was present.
func attrString(r slog.Record, key string) (string, bool) {
	var out string
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			out = a.Value.String()
			found = true
			return false
		}
		return true
	})
	return out, found
}

// TestDropLog_carriesChannelAndEnvelopeID asserts the drop-on-saturation log
// carries the standardized funnel fields channel and envelope_id (ADR-0020 §1),
// so a dropped message can be correlated by channel and envelope across logs
// and metrics.
func TestDropLog_carriesChannelAndEnvelopeID(t *testing.T) {
	logger, recs, mu := newCapturingLogger()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithInboundCapacity(1),
		WithEnqueueTimeout(20*time.Millisecond),
		WithLogger(logger),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	// Fill the single-slot buffer, then force a drop on the next dispatch.
	a.dispatchUpdate(context.Background(), newTextUpdate(1, 11, "first"))
	a.dispatchUpdate(context.Background(), newTextUpdate(1, 12, "second"))
	if got := a.DroppedCount(); got != 1 {
		t.Fatalf("DroppedCount = %d, want 1", got)
	}

	rec := findRecord(t, recs, mu, "telegram: dropped inbound envelope after enqueue timeout")

	if ch, ok := attrString(rec, "channel"); !ok || ch != a.Name() {
		t.Errorf("channel field = %q (present=%v), want %q", ch, ok, a.Name())
	}
	if id, ok := attrString(rec, "envelope_id"); !ok || id == "" {
		t.Errorf("envelope_id field = %q (present=%v), want a non-empty id", id, ok)
	}
}
