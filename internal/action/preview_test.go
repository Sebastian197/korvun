// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ActionPreview domain — Etapa 5, lote 1, pieza 2 (spec FR-PRV):
// the §15.2 agent diff as a pure type with a deterministic canonical
// form and digest. Every §15.2 row the blueprint demands is a field;
// the plan-diff row is ABSENT BY DESIGN (RESERVED→E6, stated in the
// type). The digest seals what the human will be shown — a preview
// that changes is a different digest, which the approval binding
// turns into an invalidation. Approved-red contract.

package action

import (
	"strings"
	"testing"
)

func testPreview() ActionPreview {
	return ActionPreview{
		ActionID:      "act_p1",
		SchemaVersion: 1,
		IntentPurpose: "semana de pruebas",
		PrincipalID:   "principal_brain_asistente",
		GrantID:       "grant_cfg_5446517958b987c7",
		GrantDepth:    1,
		Operation:     "tool/webhook_call",
		Resources:     []string{"https://hooks.example"},
		DataEgress:    "request body leaves the system to the webhook target",
		ArgsDigest:    "sha256:abcd",
		CostLine:      "3 of 5 actions consumed under the grant budget",
		EffectClass:   EffectWriteIrreversible,
		Reversibility: "irreversible — no documented undo",
		ToolCage:      "webhook_call cage: outbound POST, allowlisted hosts",
		PolicyVersion: 7,
		PolicyDigest:  "sha256:law",
		RequiredRule:  "require_approval",
	}
}

func TestPreviewDigest_deterministicAndFieldSensitive(t *testing.T) {
	t.Parallel()
	p := testPreview()
	d1, d2 := p.Digest(), p.Digest()
	if d1 != d2 || !strings.HasPrefix(d1, "sha256:") {
		t.Fatalf("deterministic pinned-form digest: %q vs %q", d1, d2)
	}
	// EVERY §15.2 row the human sees is sealed: any change is a
	// different preview.
	mutations := []func(*ActionPreview){
		func(x *ActionPreview) { x.IntentPurpose = "otra cosa" },
		func(x *ActionPreview) { x.PrincipalID = "principal_other" },
		func(x *ActionPreview) { x.GrantID = "grant_other" },
		func(x *ActionPreview) { x.GrantDepth = 2 },
		func(x *ActionPreview) { x.Operation = "tool/echo" },
		func(x *ActionPreview) { x.Resources = []string{"https://other.example"} },
		func(x *ActionPreview) { x.DataEgress = "nothing leaves" },
		func(x *ActionPreview) { x.ArgsDigest = "sha256:ffff" },
		func(x *ActionPreview) { x.CostLine = "4 of 5" },
		func(x *ActionPreview) { x.EffectClass = EffectPure },
		func(x *ActionPreview) { x.Reversibility = "reversible" },
		func(x *ActionPreview) { x.ToolCage = "no cage" },
		func(x *ActionPreview) { x.PolicyVersion = 8 },
		func(x *ActionPreview) { x.PolicyDigest = "sha256:otherlaw" },
		func(x *ActionPreview) { x.RequiredRule = "effect_ceiling" },
	}
	for i, mutate := range mutations {
		x := p
		mutate(&x)
		if x.Digest() == d1 {
			t.Fatalf("mutation %d must change the preview digest", i)
		}
	}
	// Resource ORDER is presentation, not substance: the sealed form
	// sorts the set (the E2 contract-terms mold).
	x := testPreview()
	x.Resources = []string{"https://b.example", "https://a.example"}
	y := testPreview()
	y.Resources = []string{"https://a.example", "https://b.example"}
	if x.Digest() != y.Digest() {
		t.Fatal("resource order must not change the digest (sorted set)")
	}
}

func TestPreviewCanonical_roundTripsStrict(t *testing.T) {
	t.Parallel()
	p := testPreview()
	raw := CanonicalPreview(p)
	parsed, err := ParseCanonicalPreview(raw)
	if err != nil {
		t.Fatalf("canonical form must parse: %v", err)
	}
	if parsed.Digest() != p.Digest() {
		t.Fatal("round trip diverged")
	}
	if _, err := ParseCanonicalPreview([]byte(`{"action_id":"act_x","plan_diff":"E6"}`)); err == nil {
		t.Fatal("unknown fields must be refused — plan_diff is RESERVED, not silent")
	}
	if _, err := ParseCanonicalPreview(append(raw, ' ', '{', '}')); err == nil {
		t.Fatal("trailing bytes must be refused")
	}
}

func FuzzPreviewCanonical(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add(CanonicalPreview(testPreview()))
	f.Fuzz(func(t *testing.T, raw []byte) {
		p, err := ParseCanonicalPreview(raw)
		if err != nil {
			return
		}
		again, err := ParseCanonicalPreview(CanonicalPreview(p))
		if err != nil {
			t.Fatalf("re-parse of canonical form failed: %v", err)
		}
		if again.Digest() != p.Digest() {
			t.Fatal("canonical round trip diverged")
		}
	})
}
