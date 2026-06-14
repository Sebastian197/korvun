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
// Phase 2E.7 — inbound MessageReaction.
//
// Telegram delivers Update.MessageReaction when a specific user changes
// their reactions on a message. The adapter:
//
//   - Filters non-emoji ReactionType variants (custom_emoji, paid) BEFORE
//     computing the diff. After filtering, the comparison uses sorted
//     emoji slices as sets, so a user-perceived no-op is dropped.
//   - Derives a single action ∈ {added, removed, changed} from the
//     diff and sets Meta[telegram.reaction_action] accordingly.
//   - Populates Parts with the emojis that the action references
//     (added → New, removed → Old, changed → New). For 'changed',
//     Meta[telegram.reaction_previous] carries the previous emoji set
//     as a comma-separated string.
//   - Drops the no-op case (Old == New after filter) with
//     ErrUnsupportedContent.
//   - Drops the nil-User case (anonymous admin via ActorChat only) with
//     ErrUnsupportedContent — that path is out of scope for ADR-0007.
// ----------------------------------------------------------------------------

// ---------- Happy paths (from fixtures) -----------------------------------

func TestInboundFromUpdate_Reaction_Added(t *testing.T) {
	u := loadUpdateFixture(t, "reaction_added.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Direction != envelope.Inbound {
		t.Errorf("Direction = %v, want Inbound", env.Direction)
	}
	if env.Sender.ID != "555" {
		t.Errorf("Sender.ID = %q, want 555", env.Sender.ID)
	}
	if env.Meta[MetaReactionAction] != "added" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionAction, env.Meta[MetaReactionAction], "added")
	}
	if _, has := env.Meta[MetaReactionPrevious]; has {
		t.Errorf("Meta[%s] must be absent for 'added' action", MetaReactionPrevious)
	}
	if env.Meta[MetaChatID] != "1000" || env.Meta[MetaMessageID] != "801" {
		t.Errorf("target IDs = (%q, %q), want (1000, 801)",
			env.Meta[MetaChatID], env.Meta[MetaMessageID])
	}
	if len(env.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(env.Parts))
	}
	if env.Parts[0].Type != envelope.Reaction {
		t.Errorf("Parts[0].Type = %v, want Reaction", env.Parts[0].Type)
	}
	if env.Parts[0].Content != "👍" {
		t.Errorf("Parts[0].Content = %q, want 👍", env.Parts[0].Content)
	}
}

func TestInboundFromUpdate_Reaction_Removed(t *testing.T) {
	u := loadUpdateFixture(t, "reaction_removed.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaReactionAction] != "removed" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionAction, env.Meta[MetaReactionAction], "removed")
	}
	if _, has := env.Meta[MetaReactionPrevious]; has {
		t.Errorf("Meta[%s] must be absent for 'removed' action", MetaReactionPrevious)
	}
	// For 'removed', Parts contains the emojis that were removed.
	if len(env.Parts) != 1 || env.Parts[0].Content != "👍" {
		t.Errorf("Parts = %+v, want one Reaction with 👍 (removed emoji)", env.Parts)
	}
}

func TestInboundFromUpdate_Reaction_Changed(t *testing.T) {
	u := loadUpdateFixture(t, "reaction_changed.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaReactionAction] != "changed" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionAction, env.Meta[MetaReactionAction], "changed")
	}
	if env.Meta[MetaReactionPrevious] != "👍" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionPrevious, env.Meta[MetaReactionPrevious], "👍")
	}
	// For 'changed', Parts contains the NEW emojis.
	if len(env.Parts) != 1 || env.Parts[0].Content != "❤️" {
		t.Errorf("Parts = %+v, want one Reaction with ❤️ (new emoji)", env.Parts)
	}
}

func TestInboundFromUpdate_Reaction_PassesValidate(t *testing.T) {
	for _, name := range []string{
		"reaction_added.json",
		"reaction_removed.json",
		"reaction_changed.json",
	} {
		t.Run(name, func(t *testing.T) {
			u := loadUpdateFixture(t, name)
			env, err := InboundFromUpdate(u)
			if err != nil {
				t.Fatalf("InboundFromUpdate: %v", err)
			}
			if err := env.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

// ---------- Edge cases (inline fixtures) -----------------------------------

func emojiReaction(em string) models.ReactionType {
	return models.ReactionType{
		Type: models.ReactionTypeTypeEmoji,
		ReactionTypeEmoji: &models.ReactionTypeEmoji{
			Type:  models.ReactionTypeTypeEmoji,
			Emoji: em,
		},
	}
}

func customEmojiReaction(id string) models.ReactionType {
	return models.ReactionType{
		Type: models.ReactionTypeTypeCustomEmoji,
		ReactionTypeCustomEmoji: &models.ReactionTypeCustomEmoji{
			Type:          models.ReactionTypeTypeCustomEmoji,
			CustomEmojiID: id,
		},
	}
}

func paidReaction() models.ReactionType {
	return models.ReactionType{
		Type:             models.ReactionTypeTypePaid,
		ReactionTypePaid: &models.ReactionTypePaid{Type: string(models.ReactionTypeTypePaid)},
	}
}

func reactionUpdate(old, new []models.ReactionType) *models.Update {
	return &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1000, Type: "private"},
			MessageID:   42,
			User:        &models.User{ID: 555, Username: "alice"},
			Date:        1786000999,
			OldReaction: old,
			NewReaction: new,
		},
	}
}

func TestInboundFromUpdate_Reaction_NoOpDropped(t *testing.T) {
	// Old == New after the emoji filter: nothing to deliver upstream.
	u := reactionUpdate(
		[]models.ReactionType{emojiReaction("👍")},
		[]models.ReactionType{emojiReaction("👍")},
	)
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent (no-op)", err)
	}
}

func TestInboundFromUpdate_Reaction_SameSetDifferentOrder_NoOp(t *testing.T) {
	// The diff treats the slices as sets — a user-perceived no-op
	// (same multiset reordered) must not surface as a spurious
	// "changed" event.
	u := reactionUpdate(
		[]models.ReactionType{emojiReaction("👍"), emojiReaction("❤️")},
		[]models.ReactionType{emojiReaction("❤️"), emojiReaction("👍")},
	)
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent (no-op, reordered)", err)
	}
}

func TestInboundFromUpdate_Reaction_NilUser_Dropped(t *testing.T) {
	// Anonymous admin reactions (ActorChat-only) are out of scope for
	// ADR-0007; the adapter refuses such updates rather than
	// surfacing an envelope without a sender.
	u := &models.Update{
		MessageReaction: &models.MessageReactionUpdated{
			Chat:        models.Chat{ID: 1000, Type: "supergroup"},
			MessageID:   42,
			User:        nil,
			ActorChat:   &models.Chat{ID: 5000, Type: "channel"},
			Date:        1786001000,
			OldReaction: []models.ReactionType{},
			NewReaction: []models.ReactionType{emojiReaction("👍")},
		},
	}
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent (nil User)", err)
	}
}

func TestInboundFromUpdate_Reaction_CustomEmojiFilteredOut(t *testing.T) {
	// A user with only custom_emoji reactions on both sides is
	// invisible after the filter; the adapter drops the event because
	// the filtered slices are equal (both empty).
	u := reactionUpdate(
		[]models.ReactionType{customEmojiReaction("CUST_1")},
		[]models.ReactionType{customEmojiReaction("CUST_2")},
	)
	_, err := InboundFromUpdate(u)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want ErrUnsupportedContent (only custom_emoji)", err)
	}
}

func TestInboundFromUpdate_Reaction_MixedTypes_OnlyEmojiSurfaces(t *testing.T) {
	// Old=[custom], New=[emoji + custom + paid]. After filtering only
	// emoji types remain: Old=[], New=["👍"]. Surfaces as 'added' with
	// a single emoji.
	u := reactionUpdate(
		[]models.ReactionType{customEmojiReaction("CUST_OLD")},
		[]models.ReactionType{
			emojiReaction("👍"),
			customEmojiReaction("CUST_NEW"),
			paidReaction(),
		},
	)
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaReactionAction] != "added" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionAction, env.Meta[MetaReactionAction], "added")
	}
	if len(env.Parts) != 1 || env.Parts[0].Content != "👍" {
		t.Errorf("Parts = %+v, want one Reaction with 👍 (only emoji surfaces)",
			env.Parts)
	}
}

func TestInboundFromUpdate_Reaction_Changed_MultiPrevious_CSV(t *testing.T) {
	// Old = ["👍","❤️"], New = ["🎉"] → action="changed",
	// Parts=["🎉"], Meta[reaction_previous]="👍,❤️" (or "❤️,👍" — the
	// adapter preserves Telegram's delivery order in the CSV).
	u := reactionUpdate(
		[]models.ReactionType{emojiReaction("👍"), emojiReaction("❤️")},
		[]models.ReactionType{emojiReaction("🎉")},
	)
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaReactionAction] != "changed" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionAction, env.Meta[MetaReactionAction], "changed")
	}
	if env.Meta[MetaReactionPrevious] != "👍,❤️" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaReactionPrevious, env.Meta[MetaReactionPrevious], "👍,❤️")
	}
	if len(env.Parts) != 1 || env.Parts[0].Content != "🎉" {
		t.Errorf("Parts = %+v, want one Reaction with 🎉", env.Parts)
	}
}
