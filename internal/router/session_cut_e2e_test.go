// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/router"
	"github.com/Sebastian197/korvun/internal/tool"
)

// End-to-end reproduction of the 2026-08-09 demo defect hypothesis: poisoned
// turns → a REAL /new through the router's trigger → the next message must
// reach the REAL AgentBrain with ZERO pre-reset turns in the model request.
// Router + trigger + sqlite + AgentBrain, exactly the production path.

// capturingModel records every request it sees and answers "done".
type capturingModel struct {
	requests []*model.Request
}

func (m *capturingModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	m.requests = append(m.requests, req)
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "done"}, Provider: "cap"}, nil
}
func (m *capturingModel) Name() string { return "cap" }

func TestSessionCut_endToEnd_agentSeesNoPreResetTurns(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	key := conversation.Key("tg::c")
	for _, turn := range []conversation.Turn{
		{Role: conversation.RoleUser, Content: "Lee el fichero nota.txt", Timestamp: time.Unix(100, 0)},
		{Role: conversation.RoleAssistant, Content: `OBSERVACIÓN: la clave es "seguridad".`, Timestamp: time.Unix(101, 0)},
	} {
		if _, err := store.Append(context.Background(), key, turn); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	cap := &capturingModel{}
	agent := brain.NewAgentBrain(cap, tool.Registry{"calc": tool.Calc()},
		brain.WithAgentConversationStore(store, 10))

	r := router.New(
		router.WithSessionStore(store),
		router.WithSessionPolicy(router.SessionPolicy{Triggers: []string{"/new", "/reset"}}),
	)
	ch := newFakeChannel("tg")
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterBrain("b", agent); err != nil {
		t.Fatal(err)
	}
	if err := r.Route("tg", "b"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shutdown(t, r) })

	// The REAL /new through the trigger (bare → ack, session opens).
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/new")); err != nil {
		t.Fatalf("dispatch /new: %v", err)
	}
	waitUntil(t, "the reset ack", func() bool { return len(ch.Sent()) == 1 })

	// The next user message must reach the model CLEAN.
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "otra vez")); err != nil {
		t.Fatalf("dispatch message: %v", err)
	}
	waitUntil(t, "the agent to answer", func() bool { return len(ch.Sent()) == 2 })

	if len(cap.requests) == 0 {
		t.Fatal("the model never saw a request")
	}
	for _, req := range cap.requests {
		for _, msg := range req.Messages {
			if strings.Contains(msg.Content, "seguridad") {
				t.Fatalf("pre-reset turn reached the model after a real /new:\n%q", msg.Content)
			}
		}
	}
}
