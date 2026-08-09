// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/ollama"
	"github.com/Sebastian197/korvun/internal/model/retry"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// LIVE smoke of the native lane against a REAL local model (the 2026-08-09
// house law: model-dependent behavior needs a real model inside its own
// sub-phase — fakes prove our code, only a real model proves the contract).
// Opt-in like the keyring_live pattern: set KORVUN_LIVE_OLLAMA=1 with a
// local Ollama serving llama3.2 (capability: tools). Skipped otherwise, so
// CI stays hermetic. The model is wired through the PRODUCTION chain —
// retry(adapter) → WithModelID — so capability propagation is exercised
// end to end, not just asserted.

func liveProductionModel(t *testing.T) model.Model {
	t.Helper()
	if os.Getenv("KORVUN_LIVE_OLLAMA") == "" {
		t.Skip("live smoke; set KORVUN_LIVE_OLLAMA=1 with a local Ollama running llama3.2")
	}
	decorated := retry.New(ollama.New(), retry.Config{PerAttempt: 90 * time.Second, MaxRetries: 1})
	return WithModelID(decorated, "llama3.2")
}

func TestLive_nativeLane_readFileThroughTheJail(t *testing.T) {
	m := liveProductionModel(t)
	if _, ok := m.(model.ToolCallingModel); !ok {
		t.Fatal("production chain lost the tool-calling capability")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "nota.txt"),
		[]byte("La palabra clave es ALMENDRA."), 0o600); err != nil {
		t.Fatal(err)
	}
	rf, err := tool.ReadFile(tool.ReadFileConfig{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	pub := &spyPublisher{}
	a := NewAgentBrain(m, tool.Registry{"read_file": rf},
		WithAgentLogger(quietLogger()), WithAgentToolAudit(pub, "live"))

	out, err := a.Handle(context.Background(),
		inboundText("console", "live", "Lee el fichero nota.txt con la herramienta read_file y dime la palabra clave."))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("replies = %d, want 1", len(out))
	}
	reply := out[0].Parts[0].Content
	if strings.TrimSpace(reply) == "" {
		t.Fatal("empty final reply")
	}
	// THE contract under test (deterministic): the real model DRIVES the
	// jailed tool through the native lane and the use audits. Whether its
	// closing prose repeats the keyword verbatim is model wording — logged,
	// not asserted (measured 2026-08-09: tool fired 6/6; keyword phrasing
	// varied).
	events := pub.snapshot()
	if len(events) == 0 || events[0].Tool != "read_file" || events[0].Outcome != "ok" {
		t.Fatalf("audit = %+v, want a tool_used ok for read_file", events)
	}
	if !strings.Contains(strings.ToUpper(reply), "ALMENDRA") {
		t.Logf("note: keyword not verbatim in the closing prose (model wording): %q", reply)
	}
}

func TestLive_nativeLane_shadowNeverExecutesWithARealModel(t *testing.T) {
	m := liveProductionModel(t)
	spy := &spyTool{}
	pub := &spyPublisher{}
	g := governanceFor(
		[]policy.ToolGrant{{Name: "spy", Mode: policy.ToolShadow}},
		nil, policy.Public, policy.Local)
	a := NewAgentBrain(m, spyRegistry(spy),
		WithAgentLogger(quietLogger()), WithAgentToolAudit(pub, "live"),
		WithAgentGovernance(g),
		WithAgentSystemPrompt("When asked to use the spy tool, call it."))

	out, err := a.Handle(context.Background(),
		inboundText("console", "live2", "Usa la herramienta spy con el argumento hola."))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if spy.count() != 0 {
		t.Fatalf("shadowed tool executed %d times with a real model, want 0", spy.count())
	}
	if len(out) != 1 || out[0].Parts[0].Content == "" {
		t.Fatalf("no usable final answer: %+v", out)
	}
	for _, ev := range pub.snapshot() {
		if ev.Type.String() == "tool_used" {
			t.Fatalf("a shadowed grant produced a tool_used event: %+v", ev)
		}
	}
}
