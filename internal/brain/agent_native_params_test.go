// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Structured params on the native lane (the demo lesson): a ParamTool's
// fields ride in the spec, the call's fields are reconstructed through
// ArgsFromCall, and the SAME runTool gate still runs on the result.

// paramSpyTool is a ParamTool recording the reconstructed args it executed.
type paramSpyTool struct {
	mu   sync.Mutex
	args []string
}

func (p *paramSpyTool) Name() string        { return "notify" }
func (p *paramSpyTool) Description() string { return "sends a notice. Use the url and message fields." }
func (p *paramSpyTool) Execute(_ context.Context, args string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.args = append(p.args, args)
	return "sent", nil
}
func (p *paramSpyTool) Params() []tool.ToolParam {
	return []tool.ToolParam{
		{Name: "url", Description: "the webhook URL", Required: true},
		{Name: "message", Description: "the plain-text notice", Required: true},
	}
}
func (p *paramSpyTool) ArgsFromCall(fields map[string]any) (string, error) {
	u, _ := fields["url"].(string)
	m, _ := fields["message"].(string)
	return u + " | " + m, nil
}

func (p *paramSpyTool) executed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.args))
	copy(out, p.args)
	return out
}

func TestNativeLane_paramToolSpecCarriesFields(t *testing.T) {
	t.Parallel()
	spy := &paramSpyTool{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply("done")}}
	a := NewAgentBrain(m, tool.Registry{"notify": spy}, WithAgentLogger(quietLogger()))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "hola")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(m.lastSpecs) != 1 {
		t.Fatalf("specs = %+v, want 1", m.lastSpecs)
	}
	params := m.lastSpecs[0].Params
	if len(params) != 2 || params[0].Name != "url" || !params[0].Required {
		t.Fatalf("spec params = %+v, want the tool's declared fields", params)
	}
}

func TestNativeLane_paramCallReconstructsArgs(t *testing.T) {
	t.Parallel()
	spy := &paramSpyTool{}
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{
			Name:      "notify",
			Arguments: map[string]any{"url": "http://127.0.0.1:8765/aviso", "message": "hola"},
		}}},
		finalReply("done"),
	}}
	a := NewAgentBrain(m, tool.Registry{"notify": spy}, WithAgentLogger(quietLogger()))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "avisa")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := spy.executed()
	if len(got) != 1 || got[0] != "http://127.0.0.1:8765/aviso | hola" {
		t.Fatalf("executed args = %+v, want the reconstructed field join", got)
	}
}

// The gate still rules a ParamTool: a shadow grant never executes even with
// perfect structured fields.
func TestNativeLane_paramToolStillGated(t *testing.T) {
	t.Parallel()
	spy := &paramSpyTool{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "notify", Mode: policy.ToolShadow}},
		nil, policy.Public, policy.Local)
	m := &nativeScriptedModel{name: "n", replies: []model.Message{
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{
			Name:      "notify",
			Arguments: map[string]any{"url": "http://x/", "message": "hola"},
		}}},
		finalReply("done"),
	}}
	a := NewAgentBrain(m, tool.Registry{"notify": spy},
		WithAgentLogger(quietLogger()), WithAgentGovernance(g))

	if _, err := a.Handle(context.Background(), inboundText("console", "c", "avisa")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if n := len(spy.executed()); n != 0 {
		t.Fatalf("shadowed ParamTool executed %d times, want 0", n)
	}
}

// The hardened simulation text (mandate 4a): unmistakable, and it forbids
// the manual-offer failure mode seen in the demo.
func TestShadowObservation_hardenedText(t *testing.T) {
	t.Parallel()
	got := shadowObservation("webhook_call")
	for _, want := range []string{
		"REHEARSAL",
		"shadow mode",
		"NOT executed by design",
		"not an error, not a failure",
		"simulated, not performed",
		"do NOT offer to do it manually",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("simulation text missing %q:\n%s", want, got)
		}
	}
}
