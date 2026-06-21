// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"

	"github.com/Sebastian197/korvun/internal/conversation"
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
// The latest non-empty text Part becomes one RoleUser Message; a configured
// systemPrompt prepends a RoleSystem Message. It is the memoryless path: a thin
// wrapper over requestWithHistory with no prior turns.
func envelopeToRequest(in *envelope.Envelope, systemPrompt string) (*model.Request, bool) {
	return requestWithHistory(in, systemPrompt, nil)
}

// requestWithHistory builds the request for the current Envelope with prior
// conversation turns placed between an optional systemPrompt and the current
// user message (oldest first), so the models answer with memory in context
// (ADR-0018 §5). It is pure. The bool reports whether there was anything to ask;
// an Envelope with no text yields false (no reply, no fan-out). A nil/empty
// history degenerates to the single-turn stateless request.
func requestWithHistory(in *envelope.Envelope, systemPrompt string, history []conversation.Turn) (*model.Request, bool) {
	text := latestText(in.Parts)
	if text == "" {
		return nil, false
	}
	msgs := make([]model.Message, 0, len(history)+2)
	if systemPrompt != "" {
		msgs = append(msgs, model.Message{Role: model.RoleSystem, Content: systemPrompt})
	}
	for _, t := range history {
		msgs = append(msgs, model.Message{Role: toModelRole(t.Role), Content: t.Content})
	}
	msgs = append(msgs, model.Message{Role: model.RoleUser, Content: text})
	return &model.Request{Model: fanoutModelPlaceholder, Messages: msgs}, true
}

// toModelRole maps a stored conversation.Role to the model role the providers
// understand. This translation lives in the Orchestrator side (ADR-0018 §2,
// resolution 3) so the conversation package stays a leaf with no model import.
func toModelRole(r conversation.Role) model.Role {
	switch r {
	case conversation.RoleSystem:
		return model.RoleSystem
	case conversation.RoleAssistant:
		return model.RoleAssistant
	default:
		return model.RoleUser
	}
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
