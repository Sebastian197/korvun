// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// This file is the AgentBrain's native tool-calling loop (ADR-0042 §5). The
// governance surface is IDENTICAL to the prompt-protocol lane BY
// CONSTRUCTION: the advertised registry (allow ∪ shadow, already filtered by
// the gate) becomes the spec catalog, and every returned tool_call routes
// through the SAME runTool — two-point gate, shadow simulation, cages,
// dial-time shield, and the three-surface metadata-only audit included.

// runLoopNative runs the bounded native model→tools→model loop. Same return
// contract as runLoop: the final answer and true, or "" and false (cap hit,
// model failure, ctx done → the caller degrades to the fallback).
func (a *AgentBrain) runLoopNative(ctx context.Context, env *envelope.Envelope, req *model.Request, tcm model.ToolCallingModel, advertised tool.Registry, decisions map[string]policy.ToolDecision) (string, bool) {
	specs := toToolSpecs(advertised)

	for iter := 0; iter < a.maxIters; iter++ {
		if err := ctx.Err(); err != nil {
			a.logger.Warn("agent: context done mid-loop (native)",
				"envelope_id", env.ID, "channel", env.Channel, "iter", iter, "cause", err)
			return "", false
		}

		resp, latency, err := a.callNative(ctx, tcm, req, specs)
		a.metrics.ObserveProviderDuration(tcm.Name(), err == nil, latency)
		if err != nil {
			a.metrics.IncProviderFailure(tcm.Name())
			a.logger.Warn("agent: native model call failed",
				"envelope_id", env.ID, "channel", env.Channel, "iter", iter, "cause", err)
			return "", false
		}

		if len(resp.Message.ToolCalls) == 0 {
			content := resp.Message.Content
			if content == "" {
				a.logger.Warn("agent: empty native reply, no answer",
					"envelope_id", env.ID, "channel", env.Channel, "iter", iter)
				return "", false
			}
			return content, true // final answer
		}

		// The model wants tools: append its own turn, then run every call IN
		// ORDER through the SAME runTool the prompt-protocol lane uses —
		// governance, shadow, cages, shield, and audit are one code path.
		// Each result returns as a RoleTool turn per the verified contract.
		req.Messages = append(req.Messages, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			observation := a.runTool(ctx, env, decisions, call.Name, nativeArgs(call))
			req.Messages = append(req.Messages, model.Message{
				Role:     model.RoleTool,
				ToolName: call.Name,
				Content:  observation,
			})
		}
	}

	a.logger.Warn("agent: iteration cap reached without an answer (native)",
		"envelope_id", env.ID, "channel", env.Channel, "max_iters", a.maxIters)
	return "", false
}

// callNative is one native model step with the ADR-0011 per-call discipline
// the loop needs locally: wall-clock latency from the injected clock and a
// panic recovered into an error (a panicking adapter must not kill the
// router worker).
func (a *AgentBrain) callNative(ctx context.Context, tcm model.ToolCallingModel, req *model.Request, specs []model.ToolSpec) (resp *model.Response, latency time.Duration, err error) {
	start := a.now()
	defer func() {
		latency = a.now().Sub(start)
		if r := recover(); r != nil {
			resp, err = nil, fmt.Errorf("agent: native model panicked: %v", r)
		}
	}()
	resp, err = tcm.GenerateWithTools(ctx, req, specs)
	if err == nil && resp == nil {
		err = fmt.Errorf("agent: native model returned nil response")
	}
	return resp, 0, err // latency is set by the deferred stamp
}

// toToolSpecs renders the ADVERTISED registry (the gate's output — a denied
// tool never reaches here) as the model's spec catalog, sorted by name so
// the request is deterministic like the textual catalog was.
func toToolSpecs(reg tool.Registry) []model.ToolSpec {
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)
	specs := make([]model.ToolSpec, len(names))
	for i, n := range names {
		specs[i] = model.ToolSpec{Name: reg[n].Name(), Description: reg[n].Description()}
	}
	return specs
}

// nativeArgs extracts the Tool seam's args string from a native call: the
// uniform v1 schema puts it at Arguments["args"]; anything else is
// re-serialized verbatim — the tool owns parsing, a mismatch is an ordinary
// tool error (ADR-0042 §5).
func nativeArgs(call model.ToolCall) string {
	if s, ok := call.Arguments["args"].(string); ok {
		return s
	}
	if len(call.Arguments) == 0 {
		return ""
	}
	raw, err := json.Marshal(call.Arguments)
	if err != nil {
		return fmt.Sprintf("%v", call.Arguments)
	}
	return string(raw)
}
