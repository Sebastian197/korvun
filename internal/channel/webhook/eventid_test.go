// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// Audit finding R-1: a webhook caller that sends X-Idempotency-Key gets
// router-side deduplication of its retries; without the header there is no
// event id and NO dedup (fail-open — documented contract, the sender opts in).

func postInbound(t *testing.T, a *Adapter, idempotencyKey string) *envelope.Envelope {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"sender_id": "user-1", "sender_name": "Ana", "text": "hola",
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case env := <-a.Inbound():
		return env
	case <-ctx.Done():
		t.Fatal("timed out waiting for envelope")
		return nil
	}
}

func TestInbound_IdempotencyKeyBecomesProviderEventID(t *testing.T) {
	a := New("test-webhook", defaultMapping())
	env := postInbound(t, a, "req-42")
	if got := env.Meta[envelope.MetaProviderEventID]; got != "req-42" {
		t.Errorf("Meta[%q] = %q, want %q", envelope.MetaProviderEventID, got, "req-42")
	}
}

func TestInbound_NoIdempotencyKeyMeansNoEventID(t *testing.T) {
	a := New("test-webhook", defaultMapping())
	env := postInbound(t, a, "")
	if got, ok := env.Meta[envelope.MetaProviderEventID]; ok {
		t.Errorf("Meta[%q] = %q, want absent (no header, no dedup)",
			envelope.MetaProviderEventID, got)
	}
}

func TestInbound_oversizedIdempotencyKeyRejected(t *testing.T) {
	// Estreno E-9: the key is sender-controlled and feeds a 4096-entry
	// window — unbounded lengths turn the dedup memory into a balloon. An
	// oversized key is a loud sender bug (400), never a silent truncation.
	a := New("test-webhook", defaultMapping())
	body, _ := json.Marshal(map[string]string{
		"sender_id": "user-1", "sender_name": "Ana", "text": "hola",
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", strings.Repeat("k", maxIdempotencyKeyBytes+1))
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized key: status = %d, want 400", rec.Code)
	}
}

func TestInbound_maxLengthIdempotencyKeyAccepted(t *testing.T) {
	a := New("test-webhook", defaultMapping())
	key := strings.Repeat("k", maxIdempotencyKeyBytes)
	env := postInboundWithKey(t, a, key)
	if got := env.Meta[envelope.MetaProviderEventID]; got != key {
		t.Fatalf("Meta id length = %d, want the full %d-byte key", len(got), maxIdempotencyKeyBytes)
	}
}

// postInboundWithKey mirrors postInbound for arbitrary keys.
func postInboundWithKey(t *testing.T, a *Adapter, key string) *envelope.Envelope {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"sender_id": "user-1", "sender_name": "Ana", "text": "hola",
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", key)
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case env := <-a.Inbound():
		return env
	case <-ctx.Done():
		t.Fatal("timed out waiting for envelope")
		return nil
	}
}
