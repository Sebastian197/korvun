// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C8 of the E5 consolidation: the auditor's cross battery, permanent.
// This file holds the member the C1-C6 cures did not already pin
// explicitly — an ALLOWLIST change (a caged tool's hosts) between park
// and decision is a different law and refuses the approve. The other
// members live beside their cures: tool revocation and policy change
// (approvals_c1_test.go, approval_executor_revoked_test.go), swapped
// preview (sqlite approvals_c2_test.go), decide→execute crash and
// resume (approvals_c3_test.go), CLI beside a live server (sqlite
// store_c4_test.go).

package cli

import (
	"strings"
	"testing"
)

func TestApprovalsApprove_allowlistChangeInvalidates(t *testing.T) {
	t.Parallel()
	cfgPath, _, approvalID := parkedRequest(t)
	// The operator widens the webhook cage on the SAME brain — the
	// cage-governing content moved, so the law moved.
	mutateConfig(t, cfgPath, func(cfg map[string]any) {
		agent := cfg["brains"].([]any)[0].(map[string]any)["agent"].(map[string]any)
		agent["tools"] = []any{"calc", "webhook_call"}
		agent["webhook_call"] = map[string]any{"allow_hosts": []any{"hooks.example.com"}}
	})
	code, _, stderr := runIntentCLI(t, "approvals", "approve", "--config", cfgPath, approvalID)
	if code == 0 {
		t.Fatal("AUDIT C8: an allowlist change is a DIFFERENT law and must refuse the approve")
	}
	if !strings.Contains(stderr, "approval_invalidated") || !strings.Contains(stderr, "policy") {
		t.Fatalf("the refusal must name approval_invalidated/policy: %q", stderr)
	}
}
