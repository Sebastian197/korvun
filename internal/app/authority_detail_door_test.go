// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/action/executor"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/tool"
)

// TestApprovalsAdapter_DetailCarriesTheStoredAuthority attacks FR-UI-01 at the
// door the screen actually reads: ApprovalsAdapter.Detail, the production
// adapter behind GET /api/approvals/{id}. Until this mould the adapter's copy of
// the authority snapshot into the API document was executed by NO Go test — only
// the browser scenario reached it — and neither was the name it gives a snapshot
// that no longer verifies.
//
// A strict request is parked by the REAL coordinator through the production
// recorder; one more start is then spent against the same intent and grant, so
// the live remainder differs from the parked one. The document must carry what
// the store signed at the park — requester, intent, purpose, chain, and the
// remainder of THAT instant — and a snapshot rewritten without the key must be
// named evidence-corrupt, never served and never called unavailable.
//
// Evidence level: in-process; the production adapter over a real strict store
// built through exported doors, the park through the real coordinator. NOT the
// HTTP handler and NOT a browser.
// Probing mutations executed, each alone: (1) the adapter drops the authority
// object — red with «the document carries no authority object»; (2) the adapter
// stops naming a corrupt snapshot — red with «error = approvals: store
// unavailable», the residual of the read door.
func TestApprovalsAdapter_DetailCarriesTheStoredAuthority(t *testing.T) {
	ctx := context.Background()
	f := newStrictDoorFixture(t)
	probe, launch := &strictDoorTool{name: "probe"}, &strictDoorTool{name: "launch"}
	exec := f.coordinator(tool.Registry{"probe": probe, "launch": launch})

	parked := f.submit(t, exec, "launch")
	if parked.Branch != executor.BranchPending || parked.ApprovalID == "" {
		t.Fatalf("pending birth: branch=%q approval=%q error=%v", parked.Branch, parked.ApprovalID, parked.ApprovalError)
	}
	// One live start AFTER the park: the intent allows five, so live is four.
	if spent := f.submit(t, exec, "probe"); spent.Branch != executor.BranchExecuted {
		t.Fatalf("the live start after the park: branch=%q error=%v", spent.Branch, spent.RecordError)
	}

	adapter := NewApprovalsAdapter(&config.Config{Approvals: &config.ApprovalsConfig{Enabled: true}}, f.store)
	doc, err := adapter.Detail(ctx, parked.ApprovalID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if doc.Authority == nil {
		t.Fatal("the document carries no authority object")
	}
	a := doc.Authority
	if a.RequesterPrincipalID != "principal_webhook" || a.IntentID != "int_strict_doors" ||
		a.IntentPurpose != "Exercise the strict doors" {
		t.Errorf("authority = %q under %q (%q), want the requester and the intent the store established",
			a.RequesterPrincipalID, a.IntentID, a.IntentPurpose)
	}
	if len(a.PrincipalChain) != 1 || a.PrincipalChain[0] != "principal_brain_alpha" {
		t.Errorf("principal chain = %v, want [principal_brain_alpha]", a.PrincipalChain)
	}
	if a.Budget.Kind != "finite" || a.Budget.Remaining == nil || *a.Budget.Remaining != 5 {
		t.Errorf("budget = %q %v, want the 5 that remained WHEN IT WAS PARKED, not the live 4", a.Budget.Kind, a.Budget.Remaining)
	}

	// A writer with the database and without the key: coherent columns, a
	// signature that is not the key's.
	hand, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f.dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hand.Close() }()
	if _, err := hand.ExecContext(ctx, `UPDATE authorization_snapshots SET signature='00' WHERE approval_id=?`,
		parked.ApprovalID); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Detail(ctx, parked.ApprovalID); !errors.Is(err, controlapi.ErrApprovalEvidenceCorrupt) {
		t.Errorf("detail over a snapshot that no longer verifies: error = %v, want %v", err, controlapi.ErrApprovalEvidenceCorrupt)
	}
}
