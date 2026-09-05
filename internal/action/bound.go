// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The bundle born whole (R4 Phase 2, FR-R4F2-1): BoundApprovalRequest
// carries envelope, gate decision, approval and preview DERIVED
// together by the ONE factory below. Its fields are unexported —
// outside this package there is no way to narrate a preview that the
// facts do not derive; the store accepts only this bundle. The caller
// RESOLVES context facts (intent purpose, grant identity and cost —
// lookups the domain cannot do), and the factory derives every
// dimension of the story from them plus the envelope, the effect
// descriptor and the law pin.

package action

import (
	"fmt"
	"strings"
	"time"
)

// ApprovalContext carries the facts the caller RESOLVES (not narrates
// about the action) plus the injected clock and the law pin.
type ApprovalContext struct {
	// IntentPurpose is the intent row's purpose ("-" when unresolved).
	IntentPurpose string
	// GrantID / GrantDepth / CostLine come from the grant row.
	GrantID    string
	GrantDepth int
	CostLine   string
	// ToolCage is the cage's display name.
	ToolCage string
	// Descriptor is the DECLARED effect-registry entry for the
	// operation; HasDescriptor false means the tool is undeclared and
	// the envelope's class stands alone.
	Descriptor    EffectDescriptor
	HasDescriptor bool
	// LawVersion / LawDigest pin the law that demanded the approval.
	LawVersion int64
	LawDigest  string
	// Rule is the gate rule that parked the action (require_approval).
	Rule string
	// Now / TTL drive the expiry window (E2 clock injection).
	Now time.Time
	TTL time.Duration
}

// BoundApprovalRequest is the born-whole bundle. Constructible ONLY
// through NewBoundApprovalRequest.
type BoundApprovalRequest struct {
	env      Envelope
	approval Approval
	preview  ActionPreview
	params   string
}

// Envelope returns the bound envelope.
func (b BoundApprovalRequest) Envelope() Envelope { return b.env }

// Approval returns the derived approval row.
func (b BoundApprovalRequest) Approval() Approval { return b.approval }

// Preview returns the derived §15.2 preview.
func (b BoundApprovalRequest) Preview() ActionPreview { return b.preview }

// RawParams returns the exact params the executor will run.
func (b BoundApprovalRequest) RawParams() string { return b.params }

// NewBoundApprovalRequest derives the WHOLE approval story. Nothing is
// narrated: digests come from the envelope (the raw params must
// re-derive them), operation and principal from the envelope, effect
// and reversibility and egress from the declared descriptor (which
// must AGREE with the envelope's class — the auditor's saboteur (a)
// refuses at birth as preview_effect_mismatch), the law from the pin,
// the rule from the gate.
func NewBoundApprovalRequest(env Envelope, rawParams string, actx ApprovalContext) (BoundApprovalRequest, error) {
	if actx.Rule == "" {
		return BoundApprovalRequest{}, fmt.Errorf("action: bound approval: an empty gate rule cannot park anything")
	}
	// R12-A12: the law pin's non-emptiness was load-bearing prose —
	// production always passes a real digest, but nothing refused an
	// empty one, and the tombstone contract would later call that
	// story corrupt. The wall lives where the story is born.
	if actx.LawDigest == "" {
		return BoundApprovalRequest{}, fmt.Errorf("action: bound approval: an empty law pin cannot park anything — authority is consumed under a NAMED law")
	}
	if got := Digest(env.Operation, rawParams); got != env.ParametersDigest {
		return BoundApprovalRequest{}, fmt.Errorf("action: bound approval: params re-derive digest %s but the envelope carries %s", got, env.ParametersDigest)
	}
	class := EffectClass(env.Effect.Class)
	egress := "no declared data egress"
	reversibility := "unclassified consequence"
	if actx.HasDescriptor {
		if string(actx.Descriptor.Class) != env.Effect.Class {
			return BoundApprovalRequest{}, fmt.Errorf("action: bound approval: preview_effect_mismatch: the descriptor declares %s but the envelope carries %s — a preview must never understate the consequence", actx.Descriptor.Class, env.Effect.Class)
		}
		class = actx.Descriptor.Class
		if actx.Descriptor.DataEgress {
			egress = "request/result content LEAVES the kernel boundary"
		}
		switch {
		case actx.Descriptor.Reversible:
			reversibility = string(class) + " — reversible"
		case actx.Descriptor.Compensatable:
			reversibility = string(class) + " — compensatable, not reversible"
		default:
			reversibility = string(class) + " — irreversible, no documented undo"
		}
	} else if class != "" {
		reversibility = string(class)
	}
	preview := ActionPreview{
		ActionID:      env.ActionID,
		SchemaVersion: 1,
		IntentPurpose: actx.IntentPurpose,
		PrincipalID:   env.Principal.PrincipalID,
		GrantID:       actx.GrantID,
		GrantDepth:    actx.GrantDepth,
		Operation:     env.Operation.Namespace + "/" + env.Operation.Name,
		Resources:     []string{strings.TrimSpace(env.Source.Channel)},
		DataEgress:    egress,
		ArgsDigest:    env.ParametersDigest,
		CostLine:      actx.CostLine,
		EffectClass:   class,
		Reversibility: reversibility,
		ToolCage:      actx.ToolCage,
		PolicyVersion: actx.LawVersion,
		PolicyDigest:  actx.LawDigest,
		RequiredRule:  actx.Rule,
	}
	approval := Approval{
		ApprovalID:    NewApprovalID(),
		SchemaVersion: 1,
		ActionID:      env.ActionID,
		ActionDigest:  env.ParametersDigest,
		PreviewDigest: preview.Digest(),
		RequestedFrom: OperatorPrincipal().PrincipalID,
		Reason:        actx.Rule,
		RiskSummary:   reversibility,
		PolicyVersion: actx.LawVersion,
		PolicyDigest:  actx.LawDigest,
		RequestedAt:   actx.Now,
		ExpiresAt:     actx.Now.Add(actx.TTL),
		Status:        ApprovalPending,
	}
	if err := ValidatePreviewBinding(approval, preview); err != nil {
		// Unreachable by construction; kept as the factory's own belt.
		return BoundApprovalRequest{}, fmt.Errorf("action: bound approval: %w", err)
	}
	return BoundApprovalRequest{env: env, approval: approval, preview: preview, params: rawParams}, nil
}
