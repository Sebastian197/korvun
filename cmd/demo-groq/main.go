// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-groq is the live skeleton for Phase 4.2: a tiny
// end-to-end harness that takes a prompt, calls Groq through
// internal/model/groq, and prints the assistant response.
//
// THIS IS TEMPORARY. ADR-0010 §5 frames it as a stage-closure
// check, not a Korvun runtime entry point. It will be deleted (or
// rewritten as a real integration test) when Stage 5+ ships
// cmd/korvun proper.
//
// Usage:
//
//	demo-groq "¿Por qué el cielo es azul?"
//	echo "..." | demo-groq
//
// Environment (the API key is read ONLY from the environment per
// ADR-0010 §3 — never accepted as a CLI argument, because argv
// leaks via process listing and shell history):
//
//	GROQ_API_KEY        — REQUIRED. Your Groq API key.
//	KORVUN_DEMO_MODEL   — Groq model name (default
//	                     llama-3.1-8b-instant: free-tier with the
//	                     most permissive limits as of ADR-0010).
//	KORVUN_DEMO_SYSTEM  — optional system prompt.
//	KORVUN_DEMO_TIMEOUT — per-call timeout in seconds (default 60;
//	                     Groq returns much faster than Ollama cold
//	                     starts).
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
	"github.com/Sebastian197/korvun/internal/model/groq"
)

const (
	defaultModelName   = "llama-3.1-8b-instant"
	defaultTimeoutSecs = 60
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo-groq:", err)
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
	timeout := parseTimeout(os.Getenv("KORVUN_DEMO_TIMEOUT"))

	// Construct the adapter. groq.New itself enforces the env-only
	// key contract: if GROQ_API_KEY is empty we surface
	// ErrMissingAPIKey via a clear message to the operator and
	// exit non-zero. The key value never reaches this CLI.
	adapter, err := groq.New(groq.WithRequestTimeout(timeout))
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

	fmt.Fprintf(os.Stderr,
		"demo-groq: provider=%s model=%s timeout=%s\n",
		adapter.Name(), modelName, timeout)
	fmt.Fprintf(os.Stderr, "demo-groq: prompt = %q\n", prompt)
	if systemPrompt != "" {
		fmt.Fprintf(os.Stderr, "demo-groq: system = %q\n", systemPrompt)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	resp, err := adapter.Generate(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		// Surface the wrapped sentinel so an operator (or a test)
		// can grep for it. The adapter is responsible for not
		// leaking the API key into err.Error(); see
		// TestGenerate_errorsDoNotLeak* in internal/model/groq.
		return fmt.Errorf("Generate failed after %s: %w",
			elapsed.Round(time.Millisecond), err)
	}

	fmt.Fprintf(os.Stderr, "demo-groq: ok in %s, model=%s\n",
		elapsed.Round(time.Millisecond), resp.ModelName)
	fmt.Println(resp.Message.Content)
	return nil
}

// readPrompt collects the prompt from args or stdin. Args win if
// any are present; stdin is only consulted when args are empty
// AND something is being piped in.
func readPrompt(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		return strings.TrimSpace(strings.Join(args, " ")), nil
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stat stdin: %w", err)
	}
	if (info.Mode() & os.ModeCharDevice) != 0 {
		// stdin is a TTY (no pipe). Nothing to read.
		return "", nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// envOr returns the value of env if non-empty, otherwise fallback.
func envOr(env, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	return fallback
}

// parseTimeout reads the per-call timeout from the env var. Invalid
// values fall back to the default — the demo prefers running with a
// sane default over failing on bad config.
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
