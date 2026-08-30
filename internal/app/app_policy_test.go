// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The versioned Policy Decision — Etapa 3, lote 4, pieza 2 (spec FR-POL):
// every gate decision pins the EXACT law that took it, and the three
// blueprint-mandatory contracts hold end to end through the public
// Handle path against a REAL store: changed arguments force a NEW
// decision; a reload NEVER alters recorded decisions; read and write
// receive different outcomes under the SAME pinned law. Plus the full
// §10.7 traceability: who (E2) + intent (E2) + authority (E2) + which
// exact law decided (E3), in real rows. Approved-red contract.

package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/tool"
)

// scriptedPolicyModel replays fixed replies through the public Model
// seam — the app-side twin of the brain's test model.
type scriptedPolicyModel struct {
	replies []string
	calls   int
}

func (m *scriptedPolicyModel) Generate(_ context.Context, _ *model.Request) (*model.Response, error) {
	reply := "done"
	if m.calls < len(m.replies) {
		reply = m.replies[m.calls]
	}
	m.calls++
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: reply}}, nil
}

func (m *scriptedPolicyModel) Name() string { return "scripted" }

// echoPolicyTool answers under any registered name.
type echoPolicyTool struct{ name string }

func (t echoPolicyTool) Name() string        { return t.name }
func (t echoPolicyTool) Description() string { return "policy test tool" }
func (t echoPolicyTool) Execute(ctx context.Context, args string) (string, error) {
	return "ok", nil
}

func TestPolicyDigest_coversGovernanceAndTheEffectSnapshot(t *testing.T) {
	t.Parallel()
	gov := []config.ToolGrantConfig{{Tool: "calc", Mode: "allow"}}
	effects := map[string]action.EffectDescriptor{
		"calc": {Class: action.EffectPure},
	}
	first := policyDigestFrom(gov, effects)
	if !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("the law digest carries the pinned algorithm, got %q", first)
	}
	if policyDigestFrom(gov, effects) != first {
		t.Fatal("the law digest must be deterministic")
	}
	tightened := []config.ToolGrantConfig{{Tool: "calc", Mode: "deny"}}
	if policyDigestFrom(tightened, effects) == first {
		t.Fatal("a governance change is a DIFFERENT law")
	}
	reclassified := map[string]action.EffectDescriptor{
		"calc": {Class: action.EffectCritical},
	}
	if policyDigestFrom(gov, reclassified) == first {
		t.Fatal("an effect-registry change is a DIFFERENT law")
	}
}

// policyHarness wires a real agent brain over a REAL store with the
// pinned adapter — the exact production shape, scripted model aside.
func policyHarness(t *testing.T, store *actionsqlite.Store, pin actionsqlite.PolicyPin, replies []string, ceiling action.EffectClass) *brain.AgentBrain {
	t.Helper()
	registry := tool.Registry{
		"calc":         echoPolicyTool{name: "calc"},
		"webhook_call": echoPolicyTool{name: "webhook_call"},
	}
	identity := brain.ActionIdentity{
		Registry: action.ProvenanceRegistry{
			"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
		},
		IntentID:      action.RootIntentID,
		EffectCeiling: ceiling,
	}
	return brain.NewAgentBrain(&scriptedPolicyModel{replies: replies}, registry,
		brain.WithAgentName("pol"),
		brain.WithActionRecorder(actionRecorder{store: store, pin: pin}),
		brain.WithEffectClassifier(tool.BuiltinEffects),
		brain.WithActionIdentity(identity),
	)
}

func policyEnv(id string) *envelope.Envelope {
	e := envelope.New("console", envelope.Inbound, envelope.Participant{ID: "console-user"})
	e.ID = id
	return e.AddText("go")
}

func openPolicyStore(t *testing.T) *actionsqlite.Store {
	t.Helper()
	store, err := actionsqlite.Open(filepath.Join(t.TempDir(), "korvun.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func decisionsOf(t *testing.T, store *actionsqlite.Store) []actionsqlite.Record {
	t.Helper()
	recs, err := store.ListByOperation(context.Background(), "tool", "calc")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return recs
}

// AS-args (blueprint mandatory 1): changed arguments change the digest
// and force a NEW decision — one row per attempt, never reuse.
func TestPolicy_changedArgumentsForceANewDecision(t *testing.T) {
	t.Parallel()
	store := openPolicyStore(t)
	pin := actionsqlite.PolicyPin{Version: 100, Digest: "sha256:law-a"}
	a := policyHarness(t, store, pin,
		[]string{"TOOL: calc(2+2)", "done", "TOOL: calc(3+3)", "done"}, "")
	if _, err := a.Handle(context.Background(), policyEnv("env-a1")); err != nil {
		t.Fatalf("handle 1: %v", err)
	}
	if _, err := a.Handle(context.Background(), policyEnv("env-a2")); err != nil {
		t.Fatalf("handle 2: %v", err)
	}
	recs := decisionsOf(t, store)
	if len(recs) != 2 {
		t.Fatalf("two attempts, two decisions, got %d", len(recs))
	}
	if recs[0].Envelope.ParametersDigest == recs[1].Envelope.ParametersDigest {
		t.Fatal("changed arguments must change the digest")
	}
	for _, rec := range recs {
		if rec.Decision.PolicyDigest != "sha256:law-a" || rec.Decision.PolicyVersion != 100 {
			t.Fatalf("both decisions pin the law that took them, got %+v", rec.Decision)
		}
	}
}

// AS-reload (blueprint mandatory 2): a reload NEVER alters recorded
// decisions — the old row keeps its pinned law; the next attempt pins
// the new one.
func TestPolicy_reloadNeverAltersRecordedDecisions(t *testing.T) {
	t.Parallel()
	store := openPolicyStore(t)
	before := policyHarness(t, store, actionsqlite.PolicyPin{Version: 1, Digest: "sha256:law-v1"},
		[]string{"TOOL: calc(1)", "done"}, "")
	if _, err := before.Handle(context.Background(), policyEnv("env-r1")); err != nil {
		t.Fatalf("handle before: %v", err)
	}
	// The reload: a tightened config yields a NEW pinned law.
	after := policyHarness(t, store, actionsqlite.PolicyPin{Version: 2, Digest: "sha256:law-v2"},
		[]string{"TOOL: calc(1)", "done"}, "")
	if _, err := after.Handle(context.Background(), policyEnv("env-r2")); err != nil {
		t.Fatalf("handle after: %v", err)
	}
	recs := decisionsOf(t, store)
	if len(recs) != 2 {
		t.Fatalf("want 2 rows, got %d", len(recs))
	}
	if recs[0].Decision.PolicyDigest != "sha256:law-v1" || recs[0].Decision.PolicyVersion != 1 {
		t.Fatalf("the recorded decision must NOT move on reload: %+v", recs[0].Decision)
	}
	if recs[1].Decision.PolicyDigest != "sha256:law-v2" || recs[1].Decision.PolicyVersion != 2 {
		t.Fatalf("the next attempt pins the new law: %+v", recs[1].Decision)
	}
}

// AS-rw (blueprint mandatory 3): read and write receive DIFFERENT
// outcomes under the SAME pinned law — the class decides, the law is one.
func TestPolicy_readAndWriteReceiveDifferentPolicyUnderOneLaw(t *testing.T) {
	t.Parallel()
	store := openPolicyStore(t)
	pin := actionsqlite.PolicyPin{Version: 7, Digest: "sha256:one-law"}
	a := policyHarness(t, store, pin,
		[]string{"TOOL: calc(2+2)", "done", "TOOL: webhook_call({})", "done"},
		action.EffectReadExternal)
	if _, err := a.Handle(context.Background(), policyEnv("env-rw1")); err != nil {
		t.Fatalf("handle read: %v", err)
	}
	if _, err := a.Handle(context.Background(), policyEnv("env-rw2")); err != nil {
		t.Fatalf("handle write: %v", err)
	}
	reads := decisionsOf(t, store)
	if len(reads) != 1 || reads[0].State != action.StateSucceeded {
		t.Fatalf("the read (pure calc under a read ceiling) executes: %+v", reads)
	}
	writes, err := store.ListByOperation(context.Background(), "tool", "webhook_call")
	if err != nil {
		t.Fatalf("list writes: %v", err)
	}
	if len(writes) != 1 || writes[0].State != action.StateDenied {
		t.Fatalf("the write dies at the ceiling: %+v", writes)
	}
	if writes[0].Decision.Rule != "effect_ceiling" {
		t.Fatalf("write rule = %q", writes[0].Decision.Rule)
	}
	if reads[0].Decision.PolicyDigest != "sha256:one-law" ||
		writes[0].Decision.PolicyDigest != "sha256:one-law" {
		t.Fatal("ONE law, two outcomes: both rows pin the same digest")
	}
}

// The §10.7 promise in real rows: who + intent + authority + the exact
// law, on one receipt.
func TestPolicy_fullTraceabilityOnOneReceipt(t *testing.T) {
	t.Parallel()
	store := openPolicyStore(t)
	pin := actionsqlite.PolicyPin{Version: 9, Digest: "sha256:full-law"}
	a := policyHarness(t, store, pin, []string{"TOOL: calc(5*5)", "done"}, "")
	if _, err := a.Handle(context.Background(), policyEnv("env-t1")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	recs := decisionsOf(t, store)
	if len(recs) != 1 {
		t.Fatalf("one receipt, got %d", len(recs))
	}
	rec := recs[0]
	if rec.Identity == nil || rec.Identity.PrincipalID != action.BrainPrincipal("pol").PrincipalID {
		t.Fatalf("WHO must be on the receipt: %+v", rec.Identity)
	}
	if rec.Identity.IntentID != action.RootIntentID {
		t.Fatalf("UNDER WHICH INTENT must be on the receipt: %q", rec.Identity.IntentID)
	}
	if rec.Decision.PolicyDigest != "sha256:full-law" || rec.Decision.PolicyVersion != 9 {
		t.Fatalf("WHICH EXACT LAW must be on the receipt: %+v", rec.Decision)
	}
	if rec.Envelope.Effect.Class != string(action.EffectPure) {
		t.Fatalf("the woken class rides the same receipt: %q", rec.Envelope.Effect.Class)
	}
	evidence, err := store.GetEvidence(context.Background(), rec.Envelope.ActionID)
	if err != nil {
		t.Fatalf("the evidence rides too: %v", err)
	}
	if evidence.Credential != action.CredentialLoopbackInProcess {
		t.Fatalf("evidence credential: %q", evidence.Credential)
	}
}
