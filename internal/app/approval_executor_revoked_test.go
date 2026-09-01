// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// C1 depth check: the approval executor rebuilds tools ONLY from the
// CURRENT grant list — a tool revoked from agent.tools after the park
// must refuse by name even if every other gate were blind to it.

package app

import (
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestBuildApprovalExecutor_revokedToolRefusesByName(t *testing.T) {
	t.Parallel()
	cfg := lawBaseCfg(t, "rvk")
	// The current config grants only what it grants: calc is NOT there.
	cfg.Brains[0].Agent.Tools = []string{"time"}
	preview := action.ActionPreview{
		PrincipalID: "principal_brain_" + cfg.Brains[0].Name,
		Operation:   "tool/calc",
	}
	_, err := BuildApprovalExecutor(cfg, preview)
	if err == nil {
		t.Fatal("AUDIT C1: a tool absent from the CURRENT agent.tools must never rebuild")
	}
	if !strings.Contains(err.Error(), "calc") || !strings.Contains(err.Error(), "grant") {
		t.Fatalf("the refusal must name the tool and the missing grant: %v", err)
	}
}
