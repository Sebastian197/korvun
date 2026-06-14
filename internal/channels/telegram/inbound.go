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
// Envelope. Supported update kinds:
//
//   - Message (Phase 2.3 / 2E.1 / 2E.3): text, photo, voice, audio,
//     video, document, location, plus optional caption.
//   - EditedMessage (Phase 2E.6): a user edit of one of their own
//     previous messages. Same *Message shape as a fresh Message; the
//     discriminator is EditDate (Unix timestamp, non-zero iff edited).
//     The resulting Envelope's Timestamp is EditDate (the moment of
//     the event), Meta[telegram.edited_at] carries the same value,
//     and Meta[telegram.message_id] is identical to the original
//     send so the orchestrator can correlate by ID.
//   - CallbackQuery (Phase 2E.4): a tap on an inline-keyboard button.
//     The resulting Envelope carries a single Callback Part whose
//     Content is the callback Data string, and Meta carries the
//     callback ID so a later outbound OpCallbackAck Envelope can
//     address it (ADR-0006).
//
// Returns ErrNoMessage when the update carries neither a Message, a
// CallbackQuery, nor an EditedMessage (or when the CallbackQuery
// lacks the ID needed to ack it), and ErrUnsupportedContent when the
// carried kind has no content the adapter can translate (no sender,
// no text/media, no callback data).
func InboundFromUpdate(u *models.Update) (*envelope.Envelope, error) {
	if u == nil {
		return nil, ErrNoMessage
	}
	if u.CallbackQuery != nil {
		return inboundFromCallbackQuery(u.CallbackQuery)
	}
	if u.EditedMessage != nil {
		return inboundFromMessage(u.EditedMessage)
	}
	if u.Message == nil {
		return nil, ErrNoMessage
	}
	return inboundFromMessage(u.Message)
}

// inboundFromMessage handles the Message update kind. Kept as a
// separate function so InboundFromUpdate stays a thin dispatcher.
// The same function handles both fresh Messages and EditedMessages;
// EditDate > 0 is the discriminator that shifts Timestamp and
// stamps Meta[MetaEditedAt].
func inboundFromMessage(m *models.Message) (*envelope.Envelope, error) {
	if m.From == nil {
		return nil, ErrUnsupportedContent
	}
	sender := envelope.Participant{
		ID:   strconv.FormatInt(m.From.ID, 10),
		Name: senderName(m.From),
	}
	env := envelope.New(ChannelName, envelope.Inbound, sender)
	if m.EditDate > 0 {
		env.Timestamp = time.Unix(int64(m.EditDate), 0).UTC()
		env.Meta[MetaEditedAt] = strconv.Itoa(m.EditDate)
	} else {
		env.Timestamp = time.Unix(int64(m.Date), 0).UTC()
	}
	env.Meta[MetaChatID] = strconv.FormatInt(m.Chat.ID, 10)
	env.Meta[MetaMessageID] = strconv.Itoa(m.ID)
	if t := string(m.Chat.Type); t != "" {
		env.Meta[MetaChatType] = t
	}

	appendMediaPart(env, m)
	appendTextPart(env, m)
	parseBotCommand(env, m)

	if len(env.Parts) == 0 {
		return nil, ErrUnsupportedContent
	}
	return env, nil
}

// parseBotCommand inspects the message for a bot_command MessageEntity
// at offset 0 and, when present, exposes the command name and arguments
// in Meta (MetaCommand, MetaCommandArgs). The Text Part itself is left
// intact, so existing text consumers continue to see the original /cmd
// payload verbatim. Phase 2E.5 — see the stage notes for why this
// mapping stays in Meta with the telegram. prefix instead of being
// promoted to a canonical envelope concept.
func parseBotCommand(env *envelope.Envelope, m *models.Message) {
	if m.Text == "" {
		return
	}
	cmdLength := 0
	for _, e := range m.Entities {
		if e.Type == models.MessageEntityTypeBotCommand && e.Offset == 0 {
			cmdLength = e.Length
			break
		}
	}
	// cmdLength must cover at least "/X" (a leading slash plus one
	// character) and stay within Text bounds. Anything else is treated
	// as a missing or malformed command entity and skipped silently.
	if cmdLength <= 1 || cmdLength > len(m.Text) {
		return
	}
	// Bot command names are restricted to ASCII letters, digits and
	// underscore by Telegram, so byte slicing of the command portion
	// is safe; the args portion is taken whole, preserving any unicode
	// the caller placed after the entity boundary.
	nameWithBotname := m.Text[1:cmdLength]
	name := nameWithBotname
	if at := strings.IndexByte(name, '@'); at >= 0 {
		name = name[:at]
	}
	if name == "" {
		return
	}
	env.Meta[MetaCommand] = name
	args := strings.TrimLeft(m.Text[cmdLength:], " \t\n")
	if args != "" {
		env.Meta[MetaCommandArgs] = args
	}
}

// inboundFromCallbackQuery handles the CallbackQuery update kind.
// A callback without an ID cannot be acknowledged, so the adapter
// refuses it rather than letting the orchestrator hold an envelope
// it cannot translate to a SendAnswerCallbackQuery later. An empty
// Data string is treated the same way an empty Message is in
// inboundFromMessage: ErrUnsupportedContent.
func inboundFromCallbackQuery(cq *models.CallbackQuery) (*envelope.Envelope, error) {
	if cq.From.ID == 0 {
		return nil, ErrUnsupportedContent
	}
	if cq.ID == "" {
		return nil, ErrNoMessage
	}
	if cq.Data == "" {
		return nil, ErrUnsupportedContent
	}
	sender := envelope.Participant{
		ID:   strconv.FormatInt(cq.From.ID, 10),
		Name: senderName(&cq.From),
	}
	env := envelope.New(ChannelName, envelope.Inbound, sender)
	env.Meta[MetaCallbackQueryID] = cq.ID
	// The original message that carried the keyboard may be either
	// accessible (full Message) or inaccessible (only chat + id + date
	// retained by Telegram). Preserve chat/message identifiers when
	// available so a downstream consumer can address the original
	// message (e.g. edit it once the side-effect ADR lands).
	if cq.Message.Message != nil {
		om := cq.Message.Message
		env.Meta[MetaChatID] = strconv.FormatInt(om.Chat.ID, 10)
		env.Meta[MetaMessageID] = strconv.Itoa(om.ID)
		if t := string(om.Chat.Type); t != "" {
			env.Meta[MetaChatType] = t
		}
	} else if cq.Message.InaccessibleMessage != nil {
		im := cq.Message.InaccessibleMessage
		env.Meta[MetaChatID] = strconv.FormatInt(im.Chat.ID, 10)
		env.Meta[MetaMessageID] = strconv.Itoa(im.MessageID)
		if t := string(im.Chat.Type); t != "" {
			env.Meta[MetaChatType] = t
		}
	}
	env.AddCallback(cq.Data)
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
// or location Part to env. Telegram messages carry at most one of
// photo / voice / audio / video / document / location in practice; the
// iteration order is deterministic but defensive — if a message ever
// happens to carry more than one, the first match wins.
//
// Location parts are not media in the file-attachment sense but share
// the same exclusivity rule and live alongside media in the switch so
// the "one non-text part per message" invariant stays in one place.
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
	case m.Location != nil:
		// Telegram delivers latitude/longitude as float64. Companion
		// fields (horizontal_accuracy, live_period, heading,
		// proximity_alert_radius) are intentionally NOT mapped: ADR-0004
		// fixes the canonical envelope payload to {lat, lon} until a
		// future amending ADR widens the schema.
		env.AddLocation(m.Location.Latitude, m.Location.Longitude)
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
