// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-policy is a disposable live skeleton that shows the policy
// engine deciding over a hand-built fan-out Result. It does NOT call any
// model — the Outcomes are fabricated so the two reducers (PriorityReducer
// and ConsensusReducer, ADR-0012 / ADR-0013) can be compared on identical
// input. Real, model-driven dispatch arrives with the Brain in Stage 7.
//
// Delete this command when cmd/korvun proper boots (same disposition as
// cmd/demo-model, cmd/demo-groq, cmd/demo-fanout).
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
)

// ok builds a successful Outcome voting content.
func ok(provider, content string, latency time.Duration) fanout.Outcome {
	return fanout.Outcome{
		Provider: provider,
		Response: &model.Response{
			Message:   model.Message{Role: model.RoleAssistant, Content: content},
			Provider:  provider,
			ModelName: provider + "/demo",
		},
		Latency: latency,
	}
}

// fail builds a failed Outcome.
func fail(provider string, err error, latency time.Duration) fanout.Outcome {
	return fanout.Outcome{Provider: provider, Err: err, Latency: latency}
}

// printDecision renders a Decision (or the error path) the way the Brain
// will eventually log it: chosen reply, who contributed, and accounting.
// dec is non-nil even on the ErrNoConsensus / ErrNoUsableOutcome paths.
func printDecision(label string, dec *policy.Decision, err error) {
	fmt.Printf("  %s\n", label)
	switch {
	case err != nil && dec != nil && dec.Response == nil:
		fmt.Printf("    -> no reply: %v\n", err)
	case err != nil:
		fmt.Printf("    -> error: %v\n", err)
	default:
		fmt.Printf("    -> reply: %q (from %s)\n", dec.Response.Message.Content, dec.Response.Provider)
	}
	if dec == nil {
		return
	}
	fmt.Printf("    provenance:\n")
	for i, c := range dec.Provenance.Considered {
		used := "    "
		if c.Used {
			used = "USED"
		}
		line := fmt.Sprintf("      [%s] %-9s %6s", used, c.Provider, dec.Accounting[i].Latency)
		switch {
		case c.Err != nil:
			line += fmt.Sprintf("  failed: %v", c.Err)
		case !c.Used:
			line += "  (considered, not used)"
		}
		fmt.Println(line)
	}
}

// scenario runs both reducers over the same Result and prints both Decisions.
func scenario(title string, order []string, outcomes []fanout.Outcome) {
	fmt.Printf("\n=== %s ===\n", title)
	res := &fanout.Result{Outcomes: outcomes}
	ctx := context.Background()

	prio := policy.PriorityReducer{Order: order}
	pd, pe := prio.Apply(ctx, res)
	printDecision(fmt.Sprintf("PriorityReducer (Order=%v)", order), pd, pe)

	cons := policy.ConsensusReducer{Order: order}
	cd, ce := cons.Apply(ctx, res)
	printDecision(fmt.Sprintf("ConsensusReducer (Order=%v)", order), cd, ce)
}

func main() {
	fmt.Println("Korvun policy engine — demo over a hand-built fan-out Result")
	fmt.Println("(fabricated Outcomes; no models are called — Stage 7 wires the real Brain)")

	// Scenario 1: the two reducers DISAGREE on the same data.
	// The top-priority provider (mistral) answered "negative", but three of
	// the four usable providers agree on "positive" (with casing variants to
	// show normalization). Priority follows the trusted provider; consensus
	// follows the crowd, with groq as the highest-priority representative of
	// the winning class.
	scenario(
		"Consensus reached — and it disagrees with priority",
		[]string{"mistral", "groq", "ollama", "together"},
		[]fanout.Outcome{
			ok("ollama", "Positive", 12*time.Millisecond),
			ok("groq", "positive", 8*time.Millisecond),
			ok("together", "POSITIVE", 21*time.Millisecond),
			ok("mistral", "negative", 9*time.Millisecond),
			fail("openai", model.ErrAuthInvalid, 2*time.Millisecond),
		},
	)

	// Scenario 2: a 2-2 split. Priority still decides (its top usable
	// provider); consensus refuses with ErrNoConsensus — the policy engine's
	// distinguishing behavior.
	scenario(
		"No consensus — a 2 vs 2 split",
		[]string{"groq", "ollama", "together", "mistral"},
		[]fanout.Outcome{
			ok("ollama", "yes", 10*time.Millisecond),
			ok("groq", "no", 7*time.Millisecond),
			ok("together", "yes", 15*time.Millisecond),
			ok("mistral", "no", 8*time.Millisecond),
		},
	)

	fmt.Println()
}
