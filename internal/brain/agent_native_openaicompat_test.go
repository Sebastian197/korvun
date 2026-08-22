// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

// AS-B-4 (ADR-0044 SP-B): the FULL agent loop over the REAL openaicompat
// adapter against an httptest compat server scripting call → observation
// → final answer — the ollama native-test mold, at the brain level. No
// cycle: this package never imports openaicompat outside tests.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/openaicompat"
	"github.com/Sebastian197/korvun/internal/tool"
)

// recordingCalc is a trivial tool that records the args it ran with.
type recordingCalc struct {
	mu   sync.Mutex
	args []string
}

func (c *recordingCalc) Name() string        { return "calc" }
func (c *recordingCalc) Description() string { return "evaluate an arithmetic expression" }
func (c *recordingCalc) Execute(_ context.Context, args string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.args = append(c.args, args)
	return "4", nil
}
func (c *recordingCalc) executed() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.args...)
}

// TestNativeLane_openaicompatFullLoop drives one inbound message through
// an AgentBrain wired with the REAL adapter: the scripted server first
// returns a calls-bearing reply (id "call_1", STRING-JSON arguments),
// then asserts the second request replays the assistant turn AND carries
// the role:"tool" turn whose tool_call_id equals the call's id (FR-GWB-1
// threading proven at the wire), and finally answers.
func TestNativeLane_openaicompatFullLoop(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		requests = append(requests, body)
		n := len(requests)
		mu.Unlock()

		if n == 1 {
			_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"calc","arguments":"{\"args\":\"2+2\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"the answer is 4"}}]}`))
	}))
	t.Cleanup(srv.Close)

	adapter, err := openaicompat.New(openaicompat.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	calc := &recordingCalc{}
	a := NewAgentBrain(adapter, tool.Registry{calc.Name(): calc}, WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("console", "c-1", "what is 2+2?"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) == 0 || len(out[0].Parts) == 0 || !strings.Contains(out[0].Parts[0].Content, "the answer is 4") {
		t.Errorf("final answer = %+v, want the scripted final reply", out)
	}
	if got := calc.executed(); len(got) != 1 || got[0] != "2+2" {
		t.Errorf("calc executions = %v, want exactly one with args 2+2", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("server requests = %d, want 2 (call round + final round)", len(requests))
	}
	msgs, _ := requests[1]["messages"].([]any)
	var sawReplay, sawToolTurn bool
	for _, raw := range msgs {
		m, _ := raw.(map[string]any)
		if calls, ok := m["tool_calls"].([]any); ok && len(calls) == 1 && m["role"] == "assistant" {
			cw, _ := calls[0].(map[string]any)
			if cw["id"] == "call_1" {
				sawReplay = true
			}
		}
		if m["role"] == "tool" {
			sawToolTurn = true
			if m["tool_call_id"] != "call_1" {
				t.Errorf("tool turn tool_call_id = %v, want call_1 (FR-GWB-1: the agent threads the id)", m["tool_call_id"])
			}
			if m["content"] != "4" {
				t.Errorf("tool turn content = %v, want the observation 4", m["content"])
			}
		}
	}
	if !sawReplay {
		t.Error("second request did not replay the assistant calls-bearing turn (id call_1)")
	}
	if !sawToolTurn {
		t.Error("second request carried no role:\"tool\" turn")
	}
	if _, ok := interface{}(adapter).(model.ToolCallingModel); !ok {
		t.Fatal("adapter does not satisfy ToolCallingModel — the loop above cannot have used the native lane")
	}
}
