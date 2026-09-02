// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R1 of the third Codex pass (adjudicated 2026-09-01): born-whole was
// missing its CROSS LINKS — the approval and its preview could tell
// different stories (a preview consistent WITH ITSELF but lying about
// the law, the args or the rule it claims). Every link is enforced at
// BIRTH and re-verified at the DECISION touch, each with a NAMED
// failure: preview_digest_mismatch (C2, kept), preview_policy_mismatch,
// preview_args_mismatch, preview_rule_mismatch. One saboteur per link.
// Reproduction-first contract.

package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// birthSaboteurs returns, per link, a mutation of the request parts
// that keeps each part self-consistent while breaking the CROSS story.
func birthSaboteurs() map[string]func(a *action.Approval, p *action.ActionPreview) {
	return map[string]func(a *action.Approval, p *action.ActionPreview){
		"preview_policy_mismatch": func(a *action.Approval, p *action.ActionPreview) {
			p.PolicyDigest = "sha256:another-law"
		},
		"preview_args_mismatch": func(a *action.Approval, p *action.ActionPreview) {
			p.ArgsDigest = "sha256:other-args"
		},
		"preview_rule_mismatch": func(a *action.Approval, p *action.ActionPreview) {
			p.RequiredRule = "some_other_rule"
		},
		"preview_digest_mismatch": func(a *action.Approval, p *action.ActionPreview) {
			// Content moved, pin not: the C2 link, one saboteur mold.
			p.IntentPurpose = "a different story than the pinned one"
		},
	}
}

func TestCreateApprovalRequest_everyCrossLinkEnforcedAtBirth(t *testing.T) {
	t.Parallel()
	store, _ := openTemp(t)
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	i := 0
	for wantName, sabotage := range birthSaboteurs() {
		env := testEnvelope("act_r1_birth_" + wantName)
		a, p := testApprovalFor(env)
		sabotage(&a, &p)
		// The digest link self-heals for the non-digest saboteurs: the
		// preview digest is recomputed over the LYING preview, so the
		// pair stays self-consistent — only the cross link is broken.
		if wantName != "preview_digest_mismatch" {
			a.PreviewDigest = p.Digest()
		}
		err := store.createApprovalParts(ctx, env,
			Decision{Outcome: "require_approval", Rule: a.Reason,
				PolicyVersion: a.PolicyVersion, PolicyDigest: a.PolicyDigest},
			a, p, `{"a":1}`)
		if err == nil {
			t.Fatalf("AUDIT R1: the %s saboteur must refuse at birth", wantName)
		}
		if !strings.Contains(err.Error(), wantName) {
			t.Fatalf("the refusal must NAME %s: %v", wantName, err)
		}
		i++
	}
	if i != 4 {
		t.Fatalf("four links, four saboteurs: %d", i)
	}
}

func TestDecide_everyCrossLinkReverifiedAtTheTouch(t *testing.T) {
	t.Parallel()
	for wantName, sabotage := range birthSaboteurs() {
		store, _ := sealedStore(t)
		ctx := context.Background()
		env := testEnvelope("act_r1_dec")
		a, p := testApprovalFor(env)
		if err := store.createApprovalParts(ctx, env,
			Decision{Outcome: "require_approval", Rule: a.Reason,
				PolicyVersion: a.PolicyVersion, PolicyDigest: a.PolicyDigest},
			a, p, `{"a":1}`); err != nil {
			t.Fatalf("birth: %v", err)
		}
		// The auditor's raw hand: rewrite the sealed preview AND its
		// pinned digest IN AGREEMENT — self-consistent, cross-lying.
		lying := p
		sabotage(&a, &lying)
		corruptCell(t, store, "approvals", "canonical_preview", "approval_id",
			a.ApprovalID, string(action.CanonicalPreview(lying)))
		if wantName != "preview_digest_mismatch" {
			corruptCell(t, store, "approvals", "preview_digest", "approval_id",
				a.ApprovalID, lying.Digest())
		}
		envD, identD := operatorDecisionEnv("reject", a.ApprovalID)
		_, err := store.decideApproval(ctx, a.ApprovalID, "rejected",
			a.RequestedAt.Add(time.Minute), envD, identD, "")
		if err == nil {
			t.Fatalf("AUDIT R1: the %s saboteur must refuse the decision touch", wantName)
		}
		if !strings.Contains(err.Error(), wantName) {
			t.Fatalf("the decision refusal must NAME %s: %v", wantName, err)
		}
	}
}
