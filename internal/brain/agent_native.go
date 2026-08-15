// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
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
			// Small models sometimes print the call as their FINAL text
			// instead of emitting it natively — verified twice against a
			// real 3B on 2026-08-09, base instruction notwithstanding. That
			// blob must never reach the user: when the whole reply is a
			// tool-call-shaped JSON object naming a REGISTERED tool, rescue
			// it through the SAME governed runTool (audit + honest
			// observation) and let the loop continue so the model authors a
			// real answer.
			if call, rescued := rescueTextToolCall(a.tools, content); rescued {
				a.logger.Info("agent: rescued tool call printed as text",
					"envelope_id", env.ID, "channel", env.Channel, "tool", call.Name, "iter", iter)
				resp.Message.ToolCalls = []model.ToolCall{call}
			} else {
				return content, true // final answer
			}
		}

		// The model wants tools: append its own turn, then run every call IN
		// ORDER through the SAME runTool the prompt-protocol lane uses —
		// governance, shadow, cages, shield, and audit are one code path.
		// Each result returns as a RoleTool turn per the verified contract.
		req.Messages = append(req.Messages, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			var observation string
			if gated := decisions != nil && func() bool {
				d, ok := decisions[call.Name]
				return !ok || d.Mode != policy.ToolAllow
			}(); gated {
				// Governance rules BEFORE argument parsing (estreno E-3): a
				// shadowed/denied call must produce the gate's observation
				// and audit even when its args would not parse — a field
				// error must never smuggle the attempt past the gate.
				observation = a.runTool(ctx, env, decisions, call.Name, nativeArgs(call))
			} else if args, argErr := a.nativeCallArgs(advertised, call); argErr != nil {
				// A useful, model-facing field error (the ParamTool contract)
				// — same failure class as a tool error, loop stays alive, and
				// the attempt is AUDITED like any errored use (closes the
				// known ParamTool audit gap).
				a.logger.Warn("agent: tool args rejected",
					"envelope_id", env.ID, "channel", env.Channel, "tool", call.Name, "cause", argErr)
				a.auditTool(ctx, env, bus.Event{Type: bus.ToolUsed, Tool: call.Name, Outcome: "error"})
				observation = fmt.Sprintf("tool %s failed: %v", call.Name, argErr)
			} else {
				observation = a.runTool(ctx, env, decisions, call.Name, args)
			}
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
// the request is deterministic like the textual catalog was. A ParamTool's
// declared fields ride as the spec's structured schema.
func toToolSpecs(reg tool.Registry) []model.ToolSpec {
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)
	specs := make([]model.ToolSpec, len(names))
	for i, n := range names {
		spec := model.ToolSpec{Name: reg[n].Name(), Description: reg[n].Description()}
		if pt, ok := reg[n].(tool.ParamTool); ok {
			for _, p := range pt.Params() {
				spec.Params = append(spec.Params, model.ToolParamSpec{
					Name: p.Name, Description: p.Description, Required: p.Required,
				})
			}
		}
		specs[i] = spec
	}
	return specs
}

// nativeCallArgs resolves the seam args for one native call: a ParamTool
// reconstructs them from its structured fields (ArgsFromCall — tolerant,
// useful errors); everything else takes the uniform Arguments["args"], or a
// verbatim re-serialization as the last resort (the tool owns parsing).
func (a *AgentBrain) nativeCallArgs(advertised tool.Registry, call model.ToolCall) (string, error) {
	if t, ok := advertised[call.Name]; ok {
		if pt, isParam := t.(tool.ParamTool); isParam {
			return pt.ArgsFromCall(call.Arguments)
		}
	}
	return nativeArgs(call), nil
}

// rescueTextToolCall recognises a final reply that is EXACTLY one
// tool-call-shaped JSON object — {"name": <registered tool>, and one of
// "parameters"/"arguments"/"args"} — and converts it to a ToolCall for the
// governed path. Anything else (including JSON naming no registered tool)
// is an ordinary answer and passes through untouched.
func rescueTextToolCall(reg tool.Registry, content string) (model.ToolCall, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return model.ToolCall{}, false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return model.ToolCall{}, false
	}
	name, ok := obj["name"].(string)
	if !ok {
		return model.ToolCall{}, false
	}
	if _, registered := reg[name]; !registered {
		return model.ToolCall{}, false
	}
	for _, key := range []string{"arguments", "parameters", "args"} {
		raw, present := obj[key]
		if !present {
			continue
		}
		switch v := raw.(type) {
		case map[string]any:
			return model.ToolCall{Name: name, Arguments: v}, true
		case string:
			return model.ToolCall{Name: name, Arguments: map[string]any{"args": v}}, true
		default:
			return model.ToolCall{Name: name, Arguments: map[string]any{}}, true
		}
	}
	return model.ToolCall{}, false
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
