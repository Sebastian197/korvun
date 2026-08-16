// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-B red suite (minimal-memory spec 2026-08-16): the Stage-16-style e2e
// halves of AS-B1 and AS-B4. The fake Ollama endpoint speaks the NATIVE
// tool-calling wire (the production adapter's lane): its first reply is a
// memory_note tool call, every later reply is a plain answer, and it
// records each request's system prompt. The config carries ONLY this local
// model, so "no cloud provider is ever contacted" holds by SelectModels
// construction — every wire request lands on this one local endpoint, and
// the note content appears nowhere else.
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

const e2eNote = "el usuario prefiere trato de usted"

// notesChatScript fakes the native Ollama /api/chat: request #1 answers a
// memory_note tool call carrying e2eNote; every later request answers a
// plain final reply. It records each request's leading system message.
type notesChatScript struct {
	mu      sync.Mutex
	systems []string
}

func (s *notesChatScript) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		sys := ""
		if len(body.Messages) > 0 && body.Messages[0].Role == "system" {
			sys = body.Messages[0].Content
		}
		s.systems = append(s.systems, sys)
		n := len(s.systems)
		s.mu.Unlock()

		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "llama3.2",
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"function": map[string]any{
							"name":      "memory_note",
							"arguments": map[string]any{"note": e2eNote},
						},
					}},
				},
				"done": true,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama3.2",
			"message": map[string]string{"role": "assistant", "content": "ok"},
			"done":    true,
		})
	}
}

func (s *notesChatScript) systemPrompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.systems...)
}

// memoryAgentCfg is a single-local-model agent brain with governed
// memory_note, its memory block, and the required storage.
func memoryAgentCfg(t *testing.T, baseURL, sensitivity, scope string) *config.Config {
	t.Helper()
	return &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Storage:       &config.StorageConfig{Path: filepath.Join(t.TempDir(), "k.db")},
		Brains: []config.BrainConfig{{
			Name:        "memo",
			Sensitivity: sensitivity,
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: "2s"},
			},
			Agent: &config.AgentConfig{
				Tools:      []string{"memory_note"},
				Governance: []config.ToolGrantConfig{{Tool: "memory_note", Mode: "allow"}},
				Memory:     &config.MemoryConfig{Scope: scope},
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "memo"}},
	}
}

// bootMemoryApp boots the app and returns the capturing channel; ask sends
// one inbound on conv and blocks until its reply lands.
func bootMemoryApp(t *testing.T, cfg *config.Config) *capturingChannel {
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
	return ch
}

func ask(t *testing.T, ch *capturingChannel, conv, text string) {
	t.Helper()
	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u-1"})
	in.AddText(text)
	in.Meta[router.MetaConversationID] = conv
	ch.inbound <- in
	select {
	case <-ch.sent:
	case <-time.After(8 * time.Second):
		t.Fatalf("no reply for %q within 8s", text)
	}
}

// wantNotesBlock derives the expected prompt block from the SAME source of
// truth the brain must use (brain.ComposeNotes), never a duplicated literal.
func wantNotesBlock() string {
	return brain.ComposeNotes([]conversation.Note{{Seq: 1, Content: e2eNote, Timestamp: time.Unix(100, 0)}}, 2000)
}

// AS-B1: a Private agent brain stores a note through the governed tool and
// the block rides every later request to the LOCAL model — the note content
// reaches this one local endpoint and nothing else, ever.
func TestBuild_memoryNoteRidesLaterPromptsPrivateLocal(t *testing.T) {
	script := &notesChatScript{}
	srv := httptest.NewServer(script.handler())
	t.Cleanup(srv.Close)

	ch := bootMemoryApp(t, memoryAgentCfg(t, srv.URL, "private", "conversation"))
	ask(t, ch, "c-1", "recuerda cómo tratarme")
	ask(t, ch, "c-1", "hola de nuevo")

	systems := script.systemPrompts()
	if len(systems) < 3 {
		t.Fatalf("local endpoint saw %d requests, want >= 3 (tool call + observation round + second message)", len(systems))
	}
	if strings.Contains(systems[0], e2eNote) {
		t.Fatalf("the FIRST request already carried the note — nothing was stored yet:\n%q", systems[0])
	}
	last := systems[len(systems)-1]
	if !strings.Contains(last, wantNotesBlock()) {
		t.Fatalf("the second message's system prompt misses the composed notes block:\nwant fragment %q\ngot %q", wantNotesBlock(), last)
	}
}

// AS-B4 (positive half): scope "brain" on an all-local brain — a note
// stored in conversation X rides prompts in conversation Y of the SAME
// brain.
func TestBuild_memoryBrainScopeCrossesConversationsAllLocal(t *testing.T) {
	script := &notesChatScript{}
	srv := httptest.NewServer(script.handler())
	t.Cleanup(srv.Close)

	ch := bootMemoryApp(t, memoryAgentCfg(t, srv.URL, "public", "brain"))
	ask(t, ch, "c-1", "recuerda cómo tratarme")
	ask(t, ch, "c-2", "hola desde otra conversación")

	systems := script.systemPrompts()
	if len(systems) < 3 {
		t.Fatalf("local endpoint saw %d requests, want >= 3", len(systems))
	}
	last := systems[len(systems)-1]
	if !strings.Contains(last, e2eNote) {
		t.Fatalf("brain-scope note from c-1 does not ride c-2's prompt (AS-B4):\n%q", last)
	}
}
