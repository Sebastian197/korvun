// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// A bare deadline on the brain path — P1-3 of the v0.15.0 external review.
//
// The approved path closes every context.DeadlineExceeded OUTCOME_UNKNOWN. The
// brain path only did so when the tool also wrapped tool.ErrEffectDelivered, so
// a tool that produced its effect and then outlived its deadline closed FAILED:
// the same definite claim over the same effect, on the path that runs when the
// gate does not park.
//
// Evidence level, honest: in-process, through the production door, Handle, with
// a scripted model that asks for the tool, and the REAL executor's per-tool
// timeout. Not a compiled binary.
package brain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/tool"
)

// effectThenWaitTool is the tool the review describes: it produces its effect,
// waits for ctx.Done() and returns ctx.Err().
type effectThenWaitTool struct{ effect atomic.Bool }

func (*effectThenWaitTool) Name() string { return "webhook_call" }
func (*effectThenWaitTool) Description() string {
	return "produces its effect, then outlives its deadline"
}
func (e *effectThenWaitTool) Execute(ctx context.Context, _ string) (string, error) {
	e.effect.Store(true)
	<-ctx.Done()
	return "", ctx.Err()
}

// TestRunTool_aBareDeadlineAfterTheEffectClosesUnknown drives that tool with a
// short per-tool timeout through the brain path.
//
// Probing mutation (executed, red, declared in the canto): restore the close
// that only reads tool.ErrEffectDelivered, so the bare deadline closes FAILED ⇒
// this reddens. The reverse direction stays pinned by
// TestRunTool_aDialThatNeverConnectsStillClosesFailed.
func TestRunTool_aBareDeadlineAfterTheEffectClosesUnknown(t *testing.T) {
	t.Parallel()
	fx := &effectThenWaitTool{}
	journal := &[]string{}
	rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	m := &scriptedModel{name: "m", replies: []string{
		`TOOL: webhook_call(https://hooks.example {"event":"ping"})`, "done",
	}}
	a := NewAgentBrain(m, tool.Registry{"webhook_call": fx},
		WithAgentLogger(quietLogger()), WithActionRecorder(rec),
		WithAgentPerToolTimeout(20*time.Millisecond))

	if _, err := a.Handle(context.Background(), inboundText("telegram", "c", "go")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if !fx.effect.Load() {
		t.Fatal("the tool never produced its effect — this is not the scenario")
	}
	if len(rec.finishes) != 1 {
		t.Fatalf("the attempt must close exactly once: %v", rec.finishes)
	}
	if rec.finishes[0] != action.StateOutcomeUnknown {
		t.Fatalf("state = %v, want OUTCOME_UNKNOWN — the effect may have happened and the deadline hid its answer", rec.finishes[0])
	}
}

// effectThenCancelledTool produces its effect, signals it, and waits for a
// cancellation that someone else delivers.
type effectThenCancelledTool struct {
	done chan struct{}
	once sync.Once
}

func (*effectThenCancelledTool) Name() string { return "webhook_call" }
func (*effectThenCancelledTool) Description() string {
	return "produces its effect, then sees its context cancelled"
}
func (e *effectThenCancelledTool) Execute(ctx context.Context, _ string) (string, error) {
	e.once.Do(func() { close(e.done) })
	<-ctx.Done()
	return "", ctx.Err()
}

// TestRunTool_aCancellationAfterTheEffectClosesUnknown is the sister the diff's
// adversary found: the same tool shape with context.Canceled instead of the
// deadline. The per-tool timeout is long, so only the cancellation can end the
// call.
//
// Probing mutation (executed, red, declared in the canto): drop the
// context.Canceled arm from closeState ⇒ this reddens.
func TestRunTool_aCancellationAfterTheEffectClosesUnknown(t *testing.T) {
	t.Parallel()
	fx := &effectThenCancelledTool{done: make(chan struct{})}
	journal := &[]string{}
	rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	m := &scriptedModel{name: "m", replies: []string{
		`TOOL: webhook_call(https://hooks.example {"event":"ping"})`, "done",
	}}
	a := NewAgentBrain(m, tool.Registry{"webhook_call": fx},
		WithAgentLogger(quietLogger()), WithActionRecorder(rec),
		WithAgentPerToolTimeout(10*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-fx.done
		cancel()
	}()
	_, _ = a.Handle(ctx, inboundText("telegram", "c", "go"))

	select {
	case <-fx.done:
	default:
		t.Fatal("the tool never produced its effect — this is not the scenario")
	}
	if len(rec.finishes) != 1 {
		t.Fatalf("the attempt must close exactly once: %v", rec.finishes)
	}
	if rec.finishes[0] != action.StateOutcomeUnknown {
		t.Fatalf("state = %v, want OUTCOME_UNKNOWN — the effect happened and the cancellation hid its answer", rec.finishes[0])
	}
}
