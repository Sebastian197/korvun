// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"fmt"
	"strconv"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// OutboundKind classifies which Telegram Send* method a translated
// outbound Envelope is destined for. Exactly one of the matching
// fields of an Outbound is populated for each Kind.
type OutboundKind int

// Recognised outbound dispatch kinds. The zero value is intentionally
// invalid so an uninitialised Outbound never silently dispatches.
const (
	OutboundKindMessage OutboundKind = iota + 1
	OutboundKindPhoto
	OutboundKindDocument
	OutboundKindVoice
	OutboundKindAudio
	OutboundKindVideo
	OutboundKindLocation
	OutboundKindAnswerCallback
	OutboundKindEditText
	OutboundKindEditCaption
	OutboundKindDelete
	OutboundKindSetReaction
)

// String returns a short lowercase name for the kind, suitable for
// logs and error messages.
func (k OutboundKind) String() string {
	switch k {
	case OutboundKindMessage:
		return "message"
	case OutboundKindPhoto:
		return "photo"
	case OutboundKindDocument:
		return "document"
	case OutboundKindVoice:
		return "voice"
	case OutboundKindAudio:
		return "audio"
	case OutboundKindVideo:
		return "video"
	case OutboundKindLocation:
		return "location"
	case OutboundKindAnswerCallback:
		return "answer_callback"
	case OutboundKindEditText:
		return "edit_text"
	case OutboundKindEditCaption:
		return "edit_caption"
	case OutboundKindDelete:
		return "delete"
	case OutboundKindSetReaction:
		return "set_reaction"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// Outbound is the tagged-union result of OutboundParams. Exactly one
// of the typed fields is non-nil, matching Kind. The caller dispatches
// the appropriate bot.SendXxx method against the populated field.
type Outbound struct {
	Kind           OutboundKind
	Message        *bot.SendMessageParams
	Photo          *bot.SendPhotoParams
	Document       *bot.SendDocumentParams
	Voice          *bot.SendVoiceParams
	Audio          *bot.SendAudioParams
	Video          *bot.SendVideoParams
	Location       *bot.SendLocationParams
	AnswerCallback *bot.AnswerCallbackQueryParams
	EditText       *bot.EditMessageTextParams
	EditCaption    *bot.EditMessageCaptionParams
	Delete         *bot.DeleteMessageParams
	SetReaction    *bot.SetMessageReactionParams
}

// OutboundParams converts an outbound Envelope into the appropriate
// Telegram Send*/Edit*/Delete*/AnswerCallbackQuery Params, packaged in
// an Outbound tagged union.
//
// Dispatch order:
//
//   - If Envelope.Operation != nil, route to outboundOperation. This
//     covers OpCallbackAck (notification side-effect) and the three
//     mutations OpEditText / OpEditCaption / OpDelete (ADR-0006).
//     Operations bypass the chat-ID / classify-parts pipeline because
//     OpCallbackAck is addressed by callback_query_id alone, and the
//     edit/delete kinds read their own preconditions inside
//     outboundOperation.
//   - Otherwise, fall through the message-shaped dispatch:
//
// Message-shaped dispatch rules (Phase 2.3 + 2E.2 + 2E.3 + 2E.4):
//
//   - An Envelope carrying only text part(s) returns OutboundKindMessage
//     with Message populated; the first non-empty text part becomes
//     SendMessageParams.Text.
//   - An Envelope carrying one media part returns the matching
//     OutboundKind*: Image -> Photo, Video -> Video, File -> Document,
//     Audio -> Audio by default, Audio with Meta[MetaAudioKind] ==
//     AudioKindVoice -> Voice. The first non-empty text part becomes
//     that Send*Params.Caption.
//   - A Location part returns OutboundKindLocation populating
//     SendLocationParams with the canonical lat/lon (ADR-0004).
//   - An Envelope carrying more than one media part returns
//     ErrTooManyMediaParts; per-message Send* methods address one
//     media item at a time and media-group support is out of scope.
//   - When Envelope.Keyboard is non-nil it is translated to an
//     InlineKeyboardMarkup and attached as ReplyMarkup on every
//     Send*Params produced.
//
// The media part's Source must be a Telegram file_id or URL — this
// phase only supports referenced files via models.InputFileString;
// raw uploads via InputFileUpload are deferred to a later phase.
func OutboundParams(e *envelope.Envelope) (*Outbound, error) {
	if e == nil {
		return nil, ErrNilEnvelope
	}
	if e.Direction != envelope.Outbound {
		return nil, ErrNotOutbound
	}
	if e.Channel != ChannelName {
		return nil, fmt.Errorf("%w: got %q", ErrWrongChannel, e.Channel)
	}

	// Side-effect operations (ADR-0006) are routed before the
	// chat-ID / classify-parts pipeline so that operations addressed by
	// a non-chat target (currently only OpCallbackAck) don't trip on
	// the standard message-shaped preconditions.
	if e.Operation != nil {
		return outboundOperation(e)
	}

	chatID, err := parseChatID(e)
	if err != nil {
		return nil, err
	}

	media, text, err := classifyParts(e.Parts)
	if err != nil {
		return nil, err
	}

	replyMarkup := buildReplyMarkup(e.Keyboard)

	if media == nil {
		if text == "" {
			return nil, ErrNoPartsToSend
		}
		return &Outbound{
			Kind: OutboundKindMessage,
			Message: &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        text,
				ReplyMarkup: replyMarkup,
			},
		}, nil
	}

	// Location is the only non-text part that does not carry a Source
	// (its coordinates live in Content per ADR-0004), so it is routed
	// before the InputFileString construction below. Telegram's
	// SendLocationParams has no Caption field; any accompanying text
	// part is intentionally dropped — locked in by a test in
	// location_outbound_test.go.
	if media.Type == envelope.Location {
		lat, lon, ok := media.Location()
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidLocation, media.Content)
		}
		return &Outbound{
			Kind: OutboundKindLocation,
			Location: &bot.SendLocationParams{
				ChatID:      chatID,
				Latitude:    lat,
				Longitude:   lon,
				ReplyMarkup: replyMarkup,
			},
		}, nil
	}

	if media.Source == "" {
		return nil, ErrMissingMediaSource
	}
	input := &models.InputFileString{Data: media.Source}

	switch media.Type {
	case envelope.Image:
		return &Outbound{
			Kind: OutboundKindPhoto,
			Photo: &bot.SendPhotoParams{
				ChatID:      chatID,
				Photo:       input,
				Caption:     text,
				ReplyMarkup: replyMarkup,
			},
		}, nil
	case envelope.Audio:
		if e.Meta[MetaAudioKind] == AudioKindVoice {
			return &Outbound{
				Kind: OutboundKindVoice,
				Voice: &bot.SendVoiceParams{
					ChatID:      chatID,
					Voice:       input,
					Caption:     text,
					ReplyMarkup: replyMarkup,
				},
			}, nil
		}
		return &Outbound{
			Kind: OutboundKindAudio,
			Audio: &bot.SendAudioParams{
				ChatID:      chatID,
				Audio:       input,
				Caption:     text,
				ReplyMarkup: replyMarkup,
			},
		}, nil
	case envelope.Video:
		return &Outbound{
			Kind: OutboundKindVideo,
			Video: &bot.SendVideoParams{
				ChatID:      chatID,
				Video:       input,
				Caption:     text,
				ReplyMarkup: replyMarkup,
			},
		}, nil
	case envelope.File:
		return &Outbound{
			Kind: OutboundKindDocument,
			Document: &bot.SendDocumentParams{
				ChatID:      chatID,
				Document:    input,
				Caption:     text,
				ReplyMarkup: replyMarkup,
			},
		}, nil
	default:
		// envelope.Text already handled above; any other PartType is
		// not covered yet.
		return nil, ErrUnsupportedContent
	}
}

// outboundOperation routes a side-effect Envelope to its native
// Telegram primitive based on Operation.Kind. Notifications
// (OpCallbackAck) and mutations (OpEditText, OpEditCaption, OpDelete)
// all flow through here per the side-effect taxonomy in ADR-0006.
func outboundOperation(e *envelope.Envelope) (*Outbound, error) {
	switch e.Operation.Kind {
	case envelope.OpCallbackAck:
		return outboundAnswerCallback(e)
	case envelope.OpEditText:
		return outboundEditText(e)
	case envelope.OpEditCaption:
		return outboundEditCaption(e)
	case envelope.OpDelete:
		return outboundDelete(e)
	case envelope.OpSetReaction:
		return outboundSetReaction(e)
	default:
		return nil, ErrUnsupportedContent
	}
}

// outboundAnswerCallback translates an OpCallbackAck Envelope into a
// *bot.AnswerCallbackQueryParams. The callback_query_id Meta key is
// required; its absence is the only failure mode here (Operation
// shape has already been guarded by Validate at the call seam). The
// toast text rides as the first Text Part's Content; a silent ack
// (empty Parts) yields an empty Text on the params.
func outboundAnswerCallback(e *envelope.Envelope) (*Outbound, error) {
	id := e.Meta[MetaCallbackQueryID]
	if id == "" {
		return nil, ErrMissingCallbackQueryID
	}
	text := ""
	if len(e.Parts) == 1 {
		text = e.Parts[0].Content
	}
	return &Outbound{
		Kind: OutboundKindAnswerCallback,
		AnswerCallback: &bot.AnswerCallbackQueryParams{
			CallbackQueryID: id,
			Text:            text,
		},
	}, nil
}

// outboundEditText translates an OpEditText Envelope into a
// *bot.EditMessageTextParams. Target chat and message IDs come from
// Meta; the new body is the first Text Part's Content (Validate
// guaranteed exactly one Text Part with non-empty Content);
// Envelope.Keyboard, if any, becomes the new ReplyMarkup.
func outboundEditText(e *envelope.Envelope) (*Outbound, error) {
	chatID, err := parseChatID(e)
	if err != nil {
		return nil, err
	}
	messageID, err := parseTargetMessageID(e)
	if err != nil {
		return nil, err
	}
	return &Outbound{
		Kind: OutboundKindEditText,
		EditText: &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        e.Parts[0].Content,
			ReplyMarkup: buildReplyMarkup(e.Keyboard),
		},
	}, nil
}

// outboundEditCaption translates an OpEditCaption Envelope into a
// *bot.EditMessageCaptionParams. The Text Part's Content becomes
// Caption (an empty Content is a legitimate intent to clear the
// caption); Envelope.Keyboard, if any, becomes the new ReplyMarkup.
func outboundEditCaption(e *envelope.Envelope) (*Outbound, error) {
	chatID, err := parseChatID(e)
	if err != nil {
		return nil, err
	}
	messageID, err := parseTargetMessageID(e)
	if err != nil {
		return nil, err
	}
	return &Outbound{
		Kind: OutboundKindEditCaption,
		EditCaption: &bot.EditMessageCaptionParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Caption:     e.Parts[0].Content,
			ReplyMarkup: buildReplyMarkup(e.Keyboard),
		},
	}, nil
}

// outboundSetReaction translates an OpSetReaction Envelope into a
// *bot.SetMessageReactionParams. Each Text Part's Content becomes a
// models.ReactionType of the emoji variant; empty Parts maps to an
// empty []ReactionType, which Telegram's setMessageReaction
// endpoint interprets as "clear all the bot's reactions on the
// target". Validate has already guaranteed that every Part is Text
// with non-empty Content.
func outboundSetReaction(e *envelope.Envelope) (*Outbound, error) {
	chatID, err := parseChatID(e)
	if err != nil {
		return nil, err
	}
	messageID, err := parseTargetMessageID(e)
	if err != nil {
		return nil, err
	}
	reactions := make([]models.ReactionType, len(e.Parts))
	for i, p := range e.Parts {
		reactions[i] = models.ReactionType{
			Type: models.ReactionTypeTypeEmoji,
			ReactionTypeEmoji: &models.ReactionTypeEmoji{
				Type:  models.ReactionTypeTypeEmoji,
				Emoji: p.Content,
			},
		}
	}
	return &Outbound{
		Kind: OutboundKindSetReaction,
		SetReaction: &bot.SetMessageReactionParams{
			ChatID:    chatID,
			MessageID: messageID,
			Reaction:  reactions,
		},
	}, nil
}

// outboundDelete translates an OpDelete Envelope into a
// *bot.DeleteMessageParams. No Parts, no Keyboard (Validate
// guaranteed both); only chat and message IDs are consulted.
func outboundDelete(e *envelope.Envelope) (*Outbound, error) {
	chatID, err := parseChatID(e)
	if err != nil {
		return nil, err
	}
	messageID, err := parseTargetMessageID(e)
	if err != nil {
		return nil, err
	}
	return &Outbound{
		Kind: OutboundKindDelete,
		Delete: &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		},
	}, nil
}

// parseTargetMessageID extracts the int message ID from
// env.Meta[MetaMessageID] for edit/delete operations. Returns
// ErrMissingTargetMessageID when the entry is absent, empty, or not
// parseable as an int.
func parseTargetMessageID(e *envelope.Envelope) (int, error) {
	s, ok := e.Meta[MetaMessageID]
	if !ok || s == "" {
		return 0, ErrMissingTargetMessageID
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %v", ErrMissingTargetMessageID, s, err)
	}
	return id, nil
}

// buildReplyMarkup translates a canonical envelope.Keyboard into a
// Telegram InlineKeyboardMarkup value. Returns nil when k is nil, so
// the caller can assign the result unconditionally to ReplyMarkup
// without a guarded branch per Send*Params.
func buildReplyMarkup(k *envelope.Keyboard) models.ReplyMarkup {
	if k == nil {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, len(k.Rows))
	for i, row := range k.Rows {
		buttons := make([]models.InlineKeyboardButton, len(row))
		for j, b := range row {
			buttons[j] = models.InlineKeyboardButton{
				Text:         b.Text,
				CallbackData: b.CallbackData,
				URL:          b.URL,
			}
		}
		rows[i] = buttons
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// OutboundToSendMessage converts an outbound Envelope into a
// bot.SendMessageParams ready to be passed to bot.SendMessage.
//
// Phase 2.3 only supports text messages: the first non-empty text
// part of the Envelope is used as the message body. The Envelope
// must carry telegram.chat_id in Meta as a stringified int64; missing,
// empty, or non-numeric values are rejected with a sentinel error.
//
// New callers should prefer OutboundParams (Phase 2E.2), which also
// handles media; OutboundToSendMessage is preserved for the
// text-only contract pinned in Phase 2.3.
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
	chatID, err := parseChatID(e)
	if err != nil {
		return nil, err
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

// parseChatID extracts the int64 chat ID from env.Meta[MetaChatID].
// Returns ErrMissingChatID for absent or empty values and
// ErrInvalidChatID (wrapped with the offending string) for values
// that are not parseable as int64.
func parseChatID(e *envelope.Envelope) (int64, error) {
	chatIDStr, ok := e.Meta[MetaChatID]
	if !ok || chatIDStr == "" {
		return 0, ErrMissingChatID
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %v", ErrInvalidChatID, chatIDStr, err)
	}
	return chatID, nil
}

// classifyParts walks parts and returns:
//   - a pointer to the single media part, or nil if none.
//   - the first non-empty text part's content as the caption / body.
//   - ErrTooManyMediaParts if more than one media part is present.
//
// The returned pointer aliases an element of the caller's slice; the
// caller is responsible for not mutating it.
func classifyParts(parts []envelope.Part) (*envelope.Part, string, error) {
	var media *envelope.Part
	var text string
	for i := range parts {
		p := &parts[i]
		if p.Type == envelope.Text {
			if text == "" && p.Content != "" {
				text = p.Content
			}
			continue
		}
		if media != nil {
			return nil, "", ErrTooManyMediaParts
		}
		media = p
	}
	return media, text, nil
}

// firstTextPart returns the content of the first non-empty text part
// in parts, or "" if there is none. Kept for the Phase 2.3
// OutboundToSendMessage path; OutboundParams uses classifyParts.
func firstTextPart(parts []envelope.Part) string {
	for _, p := range parts {
		if p.Type == envelope.Text && p.Content != "" {
			return p.Content
		}
	}
	return ""
}
