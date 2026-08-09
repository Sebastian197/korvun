// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import "fmt"

// This file is the tool half of the pre-dispatch policy engine (ADR-0041 §1,
// the tool-shaped sibling of SelectModels): a PURE per-message filter deciding,
// for one brain and one inbound message, which tools the agent may see and run,
// in which mode. Unlike SelectModels (run once at construction), SelectTools
// runs once per Handle — the channel arrives per-message in the Envelope — so
// it must stay pure, allocation-light, and I/O-free.

// ToolMode is the tri-state grant mode (ADR-0041 §1, spec FR-GOV-5). The zero
// value is intentionally invalid so an unconfigured grant fails loud
// (ErrUnknownToolMode) rather than silently defaulting — the same discipline
// Sensitivity carries (ADR-0015).
type ToolMode int

const (
	// ToolAllow advertises the tool to the model and executes its calls.
	ToolAllow ToolMode = iota + 1
	// ToolShadow advertises the tool but NEVER executes it: the call is
	// recorded and the model receives an honest simulation observation
	// (ADR-0041 §2). Shadow exists to observe the model's real judgment
	// before promoting a grant to ToolAllow.
	ToolShadow
	// ToolDeny neither advertises nor executes the tool.
	ToolDeny
)

// ToolAttrs are the DECLARED attributes of a catalog tool the gate routes on
// (house default + operator override, ADR-0041 §1). They are declared at
// wiring time, NEVER inferred — inferring sensitivity is the recursive
// privacy trap ADR-0012 §5e forbids. The zero value (not sensitive, not
// network) is the house default for a tool with no declaration.
type ToolAttrs struct {
	// Sensitive restricts the tool to locally-running models: on a brain
	// whose model has Locality Cloud, a sensitive tool is denied outright.
	Sensitive bool
	// Network marks a tool that reaches the network, making it subject to
	// the Private-brain network shield (ADR-0041 §3): ToolDecision.Shield is
	// armed so the execution layer validates the dialed IP.
	Network bool
}

// ToolGrant is one per-brain grant (config: the agent block, ADR-0041 §1).
type ToolGrant struct {
	// Name is the tool's protocol name (the tool.Tool Name()).
	Name string
	// Mode is the tri-state grant mode. Zero fails loud.
	Mode ToolMode
	// Channels, when non-empty, restricts the grant to those channels
	// (exact match on Envelope.Channel). Empty = every channel.
	Channels []string
}

// ToolRule names the dimension that decided a non-allow outcome, for the
// audit surfaces (ADR-0041 §5). The selection-layer rules are produced by
// SelectTools; the dial-time/execution-layer rules (shield, cage) share the
// same grammar and are emitted by the caged tools.
type ToolRule string

const (
	// ToolRuleNone marks an allowed or shadowed decision (no denial).
	ToolRuleNone ToolRule = ""
	// ToolRuleDenyGrant marks an explicit ToolDeny grant.
	ToolRuleDenyGrant ToolRule = "deny"
	// ToolRuleNotGranted marks a tool with no grant at all.
	ToolRuleNotGranted ToolRule = "not_granted"
	// ToolRuleChannel marks a grant whose channel restriction did not match.
	ToolRuleChannel ToolRule = "channel"
	// ToolRuleSensitiveLocality marks a sensitive tool on a cloud model.
	ToolRuleSensitiveLocality ToolRule = "sensitive_locality"
	// ToolRulePrivateShield marks a network-shield denial at dial time
	// (emitted by the execution layer, ADR-0041 §3; never by SelectTools).
	ToolRulePrivateShield ToolRule = "private_network_shield"
	// ToolRuleCage marks a per-tool cage denial (allow-list, cap, timeout;
	// emitted by the execution layer, ADR-0041 §4; never by SelectTools).
	ToolRuleCage ToolRule = "cage"
)

// ToolDecision is the per-tool outcome for one message.
type ToolDecision struct {
	// Mode is the effective mode after restrictions (restrictions apply
	// BEFORE the granted mode — the gate restricts, never widens).
	Mode ToolMode
	// Rule names why, when Mode is ToolDeny.
	Rule ToolRule
	// Shield arms the dial-time private-network guard: a network tool on a
	// Private brain (ADR-0041 §3). Selection only FLAGS it; enforcement is
	// the execution layer's.
	Shield bool
}

// ToolQuery is the per-message context SelectTools filters on.
type ToolQuery struct {
	// Channel is the inbound Envelope's channel name.
	Channel string
	// Sensitivity is the brain's declared tier (ADR-0015).
	Sensitivity Sensitivity
	// Locality is where the agent's single model runs (ADR-0015).
	Locality Locality
}

// SelectTools filters a brain's tool grants down to one ToolDecision per tool
// for the given per-message query. The result covers the UNION of grant names
// and attrs keys: a tool in attrs without a grant maps to ToolDeny /
// ToolRuleNotGranted; a granted tool absent from attrs gets zero attrs (the
// house default). Rule precedence, restrictions before mode (ADR-0041 §1):
// explicit deny → channel restriction → sensitive×cloud → the granted mode,
// with Shield flagged for network tools on a Private brain.
//
// It is PURE and deterministic: no I/O, inputs never mutated, same inputs →
// same decisions. Errors fail loud (construction-class misconfiguration,
// all %w-wrapped): ErrUnknownSensitivity / ErrUnknownLocality on a zero or
// unknown query value, ErrUnknownToolMode on a zero or unknown grant mode,
// ErrDuplicateToolGrant on a repeated grant name, ErrInvalidToolGrant on an
// empty one. On any error the returned map is nil.
func SelectTools(grants []ToolGrant, attrs map[string]ToolAttrs, q ToolQuery) (map[string]ToolDecision, error) {
	switch q.Sensitivity {
	case Public, Private:
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownSensitivity, int(q.Sensitivity))
	}
	switch q.Locality {
	case Local, Cloud:
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownLocality, int(q.Locality))
	}

	decisions := make(map[string]ToolDecision, len(grants)+len(attrs))
	for _, g := range grants {
		if g.Name == "" {
			return nil, fmt.Errorf("%w: empty tool name", ErrInvalidToolGrant)
		}
		if _, dup := decisions[g.Name]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateToolGrant, g.Name)
		}
		switch g.Mode {
		case ToolAllow, ToolShadow, ToolDeny:
		default:
			return nil, fmt.Errorf("%w: grant %q: %d", ErrUnknownToolMode, g.Name, int(g.Mode))
		}
		decisions[g.Name] = decideTool(g, attrs[g.Name], q)
	}
	for name := range attrs {
		if _, granted := decisions[name]; !granted {
			decisions[name] = ToolDecision{Mode: ToolDeny, Rule: ToolRuleNotGranted}
		}
	}
	return decisions, nil
}

// decideTool applies the precedence for one validated grant.
func decideTool(g ToolGrant, a ToolAttrs, q ToolQuery) ToolDecision {
	if g.Mode == ToolDeny {
		return ToolDecision{Mode: ToolDeny, Rule: ToolRuleDenyGrant}
	}
	if len(g.Channels) > 0 && !containsChannel(g.Channels, q.Channel) {
		return ToolDecision{Mode: ToolDeny, Rule: ToolRuleChannel}
	}
	if a.Sensitive && q.Locality == Cloud {
		return ToolDecision{Mode: ToolDeny, Rule: ToolRuleSensitiveLocality}
	}
	return ToolDecision{Mode: g.Mode, Shield: a.Network && q.Sensitivity == Private}
}

// containsChannel reports whether channel is in the restriction list
// (exact match — the same shape SessionPolicy.Triggers uses).
func containsChannel(channels []string, channel string) bool {
	for _, c := range channels {
		if c == channel {
			return true
		}
	}
	return false
}
