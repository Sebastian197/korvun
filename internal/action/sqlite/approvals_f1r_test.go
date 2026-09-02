// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// F1 of the third-pass self-audit (adjudicated 2026-09-02): the READ
// validated only the digest axis (C2), so the self-consistent-but-
// lying preview — rewritten WITH its pinned digest in agreement —
// sailed through GetApproval and the human read a FALSE law on show.
// The decision already caught it (R1); now the READ runs the whole
// ValidatePreviewBinding too: the human never reads a lie.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestGetApproval_theReadRunsTheWholeBinding(t *testing.T) {
	t.Parallel()
	for wantName, sabotage := range birthSaboteurs() {
		store, _ := sealedStore(t)
		ctx := context.Background()
		env := testEnvelope("act_f1r")
		a, p := testApprovalFor(env)
		if err := store.createApprovalParts(ctx, env,
			Decision{Outcome: "require_approval", Rule: a.Reason,
				PolicyVersion: a.PolicyVersion, PolicyDigest: a.PolicyDigest},
			a, p, `{"a":1}`); err != nil {
			t.Fatalf("birth: %v", err)
		}
		// The re-sealed saboteur: content AND pinned digest in agreement.
		lying := p
		sabotage(&a, &lying)
		corruptCell(t, store, "approvals", "canonical_preview", "approval_id",
			a.ApprovalID, string(action.CanonicalPreview(lying)))
		if wantName != "preview_digest_mismatch" {
			corruptCell(t, store, "approvals", "preview_digest", "approval_id",
				a.ApprovalID, lying.Digest())
		}
		_, _, err := store.GetApproval(ctx, a.ApprovalID)
		if err == nil {
			t.Fatalf("AUDIT F1: the READ must refuse the %s saboteur — the human never reads a lie", wantName)
		}
		if !strings.Contains(err.Error(), wantName) {
			t.Fatalf("the read refusal must NAME %s: %v", wantName, err)
		}
	}
}
