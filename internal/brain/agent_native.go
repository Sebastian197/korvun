// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"encoding/json"
	"errors"
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

// maxNativeCallsPerTurn caps how many tool calls ONE native model response
// may execute (estreno E-5 / red-team): the model — not the operator —
// controls the slice's length, and the text lane is structurally one call
// per iteration, so without this bound a single Handle could run
// maxIters × N governed executions with N unbounded. The slice is TRUNCATED
// before it enters the history (bounded memory and context too — re-review
// follow-up), and the discard is announced once, aggregated.
const maxNativeCallsPerTurn = 8

// MaxNativeCallsPerTurn exports the per-turn cap for the wiring layer: the
// ceiling derivation multiplies it into the agent's tool-time budget
// (re-review follow-up F4), so runtime and derivation share one constant.
const MaxNativeCallsPerTurn = maxNativeCallsPerTurn

// runLoopNative runs the bounded native model→tools→model loop. Same return
// contract as runLoop plus a degrade flag: the final answer and true, or ""
// and false (cap hit, model failure, ctx done). degraded is true only when
// the provider refused the tools protocol (ErrToolsUnsupported).
func (a *AgentBrain) runLoopNative(ctx context.Context, env *envelope.Envelope, req *model.Request, tcm model.ToolCallingModel, advertised tool.Registry, decisions map[string]policy.ToolDecision) (string, bool, bool) {
	specs := toToolSpecs(advertised)

	for iter := 0; iter < a.maxIters; iter++ {
		if err := ctx.Err(); err != nil {
			a.logger.Warn("agent: context done mid-loop (native)",
				"envelope_id", env.ID, "channel", env.Channel, "iter", iter, "cause", err)
			return "", false, false
		}

		resp, latency, err := a.callNative(ctx, tcm, req, specs)
		a.metrics.ObserveProviderDuration(tcm.Name(), err == nil, latency)
		if err != nil {
			// A capability refusal degrades (the caller retries on the text
			// lane); it is NOT counted as a provider failure — the model is
			// healthy, it just lacks the tools protocol.
			if errors.Is(err, model.ErrToolsUnsupported) {
				return "", false, true
			}
			a.metrics.IncProviderFailure(tcm.Name())
			a.logger.Warn("agent: native model call failed",
				"envelope_id", env.ID, "channel", env.Channel, "iter", iter, "cause", err)
			return "", false, false
		}

		if len(resp.Message.ToolCalls) == 0 {
			content := resp.Message.Content
			if content == "" {
				a.logger.Warn("agent: empty native reply, no answer",
					"envelope_id", env.ID, "channel", env.Channel, "iter", iter)
				return "", false, false
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
				// Blank the printed JSON before the turn is replayed into the
				// context (estreno E-5 / red-team): leaving it as the model's
				// "own words" invites it to print the syntax again — the exact
				// failure mode the clean-context fix targeted. An assistant
				// turn carrying ToolCalls is valid words-free (ADR-0042 §3).
				resp.Message.Content = ""
			} else {
				return content, true, false // final answer
			}
		}

		// The model wants tools: append its own turn, then run every call IN
		// ORDER through the SAME runTool the prompt-protocol lane uses —
		// governance, shadow, cages, shield, and audit are one code path.
		// Each result returns as a RoleTool turn per the verified contract.
		// The model, not the operator, controls how many calls one response
		// carries. TRUNCATE before the message enters the history (re-review
		// follow-up of E-5): appending the full slice kept O(N) model-
		// controlled memory and replayed it as context on every following
		// call. The discard is announced once, aggregated, and audited with
		// a FINITE label.
		discarded := 0
		if len(resp.Message.ToolCalls) > maxNativeCallsPerTurn {
			discarded = len(resp.Message.ToolCalls) - maxNativeCallsPerTurn
			resp.Message.ToolCalls = resp.Message.ToolCalls[:maxNativeCallsPerTurn]
		}
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
			// An empty observation (http_fetch on a 200-empty body, read_file
			// of a 0-byte file) must not become an empty RoleTool turn: the
			// next real-adapter call would refuse the WHOLE request
			// (ValidateRequest ErrEmptyContent) and the user would get the
			// canned fallback for a tool that actually worked (estreno E-4).
			if observation == "" {
				observation = "(the tool returned an empty result)"
			}
			req.Messages = append(req.Messages, model.Message{
				Role:     model.RoleTool,
				ToolName: call.Name,
				Content:  observation,
			})
		}
		if discarded > 0 {
			// ONE aggregated announcement for the whole discard: honest to
			// the model, audited on the shared surfaces with a FINITE label
			// (the raw names are model-controlled), counted in the log.
			a.logger.Warn("agent: per-turn tool budget exceeded",
				"envelope_id", env.ID, "channel", env.Channel,
				"discarded", discarded, "cap", maxNativeCallsPerTurn)
			a.auditTool(ctx, env, bus.Event{Type: bus.ToolDenied, Tool: "overflow", Outcome: "denied", Rule: "per_turn_budget"})
			req.Messages = append(req.Messages, model.Message{
				Role:     model.RoleTool,
				ToolName: "budget",
				Content: fmt.Sprintf("%d tool call(s) skipped: per-turn tool budget exceeded (%d calls max per reply)",
					discarded, maxNativeCallsPerTurn),
			})
		}
	}

	a.logger.Warn("agent: iteration cap reached without an answer (native)",
		"envelope_id", env.ID, "channel", env.Channel, "max_iters", a.maxIters)
	return "", false, false
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
