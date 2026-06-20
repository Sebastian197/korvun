// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-brain is a disposable live skeleton of the Stage 7 Brain
// (ADR-0014). It builds an inbound Envelope by hand, hands it to an
// Orchestrator wired to REAL Ollama (+ Groq, if GROQ_API_KEY is set) over the
// fan-out and a priority policy, and prints the outbound reply Envelope. It is
// the first end-to-end path: Envelope in -> translate -> fan-out -> policy ->
// translate -> Envelope out.
//
// If Ollama is not running and Groq is unavailable, every provider fails and
// the Brain returns its configured fallback reply (ADR-0014 §3) — which is
// itself a valid demonstration of the no-answer path.
//
// The channel + router wiring into a single binary is Stage 11 (cmd/korvun);
// here Handle is invoked directly. Delete this command then, with the other
// demos.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/model/groq"
	"github.com/Sebastian197/korvun/internal/model/ollama"
	"github.com/Sebastian197/korvun/internal/policy"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// readPrompt takes the prompt from the CLI args, else stdin, else a default.
func readPrompt() string {
	if len(os.Args) > 1 {
		return strings.Join(os.Args[1:], " ")
	}
	info, _ := os.Stdin.Stat()
	if (info.Mode() & os.ModeCharDevice) == 0 {
		sc := bufio.NewScanner(os.Stdin)
		if sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
				return line
			}
		}
	}
	return "In one short sentence: what is a messaging gateway?"
}

func main() {
	prompt := readPrompt()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const perModel = 30 * time.Second
	var models []model.Model

	ollamaModel := getenv("OLLAMA_MODEL", "llama3.2")
	models = append(models, brain.WithModelID(ollama.New(ollama.WithRequestTimeout(perModel)), ollamaModel))
	fmt.Printf("provider: ollama   model=%s\n", ollamaModel)

	groqModel := getenv("GROQ_MODEL", "llama-3.3-70b-versatile")
	if g, err := groq.New(groq.WithRequestTimeout(perModel)); err == nil {
		models = append(models, brain.WithModelID(g, groqModel))
		fmt.Printf("provider: groq     model=%s\n", groqModel)
	} else {
		fmt.Printf("provider: groq     SKIPPED (%v)\n", err)
	}

	coord := fanout.New(fanout.WithPerModelTimeout(perModel))
	// Prefer the local model; fall to Groq only if Ollama did not answer.
	pol := policy.PriorityReducer{Order: []string{"ollama", "groq"}}
	b := brain.NewOrchestrator(coord, models, pol,
		brain.WithFallback("(no answer — is Ollama running? is GROQ_API_KEY set?)"))

	in := envelope.New("demo", envelope.Inbound, envelope.Participant{ID: "user-1", Name: "Operator"})
	in.AddText(prompt)
	in.Meta["demo.chat_id"] = "chat-123"

	fmt.Printf("\ninbound : %q\n          channel=%s chat=%s\n",
		prompt, in.Channel, in.Meta["demo.chat_id"])

	out, err := b.Handle(ctx, in)
	if err != nil {
		// Mechanism error: the Brain is misconfigured (ADR-0014 §3).
		fmt.Fprintf(os.Stderr, "\nbrain misconfiguration: %v\n", err)
		os.Exit(1)
	}
	if len(out) == 0 {
		fmt.Println("\noutbound: (no reply — nothing to ask)")
		return
	}

	reply := out[0]
	fmt.Printf("\noutbound: %q\n", reply.Parts[0].Content)
	fmt.Printf("          channel=%s direction=%s chat=%s (addressing echoed back)\n",
		reply.Channel, reply.Direction, reply.Meta["demo.chat_id"])
	if err := reply.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: outbound envelope invalid: %v\n", err)
	}
}
