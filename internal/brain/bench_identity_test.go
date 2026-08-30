// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The re-measured toll (Etapa 2, lote 4): the FULL identified hot path —
// envelope + digest + provenance resolution + identity refs + atomic
// record WITH evidence + execute + terminal finish — against a real
// sqlite store, so the identity cost is a recorded number under the same
// <=5ms p95 ceiling the E1 spec pinned. Run with:
//
//	go test ./internal/brain/ -run '^$' -bench BenchmarkRunToolHotPathIdentified -benchtime 2s
package brain

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/tool"
)

// benchIdentifiedRecorder adapts BOTH seams exactly like the app's adapter.
type benchIdentifiedRecorder struct{ benchRecorder }

func (r benchIdentifiedRecorder) RecordAttemptIdentified(ctx context.Context, env action.Envelope, outcome, rule string, state action.State, ev action.IdentityEvidence) error {
	return r.store.RecordAttemptIdentified(ctx, env,
		actionsqlite.Decision{Outcome: outcome, Rule: rule}, state,
		actionsqlite.AttemptIdentity{
			PrincipalID:   env.Principal.PrincipalID,
			IntentID:      env.IntentID,
			AuthorityRefs: env.AuthorityRefs,
			Evidence:      ev,
		})
}

func BenchmarkRunToolHotPathIdentified(b *testing.B) {
	store, err := actionsqlite.Open(filepath.Join(b.TempDir(), "korvun.db"))
	if err != nil {
		b.Fatalf("open action store: %v", err)
	}
	defer func() { _ = store.Close() }()
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()},
		WithAgentName("bench"),
		WithActionRecorder(benchIdentifiedRecorder{benchRecorder{store: store}}),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID: action.RootIntentID,
			GrantID:  action.DeriveConfigGrant("bench", []string{"echo"}, []string{"*"}).GrantID,
		}))
	env := &envelope.Envelope{ID: "bench-env", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := a.runTool(ctx, env, nil, laneText, "echo", `{"say":"hola"}`); out == "" {
			b.Fatal("echo must produce an observation")
		}
	}
}

// BenchmarkRunToolHotPathClassified re-measures the toll with the Etapa-3
// effect engine wired on top of the identified path: classification is a
// registry lookup by name — the ceiling must not notice it.
func BenchmarkRunToolHotPathClassified(b *testing.B) {
	store, err := actionsqlite.Open(filepath.Join(b.TempDir(), "korvun.db"))
	if err != nil {
		b.Fatalf("open action store: %v", err)
	}
	defer func() { _ = store.Close() }()
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()},
		WithAgentName("bench"),
		WithActionRecorder(benchIdentifiedRecorder{benchRecorder{store: store}}),
		WithEffectClassifier(tool.BuiltinEffects),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID: action.RootIntentID,
			GrantID:  action.DeriveConfigGrant("bench", []string{"echo"}, []string{"*"}).GrantID,
		}))
	env := &envelope.Envelope{ID: "bench-env", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := a.runTool(ctx, env, nil, laneText, "echo", `{"say":"hola"}`); out == "" {
			b.Fatal("echo must produce an observation")
		}
	}
}
