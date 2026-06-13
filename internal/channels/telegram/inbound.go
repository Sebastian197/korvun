// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"strconv"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot/models"
)

// InboundFromUpdate converts a Telegram Update into a canonical Envelope.
//
// The Update must carry a non-nil Message with a non-empty Text and a
// non-nil From; other shapes are rejected with ErrNoMessage or
// ErrUnsupportedContent so future phases can extend coverage without
// breaking this contract.
func InboundFromUpdate(u *models.Update) (*envelope.Envelope, error) {
	if u == nil || u.Message == nil {
		return nil, ErrNoMessage
	}
	m := u.Message
	if m.Text == "" || m.From == nil {
		return nil, ErrUnsupportedContent
	}
	sender := envelope.Participant{
		ID:   strconv.FormatInt(m.From.ID, 10),
		Name: senderName(m.From),
	}
	env := envelope.New(ChannelName, envelope.Inbound, sender)
	env.AddText(m.Text)
	env.Timestamp = time.Unix(int64(m.Date), 0).UTC()
	env.Meta[MetaChatID] = strconv.FormatInt(m.Chat.ID, 10)
	env.Meta[MetaMessageID] = strconv.Itoa(m.ID)
	if t := string(m.Chat.Type); t != "" {
		env.Meta[MetaChatType] = t
	}
	return env, nil
}

// senderName picks a display name for a Telegram user, preferring the
// public @username when present so the Envelope sender label survives
// first-name changes; otherwise falls back to first + last name.
func senderName(u *models.User) string {
	if u.Username != "" {
		return u.Username
	}
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}
