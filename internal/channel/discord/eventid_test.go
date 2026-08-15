// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// Audit finding R-1: MESSAGE_CREATE envelopes must carry the Discord message
// id as the provider event id, so a Gateway resume replay deduplicates in
// the router. A payload without an id yields NO event id (fail-open).

func TestMapMessageCreate_StampsProviderEventID(t *testing.T) {
	payload := `{"id":"111222333444555666","channel_id":"c1",
		"content":"hola","author":{"id":"u1","username":"ana"}}`
	env, drop := mapMessageCreate([]byte(payload), "self-id")
	if env == nil {
		t.Fatalf("dropped: %v", drop)
	}
	if got := env.Meta[envelope.MetaProviderEventID]; got != "111222333444555666" {
		t.Errorf("Meta[%q] = %q, want the message id", envelope.MetaProviderEventID, got)
	}
}

func TestMapMessageCreate_NoIDMeansNoEventID(t *testing.T) {
	payload := `{"channel_id":"c1","content":"hola",
		"author":{"id":"u1","username":"ana"}}`
	env, drop := mapMessageCreate([]byte(payload), "self-id")
	if env == nil {
		t.Fatalf("dropped: %v", drop)
	}
	if got, ok := env.Meta[envelope.MetaProviderEventID]; ok {
		t.Errorf("Meta[%q] = %q, want absent (fail-open: no id, no dedup)",
			envelope.MetaProviderEventID, got)
	}
}
