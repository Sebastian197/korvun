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
			rule, err := store.DecideApproval(ctx, a.ApprovalID, "approved",
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
	rule, err := store.DecideApproval(ctx, a.ApprovalID, "rejected",
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
	if _, err := store2.DecideApproval(ctx, a2.ApprovalID, "rejected",
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
	rule, err := store.DecideApproval(ctx, a.ApprovalID, "approved", late, env, ident, "")
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
	if _, err := store.DecideApproval(context.Background(), a.ApprovalID, "burn",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err == nil {
		t.Fatal("an unknown decision verb must fail closed")
	}
	if _, err := store.DecideApproval(context.Background(), "apr_ghost", "approved",
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
	if _, err := store.DecideApproval(ctx, a2.ApprovalID, "rejected",
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
	if _, err := closed.DecideApproval(ctx, a.ApprovalID, "approved", a.RequestedAt.Add(time.Minute), envD, identD, ""); err == nil {
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
	if _, err := store2.DecideApproval(ctx, a3.ApprovalID, "rejected", a3.RequestedAt.Add(time.Minute), env3d, ident3, ""); err != nil {
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
	if _, err := store.DecideApproval(ctx, a.ApprovalID, "approved",
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
	if _, err := store2.DecideApproval(ctx, a2.ApprovalID, "approved",
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
	rule, err := store3.DecideApproval(ctx, a3.ApprovalID, "cancelled",
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
	if _, err := store.DecideApproval(ctx, a.ApprovalID, "approved",
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
	rule, err := store2.DecideApproval(ctx, a2.ApprovalID, "approved",
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
	if _, err := store.DecideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Minute), env, ident, ""); err == nil {
		t.Fatal("reject over a moved row must fail whole")
	}
	// Reject whose terminal receipt cannot be reified (corrupt
	// requested_at): fails whole, approval stays PENDING.
	store2, _ := sealedStore(t)
	a2, _ := pendingRequest(t, store2, "act_rcorr")
	corruptCell(t, store2, "actions", "requested_at", "action_id", "act_rcorr", "garbage")
	env2, ident2 := operatorDecisionEnv("reject", a2.ApprovalID)
	if _, err := store2.DecideApproval(ctx, a2.ApprovalID, "rejected",
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
	if _, err := store3.DecideApproval(ctx, a3.ApprovalID, "approved",
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
