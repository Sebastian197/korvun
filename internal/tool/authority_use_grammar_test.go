// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestAuthorityUse_AnalyzersSpeakTheRealToolsGrammar attacks AS-AUTH-16 where
// the adversary's pass over piece 3 phase 3 broke it (F2): the operation-use
// analyzers parsed an argument grammar NO shipped tool accepts, so every mould
// that fed them was a sentence about a grammar that does not exist. Here the
// REAL tool and the analyzer are handed the SAME string — the string the
// executor hands both — and must agree on what it names:
//
//   - what the tool accepts, the analyzer resolves, to the target the tool acts
//     on (read_file really reads the file; the http tools are stopped at their
//     own host allow-list, whose refusal names the host THEY parsed — no
//     network is touched);
//   - what the tool refuses as malformed, the analyzer leaves unresolved;
//   - the one declared divergence is in the closed direction: read_file accepts
//     a path relative to its jail root, the analyzer refuses it as unresolved,
//     because it does not know that root.
//
// Evidence level: in-process; the real tools built through their exported
// constructors, a real file in a real jail; no network.
// Probing mutation executed: the analyzers parse the JSON object grammar again —
// red on every accepted row with «analyzer error = action: authority use
// unresolved».
func TestAuthorityUse_AnalyzersSpeakTheRealToolsGrammar(t *testing.T) {
	registry := action.NewOperationUseRegistry()
	if err := action.RegisterBuiltInOperationUse(registry); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("read_file", func(t *testing.T) {
		jail := t.TempDir()
		file := filepath.Join(jail, "ok.txt")
		if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
			t.Fatal(err)
		}
		reader, err := ReadFile(ReadFileConfig{Root: jail})
		if err != nil {
			t.Fatal(err)
		}
		// Absolute: the tool reads it, the analyzer names exactly it.
		args := "  " + file + "  "
		if out, err := reader.Execute(ctx, args); err != nil || out != "hello" {
			t.Fatalf("the real tool on its own grammar: %q, %v", out, err)
		}
		use, err := registry.Analyze("read_file", args)
		if err != nil {
			t.Fatalf("analyzer error = %v over arguments the real tool accepts", err)
		}
		if len(use.Resources) != 1 || use.Resources[0].Kind != "path" || use.Resources[0].ID != filepath.Clean(file) {
			t.Errorf("derived resources = %#v, want the path the tool read, %q", use.Resources, filepath.Clean(file))
		}
		// Relative: the tool joins it to its root; the analyzer does not know
		// the root and fails closed.
		if out, err := reader.Execute(ctx, "ok.txt"); err != nil || out != "hello" {
			t.Fatalf("the real tool on a jail-relative path: %q, %v", out, err)
		}
		if _, err := registry.Analyze("read_file", "ok.txt"); !errors.Is(err, action.ErrAuthorityUseUnresolved) {
			t.Errorf("analyzer on a jail-relative path: error = %v, want %v", err, action.ErrAuthorityUseUnresolved)
		}
		// Empty: refused by both.
		if _, err := reader.Execute(ctx, "   "); err == nil {
			t.Error("the real tool accepted an empty path")
		}
		if _, err := registry.Analyze("read_file", "   "); !errors.Is(err, action.ErrAuthorityUseUnresolved) {
			t.Errorf("analyzer on an empty path: error = %v, want %v", err, action.ErrAuthorityUseUnresolved)
		}
	})

	t.Run("http_fetch", func(t *testing.T) {
		fetch, err := HTTPFetch(HTTPFetchConfig{AllowHosts: []string{"allowed.example"}})
		if err != nil {
			t.Fatal(err)
		}
		const args = "  https://outside.example/reports/q1  "
		_, err = fetch.Execute(ctx, args)
		if !errors.Is(err, ErrCageViolation) || !strings.Contains(err.Error(), `"outside.example"`) {
			t.Fatalf("the real tool's refusal = %v, want its allow-list naming the host it parsed", err)
		}
		use, err := registry.Analyze("http_fetch", args)
		if err != nil {
			t.Fatalf("analyzer error = %v over arguments the real tool parses", err)
		}
		if len(use.Resources) != 1 || use.Resources[0].ID != "https://outside.example/reports/q1" ||
			len(use.Destinations) != 1 || use.Destinations[0] != "outside.example" {
			t.Errorf("derived use = %#v, want the URL and the host the tool parsed", use)
		}
		if _, err := fetch.Execute(ctx, ""); err == nil {
			t.Error("the real tool accepted an empty URL")
		}
		if _, err := registry.Analyze("http_fetch", ""); !errors.Is(err, action.ErrAuthorityUseUnresolved) {
			t.Errorf("analyzer on an empty URL: error = %v, want %v", err, action.ErrAuthorityUseUnresolved)
		}
	})

	t.Run("webhook_call", func(t *testing.T) {
		call, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{"allowed.example"}})
		if err != nil {
			t.Fatal(err)
		}
		const args = `https://outside.example/hook {"order":1}`
		_, err = call.Execute(ctx, args)
		if !errors.Is(err, ErrCageViolation) || !strings.Contains(err.Error(), `"outside.example"`) {
			t.Fatalf("the real tool's refusal = %v, want its allow-list naming the host it parsed", err)
		}
		use, err := registry.Analyze("webhook_call", args)
		if err != nil {
			t.Fatalf("analyzer error = %v over arguments the real tool parses", err)
		}
		if len(use.Resources) != 1 || use.Resources[0].ID != "https://outside.example/hook" ||
			len(use.Destinations) != 1 || use.Destinations[0] != "outside.example" ||
			len(use.Data) != 1 || use.Data[0] != "payload" {
			t.Errorf("derived use = %#v, want the URL, the host and the payload tag", use)
		}
		// No body, and a body that is not JSON: malformed for both.
		for _, malformed := range []string{"https://outside.example/hook", "https://outside.example/hook not-json"} {
			if _, err := call.Execute(ctx, malformed); err == nil || errors.Is(err, ErrCageViolation) {
				t.Errorf("the real tool on %q: error = %v, want its malformed-arguments refusal", malformed, err)
			}
			if _, err := registry.Analyze("webhook_call", malformed); !errors.Is(err, action.ErrAuthorityUseUnresolved) {
				t.Errorf("analyzer on %q: error = %v, want %v", malformed, err, action.ErrAuthorityUseUnresolved)
			}
		}
	})
}
