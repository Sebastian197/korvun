// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// capturingHandler records every Record it handles so a test can assert which
// structured fields a log call carried.
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

// TestLogRouterError_carriesEnvelopeID asserts the router-error funnel log
// carries envelope_id (alongside the existing kind/channel/brain), so an async
// router failure can be correlated to the offending envelope (ADR-0020 §1).
func TestLogRouterError_carriesEnvelopeID(t *testing.T) {
	logger, recs, mu := newCapturingLogger()
	env := &envelope.Envelope{ID: "env-xyz"}
	re := router.RouterError{
		Kind:     router.ErrKindHandle,
		Brain:    "b1",
		Channel:  "telegram",
		Envelope: env,
		Err:      errors.New("boom"),
	}

	logRouterError(logger, re)

	rec := findRecord(t, recs, mu, "router error")
	if id, ok := attrString(rec, "envelope_id"); !ok || id != "env-xyz" {
		t.Errorf("envelope_id = %q (present=%v), want %q", id, ok, "env-xyz")
	}
	if k, ok := attrString(rec, "kind"); !ok || k != "handle" {
		t.Errorf("kind = %q (present=%v), want %q", k, ok, "handle")
	}
	if c, ok := attrString(rec, "channel"); !ok || c != "telegram" {
		t.Errorf("channel = %q (present=%v), want %q", c, ok, "telegram")
	}
	if b, ok := attrString(rec, "brain"); !ok || b != "b1" {
		t.Errorf("brain = %q (present=%v), want %q", b, ok, "b1")
	}
}

// TestLogRouterError_nilEnvelope asserts a RouterError with no Envelope logs an
// empty envelope_id without panicking (some kinds carry no envelope).
func TestLogRouterError_nilEnvelope(t *testing.T) {
	logger, recs, mu := newCapturingLogger()
	re := router.RouterError{
		Kind:    router.ErrKindSend,
		Channel: "telegram",
		Err:     errors.New("x"),
	}

	logRouterError(logger, re)

	rec := findRecord(t, recs, mu, "router error")
	if id, ok := attrString(rec, "envelope_id"); !ok || id != "" {
		t.Errorf("envelope_id = %q (present=%v), want present-and-empty", id, ok)
	}
}
