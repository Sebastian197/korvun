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
)
