// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestV0151B_P2_6_anOutOfRangeSchemaVersionIsEvidenceCorrupt: a stored
// schema_version outside the int32 range is corrupt evidence — and only that —
// when the raw reader narrows it to int, instead of a value truncated on a
// platform whose int is 32 bits wide (the code-scanning alert
// go/incorrect-integer-conversion on approvalRawIntAsInt, PR #46).
//
// Evidence: real store file, the cell written through a second real
// connection, in-process.
// The rows sit on both edges of int32 (2147483648, -2147483649) and at ±2^32,
// so a bound widened to uint32 reddens the edge rows.
// Probing mutations EXECUTED: drop the range check in approvalRawIntAsInt ⇒
// every row red (m69); widen the bound to uint32 ⇒ the two edge rows red (m70).
func TestV0151B_P2_6_anOutOfRangeSchemaVersionIsEvidenceCorrupt(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"2147483648", "-2147483649", "4294967296", "-4294967296"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			store, _ := sealedStore(t)
			a, _ := pendingRequest(t, store, "act_range_"+value)
			attack(t, store, `UPDATE approvals SET schema_version = `+value+` WHERE approval_id = ?`, a.ApprovalID) // #nosec G202 -- test-owned literals

			_, _, err := store.GetApproval(context.Background(), a.ApprovalID)
			if !errors.Is(err, ErrApprovalEvidenceCorrupt) {
				t.Fatalf("GetApproval err = %v, want ErrApprovalEvidenceCorrupt", err)
			}
			if errors.Is(err, ErrApprovalUnreadable) {
				t.Fatalf("err = %v carries BOTH sentinels: corrupt and unreadable are exclusive (class i)", err)
			}
			_, lerr := store.ListApprovals(context.Background(), action.ApprovalPending)
			if !errors.Is(lerr, ErrApprovalEvidenceCorrupt) {
				t.Fatalf("ListApprovals err = %v, want ErrApprovalEvidenceCorrupt", lerr)
			}
			if errors.Is(lerr, ErrApprovalUnreadable) {
				t.Fatalf("ListApprovals err = %v carries BOTH sentinels (class i)", lerr)
			}
		})
	}
}
