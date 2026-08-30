// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ceiling at the gate — Etapa 3, lote 3, pieza 4 (spec FR-CEIL-3):
// the action's class is judged against the governing authority's ceiling
// at decision time — above the ceiling is DENIED with the stable
// effect_ceiling rule, receipt included; an unknown class (ranking above
// critical) can never fit under any finite ceiling here either; no
// ceiling wired keeps today's behavior byte-for-byte (production derived
// grants carry none — the exterior stands). Approved-red contract.

package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/tool"
)

// ceilingHarness wires classifier + identity with a ceiling.
func ceilingHarness(t *testing.T, class action.EffectClass, ceiling action.EffectClass) (*AgentBrain, *identifiedFakeRecorder, *[]string) {
	t.Helper()
	journal := &[]string{}
	rec := &identifiedFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithAgentName("asistente"),
		WithActionRecorder(rec),
		WithEffectClassifier(func(name string) (action.EffectDescriptor, bool) {
			return action.EffectDescriptor{Class: class}, true
		}),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID:      action.RootIntentID,
			EffectCeiling: ceiling,
		}),
	)
	return a, rec, journal
}

func ceilingEnv() *envelope.Envelope {
	return &envelope.Envelope{ID: "env-ceil", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
}

func TestGate_classAboveTheCeilingIsDenied(t *testing.T) {
	t.Parallel()
	a, rec, journal := ceilingHarness(t, action.EffectWriteIrreversible, action.EffectReadExternal)
	out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("a class above the ceiling must be denied, got %q", out)
	}
	for _, step := range *journal {
		if step == "execute" {
			t.Fatal("above the ceiling NEVER executes")
		}
	}
	if len(rec.identifiedEnvs) != 1 {
		t.Fatalf("the refusal leaves its identified receipt, got %d", len(rec.identifiedEnvs))
	}
	if rec.identifiedRules[0] != "effect_ceiling" {
		t.Fatalf("stable rule: got %q, want effect_ceiling", rec.identifiedRules[0])
	}
	if rec.identifiedEnvs[0].Effect.Class != string(action.EffectWriteIrreversible) {
		t.Fatalf("the receipt carries the REAL class that was refused, got %q", rec.identifiedEnvs[0].Effect.Class)
	}
}

func TestGate_classAtAndUnderTheCeilingPasses(t *testing.T) {
	t.Parallel()
	for _, class := range []action.EffectClass{action.EffectPure, action.EffectReadExternal} {
		a, _, journal := ceilingHarness(t, class, action.EffectReadExternal)
		out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
		if out != "done" {
			t.Fatalf("class %s at/under the ceiling must execute, got %q", class, out)
		}
		executed := false
		for _, step := range *journal {
			if step == "execute" {
				executed = true
			}
		}
		if !executed {
			t.Fatalf("class %s must have executed", class)
		}
	}
}

// TestGate_unknownClassUnderAnyCeilingIsDenied: the ladder and the
// ceiling swear together at the gate too — an unknown class ranks above
// critical and fits under nothing finite.
func TestGate_unknownClassUnderAnyCeilingIsDenied(t *testing.T) {
	t.Parallel()
	a, rec, journal := ceilingHarness(t, action.EffectClass("corrupted"), action.EffectCritical)
	out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("an unknown class under even the critical ceiling must be denied, got %q", out)
	}
	for _, step := range *journal {
		if step == "execute" {
			t.Fatal("never executes")
		}
	}
	if rec.identifiedRules[0] != "effect_ceiling" {
		t.Fatalf("rule = %q", rec.identifiedRules[0])
	}
}

func TestGate_noCeilingKeepsTodayByteForByte(t *testing.T) {
	t.Parallel()
	// Ceiling absent ("" — production's derived grants today): the write
	// class executes exactly as in E2.
	a, _, journal := ceilingHarness(t, action.EffectWriteIrreversible, "")
	out := a.runTool(context.Background(), ceilingEnv(), nil, laneText, "journal", `{}`)
	if out != "done" {
		t.Fatalf("no ceiling wired must keep today's behavior, got %q", out)
	}
	if (*journal)[len(*journal)-2] != "execute" {
		t.Fatalf("journal = %v", *journal)
	}
}
