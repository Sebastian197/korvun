// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// This file pins the CONFIG-SCHEMA contract of ADR-0031 sub-phase 6 (boot
// warmup, Decision 1b): the per-model `warmup` toggle and its fail-loud
// validation on a cloud model (FR-C1/FR-C2).
//
// RED note: ModelConfig.Warmup does not exist yet, so referencing it fails the
// config_test build. That compile failure IS the red for the schema addition;
// GREEN adds the bool field and the Validate rule.

// TestLoad_warmupParses pins the wire shape: `warmup:true` on a local model
// parses to Warmup==true; an omitted key is false (FR-C1).
func TestLoad_warmupParses(t *testing.T) {
	t.Parallel()
	js := `{
	  "channels":[{"type":"telegram","mode":"polling","token_env":"T"}],
	  "brains":[{
	    "name":"d","sensitivity":"public","dispatch":"fanout",
	    "policy":{"kind":"priority","order":["ollama"]},
	    "models":[
	      {"provider":"ollama","model_id":"m","locality":"local","warmup":true},
	      {"provider":"ollama","model_id":"n","locality":"local"}
	    ]
	  }],
	  "routes":[{"channel":"telegram","brain":"d"}]
	}`
	cfg, err := config.Load(writeConfig(t, js))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Brains[0].Models[0].Warmup {
		t.Errorf("models[0].Warmup = false, want true")
	}
	if cfg.Brains[0].Models[1].Warmup {
		t.Errorf("models[1].Warmup = true, want false (absent)")
	}
}

// TestValidate_warmupOnCloudRejected pins FR-C2: warmup:true on a cloud model is
// a fail-loud config error naming brains[i].models[j].warmup (a warmup call to a
// cloud model bills the user real money for no cold-load benefit). warmup:true on
// a local model, and an absent warmup on either locality, are all fine.
func TestValidate_warmupOnCloudRejected(t *testing.T) {
	t.Parallel()

	cloudModel := func(warmup string) string {
		return `{
		  "channels":[{"type":"telegram","mode":"polling","token_env":"T"}],
		  "brains":[{
		    "name":"d","sensitivity":"public","dispatch":"fanout",
		    "policy":{"kind":"priority","order":["groq"]},
		    "models":[{"provider":"groq","model_id":"g","locality":"cloud","api_key_env":"GROQ_API_KEY"` + warmup + `}]
		  }],
		  "routes":[{"channel":"telegram","brain":"d"}]
		}`
	}
	localModel := func(warmup string) string {
		return `{
		  "channels":[{"type":"telegram","mode":"polling","token_env":"T"}],
		  "brains":[{
		    "name":"d","sensitivity":"public","dispatch":"fanout",
		    "policy":{"kind":"priority","order":["ollama"]},
		    "models":[{"provider":"ollama","model_id":"m","locality":"local"` + warmup + `}]
		  }],
		  "routes":[{"channel":"telegram","brain":"d"}]
		}`
	}

	t.Run("cloud + warmup:true rejected", func(t *testing.T) {
		_, err := config.Load(writeConfig(t, cloudModel(`,"warmup":true`)))
		if !errors.Is(err, config.ErrInvalidConfig) {
			t.Fatalf("err = %v, want ErrInvalidConfig", err)
		}
		if !strings.Contains(err.Error(), "warmup") {
			t.Errorf("err = %v, want it to name the warmup field", err)
		}
	})
	t.Run("cloud + warmup absent is allowed", func(t *testing.T) {
		if _, err := config.Load(writeConfig(t, cloudModel(``))); err != nil {
			t.Errorf("Load = %v, want nil (no warmup on cloud is fine)", err)
		}
	})
	t.Run("local + warmup:true is allowed", func(t *testing.T) {
		if _, err := config.Load(writeConfig(t, localModel(`,"warmup":true`))); err != nil {
			t.Errorf("Load = %v, want nil (warmup on local is the whole point)", err)
		}
	})
	t.Run("local + warmup absent is allowed", func(t *testing.T) {
		if _, err := config.Load(writeConfig(t, localModel(``))); err != nil {
			t.Errorf("Load = %v, want nil", err)
		}
	})
}
