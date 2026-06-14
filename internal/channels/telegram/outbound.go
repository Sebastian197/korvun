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
}

// OutboundParams converts an outbound Envelope into the appropriate
// Telegram Send*Params, packaged in an Outbound tagged union.
//
// Dispatch rules (Phase 2E.2):
//
//   - An Envelope carrying only text part(s) returns OutboundKindMessage
//     with Message populated; the first non-empty text part becomes
//     SendMessageParams.Text.
//   - An Envelope carrying one media part returns the matching
//     OutboundKind*: Image -> Photo, Video -> Video, File -> Document,
//     Audio -> Audio by default, Audio with Meta[MetaAudioKind] ==
//     AudioKindVoice -> Voice. The first non-empty text part becomes
//     that Send*Params.Caption.
//   - An Envelope carrying more than one media part returns
//     ErrTooManyMediaParts; per-message Send* methods address one
//     media item at a time and media-group support is out of scope.
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

	// CallbackAck is an outbound primitive with no ChatID and no
	// message body; it is addressed by the callback query ID alone.
	// Route it before parseChatID / classifyParts so those
	// message-shaped preconditions don't trip on it.
	if ack := findCallbackAckPart(e.Parts); ack != nil {
		return outboundAnswerCallback(e, ack)
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

// findCallbackAckPart returns the first CallbackAck part in parts, or
// nil if none is present. Validate's exclusivity rule guarantees that
// a CallbackAck part, when present, is the only part — so the search
// terminates on the first match without scanning further.
func findCallbackAckPart(parts []envelope.Part) *envelope.Part {
	for i := range parts {
		if parts[i].Type == envelope.CallbackAck {
			return &parts[i]
		}
	}
	return nil
}

// outboundAnswerCallback translates a CallbackAck envelope into a
// *bot.AnswerCallbackQueryParams. The callback_query_id Meta key is
// required; its absence is the only failure mode here (the Part shape
// has already been guarded by Validate at the call seam).
func outboundAnswerCallback(e *envelope.Envelope, ack *envelope.Part) (*Outbound, error) {
	id := e.Meta[MetaCallbackQueryID]
	if id == "" {
		return nil, ErrMissingCallbackQueryID
	}
	return &Outbound{
		Kind: OutboundKindAnswerCallback,
		AnswerCallback: &bot.AnswerCallbackQueryParams{
			CallbackQueryID: id,
			Text:            ack.Content,
		},
	}, nil
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
