// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/router"
	"github.com/go-telegram/bot/models"
)

func TestWebhookHandler_acceptsValidUpdate(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	body := mustMarshal(t, newTextUpdate(99, 1, "via webhook"))
	req := newWebhookRequest(t, "topsecret", body)
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	select {
	case env := <-a.inbound:
		if env.Meta[router.MetaConversationID] != "99" {
			t.Errorf("Meta[conversation.id] = %q, want %q",
				env.Meta[router.MetaConversationID], "99")
		}
		if env.Parts[0].Content != "via webhook" {
			t.Errorf("Content = %q", env.Parts[0].Content)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no envelope on inbound after successful POST")
	}
}

func TestWebhookHandler_rejectsMissingSecret(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	body := mustMarshal(t, newTextUpdate(99, 1, "x"))
	req := httptest.NewRequest(http.MethodPost, "/wh", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWebhookHandler_rejectsWrongSecret(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	body := mustMarshal(t, newTextUpdate(99, 1, "x"))
	req := newWebhookRequest(t, "wrong-secret", body)
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWebhookHandler_rejectsNonPost(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	req := httptest.NewRequest(http.MethodGet, "/wh", nil)
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestWebhookHandler_rejectsInvalidJSON(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	req := newWebhookRequest(t, "topsecret", []byte("not json"))
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestWebhookHandler_rejectsOversizedBody(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	huge := bytes.Repeat([]byte{'{'}, maxWebhookBodyBytes+1)
	req := newWebhookRequest(t, "topsecret", huge)
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 on oversized body", rec.Code)
	}
}

func TestWebhookHandler_acksOnSilentSkip(t *testing.T) {
	a := newWebhookAdapter(t, "topsecret")
	body := mustMarshal(t, &models.Update{Message: &models.Message{}})
	req := newWebhookRequest(t, "topsecret", body)
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for silent-skip body", rec.Code)
	}
	select {
	case env := <-a.inbound:
		t.Fatalf("envelope unexpectedly delivered: %+v", env)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWebhookHandler_acksOnSaturation(t *testing.T) {
	a, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr(":8443"),
		WithSecretToken("topsecret"),
		WithReverseProxyTermination(),
		WithInboundCapacity(1),
		WithEnqueueTimeout(20*time.Millisecond),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	// Pre-fill so the next dispatch saturates.
	a.dispatchUpdate(httptest.NewRequest("", "/", nil).Context(),
		newTextUpdate(7, 1, "first"))

	body := mustMarshal(t, newTextUpdate(7, 2, "second"))
	req := newWebhookRequest(t, "topsecret", body)
	rec := httptest.NewRecorder()

	a.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 on saturation (Telegram should not back off)", rec.Code)
	}
	if a.DroppedCount() != 1 {
		t.Errorf("DroppedCount = %d, want 1", a.DroppedCount())
	}
}

func newWebhookAdapter(t *testing.T, secret string) *Adapter {
	t.Helper()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr(":8443"),
		WithSecretToken(secret),
		WithReverseProxyTermination(),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("newWebhookAdapter: New() err = %v", err)
	}
	return a
}

func newWebhookRequest(t *testing.T, secret string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/wh", bytes.NewReader(body))
	if secret != "" {
		req.Header.Set(secretTokenHeader, secret)
	}
	return req
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}
