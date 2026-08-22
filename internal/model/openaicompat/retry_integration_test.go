// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package openaicompat_test holds the H12a/N3 integration pins: the retry
// decorator WRAPPING the compat adapter. Location rationale (declared per
// the spec): openaicompat does NOT import internal/model/retry, so this
// external test package importing both creates no import cycle.
package openaicompat_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/openaicompat"
	"github.com/Sebastian197/korvun/internal/model/retry"
)

// recordingClock implements retry.Clock and records every requested wait
// without sleeping wall-clock time.
type recordingClock struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (c *recordingClock) Sleep(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waits = append(c.waits, d)
	return nil
}

func (c *recordingClock) recorded() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

func req() *model.Request {
	return &model.Request{
		Model:    "test-model",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hello"}},
	}
}

// TestRetryIntegration_retryAfterDrivesTheWait pins the WHOLE chain
// (H12a, AS-3): a 429 with Retry-After: 7 from the compat adapter makes
// the wrapping decorator wait >= 7s, observed on the injected fake clock
// (retry.WithClock), before the retried attempt succeeds.
func TestRetryIntegration_retryAfterDrivesTheWait(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_exceeded","code":""}}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"after the wait"}}]}`))
	}))
	t.Cleanup(srv.Close)

	adapter, err := openaicompat.New(openaicompat.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clock := &recordingClock{}
	decorated := retry.New(adapter, retry.Config{PerAttempt: 5 * time.Second, MaxRetries: 2},
		retry.WithClock(clock), retry.WithRand(func() float64 { return 0 }))

	resp, err := decorated.Generate(context.Background(), req())
	if err != nil {
		t.Fatalf("decorated Generate: %v", err)
	}
	if resp.Message.Content != "after the wait" {
		t.Errorf("content = %q, want the retried answer", resp.Message.Content)
	}
	waits := clock.recorded()
	if len(waits) == 0 {
		t.Fatal("fake clock observed no wait; want one >= 7s (the Retry-After hint consumed)")
	}
	if waits[0] < 7*time.Second {
		t.Errorf("observed wait = %s, want >= 7s (Retry-After honored through the chain)", waits[0])
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want 2 (one 429 + one retried success)", n)
	}
}

// TestRetryIntegration_deadlineIsNotRetried is the N3 tripwire through
// the REAL chain: with retries ENABLED and a per-attempt deadline shorter
// than the server, the decorated call fails with a chain that preserves
// context.DeadlineExceeded (the adapter must not flatten it — F6 depends
// on errors.Is holding at the decorator) and the server sees EXACTLY ONE
// call: no retry fired.
func TestRetryIntegration_deadlineIsNotRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"role":"assistant","content":"too late"}}]}`))
	}))
	t.Cleanup(srv.Close)

	adapter, err := openaicompat.New(openaicompat.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clock := &recordingClock{}
	decorated := retry.New(adapter, retry.Config{PerAttempt: 50 * time.Millisecond, MaxRetries: 2},
		retry.WithClock(clock), retry.WithRand(func() float64 { return 0 }))

	_, err = decorated.Generate(context.Background(), req())
	if err == nil {
		t.Fatal("decorated Generate: nil error, want a deadline expiry")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(context.DeadlineExceeded) = false for err %v — the adapter flattened the cause (N3)", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want exactly 1 (F6: deadline expiry is never retried)", n)
	}
}
