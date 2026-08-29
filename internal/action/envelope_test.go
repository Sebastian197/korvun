// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// ActionEnvelope v1 contract (spec FR-DOM-2): the sealed §10.5 subset,
// schema pinned at 1, digest computed at construction, effect held at the
// Etapa-1 placeholder, timestamps normalized to UTC.

package action

import (
	"strings"
	"testing"
	"time"
)

func TestNewEnvelope_fillsTheSealedSubset(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 30, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	src := Source{Kind: "agent_brain", Protocol: "text", Channel: "console"}
	operation := Operation{Namespace: "tool", Name: "echo", Version: 1}

	env := NewEnvelope("act_test01", "env-123", src, operation, `{"a":1}`, at)

	if env.SchemaVersion != 1 {
		t.Fatalf("schema_version must be pinned at 1, got %d", env.SchemaVersion)
	}
	if env.ActionID != "act_test01" || env.CorrelationID != "env-123" {
		t.Fatalf("ids must be copied verbatim, got %q %q", env.ActionID, env.CorrelationID)
	}
	if env.Source != src || env.Operation != operation {
		t.Fatalf("source and operation must be copied verbatim")
	}
	if want := Digest(operation, `{"a":1}`); env.ParametersDigest != want {
		t.Fatalf("the digest is computed at construction: got %q want %q", env.ParametersDigest, want)
	}
	if env.Effect.Class != "unclassified" {
		t.Fatalf("Etapa 1 pins the effect placeholder, got %q", env.Effect.Class)
	}
	if env.RequestedAt.Location() != time.UTC || !env.RequestedAt.Equal(at) {
		t.Fatalf("requested_at must be the same instant normalized to UTC, got %v", env.RequestedAt)
	}
}

func TestNewID_shapeAndUniqueness(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if !strings.HasPrefix(id, "act_") {
			t.Fatalf("action ids carry the act_ prefix, got %q", id)
		}
		hexPart := strings.TrimPrefix(id, "act_")
		if len(hexPart) != 32 {
			t.Fatalf("action ids carry 16 random bytes in hex (32 chars), got %d in %q", len(hexPart), id)
		}
		if strings.Trim(hexPart, "0123456789abcdef") != "" {
			t.Fatalf("action id randomness must be lowercase hex, got %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = true
	}
}
