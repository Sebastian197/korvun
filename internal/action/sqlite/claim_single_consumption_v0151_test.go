// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// A purge that does not hold inside the claim's own transaction is refused —
// v0.15.1 block A, A1 of the seventeenth external pass. Scope: the in-
// transaction restore (a trigger). Another connection restoring the column
// AFTER the claim commits is not covered and is filed for v0.15.2.
//
// The claim purges canonical_params with a conditioned UPDATE and trusts its
// RowsAffected. A trigger that restores the column after the purge makes that
// UPDATE report one row while the parameters are still there when the
// transaction commits: the next claim finds them and consumes them again. The
// cure re-reads the column after the purge, inside the same transaction, and
// demands the empty string before the commit.
//
// Evidence level, honest: multiple real database connections — the triggers
// and the probe table are installed from a second *sql.DB on the same file.
// In-process; not a compiled binary.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestClaim_aPurgeThatDoesNotHoldConsumesNothing is A1's mould at the store:
// two consecutive claims under a restoring trigger. Each must be refused by
// name, hand back no parameters, commit nothing, and leave the row as the
// decide left it.
//
// Two restorations, because two wrong cures pass one of them:
//   - the SAME bytes (Codex's trigger, OLD.canonical_params);
//   - DIFFERENT bytes with the SAME digest ('{"a": 1}' for '{"a":1}':
//     action.Digest canonicalizes, so both re-derive the approved digest). A
//     cure that compares the re-read against the value it read before the
//     purge, instead of demanding the empty string, sees «changed» and lets it through.
//
// The oracle by impossibility for «refuses AND rolls back»: the restoring
// trigger also INSERTs into purge_probe. That row exists only if a claim's
// transaction committed with the purge in it. A cure that commits first and
// re-reads afterwards (outside the transaction) refuses by name and still
// leaves probe rows and a consumed-then-restored history behind.
//
// Planned probing mutations (after green):
//   - move the re-read after the Commit (outside the transaction) ⇒ probe rows
//     appear;
//   - compare the re-read with the value read before the purge instead of
//     demanding the empty string ⇒ the different-bytes row hands out parameters.
func TestClaim_aPurgeThatDoesNotHoldConsumesNothing(t *testing.T) {
	t.Parallel()
	const triggerHead = `CREATE TRIGGER restore_params AFTER UPDATE OF canonical_params ON approvals
		WHEN NEW.canonical_params = '' AND OLD.canonical_params != ''
		BEGIN
		  INSERT INTO purge_probe (approval_id) VALUES (NEW.approval_id);
		  UPDATE approvals SET canonical_params = `
	const triggerTail = `
		   WHERE approval_id = NEW.approval_id;
		END`
	cases := []struct {
		name    string
		trigger string
	}{
		{"the trigger restores the same bytes",
			triggerHead + `OLD.canonical_params` + triggerTail},
		{"the trigger restores different bytes with the same digest",
			triggerHead + `CASE WHEN OLD.canonical_params = '{"a":1}' THEN '{"a": 1}' ELSE '{"a":1}' END` + triggerTail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			ctx := context.Background()
			a := boundPark(t, store, "act_single_consumption")
			envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
			if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
				a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
				t.Fatalf("approve: %v", err)
			}

			hand := claimAttackConn(t, store)
			if _, err := hand.Exec(`CREATE TABLE purge_probe (approval_id TEXT NOT NULL)`); err != nil {
				t.Fatalf("create the probe table: %v", err)
			}
			if _, err := hand.Exec(tc.trigger); err != nil {
				t.Fatalf("install the restoring trigger: %v", err)
			}

			handed := 0
			for i, label := range []string{"first claim", "second claim"} {
				params, _, err := store.ClaimApprovalParamsUnderDigest(ctx, a.ApprovalID, nil, a.ActionDigest, nil)
				if params != nil {
					handed++
				}
				if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
					t.Fatalf("%s (#%d): err = %v, params = %q, want ErrApprovalEvidenceCorrupt (claims that handed parameters so far: %d)",
						label, i+1, err, params, handed)
				}
			}
			if handed != 0 {
				t.Fatalf("%d claim(s) handed back parameters whose purge never held", handed)
			}
			var probes int
			if err := hand.QueryRow(`SELECT COUNT(*) FROM purge_probe`).Scan(&probes); err != nil {
				t.Fatalf("read the probe: %v", err)
			}
			if probes != 0 {
				t.Fatalf("purge_probe holds %d row(s): a refused claim's transaction COMMITTED its purge", probes)
			}

			var raw, status, state string
			if err := hand.QueryRow(`SELECT canonical_params, status FROM approvals WHERE approval_id = ?`,
				a.ApprovalID).Scan(&raw, &status); err != nil {
				t.Fatalf("re-read the approval: %v", err)
			}
			if err := hand.QueryRow(`SELECT state FROM actions WHERE action_id = ?`, a.ActionID).Scan(&state); err != nil {
				t.Fatalf("re-read the action: %v", err)
			}
			if raw != `{"a":1}` || status != "APPROVED" || state != "APPROVED" {
				t.Fatalf("after two refused claims: params=%q status=%q state=%q, want {\"a\":1}/APPROVED/APPROVED",
					raw, status, state)
			}
		})
	}
}

// TestClaim_aMovedRowIsNamedBeforeAMovedLaw is the cross-block sister of A1
// that block B's adversary reported (executed there as a race, 63 of 400):
// the claim judges the LAW before it compares the caller's snapshot, so a
// request whose row had already moved under the caller is published
// `invalidated`, whose literal says rejecting it still works. The moved row is
// the more specific fact and must be named first.
//
// Deterministic form, store level: the caller holds the APPROVED row
// (ExecuteApprovedAction's precheck admits nothing else), a second real
// connection moves it to REJECTED, and the law passed to the claim differs
// from the parked one. Both facts are true; the name must be
// ErrApprovalMovedUnderTheClaim (existing, approvals_v15.go).
//
// Reachability, stated to its width: with the snapshot APPROVED, no
// production writer moves a column the snapshot comparison reads — every
// UPDATE of approvals is guarded by status = PENDING or touches only
// canonical_params. This row is reachable by OUT-OF-BAND tampering only (a
// second process writing the file); the mould forces that hand. It is not a
// production race.
//
// Evidence level, honest: multiple real database connections, in-process.
// The end-to-end race and the literal the window prints are block B's
// (internal/app/approvals_adapter.go).
//
// Planned probing mutation (after green): put the law check back ahead of the
// snapshot comparison ⇒ this reddens with ErrApprovalInvalidated.
func TestClaim_aMovedRowIsNamedBeforeAMovedLaw(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	a := boundPark(t, store, "act_law_after_seen")
	envD, identD := operatorDecisionEnv("approve", a.ApprovalID)
	if _, err := store.decideApproval(ctx, a.ApprovalID, "approved",
		a.RequestedAt.Add(time.Minute), envD, identD, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	seen, _, err := store.GetApproval(ctx, a.ApprovalID)
	if err != nil {
		t.Fatalf("the caller's read: %v", err)
	}
	if seen.Status != "APPROVED" {
		t.Fatalf("the caller's snapshot is %s, want APPROVED", seen.Status)
	}
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`UPDATE approvals SET status = 'REJECTED' WHERE approval_id = ?`, a.ApprovalID); err != nil {
		t.Fatalf("move the row behind the caller: %v", err)
	}
	moved := &PolicyPin{Version: seen.PolicyVersion + 1, Digest: "sha256:another-law"}

	params, _, err := store.ClaimApprovalParamsUnderDigest(ctx, a.ApprovalID, moved, a.ActionDigest, &seen)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	if !errors.Is(err, ErrApprovalMovedUnderTheClaim) {
		t.Fatalf("err = %v, want ErrApprovalMovedUnderTheClaim — the row moved before the law was judged", err)
	}
}

// rebuildApprovalsWithoutConstraints recreates the approvals table from a
// second connection with CREATE TABLE … AS SELECT, which drops its NOT NULL
// and PRIMARY KEY constraints — the hand that makes a NULL cell and a
// duplicated row placeable at all (TestClaim_aNullLeftByThePurgeIsCorruptEvidence,
// TestClaim_aPurgeOfMoreThanOneRowIsCorruptEvidence and the NULL-cell moulds
// of TestClaim_everyNullCellInsideTheClaimIsCorruptEvidence).
func rebuildApprovalsWithoutConstraints(t *testing.T, store *Store) *sql.DB {
	t.Helper()
	hand := claimAttackConn(t, store)
	for _, q := range []string{
		`ALTER TABLE approvals RENAME TO approvals_old`,
		`CREATE TABLE approvals AS SELECT * FROM approvals_old`,
	} {
		if _, err := hand.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return hand
}

// TestClaim_aNullLeftByThePurgeIsCorruptEvidence is the diff pass #2's P2-1:
// a trigger writes NULL into the purged cell. The re-read scanned into a
// string, so the conversion failed and the claim published a store that did
// not answer (unreadable). A NULL in the cell is a value that is there and
// wrong: corrupt evidence, nothing handed out.
//
// Planned probing mutation: scan the re-read back into a string ⇒ this
// reddens with the unreadable class.
func TestClaim_aNullLeftByThePurgeIsCorruptEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := approvedForClaim(t, store, "act_a1_null")
	hand := rebuildApprovalsWithoutConstraints(t, store)
	if _, err := hand.Exec(`CREATE TRIGGER null_params AFTER UPDATE OF canonical_params ON approvals
		WHEN NEW.canonical_params = ''
		BEGIN UPDATE approvals SET canonical_params = NULL WHERE approval_id = NEW.approval_id; END`); err != nil {
		t.Fatalf("install the NULL trigger: %v", err)
	}
	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	assertOneClass(t, err, true)
}

// TestClaim_aPurgeOfMoreThanOneRowIsCorruptEvidence is the diff pass #2's
// P2-2: the table rebuilt without its key and the approval row duplicated, so
// the purge affects two rows. A store that no longer holds its own schema is
// corrupt evidence and the claim hands out nothing.
//
// Planned probing mutation: drop the n != 1 check ⇒ the claim commits and
// hands out parameters.
func TestClaim_aPurgeOfMoreThanOneRowIsCorruptEvidence(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := approvedForClaim(t, store, "act_a1_dup")
	hand := rebuildApprovalsWithoutConstraints(t, store)
	if _, err := hand.Exec(`INSERT INTO approvals SELECT * FROM approvals_old`); err != nil {
		t.Fatalf("duplicate the approval row: %v", err)
	}
	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if params != nil {
		t.Fatalf("a refused claim handed back parameters: %q", params)
	}
	assertOneClass(t, err, true)
	if !strings.Contains(err.Error(), "purged 2 rows") {
		t.Fatalf("err = %v: the refusal is not the two-row purge — the row does not attack what it names", err)
	}
}

// TestClaim_everyNullCellInsideTheClaimIsCorruptEvidence (diff passes #3 and
// #4): a cell the claim reads inside its own transaction holds a value that is
// there — a NULL, or a status/state outside its domain, is corrupt evidence,
// never a store that failed to answer and never a legitimate loss of
// authority. principal_id is the one nullable cell, by design. Rows, by read:
// the story's first read (effect_class, op_namespace, op_name), the cells
// before the purge (parameters, preview), the approval status after it
// (claimAuthorityTx: NULL and out of domain) and the action state after it
// (ternaOf through claimAuthorityTx: NULL and out of domain).
//
// Planned probing mutations: put back the base's typed Scan on the story and
// the status (NULL-story-scan, NULL-status-scan), drop the domain checks
// (DOMAIN-status, DOMAIN-state) ⇒ the matching row reddens. Dropping only the
// Valid checks is NOT always equivalent: for the story it is caught by
// TestClaim_aStoryCellWithACoherentPreviewIsStillCorrupt (a preview rewritten
// to agree with the NULL cell); for the status and the state it is equivalent
// in CLASS — the domain check names "" corrupt — but not in TEXT (the operator
// reads «status ""» instead of «NULL»). The preview row's Valid check is
// equivalent: the empty string fails ParseCanonicalPreview anyway.
func TestClaim_everyNullCellInsideTheClaimIsCorruptEvidence(t *testing.T) {
	t.Parallel()
	rebuildActions := []string{
		`ALTER TABLE actions RENAME TO actions_old`,
		`CREATE TABLE actions AS SELECT * FROM actions_old`,
	}
	cases := []struct {
		name  string
		setup []string
	}{
		{"effect_class is NULL (the story's first read)", append(append([]string{}, rebuildActions...), `UPDATE actions SET effect_class = NULL`)},
		{"op_namespace is NULL (the story's first read)", append(append([]string{}, rebuildActions...), `UPDATE actions SET op_namespace = NULL`)},
		{"op_name is NULL (the story's first read)", append(append([]string{}, rebuildActions...), `UPDATE actions SET op_name = NULL`)},
		{"the preview cell is NULL before the purge", []string{
			`ALTER TABLE approvals RENAME TO approvals_old`,
			`CREATE TABLE approvals AS SELECT * FROM approvals_old`,
			`UPDATE approvals SET canonical_preview = NULL`,
		}},
		{"the approval status leaves its domain after the purge", []string{
			`CREATE TRIGGER bogus_status AFTER UPDATE OF canonical_params ON approvals
			 WHEN NEW.canonical_params = ''
			 BEGIN UPDATE approvals SET status = 'BOGUS' WHERE approval_id = NEW.approval_id; END`,
		}},
		{"the action state leaves its domain after the purge", []string{
			`CREATE TRIGGER bogus_state AFTER UPDATE OF canonical_params ON approvals
			 WHEN NEW.canonical_params = ''
			 BEGIN UPDATE actions SET state = 'BOGUS' WHERE action_id = NEW.action_id; END`,
		}},
		{"the parameters cell is NULL before the purge", []string{
			`ALTER TABLE approvals RENAME TO approvals_old`,
			`CREATE TABLE approvals AS SELECT * FROM approvals_old`,
			`UPDATE approvals SET canonical_params = NULL`,
		}},
		{"the approval status turns NULL after the purge", []string{
			`ALTER TABLE approvals RENAME TO approvals_old`,
			`CREATE TABLE approvals AS SELECT * FROM approvals_old`,
			`CREATE TRIGGER null_status AFTER UPDATE OF canonical_params ON approvals
			 WHEN NEW.canonical_params = ''
			 BEGIN UPDATE approvals SET status = NULL WHERE approval_id = NEW.approval_id; END`,
		}},
		{"the action state turns NULL after the purge", []string{
			`ALTER TABLE actions RENAME TO actions_old`,
			`CREATE TABLE actions AS SELECT * FROM actions_old`,
			`CREATE TRIGGER null_state AFTER UPDATE OF canonical_params ON approvals
			 WHEN NEW.canonical_params = ''
			 BEGIN UPDATE actions SET state = NULL WHERE action_id = NEW.action_id; END`,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			approvalID, digest := approvedForClaim(t, store, "act_null_cell")
			hand := claimAttackConn(t, store)
			for _, q := range tc.setup {
				if _, err := hand.Exec(q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
			if params != nil {
				t.Fatalf("a refused claim handed back parameters: %q", params)
			}
			if errors.Is(err, ErrApprovalNoLongerApproved) {
				t.Fatalf("err = %v: a NULL was published as a legitimate loss of authority", err)
			}
			assertOneClass(t, err, true)
		})
	}
}

// rewritePreviewCoherently makes the stored preview AGREE with a tampered
// action row: it rewrites canonical_preview through mutate and stores the
// re-derived preview_digest — the same hand that tampered with the row. The
// preview comparison then passes, and only the cell's own check can refuse.
func rewritePreviewCoherently(t *testing.T, hand *sql.DB, approvalID string, mutate func(*action.ActionPreview)) {
	t.Helper()
	var raw string
	if err := hand.QueryRow(`SELECT canonical_preview FROM approvals WHERE approval_id = ?`, approvalID).Scan(&raw); err != nil {
		t.Fatalf("read the preview: %v", err)
	}
	p, err := action.ParseCanonicalPreview([]byte(raw))
	if err != nil {
		t.Fatalf("parse the preview: %v", err)
	}
	mutate(&p)
	if _, err := hand.Exec(`UPDATE approvals SET canonical_preview = ?, preview_digest = ? WHERE approval_id = ?`,
		string(action.CanonicalPreview(p)), p.Digest(), approvalID); err != nil {
		t.Fatalf("rewrite the preview: %v", err)
	}
}

// TestClaim_aStoryCellWithACoherentPreviewIsStillCorrupt is the diff pass #5's
// P2 and P3: the story's NULL checks are NOT equivalent mutants. A NULL (or an
// empty or out-of-domain effect class) with a preview rewritten to agree —
// effect class "", operation "/<name>" — passes the preview comparison, and
// without the cell's own check the claim COMMITS and hands out the parameters;
// at the Detail door (a PENDING request, the only kind whose story Detail
// judges) under a moved law it answers invalidated instead of corrupt. Every
// row, at both doors, must be corrupt evidence.
//
// Planned probing mutations: drop the story's NULL check (NULL-story) ⇒ only
// the op_namespace and op_name rows at the Detail door redden (at the claim
// those NULLs are caught later by ternaOf, and a NULL effect_class becomes ""
// and is caught by the domain check); drop the effect-class domain check
// (EFFECT-domain) ⇒ the empty and out-of-domain rows redden. The domain is
// the sealed enum plus the E1 placeholder «unclassified» (director's
// decision, v0.15.1 diff pass #5);
// TestClaim_anUnclassifiedStoryStillClaims pins that the placeholder passes.
func TestClaim_aStoryCellWithACoherentPreviewIsStillCorrupt(t *testing.T) {
	t.Parallel()
	rebuildActions := []string{
		`ALTER TABLE actions RENAME TO actions_old`,
		`CREATE TABLE actions AS SELECT * FROM actions_old`,
	}
	cases := []struct {
		name    string
		cell    string
		preview func(*action.ActionPreview)
	}{
		{"effect_class NULL, preview effect \"\"", `UPDATE actions SET effect_class = NULL`,
			func(p *action.ActionPreview) { p.EffectClass = "" }},
		{"op_namespace NULL, preview operation \"/<name>\"", `UPDATE actions SET op_namespace = NULL`,
			func(p *action.ActionPreview) { p.Operation = "/" + p.Operation[strings.Index(p.Operation, "/")+1:] }},
		{"op_name NULL, preview operation \"<ns>/\"", `UPDATE actions SET op_name = NULL`,
			func(p *action.ActionPreview) { p.Operation = p.Operation[:strings.Index(p.Operation, "/")+1] }},
		{"effect_class empty, preview effect \"\"", `UPDATE actions SET effect_class = ''`,
			func(p *action.ActionPreview) { p.EffectClass = "" }},
		{"effect_class out of domain, preview agrees", `UPDATE actions SET effect_class = 'bogus'`,
			func(p *action.ActionPreview) { p.EffectClass = "bogus" }},
	}
	for _, tc := range cases {
		for _, door := range []string{"claim", "detail under a moved law"} {
			t.Run(tc.name+" · "+door, func(t *testing.T) {
				t.Parallel()
				store, _ := sealedStore(t)
				// The claim needs an APPROVED request; the Detail door judges the
				// story only of a PENDING one (a decided row is served unjudged,
				// FR-API-18), so that door attacks a parked request.
				var approvalID, digest string
				if door == "claim" {
					approvalID, digest = approvedForClaim(t, store, "act_story_coherent")
				} else {
					a := boundPark(t, store, "act_story_coherent")
					approvalID = a.ApprovalID
				}
				hand := claimAttackConn(t, store)
				for _, q := range append(append([]string{}, rebuildActions...), tc.cell) {
					if _, err := hand.Exec(q); err != nil {
						t.Fatalf("%s: %v", q, err)
					}
				}
				rewritePreviewCoherently(t, hand, approvalID, tc.preview)
				var err error
				if door == "claim" {
					var params []byte
					params, _, err = store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
					if params != nil {
						t.Fatalf("the claim handed out parameters over a tampered story: %q", params)
					}
				} else {
					_, err = store.ApprovalDetailUnderLaw(context.Background(), approvalID, PolicyPin{Version: 99, Digest: "sha256:moved"})
				}
				if errors.Is(err, ErrApprovalInvalidated) {
					t.Fatalf("err = %v: a tampered story was published invalidated", err)
				}
				assertOneClass(t, err, true)
			})
		}
	}
}

// TestDetail_anOutOfDomainActionStateIsCorrupt pins the Detail door's side of
// ternaOf's domain check (diff pass #5, P3): at base a parked request whose
// action row carries state 'BOGUS' was served with ActionState "BOGUS"; it is
// corrupt evidence.
//
// Planned probing mutation: drop the action-state domain check
// (DOMAIN-state) ⇒ this reddens with a served row.
func TestDetail_anOutOfDomainActionStateIsCorrupt(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	a := boundPark(t, store, "act_detail_bogus")
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`UPDATE actions SET state = 'BOGUS' WHERE action_id = ?`, a.ActionID); err != nil {
		t.Fatalf("tamper the state: %v", err)
	}
	row, err := store.ApprovalDetail(context.Background(), a.ApprovalID)
	if err == nil {
		t.Fatalf("Detail served a row with ActionState %q", row.ActionState)
	}
	assertOneClass(t, err, true)
}

// TestClaim_anUnclassifiedStoryStillClaims pins the other side of the domain
// check: an action carrying the E1 placeholder «unclassified», with its
// preview agreeing, is legitimate and still claims — the domain is the sealed
// enum PLUS the placeholder, not the enum alone.
//
// Planned probing mutation: restrict the domain to action.EffectClass.Known
// ⇒ this reddens with ErrApprovalEvidenceCorrupt.
func TestClaim_anUnclassifiedStoryStillClaims(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	approvalID, digest := approvedForClaim(t, store, "act_unclassified")
	hand := claimAttackConn(t, store)
	if _, err := hand.Exec(`UPDATE actions SET effect_class = 'unclassified'`); err != nil {
		t.Fatalf("set the placeholder: %v", err)
	}
	rewritePreviewCoherently(t, hand, approvalID, func(p *action.ActionPreview) { p.EffectClass = "unclassified" })
	params, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), approvalID, nil, digest, nil)
	if err != nil {
		t.Fatalf("an unclassified request was refused: %v", err)
	}
	if string(params) != `{"a":1}` {
		t.Fatalf("params = %q, want {\"a\":1}", params)
	}
}
