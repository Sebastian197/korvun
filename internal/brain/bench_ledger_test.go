// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The re-measured toll WITH the ink inside (Etapa 4, lote 3, spec §19.1):
// the full identified hot path now births a SIGNED receipt inside the
// record transaction (DENIED/SHADOWED) or the terminal close
// (SUCCEEDED/FAILED) — canonicalize + sha256 + Ed25519 + chain link all
// ride the same <=5ms p95 ceiling the E1 spec pinned. Run with:
//
//	go test ./internal/brain/ -run '^$' -bench BenchmarkRunToolHotPathSealed -benchtime 2s
package brain

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/tool"
)

// benchSealedRecorder adapts all three seams exactly like the app's
// adapter: identified record, plain finish, and the optional
// FinishWithResult extension carrying the result digest.
type benchSealedRecorder struct{ benchIdentifiedRecorder }

func (r benchSealedRecorder) FinishWithResult(ctx context.Context, actionID string, to action.State, at time.Time, resultDigest string) error {
	return r.store.FinishWithResult(ctx, actionID, to, at, resultDigest)
}

func BenchmarkRunToolHotPathSealed(b *testing.B) {
	store, err := actionsqlite.Open(filepath.Join(b.TempDir(), "korvun.db"))
	if err != nil {
		b.Fatalf("open action store: %v", err)
	}
	defer func() { _ = store.Close() }()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("generate key: %v", err)
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		return action.SignReceipt(priv, r)
	})
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()},
		WithAgentName("bench"),
		WithActionRecorder(benchSealedRecorder{benchIdentifiedRecorder{benchRecorder{store: store}}}),
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
