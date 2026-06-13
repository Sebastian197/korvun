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

// InboundFromUpdate converts a Telegram Update into a canonical
// Envelope. Supported content (Phase 2E.1):
//
//   - Text messages (Phase 2.3).
//   - Photo, voice, audio, video, document — each mapped to an
//     Envelope Part with file_id as Source and mime_type (when
//     present) as MIMEType.
//   - Caption (the text Telegram attaches to a media message) is
//     appended as a separate text Part after the media Part.
//
// The Update must carry a non-nil Message with a non-nil From; if
// the resulting Envelope would carry no Parts (no text, no caption,
// no supported media) the call returns ErrUnsupportedContent.
func InboundFromUpdate(u *models.Update) (*envelope.Envelope, error) {
	if u == nil || u.Message == nil {
		return nil, ErrNoMessage
	}
	m := u.Message
	if m.From == nil {
		return nil, ErrUnsupportedContent
	}
	sender := envelope.Participant{
		ID:   strconv.FormatInt(m.From.ID, 10),
		Name: senderName(m.From),
	}
	env := envelope.New(ChannelName, envelope.Inbound, sender)
	env.Timestamp = time.Unix(int64(m.Date), 0).UTC()
	env.Meta[MetaChatID] = strconv.FormatInt(m.Chat.ID, 10)
	env.Meta[MetaMessageID] = strconv.Itoa(m.ID)
	if t := string(m.Chat.Type); t != "" {
		env.Meta[MetaChatType] = t
	}

	appendMediaPart(env, m)
	appendTextPart(env, m)

	if len(env.Parts) == 0 {
		return nil, ErrUnsupportedContent
	}
	return env, nil
}

// senderName picks a display name for a Telegram user, preferring the
// public @username so the Envelope sender label survives first-name
// changes; otherwise falls back to first + last name.
func senderName(u *models.User) string {
	if u.Username != "" {
		return u.Username
	}
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// appendMediaPart inspects the message and appends at most one media
// Part to env. Telegram messages carry at most one of photo / voice /
// audio / video / document in practice; the iteration order is
// deterministic but defensive — if a message ever happens to carry
// more than one, the first match wins.
func appendMediaPart(env *envelope.Envelope, m *models.Message) {
	switch {
	case len(m.Photo) > 0:
		p := largestPhotoSize(m.Photo)
		// PhotoSize has no mime_type field, so MIMEType stays empty.
		env.AddMedia(envelope.Image, p.FileID, "")
	case m.Voice != nil:
		env.AddMedia(envelope.Audio, m.Voice.FileID, m.Voice.MimeType)
	case m.Audio != nil:
		env.AddMedia(envelope.Audio, m.Audio.FileID, m.Audio.MimeType)
	case m.Video != nil:
		env.AddMedia(envelope.Video, m.Video.FileID, m.Video.MimeType)
	case m.Document != nil:
		env.AddMedia(envelope.File, m.Document.FileID, m.Document.MimeType)
	}
}

// appendTextPart adds the message text as a text Part. When the
// message carries media, Telegram puts the accompanying text in
// Caption; otherwise it lives in Text. Both never appear together in
// a regular Telegram message, so a simple priority (caption > text)
// covers every case without ambiguity. The text Part is appended
// AFTER the media Part so the Envelope reads "media then its
// description", matching how Telegram's UI presents it.
func appendTextPart(env *envelope.Envelope, m *models.Message) {
	text := m.Caption
	if text == "" {
		text = m.Text
	}
	if text != "" {
		env.AddText(text)
	}
}

// largestPhotoSize returns the PhotoSize entry with the maximum
// FileSize, i.e. the highest fidelity Telegram delivers without a
// re-download. Slice is guaranteed non-empty by the caller.
func largestPhotoSize(sizes []models.PhotoSize) models.PhotoSize {
	largest := sizes[0]
	for _, p := range sizes[1:] {
		if p.FileSize > largest.FileSize {
			largest = p
		}
	}
	return largest
}
