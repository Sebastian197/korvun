// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP3 red (operator-console spec, FR-SESS-3/5 wiring): the session block —
// parsing, defaults, resolution, and the invalid shapes that must FAIL
// CLEARLY at startup (config check / boot), never be silently ignored.
package config

import (
	"errors"
	"testing"
)

func validBase() *Config {
	return &Config{
		Channels: []ChannelConfig{{Type: "telegram", Mode: "polling", TokenEnv: "TG"}},
		Brains: []BrainConfig{{
			Name:        "b",
			Sensitivity: "public",
			Policy:      PolicyConfig{Kind: "priority"},
			Models: []ModelConfig{{
				Provider: "ollama", ModelID: "llama3.2", Locality: "local",
			}},
		}},
		Routes:  []RouteConfig{{Channel: "telegram", Brain: "b"}},
		Storage: &StorageConfig{Path: "/tmp/k.db"},
	}
}

func TestSessionConfig_DefaultsAndResolution(t *testing.T) {
	c := validBase()
	c.Session = &SessionConfig{DailyAt: "04:30", IdleMin: 45}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	s := c.SessionSettings()
	if len(s.Triggers) != 2 || s.Triggers[0] != "/new" || s.Triggers[1] != "/reset" {
		t.Fatalf("default triggers = %v, want [/new /reset]", s.Triggers)
	}
	if !s.Daily || s.DailyHour != 4 || s.DailyMin != 30 {
		t.Fatalf("daily = %+v, want 04:30 enabled", s)
	}
	if s.IdleMin != 45 {
		t.Fatalf("idle = %d, want 45", s.IdleMin)
	}
}

func TestSessionConfig_ExplicitTriggersRespected(t *testing.T) {
	c := validBase()
	c.Session = &SessionConfig{Triggers: []string{"/otra"}}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	s := c.SessionSettings()
	if len(s.Triggers) != 1 || s.Triggers[0] != "/otra" {
		t.Fatalf("triggers = %v, want [/otra]", s.Triggers)
	}
	if s.Daily || s.IdleMin != 0 {
		t.Fatalf("expiry should be off by default: %+v", s)
	}
}

func TestSessionConfig_InvalidShapesFailClearly(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"bad daily format", func(c *Config) { c.Session = &SessionConfig{DailyAt: "4am"} }},
		{"daily out of range", func(c *Config) { c.Session = &SessionConfig{DailyAt: "25:99"} }},
		{"negative idle", func(c *Config) { c.Session = &SessionConfig{IdleMin: -5} }},
		{"empty trigger", func(c *Config) { c.Session = &SessionConfig{Triggers: []string{""}} }},
		{"session without storage", func(c *Config) {
			c.Storage = nil
			c.Session = &SessionConfig{IdleMin: 10}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validBase()
			tc.mut(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("Validate accepted an invalid session block (%s)", tc.name)
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestSessionConfig_AbsentBlockMeansNone(t *testing.T) {
	c := validBase()
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	s := c.SessionSettings()
	if len(s.Triggers) != 0 || s.Daily || s.IdleMin != 0 {
		t.Fatalf("absent block must resolve to none, got %+v", s)
	}
}
