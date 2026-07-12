// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/model/retry"
)

// This file pins ADR-0031 sub-phase 7's F8: with the retry decorator in place,
// the Outcome.Latency CallOne captures spans the TOTAL provider time (all
// attempts + backoff), not just the last attempt. A shared virtual clock drives
// both the decorator's backoff sleep and CallOne's latency measurement, so the
// total is deterministic with zero real sleeps.
//
// NOTE: the decorator already exists (sub-phase 4) and CallOne already times the
// decorated m.Generate, so this is a born-GREEN pin of emergent behaviour (like
// the benign-503 guard); the godoc is updated in green.

// virtualClock is both a retry.Clock (Sleep advances virtual time) and the
// source of CallOne's now(), so a fake backoff advances the measured latency.
type virtualClock struct {
	mu sync.Mutex
	t  time.Duration
}

func (v *virtualClock) now() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return time.Unix(0, 0).Add(v.t)
}

func (v *virtualClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	v.mu.Lock()
	v.t += d
	v.mu.Unlock()
	return nil
}

// failThenOKModel fails once (transient) then succeeds.
type failThenOKModel struct {
	name string
	mu   sync.Mutex
	n    int
}

func (m *failThenOKModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	m.mu.Lock()
	m.n++
	c := m.n
	m.mu.Unlock()
	if c == 1 {
		return nil, fmt.Errorf("%w: 503", model.ErrProviderUnavailable)
	}
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "ok"}, Provider: m.name}, nil
}
func (m *failThenOKModel) Name() string { return m.name }

func TestF8_latencyIncludesRetryBackoff(t *testing.T) {
	t.Parallel()
	vc := &virtualClock{}
	inner := &failThenOKModel{name: "ollama"}
	// full jitter rand=1.0 → first backoff wait = min(2s, 200ms·2⁰) = 200ms.
	d := retry.New(inner, retry.Config{PerAttempt: time.Second, MaxRetries: 1},
		retry.WithClock(vc), retry.WithRand(func() float64 { return 1.0 }))

	req := &model.Request{Model: "m", Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}}}
	oc := fanout.CallOne(context.Background(), req, d, 0, vc.now)

	if oc.Err != nil {
		t.Fatalf("Outcome.Err = %v, want success on the retry", oc.Err)
	}
	if oc.Latency < 200*time.Millisecond {
		t.Errorf("Outcome.Latency = %v, want >= 200ms (TOTAL incl. the retry backoff — F8, not just the last attempt)", oc.Latency)
	}
}
