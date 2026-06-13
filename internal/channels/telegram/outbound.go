// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"fmt"
	"strconv"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot"
)

// OutboundToSendMessage converts an outbound Envelope into a
// bot.SendMessageParams ready to be passed to bot.SendMessage.
//
// Phase 2.3 only supports text messages: the first non-empty text part
// of the Envelope is used as the message body. The Envelope must carry
// telegram.chat_id in Meta as a stringified int64; missing, empty, or
// non-numeric values are rejected with a sentinel error.
func OutboundToSendMessage(e *envelope.Envelope) (*bot.SendMessageParams, error) {
	if e == nil {
		return nil, ErrNilEnvelope
	}
	if e.Direction != envelope.Outbound {
		return nil, ErrNotOutbound
	}
	if e.Channel != ChannelName {
		return nil, fmt.Errorf("%w: got %q", ErrWrongChannel, e.Channel)
	}
	chatIDStr, ok := e.Meta[MetaChatID]
	if !ok || chatIDStr == "" {
		return nil, ErrMissingChatID
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v", ErrInvalidChatID, chatIDStr, err)
	}
	text := firstTextPart(e.Parts)
	if text == "" {
		return nil, ErrNoTextPart
	}
	return &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}, nil
}

// firstTextPart returns the content of the first non-empty text part in
// parts, or "" if there is none. Media parts are skipped (Phase 2.3 only
// emits text messages).
func firstTextPart(parts []envelope.Part) string {
	for _, p := range parts {
		if p.Type == envelope.Text && p.Content != "" {
			return p.Content
		}
	}
	return ""
}
