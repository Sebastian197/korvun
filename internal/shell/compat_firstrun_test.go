// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// N1 RED — the compat model probe and the first-run rewrite (spec
// 2026-08-29-n1-onboarding-gateway.md AS-6/AS-7). The probe is exercised
// against a deterministic fake /models server; outcomes are results the
// onboarding paints, never Go errors, and never a secret value.

// fakeModelsServer answers GET /models in the OpenAI-compatible list shape.
// requireKey != "" demands that exact Bearer, else 401.
func fakeModelsServer(t *testing.T, requireKey string, ids ...string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if requireKey != "" && r.Header.Get("Authorization") != "Bearer "+requireKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"missing key"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := `{"data":[`
		for i, id := range ids {
			if i > 0 {
				body += ","
			}
			body += `{"id":"` + id + `"}`
		}
		body += `]}`
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestCheckCompatModel_foundMissingUnreachable(t *testing.T) {
	s := fakeModelsServer(t, "", "qwen3-4b", "llama-3.3-70b")
	d := testDesktop(testController())

	got := d.CheckCompatModel(s.URL+"/v1", "qwen3-4b", "")
	if !got.Reachable || !got.ModelFound || got.NeedsKey {
		t.Fatalf("found case = %+v, want reachable+found", got)
	}

	got = d.CheckCompatModel(s.URL+"/v1", "no-such-model", "")
	if !got.Reachable || got.ModelFound {
		t.Fatalf("missing case = %+v, want reachable but NOT found", got)
	}

	got = d.CheckCompatModel("http://127.0.0.1:1/v1", "qwen3-4b", "")
	if got.Reachable || got.ModelFound {
		t.Fatalf("unreachable case = %+v, want honest unreachable", got)
	}
}

func TestCheckCompatModel_keyRidesFromEnvAnd401SetsNeedsKey(t *testing.T) {
	s := fakeModelsServer(t, "sk-real", "qwen3-4b")
	d := testDesktop(testController())

	// No key in the environment → 401 → needsKey, honestly reachable.
	got := d.CheckCompatModel(s.URL+"/v1", "qwen3-4b", "N1_TEST_API_KEY")
	if !got.Reachable || got.ModelFound || !got.NeedsKey {
		t.Fatalf("keyless case = %+v, want reachable+needsKey", got)
	}
	if got.Detail != "" && containsAny(got.Detail, "sk-real") {
		t.Fatalf("detail leaked a value: %q", got.Detail)
	}

	// The env key rides as the Bearer.
	t.Setenv("N1_TEST_API_KEY", "sk-real")
	got = d.CheckCompatModel(s.URL+"/v1", "qwen3-4b", "N1_TEST_API_KEY")
	if !got.Reachable || !got.ModelFound || got.NeedsKey {
		t.Fatalf("keyed case = %+v, want found via the env Bearer", got)
	}
	if containsAny(got.Detail, "sk-real") {
		t.Fatalf("detail leaked the key: %q", got.Detail)
	}
}

func containsAny(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestApplyCompatFirstRun_rewritesTheTemplateBrainValidated(t *testing.T) {
	// Provision the REAL first-run template into a sandbox path.
	path := filepath.Join(t.TempDir(), "korvun.json")
	if created, err := EnsureDefaultConfig(path); err != nil || !created {
		t.Fatalf("EnsureDefaultConfig: created=%v err=%v", created, err)
	}
	d := testDesktop(testController(), withConfigPathFallback(func() (string, error) {
		return path, nil
	}))

	if err := d.ApplyCompatFirstRun("http://localhost:1234/v1", "qwen3-4b", "MY_COMPAT_KEY", "local"); err != nil {
		t.Fatalf("ApplyCompatFirstRun: %v", err)
	}

	// The applied file VALIDATES (config.Load runs Validate) and carries
	// exactly the compat entry.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("applied config does not load/validate: %v", err)
	}
	if len(cfg.Brains) != 1 || len(cfg.Brains[0].Models) != 1 {
		t.Fatalf("brains/models shape = %d/%d, want 1/1", len(cfg.Brains), len(cfg.Brains[0].Models))
	}
	m := cfg.Brains[0].Models[0]
	if m.Provider != "openai-compatible" || m.ModelID != "qwen3-4b" ||
		m.BaseURL != "http://localhost:1234/v1" || m.APIKeyEnv != "MY_COMPAT_KEY" || m.Locality != "local" {
		t.Fatalf("compat entry = %+v, want the four mold fields", m)
	}
}

func TestApplyCompatFirstRun_invalidShapeRefusesLoudly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.json")
	if _, err := EnsureDefaultConfig(path); err != nil {
		t.Fatalf("EnsureDefaultConfig: %v", err)
	}
	d := testDesktop(testController(), withConfigPathFallback(func() (string, error) {
		return path, nil
	}))
	// A relative base_url must be refused by config.Validate — and the file
	// on disk must stay the untouched template.
	if err := d.ApplyCompatFirstRun("localhost:1234/v1", "qwen3-4b", "", "local"); err == nil {
		t.Fatalf("ApplyCompatFirstRun with a scheme-less base_url = nil error, want the validation refusal")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("template no longer loads after the refused apply: %v", err)
	}
	if cfg.Brains[0].Models[0].Provider == "openai-compatible" {
		t.Fatalf("the refused apply mutated the on-disk template")
	}
}
