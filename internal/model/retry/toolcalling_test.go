// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package retry

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// Capability propagation (ADR-0042 §4): the decorator satisfies
// ToolCallingModel IF AND ONLY IF the wrapped model does, applying the SAME
// retry policy to GenerateWithTools. A capability must not vanish inside a
// decorator; the native lane must not silently lose retry.

// plainModel has no tool-calling capability.
type plainModel struct{}

func (plainModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "ok"}}, nil
}
func (plainModel) Name() string { return "plain" }

// flakyToolModel fails GenerateWithTools with a retryable error n times,
// then succeeds.
type flakyToolModel struct {
	mu       sync.Mutex
	failures int
	calls    int
}

func (m *flakyToolModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "gen"}}, nil
}
func (m *flakyToolModel) Name() string { return "flaky" }
func (m *flakyToolModel) GenerateWithTools(_ context.Context, _ *model.Request, _ []model.ToolSpec) (*model.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failures {
		return nil, fmt.Errorf("%w: transient", model.ErrProviderUnavailable)
	}
	return &model.Response{Message: model.Message{
		Role:      model.RoleAssistant,
		ToolCalls: []model.ToolCall{{Name: "calc", Arguments: map[string]any{"args": "2+2"}}},
	}}, nil
}

func TestNew_propagatesCapabilityIffInnerHasIt(t *testing.T) {
	t.Parallel()
	withCap := New(&flakyToolModel{}, Config{MaxRetries: 1})
	if _, ok := withCap.(model.ToolCallingModel); !ok {
		t.Fatal("decorator hid the inner model's tool-calling capability")
	}
	withoutCap := New(plainModel{}, Config{MaxRetries: 1})
	if _, ok := withoutCap.(model.ToolCallingModel); ok {
		t.Fatal("decorator invented a capability the inner model lacks")
	}
}

func TestGenerateWithTools_retriesLikeGenerate(t *testing.T) {
	t.Parallel()
	inner := &flakyToolModel{failures: 1}
	wrapped := New(inner, Config{MaxRetries: 2, PerAttempt: 0},
		WithClock(&fakeClock{}), WithRand(fixedRand(1.0)))

	tcm, ok := wrapped.(model.ToolCallingModel)
	if !ok {
		t.Fatal("capability not propagated")
	}
	req := &model.Request{Model: "m", Messages: []model.Message{{Role: model.RoleUser, Content: "x"}}}
	resp, err := tcm.GenerateWithTools(context.Background(), req, []model.ToolSpec{{Name: "calc", Description: "d"}})
	if err != nil {
		t.Fatalf("GenerateWithTools after a transient failure: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("response mangled: %+v", resp.Message)
	}
	if inner.calls != 2 {
		t.Fatalf("inner called %d times, want 2 (one retry)", inner.calls)
	}
}

func TestGenerateWithTools_nonRetryableStops(t *testing.T) {
	t.Parallel()
	inner := &flakyToolModel{failures: 99}
	wrapped := New(inner, Config{MaxRetries: 2, PerAttempt: 0},
		WithClock(&fakeClock{}), WithRand(fixedRand(1.0)))

	tcm := wrapped.(model.ToolCallingModel)
	req := &model.Request{Model: "m", Messages: []model.Message{{Role: model.RoleUser, Content: "x"}}}
	_, err := tcm.GenerateWithTools(context.Background(), req, nil)
	if err == nil {
		t.Fatal("exhausted retries must surface the error")
	}
	if inner.calls != 3 {
		t.Fatalf("inner called %d times, want 3 (initial + MaxRetries)", inner.calls)
	}
}
