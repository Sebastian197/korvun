// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R4 Phase 2 (FR-R4F2-1): the bundle born whole. The factory DERIVES
// the whole story from the envelope, the rule, the raw params, the
// resolved context, the effect descriptor and the law pin — narrated
// previews die by construction (unexported fields; the only door is
// the factory). A descriptor class contradicting the envelope's
// refuses AT BIRTH by name (the auditor's saboteur (a)).
// Reproduction-first contract.

package action

import (
	"strings"
	"testing"
	"time"
)

func boundInputs() (Envelope, ApprovalContext) {
	env := NewEnvelope("act_bound1", "env-b",
		Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		`http://h/x {"a":1}`, time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC))
	env.IntentID = RootIntentID
	env.Principal = PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = Effect{Class: string(EffectWriteIrreversible)}
	ctx := ApprovalContext{
		IntentPurpose: "semana de pruebas",
		GrantID:       "grant_1",
		GrantDepth:    1,
		CostLine:      "1 of 5",
		ToolCage:      "webhook cage",
		Descriptor: EffectDescriptor{
			Class: EffectWriteIrreversible, DataEgress: true,
		},
		HasDescriptor: true,
		LawVersion:    3,
		LawDigest:     "sha256:law",
		Rule:          "require_approval",
		Now:           time.Date(2026, 9, 2, 10, 0, 1, 0, time.UTC),
		TTL:           time.Hour,
	}
	return env, ctx
}

func TestNewBoundApprovalRequest_derivesTheWholeStory(t *testing.T) {
	t.Parallel()
	env, actx := boundInputs()
	b, err := NewBoundApprovalRequest(env, `http://h/x {"a":1}`, actx)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	a, p := b.Approval(), b.Preview()
	// DERIVED, never narrated: every dimension traces to an input fact.
	if a.ActionDigest != env.ParametersDigest || p.ArgsDigest != env.ParametersDigest {
		t.Fatal("the digests derive from the envelope")
	}
	if p.Operation != "tool/webhook_call" || p.PrincipalID != "principal_brain_a" {
		t.Fatal("operation and principal derive from the envelope")
	}
	if p.EffectClass != EffectWriteIrreversible || !strings.Contains(p.Reversibility, "irreversible") {
		t.Fatalf("effect and reversibility derive from the descriptor: %v %q", p.EffectClass, p.Reversibility)
	}
	if !strings.Contains(p.DataEgress, "LEAVES") {
		t.Fatalf("egress derives from the descriptor: %q", p.DataEgress)
	}
	if p.PolicyVersion != 3 || p.PolicyDigest != "sha256:law" ||
		a.PolicyVersion != 3 || a.PolicyDigest != "sha256:law" {
		t.Fatal("the law pin rides both halves")
	}
	if a.Reason != "require_approval" || p.RequiredRule != "require_approval" {
		t.Fatal("the rule rides both halves")
	}
	if a.PreviewDigest != p.Digest() {
		t.Fatal("the preview digest is sealed by the factory")
	}
	if a.ExpiresAt != actx.Now.Add(actx.TTL) {
		t.Fatalf("expiry derives from the injected clock: %v", a.ExpiresAt)
	}
	// The whole bundle passes the R1 binding by construction.
	if err := ValidatePreviewBinding(a, p); err != nil {
		t.Fatalf("born whole means the binding holds: %v", err)
	}
}

func TestNewBoundApprovalRequest_refusesTheLyingDescriptorAtBirth(t *testing.T) {
	t.Parallel()
	env, actx := boundInputs()
	// The auditor's saboteur (a): a write_reversible descriptor over a
	// CRITICAL envelope — the preview would understate the consequence.
	env.Effect = Effect{Class: string(EffectCritical)}
	actx.Descriptor = EffectDescriptor{Class: EffectWriteReversible, Reversible: true}
	_, err := NewBoundApprovalRequest(env, `http://h/x {"a":1}`, actx)
	if err == nil {
		t.Fatal("AUDIT R4-F2(a): a descriptor contradicting the envelope's class must refuse at birth")
	}
	if !strings.Contains(err.Error(), "preview_effect_mismatch") {
		t.Fatalf("the refusal must name preview_effect_mismatch: %v", err)
	}
}

func TestNewBoundApprovalRequest_paramsMustDeriveTheDigest(t *testing.T) {
	t.Parallel()
	env, actx := boundInputs()
	_, err := NewBoundApprovalRequest(env, `http://h/x {"a":2}`, actx)
	if err == nil {
		t.Fatal("params that do not re-derive the envelope digest must refuse at birth")
	}
	if !strings.Contains(err.Error(), "digest") {
		t.Fatalf("the refusal names the digest: %v", err)
	}
}
