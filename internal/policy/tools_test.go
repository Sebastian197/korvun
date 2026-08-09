// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"errors"
	"testing"
)

// The SelectTools contract under test (ADR-0041 §1, spec FR-GOV-1/2/5/6):
// a PURE per-message filter producing one ToolDecision per tool for
// (grants, declared attrs, channel, brain sensitivity, model locality).
// Restrictions apply BEFORE the granted mode (restrict, never widen);
// unknown/zero inputs fail loud with wrapped sentinels.

func publicLocalQuery(channel string) ToolQuery {
	return ToolQuery{Channel: channel, Sensitivity: Public, Locality: Local}
}

func TestSelectTools_decisions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		grants []ToolGrant
		attrs  map[string]ToolAttrs
		q      ToolQuery
		tool   string
		want   ToolDecision
	}{
		{
			name:   "plain allow grant",
			grants: []ToolGrant{{Name: "calc", Mode: ToolAllow}},
			q:      publicLocalQuery("telegram"),
			tool:   "calc",
			want:   ToolDecision{Mode: ToolAllow},
		},
		{
			name:   "plain shadow grant",
			grants: []ToolGrant{{Name: "calc", Mode: ToolShadow}},
			q:      publicLocalQuery("telegram"),
			tool:   "calc",
			want:   ToolDecision{Mode: ToolShadow},
		},
		{
			name:   "explicit deny grant",
			grants: []ToolGrant{{Name: "calc", Mode: ToolDeny}},
			q:      publicLocalQuery("telegram"),
			tool:   "calc",
			want:   ToolDecision{Mode: ToolDeny, Rule: ToolRuleDenyGrant},
		},
		{
			name:   "channel restriction matches",
			grants: []ToolGrant{{Name: "calc", Mode: ToolAllow, Channels: []string{"telegram", "console"}}},
			q:      publicLocalQuery("console"),
			tool:   "calc",
			want:   ToolDecision{Mode: ToolAllow},
		},
		{
			name:   "channel restriction unmatched denies",
			grants: []ToolGrant{{Name: "calc", Mode: ToolAllow, Channels: []string{"telegram"}}},
			q:      publicLocalQuery("discord"),
			tool:   "calc",
			want:   ToolDecision{Mode: ToolDeny, Rule: ToolRuleChannel},
		},
		{
			name:   "channel restriction beats shadow too",
			grants: []ToolGrant{{Name: "calc", Mode: ToolShadow, Channels: []string{"telegram"}}},
			q:      publicLocalQuery("discord"),
			tool:   "calc",
			want:   ToolDecision{Mode: ToolDeny, Rule: ToolRuleChannel},
		},
		{
			name:   "sensitive tool on a cloud model denies",
			grants: []ToolGrant{{Name: "read_file", Mode: ToolAllow}},
			attrs:  map[string]ToolAttrs{"read_file": {Sensitive: true}},
			q:      ToolQuery{Channel: "telegram", Sensitivity: Public, Locality: Cloud},
			tool:   "read_file",
			want:   ToolDecision{Mode: ToolDeny, Rule: ToolRuleSensitiveLocality},
		},
		{
			name:   "sensitivity beats shadow (no rehearsal of a privacy-forbidden tool)",
			grants: []ToolGrant{{Name: "read_file", Mode: ToolShadow}},
			attrs:  map[string]ToolAttrs{"read_file": {Sensitive: true}},
			q:      ToolQuery{Channel: "telegram", Sensitivity: Public, Locality: Cloud},
			tool:   "read_file",
			want:   ToolDecision{Mode: ToolDeny, Rule: ToolRuleSensitiveLocality},
		},
		{
			name:   "sensitive tool on a local model allows",
			grants: []ToolGrant{{Name: "read_file", Mode: ToolAllow}},
			attrs:  map[string]ToolAttrs{"read_file": {Sensitive: true}},
			q:      publicLocalQuery("telegram"),
			tool:   "read_file",
			want:   ToolDecision{Mode: ToolAllow},
		},
		{
			name:   "network tool on a private brain arms the shield",
			grants: []ToolGrant{{Name: "http_fetch", Mode: ToolAllow}},
			attrs:  map[string]ToolAttrs{"http_fetch": {Network: true}},
			q:      ToolQuery{Channel: "telegram", Sensitivity: Private, Locality: Local},
			tool:   "http_fetch",
			want:   ToolDecision{Mode: ToolAllow, Shield: true},
		},
		{
			name:   "network tool on a public brain has no shield",
			grants: []ToolGrant{{Name: "http_fetch", Mode: ToolAllow}},
			attrs:  map[string]ToolAttrs{"http_fetch": {Network: true}},
			q:      publicLocalQuery("telegram"),
			tool:   "http_fetch",
			want:   ToolDecision{Mode: ToolAllow},
		},
		{
			name:   "shadowed network tool on a private brain keeps the shield flag",
			grants: []ToolGrant{{Name: "http_fetch", Mode: ToolShadow}},
			attrs:  map[string]ToolAttrs{"http_fetch": {Network: true}},
			q:      ToolQuery{Channel: "telegram", Sensitivity: Private, Locality: Local},
			tool:   "http_fetch",
			want:   ToolDecision{Mode: ToolShadow, Shield: true},
		},
		{
			name:   "attrs key without a grant denies as not granted",
			grants: []ToolGrant{{Name: "calc", Mode: ToolAllow}},
			attrs:  map[string]ToolAttrs{"http_fetch": {Network: true}},
			q:      publicLocalQuery("telegram"),
			tool:   "http_fetch",
			want:   ToolDecision{Mode: ToolDeny, Rule: ToolRuleNotGranted},
		},
		{
			name:   "granted tool absent from attrs gets zero attrs",
			grants: []ToolGrant{{Name: "echo", Mode: ToolAllow}},
			q:      ToolQuery{Channel: "telegram", Sensitivity: Private, Locality: Cloud},
			tool:   "echo",
			want:   ToolDecision{Mode: ToolAllow},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SelectTools(tc.grants, tc.attrs, tc.q)
			if err != nil {
				t.Fatalf("SelectTools: %v", err)
			}
			d, ok := got[tc.tool]
			if !ok {
				t.Fatalf("no decision for %q in %+v", tc.tool, got)
			}
			if d != tc.want {
				t.Fatalf("decision for %q = %+v, want %+v", tc.tool, d, tc.want)
			}
		})
	}
}

func TestSelectTools_coversUnionOfGrantsAndAttrs(t *testing.T) {
	t.Parallel()
	grants := []ToolGrant{{Name: "calc", Mode: ToolAllow}}
	attrs := map[string]ToolAttrs{"http_fetch": {Network: true}}

	got, err := SelectTools(grants, attrs, publicLocalQuery("telegram"))
	if err != nil {
		t.Fatalf("SelectTools: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("decisions cover %d tools, want 2 (union of grants and attrs): %+v", len(got), got)
	}
}

func TestSelectTools_errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		grants  []ToolGrant
		q       ToolQuery
		wantErr error
	}{
		{
			name:    "zero sensitivity fails loud",
			grants:  []ToolGrant{{Name: "calc", Mode: ToolAllow}},
			q:       ToolQuery{Channel: "telegram", Locality: Local},
			wantErr: ErrUnknownSensitivity,
		},
		{
			name:    "zero locality fails loud",
			grants:  []ToolGrant{{Name: "calc", Mode: ToolAllow}},
			q:       ToolQuery{Channel: "telegram", Sensitivity: Public},
			wantErr: ErrUnknownLocality,
		},
		{
			name:    "zero grant mode fails loud",
			grants:  []ToolGrant{{Name: "calc"}},
			q:       publicLocalQuery("telegram"),
			wantErr: ErrUnknownToolMode,
		},
		{
			name:    "duplicate grant fails loud",
			grants:  []ToolGrant{{Name: "calc", Mode: ToolAllow}, {Name: "calc", Mode: ToolDeny}},
			q:       publicLocalQuery("telegram"),
			wantErr: ErrDuplicateToolGrant,
		},
		{
			name:    "empty grant name fails loud",
			grants:  []ToolGrant{{Name: "", Mode: ToolAllow}},
			q:       publicLocalQuery("telegram"),
			wantErr: ErrInvalidToolGrant,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SelectTools(tc.grants, nil, tc.q)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErr)
			}
			if got != nil {
				t.Fatalf("decisions = %+v, want nil on error", got)
			}
		})
	}
}

// SelectTools is PURE: repeated calls agree and inputs are never mutated —
// it runs on the hot path once per Handle (spec FR-GOV-1).
func TestSelectTools_pureAndDeterministic(t *testing.T) {
	t.Parallel()
	grants := []ToolGrant{
		{Name: "calc", Mode: ToolAllow, Channels: []string{"telegram"}},
		{Name: "http_fetch", Mode: ToolShadow},
	}
	attrs := map[string]ToolAttrs{"http_fetch": {Network: true}}
	q := ToolQuery{Channel: "telegram", Sensitivity: Private, Locality: Local}

	first, err := SelectTools(grants, attrs, q)
	if err != nil {
		t.Fatalf("SelectTools: %v", err)
	}
	second, err := SelectTools(grants, attrs, q)
	if err != nil {
		t.Fatalf("SelectTools (second): %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("non-deterministic size: %d vs %d", len(first), len(second))
	}
	for name, d := range first {
		if second[name] != d {
			t.Fatalf("non-deterministic decision for %q: %+v vs %+v", name, d, second[name])
		}
	}
	if grants[0].Name != "calc" || len(grants[0].Channels) != 1 || grants[0].Channels[0] != "telegram" {
		t.Fatalf("grants mutated: %+v", grants)
	}
	if a := attrs["http_fetch"]; !a.Network || a.Sensitive {
		t.Fatalf("attrs mutated: %+v", attrs)
	}
}
