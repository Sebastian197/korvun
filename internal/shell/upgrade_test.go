// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Red for the upgrade half of first-run provisioning (operator-console
// close, 2026-08-08): a config written before the chat piece lacks the
// storage and session blocks, so after an app upgrade the Chat tab would
// boot dead — no store, no sessions. EnsureChatBlocks adds exactly the
// missing blocks, preserves everything else (including fields this build
// does not know about), touches nothing when both are present, and the
// embedded first-run template ships chat-alive so NEW installs never need
// the upgrade at all. The desktop binding composes both halves on mount.
package shell

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// preChatConfig is a minimal valid pre-upgrade config (no storage, no
// session) carrying a field this build does not know — the upgrade must
// pass it through verbatim, not silently drop it.
const preChatConfig = `{
  "channels": [],
  "brains": [
    {
      "name": "default",
      "sensitivity": "private",
      "policy": {"kind": "priority", "order": ["ollama"]},
      "dispatch": "fanout",
      "models": [
        {
          "provider": "ollama",
          "model_id": "llama3.2:1b",
          "locality": "local",
          "base_url": "http://localhost:11434",
          "api_key_env": ""
        }
      ]
    }
  ],
  "routes": [],
  "x_future": {"keep": true}
}`

func writeFixtureConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	return path
}

func readTopLevel(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read upgraded config: %v", err)
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("upgraded config is not a JSON object: %v", err)
	}
	return m
}

func TestEnsureChatBlocks_addsBothWhenMissing(t *testing.T) {
	path := writeFixtureConfig(t, preChatConfig)

	changed, err := EnsureChatBlocks(path)
	if err != nil || !changed {
		t.Fatalf("EnsureChatBlocks = (%v, %v), want (true, nil)", changed, err)
	}

	m := readTopLevel(t, path)
	if _, ok := m["storage"]; !ok {
		t.Fatal("storage block not provisioned")
	}
	if _, ok := m["session"]; !ok {
		t.Fatal("session block not provisioned")
	}
	if !strings.Contains(string(m["x_future"]), "keep") {
		t.Fatalf("unknown field dropped by the upgrade: x_future = %q", m["x_future"])
	}

	// The upgraded file must load AND validate as a real config whose chat
	// blocks are live (defaults: DB path and triggers resolve at boot).
	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	data, _ := os.ReadFile(path)
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("upgraded config does not parse as config.Config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("upgraded config does not validate: %v", err)
	}
	if cfg.Storage == nil || cfg.Session == nil {
		t.Fatalf("upgraded config: Storage=%v Session=%v, want both present",
			cfg.Storage, cfg.Session)
	}
}

func TestEnsureChatBlocks_noopWhenBothPresent(t *testing.T) {
	// Steady state keeps FR-FIRST-2's spirit: not even an identical rewrite.
	body := `{"channels":[],"brains":[],"routes":[],"storage":{"path":"custom.db"},"session":{"triggers":["/n"]}}`
	path := writeFixtureConfig(t, body)

	changed, err := EnsureChatBlocks(path)
	if err != nil || changed {
		t.Fatalf("EnsureChatBlocks = (%v, %v), want (false, nil)", changed, err)
	}
	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after no-op: %v", err)
	}
	if !bytes.Equal(after, []byte(body)) {
		t.Fatal("no-op rewrote the file: bytes differ from the original")
	}
}

func TestEnsureChatBlocks_addsOnlyMissing(t *testing.T) {
	body := `{
  "channels": [],
  "brains": [],
  "routes": [],
  "storage": {"path": "custom.db"}
}`
	path := writeFixtureConfig(t, body)

	changed, err := EnsureChatBlocks(path)
	if err != nil || !changed {
		t.Fatalf("EnsureChatBlocks = (%v, %v), want (true, nil)", changed, err)
	}
	m := readTopLevel(t, path)
	if !strings.Contains(string(m["storage"]), "custom.db") {
		t.Fatalf("present storage block edited by the upgrade: %q", m["storage"])
	}
	if _, ok := m["session"]; !ok {
		t.Fatal("missing session block not provisioned")
	}
}

func TestEnsureChatBlocks_missingFile(t *testing.T) {
	// The binding runs EnsureDefaultConfig first, so a missing file here is
	// a real fault, not a first run — it must surface, not be masked.
	changed, err := EnsureChatBlocks(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil || changed {
		t.Fatalf("EnsureChatBlocks on a missing file = (%v, %v), want (false, error)", changed, err)
	}
}

func TestEnsureChatBlocks_invalidJSON(t *testing.T) {
	path := writeFixtureConfig(t, "not json {")
	changed, err := EnsureChatBlocks(path)
	if err == nil || changed {
		t.Fatalf("EnsureChatBlocks on invalid JSON = (%v, %v), want (false, error)", changed, err)
	}
	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	after, _ := os.ReadFile(path)
	if string(after) != "not json {" {
		t.Fatal("invalid file was modified — the upgrade must leave it untouched")
	}
}

func TestFirstRunTemplate_includesChatBlocks(t *testing.T) {
	// New installs never need the upgrade: the template ships chat-alive.
	var cfg config.Config
	if err := json.Unmarshal(firstRunTemplate, &cfg); err != nil {
		t.Fatalf("parse embedded template: %v", err)
	}
	if cfg.Storage == nil {
		t.Fatal("first-run template lacks the storage block — Chat tab born dead on a fresh install")
	}
	if cfg.Session == nil {
		t.Fatal("first-run template lacks the session block — /new and /reset dead on a fresh install")
	}
}

// The binding half: the chrome's single mount call must heal an EXISTING
// pre-chat config, not only provision fresh installs.
func TestDesktop_ensureDefaultConfig_upgradesExisting(t *testing.T) {
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin", "linux":
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", "") // linux: fall through to HOME/.config
	case "windows":
		t.Setenv("AppData", tmp)
	}
	d := NewDesktop(testController(), WithDesktopLogger(slog.New(slog.DiscardHandler)))

	path, err := d.DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(preChatConfig), 0o600); err != nil {
		t.Fatalf("seed pre-chat config: %v", err)
	}

	created, err := d.EnsureDefaultConfig()
	if err != nil || created {
		t.Fatalf("EnsureDefaultConfig over existing = (%v, %v), want (false, nil)", created, err)
	}
	m := readTopLevel(t, path)
	if _, ok := m["storage"]; !ok {
		t.Fatal("existing config not upgraded: storage block still missing after mount")
	}
	if _, ok := m["session"]; !ok {
		t.Fatal("existing config not upgraded: session block still missing after mount")
	}
}
