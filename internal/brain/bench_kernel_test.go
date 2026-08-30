// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The measured toll (Action Kernel lote 3, spec FR-COMPAT-3): the hot path
// of one tool call, benchmarked BEFORE and AFTER the kernel adapter so the
// added cost (envelope + digest + record + finish) is a recorded number,
// bounded at <=5ms p95 by the spec. Run with:
//
//	go test ./internal/brain/ -run '^$' -bench BenchmarkRunToolHotPath -benchtime 2s
package brain

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/tool"
)

// benchEnvelope is one inbound envelope reused across iterations.
func benchEnvelope() *envelope.Envelope {
	return &envelope.Envelope{ID: "bench-env", Channel: "console"}
}

func BenchmarkRunToolHotPath(b *testing.B) {
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()})
	env := benchEnvelope()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := a.runTool(ctx, env, nil, laneText, "echo", `{"say":"hola"}`); out == "" {
			b.Fatal("echo must produce an observation")
		}
	}
}

// BenchmarkRunToolHotPathRecorded measures the FULL kernel toll: envelope +
// digest + atomic record + execute + terminal finish against a real sqlite
// store on a temp file — the number the spec bounds at <=5ms p95.
func BenchmarkRunToolHotPathRecorded(b *testing.B) {
	store, err := actionsqlite.Open(filepath.Join(b.TempDir(), "korvun.db"))
	if err != nil {
		b.Fatalf("open action store: %v", err)
	}
	defer func() { _ = store.Close() }()
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()},
		WithActionRecorder(benchRecorder{store: store}))
	env := benchEnvelope()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := a.runTool(ctx, env, nil, laneText, "echo", `{"say":"hola"}`); out == "" {
			b.Fatal("echo must produce an observation")
		}
	}
}

// benchRecorder adapts the sqlite store exactly like the app's adapter.
type benchRecorder struct{ store *actionsqlite.Store }

func (r benchRecorder) RecordAttempt(ctx context.Context, env action.Envelope, outcome, rule string, state action.State) error {
	return r.store.RecordAttempt(ctx, env, actionsqlite.Decision{Outcome: outcome, Rule: rule}, state)
}

func (r benchRecorder) Finish(ctx context.Context, id string, to action.State, at time.Time) error {
	return r.store.Finish(ctx, id, to, at)
}
