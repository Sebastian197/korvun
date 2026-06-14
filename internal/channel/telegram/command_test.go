// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

// ----------------------------------------------------------------------------
// Phase 2E.5 — inbound bot_command parsing.
//
// Commands ride as ordinary Text messages with a MessageEntity of type
// "bot_command" at offset 0. The adapter parses these into two Meta keys
// (telegram.command, telegram.command_args) so downstream consumers can
// dispatch on the command without re-implementing the entity-aware parse.
// The Text Part itself stays intact and carries the original /cmd payload
// verbatim, so existing text consumers are unaffected.
//
// Modelling stays adapter-local (channel-prefixed Meta) — see the Phase
// 2E.5 notes. Promoting to a canonical envelope concept is deferred until
// at least a second channel needs commands.
// ----------------------------------------------------------------------------

func TestInboundFromUpdate_Command_StartNoArgs(t *testing.T) {
	u := loadUpdateFixture(t, "command_start.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaCommand] != "start" {
		t.Errorf("Meta[%s] = %q, want %q", MetaCommand, env.Meta[MetaCommand], "start")
	}
	if _, ok := env.Meta[MetaCommandArgs]; ok {
		t.Errorf("Meta[%s] = %q, want absent for /start with no args",
			MetaCommandArgs, env.Meta[MetaCommandArgs])
	}
	// The Text Part must remain the original "/start" payload so existing
	// text consumers are unaffected.
	if len(env.Parts) != 1 || env.Parts[0].Content != "/start" {
		t.Errorf("Parts = %+v, want one Text part with /start", env.Parts)
	}
}

func TestInboundFromUpdate_Command_WithArgs(t *testing.T) {
	u := loadUpdateFixture(t, "command_with_args.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaCommand] != "help" {
		t.Errorf("Meta[%s] = %q, want %q", MetaCommand, env.Meta[MetaCommand], "help")
	}
	if env.Meta[MetaCommandArgs] != "foo bar" {
		t.Errorf("Meta[%s] = %q, want %q", MetaCommandArgs, env.Meta[MetaCommandArgs], "foo bar")
	}
	if env.Parts[0].Content != "/help foo bar" {
		t.Errorf("Parts[0].Content = %q, want %q", env.Parts[0].Content, "/help foo bar")
	}
}

func TestInboundFromUpdate_Command_StripsBotnameSuffix(t *testing.T) {
	// In group chats Telegram adds @botname so the command targets a
	// specific bot; the entity Length covers /cmd + @botname together.
	// The parser must strip the suffix from the stored command name
	// but keep args (after the entity boundary) intact.
	u := loadUpdateFixture(t, "command_with_botname.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaCommand] != "cmd" {
		t.Errorf("Meta[%s] = %q, want %q (botname stripped)",
			MetaCommand, env.Meta[MetaCommand], "cmd")
	}
	if env.Meta[MetaCommandArgs] != "args here" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaCommandArgs, env.Meta[MetaCommandArgs], "args here")
	}
	if env.Parts[0].Content != "/cmd@korvun_bot args here" {
		t.Errorf("Parts[0].Content = %q, want original payload preserved",
			env.Parts[0].Content)
	}
}

func TestInboundFromUpdate_Command_BotnameOnly_NoArgs(t *testing.T) {
	// "/cmd@botname" with no trailing args must produce command without
	// a command_args key (rather than an empty string value).
	u := &models.Update{
		Message: &models.Message{
			ID:   601,
			Date: 1786000600,
			From: &models.User{ID: 555, Username: "alice"},
			Chat: models.Chat{ID: 2000, Type: "supergroup"},
			Text: "/cmd@korvun_bot",
			Entities: []models.MessageEntity{
				{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 15},
			},
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaCommand] != "cmd" {
		t.Errorf("Meta[%s] = %q, want %q", MetaCommand, env.Meta[MetaCommand], "cmd")
	}
	if _, ok := env.Meta[MetaCommandArgs]; ok {
		t.Errorf("Meta[%s] = %q, want absent", MetaCommandArgs, env.Meta[MetaCommandArgs])
	}
}

func TestInboundFromUpdate_Command_RegularText_NoCommandMeta(t *testing.T) {
	// A plain text message without a bot_command entity must not get
	// command Meta keys.
	u := loadUpdateFixture(t, "text_message.json")
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if _, ok := env.Meta[MetaCommand]; ok {
		t.Errorf("Meta[%s] set on plain text envelope", MetaCommand)
	}
	if _, ok := env.Meta[MetaCommandArgs]; ok {
		t.Errorf("Meta[%s] set on plain text envelope", MetaCommandArgs)
	}
}

func TestInboundFromUpdate_Command_OffsetNonZero_NotTreatedAsCommand(t *testing.T) {
	// Telegram only treats an entity at Offset == 0 as the command for
	// the message. A bot_command entity later in the text (e.g. quoted
	// or referenced mid-sentence) must NOT be promoted to a command.
	u := &models.Update{
		Message: &models.Message{
			ID:   602,
			Date: 1786000700,
			From: &models.User{ID: 555, Username: "alice"},
			Chat: models.Chat{ID: 1000, Type: "private"},
			Text: "look at /start",
			Entities: []models.MessageEntity{
				{Type: models.MessageEntityTypeBotCommand, Offset: 8, Length: 6},
			},
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if _, ok := env.Meta[MetaCommand]; ok {
		t.Errorf("Meta[%s] = %q set when bot_command entity Offset != 0",
			MetaCommand, env.Meta[MetaCommand])
	}
}

func TestInboundFromUpdate_Command_TextWithMultipleEntities_PicksFirstAtOffsetZero(t *testing.T) {
	// A message can carry several entities (e.g. /start followed by a
	// URL). The parser must pick the bot_command at offset 0 and
	// ignore the others.
	u := &models.Update{
		Message: &models.Message{
			ID:   603,
			Date: 1786000800,
			From: &models.User{ID: 555, Username: "alice"},
			Chat: models.Chat{ID: 1000, Type: "private"},
			Text: "/start https://example.com",
			Entities: []models.MessageEntity{
				{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 6},
				{Type: models.MessageEntityTypeURL, Offset: 7, Length: 19},
			},
		},
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		t.Fatalf("InboundFromUpdate: %v", err)
	}
	if env.Meta[MetaCommand] != "start" {
		t.Errorf("Meta[%s] = %q, want %q", MetaCommand, env.Meta[MetaCommand], "start")
	}
	if env.Meta[MetaCommandArgs] != "https://example.com" {
		t.Errorf("Meta[%s] = %q, want %q",
			MetaCommandArgs, env.Meta[MetaCommandArgs], "https://example.com")
	}
}

func TestInboundFromUpdate_Command_PassesValidate(t *testing.T) {
	fixtures := []string{
		"command_start.json",
		"command_with_args.json",
		"command_with_botname.json",
	}
	for _, name := range fixtures {
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
