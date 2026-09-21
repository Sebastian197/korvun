// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"slices"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/policy"
)

// configClauseAdmits is what a derived clause set says about one tool on one
// channel: some clause names the tool and admits the channel.
func configClauseAdmits(clauses []action.ConfigAuthorityClause, toolName, channel string) bool {
	for _, clause := range clauses {
		if clause.ToolName == toolName &&
			(slices.Contains(clause.Channels, channel) || slices.Contains(clause.Channels, "*")) {
			return true
		}
	}
	return false
}

// TestAuthority_ConfigMigrationPreservesToolChannelRelation (AS-AUTH-12) holds
// the derived config clauses against the LIVE reducer they must never exceed:
// policy.SelectTools, called with the same grants, for EVERY tool on EVERY
// channel of a small universe. A clause set that admits a (tool, channel) pair
// SelectTools would not allow has manufactured authority — which is exactly
// what flattening the clauses into one channel union does.
//
// The brain mixes every mode on purpose: two tools allowed on DIFFERENT single
// channels, one allowed everywhere, one shadowed, one denied, and one listed
// with no grant at all. Shadow, deny and absent must produce NO clause.
//
// The comparison is made for a Local, Public query. A sensitive tool on a Cloud
// model is refused by SelectTools and not by the clause; that direction only
// narrows, and authority can only reduce SelectTools, never widen it.
//
// Evidence level: in-process, pure functions — the production deriver against
// the production reducer; no store, no boot.
// Probing mutation executed: give every restricted clause the union of all
// configured channels — red, naming the pairs the union invents: «webhook_call
// on console» and «read_file on telegram».
func TestAuthority_ConfigMigrationPreservesToolChannelRelation(t *testing.T) {
	governance := []config.ToolGrantConfig{
		{Tool: "read_file", Mode: "allow", Channels: []string{"console"}},
		{Tool: "webhook_call", Mode: "allow", Channels: []string{"telegram"}},
		{Tool: "time", Mode: "allow"},
		{Tool: "http_fetch", Mode: "shadow", Channels: []string{"console"}},
		{Tool: "write_file", Mode: "deny"},
	}
	tools := []string{"read_file", "webhook_call", "time", "http_fetch", "write_file", "ungranted_tool"}
	channels := []string{"console", "telegram", "discord", "webhook"}
	brain := config.BrainConfig{Name: "accounts", Sensitivity: "public", Agent: &config.AgentConfig{
		Tools: tools, Governance: governance,
	}}

	clauses, err := deriveConfigAuthorityClauses("profile_a", brain)
	if err != nil {
		t.Fatal(err)
	}
	grants, err := toolGrants(governance)
	if err != nil {
		t.Fatal(err)
	}
	attrs := make(map[string]policy.ToolAttrs, len(tools))
	for _, name := range tools {
		attrs[name] = policy.ToolAttrs{}
	}
	for _, channel := range channels {
		decisions, err := policy.SelectTools(grants, attrs, policy.ToolQuery{
			Channel: channel, Sensitivity: policy.Public, Locality: policy.Local,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range tools {
			selected := decisions[name].Mode == policy.ToolAllow
			if derived := configClauseAdmits(clauses, name, channel); derived != selected {
				t.Errorf("%s on %s: the derived clauses admit=%t, SelectTools allows=%t", name, channel, derived, selected)
			}
		}
	}

	// No cartesian product and no duplicates: exactly one clause per ALLOWED
	// tool, and none for a tool that is shadowed, denied or ungranted.
	perTool := map[string]int{}
	for _, clause := range clauses {
		perTool[clause.ToolName]++
	}
	want := map[string]int{"read_file": 1, "webhook_call": 1, "time": 1}
	for _, name := range tools {
		if perTool[name] != want[name] {
			t.Errorf("clauses for %s = %d, want %d", name, perTool[name], want[name])
		}
	}

	// An UNGOVERNED brain gets one explicit compatibility clause per tool, on
	// every channel — and nothing else.
	ungoverned := config.BrainConfig{Name: "accounts", Sensitivity: "public", Agent: &config.AgentConfig{Tools: tools}}
	compat, err := deriveConfigAuthorityClauses("profile_a", ungoverned)
	if err != nil {
		t.Fatal(err)
	}
	if len(compat) != len(tools) {
		t.Errorf("compatibility clauses = %d, want one per tool (%d)", len(compat), len(tools))
	}
	for _, clause := range compat {
		if !slices.Equal(clause.Channels, []string{"*"}) {
			t.Errorf("compatibility clause for %s admits %v, want every channel", clause.ToolName, clause.Channels)
		}
	}
}
