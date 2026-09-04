// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R11 (the trimmed round): the verifier's absence note tells the
// epistemological truth VERBATIM — "no tombstone with the sealed
// digest exists; legacy history, deletion, or a coherent rewrite are
// indistinguishable" — and a NULL decision instant is narrated
// decision_at_absent, never the lying year 0001. The
// TombstoneIntegrityByAction arm died with its false positive and its
// false negative (direction decision, SECURITY.md documents the v2-era
// limits until sealed provenance). Reproduction-first contract.

package cli

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// R6 — the narration: an absent decision instant is said out loud.
func TestReconstructionNote_absentDecisionAtIsNamedNeverYearZero(t *testing.T) {
	t.Parallel()
	tomb := action.Approval{
		ApprovalID: "apr_note", ActionID: "act_note",
		ActionDigest: "sha256:a", PreviewDigest: "sha256:p",
		PolicyVersion: 3, PolicyDigest: "sha256:l",
		DecisionPrincipalID: "principal_operator", Decision: "rejected",
	}
	note := reconstructionNote(tomb, false)
	if !strings.Contains(note, "decision_at_absent") ||
		!strings.Contains(note, "compatible with legacy history") {
		t.Fatalf("AUDIT R11-R6: an absent instant must be narrated by name with its ambiguity: %q", note)
	}
	if strings.Contains(note, "0001-") {
		t.Fatalf("the year-0001 lie must never be printed: %q", note)
	}
	// And a present instant keeps the plain date.
	tomb.DecisionAt = time.Date(2026, 9, 3, 1, 2, 3, 0, time.UTC)
	note = reconstructionNote(tomb, true)
	if !strings.Contains(note, "2026-09-03T01:02:03Z") || strings.Contains(note, "decision_at_absent") {
		t.Fatalf("a present instant narrates the date itself: %q", note)
	}
}

// R7 — deletion and coherent rewrite end in the SAME verbatim
// ambiguous note: the verifier cannot tell them apart and says so.
func TestReceiptVerify_absenceNoteTellsTheEpistemologicalTruth(t *testing.T) {
	t.Parallel()
	verbatim := "no tombstone with the sealed digest exists; legacy history, deletion, or a coherent rewrite are indistinguishable"
	t.Run("deletion", func(t *testing.T) {
		t.Parallel()
		cfgPath, dbPath, approvalID := parkedRequest(t)
		receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)
		db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
		if err != nil {
			t.Fatalf("raw: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`); err != nil {
			t.Fatalf("prune-style delete: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM approval_tombstones WHERE action_id = 'act_inbox1'`); err != nil {
			t.Fatalf("deletion: %v", err)
		}
		_ = db.Close()
		code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
		out := stdout + stderr
		if code != 0 || !strings.Contains(out, verbatim) {
			t.Fatalf("AUDIT R11-R7: deletion must end in the verbatim ambiguous note: %d %q", code, out)
		}
		if strings.Contains(out, "pre-v10") || strings.Contains(out, "pre-tombstone") {
			t.Fatalf("the pre-v10 certainty is dead: %q", out)
		}
	})
	t.Run("coherent_rewrite", func(t *testing.T) {
		t.Parallel()
		cfgPath, dbPath, approvalID := parkedRequest(t)
		receiptID := approvedReceiptID(t, cfgPath, dbPath, approvalID)
		// Rewrite the tombstone into ANOTHER self-consistent story:
		// digest recomputed over the new preimage — coherent, wrong.
		other := action.Approval{
			ApprovalID: "apr_rewrite000000000000000000001", ActionID: "act_inbox1",
			ActionDigest: "sha256:other", PreviewDigest: "sha256:other",
			PolicyVersion: 9, PolicyDigest: "sha256:other",
			DecisionPrincipalID: "principal_saboteur", Decision: "approved",
			DecisionAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
		}
		db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
		if err != nil {
			t.Fatalf("raw: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM actions WHERE action_id = 'act_inbox1'`); err != nil {
			t.Fatalf("prune-style delete: %v", err)
		}
		if _, err := db.Exec(`UPDATE approval_tombstones SET approval_id=?, approval_digest=?,
		    action_digest=?, preview_digest=?, policy_version=?, policy_digest=?,
		    decision_principal_id=?, decision=?, decision_at=? WHERE action_id='act_inbox1'`,
			other.ApprovalID, other.Digest(), other.ActionDigest, other.PreviewDigest,
			other.PolicyVersion, other.PolicyDigest, other.DecisionPrincipalID,
			other.Decision, other.DecisionAt.Format(time.RFC3339Nano)); err != nil {
			t.Fatalf("coherent rewrite: %v", err)
		}
		_ = db.Close()
		code, stdout, stderr := runIntentCLI(t, "receipt", "verify", "--config", cfgPath, receiptID)
		out := stdout + stderr
		if code != 0 || !strings.Contains(out, verbatim) {
			t.Fatalf("AUDIT R11-R7: a coherent rewrite is INDISTINGUISHABLE from absence and gets the same note: %d %q", code, out)
		}
	})
}
