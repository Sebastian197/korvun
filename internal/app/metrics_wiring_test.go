// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/Sebastian197/korvun/internal/router"
)

// routerErrRecorder records IncRouterError calls; other methods are Nop.
type routerErrRecorder struct {
	metrics.Nop
	kinds []string
}

func (r *routerErrRecorder) IncRouterError(kind string) { r.kinds = append(r.kinds, kind) }

// TestOnRouterError_countsByKind asserts the router-error funnel increments the
// metric with the kind string (the WithErrorHandler funnel, ADR-0020 §3).
func TestOnRouterError_countsByKind(t *testing.T) {
	rec := &routerErrRecorder{}
	onRouterError(slog.New(slog.DiscardHandler), rec, nil, router.RouterError{
		Kind:    router.ErrKindSend,
		Channel: "telegram",
		Err:     errors.New("boom"),
	})

	if len(rec.kinds) != 1 || rec.kinds[0] != "send" {
		t.Errorf("router error kinds = %v, want [send]", rec.kinds)
	}
}

// msgRecorder records IncMessages calls; other methods are Nop.
type msgRecorder struct {
	metrics.Nop
	channels []string
}

func (m *msgRecorder) IncMessages(channel string) { m.channels = append(m.channels, channel) }

// TestBuildBrain_wiresMetrics asserts the metrics backend reaches the brain:
// handling one message records it, proving buildBrain injected the recorder.
// The ollama provider has no server in the test, so dispatch fails and the
// reply is the fallback — but IncMessages fires before dispatch, which is what
// this asserts.
func TestBuildBrain_wiresMetrics(t *testing.T) {
	rec := &msgRecorder{}
	b := &builder{
		logger:          slog.New(slog.DiscardHandler),
		perModelTimeout: DefaultPerModelTimeout,
		newChannel:      defaultChannelFactory,
		metrics:         rec,
	}
	orch, err := b.buildBrain(ollamaBrain())
	if err != nil {
		t.Fatalf("buildBrain: %v", err)
	}

	if _, err := orch.Handle(context.Background(), inboundText("telegram", "c1", "q")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rec.channels) != 1 || rec.channels[0] != "telegram" {
		t.Errorf("recorded messages = %v, want [telegram]", rec.channels)
	}
}

// fakeRegistrar records RegisterDroppedSource calls.
type fakeRegistrar struct {
	got map[string]func() uint64
}

func (f *fakeRegistrar) RegisterDroppedSource(channel string, count func() uint64) error {
	if f.got == nil {
		f.got = map[string]func() uint64{}
	}
	f.got[channel] = count
	return nil
}

// droppingChannel is a Channel that also exposes a cumulative DroppedCount.
type droppingChannel struct {
	*fakeChannel
	dropped uint64
}

func (d *droppingChannel) DroppedCount() uint64 { return d.dropped }

// TestRegisterDroppedSources asserts only channels exposing DroppedCount are
// registered, and the registered function reads the live count (ADR-0020 §3).
func TestRegisterDroppedSources(t *testing.T) {
	dc := &droppingChannel{fakeChannel: newFakeChannel("telegram"), dropped: 7}
	plain := newFakeChannel("webhook") // no DroppedCount: must be skipped
	reg := &fakeRegistrar{}

	registerDroppedSources(reg, []Channel{dc, plain}, slog.New(slog.DiscardHandler))

	if len(reg.got) != 1 {
		t.Fatalf("registered %d sources, want 1 (only the dropping channel)", len(reg.got))
	}
	fn, ok := reg.got["telegram"]
	if !ok {
		t.Fatalf("telegram dropped source not registered; got keys %v", reg.got)
	}
	if got := fn(); got != 7 {
		t.Errorf("source fn = %d, want 7 (reads the live count)", got)
	}
}

// inboundText builds an inbound text Envelope carrying a conversation id, so the
// brain has something to dispatch.
func inboundText(channel, convID, text string) *envelope.Envelope {
	e := envelope.New(channel, envelope.Inbound, envelope.Participant{ID: "u1", Name: "User"})
	e.AddText(text)
	e.Meta[conversation.MetaConversationID] = convID
	return e
}
