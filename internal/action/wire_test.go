// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Wiring domain contract — Etapa 2, lote 4, pieza 1 (spec FR-ENV-1/2 +
// FR-MIG-1 + decisión sellada 1): the envelope wakes its reserved
// identity fields WITHOUT touching the receipt digest; the root intent
// is deterministic; config grants derive with deterministic
// `grant_cfg_<digest>` ids. Approved-red contract.

package action

import (
	"strings"
	"testing"
	"time"
)

// TestEnvelopeIdentity_digestUntouched is FR-ENV-2: identity fields are
// row data, NEVER digest inputs — every Etapa-1 receipt stays valid and
// comparable.
func TestEnvelopeIdentity_digestUntouched(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	bare := NewEnvelope("act_1", "corr-1",
		Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		Operation{Namespace: "tool", Name: "calc", Version: 1},
		`{"a":1}`, at)
	identified := NewEnvelope("act_1", "corr-1",
		Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		Operation{Namespace: "tool", Name: "calc", Version: 1},
		`{"a":1}`, at)
	identified.Principal = PrincipalRef{
		PrincipalID:        "principal_brain_a",
		EvidenceID:         "evd_0123",
		ResponsibleHumanID: "principal_operator",
	}
	identified.IntentID = RootIntentID
	identified.AuthorityRefs = []string{"grant_cfg_abc"}
	if bare.ParametersDigest != identified.ParametersDigest {
		t.Fatal("FR-ENV-2: identity fields must NEVER change the receipt digest")
	}
	if identified.ParametersDigest != Digest(identified.Operation, `{"a":1}`) {
		t.Fatal("the digest algorithm and inputs are untouched — every Etapa-1 receipt stays comparable")
	}
}

func TestRootIntent_deterministicStandingAuthority(t *testing.T) {
	t.Parallel()
	root := RootIntent()
	if root.IntentID != RootIntentID {
		t.Fatalf("root intent id = %q", root.IntentID)
	}
	if root.Status != LifecycleActive {
		t.Fatalf("the root intent is standing authority: status = %s", root.Status)
	}
	if root.OwnerPrincipalID != OperatorPrincipal().PrincipalID {
		t.Fatalf("the root belongs to the operator, got %q", root.OwnerPrincipalID)
	}
	if len(root.AllowedOperations) != 1 || root.AllowedOperations[0] != "*" {
		t.Fatalf("root operations = %v", root.AllowedOperations)
	}
	if root.Budgets.MaxActions != 0 {
		t.Fatal("FR-BUD-2: the root carries NO budget")
	}
	if !root.ExpiresAt.IsZero() || !root.ValidFrom.IsZero() {
		t.Fatal("the root is timeless: deterministic digest across installs")
	}
	if RootIntent().Digest() != root.Digest() {
		t.Fatal("the root digest must be deterministic")
	}
}

func TestDeriveConfigGrant_deterministicIdsAndShape(t *testing.T) {
	t.Parallel()
	g := DeriveConfigGrant("asistente", []string{"calc", "time"}, []string{"*"})
	if !strings.HasPrefix(g.GrantID, "grant_cfg_") {
		t.Fatalf("derived ids carry the grant_cfg_ prefix, got %q", g.GrantID)
	}
	if g.IntentID != RootIntentID {
		t.Fatalf("derived grants live under the root intent, got %q", g.IntentID)
	}
	if g.IssuerPrincipalID != OperatorPrincipal().PrincipalID {
		t.Fatalf("the operator issues config authority, got %q", g.IssuerPrincipalID)
	}
	if g.SubjectPrincipalID != BrainPrincipal("asistente").PrincipalID {
		t.Fatalf("the subject is the brain principal, got %q", g.SubjectPrincipalID)
	}
	if g.Status != LifecycleActive {
		t.Fatalf("a derived grant is in force, got %s", g.Status)
	}
	if g.DelegationDepthRemaining != 0 {
		t.Fatal("config grants do not delegate in E2: depth must be 0")
	}
	// Deterministic across boots (AS-7) and order-insensitive (sets).
	same := DeriveConfigGrant("asistente", []string{"time", "calc"}, []string{"*"})
	if same.GrantID != g.GrantID {
		t.Fatalf("permuted operations must derive the SAME id: %q vs %q", same.GrantID, g.GrantID)
	}
	// Every term matters.
	for name, other := range map[string]AuthorityGrant{
		"operations": DeriveConfigGrant("asistente", []string{"calc"}, []string{"*"}),
		"brain":      DeriveConfigGrant("otro", []string{"calc", "time"}, []string{"*"}),
		"resources":  DeriveConfigGrant("asistente", []string{"calc", "time"}, []string{"channel:main"}),
	} {
		if other.GrantID == g.GrantID {
			t.Fatalf("changing %s must change the derived id", name)
		}
	}
}
