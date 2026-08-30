// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The attenuation validator contract — Etapa 2, lote 2, pieza 1 (spec
// FR-AUTH-2, §14.3): a delegation is valid ONLY if the child is a subset
// of its parent in EVERY present dimension. Property tests never accept a
// widening on any dimension; the fuzzer drives arbitrary pairs against a
// naive independent oracle. Approved-red contract.

package action

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"
)

var attBase = time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)

func parentGrant() AuthorityGrant {
	return AuthorityGrant{
		GrantID: "grant_parent", IntentID: "int_root",
		IssuerPrincipalID:        OperatorPrincipal().PrincipalID,
		SubjectPrincipalID:       BrainPrincipal("a").PrincipalID,
		Operations:               []string{"calc", "time", "read_file"},
		ResourceScope:            []string{"res-a", "res-b"},
		Budgets:                  Budgets{MaxActions: 10},
		ValidFrom:                attBase,
		ExpiresAt:                attBase.Add(48 * time.Hour),
		DelegationDepthRemaining: 3,
		Status:                   LifecycleActive,
	}
}

func childOf(p AuthorityGrant) AuthorityGrant {
	return AuthorityGrant{
		GrantID: "grant_child", IntentID: p.IntentID,
		IssuerPrincipalID:        p.SubjectPrincipalID,
		SubjectPrincipalID:       "principal_ch_hooks",
		ParentGrantID:            p.GrantID,
		Operations:               []string{"calc"},
		ResourceScope:            []string{"res-a"},
		Budgets:                  Budgets{MaxActions: 5},
		ValidFrom:                attBase.Add(time.Hour),
		ExpiresAt:                attBase.Add(24 * time.Hour),
		DelegationDepthRemaining: 1,
		Status:                   LifecycleActive,
	}
}

func TestAttenuation_validStrictSubsetPasses(t *testing.T) {
	t.Parallel()
	if err := ValidateAttenuation(parentGrant(), childOf(parentGrant())); err != nil {
		t.Fatalf("a strict subset delegation must be valid, got %v", err)
	}
}

func TestAttenuation_everyWideningDimensionIsRejectedAndNamed(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*AuthorityGrant){
		"operations": func(c *AuthorityGrant) { c.Operations = []string{"calc", "webhook_call"} },
		"resources":  func(c *AuthorityGrant) { c.ResourceScope = []string{"res-a", "res-z"} },
		"budget":     func(c *AuthorityGrant) { c.Budgets.MaxActions = 11 },
		"expiry":     func(c *AuthorityGrant) { c.ExpiresAt = attBase.Add(72 * time.Hour) },
		"window":     func(c *AuthorityGrant) { c.ValidFrom = attBase.Add(-time.Hour) },
		"depth":      func(c *AuthorityGrant) { c.DelegationDepthRemaining = 3 },
		"intent":     func(c *AuthorityGrant) { c.IntentID = "int_other" },
		"parent":     func(c *AuthorityGrant) { c.ParentGrantID = "grant_stranger" },
		"issuer":     func(c *AuthorityGrant) { c.IssuerPrincipalID = "principal_ch_hooks" },
	}
	for dimension, widen := range cases {
		child := childOf(parentGrant())
		widen(&child)
		err := ValidateAttenuation(parentGrant(), child)
		if !errors.Is(err, ErrAttenuationViolated) {
			t.Fatalf("widening %s must be rejected with the sentinel, got %v", dimension, err)
		}
		if !strings.Contains(err.Error(), dimension) {
			t.Fatalf("the rejection must NAME the dimension %s, got %q", dimension, err)
		}
	}
}

func TestAttenuation_unlimitedSemantics(t *testing.T) {
	t.Parallel()
	// A parent with zero budget (unlimited) accepts any child budget.
	parent := parentGrant()
	parent.Budgets = Budgets{}
	child := childOf(parent)
	child.Budgets.MaxActions = 1000
	if err := ValidateAttenuation(parent, child); err != nil {
		t.Fatalf("an unlimited parent accepts any child budget, got %v", err)
	}
	// A LIMITED parent rejects an unlimited (zero) child: zero would widen.
	parent = parentGrant()
	child = childOf(parent)
	child.Budgets = Budgets{}
	if err := ValidateAttenuation(parent, child); !errors.Is(err, ErrAttenuationViolated) {
		t.Fatalf("unlimited child under a limited parent is a widening, got %v", err)
	}
	// Same shape for expiry: no-expiry parent accepts anything; no-expiry
	// child under an expiring parent widens.
	parent = parentGrant()
	parent.ExpiresAt = time.Time{}
	child = childOf(parent)
	child.ExpiresAt = attBase.Add(1000 * time.Hour)
	if err := ValidateAttenuation(parent, child); err != nil {
		t.Fatalf("a never-expiring parent accepts any child expiry, got %v", err)
	}
	parent = parentGrant()
	child = childOf(parent)
	child.ExpiresAt = time.Time{}
	if err := ValidateAttenuation(parent, child); !errors.Is(err, ErrAttenuationViolated) {
		t.Fatalf("a never-expiring child under an expiring parent widens, got %v", err)
	}
}

func TestAttenuation_wildcardOperations(t *testing.T) {
	t.Parallel()
	// A parent with "*" covers any child set; a child "*" requires "*".
	parent := parentGrant()
	parent.Operations = []string{"*"}
	child := childOf(parent)
	child.Operations = []string{"anything", "at", "all"}
	if err := ValidateAttenuation(parent, child); err != nil {
		t.Fatalf("wildcard parent covers any operations, got %v", err)
	}
	parent = parentGrant()
	child = childOf(parent)
	child.Operations = []string{"*"}
	if err := ValidateAttenuation(parent, child); !errors.Is(err, ErrAttenuationViolated) {
		t.Fatalf("a wildcard child under an enumerated parent widens, got %v", err)
	}
}

func TestAttenuation_depthMustStrictlyDecreaseAndAllowDelegation(t *testing.T) {
	t.Parallel()
	parent := parentGrant()
	parent.DelegationDepthRemaining = 0
	child := childOf(parent)
	child.DelegationDepthRemaining = 0
	err := ValidateAttenuation(parent, child)
	if !errors.Is(err, ErrAttenuationViolated) || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("a zero-depth parent cannot delegate at all, got %v", err)
	}
}

// naiveSubsetOracle is the INDEPENDENT re-statement of §14.3 the property
// and fuzz tests judge the validator against.
func naiveSubsetOracle(parent, child AuthorityGrant) bool {
	inSet := func(needle string, set []string) bool {
		for _, s := range set {
			if s == needle || s == "*" {
				return true
			}
		}
		return false
	}
	for _, op := range child.Operations {
		if op == "*" && !inSet("*", parent.Operations) {
			return false
		}
		if op != "*" && !inSet(op, parent.Operations) {
			return false
		}
	}
	for _, r := range child.ResourceScope {
		if r == "*" && !inSet("*", parent.ResourceScope) {
			return false
		}
		if r != "*" && !inSet(r, parent.ResourceScope) {
			return false
		}
	}
	if parent.Budgets.MaxActions != 0 &&
		(child.Budgets.MaxActions == 0 || child.Budgets.MaxActions > parent.Budgets.MaxActions) {
		return false
	}
	if !parent.ExpiresAt.IsZero() &&
		(child.ExpiresAt.IsZero() || child.ExpiresAt.After(parent.ExpiresAt)) {
		return false
	}
	if child.ValidFrom.Before(parent.ValidFrom) {
		return false
	}
	if parent.DelegationDepthRemaining <= 0 ||
		child.DelegationDepthRemaining >= parent.DelegationDepthRemaining {
		return false
	}
	if child.IntentID != parent.IntentID || child.ParentGrantID != parent.GrantID ||
		child.IssuerPrincipalID != parent.SubjectPrincipalID {
		return false
	}
	return true
}

func TestAttenuation_propertyAgainstTheOracle(t *testing.T) {
	t.Parallel()
	// #nosec G404 -- seeded math/rand ON PURPOSE: deterministic property rounds.
	rng := rand.New(rand.NewSource(20260830))
	ops := []string{"calc", "time", "read_file", "http_fetch", "*"}
	for round := 0; round < 400; round++ {
		pick := func(from []string) []string {
			var out []string
			for _, s := range from {
				if rng.Intn(2) == 0 {
					out = append(out, s)
				}
			}
			return out
		}
		parent := parentGrant()
		parent.Operations = pick(ops)
		parent.Budgets.MaxActions = rng.Intn(4) * 5
		parent.DelegationDepthRemaining = rng.Intn(3)
		if rng.Intn(3) == 0 {
			parent.ExpiresAt = time.Time{}
		}
		child := childOf(parent)
		child.Operations = pick(ops)
		child.Budgets.MaxActions = rng.Intn(4) * 5
		child.DelegationDepthRemaining = rng.Intn(3)
		child.ExpiresAt = attBase.Add(time.Duration(rng.Intn(96)) * time.Hour)
		if rng.Intn(4) == 0 {
			child.ExpiresAt = time.Time{}
		}
		got := ValidateAttenuation(parent, child) == nil
		want := naiveSubsetOracle(parent, child)
		if got != want {
			t.Fatalf("round %d: validator=%v oracle=%v\nparent=%+v\nchild=%+v",
				round, got, want, parent, child)
		}
	}
}

// FuzzAttenuation drives arbitrary pairs: never a panic, and NEVER an
// accepted pair the oracle rejects (no widening slips through).
func FuzzAttenuation(f *testing.F) {
	f.Add("calc,time", "calc", 10, 5, 3, 1, int64(24), int64(12))
	f.Add("*", "*", 0, 0, 2, 1, int64(0), int64(0))
	f.Add("calc", "calc,extra", 5, 50, 1, 0, int64(1), int64(100))
	f.Fuzz(func(t *testing.T, parentOps, childOps string, parentBudget, childBudget, parentDepth, childDepth int, parentExpH, childExpH int64) {
		parent := parentGrant()
		parent.Operations = strings.Split(parentOps, ",")
		parent.Budgets.MaxActions = parentBudget
		parent.DelegationDepthRemaining = parentDepth
		if parentExpH > 0 {
			parent.ExpiresAt = attBase.Add(time.Duration(parentExpH%100000) * time.Hour)
		} else {
			parent.ExpiresAt = time.Time{}
		}
		child := childOf(parent)
		child.Operations = strings.Split(childOps, ",")
		child.Budgets.MaxActions = childBudget
		child.DelegationDepthRemaining = childDepth
		if childExpH > 0 {
			child.ExpiresAt = attBase.Add(time.Duration(childExpH%100000) * time.Hour)
		} else {
			child.ExpiresAt = time.Time{}
		}
		if ValidateAttenuation(parent, child) == nil && !naiveSubsetOracle(parent, child) {
			t.Fatalf("validator accepted a widening the oracle rejects:\nparent=%+v\nchild=%+v", parent, child)
		}
	})
}
