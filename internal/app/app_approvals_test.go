// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The approvals knob and its wiring — Etapa 5, lote 3, pieza 1b (spec
// FR-GATE-1, sealed): approvals.enabled decodes strictly (unknown
// fields die; absent = OFF), and the wiring obeys it — OFF or absent
// means the recorder adapter does NOT carry the RequestApproval
// extension (the brain's sacred pin then keeps E3 byte-for-byte); ON
// wires the extension, and a real request is born WHOLE in the real
// store: parked action, approval row, sealed preview, canonical
// params, resolvable purpose. Approved-red contract.

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

func approvalsConfig(t *testing.T, dbPath string, enabled bool) *config.Config {
	t.Helper()
	cfg := kernelWiringConfig(dbPath)
	if enabled {
		cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
	}
	return cfg
}

// requester digs the approval extension out of a built app's recorder.
type requester interface {
	RequestApproval(ctx context.Context, env action.Envelope, rule string, rawParams string) (string, error)
}

func TestApprovalsKnob_absentMeansOffAndNoExtension(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	app, err := Build(approvalsConfig(t, dbPath, false), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer shutdownApp(t, app)
	if _, ok := app.recorderForTest().(requester); ok {
		t.Fatal("SACRED PIN: with approvals absent the adapter must NOT carry the extension — E3 byte-for-byte")
	}
}

func TestApprovalsKnob_enabledWiresTheExtensionAndBirthsWholeRequests(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	app, err := Build(approvalsConfig(t, dbPath, true), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer shutdownApp(t, app)
	ar, ok := app.recorderForTest().(requester)
	if !ok {
		t.Fatal("with approvals.enabled the adapter must carry the extension")
	}
	ctx := context.Background()
	env := action.NewEnvelope("act_wire1", "env-wire",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		`{"url":"https://a.example"}`, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	approvalID, err := ar.RequestApproval(ctx, env, "require_approval", `{"url":"https://a.example"}`)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if !strings.HasPrefix(approvalID, "apr_") {
		t.Fatalf("the request id mold: %q", approvalID)
	}
	store := app.actions.(*actionsqlite.Store)
	// Born whole in the REAL store: parked action + approval + preview.
	rec, err := store.Get(ctx, "act_wire1")
	if err != nil || rec.State != action.StatePendingApproval {
		t.Fatalf("parked: %v %v", err, rec.State)
	}
	a, p, err := store.GetApproval(ctx, approvalID)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	if a.ActionDigest != env.ParametersDigest || a.Reason != "require_approval" {
		t.Fatalf("approval binds the exact digest under the rule: %+v", a)
	}
	if a.ExpiresAt.IsZero() {
		t.Fatal("the request must carry the default TTL expiry")
	}
	// The preview tells the §15.2 truth from the real wiring.
	if p.Operation != "tool/webhook_call" || p.EffectClass != action.EffectWriteIrreversible {
		t.Fatalf("preview facts: %+v", p)
	}
	if p.IntentPurpose == "" {
		t.Fatal("the purpose must resolve from the stored root intent")
	}
	if p.Reversibility == "" || p.PolicyDigest == "" {
		t.Fatalf("reversibility and the pinned law must ride the preview: %+v", p)
	}
	// And the params round-trip for the deferred execution.
	params, err := store.ApprovalParams(ctx, approvalID)
	if err != nil {
		t.Fatalf("params: %v", err)
	}
	if action.Digest(env.Operation, string(params)) != a.ActionDigest {
		t.Fatal("the recoverable params must re-derive the approved digest")
	}
}

func TestApprovalsKnob_strictDecode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version":1,"approvals":{"enabled":true,"bogus":1},
		"brains":[{"name":"a","sensitivity":"public","policy":{"kind":"priority"},
		"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}]}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := config.Load(bad); err == nil {
		t.Fatal("unknown approvals fields must die at decode")
	}
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`{"schema_version":1,"approvals":{"enabled":true},
		"brains":[{"name":"a","sensitivity":"public","policy":{"kind":"priority"},
		"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}]}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := config.Load(good)
	if err != nil {
		t.Fatalf("valid approvals block: %v", err)
	}
	if cfg.Approvals == nil || !cfg.Approvals.Enabled {
		t.Fatalf("the knob must decode: %+v", cfg.Approvals)
	}
}
