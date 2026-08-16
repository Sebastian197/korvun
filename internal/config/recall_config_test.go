// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-A red suite (minimal-memory spec 2026-08-16, FR-CFG-A1 + AS-A9): the
// session.recall_max range and resolution, and the reserved command tokens
// that session.triggers must reject (triggers run first in the dispatch and
// would silently shadow /recall and /notes).
package config

import (
	"errors"
	"strings"
	"testing"
)

func TestRecallMax_ResolvesIntoSettings(t *testing.T) {
	c := validBase()
	c.Session = &SessionConfig{RecallMax: 10}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := c.SessionSettings().RecallMax; got != 10 {
		t.Fatalf("SessionSettings().RecallMax = %d, want 10", got)
	}
}

func TestRecallMax_ZeroIsDisabledDefault(t *testing.T) {
	c := validBase()
	c.Session = &SessionConfig{IdleMin: 5}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := c.SessionSettings().RecallMax; got != 0 {
		t.Fatalf("SessionSettings().RecallMax = %d, want 0 (disabled by default)", got)
	}
}

func TestRecallMax_RangeValidatedFailLoud(t *testing.T) {
	valid := []int{1, 50}
	for _, v := range valid {
		c := validBase()
		c.Session = &SessionConfig{RecallMax: v}
		if err := c.Validate(); err != nil {
			t.Fatalf("Validate rejected valid recall_max %d: %v", v, err)
		}
	}
	invalid := []int{51, -1}
	for _, v := range invalid {
		c := validBase()
		c.Session = &SessionConfig{RecallMax: v}
		err := c.Validate()
		if err == nil {
			t.Fatalf("Validate accepted recall_max %d, want fail loud (valid range 0..50)", v)
		}
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("recall_max %d error = %v, want ErrInvalidConfig", v, err)
		}
		if !strings.Contains(err.Error(), "session.recall_max") {
			t.Fatalf("recall_max %d error %q does not name the field path session.recall_max", v, err)
		}
	}
}

func TestReservedTriggerTokensRejected(t *testing.T) {
	cases := []struct {
		name     string
		triggers []string
		wantPath string
	}{
		{"recall at index 0", []string{"/recall"}, "session.triggers[0]"},
		{"notes at index 1", []string{"/new", "/notes"}, "session.triggers[1]"},
		{"recall behind valid triggers", []string{"/new", "/reset", "/recall"}, "session.triggers[2]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validBase()
			c.Session = &SessionConfig{Triggers: tc.triggers}
			err := c.Validate()
			if err == nil {
				t.Fatalf("Validate accepted the reserved token in %v (AS-A9)", tc.triggers)
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Fatalf("error %q does not name %s", err, tc.wantPath)
			}
		})
	}
}
