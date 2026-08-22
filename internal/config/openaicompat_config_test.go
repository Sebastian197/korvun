// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config

// RED suite for AS-1 of the universal model gateway spec
// (docs/superpowers/specs/2026-08-22-universal-model-gateway.md, FINAL):
// the "openai-compatible" provider in validateModels — base_url
// fail-loud (incl. the four H4 negatives), the canonical-triplet guard
// in both directions, and the positive anchors (N2).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadCompatJSON writes the JSON to a temp file and runs Load.
func loadCompatJSON(t *testing.T, doc string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_, err := Load(path)
	return err
}

// compatDoc wraps the given models JSON array in a minimal valid config.
func compatDoc(models string) string {
	return `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],` +
		`"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":` + models + `}],` +
		`"routes":[{"channel":"telegram","brain":"d"}]}`
}

func TestValidate_openaicompat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		models string
		// wantFields empty => the config must LOAD; otherwise every entry
		// must appear in the ErrInvalidConfig error text.
		wantFields []string
	}{
		{
			name:   "valid compat entry loads (AS-1)",
			models: `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1","api_key_env":"K"}]`,
		},
		{
			name:       "base_url missing fails naming the field (AS-1)",
			models:     `[{"provider":"openai-compatible","model_id":"m","locality":"cloud"}]`,
			wantFields: []string{"brains[0].models[0].base_url"},
		},
		{
			name:       "base_url not a URL fails naming the field (AS-1)",
			models:     `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"://not a url"}]`,
			wantFields: []string{"brains[0].models[0].base_url"},
		},
		{
			name:       "base_url with userinfo is rejected (H4)",
			models:     `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://user:pass@api.example.com/v1"}]`,
			wantFields: []string{"brains[0].models[0].base_url"},
		},
		{
			name:       "base_url with a query is rejected (H4)",
			models:     `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1?key=v"}]`,
			wantFields: []string{"brains[0].models[0].base_url"},
		},
		{
			name:       "base_url with a fragment is rejected (H4)",
			models:     `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1#frag"}]`,
			wantFields: []string{"brains[0].models[0].base_url"},
		},
		{
			name:       "base_url with an empty host is rejected (H4)",
			models:     `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https:///v1"}]`,
			wantFields: []string{"brains[0].models[0].base_url"},
		},
		{
			name: "identical canonical triplet is rejected naming both indices (H9)",
			models: `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1"},` +
				`{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1"}]`,
			wantFields: []string{"brains[0].models[0]", "brains[0].models[1]"},
		},
		{
			name: "trailing-slash variant collides after normalization (H9)",
			models: `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1"},` +
				`{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://api.example.com/v1/"}]`,
			wantFields: []string{"brains[0].models[0]", "brains[0].models[1]"},
		},
		{
			name: "two compat entries, same base_url, distinct model_ids load (N2)",
			models: `[{"provider":"openai-compatible","model_id":"m1","locality":"cloud","base_url":"https://api.example.com/v1"},` +
				`{"provider":"openai-compatible","model_id":"m2","locality":"cloud","base_url":"https://api.example.com/v1"}]`,
		},
		{
			name: "two compat entries, distinct base_urls, same model_id load (N2)",
			models: `[{"provider":"openai-compatible","model_id":"m","locality":"cloud","base_url":"https://a.example.com/v1"},` +
				`{"provider":"openai-compatible","model_id":"m","locality":"local","base_url":"http://127.0.0.1:1234/v1"}]`,
		},
		{
			name: "ollama x2 with distinct model_ids still loads (guard is compat-scoped)",
			models: `[{"provider":"ollama","model_id":"llama3.2","locality":"local"},` +
				`{"provider":"ollama","model_id":"mistral","locality":"local"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := loadCompatJSON(t, compatDoc(tc.models))
			if len(tc.wantFields) == 0 {
				if err != nil {
					t.Fatalf("Load: %v, want success", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Load: nil error, want ErrInvalidConfig")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("errors.Is(ErrInvalidConfig) = false for err %v", err)
			}
			for _, field := range tc.wantFields {
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error %q does not name %q", err.Error(), field)
				}
			}
		})
	}
}
