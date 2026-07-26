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
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// This file is the SP4 TDD contract for conversation.id mapping (ADR-0038 FR-MAP-1)
// and inbound saturation (ADR-0038 §5, AS-7). RED-first: it references the surface the
// implementation must add — FieldMapping.ConversationID (json "conversation_id",
// additive) and (a *Adapter) DroppedCount() uint64 (the house drop seam) — neither of
// which exists yet, so the webhook test package does not compile until SP4 lands. The
// conversation key uses the EXACT canonical symbol conversation.MetaConversationID
// ("conversation.id"), the same one the Discord adapter records (verified on disk in
// internal/channel/discord/inbound.go); it matches the spec's FR-MAP-1 name — no
// divergence. Reuses testSecret / defaultMapping / postJSON from the SP2/Stage-2
// files (same package); never modifies them.

// inboundBuffer is the Stage-2 inbound channel capacity (webhook.go New): the buffer
// the saturation tests fill.
const inboundBuffer = 64

// convMapping is the canonical mapping WITH the conversation_id field set — built from
// the Stage-2 defaultMapping() helper without modifying it.
func convMapping() FieldMapping {
	m := defaultMapping()
	m.ConversationID = "conversation_id"
	return m
}

func newConvAdapter(t *testing.T, outboundURL string) *Adapter {
	t.Helper()
	return NewWithOptions("test-webhook", Options{
		Bind:        "127.0.0.1:0",
		Path:        "/hook",
		Secret:      testSecret,
		OutboundURL: outboundURL,
		Mapping:     convMapping(),
	})
}

// TestConversation_mappedField pins FR-MAP-1: when the mapped conversation field is
// present in the payload, the Envelope carries it under conversation.MetaConversationID.
func TestConversation_mappedField(t *testing.T) {
	a := newConvAdapter(t, "https://downstream.example/in")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	resp := postJSON(t, "http://"+a.BoundAddr()+"/hook", map[string]string{
		"sender_id":       "user-1",
		"text":            "hi",
		"conversation_id": "thread-42",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	select {
	case env := <-a.Inbound():
		if got := env.Meta[conversation.MetaConversationID]; got != "thread-42" {
			t.Errorf("Meta[%q] = %q, want %q", conversation.MetaConversationID, got, "thread-42")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the Envelope")
	}
}

// TestConversation_fallbackToSender pins AS-6: with no conversation field in the
// payload, the key falls back to the sender ID (never empty).
func TestConversation_fallbackToSender(t *testing.T) {
	a := newConvAdapter(t, "https://downstream.example/in")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	resp := postJSON(t, "http://"+a.BoundAddr()+"/hook", map[string]string{
		"sender_id": "user-1",
		"text":      "hi",
	})
	defer func() { _ = resp.Body.Close() }()

	select {
	case env := <-a.Inbound():
		got := env.Meta[conversation.MetaConversationID]
		if got == "" {
			t.Fatal("conversation key is empty; want the sender ID fallback")
		}
		if got != "user-1" {
			t.Errorf("Meta[%q] = %q, want sender ID %q", conversation.MetaConversationID, got, "user-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the Envelope")
	}
}

// TestConversation_echoOnSend pins the outbound leg: Send of an Envelope carrying the
// conversation key in Meta POSTs a payload whose mapped conversation field holds that
// value, alongside the existing sender/text mapping.
func TestConversation_echoOnSend(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := newConvAdapter(t, srv.URL)

	env := envelope.New("test-webhook", envelope.Outbound, envelope.Participant{ID: "bot", Name: "Bot"})
	env.AddText("reply body")
	env.Meta[conversation.MetaConversationID] = "thread-9"

	if err := a.Send(context.Background(), env); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("outbound body is not valid JSON: %v", err)
	}
	if payload["conversation_id"] != "thread-9" {
		t.Errorf("outbound conversation_id = %q, want %q", payload["conversation_id"], "thread-9")
	}
	if payload["text"] != "reply body" {
		t.Errorf("outbound text = %q, want %q", payload["text"], "reply body")
	}
	if payload["sender_id"] != "bot" {
		t.Errorf("outbound sender_id = %q, want %q", payload["sender_id"], "bot")
	}
}

// authedPOST posts a valid authenticated JSON payload to url with the given client,
// returning the response (or an error if the round-trip failed / timed out).
func authedPOST(url string, client *http.Client) (*http.Response, error) {
	body, _ := json.Marshal(map[string]string{"sender_id": "u", "text": "hi"})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testSecret)
	return client.Do(req)
}

// TestSaturation_dropCounted pins AS-7: with no consumer, exactly inboundBuffer POSTs
// fill the buffer (200 each); the next is dropped (503), DroppedCount advances, and a
// later drain yields exactly inboundBuffer Envelopes — nothing extra.
func TestSaturation_dropCounted(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()
	url := "http://" + a.BoundAddr() + "/hook"
	client := &http.Client{Timeout: 2 * time.Second}

	for i := 0; i < inboundBuffer; i++ {
		resp, err := authedPOST(url, client)
		if err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("post %d status = %d, want 200 (buffer not yet full)", i, resp.StatusCode)
		}
	}
	if a.DroppedCount() != 0 {
		t.Errorf("DroppedCount() = %d after filling the buffer, want 0", a.DroppedCount())
	}

	// The overflow POST is dropped, not blocked.
	resp, err := authedPOST(url, client)
	if err != nil {
		t.Fatalf("overflow post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("overflow status = %d, want 503", resp.StatusCode)
	}
	if a.DroppedCount() == 0 {
		t.Error("DroppedCount() did not advance on the overflow drop")
	}

	// Drain exactly inboundBuffer Envelopes, nothing more.
	for got := 0; got < inboundBuffer; got++ {
		select {
		case <-a.Inbound():
		case <-time.After(2 * time.Second):
			t.Fatalf("only drained %d of %d Envelopes", got, inboundBuffer)
		}
	}
	select {
	case env := <-a.Inbound():
		t.Errorf("extra Envelope after draining the buffer: %+v", env)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSaturation_neverBlocks pins AS-7's core property: a burst well over the buffer,
// with no consumer, still gets EVERY response back within the client timeout — no
// goroutine hangs on a full inbound channel.
func TestSaturation_neverBlocks(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()
	url := "http://" + a.BoundAddr() + "/hook"

	const burst = 2 * inboundBuffer
	var wg sync.WaitGroup
	var mu sync.Mutex
	timeouts := 0
	bad := 0
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := authedPOST(url, client)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				timeouts++
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
				bad++
			}
		}()
	}
	wg.Wait()
	if timeouts != 0 {
		t.Errorf("%d/%d requests failed to return within the timeout (a blocked handler)", timeouts, burst)
	}
	if bad != 0 {
		t.Errorf("%d responses were neither 200 nor 503", bad)
	}
}

// TestSaturation_happyPathNoDrops pins the no-drop path: with a consumer draining
// Inbound, a burst produces zero drops.
func TestSaturation_happyPathNoDrops(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Drain until Inbound closes (at Stop).
	drained := make(chan struct{})
	go func() {
		for range a.Inbound() {
		}
		close(drained)
	}()

	url := "http://" + a.BoundAddr() + "/hook"
	client := &http.Client{Timeout: 3 * time.Second}
	for i := 0; i < 3*inboundBuffer; i++ {
		resp, err := authedPOST(url, client)
		if err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("post %d status = %d, want 200 (consumer draining)", i, resp.StatusCode)
		}
	}
	if a.DroppedCount() != 0 {
		t.Errorf("DroppedCount() = %d with a draining consumer, want 0", a.DroppedCount())
	}

	_ = a.Stop(context.Background())
	<-drained
}

// TestSaturation_concurrentStopNoPanic pins AS-9's safety: goroutines posting while
// Stop runs must not panic (under -race); each response is 200, 503, or a connection
// error — all valid during shutdown.
func TestSaturation_concurrentStopNoPanic(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	url := "http://" + a.BoundAddr() + "/hook"

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := authedPOST(url, client)
			if err != nil {
				return // connection error during shutdown is valid
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 200/503 or a connection error", resp.StatusCode)
			}
		}()
	}

	// Stop concurrently with the in-flight posts.
	_ = a.Stop(context.Background())
	wg.Wait()
}
