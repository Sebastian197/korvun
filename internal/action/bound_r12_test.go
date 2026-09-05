// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R12-A12: the birth door refuses an empty law pin. Today the pin's
// non-emptiness is load-bearing prose (production always passes a
// real digest) — the wall makes it a refusal at the door, so no
// future caller can park a story the tombstone contract will later
// call corrupt. Evidence level: in-process unit.
// Reproduction-first contract.

package action

import (
	"strings"
	"testing"
	"time"
)

func TestNewBoundApprovalRequest_refusesAnEmptyLawPin(t *testing.T) {
	t.Parallel()
	env := NewEnvelope("act_r12_pin", "env-1",
		Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		Operation{Namespace: "tool", Name: "echo", Version: 1},
		`{"a":1}`, time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC))
	env.Principal = PrincipalRef{PrincipalID: "principal_brain_x"}
	env.Effect = Effect{Class: string(EffectWriteIrreversible)}
	_, err := NewBoundApprovalRequest(env, `{"a":1}`, ApprovalContext{
		Rule: "require_approval", LawVersion: 7, LawDigest: "",
		Now: time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC), TTL: time.Hour,
	})
	if err == nil || !strings.Contains(err.Error(), "law") {
		t.Fatalf("AUDIT R12-A12: an empty law pin must be refused at the birth door, named: %v", err)
	}
}
