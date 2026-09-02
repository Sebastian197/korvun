// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C4 of the E5 consolidation (second external audit): the THIRD door.
// Operator acts mutate (decide, execute, intent/grant writes) but the
// crash recovery, the schema migration of a previous life and the
// retention prune belong to the SERVER BOOT — a CLI opened BESIDE a
// live server must never close the server's in-flight work, never
// migrate under it, never prune its evidence. The auditor's
// CLI-beside-the-server scenario rides as a permanent member (the R1
// lesson, completed: read-only for consults, operator door for acts,
// full door only for the boot). Reproduction-first contract.

package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestOpenOperator_besideALiveServerTouchesNothingItDoesNotOwn(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/korvun.db"
	server, err := Open(path)
	if err != nil {
		t.Fatalf("server open: %v", err)
	}
	defer func() { _ = server.Close() }()
	ctx := context.Background()
	// The server is MID-EXECUTION of an approved action: params claimed.
	// A full-door open would close it as a crash orphan — a lie while
	// the server lives.
	aBusy, _ := pendingRequest(t, server, "act_busy")
	envB, identB := operatorDecisionEnv("approve", aBusy.ApprovalID)
	if _, err := server.decideApproval(ctx, aBusy.ApprovalID, "approved",
		aBusy.RequestedAt.Add(time.Minute), envB, identB, ""); err != nil {
		t.Fatalf("approve busy: %v", err)
	}
	if _, err := server.ClaimApprovalParams(ctx, aBusy.ApprovalID, nil); err != nil {
		t.Fatalf("claim busy: %v", err)
	}
	// And a separate parked request the operator wants to reject.
	aPark, _ := pendingRequest(t, server, "act_c4park")

	cli, err := OpenOperator(path)
	if err != nil {
		t.Fatalf("AUDIT C4: the operator door must open beside the server: %v", err)
	}
	defer func() { _ = cli.Close() }()
	envD, identD := operatorDecisionEnv("reject", aPark.ApprovalID)
	if _, err := cli.DecideApprovalUnderLaw(ctx, aPark.ApprovalID, "rejected",
		aPark.RequestedAt.Add(time.Minute), envD, identD, "", PolicyPin{}); err != nil {
		t.Fatalf("the operator act must work through the third door: %v", err)
	}
	// The server's in-flight work is UNTOUCHED: no recovery ran.
	rec, err := server.Get(ctx, "act_busy")
	if err != nil || rec.State != action.StateApproved || rec.RecoveryMarker != "" {
		t.Fatalf("AUDIT C4: the CLI must never close the live server's work: %v %v %q",
			err, rec.State, rec.RecoveryMarker)
	}
}

func TestOpenOperator_neverMigratesAPreviousLife(t *testing.T) {
	t.Parallel()
	path := buildV6File(t)
	_, err := OpenOperator(path)
	if err == nil {
		t.Fatal("AUDIT C4: an operator act must never migrate an existing store")
	}
	if !strings.Contains(err.Error(), "schema") || !strings.Contains(err.Error(), "server") {
		t.Fatalf("the refusal must name the schema gap and point at the server boot: %v", err)
	}
	// And the old store is untouched at its version.
	ro, err := sqlOpenVersion(path)
	if err != nil || ro != 6 {
		t.Fatalf("the previous life must stay at v6: %v %d", err, ro)
	}
}

// sqlOpenVersion reads the schema version raw, no doors involved.
func sqlOpenVersion(path string) (int, error) {
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	var v int
	err = db.QueryRow(`SELECT version FROM action_schema`).Scan(&v)
	return v, err
}

func TestOpenOperator_freshProfileBootstrapsButNeverRecoversOrPrunes(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/fresh/korvun.db"
	// A fresh profile is a clean bootstrap (the intent-create-before-
	// first-boot flow stays alive) — there is no previous life to harm.
	store, err := OpenOperator(path)
	if err != nil {
		t.Fatalf("fresh operator open: %v", err)
	}
	defer func() { _ = store.Close() }()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the fresh bootstrap creates the store: %v", err)
	}
}
