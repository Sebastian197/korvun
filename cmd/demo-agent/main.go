// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-agent is a disposable live skeleton for the Stage 8 tool-use
// AgentBrain (ADR-0021). It wires one Groq model to the three safe built-in tools
// (time, echo, calc) and runs the bounded model→tool→model loop against a sample
// prompt, printing the reply. It exists to exercise the loop end-to-end against a
// real provider; delete it once the binary path is exercised (like the earlier
// cmd/demo-* skeletons). Groq follows the one-line TOOL: protocol reliably, which
// is why the agent demo targets it rather than a small local model (ADR-0021 §3).
//
// Run: GROQ_API_KEY=... go run ./cmd/demo-agent
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model/groq"
	"github.com/Sebastian197/korvun/internal/tool"
)

func main() {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		fmt.Println("demo-agent: set GROQ_API_KEY to run the live tool-use loop (skipped).")
		return
	}

	g, err := groq.New(groq.WithAPIKey(key), groq.WithRequestTimeout(30*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo-agent: build groq: %v\n", err)
		os.Exit(1)
	}

	reg := tool.Registry{"time": tool.Time(nil), "echo": tool.Echo(), "calc": tool.Calc()}
	agent := brain.NewAgentBrain(
		brain.WithModelID(g, "llama-3.3-70b-versatile"),
		reg,
		brain.WithAgentLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
		brain.WithAgentMaxIterations(5),
		brain.WithAgentPerToolTimeout(5*time.Second),
		brain.WithAgentSystemPrompt("Use the calc tool for arithmetic. Be concise."),
	)

	in := envelope.New("demo", envelope.Inbound, envelope.Participant{ID: "user", Name: "User"}).
		AddText("What is 12*9 plus 7? Use the calculator.")
	in.Meta[conversation.MetaConversationID] = "demo-agent"

	out, err := agent.Handle(context.Background(), in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo-agent: handle: %v\n", err)
		os.Exit(1)
	}
	for _, e := range out {
		for _, p := range e.Parts {
			fmt.Printf("reply: %s\n", p.Content)
		}
	}
}
