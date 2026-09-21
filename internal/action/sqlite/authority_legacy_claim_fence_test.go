// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAuthority_LegacyClaimRefusesAStrictBornApproval attacks the strict
// enforcement from the door that enforced nothing: the LEGACY claim. A
// strict-born approval handed to it gave up its parameters with no debit and no
// start proof — the only fence was a flag in the executor's configuration (the
// adversary's pass over this phase, F9, its probe P-F). The store now refuses
// by what the ROW says: a row born under the strict marker starts only through
// the authority door, whoever asks and however they were configured.
//
// Evidence level: in-process, one real SQLite store, both claim doors called
// directly.
// Probing mutation executed: neutralize the marker check in the legacy claim —
// red with «error = <nil>, want … a strict-born approval starts only through
// the authority door» and «the strict door … action/sqlite: approval
// parameters column empty», the parameters having left with nothing debited.
func TestAuthority_LegacyClaimRefusesAStrictBornApproval(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	parked := parkStrictAuthorityFixture(t, f)
	a := approveStrictAuthorityFixture(t, f, parked)
	law := PolicyPin{Version: 1, Digest: "sha256:authority-law"}

	params, _, err := f.store.ClaimApprovalParamsUnderDigest(context.Background(), a.ApprovalID, &law, a.ActionDigest, nil)
	if !errors.Is(err, ErrApprovalRequiresAuthority) {
		t.Errorf("legacy claim over a strict-born approval: error = %v, want %v", err, ErrApprovalRequiresAuthority)
	}
	if len(params) != 0 {
		t.Errorf("the refused claim handed out parameters: %q", params)
	}
	for what, query := range map[string]string{
		"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
		"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
	} {
		if n := authorityScalar(t, f.store, query); n != 0 {
			t.Errorf("%s = %d, want 0", what, n)
		}
	}
	kept, err := f.store.ApprovalParams(context.Background(), a.ApprovalID)
	if err != nil || string(kept) != `{}` {
		t.Errorf("parked parameters after the refused claim = %q, %v; want them intact", kept, err)
	}
	// The refusal cost nothing: the authority door still starts the same approval.
	if _, err := f.store.StartApprovedAuthorization(context.Background(), a.ApprovalID, law, a.ActionDigest,
		f.now.Add(3*time.Second)); err != nil {
		t.Errorf("the strict door could not start the approval afterwards: %v", err)
	}
}
