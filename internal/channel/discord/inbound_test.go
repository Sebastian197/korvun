// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package discord tests — Piece 4, sub-phase 2 (pure inbound mapping). These pin
// mapMessageCreate: a pure function (no network, no state, no goroutines) that
// decodes a MESSAGE_CREATE dispatch payload and maps it to an inbound Envelope per
// ADR-0033 §4, or reports the reason it must be dropped so SP3 can count it in
// DroppedCount without guessing.
package discord

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// TestDropReason_String pins the stable log label for each drop reason (SP3 logs it
// to distinguish why a message was dropped).
func TestDropReason_String(t *testing.T) {
	for r, want := range map[dropReason]string{
		keep:             "keep",
		dropMalformed:    "malformed",
		dropNoChannelID:  "no_channel_id",
		dropNoAuthor:     "no_author",
		dropSelf:         "self",
		dropFromBot:      "bot",
		dropEmptyContent: "empty_content",
		dropReason(99):   "unknown",
	} {
		if got := r.String(); got != want {
			t.Errorf("dropReason(%d).String() = %q, want %q", int(r), got, want)
		}
	}
}

func TestMapMessageCreate(t *testing.T) {
	// The bot's own user id (SP3 reads it from the Ready event; here it is a fixed
	// parameter). Any message from this author, or from any other bot, is dropped —
	// loop prevention as a contract (ADR-0033 §4).
	const self = "111111111111111111"

	tests := []struct {
		name string
		json string
		want dropReason
		// For a kept message, the expected mapped fields:
		convID     string
		senderID   string
		senderName string
		text       string
	}{
		{
			name:       "guild message, happy path",
			json:       `{"id":"900","channel_id":"555","guild_id":"777","content":"hola korvun","author":{"id":"222","username":"alice","global_name":"Alice A.","bot":false}}`,
			want:       keep,
			convID:     "555",
			senderID:   "222",
			senderName: "Alice A.",
			text:       "hola korvun",
		},
		{
			name:       "DM (no guild_id) maps the same, keyed by channel_id",
			json:       `{"id":"901","channel_id":"666","content":"dm hi","author":{"id":"333","username":"bob","global_name":"Bob B."}}`,
			want:       keep,
			convID:     "666",
			senderID:   "333",
			senderName: "Bob B.",
			text:       "dm hi",
		},
		{
			name:       "no global_name falls back to username",
			json:       `{"id":"902","channel_id":"555","guild_id":"777","content":"hey","author":{"id":"444","username":"carol"}}`,
			want:       keep,
			convID:     "555",
			senderID:   "444",
			senderName: "carol",
			text:       "hey",
		},
		{
			name:       "emoji / multibyte text survives intact",
			json:       `{"id":"903","channel_id":"555","content":"hola 🐦‍⬛ café 日本語","author":{"id":"222","username":"alice","global_name":"Alice"}}`,
			want:       keep,
			convID:     "555",
			senderID:   "222",
			senderName: "Alice",
			text:       "hola 🐦‍⬛ café 日本語",
		},
		{
			name:       "unknown extra fields are tolerated",
			json:       `{"id":"904","channel_id":"555","content":"hi","author":{"id":"222","username":"alice","global_name":"Alice","public_flags":64},"embeds":[],"tts":false,"mentions":[]}`,
			want:       keep,
			convID:     "555",
			senderID:   "222",
			senderName: "Alice",
			text:       "hi",
		},
		{
			name: "message from the bot itself is dropped (loop prevention)",
			json: `{"id":"905","channel_id":"555","content":"my own reply","author":{"id":"111111111111111111","username":"korvun","bot":true}}`,
			want: dropSelf,
		},
		{
			name: "message from ANOTHER bot is dropped (loop prevention)",
			json: `{"id":"906","channel_id":"555","content":"other bot says hi","author":{"id":"888","username":"otherbot","bot":true}}`,
			want: dropFromBot,
		},
		{
			name: "empty content (media-only, out of v1) is dropped",
			json: `{"id":"907","channel_id":"555","content":"","author":{"id":"222","username":"alice"}}`,
			want: dropEmptyContent,
		},
		{
			name: "whitespace-only content is dropped (not routed as blank input)",
			json: `{"id":"907b","channel_id":"555","content":"  \n\t ","author":{"id":"222","username":"alice"}}`,
			want: dropEmptyContent,
		},
		{
			name: "missing author is dropped",
			json: `{"id":"908","channel_id":"555","content":"orphan"}`,
			want: dropNoAuthor,
		},
		{
			name: "author present but with no id is dropped (no empty Sender.ID)",
			json: `{"id":"908b","channel_id":"555","content":"who am i","author":{"username":"ghost"}}`,
			want: dropNoAuthor,
		},
		{
			name: "missing channel_id is dropped",
			json: `{"id":"909","content":"no channel","author":{"id":"222","username":"alice"}}`,
			want: dropNoChannelID,
		},
		{
			name: "malformed JSON is dropped",
			json: `{"id":"910","channel_id":"555",`,
			want: dropMalformed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, drop := mapMessageCreate([]byte(tt.json), self)

			if drop != tt.want {
				t.Fatalf("drop = %v, want %v", drop, tt.want)
			}
			if tt.want != keep {
				if env != nil {
					t.Errorf("a dropped message must return a nil Envelope, got %+v", env)
				}
				return
			}

			if env == nil {
				t.Fatal("a kept message must return a non-nil Envelope")
			}
			if env.Channel != "discord" {
				t.Errorf("Channel = %q, want %q", env.Channel, "discord")
			}
			if env.Direction != envelope.Inbound {
				t.Errorf("Direction = %v, want Inbound", env.Direction)
			}
			if got := env.Meta[conversation.MetaConversationID]; got != tt.convID {
				t.Errorf("conversation.id = %q, want %q (the Discord channel_id)", got, tt.convID)
			}
			if env.Sender.ID != tt.senderID {
				t.Errorf("Sender.ID = %q, want %q", env.Sender.ID, tt.senderID)
			}
			if env.Sender.Name != tt.senderName {
				t.Errorf("Sender.Name = %q, want %q", env.Sender.Name, tt.senderName)
			}
			if len(env.Parts) != 1 || env.Parts[0].Type != envelope.Text || env.Parts[0].Content != tt.text {
				t.Errorf("Parts = %+v, want a single Text part %q", env.Parts, tt.text)
			}
		})
	}
}
