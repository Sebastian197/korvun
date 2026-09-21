// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestAuthority_AStoreThatDidNotAnswerIsNotCorruptEvidence attacks the
// taxonomy of the evidence verifiers with an INTACT store that does not answer:
// a real activated profile, one committed strict start, a live transaction, and
// a context that is already over. Every one of these readers runs on each
// strict start, each strict pending birth, each detail read and the strict
// boot, so the class they answer is the class the operator and the screen see.
// An intact book that could not be read must be «store busy» carrying its
// cause — never «corrupt», which sends an operator to look for tampering that
// is not there (the adversary's pass over this phase, F5, its probe P-G).
//
// Evidence level: in-process, the package's own verifiers over a real SQLite
// file, inside one live transaction.
// Probing mutation executed: make authorityReadFailure return the corruption
// sentinel whatever it was given — red on all five rows, each naming the
// corruption sentinel it got back.
func TestAuthority_AStoreThatDidNotAnswerIsNotCorruptEvidence(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 3)
	activation := activateAuthorityFixture(t, f)
	if _, err := f.store.StartAuthorization(context.Background(),
		authorityStartRequest(f, "", f.now.Add(2*time.Second))); err != nil {
		t.Fatalf("the committed start the verifiers read: %v", err)
	}

	tx, err := f.store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var account, operation, tail string
	var spent, sequence int64
	if err := tx.QueryRowContext(context.Background(), `SELECT account_id,operation_key,spent,sequence,tail_digest
		FROM budget_counters WHERE sequence>0 ORDER BY account_id,operation_key LIMIT 1`).
		Scan(&account, &operation, &spent, &sequence, &tail); err != nil {
		t.Fatalf("a debited counter to verify: %v", err)
	}
	var grantID string
	var grantVersion int
	if err := tx.QueryRowContext(context.Background(),
		`SELECT grant_id,grant_version FROM grant_events WHERE revision=1 LIMIT 1`).
		Scan(&grantID, &grantVersion); err != nil {
		t.Fatalf("a grant origin to read: %v", err)
	}

	// Control: with a live context the same readers accept the same evidence,
	// so a refusal below is about the context and nothing else.
	live := context.Background()
	if err := f.store.verifyAuthorityActivationTx(live, tx, f.intent.ProfileID, activation); err != nil {
		t.Fatalf("control, activation ledger: %v", err)
	}
	if err := f.store.verifyAuthorizationStartsTx(live, tx); err != nil {
		t.Fatalf("control, start proofs: %v", err)
	}
	if err := f.store.verifyDebitTailTx(live, tx, account, operation, spent, sequence, tail); err != nil {
		t.Fatalf("control, debit tail: %v", err)
	}

	over, cancel := context.WithCancel(context.Background())
	cancel()
	rows := []struct {
		name    string
		read    func() error
		corrupt error
	}{
		{"activation ledger", func() error {
			return f.store.verifyAuthorityActivationTx(over, tx, f.intent.ProfileID, activation)
		}, ErrAuthorizationSnapshotCorrupt},
		{"start proofs", func() error { return f.store.verifyAuthorizationStartsTx(over, tx) },
			ErrAuthorizationSnapshotCorrupt},
		{"debit tail", func() error {
			return f.store.verifyDebitTailTx(over, tx, account, operation, spent, sequence, tail)
		}, ErrBudgetEvidenceCorrupt},
		{"empty debit tail", func() error {
			return f.store.verifyDebitTailTx(over, tx, "account_never_debited", operation, 0, 0, "")
		}, ErrBudgetEvidenceCorrupt},
		{"grant origin", func() error {
			_, _, err := grantOriginActorTx(over, tx, grantID, grantVersion)
			return err
		}, action.ErrAuthorityEvidenceCorrupt},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := row.read()
			if !errors.Is(err, ErrAuthorityStoreBusy) {
				t.Errorf("error = %v, want %v", err, ErrAuthorityStoreBusy)
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want the cause %v kept in the chain", err, context.Canceled)
			}
			if errors.Is(err, row.corrupt) {
				t.Errorf("error = %v: an intact store that did not answer was named %v", err, row.corrupt)
			}
		})
	}
}
