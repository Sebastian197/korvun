// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Executor contract (spec FR-EXEC-1): timeout, scope routing, latency and
// the unknown sentinel — runTool's execution semantics, preserved verbatim
// behind the single path.

package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/tool"
)

type plainTool struct {
	gotArgs string
	err     error
}

func (p *plainTool) Name() string        { return "plain" }
func (p *plainTool) Description() string { return "plain fixture" }
func (p *plainTool) Execute(ctx context.Context, args string) (string, error) {
	p.gotArgs = args
	return "plain:" + args, p.err
}

type scopedTool struct {
	plainTool
	gotScope tool.Scope
}

func (s *scopedTool) ExecuteScoped(ctx context.Context, scope tool.Scope, args string) (string, error) {
	s.gotScope = scope
	s.gotArgs = args
	return "scoped:" + args, nil
}

type sleepyTool struct{}

func (sleepyTool) Name() string        { return "sleepy" }
func (sleepyTool) Description() string { return "waits for ctx" }
func (sleepyTool) Execute(ctx context.Context, args string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func fixedNow() func() time.Time {
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	calls := 0
	return func() time.Time {
		calls++
		return base.Add(time.Duration(calls) * time.Millisecond)
	}
}

func TestRun_plainAndScopedRouting(t *testing.T) {
	t.Parallel()
	plain := &plainTool{}
	scoped := &scopedTool{}
	e := NewCoordinator(tool.Registry{"plain": plain, "scoped": scoped}, 0, fixedNow(), CoordinatorConfig{BrainName: "b"})
	ctx := context.Background()
	plan := testPlan(t, e, ctx, "console")
	env := testEnvelope("console", "operator", "c")

	result, err := e.Submit(ctx, testRequest(t, e, plan, env, "text", "plain", "x"))
	if err != nil || result.Output != "plain:x" {
		t.Fatalf("plain path: out=%q err=%v", result.Output, err)
	}
	if result.Latency <= 0 {
		t.Fatalf("latency must be measured, got %v", result.Latency)
	}
	result, err = e.Submit(ctx, testRequest(t, e, plan, env, "text", "scoped", "y"))
	if err != nil || result.Output != "scoped:y" {
		t.Fatalf("scoped path: out=%q err=%v", result.Output, err)
	}
	wantScope := tool.Scope{Brain: "b", Conversation: "console::c"}
	if scoped.gotScope != wantScope {
		t.Fatalf("the derived scope must reach the scoped tool, got %+v", scoped.gotScope)
	}
}

func TestRun_unknownToolSentinel(t *testing.T) {
	t.Parallel()
	e := New(tool.Registry{}, 0, time.Now)
	ctx := context.Background()
	plan := testPlan(t, e, ctx, "console")
	req := testRequest(t, e, plan, testEnvelope("console", "operator", ""), "text", "ghost", "")
	if _, err := e.Submit(ctx, req); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("unknown names carry the sentinel, got %v", err)
	}
	if e.Has("ghost") {
		t.Fatal("Has must be honest about absence")
	}
}

func TestRun_perToolTimeoutBoundsExecution(t *testing.T) {
	t.Parallel()
	e := New(tool.Registry{"sleepy": sleepyTool{}}, 25*time.Millisecond, time.Now)
	ctx := context.Background()
	plan := testPlan(t, e, ctx, "console")
	req := testRequest(t, e, plan, testEnvelope("console", "operator", ""), "text", "sleepy", "")
	start := time.Now()
	_, err := e.Submit(ctx, req)
	if err == nil {
		t.Fatal("a hung tool must be cut by the per-tool timeout")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the timeout must bound the wait, took %v", elapsed)
	}
}

func TestRun_toolErrorPassesThroughUnclassified(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	e := New(tool.Registry{"plain": &plainTool{err: boom}}, 0, time.Now)
	ctx := context.Background()
	plan := testPlan(t, e, ctx, "console")
	req := testRequest(t, e, plan, testEnvelope("console", "operator", ""), "text", "plain", "z")
	result, err := e.Submit(ctx, req)
	if !errors.Is(err, boom) {
		t.Fatalf("the tool's own error must pass through for the caller to classify, got %v", err)
	}
	if result.Output != "plain:z" {
		t.Fatalf("partial output travels with the error (today's semantics), got %q", result.Output)
	}
}
