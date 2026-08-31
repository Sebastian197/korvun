// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The Approval domain — Etapa 5, lote 1, pieza 1 (spec FR-APR-1/2,
// sealed NC-2/NC-3): the §10.8 Approval as a pure type with a
// deterministic canonical form and digest; THE INVALIDATION LAW §15.3
// enforced structurally (the E1 action digest is the anchor — any
// change of operation, resource, protected parameter, amount,
// recipient or effect class IS a different digest) plus the policy-pin
// comparison; expiry judged at the injected instant on the E2 window
// mold (no sweeper); one finite fail-closed status set. Approved-red
// contract.

package action

import (
	"strings"
	"testing"
	"time"
)

func testApproval() Approval {
	return Approval{
		ApprovalID:    "apr_0123456789abcdef0123456789abcdef",
		SchemaVersion: 1,
		ActionID:      "act_a1",
		ActionDigest:  "sha256:aaaa",
		PreviewDigest: "sha256:pppp",
		RequestedFrom: OperatorPrincipal().PrincipalID,
		Reason:        "require_approval",
		RiskSummary:   "write_irreversible — no documented undo",
		PolicyVersion: 7,
		PolicyDigest:  "sha256:law",
		RequestedAt:   time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC),
		ExpiresAt:     time.Date(2026, 8, 31, 11, 0, 0, 0, time.UTC),
		Status:        ApprovalPending,
	}
}

func TestNewApprovalID_mold(t *testing.T) {
	t.Parallel()
	id := NewApprovalID()
	if !strings.HasPrefix(id, "apr_") || len(id) != len("apr_")+32 {
		t.Fatalf("the id mold is apr_ + 16 random bytes hex, got %q", id)
	}
	if id == NewApprovalID() {
		t.Fatal("ids must not repeat")
	}
}

func TestApprovalDigest_deterministicAndFieldSensitive(t *testing.T) {
	t.Parallel()
	a := testApproval()
	a.Status = ApprovalApproved
	a.DecisionPrincipalID = OperatorPrincipal().PrincipalID
	a.Decision = "approved"
	a.DecisionAt = time.Date(2026, 8, 31, 10, 30, 0, 0, time.UTC)
	d1 := a.Digest()
	d2 := a.Digest()
	if d1 != d2 || !strings.HasPrefix(d1, "sha256:") {
		t.Fatalf("deterministic pinned-form digest: %q vs %q", d1, d2)
	}
	// The decision digest covers the CONSUMED decision terms (spec
	// FR-RCPT-1): id, action digest, decider, decision, decision_at.
	// Every one of them must move the digest.
	mutations := []func(*Approval){
		func(x *Approval) { x.ApprovalID = "apr_ffffffffffffffffffffffffffffffff" },
		func(x *Approval) { x.ActionDigest = "sha256:bbbb" },
		func(x *Approval) { x.DecisionPrincipalID = "principal_other" },
		func(x *Approval) { x.Decision = "rejected" },
		func(x *Approval) { x.DecisionAt = x.DecisionAt.Add(time.Second) },
	}
	for i, mutate := range mutations {
		x := a
		mutate(&x)
		if x.Digest() == d1 {
			t.Fatalf("mutation %d must change the approval digest", i)
		}
	}
	// Comment is NOT a decision term: it must not move the digest.
	x := a
	x.Comment = "different words, same decision"
	if x.Digest() != d1 {
		t.Fatal("the comment is not part of the sealed decision terms")
	}
}

func TestApprovalCanonical_roundTripsStrict(t *testing.T) {
	t.Parallel()
	a := testApproval()
	a.Comment = "váyase con cuidado — utf8 ok"
	raw := CanonicalApproval(a)
	parsed, err := ParseCanonicalApproval(raw)
	if err != nil {
		t.Fatalf("canonical form must parse: %v", err)
	}
	if parsed != a {
		t.Fatalf("round trip diverged:\n%+v\n%+v", parsed, a)
	}
	// Strictness: unknown fields and trailing bytes are refused.
	if _, err := ParseCanonicalApproval([]byte(`{"approval_id":"apr_x","bogus":1}`)); err == nil {
		t.Fatal("unknown fields must be refused")
	}
	if _, err := ParseCanonicalApproval(append(raw, []byte("{}")...)); err == nil {
		t.Fatal("trailing bytes must be refused")
	}
}

func TestValidateApprovalBinding_theInvalidationLaw(t *testing.T) {
	t.Parallel()
	a := testApproval()
	if rule, dim := ValidateApprovalBinding(a, "sha256:aaaa", 7, "sha256:law"); rule != "" {
		t.Fatalf("a matching binding must pass, got %q/%q", rule, dim)
	}
	// §15.3 structural half: ANY change to what was asked is a
	// different E1 digest — operation, resource, protected params,
	// amount, recipient, effect class all live under it.
	if rule, dim := ValidateApprovalBinding(a, "sha256:CHANGED", 7, "sha256:law"); rule != RuleApprovalInvalidated || dim != "digest" {
		t.Fatalf("a changed action digest invalidates naming digest, got %q/%q", rule, dim)
	}
	// The policy half: a different law between request and decision.
	if rule, dim := ValidateApprovalBinding(a, "sha256:aaaa", 8, "sha256:law"); rule != RuleApprovalInvalidated || dim != "policy" {
		t.Fatalf("a changed policy version invalidates naming policy, got %q/%q", rule, dim)
	}
	if rule, dim := ValidateApprovalBinding(a, "sha256:aaaa", 7, "sha256:otherlaw"); rule != RuleApprovalInvalidated || dim != "policy" {
		t.Fatalf("a changed policy digest invalidates naming policy, got %q/%q", rule, dim)
	}
}

func TestValidateApprovalBinding_isAnchoredToTheRealEnvelopeDigest(t *testing.T) {
	t.Parallel()
	// The living proof of the structural law: two envelopes differing in
	// ONE protected parameter produce different digests, so an approval
	// bound to the first can never consume against the second.
	env1 := NewEnvelope("act_1", "env-1",
		Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		`{"url":"https://a.example","amount":100}`,
		time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC))
	env2 := NewEnvelope("act_1", "env-1",
		Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		`{"url":"https://a.example","amount":999}`,
		time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC))
	if env1.ParametersDigest == env2.ParametersDigest {
		t.Fatal("the E1 anchor is broken: a changed amount must change the digest")
	}
	a := testApproval()
	a.ActionDigest = env1.ParametersDigest
	if rule, _ := ValidateApprovalBinding(a, env2.ParametersDigest, 7, "sha256:law"); rule != RuleApprovalInvalidated {
		t.Fatal("an approval of env1 must never bind env2")
	}
}

func TestApprovalConsumableAt_expiryAndOneShot(t *testing.T) {
	t.Parallel()
	a := testApproval()
	before := time.Date(2026, 8, 31, 10, 59, 59, 0, time.UTC)
	atExpiry := time.Date(2026, 8, 31, 11, 0, 0, 0, time.UTC)
	if rule := ApprovalConsumableAt(a, before); rule != "" {
		t.Fatalf("a pending approval inside its window consumes, got %q", rule)
	}
	// Half-open window, the E2 mold: the expiry instant itself is out.
	if rule := ApprovalConsumableAt(a, atExpiry); rule != RuleApprovalExpired {
		t.Fatalf("the expiry instant is outside the window, got %q", rule)
	}
	// Zero expiry = no expiry (the creator sets the TTL; the domain
	// keeps the E2 zero-means-none semantics).
	z := a
	z.ExpiresAt = time.Time{}
	if rule := ApprovalConsumableAt(z, atExpiry.Add(24*time.Hour)); rule != "" {
		t.Fatalf("zero expiry means no expiry, got %q", rule)
	}
	// Fail-closed status: anything but PENDING is already decided —
	// including unknown garbage.
	for _, st := range []ApprovalStatus{ApprovalApproved, ApprovalRejected, ApprovalExpired, ApprovalCancelled, ApprovalStatus("GARBAGE"), ApprovalStatus("")} {
		d := a
		d.Status = st
		if rule := ApprovalConsumableAt(d, before); rule != RuleApprovalAlreadyDecided {
			t.Fatalf("status %q must be already-decided (fail closed), got %q", st, rule)
		}
	}
}

func FuzzApprovalCanonical(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add(CanonicalApproval(testApproval()))
	f.Fuzz(func(t *testing.T, raw []byte) {
		a, err := ParseCanonicalApproval(raw)
		if err != nil {
			return
		}
		// Anything that parses must survive its own canonical round trip.
		again, err := ParseCanonicalApproval(CanonicalApproval(a))
		if err != nil {
			t.Fatalf("re-parse of canonical form failed: %v", err)
		}
		if again != a {
			t.Fatal("canonical round trip diverged")
		}
	})
}
