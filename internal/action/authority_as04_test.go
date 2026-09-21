// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"errors"
	"testing"
)

func TestAuthority_PerOperationBudgetCannotDisappear(t *testing.T) {
	parent, intent, child := authorityTestParent(), authorityTestIntent(), authorityTestChild()
	delete(child.Budget.PerOperation, "tool/webhook_call@1")
	normalized, err := NormalizeAuthorityDelegation(parent, child, intent, authorityTestRemaining(), authorityTestMatchers())
	if err != nil {
		t.Fatal(err)
	}
	if got := normalized.Budget.PerOperation["tool/webhook_call@1"]; got != 2 {
		t.Fatalf("normalized per-operation maximum = %d, want remaining maximum 2", got)
	}
	child = authorityTestChild()
	child.Budget.PerOperation["tool/webhook_call@1"] = 3
	_, err = NormalizeAuthorityDelegation(parent, child, intent, authorityTestRemaining(), authorityTestMatchers())
	var atten *AttenuationError
	if !errors.As(err, &atten) || atten.Dimension != "budget_operation_remaining" {
		t.Fatalf("error = %#v, want budget_operation_remaining", err)
	}
}
