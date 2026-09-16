// Copyright 2026 Sebastián Moreno Saavedra

// SPDX-License-Identifier: Apache-2.0

// The close after the effect does not depend on the caller's context — the
// fourth class cure the director ordered on 2026-09-16, with the REAL SQLite
// recorder on both paths.
//
// The record of an attempt lands BEFORE the effect and the terminal close
// comes after it, so the window between them is exactly where an irreversible
// effect lives unrecorded. Both paths passed the caller's ctx into that close.
// A context that ends while the tool runs — a cancelled request, a shutdown —
// therefore took the close down with it: database/sql refuses a query on a
// dead context before it reaches the driver, so the effect happened and the
// ledger kept none of it. The row was left mid-flight for the recovery pass,
// which is a worse answer than the one the store can give at once.
//
// Evidence level, honest: in-process, a real boot and a real SQLite file,
// through the app's own recorder — the same adapter wire() hands the brains.
// Not a compiled binary and not a real crash.
package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/tool"
)

// effectThenCancelTool produces its effect, announces it, and then waits for
// the context someone else is about to end.
type effectThenCancelTool struct {
	fired chan struct{}
	once  sync.Once
}

func (*effectThenCancelTool) Name() string        { return "webhook_call" }
func (*effectThenCancelTool) Description() string { return "fires, then loses its context" }
func (e *effectThenCancelTool) Execute(ctx context.Context, _ string) (string, error) {
	e.once.Do(func() { close(e.fired) })
	<-ctx.Done()
	return "", ctx.Err()
}

// oneToolModel asks for webhook_call once, then answers plainly.
type oneToolModel struct {
	mu    sync.Mutex
	calls int
}

func (m *oneToolModel) Name() string { return "fake" }
func (m *oneToolModel) Generate(_ context.Context, _ *model.Request) (*model.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	reply := "done"
	if m.calls == 1 {
		reply = `TOOL: webhook_call(https://hooks.example {"event":"ping"})`
	}
	return &model.Response{
		Message:  model.Message{Role: model.RoleAssistant, Content: reply},
		Provider: "fake",
	}, nil
}

func quietTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestExecuteApprovedAction_theCloseOutlivesTheCallersContext is the approved
// path's half.
//
// Probing mutation (executed, red, declared in the canto): pass the caller's
// ctx to FinishWithResult again ⇒ the close fails with «context canceled», the
// action stays APPROVED and this reddens.
func TestExecuteApprovedAction_theCloseOutlivesTheCallersContext(t *testing.T) {
	store, _, _, approvalID := approvedFlow(t)
	digest := digestOf(t, store, approvalID)
	actionID := actionOf(t, store, approvalID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fx := &effectThenCancelTool{fired: make(chan struct{})}
	go func() {
		<-fx.fired
		cancel()
	}()
	exec := executor.New(tool.Registry{"webhook_call": fx}, 0, time.Now)

	run, err := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
	if err != nil {
		t.Fatalf("the close must land despite the dead context: %v", err)
	}
	if !run.Unknown {
		t.Fatalf("Unknown = false over an effect whose answer was lost: %+v", run)
	}
	rec, gerr := store.Get(context.Background(), actionID)
	if gerr != nil {
		t.Fatalf("read the action back: %v", gerr)
	}
	if rec.State == action.StateApproved {
		t.Fatal("the action was left mid-flight: the ledger kept nothing of an effect that happened")
	}
	if rec.State != action.StateOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", rec.State)
	}
}

// TestBrainPath_theCloseOutlivesTheCallersContext is the brain path's half,
// against the SAME real store through the recorder wire() hands the brains.
//
// Probing mutation (executed, red, declared in the canto): pass the caller's
// ctx to finishAction again ⇒ the attempt stays AUTHORIZED and this reddens.
func TestBrainPath_theCloseOutlivesTheCallersContext(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	app, err := Build(approvalsConfig(t, dbPath, false), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { shutdownApp(t, app) })
	store := app.actions.(*actionsqlite.Store)

	fx := &effectThenCancelTool{fired: make(chan struct{})}
	a := brain.NewAgentBrain(&oneToolModel{}, tool.Registry{"webhook_call": fx},
		brain.WithAgentLogger(quietTestLogger()),
		brain.WithActionRecorder(app.recorderForTest()),
		brain.WithAgentPerToolTimeout(30*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-fx.fired
		cancel()
	}()
	env := envelope.New("console", envelope.Inbound, envelope.Participant{ID: "user1", Name: "User"})
	env.AddText("avisa al receptor")
	_, _ = a.Handle(ctx, env)

	select {
	case <-fx.fired:
	default:
		t.Fatal("the tool never produced its effect — this is not the scenario")
	}
	recs, lerr := store.ListByOperation(context.Background(), "tool", "webhook_call")
	if lerr != nil {
		t.Fatalf("list the attempt: %v", lerr)
	}
	if len(recs) != 1 {
		t.Fatalf("attempts recorded = %d, want exactly 1", len(recs))
	}
	if recs[0].State == action.StateAuthorized {
		t.Fatal("the attempt was left AUTHORIZED: the ledger kept nothing of an effect that happened")
	}
	if recs[0].State != action.StateOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", recs[0].State)
	}
}
