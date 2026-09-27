// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R13 of the redesign plan: the approvals doors NAME a foreign or unreadable
// ledger — 409 ledger_foreign_profile, 503 ledger_unreadable — never a 500.
// Evidence: httptest over the real handlers with a fake Approvals.
//
// PROBING MUTATION: drop the two entries from the outcomes registry → both
// legs answer 500 and redden (and FR-TEST-6 reddens through the spec).

package controlapi_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Sebastian197/korvun/internal/controlapi"
)

func TestApprovals_aForeignOrUnreadableLedgerIsNamed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		error_ string
	}{
		{"foreign", fmt.Errorf("%w: owner x", controlapi.ErrLedgerForeign), http.StatusConflict, "ledger_foreign_profile"},
		{"unreadable", fmt.Errorf("%w: row missing", controlapi.ErrLedgerUnreadable), http.StatusServiceUnavailable, "ledger_unreadable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeApprovals{approveErr: tc.err}
			srv := approvalsServer(t, f)
			res := doReq(t, "POST", srv.URL+"/api/approvals/apr_00000000000000000000000000000001/approve", approvalsToken, `{"digest":"sha256:abc"}`)
			if res.StatusCode != tc.status {
				t.Fatalf("approve over a %s ledger: status %d, want %d", tc.name, res.StatusCode, tc.status)
			}
			body := decode[errorBody](t, res)
			if body.Error != tc.error_ || body.Message == "" {
				t.Fatalf("approve over a %s ledger: name %q text %q, want %q with a text", tc.name, body.Error, body.Message, tc.error_)
			}
		})
	}
}
