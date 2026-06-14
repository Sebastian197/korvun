// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"fmt"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// Send delivers an outbound Envelope through Telegram by classifying
// it with OutboundParams and dispatching the resulting Outbound to
// the matching botClient method.
//
// Conversion errors (nil envelope, wrong direction, wrong channel,
// missing chat_id, no sendable parts, etc.) propagate verbatim so
// the caller sees the same sentinel that OutboundParams would
// surface; transport errors are wrapped with the outbound kind name
// to make logs and metrics legible at a glance.
//
// Send refuses to dispatch unrecognised Kind values (the zero value
// or any future-added kind not handled here) with ErrUnknownOutboundKind.
// This is defensive — every existing Kind has a switch case — and
// prevents a silently-no-op Send if OutboundParams ever grows a
// case the adapter has not been updated to handle.
func (a *Adapter) Send(ctx context.Context, env *envelope.Envelope) error {
	out, err := OutboundParams(env)
	if err != nil {
		return err
	}
	switch out.Kind {
	case OutboundKindMessage:
		_, err = a.client.SendMessage(ctx, out.Message)
	case OutboundKindPhoto:
		_, err = a.client.SendPhoto(ctx, out.Photo)
	case OutboundKindDocument:
		_, err = a.client.SendDocument(ctx, out.Document)
	case OutboundKindVoice:
		_, err = a.client.SendVoice(ctx, out.Voice)
	case OutboundKindAudio:
		_, err = a.client.SendAudio(ctx, out.Audio)
	case OutboundKindVideo:
		_, err = a.client.SendVideo(ctx, out.Video)
	case OutboundKindLocation:
		_, err = a.client.SendLocation(ctx, out.Location)
	case OutboundKindAnswerCallback:
		_, err = a.client.AnswerCallbackQuery(ctx, out.AnswerCallback)
	case OutboundKindEditText:
		_, err = a.client.EditMessageText(ctx, out.EditText)
	case OutboundKindEditCaption:
		_, err = a.client.EditMessageCaption(ctx, out.EditCaption)
	case OutboundKindDelete:
		_, err = a.client.DeleteMessage(ctx, out.Delete)
	case OutboundKindSetReaction:
		_, err = a.client.SetMessageReaction(ctx, out.SetReaction)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownOutboundKind, out.Kind)
	}
	if err != nil {
		return fmt.Errorf("telegram: send %s: %w", out.Kind, err)
	}
	return nil
}
