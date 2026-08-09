// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
)

// SP5 wiring semantics (ADR-0041, spec FR-CFG-1): the app resolves the
// grown agent block — caged tools get their cages (fail loud without one),
// governance mounts SelectTools inputs with house attrs + declared
// overrides, skills load and inject. Structural validation already happened
// in config; these are the SEMANTIC failures and the happy wiring.

// governedAgentBrain returns a private-brain config with the given agent block.
func governedAgentBrain(a *config.AgentConfig) config.BrainConfig {
	return config.BrainConfig{
		Name:        "agent",
		Sensitivity: "private",
		Policy:      config.PolicyConfig{Kind: "priority"},
		Models:      []config.ModelConfig{{Provider: "ollama", ModelID: "llama3.2", Locality: "local"}},
		Agent:       a,
	}
}

func TestBuildBrain_cagedToolWithoutItsCageFailsLoud(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tool string
		a    *config.AgentConfig
	}{
		{"read_file", &config.AgentConfig{Tools: []string{"read_file"}}},
		{"http_fetch", &config.AgentConfig{Tools: []string{"http_fetch"}}},
		{"webhook_call", &config.AgentConfig{Tools: []string{"webhook_call"}}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()
			_, err := testBuilder().buildBrain(governedAgentBrain(tc.a))
			if err == nil {
				t.Fatalf("caged tool %q wired without its cage", tc.tool)
			}
			if !strings.Contains(err.Error(), tc.tool) {
				t.Fatalf("error %q does not name the tool missing its cage", err.Error())
			}
		})
	}
}

func TestBuildBrain_cagedToolsWireWithTheirCages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := &config.AgentConfig{
		Tools:       []string{"calc", "read_file", "http_fetch", "webhook_call"},
		ReadFile:    &config.ReadFileToolConfig{Root: root},
		HTTPFetch:   &config.HTTPFetchToolConfig{AllowHosts: []string{"example.com"}},
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:5678"}},
	}
	br, err := testBuilder().buildBrain(governedAgentBrain(a))
	if err != nil {
		t.Fatalf("buildBrain: %v", err)
	}
	if _, ok := br.(*brain.AgentBrain); !ok {
		t.Fatalf("got %T, want *brain.AgentBrain", br)
	}
}

func TestBuildBrain_readFileCageWithMissingRootFailsLoud(t *testing.T) {
	t.Parallel()
	a := &config.AgentConfig{
		Tools:    []string{"read_file"},
		ReadFile: &config.ReadFileToolConfig{Root: filepath.Join(t.TempDir(), "missing")},
	}
	if _, err := testBuilder().buildBrain(governedAgentBrain(a)); err == nil {
		t.Fatal("read_file wired against a nonexistent root")
	}
}

// A grant (or an attrs override) naming a tool the brain does not list is a
// misconfiguration, not a silent no-op.
func TestBuildBrain_governanceOverUnlistedToolFailsLoud(t *testing.T) {
	t.Parallel()
	a := &config.AgentConfig{
		Tools:      []string{"calc"},
		Governance: []config.ToolGrantConfig{{Tool: "http_fetch", Mode: "allow"}},
	}
	_, err := testBuilder().buildBrain(governedAgentBrain(a))
	if err == nil || !strings.Contains(err.Error(), "http_fetch") {
		t.Fatalf("err = %v, want a loud failure naming the unlisted tool", err)
	}

	b := &config.AgentConfig{
		Tools:     []string{"calc"},
		ToolAttrs: map[string]config.ToolAttrsConfig{"read_file": {}},
	}
	_, err = testBuilder().buildBrain(governedAgentBrain(b))
	if err == nil || !strings.Contains(err.Error(), "read_file") {
		t.Fatalf("err = %v, want a loud failure naming the unlisted attrs tool", err)
	}
}

// effectiveToolAttrs: house defaults from tool.BuiltinAttrs + declared
// operator overrides (R-2). Overrides are per-field (*bool): an absent field
// keeps the house default.
func TestEffectiveToolAttrs(t *testing.T) {
	t.Parallel()
	no := false
	yes := true
	a := &config.AgentConfig{
		Tools: []string{"calc", "read_file", "http_fetch"},
		ToolAttrs: map[string]config.ToolAttrsConfig{
			"read_file":  {Sensitive: &no},  // operator relaxes the house default
			"http_fetch": {Sensitive: &yes}, // operator hardens beyond it
		},
	}
	attrs, err := effectiveToolAttrs(a)
	if err != nil {
		t.Fatalf("effectiveToolAttrs: %v", err)
	}
	if attrs["read_file"].Sensitive {
		t.Error("read_file override to non-sensitive not applied")
	}
	if !attrs["http_fetch"].Sensitive || !attrs["http_fetch"].Network {
		t.Errorf("http_fetch must keep Network house default and gain Sensitive: %+v", attrs["http_fetch"])
	}
	if attrs["calc"].Sensitive || attrs["calc"].Network {
		t.Errorf("calc must stay zero-attrs: %+v", attrs["calc"])
	}
}

// AS-4 tripwire at the wiring level: yesterday's agent config (pure tools,
// no new blocks) still builds — the growth is strictly additive.
func TestBuildBrain_yesterdaysAgentConfigStillBuilds(t *testing.T) {
	t.Parallel()
	bc := governedAgentBrain(&config.AgentConfig{Tools: []string{"calc", "echo", "time"}, MaxIterations: 4})
	br, err := testBuilder().buildBrain(bc)
	if err != nil {
		t.Fatalf("buildBrain(yesterday's config): %v", err)
	}
	if _, ok := br.(*brain.AgentBrain); !ok {
		t.Fatalf("got %T, want *brain.AgentBrain", br)
	}
}

// Skills wiring: a valid skills dir loads and mounts; a missing dir is a
// loud config error at boot.
func TestBuildBrain_skillsDirWiresAndMissingDirFailsLoud(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	skillDir := filepath.Join(root, "greeting-skill")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	md := "---\nname: greeting-skill\ndescription: Teaches greetings.\n---\nAlways greet warmly.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := &config.AgentConfig{Tools: []string{"calc"}, SkillsDir: root}
	if _, err := testBuilder().buildBrain(governedAgentBrain(a)); err != nil {
		t.Fatalf("buildBrain with skills: %v", err)
	}

	bad := &config.AgentConfig{Tools: []string{"calc"}, SkillsDir: filepath.Join(root, "missing")}
	if _, err := testBuilder().buildBrain(governedAgentBrain(bad)); err == nil {
		t.Fatal("missing skills_dir must fail loud at boot")
	}
}

// The dangerous-name boundary still stands with the grown block.
func TestBuildBrain_shellStillNeverResolves(t *testing.T) {
	t.Parallel()
	a := &config.AgentConfig{Tools: []string{"shell"}}
	_, err := testBuilder().buildBrain(governedAgentBrain(a))
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("err = %v, want ErrUnknownTool", err)
	}
}
