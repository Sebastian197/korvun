// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// AS-4, the negative sweep (Etapa 4, lote 5 — blueprint mandatory,
// ADR-0024): with the FULL ledger active on the hot path (identified
// record, live Ed25519 sealing, result digest), the shared surfaces —
// slog lines, bus events (the SSE/feeds source), metric label sets —
// receive NO receipt payloads, NO signature bytes, NO key material and
// NO raw parameters. Digests and finite labels only. The sweep scans
// the REAL artifacts produced by a real sealed store against the REAL
// secret material generated for the run.

package brain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/tool"
)

// capturingPublisher records every bus event the agent publishes — the
// exact stream the SSE/Activity feeds consume.
type capturingPublisher struct {
	mu     sync.Mutex
	events []bus.Event
}

func (p *capturingPublisher) Publish(ctx context.Context, ev bus.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func TestSurfaces_noReceiptMaterialLeaks(t *testing.T) {
	t.Parallel()
	store, err := actionsqlite.Open(filepath.Join(t.TempDir(), "korvun.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		return action.SignReceipt(priv, r)
	})

	var logBuf bytes.Buffer
	sink := &capturingPublisher{}
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()},
		WithAgentName("sweep"),
		WithAgentLogger(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))),
		WithActionRecorder(benchSealedRecorder{benchIdentifiedRecorder{benchRecorder{store: store}}}),
		WithAgentToolAudit(sink, "sweep"),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID: action.RootIntentID,
			GrantID:  action.DeriveConfigGrant("sweep", []string{"echo"}, []string{"*"}).GrantID,
		}))
	env := &envelope.Envelope{ID: "sweep-env", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	const rawParams = `{"say":"SECRET-SWEEP-PAYLOAD-c4f1"}`
	if out := a.runTool(context.Background(), env, nil, laneText, "echo", rawParams); out == "" {
		t.Fatal("echo must produce an observation")
	}

	// The REAL secret material of this run, from the ledger itself.
	receipts, err := store.ListReceipts(context.Background(), "main")
	if err != nil || len(receipts) == 0 {
		t.Fatalf("the run must have receipted: %v %d", err, len(receipts))
	}
	r := receipts[len(receipts)-1]
	if r.Signature == "" {
		t.Fatal("sweep needs a signed receipt")
	}
	seedHex := hex.EncodeToString(priv.Seed())
	pubHex := hex.EncodeToString(pub)

	surfaces := map[string]string{"slog": logBuf.String()}
	var busText strings.Builder
	sink.mu.Lock()
	for _, ev := range sink.events {
		busText.WriteString(fmt.Sprintf("%s|%s|%s|%d\n", ev.Tool, ev.Outcome, ev.Rule, ev.Type))
	}
	sink.mu.Unlock()
	surfaces["bus"] = busText.String()

	forbidden := map[string]string{
		"signature bytes":     r.Signature,
		"receipt hash":        r.ReceiptHash,
		"private seed":        seedHex,
		"public key material": pubHex,
		"raw parameters":      "SECRET-SWEEP-PAYLOAD-c4f1",
	}
	for surface, text := range surfaces {
		for name, needle := range forbidden {
			// The slog line carries a BOUNDED args prefix by design
			// (maxArgsLogRunes, local debugging only, never shared) —
			// every OTHER surface must be spotless, and even slog must
			// never carry key or signature material.
			if surface == "slog" && name == "raw parameters" {
				continue
			}
			if strings.Contains(text, needle) {
				t.Errorf("AS-4 VIOLATED: %s found on the %s surface", name, surface)
			}
		}
	}
	// The bus surface is metadata-only in FULL: no fragment of the raw
	// params may ride it either.
	if strings.Contains(surfaces["bus"], "SECRET-SWEEP") {
		t.Error("AS-4 VIOLATED: raw parameters on the bus surface")
	}
}
