// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The missing cable — agent.effect_ceiling (adjudicated 2026-09-01):
// the E5 gate demanded approval only under BOUNDED authority, but the
// production brain's identity carried no ceiling (config-derived
// grants are ceilingless by sealed E3 design), so the chat path could
// NEVER park an action — a scope hole the cross-check law caught
// BEFORE the ceremony. The per-brain field lands the ceiling in the
// production identity: absent = today byte-for-byte (pinned); an
// unknown class dies boot-fatal NAMING the valid ladder. The chat-path
// parking test below is the link the stage lacked — the real wire,
// not the hand-mounted identity. Approved-red contract.

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

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// webhookChatScript fakes /api/chat: request #1 answers a webhook_call
// tool call; later requests answer plain. It records every message
// content it receives (the observation travels back as a message).
type webhookChatScript struct {
	mu       sync.Mutex
	requests int
	contents []string
}

func (s *webhookChatScript) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		for _, m := range body.Messages {
			s.contents = append(s.contents, m.Content)
		}
		s.requests++
		n := s.requests
		s.mu.Unlock()
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "llama3.2",
				"message": map[string]any{
					"role": "assistant", "content": "",
					"tool_calls": []map[string]any{{
						"function": map[string]any{
							"name":      "webhook_call",
							"arguments": map[string]any{"url": "http://127.0.0.1:5678/hook", "message": "hi"},
						},
					}},
				},
				"done": true,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama3.2",
			"message": map[string]string{"role": "assistant", "content": "entendido"},
			"done":    true,
		})
	}
}

func (s *webhookChatScript) sawContent(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.contents {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// ceilingChatCfg builds the production-shaped config: one local brain
// with webhook_call caged to localhost, the approvals knob, and the
// per-brain ceiling under test.
func ceilingChatCfg(t *testing.T, baseURL, ceiling string) (*config.Config, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "k.db")
	cfg := &config.Config{
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:      []config.ChannelConfig{telegramChannel()},
		Storage:       &config.StorageConfig{Path: dbPath},
		Approvals:     &config.ApprovalsConfig{Enabled: true},
		Brains: []config.BrainConfig{{
			Name:        "memo",
			Sensitivity: "public",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: "2s"},
			},
			Agent: &config.AgentConfig{
				Tools:         []string{"webhook_call"},
				Governance:    []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "allow"}},
				WebhookCall:   &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:5678"}},
				EffectCeiling: ceiling,
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "memo"}},
	}
	return cfg, dbPath
}

func TestEffectCeilingConfig_unknownClassDiesNamingTheLadder(t *testing.T) {
	script := &webhookChatScript{}
	srv := httptest.NewServer(script.handler())
	defer srv.Close()
	cfg, _ := ceilingChatCfg(t, srv.URL, "mega_dangerous")
	_, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err == nil {
		t.Fatal("an unknown effect class must be boot-fatal")
	}
	for _, must := range []string{"mega_dangerous", "pure", "write_irreversible", "critical"} {
		if !strings.Contains(err.Error(), must) {
			t.Fatalf("the refusal must name the offender and the valid ladder (%q missing): %v", must, err)
		}
	}
}

// The link the stage lacked: with the ceiling in the CONFIG and the
// knob ON, the PRODUCTION wiring parks an irreversible chat attempt.
func TestEffectCeiling_theChatPathParksForReal(t *testing.T) {
	script := &webhookChatScript{}
	srv := httptest.NewServer(script.handler())
	defer srv.Close()
	cfg, dbPath := ceilingChatCfg(t, srv.URL, "write_irreversible")
	ch := bootMemoryApp(t, cfg)
	ask(t, ch, "conv-ceil", "manda el webhook")
	// The model received the honest pending observation.
	if !script.sawContent("PENDING APPROVAL") {
		t.Fatalf("the model must receive the pending observation; contents: %v", script.contents)
	}
	// And the request was born whole through the production path.
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	pending, err := store.ListApprovals(context.Background(), action.ApprovalPending)
	if err != nil || len(pending) != 1 {
		t.Fatalf("one parked request from the chat path: %v %d", err, len(pending))
	}
	rec, err := store.Get(context.Background(), pending[0].ActionID)
	if err != nil || rec.State != action.StatePendingApproval {
		t.Fatalf("parked: %v %v", err, rec.State)
	}
}

// The sacred pin: same chat, same knob, NO ceiling — unbounded lane
// untouched: the webhook executes exactly as today.
func TestEffectCeiling_absentMeansTodayByteForByte(t *testing.T) {
	script := &webhookChatScript{}
	srv := httptest.NewServer(script.handler())
	defer srv.Close()
	cfg, dbPath := ceilingChatCfg(t, srv.URL, "")
	ch := bootMemoryApp(t, cfg)
	ask(t, ch, "conv-noceil", "manda el webhook")
	if script.sawContent("PENDING APPROVAL") {
		t.Fatal("without a ceiling nothing parks — today byte-for-byte")
	}
	store, err := actionsqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if pending, _ := store.ListApprovals(context.Background(), action.ApprovalPending); len(pending) != 0 {
		t.Fatalf("no requests without a ceiling: %d", len(pending))
	}
}
