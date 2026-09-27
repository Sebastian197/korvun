// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · «¿Qué pasa hoy?» — cure 1: THE CORE PUBLISHES THE REASON.
//
// RED, and it is red for a reason the adversary captured before a line of this
// screen existed: `brainCanPark` SHORT-CIRCUITS at condition 3 and returns a
// bare bool, and `ApprovalGate` carries four numbers and no motive. Two states
// of the approved mockup — the RED one («se ejecuta al instante», no ceiling)
// and the AMBER one («se observa sin ejecutar», the tool shadowed) — produce
// the SAME `BrainsCanPark = 0`. Any screen that paints them differently is
// recalculating the five conditions, which is exactly what the plan forbids.
//
// So these moulds define the contract: the gate reports EVERY failing
// condition, per brain, from the same place that decides whether a brain can
// park. Nothing here asks `BrainsCanPark` to change — the tray's AS-19 literal
// depends on its value and must stay green.
//
// Evidence level, honest: in-process, over a REAL SQLite store and a REAL
// profile (`parkingProfile`). Not a fake, and not a binary either: nothing here
// proves the wire.
//
// Plan: docs/superpowers/specs/2026-09-23-v0162-que-pasa-hoy-pretest.md,
// guarantees G1 and G2, attacks A2, A3, A4 and A17.

package app

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// failedFor returns the conditions the gate reports for one brain, or nil when
// the gate reports no block for it at all.
func failedFor(t *testing.T, gate controlapi.ApprovalGate, brain string) []controlapi.ParkCondition {
	t.Helper()
	for _, b := range gate.Blocked {
		if b.Brain == brain {
			return b.Failed
		}
	}
	return nil
}

func hasCondition(got []controlapi.ParkCondition, want controlapi.ParkCondition) bool {
	for _, c := range got {
		if c == want {
			return true
		}
	}
	return false
}

// TestGate_reportsEveryFailedCondition is A2, A3 and A4: one condition fails at
// a time and the gate must name THAT one — not a generic «no brain can park»,
// and not a neighbour.
//
// PROBING MUTATION: make the reporter return the first failing condition and
// stop (which is what `brainCanPark` does today). The `two at once` case of the
// sibling mould then reddens, and any row here whose condition is not the first
// in evaluation order reddens with it.
func TestGate_reportsEveryFailedCondition(t *testing.T) {
	cases := []struct {
		name string
		mut  func(b *config.BrainConfig, cfg *config.Config)
		want controlapi.ParkCondition
	}{
		{"no action store", func(_ *config.BrainConfig, cfg *config.Config) {
			cfg.Storage = nil
		}, controlapi.ParkNeedsStore},
		{"not an agent brain", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent = nil
		}, controlapi.ParkNeedsAgent},
		{"the ceiling does not reach write_irreversible", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.EffectCeiling = "write_reversible"
		}, controlapi.ParkNeedsCeiling},
		{"no parkable tool in the cage", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.Tools = []string{"read_file"}
			b.Agent.WebhookCall = nil
		}, controlapi.ParkNeedsParkableTool},
		{"governance DENIES the only parkable tool", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.Governance = []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "deny"}}
		}, controlapi.ParkGovernanceDenies},
		// The AMBER state of the approved mockup, and the reason this is a
		// condition of its own rather than a flavour of «denies»: a shadowed
		// tool is SIMULATED, never executed and never parked. The screen says
		// «se observa sin ejecutar», which is a different sentence from «se
		// ejecuta al instante», and it cannot reach it without this distinction.
		{"governance SHADOWS the only parkable tool", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.Governance = []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}}
		}, controlapi.ParkToolShadowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, store, done := parkingProfile(t)
			defer done()
			tc.mut(&cfg.Brains[0], cfg)

			got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
			if err != nil {
				t.Fatalf("ListPending: %v", err)
			}
			failed := failedFor(t, got.Gate, cfg.Brains[0].Name)
			if len(failed) == 0 {
				t.Fatalf("the gate reports no blocked condition for %q; the screen cannot name what is missing",
					cfg.Brains[0].Name)
			}
			if !hasCondition(failed, tc.want) {
				t.Fatalf("the gate reports %v, which does not name %q", failed, tc.want)
			}
			// And it names ONLY that one: a reporter that marks neighbours
			// would paint four ✕ where the operator has one thing to fix.
			if len(failed) != 1 {
				t.Fatalf("exactly one condition fails here; the gate reports %v", failed)
			}
			// The tray's own number must NOT move: its literal is pinned by AS-19.
			if got.Gate.BrainsCanPark != 0 {
				t.Fatalf("brains_can_park = %d, want 0 — this mould must not change that meaning",
					got.Gate.BrainsCanPark)
			}
		})
	}
}

// TestGate_reportsBothWhenTwoFail is A17, and it is the one the short-circuit
// cannot pass. Two brains' worth of trouble in one: the ceiling is gone AND the
// only parkable tool is denied. The operator has two things to fix and the
// screen must show two.
//
// PROBING MUTATION: return after the first failing condition. The gate then
// reports one, and this mould reddens while every row of its sibling stays
// green — which is what makes the mutation specific to this branch.
func TestGate_reportsBothWhenTwoFail(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	b := &cfg.Brains[0]
	b.Agent.EffectCeiling = "write_reversible"
	b.Agent.Governance = []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "deny"}}

	got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	failed := failedFor(t, got.Gate, b.Name)
	for _, want := range []controlapi.ParkCondition{
		controlapi.ParkNeedsCeiling, controlapi.ParkGovernanceDenies,
	} {
		if !hasCondition(failed, want) {
			t.Fatalf("the gate reports %v; %q is missing — the screen would hide half the work",
				failed, want)
		}
	}
	if len(failed) != 2 {
		t.Fatalf("two conditions fail; the gate reports %d: %v", len(failed), failed)
	}
}

// TestGate_reportsNothingWhenNothingBlocks is the control, and it is what keeps
// the two moulds above from passing by accident: over the profile that CAN
// park, the gate must report no block at all. A reporter that always names
// something would satisfy both of them and fail this one.
func TestGate_reportsNothingWhenNothingBlocks(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()

	got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if failed := failedFor(t, got.Gate, cfg.Brains[0].Name); len(failed) != 0 {
		t.Fatalf("nothing blocks this profile, yet the gate reports %v", failed)
	}
	if got.Gate.BrainsCanPark != 1 {
		t.Fatalf("brains_can_park = %d, want 1", got.Gate.BrainsCanPark)
	}
}

// blockFor returns the whole ParkBlock, not only its conditions, so a mould can
// look at what the gate NAMES as well as at what it counts.
func blockFor(t *testing.T, gate controlapi.ApprovalGate, brain string) controlapi.ParkBlock {
	t.Helper()
	for _, b := range gate.Blocked {
		if b.Brain == brain {
			return b
		}
	}
	t.Fatalf("the gate reports no block for %q", brain)
	return controlapi.ParkBlock{}
}

// TestGate_namesTheToolItsConditionIsAbout is the cure for a defect found while
// wiring the screen, not by a test: the window's «Levantar la sombra» and
// «Añadir host» buttons had no way to know WHICH tool they act on, so they
// hardcoded `webhook_call`.
//
// HOW FAR THIS MOULD REACHES, stated because it is less than it looks. Today
// `webhook_call` is the ONLY builtin whose class is parkable (`internal/tool`
// effects.go: memory_note is write_reversible, read_file and http_fetch are
// read_external), so the gate cannot currently report a shadowed tool by any
// other name, and this mould cannot tell «names the shadowed tool» apart from
// «names the only parkable tool». What it DOES pin is the wire: the gate reports
// the name, so the screen stops depending on an accident of the builtin table —
// and it already did not hold for `allow-host`, which applies to `http_fetch`
// too, where the hardcoded name was simply wrong.
//
// The discriminating case (two parkable tools, the second one shadowed) is
// FILED: it becomes reachable the day a second irreversible builtin lands, and
// that train owns the mould that tells them apart.
//
// PROBING MUTATION (executed): stop setting `culprit` in the shadowed branch.
// The gate reports the condition with an empty tool and this reddens naming it.
func TestGate_namesTheToolItsConditionIsAbout(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	b := &cfg.Brains[0]
	b.Agent.Governance = []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}}

	got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	block := blockFor(t, got.Gate, b.Name)
	if !hasCondition(block.Failed, controlapi.ParkToolShadowed) {
		t.Fatalf("the gate reports %v, which does not name the shadowed tool's condition", block.Failed)
	}
	if block.Tool != "webhook_call" {
		t.Fatalf("the gate names the tool %q; the shadowed tool is webhook_call, and a screen given an empty or wrong name POSTs an edit the door refuses", block.Tool)
	}
}

// TestGate_namesNoToolWhereTheConditionIsAboutNone: a missing ceiling is not
// about a tool, and the gate must not name one. An empty name is what tells the
// screen to draw no tool button — a name invented here would put a button in
// front of the operator that could only ever fail.
//
// WHERE THE GUARANTEE ACTUALLY LIVES, and the mutation that proves it. The
// branch this mould watches is the early return the ceiling case takes when the
// TOOL is fine: `return failed, ""`. The first mutation tried here set `culprit`
// at the ceiling branch and left this mould GREEN — that early return discards
// `culprit` by writing the empty literal, so the mutation never reached the
// answer. A mutation in the wrong place proves nothing, and a green under it is
// the finding, not a pass.
//
// PROBING MUTATION (executed, M33-bis): change that early return to
// `return failed, name`. The ceiling row then carries the tool that was FINE,
// and this reddens naming it.
func TestGate_namesNoToolWhereTheConditionIsAboutNone(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	b := &cfg.Brains[0]
	b.Agent.EffectCeiling = "write_reversible"

	got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	block := blockFor(t, got.Gate, b.Name)
	if !hasCondition(block.Failed, controlapi.ParkNeedsCeiling) {
		t.Fatalf("the gate reports %v, which does not name the ceiling", block.Failed)
	}
	if block.Tool != "" {
		t.Fatalf("the gate names the tool %q on a condition that is not about a tool", block.Tool)
	}
}
