// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// ----------------------------------------------------------------------------
// Phase 2E.7 — outbound OpSetReaction.
//
// An OpSetReaction Envelope translates to *bot.SetMessageReactionParams via
// the existing outboundOperation dispatch. Each Text Part's Content is
// mapped to a models.ReactionType emoji variant; empty Parts maps to an
// empty slice, which is the standard Bot API idiom for "clear all the bot's
// reactions on the target message".
//
// Target preconditions reuse the existing helpers:
//   - parseChatID:           telegram.chat_id    (int64)
//   - parseTargetMessageID:  telegram.message_id (int)
// Missing either yields the corresponding sentinel error.
// ----------------------------------------------------------------------------

func TestOutboundParams_SetReaction_SingleEmoji(t *testing.T) {
	e := mkOpEnv()
	e.SetReactions("👍")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindSetReaction {
		t.Errorf("Kind = %v, want OutboundKindSetReaction", out.Kind)
	}
	if out.SetReaction == nil {
		t.Fatal("SetReaction is nil")
	}
	assertChatID(t, out.SetReaction.ChatID, 1000)
	if out.SetReaction.MessageID != 42 {
		t.Errorf("MessageID = %d, want 42", out.SetReaction.MessageID)
	}
	if len(out.SetReaction.Reaction) != 1 {
		t.Fatalf("Reaction len = %d, want 1", len(out.SetReaction.Reaction))
	}
	r := out.SetReaction.Reaction[0]
	if r.Type != models.ReactionTypeTypeEmoji {
		t.Errorf("Reaction[0].Type = %q, want %q", r.Type, models.ReactionTypeTypeEmoji)
	}
	if r.ReactionTypeEmoji == nil || r.ReactionTypeEmoji.Emoji != "👍" {
		t.Errorf("Reaction[0].ReactionTypeEmoji = %+v, want emoji=👍", r.ReactionTypeEmoji)
	}
	// Sibling tagged-union fields must remain nil.
	if out.Message != nil || out.Photo != nil || out.Document != nil ||
		out.Voice != nil || out.Audio != nil || out.Video != nil ||
		out.Location != nil || out.AnswerCallback != nil ||
		out.EditText != nil || out.EditCaption != nil || out.Delete != nil {
		t.Error("only SetReaction should be populated")
	}
}

func TestOutboundParams_SetReaction_MultipleEmojis(t *testing.T) {
	e := mkOpEnv()
	e.SetReactions("👍", "🎉", "❤️")
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if len(out.SetReaction.Reaction) != 3 {
		t.Fatalf("Reaction len = %d, want 3", len(out.SetReaction.Reaction))
	}
	for i, want := range []string{"👍", "🎉", "❤️"} {
		got := out.SetReaction.Reaction[i]
		if got.ReactionTypeEmoji == nil || got.ReactionTypeEmoji.Emoji != want {
			t.Errorf("Reaction[%d] emoji = %+v, want %q", i, got.ReactionTypeEmoji, want)
		}
	}
}

func TestOutboundParams_SetReaction_ClearAll(t *testing.T) {
	// Empty variadic at the builder ⇒ empty Parts ⇒ empty []ReactionType
	// on the params. Telegram's SetMessageReaction interprets the empty
	// slice as "remove all of the bot's reactions on the target".
	e := mkOpEnv()
	e.SetReactions()
	out, err := OutboundParams(e)
	if err != nil {
		t.Fatalf("OutboundParams: %v", err)
	}
	if out.Kind != OutboundKindSetReaction {
		t.Errorf("Kind = %v, want OutboundKindSetReaction", out.Kind)
	}
	if out.SetReaction == nil {
		t.Fatal("SetReaction is nil")
	}
	if len(out.SetReaction.Reaction) != 0 {
		t.Errorf("Reaction len = %d, want 0 (clear-all)", len(out.SetReaction.Reaction))
	}
}

func TestOutboundParams_SetReaction_MissingMessageID(t *testing.T) {
	e := mkOpEnv()
	delete(e.Meta, MetaMessageID)
	e.SetReactions("👍")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingTargetMessageID) {
		t.Errorf("err = %v, want ErrMissingTargetMessageID", err)
	}
}

func TestOutboundParams_SetReaction_MissingChatID(t *testing.T) {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaMessageID] = "42"
	e.SetReactions("👍")
	_, err := OutboundParams(e)
	if !errors.Is(err, ErrMissingChatID) {
		t.Errorf("err = %v, want ErrMissingChatID", err)
	}
}

func TestOutboundKind_SetReactionString(t *testing.T) {
	if got := OutboundKindSetReaction.String(); got != "set_reaction" {
		t.Errorf("OutboundKindSetReaction.String() = %q, want %q", got, "set_reaction")
	}
}
