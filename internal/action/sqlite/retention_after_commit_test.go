// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// The P1 of the ficha «Un aparcamiento confirmado puede devolver error», attacked at every door of its class.
//
// THE FAULT. Four writers paid the retention cadence after their own commit and
// RETURNED its error: the park (`approvals.go`), `RecordAttempt`
// (`store.go`), the identified writer (`evidence.go`) and the authenticated one
// (`identity_v2.go`). So a durable write reported itself as a refusal. For the
// park that is not a cosmetic wrong: `internal/app/approvals.go` dropped the
// approval id on the floor, the executor read a refusal, took its denial branch
// and recorded a SECOND actions row in StateDenied — while the PENDING_APPROVAL
// row and its approvals row sat in the store waiting for a human nobody would
// ever tell. The store's record and the operator's belief disagreed
// permanently, and no door reconciles them.
//
// THE GUARANTEE, in two halves, because dropping the failure would only trade
// the lie for a blindness: a committed write reports SUCCESS, and the
// housekeeping failure reaches the observer.
//
// Evidence level, honest: in-process, one real SQLite file per row, the
// production writers, and a real trigger that ABORTS the prune's DELETE — an
// oracle by impossibility rather than a fake returning an error.
func TestRetention_aCommittedWriteNeverReportsTheCadencesFailure(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		// write runs the production writer over a store whose cadence is
		// armed to fail, and returns what that writer told its caller.
		write func(t *testing.T, store *Store, id string) error
		// durable proves the write really landed, so the success the writer
		// reported is not itself a lie.
		durable func(t *testing.T, store *Store, id string) error
	}{
		{
			name: "the park, the door the P1 was found on",
			write: func(t *testing.T, store *Store, id string) error {
				t.Helper()
				env := testEnvelope(id)
				env.IntentID = action.RootIntentID
				env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
				env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
				b, err := action.NewBoundApprovalRequest(env, `{"a":1}`, parkContextForRetention())
				if err != nil {
					t.Fatalf("factory: %v", err)
				}
				return store.CreateApprovalRequest(context.Background(), b)
			},
			durable: func(t *testing.T, store *Store, id string) error {
				t.Helper()
				_, _, err := store.GetApprovalByAction(context.Background(), id)
				return err
			},
		},
		{
			name: "RecordAttempt",
			write: func(t *testing.T, store *Store, id string) error {
				t.Helper()
				return store.RecordAttempt(context.Background(), testEnvelope(id),
					Decision{Outcome: "deny", Rule: "r"}, action.StateDenied)
			},
			durable: func(t *testing.T, store *Store, id string) error {
				t.Helper()
				_, err := store.Get(context.Background(), id)
				return err
			},
		},
		{
			name: "the identified writer",
			write: func(t *testing.T, store *Store, id string) error {
				t.Helper()
				return store.RecordAttemptIdentified(context.Background(), testEnvelope(id),
					Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, testIdentity())
			},
			durable: func(t *testing.T, store *Store, id string) error {
				t.Helper()
				_, err := store.Get(context.Background(), id)
				return err
			},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			store, _ := openTemp(t)
			// The prune only ever removes TERMINAL rows, so a store holding
			// nothing terminal has nothing to delete and its cadence SUCCEEDS
			// however broken the DELETE is. Two of these rows write a
			// non-terminal state — a parked request and an authorized one — and
			// without this seed their cadence would never fail, so the mould
			// would have passed for the wrong reason. (It did, once: the
			// observer assertion is what caught it.)
			if err := store.RecordAttempt(context.Background(), testEnvelope("act_seed_terminal"),
				Decision{Outcome: "deny", Rule: "seed"}, action.StateDenied); err != nil {
				t.Fatalf("seeding a prunable row: %v", err)
			}
			// Only NOW is the cadence armed and the DELETE made impossible, so
			// the seed above is not itself the failure under test.
			store.capRows = 0
			store.pruneEvery = 1
			store.writes = 0
			var heard []error
			store.SetRetentionFailureObserver(func(err error) { heard = append(heard, err) })
			blockWrites(t, store, "DELETE")

			const id = "act_retention"
			if err := row.write(t, store, id); err != nil {
				t.Fatalf("a committed write reported the cadence's failure as its own: %v", err)
			}
			if err := row.durable(t, store, id); err != nil {
				t.Fatalf("the write the caller was told succeeded is not in the store: %v", err)
			}
			if len(heard) != 1 {
				t.Fatalf("the observer heard %d failures, want exactly 1 — a failure "+
					"swallowed in silence is the other half of this guarantee", len(heard))
			}
			if !strings.Contains(heard[0].Error(), "periodic") {
				t.Fatalf("the observer heard %q, which names no cadence pass", heard[0])
			}
		})
	}
}

// TestRetention_theSweepBranchIsHeardToo closes the gap the adversary found:
// the prune branch had a mould and the SWEEP branch had none, so deleting its
// observer call left every package green — the exact blindness the design
// rejected, invisible.
//
// The sweep runs only when the prune SUCCEEDS, so this store is kept under its
// cap (nothing to delete) and given an approval whose window has already
// closed, which the sweep must then touch — with the touch made impossible.
func TestRetention_theSweepBranchIsHeardToo(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	parked := boundPark(t, store, "act_sweep")
	// The window closes BEFORE the trigger is installed, or the setup would be
	// the thing the trigger refuses.
	corruptCell(t, store, "approvals", "expires_at", "approval_id", parked.ApprovalID,
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano))

	// The CONTROL: with nothing blocked the sweep reaches that row and is
	// silent. Without this the assertion below could pass because the sweep
	// never ran at all.
	swept, _, err := store.SweepExpiredApprovals(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("control sweep: %v", err)
	}
	if swept == 0 {
		t.Fatalf("control: the sweep touched nothing, so the branch under test is unreachable here")
	}

	// A second expirable row, so the armed sweep has work again.
	second := boundPark(t, store, "act_sweep_two")
	corruptCell(t, store, "approvals", "expires_at", "approval_id", second.ApprovalID,
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano))

	store.capRows = 1_000_000 // the prune finds nothing to delete and succeeds
	store.pruneEvery = 1
	store.writes = 0
	var heard []error
	store.SetRetentionFailureObserver(func(err error) { heard = append(heard, err) })
	blockWrites(t, store, "UPDATE")

	if err := store.RecordAttempt(context.Background(), testEnvelope("act_sweep_write"),
		Decision{Outcome: "deny", Rule: "r"}, action.StateDenied); err != nil {
		t.Fatalf("a committed write reported the sweep's failure as its own: %v", err)
	}
	if len(heard) != 1 {
		t.Fatalf("the observer heard %d failures, want exactly 1 from the sweep branch", len(heard))
	}
	if !strings.Contains(heard[0].Error(), "sweep") {
		t.Fatalf("the observer heard %q, which does not name the sweep", heard[0])
	}
}

// TestRetention_aFailedCommitIsStillTheCallersError is the CONTROL. The cure
// must not make every writer answer nil: a transaction that did NOT commit is
// still the caller's failure, and only the tail after a successful commit moved.
func TestRetention_aFailedCommitIsStillTheCallersError(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.pruneEvery = 1
	store.SetRetentionFailureObserver(func(error) {
		t.Errorf("the observer heard a failure for a write that never committed")
	})
	blockWrites(t, store, "INSERT")

	if err := store.RecordAttempt(context.Background(), testEnvelope("act_nocommit"),
		Decision{Outcome: "deny", Rule: "r"}, action.StateDenied); err == nil {
		t.Fatal("a write whose own INSERT was refused reported success")
	}
}

// TestRetention_noObserverIsADroppedFailureAndNotAPanic pins the declared cost
// of the nil default: the store does not crash, and the failure is gone. It is
// the reason the app wires an observer rather than leaving it unset.
func TestRetention_noObserverIsADroppedFailureAndNotAPanic(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	store.capRows = 0
	store.pruneEvery = 1
	blockWrites(t, store, "DELETE")

	if err := store.RecordAttempt(context.Background(), testEnvelope("act_noobs"),
		Decision{Outcome: "deny", Rule: "r"}, action.StateDenied); err != nil {
		t.Fatalf("with no observer the write still reported the cadence's failure: %v", err)
	}
	if _, err := store.Get(context.Background(), "act_noobs"); err != nil {
		t.Fatalf("the write did not land: %v", err)
	}
}

func parkContextForRetention() action.ApprovalContext {
	return action.ApprovalContext{
		IntentPurpose: "retention cadence",
		GrantID:       "grant_1", GrantDepth: 1, CostLine: "1 of 5",
		ToolCage: "echo cage",
		Descriptor: action.EffectDescriptor{
			Class: action.EffectWriteIrreversible, DataEgress: true,
		},
		HasDescriptor: true,
		LawVersion:    7, LawDigest: "sha256:law",
		Rule: "require_approval",
		Now:  time.Now().UTC(), TTL: time.Hour,
	}
}

// TestRetention_theAuthenticatedWriterIsCuredToo is the FOURTH door of the
// class, and it exists because the adversary proved the other three did not
// cover it: with only `identity_v2.go`'s call site left uncured, every package
// in the tree stayed green while the P1 survived whole at the door three
// operator verbs use (`intent`, `authority` and `grant` all reach it through
// `openOperatorStoreSealed`).
//
// It is a function rather than a row of the table above because
// `RecordAttemptAuthenticated` needs a signed identity, and that fixture brings
// its own store.
//
// Evidence level, honest: in-process, one real SQLite file, the production
// writer, and a real trigger that ABORTS the prune's DELETE.
func TestRetention_theAuthenticatedWriterIsCuredToo(t *testing.T) {
	store, resolver, issuer, _, now := identityStoreFixture(t)

	// A prunable row first: the prune only removes TERMINAL rows, so without
	// this the cadence would succeed however broken the DELETE is, and the
	// mould would pass for the wrong reason.
	if err := store.RecordAttempt(context.Background(), testEnvelope("act_auth_seed"),
		Decision{Outcome: "deny", Rule: "seed"}, action.StateDenied); err != nil {
		t.Fatalf("seeding a prunable row: %v", err)
	}
	store.capRows = 0
	store.pruneEvery = 1
	store.writes = 0
	var heard []error
	store.SetRetentionFailureObserver(func(err error) { heard = append(heard, err) })
	blockWrites(t, store, "DELETE")

	const actionID = "act_auth_retention"
	env, evidence := identityAttempt(t, resolver, issuer, actionID, now)
	if err := store.RecordAttemptAuthenticated(context.Background(), env,
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, evidence); err != nil {
		t.Fatalf("a committed authenticated write reported the cadence's failure as its own: %v", err)
	}
	if _, err := store.Get(context.Background(), actionID); err != nil {
		t.Fatalf("the write the caller was told succeeded is not in the store: %v", err)
	}
	if len(heard) != 1 {
		t.Fatalf("the observer heard %d failures, want exactly 1", len(heard))
	}
}
