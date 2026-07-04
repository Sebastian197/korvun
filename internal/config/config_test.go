// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// validMinimal is the smallest config that passes Validate: one Telegram
// polling channel, one public brain with a priority policy over Ollama + Groq,
// and a route binding them. Secrets are env-var references, never values.
const validMinimal = `{
  "channels": [
    {"type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN"}
  ],
  "brains": [
    {
      "name": "default",
      "sensitivity": "public",
      "dispatch": "fanout",
      "policy": {"kind": "priority", "order": ["ollama", "groq"]},
      "models": [
        {"provider": "ollama", "model_id": "llama3.2", "locality": "local", "base_url": "http://localhost:11434"},
        {"provider": "groq", "model_id": "llama-3.3-70b-versatile", "locality": "cloud", "api_key_env": "GROQ_API_KEY"}
      ]
    }
  ],
  "routes": [
    {"channel": "telegram", "brain": "default"}
  ]
}`

// writeConfig writes content to a temp file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_validMinimal(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(writeConfig(t, validMinimal))
	if err != nil {
		t.Fatalf("Load valid config: %v", err)
	}
	if len(cfg.Channels) != 1 || cfg.Channels[0].Type != "telegram" || cfg.Channels[0].TokenEnv != "TELEGRAM_BOT_TOKEN" {
		t.Errorf("channels parsed wrong: %+v", cfg.Channels)
	}
	if len(cfg.Brains) != 1 || cfg.Brains[0].Name != "default" || cfg.Brains[0].Sensitivity != "public" {
		t.Errorf("brains parsed wrong: %+v", cfg.Brains)
	}
	if cfg.Brains[0].Policy.Kind != "priority" || len(cfg.Brains[0].Policy.Order) != 2 {
		t.Errorf("policy parsed wrong: %+v", cfg.Brains[0].Policy)
	}
	if len(cfg.Brains[0].Models) != 2 || cfg.Brains[0].Models[1].APIKeyEnv != "GROQ_API_KEY" {
		t.Errorf("models parsed wrong: %+v", cfg.Brains[0].Models)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Channel != "telegram" || cfg.Routes[0].Brain != "default" {
		t.Errorf("routes parsed wrong: %+v", cfg.Routes)
	}
}

func TestLoad_fileErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
		if !errors.Is(err, config.ErrConfigRead) {
			t.Errorf("err = %v, want ErrConfigRead", err)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		t.Parallel()
		_, err := config.Load(writeConfig(t, `{"channels": [ this is not json `))
		if !errors.Is(err, config.ErrConfigParse) {
			t.Errorf("err = %v, want ErrConfigParse", err)
		}
	})
}

// TestValidate_agentConfigValid proves a well-formed agent block passes Validate
// (ADR-0021): the brain mounts a tool-use AgentBrain instead of the Orchestrator.
func TestValidate_agentConfigValid(t *testing.T) {
	t.Parallel()
	js := `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}],"agent":{"tools":["calc","echo","time"],"max_iterations":4,"system_prompt":"be terse"}}],"routes":[{"channel":"telegram","brain":"d"}]}`
	path := writeConfig(t, js)
	if _, err := config.Load(path); err != nil {
		t.Fatalf("valid agent config rejected: %v", err)
	}
}

// TestValidate_fieldErrors drives one mutation per offending field and asserts
// the error wraps ErrInvalidConfig and NAMES the field (ADR-0017 §5).
func TestValidate_fieldErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		json      string
		wantField string // a substring the error message must name
	}{
		{
			name:      "no channels",
			json:      `{"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "channels",
		},
		{
			name:      "unknown channel type",
			json:      `{"channels":[{"type":"discord","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "channels[0].type",
		},
		{
			name:      "missing token_env",
			json:      `{"channels":[{"type":"telegram","mode":"polling"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "channels[0].token_env",
		},
		{
			name:      "unknown sensitivity",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"secret","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].sensitivity",
		},
		{
			name:      "unknown policy kind",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"vote"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].policy.kind",
		},
		{
			name:      "unknown dispatch",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","dispatch":"broadcast","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].dispatch",
		},
		{
			name:      "no models",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models",
		},
		{
			name:      "unknown provider",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"openai","model_id":"m","locality":"cloud","api_key_env":"K"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models[0].provider",
		},
		{
			name:      "missing model_id",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models[0].model_id",
		},
		{
			name:      "unknown locality",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"edge"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models[0].locality",
		},
		{
			name:      "cloud provider missing api_key_env",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"groq","model_id":"m","locality":"cloud"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models[0].api_key_env",
		},
		{
			name:      "agent with empty tools",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}],"agent":{"tools":[]}}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].agent.tools",
		},
		{
			name:      "agent with negative max_iterations",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}],"agent":{"tools":["calc"],"max_iterations":-1}}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].agent.max_iterations",
		},
		{
			name:      "empty channel type",
			json:      `{"channels":[{"type":"","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "channels[0].type",
		},
		{
			name:      "missing channel mode",
			json:      `{"channels":[{"type":"telegram","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "channels[0].mode",
		},
		{
			name:      "missing brain name",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].name",
		},
		{
			name:      "duplicate brain name",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]},{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[1].name",
		},
		{
			name:      "missing sensitivity",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].sensitivity",
		},
		{
			name:      "missing policy kind",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].policy.kind",
		},
		{
			name:      "missing provider",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models[0].provider",
		},
		{
			name:      "missing locality",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains[0].models[0].locality",
		},
		{
			name:      "no brains",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[],"routes":[{"channel":"telegram","brain":"d"}]}`,
			wantField: "brains",
		},
		{
			name:      "no routes",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[]}`,
			wantField: "routes",
		},
		{
			name:      "dangling route channel",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"nope","brain":"d"}]}`,
			wantField: "routes[0].channel",
		},
		{
			name:      "dangling route brain",
			json:      `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"missing"}]}`,
			wantField: "routes[0].brain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(writeConfig(t, tt.json))
			if !errors.Is(err, config.ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("error %q does not name the offending field %q", err.Error(), tt.wantField)
			}
		})
	}
}

// TestValidate_adminBlock covers the optional admin block (ADR-0028 §1): absent is
// valid (read-only default), a present block must name a non-empty token_env (the
// env-var NAME, not the value), and an empty token_env is a named schema error.
func TestValidate_adminBlock(t *testing.T) {
	t.Parallel()
	const base = `"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"public","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]`

	t.Run("absent admin block is valid (read-only default)", func(t *testing.T) {
		if _, err := config.Load(writeConfig(t, "{"+base+"}")); err != nil {
			t.Errorf("absent admin block should be valid, got %v", err)
		}
	})
	t.Run("admin token_env present is valid", func(t *testing.T) {
		cfg, err := config.Load(writeConfig(t, "{"+base+`,"admin":{"token_env":"KORVUN_ADMIN_TOKEN"}}`))
		if err != nil {
			t.Fatalf("valid admin block rejected: %v", err)
		}
		if cfg.Admin == nil || cfg.Admin.TokenEnv != "KORVUN_ADMIN_TOKEN" {
			t.Fatalf("admin block not parsed: %+v", cfg.Admin)
		}
	})
	t.Run("admin token_env empty is rejected", func(t *testing.T) {
		_, err := config.Load(writeConfig(t, "{"+base+`,"admin":{"token_env":""}}`))
		if !errors.Is(err, config.ErrInvalidConfig) {
			t.Fatalf("err = %v, want ErrInvalidConfig", err)
		}
		if !strings.Contains(err.Error(), "admin.token_env") {
			t.Errorf("error %q does not name admin.token_env", err.Error())
		}
	})
}

// TestValidate_emptyDispatchAllowed confirms an omitted dispatch is valid
// (it defaults to fan-out in internal/app), so a minimal config need not
// spell it out.
func TestValidate_emptyDispatchAllowed(t *testing.T) {
	t.Parallel()
	j := `{"channels":[{"type":"telegram","mode":"polling","token_env":"T"}],"brains":[{"name":"d","sensitivity":"private","policy":{"kind":"priority"},"models":[{"provider":"ollama","model_id":"m","locality":"local"}]}],"routes":[{"channel":"telegram","brain":"d"}]}`
	if _, err := config.Load(writeConfig(t, j)); err != nil {
		t.Errorf("omitted dispatch should be valid, got %v", err)
	}
}
