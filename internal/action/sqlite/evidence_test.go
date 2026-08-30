// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Evidence persistence contract — Etapa 2, lote 3, pieza 2 (spec
// FR-EVID-2): identity evidence lands in the SAME transaction as the
// attempt — they enter together or nothing enters at all. Identified
// rows carry their principal/intent/authority refs; v1-path rows keep
// NULL identity; evidence is pruned WITH its action under the Etapa-1
// cap (the bounded-growth reason made executable). Approved-red contract.

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func testIdentity() AttemptIdentity {
	return AttemptIdentity{
		PrincipalID:   "principal_ch_hooks",
		IntentID:      "int_root",
		AuthorityRefs: []string{"grant_cfg_abc", "grant_child_1"},
		Evidence: action.IdentityEvidence{
			EvidenceID:       "evd_0123456789abcdef",
			Provider:         "webhook",
			Subject:          "user-77",
			Credential:       action.CredentialInboundBearer,
			IssuedAt:         time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
			TransportBinding: "hooks",
			ClaimsDigest:     "sha256:deadbeef",
		},
	}
}

func TestRecordIdentified_landsAtomicallyAndRoundTrips(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	env := testEnvelope("act_id1")
	ident := testIdentity()
	if err := store.RecordAttemptIdentified(context.Background(), env,
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, ident); err != nil {
		t.Fatalf("record identified: %v", err)
	}
	rec, err := store.Get(context.Background(), "act_id1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Identity == nil {
		t.Fatal("an identified row must surface its identity refs")
	}
	if rec.Identity.PrincipalID != ident.PrincipalID || rec.Identity.IntentID != ident.IntentID {
		t.Fatalf("identity refs corrupted: %+v", rec.Identity)
	}
	if len(rec.Identity.AuthorityRefs) != 2 || rec.Identity.AuthorityRefs[0] != "grant_cfg_abc" {
		t.Fatalf("authority refs must round-trip in order: %v", rec.Identity.AuthorityRefs)
	}
	ev, err := store.GetEvidence(context.Background(), "act_id1")
	if err != nil {
		t.Fatalf("get evidence: %v", err)
	}
	if ev != ident.Evidence {
		t.Fatalf("evidence must round-trip verbatim:\ngot  %+v\nwant %+v", ev, ident.Evidence)
	}
}

// TestRecordIdentified_evidenceFailureAbortsEverything is THE FR-EVID-2
// contract: when the evidence insert fails, the attempt and its decision
// must NOT exist either — together or nothing.
func TestRecordIdentified_evidenceFailureAbortsEverything(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_evidence BEFORE INSERT ON evidence
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install evidence blocker: %v", err)
	}
	err := store.RecordAttemptIdentified(context.Background(), testEnvelope("act_id2"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized, testIdentity())
	if err == nil {
		t.Fatal("a failed evidence insert must fail the whole record")
	}
	if _, err := store.Get(context.Background(), "act_id2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nothing may land when evidence cannot: got %v", err)
	}
	if _, err := store.GetEvidence(context.Background(), "act_id2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no orphan evidence either: got %v", err)
	}
}

func TestRecordAttempt_v1PathKeepsNullIdentity(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	if err := store.RecordAttempt(context.Background(), testEnvelope("act_v1p"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	rec, err := store.Get(context.Background(), "act_v1p")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Identity != nil {
		t.Fatalf("the identity-less path must keep NULL identity, got %+v", rec.Identity)
	}
	if _, err := store.GetEvidence(context.Background(), "act_v1p"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no phantom evidence: got %v", err)
	}
}

func TestRecordIdentified_onlyDecisionStatesEnter(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	err := store.RecordAttemptIdentified(context.Background(), testEnvelope("act_id3"),
		Decision{Outcome: "allow", Rule: "granted"}, action.StateReceived, testIdentity())
	if !errors.Is(err, ErrNotADecisionState) {
		t.Fatalf("the decision-state guard also guards the identified path, got %v", err)
	}
}

// TestEvidence_prunedWithItsAction makes the bounded-growth reason
// executable: evidence rows ride ON DELETE CASCADE, so the Etapa-1
// retention cap prunes them with their actions.
func TestEvidence_prunedWithItsAction(t *testing.T) {
	t.Parallel()
	store, err := openWithCap(t.TempDir()+"/korvun.db", 1)
	if err != nil {
		t.Fatalf("open with cap: %v", err)
	}
	defer func() { _ = store.Close() }()
	ident := testIdentity()
	if err := store.RecordAttemptIdentified(context.Background(), testEnvelope("act_1_old"),
		Decision{Outcome: "deny", Rule: "no_grant"}, action.StateDenied, ident); err != nil {
		t.Fatalf("record old: %v", err)
	}
	ident2 := ident
	ident2.Evidence.EvidenceID = "evd_fedcba9876543210"
	if err := store.RecordAttemptIdentified(context.Background(), testEnvelope("act_2_new"),
		Decision{Outcome: "deny", Rule: "no_grant"}, action.StateDenied, ident2); err != nil {
		t.Fatalf("record new: %v", err)
	}
	if _, err := store.Prune(context.Background()); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := store.Get(context.Background(), "act_1_old"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest terminal must be pruned under cap 1: %v", err)
	}
	if _, err := store.GetEvidence(context.Background(), "act_1_old"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("its evidence must cascade with it: %v", err)
	}
	if _, err := store.GetEvidence(context.Background(), "act_2_new"); err != nil {
		t.Fatalf("the surviving action keeps its evidence: %v", err)
	}
}
