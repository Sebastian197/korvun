// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
)

// This file pins the CONFIG-SCHEMA contract of ADR-0031 sub-phase 1
// (Decisions 2 and 3): the per-model request_timeout with a top-level default,
// the per-model max_retries, the per-brain retry toggle, the optional
// brain_handler_timeout override, and the fail-loud validation of each —
// including the SV2 rule that an explicit retry:true on a sequential brain is
// rejected at config load.
//
// RED note: none of the referenced fields/methods exist yet, so the package
// test binary fails to build. That compile failure IS the red for the schema
// additions; GREEN adds the fields, the DefaultRequestTimeout constant, the
// EffectiveRequestTimeout resolver, and the Validate rules.

// TestLoad_resilienceFieldsParse pins the wire shape: request_timeout at both
// the top level and per-model, max_retries per-model, and the per-brain retry
// toggle, each under its documented JSON key (ADR-0031 Decision 3).
func TestLoad_resilienceFieldsParse(t *testing.T) {
	t.Parallel()
	js := `{
	  "request_timeout": "120s",
	  "channels": [{"type":"telegram","mode":"polling","token_env":"T"}],
	  "brains": [{
	    "name":"d","sensitivity":"public","dispatch":"fanout","retry":false,
	    "policy":{"kind":"priority","order":["ollama","groq"]},
	    "models":[
	      {"provider":"ollama","model_id":"m","locality":"local","request_timeout":"15s","max_retries":2},
	      {"provider":"groq","model_id":"g","locality":"cloud","api_key_env":"GROQ_API_KEY"}
	    ]
	  }],
	  "routes":[{"channel":"telegram","brain":"d"}]
	}`
	cfg, err := config.Load(writeConfig(t, js))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RequestTimeout != "120s" {
		t.Errorf("top-level RequestTimeout = %q, want %q", cfg.RequestTimeout, "120s")
	}
	m0, m1 := cfg.Brains[0].Models[0], cfg.Brains[0].Models[1]
	if m0.RequestTimeout != "15s" {
		t.Errorf("models[0].RequestTimeout = %q, want %q", m0.RequestTimeout, "15s")
	}
	if m0.MaxRetries != 2 {
		t.Errorf("models[0].MaxRetries = %d, want 2", m0.MaxRetries)
	}
	if m1.RequestTimeout != "" {
		t.Errorf("models[1].RequestTimeout = %q, want empty (inherits top-level default)", m1.RequestTimeout)
	}
	// retry is a *bool so absent (default-on) is distinguishable from explicit false.
	if cfg.Brains[0].Retry == nil || *cfg.Brains[0].Retry != false {
		t.Errorf("brains[0].Retry = %v, want explicit false", cfg.Brains[0].Retry)
	}
}

// TestLoad_retryAbsentIsNil pins the default-on semantics: an omitted retry key
// leaves the pointer nil, which the app reads as "on" (ADR-0031 Decision 3).
func TestLoad_retryAbsentIsNil(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(writeConfig(t, validMinimal))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Brains[0].Retry != nil {
		t.Errorf("Retry = %v, want nil (absent => default on)", cfg.Brains[0].Retry)
	}
}

// TestEffectiveRequestTimeout_precedence pins the resolver: per-model overrides
// top-level, top-level overrides the package default, and an all-empty config
// falls back to DefaultRequestTimeout (ADR-0031 Decision 3 — "a top-level
// default that a per-model value overrides").
func TestEffectiveRequestTimeout_precedence(t *testing.T) {
	t.Parallel()

	perModel := config.ModelConfig{Provider: "ollama", ModelID: "m", Locality: "local", RequestTimeout: "15s"}
	noPerModel := config.ModelConfig{Provider: "ollama", ModelID: "m", Locality: "local"}

	tests := []struct {
		name        string
		topLevel    string
		model       config.ModelConfig
		wantSeconds float64
	}{
		{"per-model wins over top-level", "90s", perModel, 15},
		{"top-level used when per-model empty", "90s", noPerModel, 90},
		{"package default when both empty", "", noPerModel, config.DefaultRequestTimeout.Seconds()},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &config.Config{RequestTimeout: tc.topLevel}
			got := c.EffectiveRequestTimeout(tc.model)
			if got != time.Duration(tc.wantSeconds*float64(time.Second)) {
				t.Errorf("EffectiveRequestTimeout = %v, want %vs", got, tc.wantSeconds)
			}
		})
	}
}

// TestValidate_requestTimeoutRejectsBad pins fail-loud on a bad duration at
// either level: unparseable or non-positive. A zero/negative timeout is a
// guillotine, so it is rejected exactly like an unparseable one (ADR-0031
// Decision 2 — "never silently guillotine a model").
func TestValidate_requestTimeoutRejectsBad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		json      string
		wantField string
	}{
		{
			name:      "top-level unparseable",
			json:      `{"request_timeout":"notaduration","channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "request_timeout",
		},
		{
			name:      "top-level non-positive",
			json:      `{"request_timeout":"0s","channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "request_timeout",
		},
		{
			name:      "per-model unparseable",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local","request_timeout":"12x"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "request_timeout",
		},
		{
			name:      "per-model non-positive",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local","request_timeout":"-1s"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "request_timeout",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(writeConfig(t, tc.json))
			if !errors.Is(err, config.ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.wantField) {
				t.Errorf("err = %v, want it to name %q", err, tc.wantField)
			}
		})
	}
}

// TestValidate_maxRetriesRejectsNegative pins that a negative retry count fails
// loud, naming the field (ADR-0031 Decision 3; 0 disables, which stays valid).
func TestValidate_maxRetriesRejectsNegative(t *testing.T) {
	t.Parallel()
	js := `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local","max_retries":-1}]}],"routes":[{"channel":"telegram","brain":"d"}]}`
	_, err := config.Load(writeConfig(t, js))
	if !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
	if !strings.Contains(err.Error(), "max_retries") {
		t.Errorf("err = %v, want it to name max_retries", err)
	}
}

// TestValidate_explicitRetryTrueOnSequentialFailsLoud pins SV2 at config load:
// a sequential brain with an explicit retry:true is rejected (retry is off by
// construction for sequential — the fail-over IS the retry story, so enabling
// it would multiply the serial worst case). The controls prove the rule is
// surgical: retry:true on a fan-out brain is fine, and retry:false / absent on
// a sequential brain is fine.
func TestValidate_explicitRetryTrueOnSequentialFailsLoud(t *testing.T) {
	t.Parallel()

	brain := func(dispatch, retry string) string {
		return `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","dispatch":"` + dispatch + `"` + retry + `,"policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`
	}

	t.Run("sequential + retry:true is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := config.Load(writeConfig(t, brain("sequential", `,"retry":true`)))
		if !errors.Is(err, config.ErrInvalidConfig) {
			t.Fatalf("err = %v, want ErrInvalidConfig", err)
		}
		if !strings.Contains(err.Error(), "retry") {
			t.Errorf("err = %v, want it to name retry", err)
		}
	})

	valid := []struct {
		name     string
		dispatch string
		retry    string
	}{
		{"fan-out + retry:true is allowed", "fanout", `,"retry":true`},
		{"sequential + retry:false is allowed", "sequential", `,"retry":false`},
		{"sequential + retry absent is allowed", "sequential", ``},
	}
	for _, tc := range valid {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := config.Load(writeConfig(t, brain(tc.dispatch, tc.retry))); err != nil {
				t.Errorf("Load rejected a valid config: %v", err)
			}
		})
	}
}

// TestValidate_brainHandlerTimeoutRejectsBadDuration pins that the optional
// explicit ceiling override, when present, must parse. Whether it is >= the
// derived ceiling is checked later at boot (app.Build), not here — this is the
// pure schema check (ADR-0031 Decision 2).
func TestValidate_brainHandlerTimeoutRejectsBadDuration(t *testing.T) {
	t.Parallel()
	js := `{"brain_handler_timeout":"nope","channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`
	_, err := config.Load(writeConfig(t, js))
	if !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
	if !strings.Contains(err.Error(), "brain_handler_timeout") {
		t.Errorf("err = %v, want it to name brain_handler_timeout", err)
	}
}
