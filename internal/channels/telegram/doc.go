// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package telegram is the Korvun channel adapter for Telegram Bot updates.
//
// It exposes two pure conversion functions that bridge the canonical
// Envelope type and the native types of github.com/go-telegram/bot:
//
//   - InboundFromUpdate  (*models.Update)  -> *envelope.Envelope
//   - OutboundToSendMessage (*envelope.Envelope) -> *bot.SendMessageParams
//
// The adapter owns no transport state; it can be fed updates from a real
// long-poll loop, a webhook HTTP handler, or an in-memory test fixture.
package telegram

// ChannelName is the Envelope.Channel value used for messages originating
// from or destined for Telegram.
const ChannelName = "telegram"

// Meta keys used to carry Telegram-specific information that does not fit
// the channel-agnostic Envelope contract. Keys are namespaced under
// "telegram." so multiple adapters can coexist in the same Envelope.
const (
	// MetaChatID is the int64 chat ID, stringified.
	MetaChatID = "telegram.chat_id"
	// MetaChatType is the Telegram chat type (private, group, supergroup,
	// channel, ...).
	MetaChatType = "telegram.chat_type"
	// MetaMessageID is the Telegram message ID, stringified.
	MetaMessageID = "telegram.message_id"
	// MetaAudioKind disambiguates Telegram's two audio sub-types when an
	// outbound Envelope carries an envelope.Audio Part. Inbound voice
	// notes and audio files both collapse to envelope.Audio (no native
	// distinction in the canonical domain); on outbound the caller can
	// set this Meta key to AudioKindVoice to send the file as a voice
	// note (SendVoice) instead of the default music/audio file
	// (SendAudio).
	MetaAudioKind = "telegram.audio_kind"
	// MetaCallbackQueryID is the Telegram CallbackQuery.ID of an
	// inbound callback_query update, preserved verbatim. The same key
	// on an outbound OpCallbackAck Envelope identifies which callback
	// the ack addresses. (Phase 2E.4, routed via Operation in Phase
	// 2E.6.)
	MetaCallbackQueryID = "telegram.callback_query_id"
	// MetaCommand is the name of the bot command parsed from a Text
	// Message with a bot_command MessageEntity at offset 0. The leading
	// "/" and any "@botname" suffix are stripped. Absent if the
	// message is not a command. (Phase 2E.5.)
	MetaCommand = "telegram.command"
	// MetaCommandArgs is the trimmed argument string that follows the
	// command name in the Text. Absent when the command carries no
	// arguments. (Phase 2E.5.)
	MetaCommandArgs = "telegram.command_args"
)

// Values accepted by MetaAudioKind. Any other value (including an
// absent key) falls back to AudioKindAudio semantics.
const (
	// AudioKindVoice routes envelope.Audio outbound through
	// bot.SendVoice / SendVoiceParams.
	AudioKindVoice = "voice"
	// AudioKindAudio routes envelope.Audio outbound through
	// bot.SendAudio / SendAudioParams. This is the default.
	AudioKindAudio = "audio"
)
