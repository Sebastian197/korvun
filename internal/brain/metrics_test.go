// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// provObs records one ObserveProviderDuration call.
type provObs struct {
	provider string
	ok       bool
	d        time.Duration
}

// recordingMetrics is a metrics.Metrics that records every call, so a test can
// assert the Brain instruments its funnels.
type recordingMetrics struct {
	mu        sync.Mutex
	messages  []string
	durations []provObs
	failures  []string
	turns     []int
}

func (m *recordingMetrics) IncMessages(channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, channel)
}

func (m *recordingMetrics) ObserveProviderDuration(provider string, ok bool, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.durations = append(m.durations, provObs{provider, ok, d})
}

func (m *recordingMetrics) IncProviderFailure(provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures = append(m.failures, provider)
}

func (m *recordingMetrics) IncRouterError(string) {}

func (m *recordingMetrics) ObserveTurnsPersisted(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turns = append(m.turns, n)
}

// fixedCoord returns a hand-built *fanout.Result so a test controls the exact
// per-provider outcomes (provider, ok/err, latency) the Brain should instrument.
type fixedCoord struct{ res *fanout.Result }

func (f fixedCoord) Run(_ context.Context, _ *model.Request, _ []model.Model) (*fanout.Result, error) {
	return f.res, nil
}

// TestHandle_recordsMessageAndProviderObservations asserts the Brain records one
// message per handled inbound and one duration observation per provider outcome,
// with a failure counted for the failing provider (ADR-0020 §3).
func TestHandle_recordsMessageAndProviderObservations(t *testing.T) {
	res := &fanout.Result{Outcomes: []fanout.Outcome{
		{Provider: "groq", Response: &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "hi"}}, Latency: 100 * time.Millisecond},
		{Provider: "ollama", Err: errors.New("down"), Latency: 200 * time.Millisecond},
	}}
	rec := &recordingMetrics{}
	o := NewOrchestrator(fixedCoord{res: res}, nil, fixedDecision("the answer"),
		WithLogger(quietLogger()), WithMetrics(rec))

	if _, err := o.Handle(context.Background(), inboundText("telegram", "c1", "q")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(rec.messages) != 1 || rec.messages[0] != "telegram" {
		t.Errorf("messages = %v, want [telegram]", rec.messages)
	}
	want := []provObs{
		{"groq", true, 100 * time.Millisecond},
		{"ollama", false, 200 * time.Millisecond},
	}
	if len(rec.durations) != len(want) {
		t.Fatalf("durations = %v, want %v", rec.durations, want)
	}
	for i, w := range want {
		if rec.durations[i] != w {
			t.Errorf("durations[%d] = %v, want %v", i, rec.durations[i], w)
		}
	}
	if len(rec.failures) != 1 || rec.failures[0] != "ollama" {
		t.Errorf("failures = %v, want [ollama]", rec.failures)
	}
}

// TestHandle_observesTurnsPersisted asserts the persisted turn group size is
// recorded on a successful reply with a configured store (ADR-0020 §3).
func TestHandle_observesTurnsPersisted(t *testing.T) {
	store := conversation.NewMemStore()
	rec := &recordingMetrics{}
	res := &fanout.Result{Outcomes: []fanout.Outcome{
		{Provider: "groq", Response: &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "a"}}, Latency: time.Millisecond},
	}}
	o := NewOrchestrator(fixedCoord{res: res}, nil, fixedDecision("the answer"),
		WithLogger(quietLogger()), WithMetrics(rec), WithConversationStore(store, 10))

	if _, err := o.Handle(context.Background(), inboundConv("telegram", "c1", "q")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// One user turn + one assistant turn = a group of 2.
	if len(rec.turns) != 1 || rec.turns[0] != 2 {
		t.Errorf("turns = %v, want [2]", rec.turns)
	}
}
