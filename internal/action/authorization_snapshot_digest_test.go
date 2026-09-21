// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"errors"
	"testing"
	"time"
)

// TestAuthorizationSnapshotV1_EmptyEvidenceDigestIsMalformed attacks the
// parser with the one input it answered with NOTHING: bytes that are canonical
// and complete except for the identity evidence digest. It returned the zero
// snapshot AND a nil error — a caller that trusted the nil held a snapshot of
// no action, under no intent (the adversary's pass over piece 3 phase 3, F9,
// its probe P-C). A second belt caught it in the store; the parser must not
// depend on one.
//
// Evidence level: unit.
// Probing mutation executed: drop the digest from the parser's refusal — red
// with «error = <nil>, want action: authorization snapshot malformed».
func TestAuthorizationSnapshotV1_EmptyEvidenceDigestIsMalformed(t *testing.T) {
	remaining := int64(2)
	snapshot := AuthorizationSnapshotV1{
		Kind: AuthorizationSnapshotPending, ActionID: "act3_1", ApprovalID: "apr3_1",
		RequesterPrincipalID: "principal_requester", ActorPrincipalID: "principal_agent",
		IdentityEvidenceDigest: "", IntentID: "int_a", IntentVersion: 1,
		IntentDigest: "sha256:intent", IntentPurpose: "Send an approved update",
		PrincipalChain: []string{"principal_operator", "principal_agent"},
		BudgetKind:     AuthorizationBudgetFinite, BudgetRemaining: &remaining,
		RecordedAt: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC),
	}
	parsed, err := ParseAuthorizationSnapshotV1(snapshot.CanonicalBytes())
	if !errors.Is(err, ErrAuthorizationSnapshotMalformed) {
		t.Errorf("error = %v, want %v", err, ErrAuthorizationSnapshotMalformed)
	}
	if parsed.ActionID != "" || parsed.IntentID != "" {
		t.Errorf("a refused parse returned content: %#v", parsed)
	}
}
