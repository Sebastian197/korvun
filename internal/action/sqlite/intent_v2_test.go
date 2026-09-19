// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	_ "modernc.org/sqlite"
)

func intentV2Fixture(t *testing.T) action.IntentContractV2 {
	t.Helper()
	raw := []byte(`{"intent_id":"int_report","schema_version":2,"version":1,"profile_id":"profile_a","owner_principal_id":"principal_operator","purpose":"read reports","operations":[{"namespace":"tool","name":"read_file","version":1}],"allowed_resources":[{"kind":"cage","id":"reports"}],"denied_resources":[],"data_scope":["internal"],"output_destinations":["console"],"effect_classes":["read_external"],"budget":{"total":null,"per_operation":{"tool/read_file@1":0}},"valid_from":"2026-09-19T12:00:00Z","expires_at":"2026-09-20T12:00:00Z","approval":{"required":false},"max_delegation_depth":0}`)
	c, err := action.ParseIntentContractV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func signedIntentStore(t *testing.T) (*Store, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "kernel.db"))
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutSigningKey(context.Background(), action.SigningKeyID(pub), hex.EncodeToString(pub), time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	store.SetIntentV2Signer(func(c action.IntentContractV2) action.SignedIntentContractV2 {
		return action.SignIntentContractV2(priv, c)
	}, func(e action.IntentEventV1) action.SignedIntentEventV1 { return action.SignIntentEventV1(priv, e) })
	t.Cleanup(func() { _ = store.Close() })
	return store, pub, priv
}

func TestIntentV2_TamperedTermsFailAtUse(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", time.Date(2026, 9, 19, 12, 1, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE intent_versions SET canonical_terms = replace(canonical_terms, 'read reports', 'write reports') WHERE intent_id = ?`, c.IntentID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), time.Date(2026, 9, 19, 12, 2, 0, 0, time.UTC)); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("error = %v", err)
	}
}

func TestIntentV2_ValidSignatureDoesNotReplaceLifecycle(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeIntentV2(ctx, c.IntentID, "principal_operator", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), now.Add(3*time.Minute)); !errors.Is(err, action.ErrIntentRevoked) {
		t.Fatalf("error = %v", err)
	}
}

func TestIntentV2_ExpiryAtStartNotOnlyAtQueue(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	now := c.ValidFrom
	if err := store.CreateIntentV2(ctx, c, "principal_operator", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ExpiresAt); !errors.Is(err, action.ErrIntentExpired) {
		t.Fatalf("error = %v", err)
	}
}

func TestIntentV2_SignerCannotChangeTerms(t *testing.T) {
	store, _, priv := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	store.SetIntentV2Signer(func(got action.IntentContractV2) action.SignedIntentContractV2 {
		got.Purpose = "mutated"
		return action.SignIntentContractV2(priv, got)
	}, func(e action.IntentEventV1) action.SignedIntentEventV1 { return action.SignIntentEventV1(priv, e) })
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", c.ValidFrom); !errors.Is(err, action.ErrSignedObjectMutated) {
		t.Fatalf("error = %v", err)
	}
	if n := scalarInt(t, store.db, `SELECT COUNT(*) FROM intent_events WHERE intent_id = ?`, c.IntentID); n != 1 {
		t.Fatalf("events = %d, want draft only", n)
	}
}

func TestIntentV2_RotationPreservesHistoryRejectsRetiredWriter(t *testing.T) {
	store, pub, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	now := c.ValidFrom
	if err := store.CreateIntentV2(ctx, c, "principal_operator", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", now); err != nil {
		t.Fatal(err)
	}
	newPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := store.RotateSigningKey(ctx, action.SigningKeyID(newPub), hex.EncodeToString(newPub), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), now.Add(2*time.Minute)); err != nil {
		t.Fatalf("historical signature under %s: %v", action.SigningKeyID(pub), err)
	}
	if err := store.RevokeIntentV2(ctx, c.IntentID, "principal_operator", now.Add(2*time.Minute)); !errors.Is(err, action.ErrSigningKeyRetired) {
		t.Fatalf("error = %v", err)
	}
}

func TestIntentV2_ExplicitBindingNeverFallsBackToRoot(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	root := c
	root.IntentID = action.RootIntentID
	if err := store.CreateIntentV2(ctx, root, "principal_operator", root.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, root.IntentID, 1, "principal_operator", root.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	b := action.ExecutionBinding{BindingID: "bind_1", ActorPrincipalID: "principal_brain", Channel: "console", IntentID: c.IntentID, IntentVersion: 1, IntentDigest: "sha256:wrong", Revision: 1, Status: action.BindingActive}
	if err := store.PutExecutionBinding(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveExecutionBinding(ctx, "principal_brain", "console", "", c.ValidFrom); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("error = %v", err)
	}
}

// TestIntentV2_MutationAndReceiptAreAtomic demands what its name promises: the
// head, the event and the SEALED RECEIPT move together or not at all.
//
// Three things the twenty-second pass found missing and this shape adds: the
// fixture wires a receipt sealer, so a mould about receipts has seen one; the
// oracle counts events and receipts before and after, so committing the
// rejected evidence while leaving the head behind cannot pass; and each row
// demands ITS OWN named error, because the two refusals do not reach the same
// wire.
//
// The forged row is the one that forces verifyNewEventTx: the key is
// REGISTERED, so the foreign key is satisfied and only the seal verification
// can refuse. The unknown-key row is refused earlier, by the key lookup, and
// stays green under the same mutation — that asymmetry is the finding, kept.
//
// Evidence: real SQLite file, real sealer, counts before and after.
func TestIntentV2_MutationAndReceiptAreAtomic(t *testing.T) {
	for _, row := range []struct {
		name string
		want error
		seal func(action.IntentEventV1, ed25519.PublicKey) action.SignedIntentEventV1
	}{
		{
			name: "forged signature under a registered key",
			want: action.ErrIntentEvidenceCorrupt,
			seal: func(e action.IntentEventV1, pub ed25519.PublicKey) action.SignedIntentEventV1 {
				return action.SignedIntentEventV1{
					Event:        e,
					Digest:       "sha256:" + strings.Repeat("00", sha256.Size),
					SigningKeyID: action.SigningKeyID(pub),
					Signature:    strings.Repeat("00", ed25519.SignatureSize),
				}
			},
		},
		{
			name: "unknown signing key",
			want: ErrNotFound,
			seal: func(e action.IntentEventV1, _ ed25519.PublicKey) action.SignedIntentEventV1 {
				return action.SignedIntentEventV1{Event: e, SigningKeyID: "ed25519:missing", Signature: "00"}
			},
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			store, pub, _ := signedIntentStore(t)
			// A REAL sealer, registered like the boot does: a mould about
			// receipts must have seen one. The counter only proves it ran.
			sealer, sealPub := testSealer(t)
			registerSealerKey(t, store, sealPub)
			sealed := 0
			store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
				sealed++
				return sealer(r)
			})
			ctx := context.Background()
			c := intentV2Fixture(t)
			now := c.ValidFrom
			if err := store.CreateIntentV2(ctx, c, "principal_operator", now); err != nil {
				t.Fatal(err)
			}
			if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", now); err != nil {
				t.Fatal(err)
			}
			if sealed == 0 {
				t.Fatal("the fixture sealed no receipt: this mould would be about nothing")
			}
			events, receipts := intentCounts(t, store, c.IntentID)
			store.SetIntentV2Signer(store.intentContractSigner, func(e action.IntentEventV1) action.SignedIntentEventV1 {
				return row.seal(e, pub)
			})
			err := store.RevokeIntentV2(ctx, c.IntentID, "principal_operator", now.Add(time.Minute))
			if !errors.Is(err, row.want) {
				t.Fatalf("revoke with a bad seal = %v, want %v", err, row.want)
			}
			head, herr := store.IntentHead(ctx, c.IntentID)
			if herr != nil {
				t.Fatal(herr)
			}
			if head.Status != action.LifecycleActive || head.Revision != 2 {
				t.Fatalf("head moved under a refused seal: %+v", head)
			}
			eventsAfter, receiptsAfter := intentCounts(t, store, c.IntentID)
			if eventsAfter != events {
				t.Fatalf("events %d -> %d: a refused lifecycle mutation left evidence behind", events, eventsAfter)
			}
			if receiptsAfter != receipts {
				t.Fatalf("receipts %d -> %d: a refused lifecycle mutation left a receipt behind", receipts, receiptsAfter)
			}
		})
	}
}

// intentCounts is the oracle the atomicity mould needs: what the store HOLDS,
// not what the call returned.
func intentCounts(t *testing.T, store *Store, id string) (events, receipts int) {
	t.Helper()
	ctx := context.Background()
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM intent_events WHERE intent_id=?`, id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	return events, receipts
}

func TestIntentV2_MigrationNeverSignsLegacyAsHistoricalConsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := action.IntentContract{IntentID: "int_legacy", SchemaVersion: 1, OwnerPrincipalID: "operator", Purpose: "legacy", AllowedOperations: []string{"calc"}, AllowedResources: []string{"*"}, Status: action.LifecycleActive, Version: 1}
	if err := store.CreateIntent(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := reopened.GetIntentAnyVersion(context.Background(), legacy.IntentID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provenance != action.IntentProvenanceLegacyUnsigned || got.SignedV2 != nil {
		t.Fatalf("legacy read = %+v", got)
	}
	if n := scalarInt(t, reopened.db, `SELECT COUNT(*) FROM intent_versions WHERE intent_id = ?`, legacy.IntentID); n != 0 {
		t.Fatalf("signed rows = %d", n)
	}
}

func TestMigrationV13AddsIntentV2Tables(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if v, _ := store.SchemaVersion(context.Background()); v != 13 {
		t.Fatalf("schema = %d", v)
	}
	for _, table := range []string{"intent_versions", "intent_events", "intent_heads", "execution_bindings", "authorization_snapshots"} {
		if n := scalarInt(t, store.db, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table); n != 1 {
			t.Fatalf("missing %s", table)
		}
	}
}

func TestIntentV2_PersistenceFailureDoors(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "doors.db")
	plain, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := intentV2Fixture(t)
	if err := plain.CreateIntentV2(ctx, c, "p", c.ValidFrom); err == nil || !strings.Contains(err.Error(), "intent event signer unavailable") {
		t.Fatalf("missing signer = %v, want the unavailable-signer refusal", err)
	}
	_ = plain.Close()
	store, _, _ := signedIntentStore(t)
	bad := c
	bad.Purpose = ""
	if err := store.CreateIntentV2(ctx, bad, "p", c.ValidFrom); !errors.Is(err, action.ErrIntentMalformed) && err == nil {
		t.Fatalf("invalid terms = %v, want a validation refusal", err)
	} else if err == nil {
		t.Fatal("invalid terms accepted")
	}
	if err := store.ActivateIntentV2(ctx, "missing", 1, "p", c.ValidFrom); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing activation = %v", err)
	}
	if _, err := store.IntentHead(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing head = %v", err)
	}
	if err := store.PutExecutionBinding(ctx, action.ExecutionBinding{Status: "BAD"}); err == nil {
		t.Fatal("a binding with an unknown status was accepted")
	} else if !strings.Contains(err.Error(), "invalid binding status") {
		t.Fatalf("bad binding = %v, want the named status refusal", err)
	}
	if _, err := store.ResolveExecutionBinding(ctx, "none", "none", "", c.ValidFrom); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing binding = %v", err)
	}
	if _, err := store.GetIntentAnyVersion(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing any-version = %v", err)
	}
}

func TestIntentV2_ExpireAndHistoryCorruption(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ExpireIntentV2(ctx, c.IntentID, "p", c.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom); !errors.Is(err, action.ErrIntentExpired) {
		t.Fatalf("expired = %v", err)
	}
	if err := store.RevokeIntentV2(ctx, c.IntentID, "p", c.ExpiresAt); !errors.Is(err, action.ErrInvalidLifecycleTransition) {
		t.Fatalf("terminal transition = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE intent_heads SET last_event_digest='sha256:bad' WHERE intent_id=?`, c.IntentID); err != nil {
		t.Fatal(err)
	}
	// ONE name. The history walk runs before the lifecycle switch, so a head
	// whose last_event_digest no longer matches the chain is always corruption
	// and never expiry: the either/or hid that the second class is unreachable
	// here (the twenty-second pass, P2-11).
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("corrupt head = %v, want ErrIntentEvidenceCorrupt", err)
	}
}

func TestIntentV2_PersistenceAdditionalBranches(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err == nil {
		t.Fatal("a duplicate intent version was accepted")
	} else if !strings.Contains(err.Error(), "constraint") {
		t.Fatalf("duplicate = %v, want the primary key's refusal", err)
	}
	contractSigner, eventSigner := store.intentContractSigner, store.intentEventSigner
	store.SetIntentV2Signer(nil, nil)
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); err == nil || !strings.Contains(err.Error(), "intent signer unavailable") {
		t.Fatalf("activation without a signer = %v, want the unavailable-signer refusal", err)
	}
	store.SetIntentV2Signer(contractSigner, eventSigner)
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); !errors.Is(err, action.ErrInvalidLifecycleTransition) {
		t.Fatalf("second activation = %v", err)
	}
	any, err := store.GetIntentAnyVersion(ctx, c.IntentID)
	if err != nil || any.SignedV2 == nil || any.Provenance != action.IntentProvenanceSignedV2 {
		t.Fatalf("signed read = %+v %v", any, err)
	}
	exactVersion, err := store.GetIntentV2(ctx, c.IntentID, 1)
	if err != nil || exactVersion.Digest != c.Digest() {
		t.Fatalf("exact version = %+v %v", exactVersion, err)
	}
	if _, err := store.GetIntentV2(ctx, c.IntentID, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing version = %v", err)
	}
	general := action.ExecutionBinding{BindingID: "general", ActorPrincipalID: "brain", Channel: "console", IntentID: c.IntentID, IntentVersion: 1, IntentDigest: c.Digest(), Revision: 1, Status: action.BindingActive}
	exact := general
	exact.BindingID = "exact"
	exact.ConversationID = "conversation"
	if err := store.PutExecutionBinding(ctx, general); err != nil {
		t.Fatal(err)
	}
	if err := store.PutExecutionBinding(ctx, exact); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveExecutionBinding(ctx, "brain", "console", "conversation", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveExecutionBinding(ctx, "brain", "console", "other", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
}

func TestIntentV2_UnreadableRegisteredKeyFailsClosed(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE signing_keys SET public_key='zz'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("bad key = %v", err)
	}
}

func TestIntentV2_HistoryProjectionRejectsIndependentTampering(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	mutations := []struct{ name, set, restore string }{
		{"revision", `revision=9`, `revision=2`}, {"previous", `previous_event_digest='bad'`, `previous_event_digest=(SELECT digest FROM intent_events WHERE intent_id='int_report' AND revision=1)`},
		{"from", `from_status='REVOKED'`, `from_status='DRAFT'`}, {"signature", `signature='00'`, `signature=(SELECT signature FROM intent_events WHERE intent_id='int_report' AND revision=2)`},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			var original string
			if m.name == "signature" {
				if err := store.db.QueryRow(`SELECT signature FROM intent_events WHERE intent_id=? AND revision=2`, c.IntentID).Scan(&original); err != nil {
					t.Fatal(err)
				}
			}
			// #nosec G202 -- every fragment is a fixed test-table literal above.
			if _, err := store.db.Exec(`UPDATE intent_events SET `+m.set+` WHERE intent_id=? AND revision=2`, c.IntentID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
				t.Fatalf("tamper accepted: %v", err)
			}
			restore := m.restore
			if m.name == "signature" {
				restore = `signature='` + original + `'`
			}
			// #nosec G202 -- restore is either a fixed literal or a signature read from this test database.
			if _, err := store.db.Exec(`UPDATE intent_events SET `+restore+` WHERE intent_id=? AND (revision=2 OR revision=9)`, c.IntentID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIntentV2_RetiredContractSignerCannotActivateDraft(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := store.RotateSigningKey(ctx, action.SigningKeyID(pub), hex.EncodeToString(pub), c.ValidFrom.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom.Add(2*time.Minute)); !errors.Is(err, action.ErrSigningKeyRetired) {
		t.Fatalf("retired activation = %v", err)
	}
}

// TestIntentV2_ClosedStoreReturnsInfrastructureErrors demands ONE non-nil error
// per door, by name of the door. The earlier shape only APPENDED errors that
// were already non-nil, so a door that swallowed its error simply added nothing
// and the loop could not fail: four swallowed-error mutations left the whole
// package green (the twenty-second pass, P2-5). Each row now names its own
// surface and fails on its own.
func TestIntentV2_ClosedStoreReturnsInfrastructureErrors(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	c := intentV2Fixture(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, row := range []struct {
		door string
		call func() error
	}{
		{door: "CreateIntentV2", call: func() error { return store.CreateIntentV2(ctx, c, "p", c.ValidFrom) }},
		{door: "ActivateIntentV2", call: func() error { return store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom) }},
		{door: "RevokeIntentV2", call: func() error { return store.RevokeIntentV2(ctx, c.IntentID, "p", c.ValidFrom) }},
		{door: "ExpireIntentV2", call: func() error { return store.ExpireIntentV2(ctx, c.IntentID, "p", c.ValidFrom) }},
		{door: "PutExecutionBinding", call: func() error {
			return store.PutExecutionBinding(ctx, action.ExecutionBinding{Status: action.BindingActive})
		}},
		{door: "ResolveIntentV2", call: func() error {
			_, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom)
			return err
		}},
		{door: "IntentHead", call: func() error {
			_, err := store.IntentHead(ctx, c.IntentID)
			return err
		}},
		{door: "ResolveExecutionBinding", call: func() error {
			_, err := store.ResolveExecutionBinding(ctx, "a", "c", "", c.ValidFrom)
			return err
		}},
		{door: "GetIntentAnyVersion", call: func() error {
			_, err := store.GetIntentAnyVersion(ctx, c.IntentID)
			return err
		}},
		{door: "GetIntentV2", call: func() error {
			_, err := store.GetIntentV2(ctx, c.IntentID, 1)
			return err
		}},
	} {
		t.Run(row.door, func(t *testing.T) {
			if err := row.call(); err == nil {
				t.Fatalf("%s over a closed store returned nil; an unreadable store must be told", row.door)
			}
		})
	}
}

func TestIntentV2_EventSignerCannotRewriteCreateOrActivate(t *testing.T) {
	store, _, priv := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	contractSigner := store.intentContractSigner
	mutating := func(e action.IntentEventV1) action.SignedIntentEventV1 {
		e.ActorPrincipalID = "other"
		return action.SignIntentEventV1(priv, e)
	}
	store.SetIntentV2Signer(contractSigner, mutating)
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); !errors.Is(err, action.ErrSignedObjectMutated) {
		t.Fatalf("create mutation = %v", err)
	}
	store.SetIntentV2Signer(contractSigner, func(e action.IntentEventV1) action.SignedIntentEventV1 { return action.SignIntentEventV1(priv, e) })
	if err := store.CreateIntentV2(ctx, c, "p", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	store.SetIntentV2Signer(contractSigner, mutating)
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "p", c.ValidFrom); !errors.Is(err, action.ErrSignedObjectMutated) {
		t.Fatalf("activate mutation = %v", err)
	}
}

func scalarInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestIntentV2_SignerCannotChangeTermsInPlace is the sister the earlier mould
// did not have: it mutated Purpose, a field held BY VALUE, so the post-sign
// comparison caught it. A signer that reaches into a slice-backed field mutates
// the array BOTH sides read, and the comparison saw nothing: activation
// committed the event, moved the head and stored a signature over terms nobody
// asked for (the twenty-second pass, P2-6). Nothing may commit.
func TestIntentV2_SignerCannotChangeTermsInPlace(t *testing.T) {
	store, _, priv := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	store.SetIntentV2Signer(func(got action.IntentContractV2) action.SignedIntentContractV2 {
		got.Operations[0].Name = "delete_everything"
		if len(got.DataScope) > 0 {
			got.DataScope[0] = "secrets"
		}
		return action.SignIntentContractV2(priv, got)
	}, store.intentEventSigner)
	err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", c.ValidFrom)
	if !errors.Is(err, action.ErrSignedObjectMutated) {
		t.Fatalf("activation error = %v, want ErrSignedObjectMutated", err)
	}
	head, herr := store.IntentHead(ctx, c.IntentID)
	if herr != nil {
		t.Fatal(herr)
	}
	if head.Status != action.LifecycleDraft || head.Revision != 1 {
		t.Fatalf("head moved under a mutating signer: %+v", head)
	}
	var events int
	if qerr := store.db.QueryRowContext(ctx, `SELECT count(*) FROM intent_events WHERE intent_id=?`, c.IntentID).Scan(&events); qerr != nil {
		t.Fatal(qerr)
	}
	if events != 1 {
		t.Fatalf("events = %d, want the single birth event", events)
	}
	var signed int
	if qerr := store.db.QueryRowContext(ctx, `SELECT count(*) FROM intent_versions WHERE intent_id=? AND coalesce(signature,'')<>''`, c.IntentID).Scan(&signed); qerr != nil {
		t.Fatal(qerr)
	}
	if signed != 0 {
		t.Fatalf("signed rows = %d, want none", signed)
	}
}

// TestIntentV2_CanonicalBytesDoNotMutateTheCaller pins the other half of the
// same defect: the canonicaliser sorts in place, and it used to sort the
// CALLER's arrays.
func TestIntentV2_CanonicalBytesDoNotMutateTheCaller(t *testing.T) {
	c := intentV2Fixture(t)
	c.Operations = []action.OperationRef{{Namespace: "tool", Name: "read_file", Version: 1}, {Namespace: "aaa", Name: "z", Version: 1}}
	before := append([]action.OperationRef(nil), c.Operations...)
	_ = c.CanonicalBytes()
	for i := range before {
		if c.Operations[i] != before[i] {
			t.Fatalf("CanonicalBytes reordered the caller's operations: %+v -> %+v", before, c.Operations)
		}
	}
}

// TestIntentV2_UseRefusesASignatureThatDoesNotVerify isolates the SIGNATURE
// branch of the use door. The earlier AS-INT-03 mould rewrote terms and left
// the digest behind, so its red came from the digest comparison and the
// signature check had no mould at all: neutralising it alone left the package
// green (the twenty-second pass, P2-3). Here the stored terms and the stored
// digest agree with each other, and only the signature is wrong, so the digest
// comparison passes and exactly one branch can refuse.
func TestIntentV2_UseRefusesASignatureThatDoesNotVerify(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	// A second real connection, as an out-of-band writer: the signature bytes
	// are replaced by a syntactically valid signature of the right length that
	// verifies over nothing.
	forged := strings.Repeat("00", ed25519.SignatureSize)
	if _, err := store.db.ExecContext(ctx, `UPDATE intent_versions SET signature=? WHERE intent_id=? AND version=?`, forged, c.IntentID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("resolve error = %v, want ErrIntentEvidenceCorrupt", err)
	}
	if _, err := store.GetIntentV2(ctx, c.IntentID, 1); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("raw read error = %v, want ErrIntentEvidenceCorrupt", err)
	}
	if _, err := store.GetIntentAnyVersion(ctx, c.IntentID); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("any-version read error = %v, want ErrIntentEvidenceCorrupt", err)
	}
}

// TestIntentV2_TamperedTermsAreRefusedByEveryRead closes the door the twenty-
// second pass found open (P2-7): the raw reads returned TAMPERED terms with a
// nil error, beside the digest and signature of the original text.
func TestIntentV2_TamperedTermsAreRefusedByEveryRead(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE intent_versions SET canonical_terms = replace(cast(canonical_terms as text), ?, ?) WHERE intent_id=?`,
		`"purpose":"`+c.Purpose+`"`, `"purpose":"DELETE everything"`, c.IntentID); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		door string
		call func() error
	}{
		{door: "ResolveIntentV2", call: func() error {
			_, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom)
			return err
		}},
		{door: "GetIntentV2", call: func() error {
			_, err := store.GetIntentV2(ctx, c.IntentID, 1)
			return err
		}},
		{door: "GetIntentAnyVersion", call: func() error {
			_, err := store.GetIntentAnyVersion(ctx, c.IntentID)
			return err
		}},
	} {
		t.Run(row.door, func(t *testing.T) {
			if err := row.call(); !errors.Is(err, action.ErrIntentEvidenceCorrupt) {
				t.Fatalf("%s over tampered terms = %v, want ErrIntentEvidenceCorrupt", row.door, err)
			}
		})
	}
}

// TestIntentV2_AnUnsignedDraftIsNotReportedSigned pins the taxonomy: a v2 row
// that was never activated carries no signature, and calling it signed_v2 named
// evidence that does not exist.
func TestIntentV2_AnUnsignedDraftIsNotReportedSigned(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetIntentAnyVersion(ctx, c.IntentID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provenance != action.IntentProvenanceUnsignedV2 {
		t.Fatalf("provenance = %q, want %q", got.Provenance, action.IntentProvenanceUnsignedV2)
	}
	if got.SignedV2 == nil || got.SignedV2.Signature != "" {
		t.Fatalf("unsigned draft came back with a signature: %+v", got.SignedV2)
	}
}

// TestIntentV2_ADraftIsNamedInactiveNotMissing: a version that exists and was
// never activated must be told apart from one that does not exist. The key
// lookup used to run first, and a DRAFT carries no key, so the operator read
// "action not found" about an intent sitting in the store — and the sentinel
// that names the real state was unreachable (the twenty-second pass, P2-8).
func TestIntentV2_ADraftIsNamedInactiveNotMissing(t *testing.T) {
	store, _, _ := signedIntentStore(t)
	ctx := context.Background()
	c := intentV2Fixture(t)
	if err := store.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom); !errors.Is(err, action.ErrIntentInactive) {
		t.Fatalf("resolve of a DRAFT = %v, want ErrIntentInactive", err)
	}
	if _, err := store.ResolveIntentV2(ctx, "int_absent000000000000000000000000", 1, c.Digest(), c.ValidFrom); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve of an absent intent = %v, want ErrNotFound", err)
	}
}

// TestIntentV2_ALegitimateRevocationIsNotCalledCorruption forces the race the
// twenty-second pass captured (P2-9): the use door read the head and then
// walked the event chain in two separate statements, so a legitimate
// revocation committed by a SECOND REAL CONNECTION in between made the chain
// disagree with the head, and the door answered with the gravest name it has.
// Contention is not corruption. The outcome must be the revocation's own name.
//
// Evidence: two real connections to the same file; the writer's COMMIT is
// confirmed before the reader runs.
func TestIntentV2_ALegitimateRevocationIsNotCalledCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kernel.db")
	writer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := writer.PutSigningKey(ctx, action.SigningKeyID(pub), hex.EncodeToString(pub), time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	sign := func(c action.IntentContractV2) action.SignedIntentContractV2 {
		return action.SignIntentContractV2(priv, c)
	}
	signEvent := func(e action.IntentEventV1) action.SignedIntentEventV1 { return action.SignIntentEventV1(priv, e) }
	writer.SetIntentV2Signer(sign, signEvent)
	c := intentV2Fixture(t)
	if err := writer.CreateIntentV2(ctx, c, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	if err := writer.ActivateIntentV2(ctx, c.IntentID, 1, "principal_operator", c.ValidFrom); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	reader.SetIntentV2Signer(sign, signEvent)
	// The revocation COMMITS on the second connection before the reader runs.
	if err := writer.RevokeIntentV2(ctx, c.IntentID, "principal_operator", c.ValidFrom.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	_, err = reader.ResolveIntentV2(ctx, c.IntentID, 1, c.Digest(), c.ValidFrom)
	if errors.Is(err, action.ErrIntentEvidenceCorrupt) {
		t.Fatalf("a committed, legitimate revocation was reported as corruption: %v", err)
	}
	if !errors.Is(err, action.ErrIntentRevoked) {
		t.Fatalf("resolve after revocation = %v, want ErrIntentRevoked", err)
	}
}
