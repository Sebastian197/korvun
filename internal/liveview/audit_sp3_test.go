// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP3 — AS-6, the audit that matters most: with the WHOLE operator flow
// running through the real bus and the real router (inbound, takeover +
// silenced inbound, operator reply, session reset with its ack), every SSE
// frame the live-view emits stays SECRET-FREE — no frame ever carries
// message content. ADR-0024 §1 is asserted byte-by-byte, not remembered.
package liveview

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// auditChannel is a minimal channel.Channel for the audit flow.
type auditChannel struct {
	inbound chan *envelope.Envelope
}

func (a *auditChannel) Name() string               { return "tg" }
func (a *auditChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (a *auditChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return a.inbound, nil
}
func (a *auditChannel) Send(context.Context, *envelope.Envelope) error { return nil }

// auditBrain replies with a fixed sensitive-looking text.
type auditBrain struct{}

func (auditBrain) Handle(_ context.Context, in *envelope.Envelope) ([]*envelope.Envelope, error) {
	out := envelope.New(in.Channel, envelope.Outbound, envelope.Participant{ID: "korvun"}).
		AddText("SECRET-REPLY dato sanitario")
	for k, v := range in.Meta {
		out.Meta[k] = v
	}
	return []*envelope.Envelope{out}, nil
}

func mkAuditInbound(text string) *envelope.Envelope {
	e := envelope.New("tg", envelope.Inbound, envelope.Participant{ID: "u1"}).AddText(text)
	e.Meta[conversation.MetaConversationID] = "c"
	return e
}

func TestAudit_NoSSEFrameEverCarriesMessageContent(t *testing.T) {
	// The payloads that must NEVER appear in a frame. Distinctive strings on
	// purpose — a plain substring scan over the raw SSE bytes is the audit.
	payloads := []string{
		"SECRET-USER consulta médica",   // normal inbound
		"SECRET-REPLY dato sanitario",   // brain reply
		"SECRET-TAKEN te atiendo yo no", // inbound under takeover
		"SECRET-OPERATOR aquí Chano",    // operator reply
	}

	b := bus.New()
	t.Cleanup(func() { b.Close() })
	lv := New(b, WithNow(fixedNow))
	t.Cleanup(lv.Close)
	srv := newTestServer(t, lv)
	resp, br := connectSSE(t, srv)
	// Registered AFTER newTestServer so it runs BEFORE srv.Close (LIFO):
	// an open streaming body makes httptest.Server.Close wait forever.
	t.Cleanup(func() { _ = resp.Body.Close() })

	store := conversation.NewMemStore()
	r := router.New(
		router.WithSessionStore(store),
		router.WithSessionPolicy(router.SessionPolicy{Triggers: []string{"/new"}}),
		router.WithEventPublisher(b),
	)
	ch := &auditChannel{inbound: make(chan *envelope.Envelope)}
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b", auditBrain{}); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("tg", "b"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	ctx := context.Background()

	// The full operator flow, every beat that publishes events:
	// 1. A normal inbound → brain reply (MessageReceived + ReplySent).
	if err := r.DispatchInbound(ctx, mkAuditInbound(payloads[0])); err != nil {
		t.Fatalf("inbound: %v", err)
	}
	// 2. Takeover + silenced inbound (MessageReceived without brain).
	r.TakeOver("tg::c")
	if err := r.DispatchInbound(ctx, mkAuditInbound(payloads[2])); err != nil {
		t.Fatalf("inbound under takeover: %v", err)
	}
	// 3. The operator reply (ReplySent through the same funnel).
	opEnv := envelope.New("tg", envelope.Outbound, envelope.Participant{ID: "operator"}).
		AddText(payloads[3])
	opEnv.Meta[conversation.MetaConversationID] = "c"
	if err := r.DispatchOutbound(ctx, opEnv); err != nil {
		t.Fatalf("DispatchOutbound: %v", err)
	}
	r.Release("tg::c")
	// 4. A bare session reset → the fixed ack rides the outbound funnel too.
	if err := r.DispatchInbound(ctx, mkAuditInbound("/new")); err != nil {
		t.Fatalf("reset inbound: %v", err)
	}

	// Collect frames until the stream quiesces: at least 4 events were
	// published (received ×2, sent ×2/3); read with a deadline and audit
	// every byte we got.
	frames := make(chan string, 32)
	go func() {
		for {
			f := readFrameQuiet(br)
			if f == "" {
				return
			}
			frames <- f
		}
	}()
	var collected []string
	deadline := time.After(3 * time.Second)
	for len(collected) < 4 {
		select {
		case f := <-frames:
			collected = append(collected, f)
		case <-deadline:
			t.Fatalf("collected only %d frames before the deadline: %v", len(collected), collected)
		}
	}
	// Drain a moment longer — a late frame with content would be the leak.
	settle := time.After(300 * time.Millisecond)
drain:
	for {
		select {
		case f := <-frames:
			collected = append(collected, f)
		case <-settle:
			break drain
		}
	}

	for _, frame := range collected {
		for _, p := range payloads {
			if strings.Contains(frame, p) {
				t.Fatalf("SSE frame carries message content (ADR-0024 §1 violated):\n%s", frame)
			}
		}
		// Belt and braces: no fragment of any payload word either.
		for _, word := range []string{"SECRET", "Chano", "sanitario", "médica"} {
			if strings.Contains(frame, word) {
				t.Fatalf("SSE frame leaks payload fragment %q:\n%s", word, frame)
			}
		}
	}
}

// readFrameQuiet is readFrame without t.Fatalf — the collector goroutine ends
// quietly when the server closes the stream at test teardown.
func readFrameQuiet(br interface{ ReadString(byte) (string, error) }) string {
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return ""
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
}
