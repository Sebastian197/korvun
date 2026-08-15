// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"errors"
	"fmt"
	"net/netip"
	"syscall"
)

// This file owns the shared surface of the CAGED built-in tools (ADR-0041 §4)
// — the tools with effects the piece adds behind the §8 bar. A caged tool
// MUST NOT exist without its operator cage, so it never resolves through the
// config-less Builtin(name); it is constructed via its typed constructor
// (ReadFile, HTTPFetch, WebhookCall) at wiring time in internal/app. The
// safe-toolset boundary stays in exactly this package: the constructors here
// and the name/attrs catalog below are its single source of truth.

// ErrCageViolation marks a tool error caused by the tool's own cage — a path
// outside the jail root, a host off the allow-list, a response over the size
// cap. The AgentBrain classifies an Execute error wrapping it as a DENIAL
// (audit rule "cage", ADR-0041 §5) rather than an executed-with-error use.
// The observation fed back to the model is the tool's honest error text; the
// classification affects only the audit surfaces.
var ErrCageViolation = errors.New("tool: cage violation")

// ErrShieldViolation marks a connection attempt the private-network shield
// stopped at the dial (ADR-0041 §3): a Private brain's network tool resolved
// a public address. Classified as a DENIAL with audit rule
// "private_network_shield". Like ErrCageViolation, nothing was contacted.
var ErrShieldViolation = errors.New("tool: private network shield violation")

// Attrs are the HOUSE-DEFAULT gate attributes of a built-in tool (ADR-0041
// §4, R-2): the declared inputs the policy gate routes on. The operator may
// override them in config (SP5); this catalog is the default the wiring
// starts from. Kept as plain bools so this package stays a leaf (the policy
// package translates them into its own declared types).
type Attrs struct {
	// Sensitive restricts the tool to locally-running models.
	Sensitive bool
	// Network marks a tool that reaches the network (shield-subject).
	Network bool
}

// BuiltinAttrs returns the house-default attributes of a built-in tool by its
// protocol name — pure and caged alike — and ok=false for any name outside
// the safe toolset (a dangerous name like "shell" is not known, by decision
// ADR-0021 §8). This is the attrs half of the single safe-toolset boundary;
// a forgotten declaration here is exactly what the spec's SP3 attrs tripwire
// tests guard against.
func BuiltinAttrs(name string) (Attrs, bool) {
	switch name {
	case "time", "echo", "calc":
		return Attrs{}, true
	case "read_file":
		return Attrs{Sensitive: true}, true
	case "http_fetch", "webhook_call":
		return Attrs{Network: true}, true
	default:
		return Attrs{}, false
	}
}

// ToolParam is one structured field a ParamTool advertises to native
// tool-calling models (the 2026-08-09 demo lesson: a small model cannot
// compose "URL space JSON" into one string, but fills separate fields
// reliably). All params are strings in v1.
type ToolParam struct {
	// Name is the field name the model fills.
	Name string
	// Description tells the model what goes in the field.
	Description string
	// Required marks the field as mandatory in the advertised schema.
	Required bool
}

// ParamTool is the OPTIONAL structured-params capability of a Tool: the
// native lane advertises Params() as the tool's schema and reconstructs the
// Tool seam's args string through ArgsFromCall, so Execute's contract —
// and therefore the whole gate/cage path — stays single and untouched. A
// tool without ParamTool keeps the uniform {"args": string} schema.
type ParamTool interface {
	Tool
	// Params declares the structured fields, in advertisement order.
	Params() []ToolParam
	// ArgsFromCall reconstructs the seam args from the model's field
	// values. It validates TOLERANTLY and returns USEFUL errors naming the
	// missing/broken field — the error becomes a model-facing observation.
	ArgsFromCall(fields map[string]any) (string, error)
}

// shieldControl is the private-network shield's dial-time check (ADR-0041
// §3), installed as net.Dialer.Control on a Private brain's network tools.
// It runs on EVERY connection attempt with the RESOLVED address — after DNS —
// so a rebound hostname or an off-shield redirect dies at the socket, before
// a single byte leaves. Private means: IPv4 loopback/RFC1918, IPv6
// loopback/ULA, with IPv4-mapped-in-IPv6 forms unmapped before
// classification. Link-local unicast is deliberately EXCLUDED (estreno E-8):
// 169.254.169.254 / fe80::/10 includes the cloud metadata service — a
// credential-theft SSRF target a rebound allow-listed hostname could reach
// under the shield; genuine link-local use is exotic enough to refuse. An
// unparseable address fails CLOSED.
func shieldControl(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("shield: unparseable dial address %q: %w", address, ErrShieldViolation)
	}
	a := ap.Addr().Unmap()
	if a.IsLoopback() || a.IsPrivate() {
		return nil
	}
	return fmt.Errorf("shield: %s is not a private address: %w", a, ErrShieldViolation)
}
