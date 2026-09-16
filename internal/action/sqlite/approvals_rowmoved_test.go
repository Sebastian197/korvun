// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The comparison covers EVERY column, and says so by walking the struct.
//
// approvalRowMoved is a switch: a list. A list is exactly what the class cure
// replaced — the P1-1 cure compared two columns because an external review
// named two columns, and the next column added to action.Approval would be
// outside this one just as silently. This mould walks the type with reflection
// and requires the function to notice a change in every field, so a new column
// reddens it on the commit that adds it rather than on the incident that finds
// it.
//
// Evidence level, honest: in-process unit test over the package-private
// comparison; no store, no connection.
package sqlite

import (
	"reflect"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// baselineApproval is a row with every field set to something non-zero, so a
// mutation per field is always a real change.
func baselineApproval() action.Approval {
	at := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	return action.Approval{
		ApprovalID:          "apr_baseline",
		SchemaVersion:       1,
		ActionID:            "act_baseline",
		ActionDigest:        "sha256:aaaa",
		PreviewDigest:       "sha256:bbbb",
		RequestedFrom:       "principal_operator",
		Reason:              "require_approval",
		RiskSummary:         "write_irreversible — irreversible, no documented undo",
		PolicyVersion:       7,
		PolicyDigest:        "sha256:law",
		RequestedAt:         at,
		ExpiresAt:           at.Add(time.Hour),
		Status:              action.ApprovalApproved,
		DecisionPrincipalID: "principal_operator",
		Decision:            "approved",
		DecisionAt:          at.Add(time.Minute),
		Comment:             "the operator's words",
		DecisionReceiptID:   "rcpt_baseline",
	}
}

// moveField returns the baseline with ONE field changed, for any field type
// action.Approval uses today.
func moveField(t *testing.T, base action.Approval, i int) action.Approval {
	t.Helper()
	moved := base
	f := reflect.ValueOf(&moved).Elem().Field(i)
	switch f.Interface().(type) {
	case string:
		f.SetString(f.String() + "-moved")
	case int:
		f.SetInt(f.Int() + 1)
	case int64:
		f.SetInt(f.Int() + 1)
	case time.Time:
		f.Set(reflect.ValueOf(f.Interface().(time.Time).Add(time.Second)))
	case action.ApprovalStatus:
		f.SetString(string(action.ApprovalRejected))
	default:
		t.Fatalf("field %s has a type this mould does not know how to move: %s",
			reflect.TypeOf(base).Field(i).Name, f.Type())
	}
	return moved
}

// TestApprovalRowMoved_coversEveryColumn is the mould the comparison's godoc
// names.
//
// Probing mutation (executed, red, declared in the canto): delete any arm of
// approvalRowMoved's switch ⇒ that field's row reddens.
func TestApprovalRowMoved_coversEveryColumn(t *testing.T) {
	t.Parallel()
	base := baselineApproval()
	typ := reflect.TypeOf(base)
	if same := approvalRowMoved(base, base); same != "" {
		t.Fatalf("a row compared against itself moved in %q", same)
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			moved := moveField(t, base, i)
			if got := approvalRowMoved(base, moved); got == "" {
				t.Fatalf("%s changed and the comparison saw nothing", name)
			}
		})
	}
	// The baseline must itself be complete: a zero field would make its row
	// pass for the wrong reason.
	v := reflect.ValueOf(base)
	for i := 0; i < typ.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("baselineApproval leaves %s zero — its row would prove nothing", typ.Field(i).Name)
		}
	}
}
