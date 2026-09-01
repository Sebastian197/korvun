// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The persisted approval — Etapa 5, lote 2, pieza 2 (spec FR-APR/
// FR-STATE-2, sealed NC-2): migration v6→v7 brings the approvals table
// on the anti-zombie runner; the REQUEST IS BORN WHOLE (action parked
// PENDING_APPROVAL + approval + sealed preview in ONE transaction,
// with the stored envelope's integrity pinned — what will execute on
// approve is THAT object); the decision is ONE-SHOT ATOMIC under a
// -race hammer (exactly one winner; losers get
// approval_already_decided); every decision act leaves its E4 operator
// receipt in the SAME transaction as the transition — together or
// nothing; expiry is judged at the consume touch (E2 mold, no
// sweeper). Approved-red contract.

package sqlite

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// rawV6Delta is the v5→v6 DDL restated literally (oracle discipline).
const rawV6Delta = `
CREATE TABLE receipts (
    receipt_id            TEXT    NOT NULL PRIMARY KEY,
    action_id             TEXT    NOT NULL,
    intent_digest         TEXT    NOT NULL,
    principal_id          TEXT    NOT NULL,
    authority_digest      TEXT    NOT NULL DEFAULT '',
    decision_digest       TEXT    NOT NULL,
    action_digest         TEXT    NOT NULL,
    effect_class          TEXT    NOT NULL,
    attempt               INTEGER NOT NULL,
    outcome               TEXT    NOT NULL,
    result_digest         TEXT    NOT NULL DEFAULT '',
    started_at            TEXT,
    finished_at           TEXT,
    partition             TEXT    NOT NULL,
    chain_seq             INTEGER NOT NULL,
    previous_receipt_hash TEXT    NOT NULL,
    receipt_hash          TEXT    NOT NULL,
    signing_key_id        TEXT    NOT NULL,
    signature             TEXT    NOT NULL,
    UNIQUE (partition, chain_seq)
) WITHOUT ROWID;
CREATE INDEX receipts_by_action ON receipts(action_id);
UPDATE action_schema SET version = 6;`

// buildV6File hand-builds a real v6 store file.
func buildV6File(t *testing.T) string {
	t.Helper()
	path := buildV5File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(rawV6Delta); err != nil {
		t.Fatalf("build v6 delta: %v", err)
	}
	return path
}

func testApprovalFor(env action.Envelope) (action.Approval, action.ActionPreview) {
	preview := action.ActionPreview{
		ActionID:      env.ActionID,
		SchemaVersion: 1,
		IntentPurpose: "semana de pruebas",
		PrincipalID:   "principal_brain_asistente",
		Operation:     "tool/webhook_call",
		Resources:     []string{"https://hooks.example"},
		DataEgress:    "request body leaves to the webhook target",
		ArgsDigest:    env.ParametersDigest,
		CostLine:      "1 of 5",
		EffectClass:   action.EffectWriteIrreversible,
		Reversibility: "irreversible",
		ToolCage:      "webhook cage",
		PolicyVersion: 7,
		PolicyDigest:  "sha256:law",
		RequiredRule:  "require_approval",
	}
	a := action.Approval{
		ApprovalID:    action.NewApprovalID(),
		SchemaVersion: 1,
		ActionID:      env.ActionID,
		ActionDigest:  env.ParametersDigest,
		PreviewDigest: preview.Digest(),
		RequestedFrom: action.OperatorPrincipal().PrincipalID,
		Reason:        "require_approval",
		RiskSummary:   "write_irreversible — no documented undo",
		PolicyVersion: 7,
		PolicyDigest:  "sha256:law",
		RequestedAt:   env.RequestedAt,
		ExpiresAt:     env.RequestedAt.Add(time.Hour),
		Status:        action.ApprovalPending,
	}
	return a, preview
}

// operatorDecisionEnv builds the operator's own decision-act envelope.
func operatorDecisionEnv(name, approvalID string) (action.Envelope, AttemptIdentity) {
	env := action.NewEnvelope(action.NewID(), "cli",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: "approval", Name: name, Version: 1},
		`{"approval_id":"`+approvalID+`"}`, time.Now().UTC())
	env.Principal = action.PrincipalRef{PrincipalID: action.OperatorPrincipal().PrincipalID}
	env.IntentID = action.RootIntentID
	return env, AttemptIdentity{
		PrincipalID: action.OperatorPrincipal().PrincipalID,
		IntentID:    action.RootIntentID,
	}
}

func pendingRequest(t *testing.T, store *Store, id string) (action.Approval, action.ActionPreview) {
	t.Helper()
	env := testEnvelope(id)
	a, p := testApprovalFor(env)
	if err := store.CreateApprovalRequest(context.Background(), env,
		Decision{Outcome: "require_approval", Rule: "require_approval", PolicyVersion: 7, PolicyDigest: "sha256:law"},
		a, p, `{"a":1}`); err != nil {
		t.Fatalf("create request: %v", err)
	}
	return a, p
}

func TestMigrationV7_freshAndCrash(t *testing.T) {
	t.Parallel()
	// Fresh file lands on current with the approvals table.
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if v, _ := store.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("fresh store lands on current, got %d", v)
	}
	_ = store.Close()
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='approvals'`); n != 1 {
		t.Fatalf("the approvals table must exist, got %d", n)
	}
	// AS-8 crash mold against a HAND-BUILT v6.
	crashPath := buildV6File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(crashPath)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v7 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v7'); END;`); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	_ = db.Close()
	if _, err := Open(crashPath); err == nil {
		t.Fatal("an aborted v7 migration must be boot-fatal")
	}
	if v := inspect(t, crashPath, `SELECT version FROM action_schema`); v != 6 {
		t.Fatalf("aborted migration must leave version 6, got %d", v)
	}
	if n := inspect(t, crashPath,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='approvals'`); n != 0 {
		t.Fatalf("ZOMBIE: approvals survived the rollback (%d)", n)
	}
	db2, _ := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(crashPath)))
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v7`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	_ = db2.Close()
	recovered, err := Open(crashPath)
	if err != nil {
		t.Fatalf("the next boot must complete: %v", err)
	}
	defer func() { _ = recovered.Close() }()
	if v, _ := recovered.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("completed migration must land on current, got %v", v)
	}
}

func TestApproval_theRequestIsBornWholeOrNotAtAll(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a, p := pendingRequest(t, store, "act_apr1")
	// The action parked PENDING_APPROVAL — not terminal, NO receipt.
	rec, err := store.Get(ctx, "act_apr1")
	if err != nil || rec.State != action.StatePendingApproval {
		t.Fatalf("the action parks PENDING_APPROVAL: %v %v", err, rec.State)
	}
	if receipts, _ := store.ReceiptsByAction(ctx, "act_apr1"); len(receipts) != 0 {
		t.Fatalf("PENDING_APPROVAL is not terminal — no receipt yet, got %d", len(receipts))
	}
	// NC-2's promise pinned: the STORED request is whole — what will
	// execute on approve is THAT object: the envelope row plus the
	// canonical parameters kept WITH the parked request (the E1 no-raw
	// law holds for resting history; a parked request IS pending work,
	// and its params are purged at any close without execution).
	if rec.Envelope.ParametersDigest != a.ActionDigest {
		t.Fatal("the stored envelope must round-trip with its exact digest")
	}
	params, err := store.ApprovalParams(ctx, a.ApprovalID)
	if err != nil {
		t.Fatalf("the canonical params must be recoverable: %v", err)
	}
	if action.Digest(rec.Envelope.Operation, string(params)) != a.ActionDigest {
		t.Fatal("the recovered params must re-derive the EXACT digest the human approves")
	}
	// The approval and its sealed preview round-trip.
	got, gotPreview, err := store.GetApproval(ctx, a.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if got.Status != action.ApprovalPending || got.ActionDigest != a.ActionDigest {
		t.Fatalf("approval round-trip: %+v", got)
	}
	if gotPreview.Digest() != p.Digest() {
		t.Fatal("the sealed preview must round-trip digest-identical")
	}
	// Together or nothing: a blocked approvals insert rolls back the
	// WHOLE birth — no parked action either.
	store2, _ := sealedStore(t)
	if _, err := store2.db.Exec(
		`CREATE TRIGGER block_apr BEFORE INSERT ON approvals
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	env2 := testEnvelope("act_apr2")
	a2, p2 := testApprovalFor(env2)
	if err := store2.CreateApprovalRequest(ctx, env2,
		Decision{Outcome: "require_approval", Rule: "require_approval"}, a2, p2, `{"a":1}`); err == nil {
		t.Fatal("a blocked approval insert must fail the whole birth")
	}
	if _, err := store2.Get(ctx, "act_apr2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the parked action must not exist after rollback: %v", err)
	}
}

func TestApproval_decideOneShotAtomicUnderTheHammer(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_hammer")
	const deciders = 24
	var wins, losses int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < deciders; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			env, ident := operatorDecisionEnv("approve", a.ApprovalID)
			rule, err := store.decideApproval(ctx, a.ApprovalID, "approved",
				a.RequestedAt.Add(30*time.Minute), env, ident, "")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && rule == "":
				wins++
			case rule == action.RuleApprovalAlreadyDecided:
				losses++
			default:
				t.Errorf("decider %d: unexpected rule=%q err=%v", n, rule, err)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || losses != deciders-1 {
		t.Fatalf("EXACTLY one winner: wins=%d losses=%d", wins, losses)
	}
	rec, err := store.Get(ctx, "act_hammer")
	if err != nil || rec.State != action.StateApproved {
		t.Fatalf("the parked action moved to APPROVED once: %v %v", err, rec.State)
	}
	got, _, _ := store.GetApproval(ctx, a.ApprovalID)
	if got.Status != action.ApprovalApproved || got.DecisionReceiptID == "" {
		t.Fatalf("consumed approval with its proof of decision: %+v", got)
	}
}

func TestApproval_rejectClosesWithBothReceiptsOrNothing(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_rej")
	env, ident := operatorDecisionEnv("reject", a.ApprovalID)
	rule, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(30*time.Minute), env, ident, "not today")
	if err != nil || rule != "" {
		t.Fatalf("reject: %q %v", rule, err)
	}
	// The parked action closed REJECTED — terminal, WITH its receipt.
	rec, _ := store.Get(ctx, "act_rej")
	if rec.State != action.StateRejected {
		t.Fatalf("action state: %v", rec.State)
	}
	actionReceipts, _ := store.ReceiptsByAction(ctx, "act_rej")
	if len(actionReceipts) != 1 || actionReceipts[0].Outcome != string(action.StateRejected) {
		t.Fatalf("the terminal REJECTED births its receipt: %+v", actionReceipts)
	}
	if err := action.VerifyReceiptSignature(pub, actionReceipts[0]); err != nil {
		t.Fatalf("verify action receipt: %v", err)
	}
	// The operator's decision act left ITS receipt, referenced as the
	// proof of decision, in the same stroke.
	got, _, _ := store.GetApproval(ctx, a.ApprovalID)
	if got.Status != action.ApprovalRejected || got.Comment != "not today" {
		t.Fatalf("approval closed: %+v", got)
	}
	proof, err := store.GetReceipt(ctx, got.DecisionReceiptID)
	if err != nil {
		t.Fatalf("the proof of decision must be a real ledger receipt: %v", err)
	}
	if err := action.VerifyReceiptSignature(pub, proof); err != nil {
		t.Fatalf("verify proof receipt: %v", err)
	}
	// Together or nothing: with receipts blocked, the WHOLE decision
	// rolls back — the approval stays PENDING.
	store2, _ := sealedStore(t)
	a2, _ := pendingRequest(t, store2, "act_rej2")
	if _, err := store2.db.Exec(
		`CREATE TRIGGER block_rcpt BEFORE INSERT ON receipts
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	env2, ident2 := operatorDecisionEnv("reject", a2.ApprovalID)
	if _, err := store2.decideApproval(ctx, a2.ApprovalID, "rejected",
		a2.RequestedAt.Add(30*time.Minute), env2, ident2, ""); err == nil {
		t.Fatal("an unreceiptable decision must fail whole")
	}
	still, _, _ := store2.GetApproval(ctx, a2.ApprovalID)
	if still.Status != action.ApprovalPending {
		t.Fatalf("the failed decision must roll back whole: %+v", still.Status)
	}
	rec2, _ := store2.Get(ctx, "act_rej2")
	if rec2.State != action.StatePendingApproval {
		t.Fatalf("the parked action must stay parked: %v", rec2.State)
	}
}

func TestApproval_expiryJudgedAtTheTouch(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_exp")
	// No sweeper: before any touch, everything sits untouched.
	before, _, _ := store.GetApproval(ctx, a.ApprovalID)
	if before.Status != action.ApprovalPending {
		t.Fatalf("no sweeper may have moved it: %+v", before.Status)
	}
	// The touch arrives PAST the window: the E2 clock rule closes it.
	late := a.ExpiresAt.Add(time.Minute)
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	rule, err := store.decideApproval(ctx, a.ApprovalID, "approved", late, env, ident, "")
	if err != nil {
		t.Fatalf("expiry close: %v", err)
	}
	if rule != action.RuleApprovalExpired {
		t.Fatalf("the touch past the window returns approval_expired, got %q", rule)
	}
	got, _, _ := store.GetApproval(ctx, a.ApprovalID)
	if got.Status != action.ApprovalExpired {
		t.Fatalf("approval closes EXPIRED: %+v", got.Status)
	}
	rec, _ := store.Get(ctx, "act_exp")
	if rec.State != action.StateRejected {
		t.Fatalf("the parked action closes REJECTED: %v", rec.State)
	}
	receipts, _ := store.ReceiptsByAction(ctx, "act_exp")
	if len(receipts) != 1 {
		t.Fatalf("the expired close births the action's terminal receipt: %d", len(receipts))
	}
	// An expired approval NEVER executes: nothing moved to APPROVED —
	// and the raw canonical params are PURGED at the unexecuted close.
	if _, err := store.ApprovalParams(ctx, a.ApprovalID); err == nil {
		t.Fatal("params must be purged at a close without execution")
	}
}

func TestApproval_finiteDecisionsFailClosed(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_fin")
	env, ident := operatorDecisionEnv("burn", a.ApprovalID)
	if _, err := store.decideApproval(context.Background(), a.ApprovalID, "burn",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err == nil {
		t.Fatal("an unknown decision verb must fail closed")
	}
	if _, err := store.decideApproval(context.Background(), "apr_ghost", "approved",
		a.RequestedAt.Add(time.Minute), env, ident, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a ghost approval id reports not-found: %v", err)
	}
}

func TestApproval_listSurface(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a1, _ := pendingRequest(t, store, "act_l1")
	a2, _ := pendingRequest(t, store, "act_l2")
	env, ident := operatorDecisionEnv("reject", a2.ApprovalID)
	if _, err := store.decideApproval(ctx, a2.ApprovalID, "rejected",
		a2.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	pending, err := store.ListApprovals(ctx, action.ApprovalPending)
	if err != nil || len(pending) != 1 || pending[0].ApprovalID != a1.ApprovalID {
		t.Fatalf("pending list: %v %+v", err, pending)
	}
	rejected, err := store.ListApprovals(ctx, action.ApprovalRejected)
	if err != nil || len(rejected) != 1 {
		t.Fatalf("rejected list: %v %d", err, len(rejected))
	}
}

func TestApproval_deepErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, _ := sealedStore(t)
	env := testEnvelope("act_deep")
	a, p := testApprovalFor(env)
	// The birth integrity pins, one by one.
	if err := store.CreateApprovalRequest(ctx, env, Decision{}, a, p, `{"wrong":true}`); err == nil {
		t.Fatal("params that do not derive the envelope digest must refuse the birth")
	}
	badBind := a
	badBind.ActionDigest = "sha256:other"
	if err := store.CreateApprovalRequest(ctx, env, Decision{}, badBind, p, `{"a":1}`); err == nil {
		t.Fatal("an approval binding a different digest must refuse the birth")
	}
	badID := a
	badID.ActionID = "act_other"
	if err := store.CreateApprovalRequest(ctx, env, Decision{}, badID, p, `{"a":1}`); err == nil {
		t.Fatal("mismatched ids across request parts must refuse the birth")
	}
	// A duplicate request for the same action (UNIQUE action_id).
	if err := store.CreateApprovalRequest(ctx, env, Decision{Outcome: "require_approval", Rule: "require_approval"}, a, p, `{"a":1}`); err != nil {
		t.Fatalf("first birth: %v", err)
	}
	dup := a
	dup.ApprovalID = action.NewApprovalID()
	if err := store.CreateApprovalRequest(ctx, env, Decision{}, dup, p, `{"a":1}`); err == nil {
		t.Fatal("one request per parked action — the duplicate must refuse")
	}
	// Closed store: loud failures on every surface.
	closed, _ := openTemp(t)
	_ = closed.Close()
	if err := closed.CreateApprovalRequest(ctx, env, Decision{}, a, p, `{"a":1}`); err == nil {
		t.Fatal("closed store create must fail loud")
	}
	if _, _, err := closed.GetApproval(ctx, a.ApprovalID); err == nil {
		t.Fatal("closed store get must fail loud")
	}
	if _, err := closed.ListApprovals(ctx, action.ApprovalPending); err == nil {
		t.Fatal("closed store list must fail loud")
	}
	if _, err := closed.ApprovalParams(ctx, a.ApprovalID); err == nil {
		t.Fatal("closed store params must fail loud")
	}
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := closed.decideApproval(ctx, a.ApprovalID, "approved", a.RequestedAt.Add(time.Minute), envD, identD, ""); err == nil {
		t.Fatal("closed store decide must fail loud")
	}
	// Corrupt cells break the scan by name.
	corruptCell(t, store, "approvals", "requested_at", "approval_id", a.ApprovalID, "garbage")
	if _, _, err := store.GetApproval(ctx, a.ApprovalID); err == nil {
		t.Fatal("a corrupt requested_at must fail the read loud")
	}
	// Corrupt preview breaks its strict parse.
	store2, _ := sealedStore(t)
	env2 := testEnvelope("act_deep2")
	a2, p2 := testApprovalFor(env2)
	if err := store2.CreateApprovalRequest(ctx, env2, Decision{Outcome: "require_approval", Rule: "require_approval"}, a2, p2, `{"a":1}`); err != nil {
		t.Fatalf("birth 2: %v", err)
	}
	corruptCell(t, store2, "approvals", "canonical_preview", "approval_id", a2.ApprovalID, `{"bogus":1}`)
	if _, _, err := store2.GetApproval(ctx, a2.ApprovalID); err == nil {
		t.Fatal("a corrupt sealed preview must fail the read loud")
	}
	// ApprovalParams honest empties after a rejected close.
	env3 := testEnvelope("act_deep3")
	a3, p3 := testApprovalFor(env3)
	if err := store2.CreateApprovalRequest(ctx, env3, Decision{Outcome: "require_approval", Rule: "require_approval"}, a3, p3, `{"a":1}`); err != nil {
		t.Fatalf("birth 3: %v", err)
	}
	env3d, ident3 := operatorDecisionEnv("reject", a3.ApprovalID)
	if _, err := store2.decideApproval(ctx, a3.ApprovalID, "rejected", a3.RequestedAt.Add(time.Minute), env3d, ident3, ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := store2.ApprovalParams(ctx, a3.ApprovalID); err == nil {
		t.Fatal("params after a rejected close must be purged (not found)")
	}
	if _, err := store2.ApprovalParams(ctx, "apr_ghost"); err == nil {
		t.Fatal("ghost approval params must be not-found")
	}
}

func TestApproval_txFailureBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Blocked receipts inside the EXPIRY close: the whole touch rolls
	// back — approval stays PENDING, the parked action stays parked.
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_txe")
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_exp BEFORE INSERT ON receipts
		 BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatalf("blocker: %v", err)
	}
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.ExpiresAt.Add(time.Minute), env, ident, ""); err == nil {
		t.Fatal("an unreceiptable expiry close must fail whole")
	}
	still, _, _ := store.GetApproval(ctx, a.ApprovalID)
	if still.Status != action.ApprovalPending {
		t.Fatalf("expiry close must roll back whole: %v", still.Status)
	}
	// Blocked decision-act insert: the whole approve rolls back too.
	store2, _ := sealedStore(t)
	a2, _ := pendingRequest(t, store2, "act_txa")
	if _, err := store2.db.Exec(
		`CREATE TRIGGER block_act BEFORE INSERT ON actions
		 BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatalf("blocker 2: %v", err)
	}
	env2, ident2 := operatorDecisionEnv("approve", a2.ApprovalID)
	if _, err := store2.decideApproval(ctx, a2.ApprovalID, "approved",
		a2.RequestedAt.Add(time.Minute), env2, ident2, ""); err == nil {
		t.Fatal("an unrecordable decision act must fail whole")
	}
	still2, _, _ := store2.GetApproval(ctx, a2.ApprovalID)
	if still2.Status != action.ApprovalPending {
		t.Fatalf("approve must roll back whole: %v", still2.Status)
	}
	rec, _ := store2.Get(ctx, "act_txa")
	if rec.State != action.StatePendingApproval {
		t.Fatalf("the parked action must stay parked: %v", rec.State)
	}
	// The cancelled verb walks the reject path with its own status.
	store3, _ := sealedStore(t)
	a3, _ := pendingRequest(t, store3, "act_txc")
	env3, ident3 := operatorDecisionEnv("cancel", a3.ApprovalID)
	rule, err := store3.decideApproval(ctx, a3.ApprovalID, "cancelled",
		a3.RequestedAt.Add(time.Minute), env3, ident3, "changed my mind")
	if err != nil || rule != "" {
		t.Fatalf("cancel: %q %v", rule, err)
	}
	got, _, _ := store3.GetApproval(ctx, a3.ApprovalID)
	if got.Status != action.ApprovalCancelled {
		t.Fatalf("cancelled status: %v", got.Status)
	}
	rec3, _ := store3.Get(ctx, "act_txc")
	if rec3.State != action.StateRejected {
		t.Fatalf("a cancelled request closes the action REJECTED: %v", rec3.State)
	}
}

func TestApproval_moreBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A row moved out from under the decision (foreign hand) makes the
	// transition fail whole: "row not in expected state".
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_moved")
	if _, err := store.db.Exec(
		`UPDATE actions SET state = 'FAILED' WHERE action_id = 'act_moved'`); err != nil {
		t.Fatalf("move row: %v", err)
	}
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err == nil {
		t.Fatal("a foreign-handed row must fail the transition whole")
	}
	// No-expiry approvals round-trip their zero (timeCol NULL path).
	store2, _ := sealedStore(t)
	env2 := testEnvelope("act_noexp")
	a2, p2 := testApprovalFor(env2)
	a2.ExpiresAt = time.Time{}
	if err := store2.CreateApprovalRequest(ctx, env2,
		Decision{Outcome: "require_approval", Rule: "require_approval"}, a2, p2, `{"a":1}`); err != nil {
		t.Fatalf("no-expiry birth: %v", err)
	}
	got, _, err := store2.GetApproval(ctx, a2.ApprovalID)
	if err != nil || !got.ExpiresAt.IsZero() {
		t.Fatalf("zero expiry must round-trip as zero: %v %v", err, got.ExpiresAt)
	}
	// And it consumes far in the future (no expiry means none).
	env2d, ident2 := operatorDecisionEnv("approve", a2.ApprovalID)
	rule, err := store2.decideApproval(ctx, a2.ApprovalID, "approved",
		a2.RequestedAt.Add(1000*time.Hour), env2d, ident2, "")
	if err != nil || rule != "" {
		t.Fatalf("no-expiry consume: %q %v", rule, err)
	}
}

func TestApproval_rejectPathErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Reject over a foreign-moved row: the reject-side transition fails.
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_rmv")
	if _, err := store.db.Exec(
		`UPDATE actions SET state = 'FAILED' WHERE action_id = 'act_rmv'`); err != nil {
		t.Fatalf("move: %v", err)
	}
	env, ident := operatorDecisionEnv("reject", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err == nil {
		t.Fatal("reject over a moved row must fail whole")
	}
	// Reject whose terminal receipt cannot be reified (corrupt
	// requested_at): fails whole, approval stays PENDING.
	store2, _ := sealedStore(t)
	a2, _ := pendingRequest(t, store2, "act_rcorr")
	corruptCell(t, store2, "actions", "requested_at", "action_id", "act_rcorr", "garbage")
	env2, ident2 := operatorDecisionEnv("reject", a2.ApprovalID)
	if _, err := store2.decideApproval(ctx, a2.ApprovalID, "rejected",
		a2.RequestedAt.Add(time.Minute), env2, ident2, ""); err == nil {
		t.Fatal("an unreifiable terminal receipt must fail the reject whole")
	}
	still, _, _ := store2.GetApproval(ctx, a2.ApprovalID)
	if still.Status != action.ApprovalPending {
		t.Fatalf("failed reject must roll back: %v", still.Status)
	}
	// A blocked approvals UPDATE makes the expiry close fail loud.
	store3, _ := sealedStore(t)
	a3, _ := pendingRequest(t, store3, "act_bexp")
	if _, err := store3.db.Exec(
		`CREATE TRIGGER block_upd BEFORE UPDATE OF status ON approvals
		 BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatalf("blocker: %v", err)
	}
	env3, ident3 := operatorDecisionEnv("approve", a3.ApprovalID)
	if _, err := store3.decideApproval(ctx, a3.ApprovalID, "approved",
		a3.ExpiresAt.Add(time.Minute), env3, ident3, ""); err == nil {
		t.Fatal("a blocked expiry close must fail loud")
	}
}

func TestApproval_scanCorruptionBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, _ := sealedStore(t)
	a, _ := pendingRequest(t, store, "act_scan")
	// Corrupt expires_at breaks both the single read and the list scan.
	corruptCell(t, store, "approvals", "expires_at", "approval_id", a.ApprovalID, "garbage")
	if _, _, err := store.GetApproval(ctx, a.ApprovalID); err == nil {
		t.Fatal("a corrupt expires_at must fail the read loud")
	}
	if _, err := store.ListApprovals(ctx, action.ApprovalPending); err == nil {
		t.Fatal("a corrupt expires_at must fail the list loud")
	}
	// Corrupt decision_at breaks the scan too.
	store2, _ := sealedStore(t)
	a2, _ := pendingRequest(t, store2, "act_scan2")
	corruptCell(t, store2, "approvals", "decision_at", "approval_id", a2.ApprovalID, "garbage")
	if _, _, err := store2.GetApproval(ctx, a2.ApprovalID); err == nil {
		t.Fatal("a corrupt decision_at must fail the read loud")
	}
}

// The cross-check law strikes again (found by the lote-4 CLI tests):
// the E1 crash recovery closed EVERY non-terminal action on open —
// killing legitimately PARKED actions that wait for a human by design.
// The recovery must distinguish: PENDING_APPROVAL always survives (the
// expiry clock governs it, not the recovery); APPROVED with its params
// still held survives (awaiting deferred execution); APPROVED whose
// params were CLAIMED (crash between claim and close) is a true orphan
// and closes honestly.
func TestRecovery_parkedActionsSurviveReopen(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/korvun.db"
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	// A parked request and an approved-with-params one.
	a1, _ := pendingRequest(t, store, "act_park")
	a2, _ := pendingRequest(t, store, "act_appr")
	env, ident := operatorDecisionEnv("approve", a2.ApprovalID)
	if _, err := store.decideApproval(ctx, a2.ApprovalID, "approved",
		a2.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// And an approved one whose params were CLAIMED (the true orphan).
	a3, _ := pendingRequest(t, store, "act_claimed")
	env3, ident3 := operatorDecisionEnv("approve", a3.ApprovalID)
	if _, err := store.decideApproval(ctx, a3.ApprovalID, "approved",
		a3.RequestedAt.Add(time.Minute), env3, ident3, ""); err != nil {
		t.Fatalf("approve 3: %v", err)
	}
	if _, err := store.ClaimApprovalParams(ctx, a3.ApprovalID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = store.Close()
	// The next life opens with the full door: recovery runs.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if rec, _ := reopened.Get(ctx, "act_park"); rec.State != action.StatePendingApproval || rec.RecoveryMarker != "" {
		t.Fatalf("a PARKED action must survive the reopen: %v %q", rec.State, rec.RecoveryMarker)
	}
	if rec, _ := reopened.Get(ctx, "act_appr"); rec.State != action.StateApproved || rec.RecoveryMarker != "" {
		t.Fatalf("APPROVED-with-params awaits its deferred execution: %v %q", rec.State, rec.RecoveryMarker)
	}
	if rec, _ := reopened.Get(ctx, "act_claimed"); rec.State != action.StateFailed || rec.RecoveryMarker != "crash_recovered" {
		t.Fatalf("APPROVED-claimed is a true crash orphan and closes: %v %q", rec.State, rec.RecoveryMarker)
	}
	_ = a1
}

// REJECTED is a TERMINAL state (E5) and must survive the recovery pass
// like every other terminal — found when the recovery flattened a
// rejected action to FAILED on the next open.
func TestRecovery_rejectedIsTerminalAndSurvives(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/korvun.db"
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_rejsur")
	env, ident := operatorDecisionEnv("reject", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("reject: %v", err)
	}
	_ = store.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	rec, _ := reopened.Get(ctx, "act_rejsur")
	if rec.State != action.StateRejected || rec.RecoveryMarker != "" {
		t.Fatalf("REJECTED is terminal and survives: %v %q", rec.State, rec.RecoveryMarker)
	}
}

// rawV7Delta is the v6→v7 DDL restated literally (oracle discipline).
const rawV7Delta = `
CREATE TABLE approvals (
    approval_id           TEXT    NOT NULL PRIMARY KEY,
    schema_version        INTEGER NOT NULL,
    action_id             TEXT    NOT NULL UNIQUE,
    action_digest         TEXT    NOT NULL,
    preview_digest        TEXT    NOT NULL,
    canonical_preview     TEXT    NOT NULL,
    canonical_params      TEXT    NOT NULL,
    requested_from        TEXT    NOT NULL,
    reason                TEXT    NOT NULL,
    risk_summary          TEXT    NOT NULL,
    policy_version        INTEGER NOT NULL,
    policy_digest         TEXT    NOT NULL,
    requested_at          TEXT    NOT NULL,
    expires_at            TEXT,
    status                TEXT    NOT NULL,
    decision_principal_id TEXT    NOT NULL DEFAULT '',
    decision              TEXT    NOT NULL DEFAULT '',
    decision_at           TEXT,
    comment               TEXT    NOT NULL DEFAULT '',
    decision_receipt_id   TEXT    NOT NULL DEFAULT ''
) WITHOUT ROWID;
CREATE INDEX approvals_by_status ON approvals(status);
UPDATE action_schema SET version = 7;`

func TestMigrationV8_receiptsGainTheApprovalSeal(t *testing.T) {
	t.Parallel()
	// Fresh file lands on current with the new receipt columns.
	path := filepath.Join(t.TempDir(), "korvun.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if v, _ := store.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("fresh store lands on current, got %d", v)
	}
	_ = store.Close()
	if n := inspect(t, path,
		`SELECT COUNT(*) FROM pragma_table_info('receipts') WHERE name IN ('schema_version','approval_digest')`); n != 2 {
		t.Fatalf("the v8 receipt columns must exist, got %d", n)
	}
	// AS-8 crash mold against a HAND-BUILT v7.
	crashPath := buildV6File(t)
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(crashPath)))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(rawV7Delta); err != nil {
		t.Fatalf("build v7 delta: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER crash_mid_v8 BEFORE UPDATE ON action_schema
		 BEGIN SELECT RAISE(ABORT, 'crash mid v8'); END;`); err != nil {
		t.Fatalf("install crash trigger: %v", err)
	}
	_ = db.Close()
	if _, err := Open(crashPath); err == nil {
		t.Fatal("an aborted v8 migration must be boot-fatal")
	}
	if v := inspect(t, crashPath, `SELECT version FROM action_schema`); v != 7 {
		t.Fatalf("aborted migration must leave version 7, got %d", v)
	}
	db2, _ := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(crashPath)))
	if _, err := db2.Exec(`DROP TRIGGER crash_mid_v8`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	_ = db2.Close()
	recovered, err := Open(crashPath)
	if err != nil {
		t.Fatalf("next boot completes: %v", err)
	}
	defer func() { _ = recovered.Close() }()
	if v, _ := recovered.SchemaVersion(context.Background()); v != schemaVersionCurrent {
		t.Fatalf("completed migration lands on current, got %v", v)
	}
}

func TestReceiptsAreBornV2_withTheApprovalSealWhenApproved(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	// An unapproved terminal: v2 with the honest empty approval digest.
	if err := store.RecordAttempt(ctx, testEnvelope("act_v2a"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	receipts, _ := store.ReceiptsByAction(ctx, "act_v2a")
	if len(receipts) != 1 || receipts[0].SchemaVersion != 2 || receipts[0].ApprovalDigest != "" {
		t.Fatalf("new receipts are v2 with honest empties: %+v", receipts)
	}
	if err := action.VerifyReceiptSignature(pub, receipts[0]); err != nil {
		t.Fatalf("v2 verifies: %v", err)
	}
	// An APPROVED-then-executed action: the receipt seals the approval.
	a, _ := pendingRequest(t, store, "act_v2b")
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := store.FinishWithResult(ctx, "act_v2b", action.StateSucceeded,
		a.RequestedAt.Add(2*time.Minute), "sha256:result"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	outcome, _ := store.ReceiptsByAction(ctx, "act_v2b")
	if len(outcome) != 1 {
		t.Fatalf("one outcome receipt: %d", len(outcome))
	}
	consumed, _, _ := store.GetApproval(ctx, a.ApprovalID)
	if outcome[0].ApprovalDigest == "" || outcome[0].ApprovalDigest != consumed.Digest() {
		t.Fatalf("the receipt must seal the CONSUMED approval's digest: %q vs %q",
			outcome[0].ApprovalDigest, consumed.Digest())
	}
	if err := action.VerifyReceiptSignature(pub, outcome[0]); err != nil {
		t.Fatalf("the sealed approval verifies: %v", err)
	}
}

func TestMixedEraChain_verifiesWhole(t *testing.T) {
	t.Parallel()
	store, pub := sealedStore(t)
	ctx := context.Background()
	// Insert a HAND-BUILT v1 receipt at the chain's genesis (the frozen
	// era: canonicalized, hashed and signed with the v1 form), then let
	// the live code append v2 receipts after it.
	_, priv := pubPrivOf(t, pub)
	v1 := action.Receipt{
		ReceiptID: "rcpt_v1era0000000000000000000000001", ActionID: "act_old",
		IntentDigest: "", PrincipalID: "", DecisionDigest: action.HashCanonical(`{}`),
		ActionDigest: "sha256:old", EffectClass: action.EffectPure, Attempt: 1,
		Outcome:    string(action.StateDenied),
		StartedAt:  time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC),
		Partition:  "main", ChainSeq: 0, PreviousReceiptHash: action.GenesisPreviousHash,
	}
	v1.ReceiptHash = action.ComputeReceiptHash(v1)
	v1 = action.SignReceipt(priv, v1)
	insertReceiptRow(t, store, v1)
	// The live era appends after it.
	if err := store.RecordAttempt(ctx, testEnvelope("act_new"),
		Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	chain, err := store.ListReceipts(ctx, "main")
	if err != nil || len(chain) != 2 {
		t.Fatalf("mixed chain: %v %d", err, len(chain))
	}
	if chain[0].SchemaVersion >= 2 || chain[1].SchemaVersion != 2 {
		t.Fatalf("eras: %d then %d", chain[0].SchemaVersion, chain[1].SchemaVersion)
	}
	// BOTH eras verify — each with the canonical form of its era.
	for i, r := range chain {
		if r.ReceiptHash != action.ComputeReceiptHash(r) {
			t.Fatalf("receipt %d hash must recompute under its own era", i)
		}
		if err := action.VerifyReceiptSignature(pub, r); err != nil {
			t.Fatalf("receipt %d signature: %v", i, err)
		}
	}
	// And the chain links across the era boundary.
	if chain[1].PreviousReceiptHash != chain[0].ReceiptHash {
		t.Fatal("the chain must link across mixed eras")
	}
}

// pubPrivOf regenerates nothing: the sealedStore helper owns the pair,
// so tests that need the private half re-derive the SAME sealer pair
// via a shared registry. Simplest honest shape: sealedStore is changed
// to also expose the private key through this map.
func pubPrivOf(t *testing.T, pub ed25519.PublicKey) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	sealerKeysMu.Lock()
	defer sealerKeysMu.Unlock()
	priv, ok := sealerKeys[string(pub)]
	if !ok {
		t.Fatal("no private half registered for this sealer public key")
	}
	return pub, priv
}

// insertReceiptRow writes one pre-sealed receipt row directly (the
// hand-built historical era for mixed-chain tests).
func insertReceiptRow(t *testing.T, store *Store, r action.Receipt) {
	t.Helper()
	if _, err := store.db.Exec(
		`INSERT INTO receipts (receipt_id, action_id, intent_digest, principal_id,
		    authority_digest, decision_digest, action_digest, effect_class, attempt,
		    outcome, result_digest, started_at, finished_at, partition, chain_seq,
		    previous_receipt_hash, receipt_hash, signing_key_id, signature,
		    schema_version, approval_digest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ReceiptID, r.ActionID, r.IntentDigest, r.PrincipalID,
		r.AuthorityDigest, r.DecisionDigest, r.ActionDigest, string(r.EffectClass), r.Attempt,
		r.Outcome, r.ResultDigest,
		r.StartedAt.UTC().Format(time.RFC3339Nano), r.FinishedAt.UTC().Format(time.RFC3339Nano),
		r.Partition, r.ChainSeq, r.PreviousReceiptHash, r.ReceiptHash,
		r.SigningKeyID, r.Signature, r.SchemaVersion, r.ApprovalDigest,
	); err != nil {
		t.Fatalf("insert historical receipt: %v", err)
	}
}

func TestApprovalByAction_branches(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_byact")
	got, _, err := store.GetApprovalByAction(ctx, "act_byact")
	if err != nil || got.ApprovalID != a.ApprovalID {
		t.Fatalf("by-action lookup: %v %+v", err, got)
	}
	if _, _, err := store.GetApprovalByAction(ctx, "act_ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ghost action: %v", err)
	}
	closed, _ := openTemp(t)
	_ = closed.Close()
	if _, _, err := closed.GetApprovalByAction(ctx, "act_byact"); err == nil {
		t.Fatal("closed store must fail loud")
	}
	// approvalDigestTx honest empty: an unapproved action's finish.
	if err := store.RecordAttempt(ctx, testEnvelope("act_plain"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.Finish(ctx, "act_plain", action.StateSucceeded, time.Now().UTC()); err != nil {
		t.Fatalf("finish: %v", err)
	}
	receipts, _ := store.ReceiptsByAction(ctx, "act_plain")
	if len(receipts) != 1 || receipts[0].ApprovalDigest != "" {
		t.Fatalf("unapproved outcomes carry the honest empty: %+v", receipts)
	}
}

func TestApprovalDigestTx_corruptDecisionFallsToHonestEmpty(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_corrd")
	env, ident := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	corruptCell(t, store, "approvals", "decision_at", "approval_id", a.ApprovalID, "garbage")
	// The finish still lands; the unresolvable approval digest falls to
	// the honest empty rather than blocking the terminal close.
	if err := store.FinishWithResult(ctx, "act_corrd", action.StateSucceeded,
		a.RequestedAt.Add(2*time.Minute), "sha256:r"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	receipts, _ := store.ReceiptsByAction(ctx, "act_corrd")
	if len(receipts) != 1 {
		t.Fatalf("one receipt: %d", len(receipts))
	}
}

func TestClaimApprovalParams_branches(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a, _ := pendingRequest(t, store, "act_claim2")
	params, err := store.ClaimApprovalParams(ctx, a.ApprovalID)
	if err != nil || len(params) == 0 {
		t.Fatalf("first claim wins: %v %d", err, len(params))
	}
	if _, err := store.ClaimApprovalParams(ctx, a.ApprovalID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second claim loses by name: %v", err)
	}
	if _, err := store.ClaimApprovalParams(ctx, "apr_ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ghost claim: %v", err)
	}
	closed, _ := openTemp(t)
	_ = closed.Close()
	if _, err := closed.ClaimApprovalParams(ctx, a.ApprovalID); err == nil {
		t.Fatal("closed store must fail loud")
	}
}
