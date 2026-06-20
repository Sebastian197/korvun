// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-fanout is the live skeleton for Phase 4.3: a tiny
// end-to-end harness that takes a prompt, fans it out across Ollama
// AND Groq in parallel using internal/model/fanout, and prints every
// outcome (provider, latency, content OR sentinel-wrapped error).
//
// THIS IS TEMPORARY. ADR-0011 §"Plan" frames it as a stage-closure
// check, not a Korvun runtime entry point. It will be deleted (or
// rewritten as a real integration test) when Stage 5+ ships
// cmd/korvun proper.
//
// What the demo proves:
//
//   - The fan-out coordinator launches BOTH providers in parallel
//     from a single Run call, blocks until every one has returned,
//     and surfaces a per-Outcome slice in input order.
//   - The sentinel grammar (ErrAuthInvalid, *RateLimitError,
//     ErrProviderUnavailable, ErrProviderResponse) round-trips
//     through the fan-out unchanged.
//   - Per-Outcome latency is captured.
//   - A partial failure (one provider OK, the other erroring) is the
//     NORMAL shape, not a Run-level error.
//
// Catalog mismatch note: each provider has its own model catalog.
// The demo passes ONE req.Model to BOTH adapters; whichever provider
// does not recognise it returns ErrProviderResponse (model-not-found
// is a config bug from the provider's view, per ADR-0010 §4). This
// IS the partial-failure surface the fan-out exists to expose, and
// it is what the policy engine of Stages 5–6 will resolve by
// mapping a high-level intent to a per-provider model name.
//
// Usage:
//
//	demo-fanout "¿Por qué el cielo es azul?"
//	echo "..." | demo-fanout
//
// Environment (the Groq API key is read ONLY from the environment
// per ADR-0010 §3 — never accepted as a CLI argument):
//
//	GROQ_API_KEY             — REQUIRED. Your Groq API key.
//	OLLAMA_HOST              — Ollama base URL (default
//	                          http://127.0.0.1:11434).
//	KORVUN_DEMO_MODEL        — Model name passed to BOTH providers
//	                          (default "llama3.2": Ollama-flavoured;
//	                          Groq will surface ErrProviderResponse).
//	KORVUN_DEMO_SYSTEM       — Optional system prompt.
//	KORVUN_DEMO_TIMEOUT      — Per-model timeout in seconds
//	                          (default 60); applied via
//	                          fanout.WithPerModelTimeout.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/model/groq"
	"github.com/Sebastian197/korvun/internal/model/ollama"
)

const (
	defaultModelName   = "llama3.2"
	defaultTimeoutSecs = 60
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo-fanout:", err)
		os.Exit(1)
	}
}

func run() error {
	prompt, err := readPrompt(os.Args[1:], os.Stdin)
	if err != nil {
		return err
	}
	if prompt == "" {
		return errors.New("empty prompt; pass it as argument or on stdin")
	}

	modelName := envOr("KORVUN_DEMO_MODEL", defaultModelName)
	systemPrompt := os.Getenv("KORVUN_DEMO_SYSTEM")
	perModelTimeout := parseTimeout(os.Getenv("KORVUN_DEMO_TIMEOUT"))

	// Construct both adapters. groq.New enforces the env-only key
	// contract; if GROQ_API_KEY is empty we surface a clear message
	// and exit non-zero. The key value never reaches this CLI.
	ollamaAdapter := ollama.New(ollama.WithRequestTimeout(perModelTimeout))
	groqAdapter, err := groq.New(groq.WithRequestTimeout(perModelTimeout))
	if err != nil {
		return fmt.Errorf("groq.New: %w", err)
	}

	messages := make([]model.Message, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, model.Message{
			Role:    model.RoleSystem,
			Content: systemPrompt,
		})
	}
	messages = append(messages, model.Message{
		Role:    model.RoleUser,
		Content: prompt,
	})

	req := &model.Request{
		Model:    modelName,
		Messages: messages,
	}

	coordinator := fanout.New(fanout.WithPerModelTimeout(perModelTimeout))
	providers := []model.Model{ollamaAdapter, groqAdapter}

	fmt.Fprintf(os.Stderr,
		"demo-fanout: providers=[%s,%s] model=%s per-model-timeout=%s\n",
		ollamaAdapter.Name(), groqAdapter.Name(), modelName, perModelTimeout)
	fmt.Fprintf(os.Stderr, "demo-fanout: prompt = %q\n", prompt)
	if systemPrompt != "" {
		fmt.Fprintf(os.Stderr, "demo-fanout: system = %q\n", systemPrompt)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	res, err := coordinator.Run(ctx, req, providers)
	elapsed := time.Since(start)
	if err != nil {
		// Mechanism-level error (validation, nil ctx, etc.). Per-model
		// failures are NOT this branch — they land in Outcome.Err
		// below.
		return fmt.Errorf("fanout.Run failed after %s: %w",
			elapsed.Round(time.Millisecond), err)
	}

	fmt.Fprintf(os.Stderr, "demo-fanout: total wall-clock %s\n",
		elapsed.Round(time.Millisecond))

	anyOK := false
	for i, o := range res.Outcomes {
		printOutcome(i, o)
		if o.Err == nil {
			anyOK = true
		}
	}
	if !anyOK {
		return errors.New("every provider failed; see per-Outcome errors above")
	}
	return nil
}

// printOutcome writes one Outcome to stderr (the metadata line) and,
// for success cases, the assistant content to stdout. This keeps the
// pipe form usable: piping demo-fanout into a tool reads the
// successful completions; the metadata stays on stderr where shell
// composition does not see it.
func printOutcome(i int, o fanout.Outcome) {
	if o.Err != nil {
		fmt.Fprintf(os.Stderr,
			"demo-fanout: [%d] provider=%s latency=%s err=%v\n",
			i, o.Provider, o.Latency.Round(time.Millisecond), o.Err)
		return
	}
	modelName := ""
	if o.Response != nil {
		modelName = o.Response.ModelName
	}
	fmt.Fprintf(os.Stderr,
		"demo-fanout: [%d] provider=%s latency=%s ok model=%s\n",
		i, o.Provider, o.Latency.Round(time.Millisecond), modelName)
	fmt.Printf("--- provider=%s model=%s ---\n%s\n",
		o.Provider, modelName, o.Response.Message.Content)
}

// readPrompt collects the prompt from args or stdin. Args win if any
// are present; stdin is only consulted when args are empty AND
// something is being piped in.
func readPrompt(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		return strings.TrimSpace(strings.Join(args, " ")), nil
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stat stdin: %w", err)
	}
	if (info.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func envOr(env, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	return fallback
}

func parseTimeout(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Duration(defaultTimeoutSecs) * time.Second
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return time.Duration(defaultTimeoutSecs) * time.Second
	}
	return time.Duration(n) * time.Second
}
