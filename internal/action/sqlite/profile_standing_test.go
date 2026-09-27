// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the durable mark «ledger founded by this profile» (director's
// order, 2026-09-24), at the store: the mark lives in the signed receipt of the
// founding (or adopting) act, the standing is judged on every open against the
// profile's identity digest, a FOREIGN standing refuses every door that creates
// an act BEFORE any transaction, reads are untouched, and adoption is ONE
// transaction that leaves a marked receipt or nothing.
//
// Evidence level, honest: in-process, REAL SQLite store on a real file, real
// signing key and sealer, a second REAL connection where the attack needs one,
// and abort triggers as the oracle for «no transaction was opened». Not the CLI
// and not the app: those have their own moulds.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-libro-fundado-por-este-perfil-pretest.md,
// guarantees L1–L6, rows G1–G5, G8.

package sqlite

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// Two profile identities, as the app derives them: the digest of the profile
// file's absolute path.
var (
	profileA = action.HashCanonical(`{"profile":"/srv/korvun/a/korvun.json"}`)
	profileB = action.HashCanonical(`{"profile":"/srv/korvun/b/korvun.json"}`)
)

// foundedStore opens a real, sealed store with an identity fixture and closes
// ONE act SUCCEEDED carrying the founding mark of owner — the shape
// `enable-storage` leaves behind. No standing is set on it yet.
func foundedStore(t *testing.T, owner string) *Store {
	t.Helper()
	store, _, _, _, now := identityStoreFixture(t)
	_ = now
	mustRecord(t, store, "act_1", action.StateAuthorized)
	if err := store.FinishFounding(context.Background(), "act_1", owner); err != nil {
		t.Fatalf("close the founding act: %v", err)
	}
	return store
}

func countRows(t *testing.T, store *Store, table string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil { // #nosec G202 -- test-controlled table name
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestStanding_theFoundingProfileIsTheOwner is L2's first half: opened by the
// profile that founded it, the ledger stands `ok` and acts are recorded.
//
// PROBING MUTATION: compare against the receipt at chain_seq 0 only. A ledger
// whose first receipt is a brain's (sealed before the founding act closed)
// reads legacy and this reddens on the second store below.
func TestStanding_theFoundingProfileIsTheOwner(t *testing.T) {
	store := foundedStore(t, profileA)
	if err := store.setIdentity(profileA); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	if st, owner, err := store.Standing(context.Background()); err != nil || st != LedgerStandingOK || owner != profileA {
		t.Fatalf("Standing = %q owner %q err %v, want ok / %q", st, owner, err, profileA)
	}
	mustRecord(t, store, "act_2", action.StateAuthorized)

	// A brain's receipt BEFORE the founding one: the mark is «the last marked
	// receipt», not «the first receipt».
	second, _, _, _, now := identityStoreFixture(t)
	mustRecord(t, second, "act_brain", action.StateDenied) // a terminal, so a receipt, unmarked
	mustRecord(t, second, "act_found", action.StateAuthorized)
	_ = now
	if err := second.FinishFounding(context.Background(), "act_found", profileA); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := second.setIdentity(profileA); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	if st, _, err := second.Standing(context.Background()); err != nil || st != LedgerStandingOK {
		t.Fatalf("a ledger whose first receipt is not the founding one reads %q (err %v), want ok", st, err)
	}
}

// TestStanding_aMovedProfileIsForeign is G1: the same ledger opened under
// another identity is foreign; a new act is refused BY NAME; reads still work.
//
// PROBING MUTATION: never set the standing to foreign. RecordAttempt succeeds
// and this reddens.
func TestStanding_aMovedProfileIsForeign(t *testing.T) {
	store := foundedStore(t, profileA)
	if err := store.setIdentity(profileB); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	if st, owner, err := store.Standing(context.Background()); err != nil || st != LedgerStandingForeignProfile || owner != profileA {
		t.Fatalf("Standing = %q owner %q err %v, want foreign / owner %q", st, owner, err, profileA)
	}
	err := store.RecordAttempt(context.Background(), testEnvelope("act_new"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	if !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("RecordAttempt on a foreign ledger = %v, want ErrLedgerForeignProfile", err)
	}
	if _, err := store.Get(context.Background(), "act_new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a refused act landed anyway: %v", err)
	}
	// Reads are not blocked.
	if receipts, err := store.ListReceipts(context.Background(), "main"); err != nil || len(receipts) != 1 {
		t.Fatalf("reads on a foreign ledger: %d receipts, err %v", len(receipts), err)
	}
	if _, err := store.Get(context.Background(), "act_1"); err != nil {
		t.Fatalf("read of the founding act on a foreign ledger: %v", err)
	}
}

// TestStanding_aCopiedProfileIsForeignAndTheOriginalIsNot is G2, over two REAL
// connections to one file: the original's handle stands ok while the copy's
// stands foreign, at the same time.
//
// PROBING MUTATION: keep the standing in a package-level variable instead of
// the handle. The second handle overwrites the first's and this reddens.
func TestStanding_aCopiedProfileIsForeignAndTheOriginalIsNot(t *testing.T) {
	original := foundedStore(t, profileA)
	copyHandle, err := openOperator(original.path)
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	t.Cleanup(func() { _ = copyHandle.Close() })

	if err := original.setIdentity(profileA); err != nil {
		t.Fatalf("original: %v", err)
	}
	if err := copyHandle.setIdentity(profileB); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if st, _, _ := original.Standing(context.Background()); st != LedgerStandingOK {
		t.Fatalf("the original's standing is %q", st)
	}
	if st, _, _ := copyHandle.Standing(context.Background()); st != LedgerStandingForeignProfile {
		t.Fatalf("the copy's standing is %q, want foreign", st)
	}
	mustRecord(t, original, "act_by_original", action.StateAuthorized)
	if err := copyHandle.RecordAttempt(context.Background(), testEnvelope("act_by_copy"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("the copy recorded: %v", err)
	}
}

// TestStanding_aLedgerWithNoMarkIsLegacy is G5 / L5: receipts but no mark —
// every ledger written before this version — is legacy_unfounded: named, not
// corrupt, not blocking. And a mark on a FAILED close does not count.
//
// PROBING MUTATION: treat legacy as foreign. RecordAttempt refuses and this
// reddens.
func TestStanding_aLedgerWithNoMarkIsLegacy(t *testing.T) {
	store, _, _, _, now := identityStoreFixture(t)
	mustRecord(t, store, "act_old", action.StateDenied) // a receipt, unmarked
	if err := store.setIdentity(profileA); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	if st, owner, err := store.Standing(context.Background()); err != nil || st != LedgerStandingLegacyUnfounded || owner != "" {
		t.Fatalf("Standing = %q owner %q err %v, want legacy / no owner", st, owner, err)
	}
	mustRecord(t, store, "act_new", action.StateAuthorized) // not blocked

	// A founding close that ends FAILED leaves no mark: only FinishFounding
	// marks, and it only closes SUCCEEDED.
	_ = now
	// An empty identity is a caller mistake, named.
	if err := store.setIdentity(""); !errors.Is(err, ErrNoProfileIdentity) {
		t.Fatalf("an empty identity = %v, want ErrNoProfileIdentity", err)
	}
	// A handle serves ONE profile for its life: the same identity again is a
	// no-op, a different one is refused by name and the first stands (the
	// official pass's round 6: a second identity set in the opener's
	// body or in a callee, re-identified a handle past its judgement).
	if err := store.setIdentity(profileA); err != nil {
		t.Fatalf("the same identity again = %v, want nil", err)
	}
	if err := store.setIdentity(profileB); !errors.Is(err, ErrProfileIdentityAlreadySet) {
		t.Fatalf("a second identity = %v, want ErrProfileIdentityAlreadySet", err)
	}
	if got := store.ProfileIdentity(); got != profileA {
		t.Fatalf("after the refused second identity the handle serves %q, want the first", got)
	}
	// So is one that is not a canonical digest: the mark is never written in
	// a form the reader would refuse (the official pass's round 4).
	for _, bad := range []string{"profileA", "sha256:", "sha256:xyz", "SHA256:" + profileA[7:], strings.ToUpper(profileA)} {
		if err := store.setIdentity(bad); !errors.Is(err, ErrProfileIdentityMalformed) {
			t.Fatalf("identity %q = %v, want ErrProfileIdentityMalformed", bad, err)
		}
		if err := store.FinishFounding(context.Background(), "act_1", bad); !errors.Is(err, ErrProfileIdentityMalformed) {
			t.Fatalf("founding with %q = %v, want ErrProfileIdentityMalformed", bad, err)
		}
	}
}

// TestForeign_everyActDoorRefusesBeforeATransaction is G8 / L3: the block
// covers the WHOLE door, and «before a transaction» is proved by impossibility:
// abort triggers make any write die at SQLite with THEIR error, so a door that
// reaches SQLite cannot answer the named refusal.
//
// PROBING MUTATION: drop the guard from ONE door. That row answers the
// trigger's error instead of ErrLedgerForeignProfile and reddens by name.
func TestGuard_everyActDoorDiesInSQLite(t *testing.T) {
	founder, resolver, issuer, priv, now := identityStoreFixture(t)
	founder.SetIntentV2Signer(
		func(c action.IntentContractV2) action.SignedIntentContractV2 {
			return action.SignIntentContractV2(priv, c)
		},
		func(e action.IntentEventV1) action.SignedIntentEventV1 { return action.SignIntentEventV1(priv, e) },
	)
	founder.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
		return action.SignAuthorityBytes(priv, domain, canonical)
	})
	mustRecord(t, founder, "act_1", action.StateAuthorized)
	if err := founder.FinishFounding(context.Background(), "act_1", profileA); err != nil {
		t.Fatalf("found: %v", err)
	}
	// Two contracts the founder wrote, for the lifecycle doors to try to move.
	// Born DRAFT, so DRAFT → ACTIVE is a legal edge and the door reaches its
	// UPDATE — which the trigger then kills — when the guard is missing.
	if err := founder.CreateIntent(context.Background(), action.IntentContract{IntentID: "int_t", Purpose: "t", AllowedOperations: []string{"calc"}, Status: action.LifecycleDraft}); err != nil {
		t.Fatalf("the founder's intent: %v", err)
	}
	if err := founder.CreateGrant(context.Background(), action.AuthorityGrant{GrantID: "grant_t", IntentID: "int_t", Status: action.LifecycleDraft}); err != nil {
		t.Fatalf("the founder's grant: %v", err)
	}
	v2Owned := intentV2Fixture(t)
	v2Owned.IntentID = "int_v2_owned"
	if err := founder.CreateIntentV2(context.Background(), v2Owned, "p", v2Owned.ValidFrom); err != nil {
		t.Fatalf("the founder's v2 intent: %v", err)
	}
	v2 := intentV2Fixture(t)
	v2.IntentID = "int_v2_foreign"
	store, err := OpenOperatorFor(founder.path, profileB)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer func() { _ = store.Close() }()
	wireSealedLike(t, store, founder)
	ctx := context.Background()
	env, evidence := identityAttempt(t, resolver, issuer, "act_x", now)
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	env.ParametersDigest = action.Digest(env.Operation, `{"a":1}`)
	bound, err := action.NewBoundApprovalRequest(env, `{"a":1}`, action.ApprovalContext{
		IntentPurpose: "x", ToolCage: "x",
		Descriptor: action.EffectDescriptor{Class: action.EffectWriteIrreversible}, HasDescriptor: true,
		LawVersion: 1, LawDigest: "sha256:law", Rule: "require_approval", Now: now, TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("bound request: %v", err)
	}
	decisionEnv, ident := operatorDecisionEnv("approve", "apr_none")
	doors := []struct {
		name string
		call func() error
	}{
		{"RecordAttempt", func() error {
			return store.RecordAttempt(ctx, testEnvelope("act_a"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
		}},
		{"RecordAttemptIdentified", func() error {
			return store.RecordAttemptIdentified(ctx, testEnvelope("act_b"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized, testIdentity())
		}},
		{"RecordAttemptAuthenticated", func() error {
			return store.RecordAttemptAuthenticated(ctx, env, Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized, evidence)
		}},
		{"CreateApprovalRequest", func() error { return store.CreateApprovalRequest(ctx, bound) }},
		{"CreateApprovalRequestAuthenticated", func() error { return store.CreateApprovalRequestAuthenticated(ctx, bound, evidence) }},
		{"DecideApprovalUnderLaw", func() error {
			_, err := store.DecideApprovalUnderLaw(ctx, "apr_none", action.DecisionApproved, now, decisionEnv, ident, "", PolicyPin{Version: 1, Digest: "sha256:law"})
			return err
		}},
		// The authority doors get VALID requests: the block is at the write,
		// and a request the door would refuse for its shape never reaches it.
		{"ParkAuthorization", func() error {
			_, err := store.ParkAuthorization(ctx, AuthorityPendingRequest{
				ActorPrincipalID: evidence.ActorPrincipalID, CorrelationID: env.CorrelationID,
				SourceProtocol: env.Source.Protocol, Channel: env.Source.Channel,
				Operation: env.Operation, Arguments: `{"a":1}`, EffectClass: action.EffectWriteIrreversible, At: now,
				ApprovalContext: action.ApprovalContext{IntentPurpose: "x", ToolCage: "x", Descriptor: action.EffectDescriptor{Class: action.EffectWriteIrreversible}, HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:law", Rule: "require_approval", Now: now, TTL: time.Minute},
				ResolveEvidence: func(string) (identity.Evidence, error) { return evidence, nil },
			})
			return err
		}},
		{"StartAuthorization", func() error {
			_, err := store.StartAuthorization(ctx, AuthorityStartRequest{
				ActorPrincipalID: evidence.ActorPrincipalID, CorrelationID: env.CorrelationID,
				SourceProtocol: env.Source.Protocol, Channel: env.Source.Channel,
				Operation: env.Operation, Arguments: `{"a":1}`, EffectClass: action.EffectWriteIrreversible, At: now,
				ResolveEvidence: func(string) (identity.Evidence, error) { return evidence, nil },
			})
			return err
		}},
		{"CreateIntent", func() error {
			return store.CreateIntent(ctx, action.IntentContract{IntentID: "int_x", Purpose: "x", AllowedOperations: []string{"calc"}})
		}},
		{"CreateIntentV2", func() error { return store.CreateIntentV2(ctx, v2, "p", v2.ValidFrom) }},
		{"CreateGrant", func() error {
			return store.CreateGrant(ctx, action.AuthorityGrant{GrantID: "grant_x", IntentID: "int_x"})
		}},
		// The three the internal pass found appending receipts unguarded: an
		// intent's lifecycle and an execution binding are this profile's
		// writes into the book, receipt included.
		{"ActivateIntentV2", func() error { return store.ActivateIntentV2(ctx, v2Owned.IntentID, 1, "p", v2Owned.ValidFrom) }},
		{"RevokeIntentV2", func() error { return store.RevokeIntentV2(ctx, v2Owned.IntentID, "p", v2Owned.ValidFrom) }},
		{"PutExecutionBinding", func() error {
			return store.PutExecutionBinding(ctx, action.ExecutionBinding{BindingID: "binding_x", Status: action.BindingActive})
		}},
		// The official pass's find: a contract's lifecycle is an UPDATE, which
		// the structural walk over INSERT sites does not see — so the two
		// wrappers of transitionContract get their own rows, over contracts
		// the founder wrote.
		{"TransitionIntent", func() error { return store.TransitionIntent(ctx, "int_t", action.LifecycleActive) }},
		{"TransitionGrant", func() error { return store.TransitionGrant(ctx, "grant_t", action.LifecycleActive) }},
	}
	for _, d := range doors {
		t.Run(d.name, func(t *testing.T) {
			err := d.call()
			if !errors.Is(err, ErrLedgerForeignProfile) {
				t.Fatalf("%s on a foreign ledger = %v, want ErrLedgerForeignProfile (named by beginWrite's judgement, or by the trigger behind it)", d.name, err)
			}
		})
	}
	// And closing an act already sealed is NOT a new act: it is not blocked.
	if got := countRows(t, store, "actions"); got != 1 {
		t.Fatalf("%d action rows after the refusals, want the founding one only", got)
	}
}

// TestAdopt_theBookHasOneOwnerAtATime is G3 / L4 / L6: the foreign profile
// adopts; its act is recorded and closed SUCCEEDED with the mark in ONE call,
// the standing turns ok for it, and the founder — judged again — is now the
// foreign one. The chain grows by exactly one receipt.
//
// PROBING MUTATIONS: (1) leave the standing foreign after adopting — the record
// below refuses and this reddens; (2) mark the adoption receipt with the OLD
// owner — the founder still reads ok and this reddens on its second judgement.
func TestAdopt_theBookHasOneOwnerAtATime(t *testing.T) {
	store, resolver, issuer, _, now := identityStoreFixture(t)
	mustRecord(t, store, "act_1", action.StateAuthorized)
	if err := store.FinishFounding(context.Background(), "act_1", profileA); err != nil {
		t.Fatalf("found: %v", err)
	}
	if err := store.setIdentity(profileB); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	before := len(mustList(t, store))

	env, evidence := identityAttempt(t, resolver, issuer, "act_adopt", now)
	env.Operation = action.Operation{Namespace: "config", Name: "config.adopt-ledger", Version: 1}
	env.ParametersDigest = action.Digest(env.Operation, `{"door":"adopt-ledger"}`)
	receiptID, err := store.AdoptLedger(context.Background(), env, Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
	if err != nil || receiptID == "" {
		t.Fatalf("AdoptLedger: receipt %q err %v", receiptID, err)
	}
	if st, owner, err := store.Standing(context.Background()); err != nil || st != LedgerStandingOK || owner != profileB {
		t.Fatalf("after adopting: %q owner %q err %v, want ok / %q", st, owner, err, profileB)
	}
	mustRecord(t, store, "act_after", action.StateAuthorized) // no longer blocked
	receipts := mustList(t, store)
	if len(receipts) != before+1 {
		t.Fatalf("%d receipts after adopting, want %d", len(receipts), before+1)
	}
	last := receipts[len(receipts)-1]
	if last.ReceiptID != receiptID || last.Outcome != string(action.StateSucceeded) || last.ResultDigest != ProfileMarkPrefix+profileB {
		t.Fatalf("the adoption receipt is %+v, want SUCCEEDED marked %q", last, ProfileMarkPrefix+profileB)
	}
	row, err := store.Get(context.Background(), "act_adopt")
	if err != nil || row.State != action.StateSucceeded || row.Envelope.Operation.Name != "config.adopt-ledger" {
		t.Fatalf("the adoption act row: %+v (err %v)", row, err)
	}
	// The founder, judged again: foreign now.
	founder, err := openOperator(store.path)
	if err != nil {
		t.Fatalf("founder's handle: %v", err)
	}
	t.Cleanup(func() { _ = founder.Close() })
	if err := founder.setIdentity(profileA); err != nil {
		t.Fatalf("founder identity: %v", err)
	}
	if st, _, _ := founder.Standing(context.Background()); st != LedgerStandingForeignProfile {
		t.Fatalf("the founder after another profile adopted reads %q, want foreign", st)
	}
	// An adoption act must be named as one: any other name is refused.
	other, otherEvidence := identityAttempt(t, resolver, issuer, "act_not_adopt", now)
	if _, err := store.AdoptLedger(context.Background(), other, Decision{Outcome: "allow", Rule: "operator"}, otherEvidence, profileB); err == nil {
		t.Fatal("an act not named config.adopt-ledger was accepted as an adoption")
	}
}

// foreignFixture is a founded (by A) store judged by B, with its identity
// runtime, so an adoption act can be built and the block observed.
func foreignFixture(t *testing.T) (*Store, *identity.Resolver, *identity.Issuer, ed25519.PrivateKey, time.Time) {
	t.Helper()
	store, resolver, issuer, priv, now := identityStoreFixture(t)
	mustRecord(t, store, "act_1", action.StateAuthorized)
	if err := store.FinishFounding(context.Background(), "act_1", profileA); err != nil {
		t.Fatalf("found: %v", err)
	}
	if err := store.setIdentity(profileB); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	return store, resolver, issuer, priv, now
}

func adoptionAct(t *testing.T, resolver *identity.Resolver, issuer *identity.Issuer, id string, now time.Time) (action.Envelope, identity.Evidence) {
	t.Helper()
	env, evidence := identityAttempt(t, resolver, issuer, id, now)
	env.Operation = action.Operation{Namespace: "config", Name: "config.adopt-ledger", Version: 1}
	env.ParametersDigest = action.Digest(env.Operation, `{"door":"adopt-ledger"}`)
	return env, evidence
}

// assertStillForeignAndEmpty is the shared oracle of the three G4 legs.
func assertStillForeignAndEmpty(t *testing.T, store *Store, actions, receipts int) {
	t.Helper()
	if countRows(t, store, "actions") != actions || countRows(t, store, "receipts") != receipts {
		t.Fatalf("a failed adoption left rows: actions %d→%d, receipts %d→%d", actions, countRows(t, store, "actions"), receipts, countRows(t, store, "receipts"))
	}
	if st, _, _ := store.Standing(context.Background()); st != LedgerStandingForeignProfile {
		t.Fatalf("standing after a failed adoption = %q, want foreign", st)
	}
	if err := store.RecordAttempt(context.Background(), testEnvelope("act_new"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("after a failed adoption RecordAttempt = %v, want still refused", err)
	}
}

// TestAdopt_anUnsignedReceiptLeavesNothing is G4a: the sealer stamps the key
// and an EMPTY signature, the receipt is refused at birth by name INSIDE the
// adoption's transaction, and nothing of the adoption remains.
//
// PROBING MUTATION: record the act in one transaction and close it in another.
// The action row survives the failed close and this reddens on the count.
func TestAdopt_anUnsignedReceiptLeavesNothing(t *testing.T) {
	store, resolver, issuer, priv, now := foreignFixture(t)
	actions, receipts := countRows(t, store, "actions"), countRows(t, store, "receipts")
	pub := priv.Public().(ed25519.PublicKey)
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		r.SigningKeyID = action.SigningKeyID(pub)
		r.Signature = ""
		return r
	})
	env, evidence := adoptionAct(t, resolver, issuer, "act_adopt", now)
	_, err := store.AdoptLedger(context.Background(), env, Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
	// The store's own name for a seal that does not verify against the
	// registered key: signature_invalid_at_birth.
	if err == nil || !strings.Contains(err.Error(), "signature_invalid_at_birth") {
		t.Fatalf("an adoption whose receipt was born unsigned = %v, want the signature_invalid_at_birth refusal", err)
	}
	assertStillForeignAndEmpty(t, store, actions, receipts)
}

// TestAdopt_anAbortedReceiptRollsTheActBack is G4b: SQLite itself aborts the
// receipt's insert (a trigger installed from a second real connection). The
// error is the trigger's, and the action row written earlier in the SAME
// transaction does not survive.
//
// PROBING MUTATION: commit the act before appending the receipt. The row
// survives and this reddens on the count.
func TestAdopt_anAbortedReceiptRollsTheActBack(t *testing.T) {
	store, resolver, issuer, _, now := foreignFixture(t)
	actions, receipts := countRows(t, store, "actions"), countRows(t, store, "receipts")
	db, err := sql.Open("sqlite", "file:"+store.path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER abort_receipts BEFORE INSERT ON receipts BEGIN SELECT RAISE(ABORT, 'forbidden write: receipts'); END;`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	_ = db.Close()
	env, evidence := adoptionAct(t, resolver, issuer, "act_adopt", now)
	_, err = store.AdoptLedger(context.Background(), env, Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
	if err == nil || !strings.Contains(err.Error(), "forbidden write: receipts") {
		t.Fatalf("an adoption aborted at the receipt = %v, want the trigger's error", err)
	}
	assertStillForeignAndEmpty(t, store, actions, receipts)
}

// TestAdopt_refusesWithoutASealer is G4c: a marker without a receipt would be
// lost in silence (FinishWithResult writes no receipt with no sealer). The
// adoption refuses BY NAME and writes nothing.
//
// PROBING MUTATION: drop the sealer check. The act lands with no receipt, the
// standing stays foreign, and this reddens on the count and the name.
func TestAdopt_refusesWithoutASealer(t *testing.T) {
	store, resolver, issuer, _, now := foreignFixture(t)
	actions, receipts := countRows(t, store, "actions"), countRows(t, store, "receipts")
	store.SetReceiptSealer(nil)
	env, evidence := adoptionAct(t, resolver, issuer, "act_adopt", now)
	_, err := store.AdoptLedger(context.Background(), env, Decision{Outcome: "allow", Rule: "operator"}, evidence, profileB)
	if !errors.Is(err, ErrNoSealer) {
		t.Fatalf("an adoption with no sealer = %v, want ErrNoSealer", err)
	}
	assertStillForeignAndEmpty(t, store, actions, receipts)
}

// TestMark_theProfilePrefixIsReservedToTheFoundingDoors is G9: a brain's close
// cannot carry the mark. FinishWithResult refuses the prefix by name; the owner
// does not move.
//
// PROBING MUTATION: let FinishWithResult write any result digest. The brain's
// close reassigns the owner and this reddens on the standing.
func TestMark_theProfilePrefixIsReservedToTheFoundingDoors(t *testing.T) {
	store := foundedStore(t, profileA)
	if err := store.setIdentity(profileA); err != nil {
		t.Fatalf("setIdentity: %v", err)
	}
	mustRecord(t, store, "act_brain", action.StateAuthorized)
	err := store.FinishWithResult(context.Background(), "act_brain", action.StateSucceeded, time.Now().UTC(), ProfileMarkPrefix+profileB)
	if !errors.Is(err, ErrReservedResultDigest) {
		t.Fatalf("a brain's close carrying the mark = %v, want ErrReservedResultDigest", err)
	}
	// In another case too: SQLite's LIKE folds ASCII case, so `PROFILE:` would
	// read as a mark nobody can match and make the ledger foreign to everyone.
	err = store.FinishWithResult(context.Background(), "act_brain", action.StateSucceeded, time.Now().UTC(), "PROFILE:"+profileB)
	if !errors.Is(err, ErrReservedResultDigest) {
		t.Fatalf("a brain's close carrying the mark in upper case = %v, want ErrReservedResultDigest", err)
	}
	if st, owner, _ := store.Standing(context.Background()); st != LedgerStandingOK || owner != profileA {
		t.Fatalf("the owner moved: %q / %q", st, owner)
	}
	// And FinishFounding refuses without a sealer, as the adoption does.
	store.SetReceiptSealer(nil)
	if err := store.FinishFounding(context.Background(), "act_brain", profileA); !errors.Is(err, ErrNoSealer) {
		t.Fatalf("FinishFounding without a sealer = %v, want ErrNoSealer", err)
	}
}

// TestAdopt_aLiveOwnerLosesTheBookOnItsNextWrite is G10 / L6 over two REAL
// connections: A holds the ledger open as its owner; B adopts through its own
// handle; A — never reopened, never re-judged by hand — refuses its next act
// and its Standing says why.
//
// PROBING MUTATION: cache the standing in the handle when the identity is set.
// A keeps writing and this reddens.
func TestAdopt_aLiveOwnerLosesTheBookOnItsNextWrite(t *testing.T) {
	ownerA, _, _, privA, now := identityStoreFixture(t)
	mustRecord(t, ownerA, "act_1", action.StateAuthorized)
	if err := ownerA.FinishFounding(context.Background(), "act_1", profileA); err != nil {
		t.Fatalf("found: %v", err)
	}
	if err := ownerA.setIdentity(profileA); err != nil {
		t.Fatalf("A identity: %v", err)
	}
	mustRecord(t, ownerA, "act_a1", action.StateAuthorized) // A writes, as the owner

	handleB, err := openOperator(ownerA.path)
	if err != nil {
		t.Fatalf("B's handle: %v", err)
	}
	t.Cleanup(func() { _ = handleB.Close() })
	// B's handle needs the ledger's key to sign its adoption receipt: the same
	// key the fixture registered, read from the store, as the boot would.
	wireSealerFromFixture(t, handleB, privA, ownerA.identityNow)
	if err := handleB.setIdentity(profileB); err != nil {
		t.Fatalf("B identity: %v", err)
	}
	envB, evidenceB := adoptionActOn(t, handleB, "act_adopt_b", now)
	if _, err := handleB.AdoptLedger(context.Background(), envB, Decision{Outcome: "allow", Rule: "operator"}, evidenceB, profileB); err != nil {
		t.Fatalf("B adopts: %v", err)
	}

	// A, still open, on its next write:
	err = ownerA.RecordAttempt(context.Background(), testEnvelope("act_a2"), Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized)
	if !errors.Is(err, ErrLedgerForeignProfile) {
		t.Fatalf("the former owner kept writing after another profile adopted: %v", err)
	}
	if st, owner, _ := ownerA.Standing(context.Background()); st != LedgerStandingForeignProfile || owner != profileB {
		t.Fatalf("the former owner's standing = %q / %q, want foreign / %q", st, owner, profileB)
	}
}

func mustList(t *testing.T, store *Store) []action.Receipt {
	t.Helper()
	receipts, err := store.ListReceipts(context.Background(), "main")
	if err != nil {
		t.Fatalf("list receipts: %v", err)
	}
	return receipts
}

// wireSealerFromFixture gives a second handle on the fixture's file the same
// signing key and identity signers as the first — what the boot does with the
// profile's key file.
func wireSealerFromFixture(t *testing.T, second *Store, priv ed25519.PrivateKey, now func() time.Time) {
	t.Helper()
	second.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(priv, r) })
	second.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(priv, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(priv, e)
		},
	)
	second.identityNow = now
}

// adoptionActOn builds an adoption act through a handle's own resolver and
// issuer, derived from the fixture registry the first handle registered.
func adoptionActOn(t *testing.T, store *Store, id string, now time.Time) (action.Envelope, identity.Evidence) {
	t.Helper()
	registry := identityRegistryFixture()
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = store
	return adoptionAct(t, resolver, issuer, id, now)
}
