// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file pins the STRICT-SCHEMA contract (audit finding A-1): unknown
// keys are rejected loudly, NAMING the offending key, instead of being
// silently ignored; and the optional schema_version field is validated
// (absent == 1; anything else is a named refusal). The `_comment` root key
// is the one sanctioned annotation slot (three shipped configs use it) and
// must keep loading.

// loadFromLiteral writes literal to a temp file and runs Load on it.
func loadFromLiteral(t *testing.T, literal string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.json")
	if err := os.WriteFile(path, []byte(literal), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return Load(path)
}

// strictValidBase is a minimal config that passes Validate, used as the mutation
// baseline for the unknown-key table. Storage + session + observability +
// admin + a webhook channel are present so every nested block is exercised.
const strictValidBase = `{
  "channels": [
    {"type": "telegram", "mode": "polling", "token_env": "KORVUN_STRICT_TG"},
    {"type": "webhook", "token_env": "KORVUN_STRICT_WH",
     "webhook": {"outbound_url": "http://127.0.0.1:9/out"}}
  ],
  "brains": [{
    "name": "default", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "llama-3.3-70b-versatile",
                "locality": "cloud", "api_key_env": "KORVUN_STRICT_KEY"}]
  }],
  "routes": [{"channel": "telegram", "brain": "default"}],
  "storage": {"path": ""},
  "session": {"idle_min": 5},
  "observability": {"enabled": false},
  "admin": {"token_env": "KORVUN_STRICT_ADMIN"}
}`

func TestLoad_RejectsUnknownKeysNamingThem(t *testing.T) {
	cases := []struct {
		name    string
		literal string
		key     string // the unknown key the error must name
	}{
		{
			name: "root level typo",
			literal: `{
  "sesion": {"idle_min": 5},
  "channels": [{"type": "telegram", "mode": "polling", "token_env": "T"}],
  "brains": [{"name": "b", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "telegram", "brain": "b"}]
}`,
			key: "sesion",
		},
		{
			name: "channel entry typo",
			literal: `{
  "channels": [{"type": "telegram", "mode": "polling", "tokenenv": "T"}],
  "brains": [{"name": "b", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "telegram", "brain": "b"}]
}`,
			key: "tokenenv",
		},
		{
			name: "brain entry typo",
			literal: `{
  "channels": [{"type": "telegram", "mode": "polling", "token_env": "T"}],
  "brains": [{"name": "b", "sensitivty": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "telegram", "brain": "b"}]
}`,
			key: "sensitivty",
		},
		{
			name: "observability block typo",
			literal: `{
  "channels": [{"type": "telegram", "mode": "polling", "token_env": "T"}],
  "brains": [{"name": "b", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "telegram", "brain": "b"}],
  "observability": {"enable": false}
}`,
			key: "enable",
		},
		{
			name: "storage block typo",
			literal: `{
  "channels": [{"type": "telegram", "mode": "polling", "token_env": "T"}],
  "brains": [{"name": "b", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "telegram", "brain": "b"}],
  "storage": {"pth": "/tmp/x.db"}
}`,
			key: "pth",
		},
		{
			name: "session block typo",
			literal: `{
  "channels": [{"type": "telegram", "mode": "polling", "token_env": "T"}],
  "brains": [{"name": "b", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "telegram", "brain": "b"}],
  "storage": {"path": ""},
  "session": {"idle": 5}
}`,
			key: "idle",
		},
		{
			name: "webhook block typo",
			literal: `{
  "channels": [{"type": "webhook", "token_env": "T",
    "webhook": {"outbound_url": "http://127.0.0.1:9/out", "outbound_tokenenv": "X"}}],
  "brains": [{"name": "b", "sensitivity": "public", "dispatch": "fanout",
    "policy": {"kind": "priority", "order": ["groq"]},
    "models": [{"provider": "groq", "model_id": "m", "locality": "cloud", "api_key_env": "K"}]}],
  "routes": [{"channel": "webhook", "brain": "b"}]
}`,
			key: "outbound_tokenenv",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadFromLiteral(t, tc.literal)
			if err == nil {
				t.Fatalf("Load accepted a config with unknown key %q; want a loud refusal", tc.key)
			}
			if !errors.Is(err, ErrUnknownField) {
				t.Fatalf("error = %v; want errors.Is(err, ErrUnknownField)", err)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error %q does not NAME the unknown key %q", err, tc.key)
			}
		})
	}
}

func TestLoad_CommentSlotStillLoads(t *testing.T) {
	literal := strings.Replace(strictValidBase, "{\n", "{\n  \"_comment\": \"sanctioned annotation slot\",\n", 1)
	cfg, err := loadFromLiteral(t, literal)
	if err != nil {
		t.Fatalf("a root _comment must keep loading (three shipped configs use it): %v", err)
	}
	if cfg == nil {
		t.Fatal("nil config on success")
	}
}

func TestLoad_SchemaVersion(t *testing.T) {
	inject := func(v string) string {
		return strings.Replace(strictValidBase, "{\n", "{\n  \"schema_version\": "+v+",\n", 1)
	}

	t.Run("absent means version 1", func(t *testing.T) {
		cfg, err := loadFromLiteral(t, strictValidBase)
		if err != nil {
			t.Fatalf("baseline config must load: %v", err)
		}
		if got := cfg.EffectiveSchemaVersion(); got != 1 {
			t.Fatalf("EffectiveSchemaVersion() = %d for an absent field; want 1", got)
		}
	})

	t.Run("explicit 1 loads", func(t *testing.T) {
		cfg, err := loadFromLiteral(t, inject("1"))
		if err != nil {
			t.Fatalf("schema_version 1 must load: %v", err)
		}
		if got := cfg.EffectiveSchemaVersion(); got != 1 {
			t.Fatalf("EffectiveSchemaVersion() = %d; want 1", got)
		}
	})

	t.Run("unknown future version is a named refusal", func(t *testing.T) {
		_, err := loadFromLiteral(t, inject("2"))
		if err == nil {
			t.Fatal("schema_version 2 must refuse to load")
		}
		if !errors.Is(err, ErrUnsupportedSchemaVersion) {
			t.Fatalf("error = %v; want errors.Is(err, ErrUnsupportedSchemaVersion)", err)
		}
		if !strings.Contains(err.Error(), "2") {
			t.Fatalf("error %q does not name the offending version", err)
		}
	})

	t.Run("negative version is a named refusal", func(t *testing.T) {
		_, err := loadFromLiteral(t, inject("-1"))
		if err == nil {
			t.Fatal("schema_version -1 must refuse to load")
		}
		if !errors.Is(err, ErrUnsupportedSchemaVersion) {
			t.Fatalf("error = %v; want errors.Is(err, ErrUnsupportedSchemaVersion)", err)
		}
	})
}

func TestLoad_TrailingDataStillRejected(t *testing.T) {
	// json.Unmarshal rejected trailing garbage; the strict decoder must not
	// regress that.
	_, err := loadFromLiteral(t, strictValidBase+"\n{\"more\": true}")
	if err == nil {
		t.Fatal("trailing JSON document must refuse to load")
	}
	if !errors.Is(err, ErrConfigParse) {
		t.Fatalf("error = %v; want errors.Is(err, ErrConfigParse)", err)
	}
}

func TestLoad_ValidBaseStillLoads(t *testing.T) {
	cfg, err := loadFromLiteral(t, strictValidBase)
	if err != nil {
		t.Fatalf("the strict decoder must not reject a fully-known config: %v", err)
	}
	if len(cfg.Channels) != 2 || len(cfg.Brains) != 1 {
		t.Fatalf("unexpected shape after load: %d channels, %d brains", len(cfg.Channels), len(cfg.Brains))
	}
}
