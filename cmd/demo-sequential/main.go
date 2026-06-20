// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-sequential is a disposable live skeleton of the Stage 6
// sequential coordinator (ADR-0016). It shows the cost-saving fail-over on two
// scenarios over the same ordered model set [ollama, groq]:
//
//	scenario 1 (ollama answers): groq is NEVER called — the saving.
//	scenario 2 (ollama fails):   groq is called as fail-over and answers.
//
// Unlike the parallel fan-out, which calls and pays every provider, the
// sequential coordinator stops at the first success — so a paid cloud provider
// is contacted only when the cheap/local one failed. The behaviour is what this
// demo shows; it needs NO live Ollama/Groq, using stub models whose call
// counters make "called" vs "skipped" visible.
//
// Delete this command in Stage 11 (cmd/korvun) with the other demos.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/sequential"
)

// stubModel is a controllable provider that counts how many times it was asked
// to Generate, so the demo can show which models the coordinator actually
// called and which it skipped.
type stubModel struct {
	name  string
	fail  bool
	calls int
}

func (s *stubModel) Name() string { return s.name }

func (s *stubModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	s.calls++
	if s.fail {
		return nil, model.ErrProviderUnavailable
	}
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: "answer from " + s.name},
		Provider:  s.name,
		ModelName: s.name,
	}, nil
}

func req() *model.Request {
	return &model.Request{
		Model:    "demo",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}},
	}
}

// scenario runs the sequential coordinator over [ollama, groq] and prints what
// was called, what was skipped, and the answer.
func scenario(title string, ollamaFails bool) {
	ollama := &stubModel{name: "ollama", fail: ollamaFails} // local, called first
	groq := &stubModel{name: "groq"}                        // cloud, fail-over only
	models := []model.Model{ollama, groq}

	res, err := sequential.New().Run(context.Background(), req(), models)
	if err != nil {
		fmt.Printf("%-22s ERROR: %v\n", title, err)
		return
	}

	answer := "(none)"
	if n := len(res.Outcomes); n > 0 && res.Outcomes[n-1].Err == nil {
		answer = res.Outcomes[n-1].Response.Message.Content
	}

	fmt.Printf("%s\n", title)
	for _, m := range models {
		s := m.(*stubModel)
		switch {
		case s.calls == 0:
			fmt.Printf("    %-7s SKIPPED (not called — never paid)\n", s.name)
		case s.fail:
			fmt.Printf("    %-7s called -> failed\n", s.name)
		default:
			fmt.Printf("    %-7s called -> ok\n", s.name)
		}
	}
	fmt.Printf("    answer: %q  (outcomes recorded: %d)\n\n", answer, len(res.Outcomes))
}

func main() {
	fmt.Println("ordered model set: [ollama (local, first), groq (cloud, fail-over)]")
	fmt.Println()

	scenario("scenario 1 — ollama answers: groq never called (the cost saving)", false)
	scenario("scenario 2 — ollama fails: groq called as fail-over", true)

	os.Exit(0)
}
