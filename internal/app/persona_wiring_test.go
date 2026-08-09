// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// SP1 RED (builder-canvas FR-PERSONA-2 wiring, NC-4 resolved): the factory
// passes brains[i].persona from config to the constructed brain — Orchestrator
// via WithSystemPrompt(ComposePersona), AgentBrain via the persona PREFIX. The
// proof is end-to-end at the provider boundary (the retry_wiring_test.go
// pattern): a fake Ollama endpoint captures the /api/chat request and the
// tests assert on the system message it carries. Without a persona the wire
// request is EXACTLY today's (no system message) — the regression guard.
//
// RED note (house precedent, coordinator_carveout_test.go): config.PersonaConfig,
// BrainConfig.Persona, brain.Persona and brain.ComposePersona do not exist yet —
// the compile failure IS the red.

// chatCapture is a fake Ollama /api/chat endpoint recording every request's
// role-tagged messages and answering a fixed final reply (no TOOL: line, so an
// agent loop ends on iteration 1).
type chatCapture struct {
	mu   sync.Mutex
	seen [][]struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
}

func (c *chatCapture) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		c.mu.Lock()
		c.seen = append(c.seen, body.Messages)
		c.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama3.2",
			"message": map[string]string{"role": "assistant", "content": "hi there"},
			"done":    true,
		})
	}
}

func (c *chatCapture) first() []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.seen) == 0 {
		return nil
	}
	return c.seen[0]
}

// testPersona is the config-side persona used across these wiring tests;
// wantComposed derives the expected fragment from the SAME source of truth the
// factory must use (brain.ComposePersona), not a duplicated literal.
func testPersona() *config.PersonaConfig {
	return &config.PersonaConfig{
		DisplayName:  "Nova",
		Tone:         "warm, concise",
		Language:     "es-ES",
		Instructions: "Never reveal internal tooling.",
	}
}

func wantComposed() string {
	return brain.ComposePersona(&brain.Persona{
		DisplayName:  "Nova",
		Tone:         "warm, concise",
		Language:     "es-ES",
		Instructions: "Never reveal internal tooling.",
	})
}

// personaOrchCfg is a one-fanout-brain config over a single Ollama model,
// observability off, with an optional persona block.
func personaOrchCfg(baseURL string, p *config.PersonaConfig) *config.Config {
	return &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{{
			Name:        "default",
			Sensitivity: "public",
			Dispatch:    "fanout",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: "2s"},
			},
			Persona: p,
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "default"}},
	}
}

// personaAgentCfg mounts the same brain as a tool-use agent (one builtin tool,
// an operator system prompt) with an optional persona block.
func personaAgentCfg(baseURL string, p *config.PersonaConfig) *config.Config {
	cfg := personaOrchCfg(baseURL, p)
	cfg.Brains[0].Agent = &config.AgentConfig{
		Tools:        []string{"time"},
		SystemPrompt: "Operator rules.",
	}
	return cfg
}

// runAndAsk boots the app over cfg, feeds one inbound text message, and blocks
// until the reply lands (the capture endpoint has seen the request by then).
func runAndAsk(t *testing.T, cfg *config.Config) {
	t.Helper()
	ch := newCapturingChannel("telegram")
	a, err := Build(cfg, withChannelFactory(okFactory(ch)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- a.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-runDone
		shutdownApp(t, a)
	})

	deadline := time.Now().Add(time.Second)
	for !ch.isStarted() && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !ch.isStarted() {
		t.Fatal("Run did not start the channel")
	}

	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u-1"})
	in.AddText("hello")
	in.Meta[router.MetaConversationID] = "c-1"
	ch.inbound <- in

	select {
	case <-ch.sent:
	case <-time.After(8 * time.Second):
		t.Fatal("no reply sent within 8s")
	}
}

// TestBuild_orchestratorPersonaReachesProviderRequest (AS-PERSONA-2): with a
// persona configured, the request on the wire opens with a system message
// carrying EXACTLY the ComposePersona output — the factory maps the config
// block and wires it through WithSystemPrompt.
func TestBuild_orchestratorPersonaReachesProviderRequest(t *testing.T) {
	capture := &chatCapture{}
	srv := httptest.NewServer(capture.handler())
	t.Cleanup(srv.Close)

	runAndAsk(t, personaOrchCfg(srv.URL, testPersona()))

	msgs := capture.first()
	if len(msgs) != 2 || msgs[0].Role != "system" {
		t.Fatalf("wire messages = %+v, want [system, user]", msgs)
	}
	if msgs[0].Content != wantComposed() {
		t.Errorf("wire system prompt = %q, want %q", msgs[0].Content, wantComposed())
	}
}

// TestBuild_orchestratorNoPersona_wireUnchanged (AS-PERSONA-1): without a
// persona the wire request is EXACTLY today's — no system message at all.
func TestBuild_orchestratorNoPersona_wireUnchanged(t *testing.T) {
	capture := &chatCapture{}
	srv := httptest.NewServer(capture.handler())
	t.Cleanup(srv.Close)

	runAndAsk(t, personaOrchCfg(srv.URL, nil))

	msgs := capture.first()
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Errorf("wire messages = %+v, want exactly [user] (no persona → no system message)", msgs)
	}
}

// TestBuild_agentPersonaPrefixesProtocol (AS-PERSONA-3): on an agent brain the
// wire system message is the composed persona, then the ADR-0021 protocol
// block INTACT (grammar before catalog before the operator prompt), nothing
// overwritten — both contributions present.
func TestBuild_agentPersonaPrefixesProtocol(t *testing.T) {
	capture := &chatCapture{}
	srv := httptest.NewServer(capture.handler())
	t.Cleanup(srv.Close)

	runAndAsk(t, personaAgentCfg(srv.URL, testPersona()))

	msgs := capture.first()
	if len(msgs) == 0 || msgs[0].Role != "system" {
		t.Fatalf("wire messages = %+v, want a leading system message", msgs)
	}
	sys := msgs[0].Content
	if !strings.HasPrefix(sys, wantComposed()) {
		t.Errorf("system prompt does not open with the composed persona:\n%q", sys)
	}
	// ADR-0042 §5 updated this contract: the wired Ollama adapter carries the
	// native tool-calling capability through retry + WithModelID, so the
	// production agent seeds the NATIVE prompt — persona prefix + operator
	// prompt, NO textual grammar/catalog (the tools ride as structured specs).
	if strings.Contains(sys, "You can use tools.") || strings.Contains(sys, "Available tools:") {
		t.Errorf("native lane leaked the textual protocol block:\n%q", sys)
	}
	iOperator := strings.Index(sys, "Operator rules.")
	if iOperator < 0 || iOperator < len(wantComposed()) {
		t.Errorf("operator prompt missing or not after the persona prefix:\n%q", sys)
	}
}

// TestBuild_agentNoPersona_wireUnchanged: an agent brain without a persona
// seeds EXACTLY today's protocol prompt — it must NOT open with a persona
// fragment and the grammar must be the first line.
func TestBuild_agentNoPersona_wireUnchanged(t *testing.T) {
	capture := &chatCapture{}
	srv := httptest.NewServer(capture.handler())
	t.Cleanup(srv.Close)

	runAndAsk(t, personaAgentCfg(srv.URL, nil))

	msgs := capture.first()
	if len(msgs) == 0 || msgs[0].Role != "system" {
		t.Fatalf("wire messages = %+v, want a leading system message", msgs)
	}
	// ADR-0042 §5 updated this contract: with the wired (native-capable)
	// Ollama adapter, no persona means the seed system prompt is EXACTLY the
	// operator prompt — no persona fragment, no textual grammar (the tools
	// ride as structured specs on the request).
	if !strings.HasPrefix(msgs[0].Content, "Operator rules.") {
		t.Errorf("system prompt = %q, want it to OPEN with the operator prompt (native lane, no persona)", msgs[0].Content)
	}
	if strings.Contains(msgs[0].Content, "You can use tools.") {
		t.Errorf("native lane leaked the textual grammar:\n%q", msgs[0].Content)
	}
}
