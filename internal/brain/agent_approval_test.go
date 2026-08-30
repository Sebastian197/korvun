// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The new outcomes, failing closed — Etapa 3, lote 5 (spec FR-REQ, sealed
// NC-2(b)): under BOUNDED authority (a ceilinged grant — never the root)
// a write_irreversible or critical action requires human approval, and
// with no approval workflow until E5 it dies DENIED approval_unavailable
// with its full receipt (the real refused class inside). require_prepare
// likewise: no connector supports prepare yet, so any path demanding it
// dies prepare_unavailable — never a pass for lack of machinery. The
// root and the derived grants never meet these outcomes (pinned), and
// the gate's precedence is deterministic: the first applicable denial
// wins with its stable code. Approved-red contract.

package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestApproval_irreversibleUnderBoundedAuthorityDiesHonestly (a): the
// sealed NC-2(b) — bounded authority gets the full §10.6 treatment
// TODAY, fail-closed.
func TestApproval_irreversibleUnderBoundedAuthorityDiesHonestly(t *testing.T) {
	t.Parallel()
	for _, class := range []action.EffectClass{action.EffectWriteIrreversible, action.EffectCritical} {
		// Ceiling critical: the class FITS under the ceiling — approval is
		// the reason it dies, not the ceiling.
		a, rec, journal := ceilingHarness(t, class, action.EffectCritical)
		out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
		if out != deniedObservation("journal") {
			t.Fatalf("%s under bounded authority must be denied, got %q", class, out)
		}
		for _, step := range *journal {
			if step == "execute" {
				t.Fatalf("%s must NEVER execute without approval", class)
			}
		}
		if rec.identifiedRules[0] != "approval_unavailable" {
			t.Fatalf("stable rule for %s: got %q, want approval_unavailable", class, rec.identifiedRules[0])
		}
		if rec.identifiedEnvs[0].Effect.Class != string(class) {
			t.Fatalf("the receipt carries the REAL refused class, got %q", rec.identifiedEnvs[0].Effect.Class)
		}
	}
}

// (a) continued: classes below write_irreversible under the same bounded
// authority keep executing — approval is demanded by the §10.6 treatment
// table, not sprayed over everything.
func TestApproval_lowerClassesUnderBoundedAuthorityStillExecute(t *testing.T) {
	t.Parallel()
	for _, class := range []action.EffectClass{
		action.EffectPure, action.EffectReadExternal,
		action.EffectWriteReversible, action.EffectWriteCompensatable,
	} {
		a, _, journal := ceilingHarness(t, class, action.EffectCritical)
		out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
		if out != "done" {
			t.Fatalf("%s under bounded authority must execute, got %q", class, out)
		}
		executed := false
		for _, step := range *journal {
			if step == "execute" {
				executed = true
			}
		}
		if !executed {
			t.Fatalf("%s must have executed", class)
		}
	}
}

// TestApproval_rootAndDerivedNeverMeetTheseOutcomes (c): the exterior
// pinned explicitly — no ceiling wired (the root's standing authority
// and today's derived grants) means today's behavior byte-for-byte,
// write_irreversible included.
func TestApproval_rootAndDerivedNeverMeetTheseOutcomes(t *testing.T) {
	t.Parallel()
	for _, class := range []action.EffectClass{action.EffectWriteIrreversible, action.EffectCritical} {
		a, rec, _ := ceilingHarness(t, class, "")
		out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
		if out != "done" {
			t.Fatalf("under the root's standing authority %s executes exactly as in v0.12.0, got %q", class, out)
		}
		for _, rule := range rec.identifiedRules {
			if rule == "approval_unavailable" || rule == "prepare_unavailable" {
				t.Fatalf("today's flows must never meet the new outcomes, got %q", rule)
			}
		}
	}
}

// TestGatePrecedence_isDeterministic (d, §13.3 subset): the first
// applicable denial wins with its stable code — effect_ceiling outranks
// approval (a class over the ceiling is refused as such, even when it
// would also require approval), and the pure helper pins the whole
// precedence table.
func TestGatePrecedence_isDeterministic(t *testing.T) {
	t.Parallel()
	// Over the ceiling AND approval-class: the ceiling wins.
	a, rec, _ := ceilingHarness(t, action.EffectCritical, action.EffectReadExternal)
	out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("denied, got %q", out)
	}
	if rec.identifiedRules[0] != "effect_ceiling" {
		t.Fatalf("the FIRST applicable denial wins: got %q, want effect_ceiling", rec.identifiedRules[0])
	}
	// The pure helper's precedence table, row by row.
	cases := []struct {
		class   action.EffectClass
		ceiling action.EffectClass
		prepare bool
		want    string
	}{
		{action.EffectPure, "", false, ""},
		{action.EffectCritical, "", false, ""},                                      // root: no ceiling, no demand
		{action.EffectCritical, action.EffectReadExternal, false, "effect_ceiling"}, // ceiling first
		{action.EffectCritical, action.EffectCritical, false, "approval_unavailable"},
		{action.EffectWriteIrreversible, action.EffectCritical, false, "approval_unavailable"},
		{action.EffectWriteReversible, action.EffectCritical, false, ""},
		{action.EffectPure, action.EffectPure, true, "prepare_unavailable"},
		{action.EffectCritical, action.EffectReadExternal, true, "effect_ceiling"}, // ceiling still first
		{action.EffectClass("garbage"), action.EffectCritical, false, "effect_ceiling"},
	}
	for _, tc := range cases {
		got := effectGateRule(tc.class, tc.ceiling, tc.prepare)
		if got != tc.want {
			t.Fatalf("effectGateRule(%s, %s, prepare=%v) = %q, want %q",
				tc.class, tc.ceiling, tc.prepare, got, tc.want)
		}
	}
}

// TestPrepare_unavailableFailsClosed (b): the vocabulary and its wall
// exist NOW; the demand signal stays structurally unreachable until a
// connector declares prepare support (its descriptor field is RESERVED)
// — but whatever demands it dies closed, never passes.
func TestPrepare_unavailableFailsClosed(t *testing.T) {
	t.Parallel()
	if got := effectGateRule(action.EffectReadExternal, action.EffectCritical, true); got != "prepare_unavailable" {
		t.Fatalf("a prepare demand without machinery must die prepare_unavailable, got %q", got)
	}
	// And under NO bounded authority a prepare demand STILL dies: the rule
	// guards the mechanism's absence, not the authority's shape.
	if got := effectGateRule(action.EffectReadExternal, "", true); got != "prepare_unavailable" {
		t.Fatalf("prepare_unavailable is authority-independent, got %q", got)
	}
}
