// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
)

// fanoutModelPlaceholder is the Request.Model the Brain sets before fan-out.
// It is non-empty only to satisfy fanout.Run's shared-request validation
// (model.ValidateRequest rejects an empty Model); each provider's real id is
// supplied per-goroutine by WithModelID, which overrides Model on a copy.
const fanoutModelPlaceholder = "korvun-fanout"

// assistant is the Sender stamped on outbound replies. Validate requires a
// non-empty Sender.ID; the delivering channel addresses the reply from the
// echoed Meta (e.g. telegram.chat_id), not from Sender, so this is purely the
// assistant's identity, not a routing field.
var assistant = envelope.Participant{ID: "korvun", Name: "Korvun"}

// envelopeToRequest translates an inbound Envelope into a model.Request. It is
// pure: a deterministic function of its inputs with no I/O. The bool reports
// whether there was anything to ask — an Envelope carrying no text (a reaction,
// a location, a bare callback) yields false, so Handle returns no reply rather
// than feeding an invalid request to the fan-out (ADR-0014 §5).
//
// v1 is single-turn and stateless: the latest non-empty text Part becomes one
// RoleUser Message; a configured systemPrompt prepends a RoleSystem Message.
func envelopeToRequest(in *envelope.Envelope, systemPrompt string) (*model.Request, bool) {
	text := latestText(in.Parts)
	if text == "" {
		return nil, false
	}
	msgs := make([]model.Message, 0, 2)
	if systemPrompt != "" {
		msgs = append(msgs, model.Message{Role: model.RoleSystem, Content: systemPrompt})
	}
	msgs = append(msgs, model.Message{Role: model.RoleUser, Content: text})
	return &model.Request{Model: fanoutModelPlaceholder, Messages: msgs}, true
}

// latestText returns the Content of the last Text Part with non-whitespace
// content, or "" when the Envelope carries no askable text. Whitespace-only
// text is treated as nothing to ask (so the Brain does not spend a full
// fan-out on blanks); the original Content is returned untrimmed so the model
// sees exactly what the user sent.
func latestText(parts []envelope.Part) string {
	text := ""
	for _, p := range parts {
		if p.Type == envelope.Text && strings.TrimSpace(p.Content) != "" {
			text = p.Content
		}
	}
	return text
}

// decisionToEnvelopes builds the outbound reply carrying content, addressed
// back to the originating channel. It is pure. The inbound Meta is echoed onto
// the outbound so the channel can deliver the reply (e.g. via
// telegram.chat_id) without the Brain knowing any channel-specific key — this
// keeps the Brain channel-agnostic (ADR-0014 §5).
func decisionToEnvelopes(content string, in *envelope.Envelope) []*envelope.Envelope {
	out := envelope.New(in.Channel, envelope.Outbound, assistant).AddText(content)
	// TODO(multi-channel): with one channel in front, echo the whole inbound
	// Meta. When a second channel exists, split addressing keys that MUST
	// round-trip (e.g. telegram.chat_id) from inbound-only metadata that must
	// NOT propagate to the outbound (e.g. an edited_at / reaction_action key).
	for k, v := range in.Meta {
		out.Meta[k] = v
	}
	return []*envelope.Envelope{out}
}
