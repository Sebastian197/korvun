// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C2 of the E5 consolidation (second external audit): the preview is
// sealed FOR REAL — a stored preview that parses fine but no longer
// re-derives the pinned preview_digest is a lie about what the human
// would read, and both the READ and the DECISION touch must refuse it
// BY NAME. The parse-breaking corruption case already existed; this is
// the harder half: valid shape, wrong content.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// swapPreview replaces the sealed preview with a DIFFERENT valid one —
// the saboteur who keeps the shape and changes the story.
func swapPreview(t *testing.T, store *Store, approvalID string) {
	t.Helper()
	env2 := testEnvelope("act_c2_other")
	_, other := testApprovalFor(env2)
	other.IntentPurpose = "a different story than the one pinned"
	corruptCell(t, store, "approvals", "canonical_preview", "approval_id",
		approvalID, string(action.CanonicalPreview(other)))
}

func TestPreviewSwap_readRefusesByName(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	env := testEnvelope("act_c2_read")
	a, p := testApprovalFor(env)
	if err := store.createApprovalParts(ctx, env,
		Decision{Outcome: "require_approval", Rule: "require_approval"}, a, p, `{"a":1}`); err != nil {
		t.Fatalf("birth: %v", err)
	}
	swapPreview(t, store, a.ApprovalID)
	_, _, err := store.GetApproval(ctx, a.ApprovalID)
	if err == nil {
		t.Fatal("AUDIT C2: a swapped preview must refuse the READ")
	}
	if !strings.Contains(err.Error(), "preview_digest_mismatch") {
		t.Fatalf("the refusal must name preview_digest_mismatch: %v", err)
	}
}

func TestPreviewSwap_decisionRefusesByName(t *testing.T) {
	t.Parallel()
	store, _ := sealedStore(t)
	ctx := context.Background()
	env := testEnvelope("act_c2_dec")
	a, p := testApprovalFor(env)
	if err := store.createApprovalParts(ctx, env,
		Decision{Outcome: "require_approval", Rule: "require_approval"}, a, p, `{"a":1}`); err != nil {
		t.Fatalf("birth: %v", err)
	}
	swapPreview(t, store, a.ApprovalID)
	envD, identD := operatorDecisionEnv("reject", a.ApprovalID)
	_, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
		a.RequestedAt.Add(time.Minute), envD, identD, "")
	if err == nil {
		t.Fatal("AUDIT C2: a swapped preview must refuse the DECISION touch")
	}
	if !strings.Contains(err.Error(), "preview_digest_mismatch") {
		t.Fatalf("the refusal must name preview_digest_mismatch: %v", err)
	}
}
