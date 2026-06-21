// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package conversation_test

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

func inbound(channel, convID string) *envelope.Envelope {
	e := envelope.New(channel, envelope.Inbound, envelope.Participant{ID: "u1"})
	if convID != "" {
		e.Meta[conversation.MetaConversationID] = convID
	}
	return e
}

func TestKeyFromEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		env     *envelope.Envelope
		wantKey conversation.Key
		wantErr error
	}{
		{
			name:    "channel and conversation id compose the canonical key",
			env:     inbound("telegram", "42"),
			wantKey: conversation.Key("telegram::42"),
		},
		{
			name:    "missing conversation id is an error, empty key",
			env:     inbound("telegram", ""),
			wantKey: "",
			wantErr: conversation.ErrNoConversationID,
		},
		{
			name:    "nil envelope is an error, empty key",
			env:     nil,
			wantKey: "",
			wantErr: conversation.ErrNoConversationID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := conversation.KeyFromEnvelope(tt.env)
			if got != tt.wantKey {
				t.Errorf("key = %q, want %q", got, tt.wantKey)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestKeyFromEnvelope_MatchesLegacyComposition pins that the composition is
// exactly channel + "::" + conversation.id, so the router can delegate to it
// without changing behavior.
func TestKeyFromEnvelope_MatchesLegacyComposition(t *testing.T) {
	got, err := conversation.KeyFromEnvelope(inbound("webhook", "abc-123"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if want := conversation.Key("webhook::abc-123"); got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
}
