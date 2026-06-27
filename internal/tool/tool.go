// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package tool owns the agent Tool seam (ADR-0021 §4): the interface an
// AgentBrain invokes mid-reasoning, plus the minimal, PURE built-in tools the
// Stage 8 seam-validation slice ships. It is a leaf — it imports only the
// standard library, so a tool can be defined without importing brain, model, or
// envelope, the same seam discipline conversation.Store and metrics.Metrics
// carry.
package tool

import "context"

// Tool is one external capability an AgentBrain may invoke mid-reasoning
// (ADR-0021 §4).
//
// CONCURRENCY CONTRACT (load-bearing): an implementation MUST be safe for
// concurrent Execute calls on a SINGLE instance. The router runs N brain workers
// (ADR-0003) over ONE shared AgentBrain (ADR-0014 §4, ADR-0021 §5), so two
// workers may call the same Tool instance at the same time. This is the same
// discipline model.Model and conversation.Store already carry. The built-in
// tools (Time, Echo, Calc) are PURE and therefore trivially safe; a future
// stateful tool (a counter, a cache) OWNS its own synchronization.
type Tool interface {
	// Name is the protocol identifier the model uses to call the tool
	// (the <name> in "TOOL: <name>(<args>)", ADR-0021 §3).
	Name() string
	// Description is the one-line capability advertised to the model in the
	// system prompt (ADR-0021 §3.1).
	Description() string
	// Execute runs the tool. args is the raw string from the protocol
	// (ADR-0021 §3.2), parsed by the tool itself. ctx is the per-tool-bounded
	// context (ADR-0021 §2). A returned error becomes an OBSERVATION fed back to
	// the model (ADR-0021 §2), never a loop-killing panic.
	Execute(ctx context.Context, args string) (string, error)
}

// Registry maps a tool name to its Tool. It is injected into an AgentBrain at
// construction (ADR-0021 §4); a bad tool is removed by simply not registering
// it. The map is read-only after construction, so it is safe to share across the
// router's workers as long as every Tool honors the concurrency contract above.
type Registry map[string]Tool
