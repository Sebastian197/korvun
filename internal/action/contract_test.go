// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Intent/Authority foundations — Etapa 2, lote 1, pieza 2 (spec FR-INT-1,
// FR-AUTH-1 structs + the shared DRAFT→ACTIVE→EXPIRED/REVOKED lifecycle
// on the Etapa-1 fail-closed sentinel mold). No persistence (lote 3), no
// attenuation validator (lote 2). Approved-red contract.

package action

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var lifecycleStates = []LifecycleStatus{
	LifecycleDraft, LifecycleActive, LifecycleExpired, LifecycleRevoked,
}

var validLifecycle = map[LifecycleStatus][]LifecycleStatus{
	LifecycleDraft:  {LifecycleActive},
	LifecycleActive: {LifecycleExpired, LifecycleRevoked},
}

func lifecycleAllowed(from, to LifecycleStatus) bool {
	for _, next := range validLifecycle[from] {
		if next == to {
			return true
		}
	}
	return false
}

func TestLifecycleTransition_exactTableFailClosed(t *testing.T) {
	t.Parallel()
	for from, nexts := range validLifecycle {
		for _, to := range nexts {
			if err := LifecycleTransition(from, to); err != nil {
				t.Fatalf("valid transition %s -> %s rejected: %v", from, to, err)
			}
		}
	}
	all := append(append([]LifecycleStatus(nil), lifecycleStates...), LifecycleStatus("BOGUS"))
	for _, from := range all {
		for _, to := range all {
			if lifecycleAllowed(from, to) {
				continue
			}
			if err := LifecycleTransition(from, to); !errors.Is(err, ErrInvalidLifecycleTransition) {
				t.Fatalf("invalid transition %s -> %s must carry the sentinel, got %v", from, to, err)
			}
		}
	}
}

func TestLifecycleTerminal_truthTable(t *testing.T) {
	t.Parallel()
	want := map[LifecycleStatus]bool{
		LifecycleDraft: false, LifecycleActive: false,
		LifecycleExpired: true, LifecycleRevoked: true,
		LifecycleStatus("BOGUS"): false,
	}
	for s, expected := range want {
		if got := s.Terminal(); got != expected {
			t.Fatalf("Terminal(%s) = %v, want %v", s, got, expected)
		}
	}
}

func testIntent() IntentContract {
	return IntentContract{
		IntentID:          "int_root",
		SchemaVersion:     1,
		OwnerPrincipalID:  OperatorPrincipal().PrincipalID,
		Purpose:           "operate this Korvun instance",
		AllowedOperations: []string{"read_file", "calc", "time"},
		AllowedResources:  []string{"*"},
		Budgets:           Budgets{},
		ValidFrom:         time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		Status:            LifecycleActive,
		Version:           1,
	}
}

func TestIntentDigest_deterministicAndOrderInsensitive(t *testing.T) {
	t.Parallel()
	a := testIntent()
	b := testIntent()
	b.AllowedOperations = []string{"time", "read_file", "calc"} // permuted set
	if a.Digest() != b.Digest() {
		t.Fatal("operations are a SET: permutation must not change the contract digest")
	}
	if !strings.HasPrefix(a.Digest(), "sha256:") {
		t.Fatalf("contract digest carries the pinned algorithm, got %q", a.Digest())
	}
	first := a.Digest()
	second := a.Digest()
	if first != second {
		t.Fatal("digest must be deterministic")
	}
}

func TestIntentDigest_coversTermsNotRuntimeStatus(t *testing.T) {
	t.Parallel()
	base := testIntent()
	for name, mutate := range map[string]func(*IntentContract){
		"purpose":    func(c *IntentContract) { c.Purpose = "other" },
		"operations": func(c *IntentContract) { c.AllowedOperations = []string{"calc"} },
		"budget":     func(c *IntentContract) { c.Budgets.MaxActions = 5 },
		"window":     func(c *IntentContract) { c.ExpiresAt = base.ValidFrom.Add(time.Hour) },
		"owner":      func(c *IntentContract) { c.OwnerPrincipalID = "principal_ch_x" },
		"version":    func(c *IntentContract) { c.Version = 2 },
	} {
		mutated := testIntent()
		mutate(&mutated)
		if mutated.Digest() == base.Digest() {
			t.Fatalf("changing %s must change the contract digest", name)
		}
	}
	// Runtime status is NOT a contract term: revoking does not rewrite
	// the contract's identity, it closes its lifecycle.
	statusOnly := testIntent()
	statusOnly.Status = LifecycleRevoked
	if statusOnly.Digest() != base.Digest() {
		t.Fatal("status must not participate in the contract digest")
	}
}

func TestGrantDigest_everyDimensionMatters(t *testing.T) {
	t.Parallel()
	base := AuthorityGrant{
		GrantID: "grant_1", IntentID: "int_root",
		IssuerPrincipalID:        OperatorPrincipal().PrincipalID,
		SubjectPrincipalID:       BrainPrincipal("asistente").PrincipalID,
		Operations:               []string{"calc", "time"},
		ResourceScope:            []string{"*"},
		ValidFrom:                time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		DelegationDepthRemaining: 2,
		Status:                   LifecycleActive,
	}
	if !strings.HasPrefix(base.Digest(), "sha256:") {
		t.Fatalf("grant digest form, got %q", base.Digest())
	}
	permuted := base
	permuted.Operations = []string{"time", "calc"}
	if permuted.Digest() != base.Digest() {
		t.Fatal("grant operations are a SET: permutation must not change the digest")
	}
	for name, mutate := range map[string]func(*AuthorityGrant){
		"subject": func(g *AuthorityGrant) { g.SubjectPrincipalID = "principal_ch_hooks" },
		"parent":  func(g *AuthorityGrant) { g.ParentGrantID = "grant_0" },
		"ops":     func(g *AuthorityGrant) { g.Operations = []string{"calc"} },
		"depth":   func(g *AuthorityGrant) { g.DelegationDepthRemaining = 1 },
		"intent":  func(g *AuthorityGrant) { g.IntentID = "int_other" },
	} {
		mutated := base
		mutate(&mutated)
		if mutated.Digest() == base.Digest() {
			t.Fatalf("changing %s must change the grant digest", name)
		}
	}
}
