// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// --- compile-time interface check ---
var _ channel.Channel = (*Adapter)(nil)

// --- FieldMapping tests ---

func defaultMapping() FieldMapping {
	return FieldMapping{
		SenderID:   "sender_id",
		SenderName: "sender_name",
		Text:       "text",
		MediaURL:   "media_url",
		MediaType:  "media_type",
	}
}

// --- Inbound (HTTP → Envelope) ---

func TestInbound_valid_text_payload(t *testing.T) {
	mapping := defaultMapping()
	a := New("test-webhook", mapping)

	payload := map[string]string{
		"sender_id":   "user-1",
		"sender_name": "Alice",
		"text":        "hello from webhook",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.InboundHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	select {
	case env := <-a.Inbound():
		if env.Channel != "test-webhook" {
			t.Errorf("Channel = %q, want %q", env.Channel, "test-webhook")
		}
		if env.Sender.ID != "user-1" {
			t.Errorf("Sender.ID = %q, want %q", env.Sender.ID, "user-1")
		}
		if env.Sender.Name != "Alice" {
			t.Errorf("Sender.Name = %q, want %q", env.Sender.Name, "Alice")
		}
		if len(env.Parts) != 1 || env.Parts[0].Content != "hello from webhook" {
			t.Errorf("Parts = %+v, want text 'hello from webhook'", env.Parts)
		}
		if env.Direction != envelope.Inbound {
			t.Errorf("Direction = %v, want Inbound", env.Direction)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for envelope")
	}
}

func TestInbound_media_payload(t *testing.T) {
	mapping := defaultMapping()
	a := New("test-webhook", mapping)

	payload := map[string]string{
		"sender_id":  "user-1",
		"media_url":  "https://example.com/photo.jpg",
		"media_type": "image/jpeg",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.InboundHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	select {
	case env := <-a.Inbound():
		if len(env.Parts) != 1 {
			t.Fatalf("Parts len = %d, want 1", len(env.Parts))
		}
		p := env.Parts[0]
		if p.Type != envelope.Image {
			t.Errorf("Type = %v, want Image", p.Type)
		}
		if p.Source != "https://example.com/photo.jpg" {
			t.Errorf("Source = %q, want url", p.Source)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for envelope")
	}
}

func TestInbound_malformed_json(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.InboundHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestInbound_missing_sender(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	payload := map[string]string{
		"text": "no sender",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.InboundHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestInbound_empty_body(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.InboundHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestInbound_wrong_method(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()

	a.InboundHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// --- Outbound (Envelope → HTTP POST) ---

func TestOutbound_send(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mapping := defaultMapping()
	a := New("test-webhook", mapping)
	a.SetOutboundURL(server.URL)

	env := envelope.New("test-webhook", envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("reply from bot")

	err := a.Send(context.Background(), env)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(receivedBody, &parsed); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	if parsed["text"] != "reply from bot" {
		t.Errorf("text = %q, want %q", parsed["text"], "reply from bot")
	}
}

func TestOutbound_send_server_error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := New("test-webhook", defaultMapping())
	a.SetOutboundURL(server.URL)

	env := envelope.New("test-webhook", envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("hello")

	err := a.Send(context.Background(), env)
	if err == nil {
		t.Fatal("Send() should return error on server 500")
	}
}

func TestOutbound_send_no_url(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	env := envelope.New("test-webhook", envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("hello")

	err := a.Send(context.Background(), env)
	if err == nil {
		t.Fatal("Send() should return error when no outbound URL is set")
	}
}

// --- Channel interface compliance ---

func TestAdapter_Name(t *testing.T) {
	a := New("my-webhook", defaultMapping())
	if a.Name() != "my-webhook" {
		t.Errorf("Name() = %q, want %q", a.Name(), "my-webhook")
	}
}

func TestAdapter_Manifest(t *testing.T) {
	a := New("my-webhook", defaultMapping())
	m := a.Manifest()
	if !m.Text {
		t.Error("Manifest.Text should be true")
	}
	if !m.Image {
		t.Error("Manifest.Image should be true")
	}
}

func TestAdapter_Receive(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}

	// Send an inbound request to populate the channel
	payload := map[string]string{
		"sender_id": "user-1",
		"text":      "via receive",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)

	select {
	case env := <-ch:
		if env.Parts[0].Content != "via receive" {
			t.Errorf("content = %q, want %q", env.Parts[0].Content, "via receive")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for envelope via Receive()")
	}

	cancel()
}

func TestInbound_audio_media(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	payload := map[string]string{
		"sender_id":  "user-1",
		"media_url":  "https://example.com/clip.mp3",
		"media_type": "audio/mpeg",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)

	env := <-a.Inbound()
	if env.Parts[0].Type != envelope.Audio {
		t.Errorf("Type = %v, want Audio", env.Parts[0].Type)
	}
}

func TestInbound_video_media(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	payload := map[string]string{
		"sender_id":  "user-1",
		"media_url":  "https://example.com/vid.mp4",
		"media_type": "video/mp4",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)

	env := <-a.Inbound()
	if env.Parts[0].Type != envelope.Video {
		t.Errorf("Type = %v, want Video", env.Parts[0].Type)
	}
}

func TestInbound_file_media(t *testing.T) {
	a := New("test-webhook", defaultMapping())

	payload := map[string]string{
		"sender_id":  "user-1",
		"media_url":  "https://example.com/doc.pdf",
		"media_type": "application/pdf",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.InboundHandler().ServeHTTP(rec, req)

	env := <-a.Inbound()
	if env.Parts[0].Type != envelope.File {
		t.Errorf("Type = %v, want File", env.Parts[0].Type)
	}
}

func TestOutbound_send_text_and_media(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := New("test-webhook", defaultMapping())
	a.SetOutboundURL(server.URL)

	env := envelope.New("test-webhook", envelope.Outbound, envelope.Participant{ID: "bot", Name: "Bot"})
	env.AddText("check this")
	env.AddMedia(envelope.Image, "https://example.com/img.png", "image/png")

	err := a.Send(context.Background(), env)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(receivedBody, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if parsed["text"] != "check this" {
		t.Errorf("text = %q, want %q", parsed["text"], "check this")
	}
	if parsed["media_url"] != "https://example.com/img.png" {
		t.Errorf("media_url = %q", parsed["media_url"])
	}
	if parsed["sender_name"] != "Bot" {
		t.Errorf("sender_name = %q, want %q", parsed["sender_name"], "Bot")
	}
}
