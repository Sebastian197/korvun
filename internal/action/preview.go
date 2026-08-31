// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// ActionPreview domain (Trust Layer Etapa 5, lote 1, spec FR-PRV): the
// §15.2 agent diff as a pure type — what the human is shown before
// deciding, sealed by a deterministic digest so "the version shown" is
// a checkable identity, not a screenshot. Every §15.2 row is a field;
// "difference against a previously approved plan" is ABSENT BY DESIGN
// (RESERVED→E6 with the transaction coordinator). Raw parameters are
// NOT part of the preview type: shared surfaces carry the args digest
// only (ADR-0024), and the operator's loopback surface joins the full
// parameters from the stored envelope at render time.
package action

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ActionPreview is the structured agent diff of ONE action (§15.2).
type ActionPreview struct {
	// ActionID names the action this preview describes.
	ActionID string
	// SchemaVersion pins the preview wire form (1 in Etapa 5).
	SchemaVersion int
	// IntentPurpose is the human intention's stated goal.
	IntentPurpose string
	// PrincipalID and GrantID/GrantDepth are the actor and its
	// delegation chain position.
	PrincipalID string
	GrantID     string
	GrantDepth  int
	// Operation is the namespaced operation ("tool/<name>").
	Operation string
	// Resources are the coarse resources the operation touches.
	Resources []string
	// DataEgress states WHAT LEAVES the system, from the E3 effect
	// descriptor's declared egress — never raw content.
	DataEgress string
	// ArgsDigest is the bounded reference to the exact arguments (the
	// E1 parameters digest; raw args live only in the stored envelope).
	ArgsDigest string
	// CostLine is the budget statement (the E2 ledger view).
	CostLine string
	// EffectClass and Reversibility spell the consequence (E3 ladder).
	EffectClass   EffectClass
	Reversibility string
	// ToolCage names the credentials/systems involved, honest for
	// today: the tool and its cage — no broker until E7.
	ToolCage string
	// PolicyVersion/PolicyDigest/RequiredRule pin the relevant law and
	// the rule that demanded approval.
	PolicyVersion int64
	PolicyDigest  string
	RequiredRule  string
}

// Digest returns the deterministic digest of the preview — the sealed
// identity of "the version shown". Resource order is presentation:
// the sealed form sorts the set (the E2 contract-terms mold).
func (p ActionPreview) Digest() string {
	return HashCanonical(contractTerms(map[string]any{
		"action_id":      p.ActionID,
		"schema_version": p.SchemaVersion,
		"purpose":        p.IntentPurpose,
		"principal":      p.PrincipalID,
		"grant":          p.GrantID,
		"depth":          p.GrantDepth,
		"operation":      p.Operation,
		"resources":      sortedSet(p.Resources),
		"egress":         p.DataEgress,
		"args_digest":    p.ArgsDigest,
		"cost":           p.CostLine,
		"effect_class":   string(p.EffectClass),
		"reversibility":  p.Reversibility,
		"cage":           p.ToolCage,
		"policy_version": p.PolicyVersion,
		"policy_digest":  p.PolicyDigest,
		"required_rule":  p.RequiredRule,
	}))
}

// previewWire is the canonical JSON shape.
type previewWire struct {
	ActionID      string   `json:"action_id"`
	SchemaVersion int      `json:"schema_version"`
	IntentPurpose string   `json:"intent_purpose"`
	PrincipalID   string   `json:"principal_id"`
	GrantID       string   `json:"grant_id"`
	GrantDepth    int      `json:"grant_depth"`
	Operation     string   `json:"operation"`
	Resources     []string `json:"resources"`
	DataEgress    string   `json:"data_egress"`
	ArgsDigest    string   `json:"args_digest"`
	CostLine      string   `json:"cost_line"`
	EffectClass   string   `json:"effect_class"`
	Reversibility string   `json:"reversibility"`
	ToolCage      string   `json:"tool_cage"`
	PolicyVersion int64    `json:"policy_version"`
	PolicyDigest  string   `json:"policy_digest"`
	RequiredRule  string   `json:"required_rule"`
}

// CanonicalPreview renders the deterministic wire form of a preview.
func CanonicalPreview(p ActionPreview) []byte {
	raw, err := json.Marshal(previewWire{
		ActionID:      p.ActionID,
		SchemaVersion: p.SchemaVersion,
		IntentPurpose: p.IntentPurpose,
		PrincipalID:   p.PrincipalID,
		GrantID:       p.GrantID,
		GrantDepth:    p.GrantDepth,
		Operation:     p.Operation,
		Resources:     sortedSet(p.Resources),
		DataEgress:    p.DataEgress,
		ArgsDigest:    p.ArgsDigest,
		CostLine:      p.CostLine,
		EffectClass:   string(p.EffectClass),
		Reversibility: p.Reversibility,
		ToolCage:      p.ToolCage,
		PolicyVersion: p.PolicyVersion,
		PolicyDigest:  p.PolicyDigest,
		RequiredRule:  p.RequiredRule,
	})
	if err != nil {
		// Unreachable: plain strings, ints and a string slice.
		panic("action: canonical preview encoding failed: " + err.Error())
	}
	return raw
}

// ParseCanonicalPreview parses the canonical wire form STRICTLY:
// unknown fields (plan_diff included — RESERVED means refused, not
// silently dropped) and trailing bytes are errors.
func ParseCanonicalPreview(raw []byte) (ActionPreview, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w previewWire
	if err := dec.Decode(&w); err != nil {
		return ActionPreview{}, fmt.Errorf("action: parse canonical preview: %w", err)
	}
	if dec.More() {
		return ActionPreview{}, fmt.Errorf("action: parse canonical preview: trailing data")
	}
	return ActionPreview{
		ActionID:      w.ActionID,
		SchemaVersion: w.SchemaVersion,
		IntentPurpose: w.IntentPurpose,
		PrincipalID:   w.PrincipalID,
		GrantID:       w.GrantID,
		GrantDepth:    w.GrantDepth,
		Operation:     w.Operation,
		Resources:     w.Resources,
		DataEgress:    w.DataEgress,
		ArgsDigest:    w.ArgsDigest,
		CostLine:      w.CostLine,
		EffectClass:   EffectClass(w.EffectClass),
		Reversibility: w.Reversibility,
		ToolCage:      w.ToolCage,
		PolicyVersion: w.PolicyVersion,
		PolicyDigest:  w.PolicyDigest,
		RequiredRule:  w.RequiredRule,
	}, nil
}
