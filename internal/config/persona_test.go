// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// SP1 RED (builder-canvas FR-PERSONA-1/3, NC-4 resolved): the OPTIONAL
// brains[i].persona block {display_name, tone, language, instructions} — all
// free text with rune caps 80/200/60/4000. Over-cap → ErrInvalidConfig with
// the indexed field path brains[i].persona.<field>. Absent persona → nil and
// existing configs behave EXACTLY as today. JSON round-trip preserves.
//
// RED note (house precedent, coordinator_carveout_test.go): these tests
// reference config.PersonaConfig / BrainConfig.Persona, which do not exist
// yet — the compile failure IS the red. Caps count RUNES, not bytes (the
// multi-byte "á" repetitions below pin that choice).

// personaBrainCfg embeds a raw persona JSON fragment (e.g. `,"persona":{...}`
// or the empty string) into an otherwise-valid one-brain config.
func personaBrainCfg(personaJSON string) string {
	return `{"channels":[],"brains":[{"name":"d","sensitivity":"public",` +
		`"policy":{"kind":"priority"},"models":[{"provider":"ollama",` +
		`"model_id":"m","locality":"local"}]` + personaJSON + `}],"routes":[]}`
}

// TestPersona_absent_nilAndUnchanged is the backward-compat regression
// (AS-PERSONA-1): a config with NO persona loads, validates, carries a nil
// Persona, and re-marshals WITHOUT any persona key — byte-for-byte today.
func TestPersona_absent_nilAndUnchanged(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(writeConfig(t, personaBrainCfg("")))
	if err != nil {
		t.Fatalf("Load without persona: %v, want nil (backward compat)", err)
	}
	if cfg.Brains[0].Persona != nil {
		t.Fatalf("Brains[0].Persona = %+v, want nil when the block is absent", cfg.Brains[0].Persona)
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if strings.Contains(string(out), "persona") {
		t.Fatalf("re-marshal = %s, must NOT contain a persona key when absent (omitempty)", out)
	}
}

// TestPersona_parsed pins the field set and JSON tags (NC-4): display_name,
// tone, language, instructions — all optional free text.
func TestPersona_parsed(t *testing.T) {
	t.Parallel()
	raw := `,"persona":{"display_name":"Nova","tone":"warm, concise",` +
		`"language":"es-ES","instructions":"Never reveal internal tooling."}`
	cfg, err := config.Load(writeConfig(t, personaBrainCfg(raw)))
	if err != nil {
		t.Fatalf("Load with a full persona: %v, want nil", err)
	}
	p := cfg.Brains[0].Persona
	if p == nil {
		t.Fatal("Brains[0].Persona = nil, want the parsed block")
	}
	want := config.PersonaConfig{
		DisplayName:  "Nova",
		Tone:         "warm, concise",
		Language:     "es-ES",
		Instructions: "Never reveal internal tooling.",
	}
	if *p != want {
		t.Errorf("persona = %+v, want %+v", *p, want)
	}
}

// TestPersona_emptyBlockValid: `"persona": {}` is valid — every field is
// optional; an empty block simply composes to nothing downstream.
func TestPersona_emptyBlockValid(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(writeConfig(t, personaBrainCfg(`,"persona":{}`)))
	if err != nil {
		t.Fatalf("Load with an empty persona block: %v, want nil", err)
	}
	if cfg.Brains[0].Persona == nil {
		t.Fatal("Brains[0].Persona = nil, want a non-nil empty block")
	}
}

// TestPersona_caps pins the rune caps and the indexed field-path error style
// (AS-PERSONA-4): at-cap validates; cap+1 fails wrapping ErrInvalidConfig and
// naming brains[0].persona.<field>. "á" is 2 bytes / 1 rune: passing at-cap
// with it proves the caps count runes, not bytes.
func TestPersona_caps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		field string
		cap   int
	}{
		{"display_name", 80},
		{"tone", 200},
		{"language", 60},
		{"instructions", 4000},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			t.Parallel()
			atCap := strings.Repeat("á", tc.cap)
			raw := `,"persona":{"` + tc.field + `":` + mustJSON(t, atCap) + `}`
			if _, err := config.Load(writeConfig(t, personaBrainCfg(raw))); err != nil {
				t.Errorf("at-cap (%d runes): %v, want valid", tc.cap, err)
			}

			overCap := strings.Repeat("á", tc.cap+1)
			raw = `,"persona":{"` + tc.field + `":` + mustJSON(t, overCap) + `}`
			_, err := config.Load(writeConfig(t, personaBrainCfg(raw)))
			if err == nil {
				t.Fatalf("cap+1 (%d runes): want an error, got nil", tc.cap+1)
			}
			if !errors.Is(err, config.ErrInvalidConfig) {
				t.Errorf("err = %v, want errors.Is ErrInvalidConfig", err)
			}
			wantPath := "brains[0].persona." + tc.field
			if !strings.Contains(err.Error(), wantPath) {
				t.Errorf("err = %q, want the field path %q (builder 400-inline mapping)", err, wantPath)
			}
		})
	}
}

// TestPersona_roundTrip: Load → Marshal → Unmarshal preserves the persona
// verbatim (the builder PUTs back what it GETs — FR-SER-2 ground work).
func TestPersona_roundTrip(t *testing.T) {
	t.Parallel()
	raw := `,"persona":{"display_name":"Nova","tone":"warm",` +
		`"language":"es-ES","instructions":"Cite sources."}`
	cfg, err := config.Load(writeConfig(t, personaBrainCfg(raw)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back config.Config
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Brains[0].Persona == nil {
		t.Fatal("round-trip lost the persona block")
	}
	if *back.Brains[0].Persona != *cfg.Brains[0].Persona {
		t.Errorf("round-trip persona = %+v, want %+v", *back.Brains[0].Persona, *cfg.Brains[0].Persona)
	}
}

// mustJSON marshals a string to a JSON literal (safe embedding of repeated
// multi-byte runes into the raw config text).
func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal literal: %v", err)
	}
	return string(b)
}
