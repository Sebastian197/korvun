// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
)

func TestEnvelopeToRequest(t *testing.T) {
	t.Parallel()

	t.Run("latest text becomes a user message", func(t *testing.T) {
		t.Parallel()
		in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
		in.AddText("first").AddText("latest")
		req, ok := envelopeToRequest(in, "")
		if !ok {
			t.Fatal("ok = false, want true for a text Envelope")
		}
		if req.Model == "" {
			t.Error("req.Model is empty; fanout.Run would reject it")
		}
		if len(req.Messages) != 1 {
			t.Fatalf("got %d messages, want 1", len(req.Messages))
		}
		if req.Messages[0].Role != model.RoleUser || req.Messages[0].Content != "latest" {
			t.Errorf("message = %+v, want RoleUser %q (latest text wins)", req.Messages[0], "latest")
		}
	})

	t.Run("system prompt prepends a system message", func(t *testing.T) {
		t.Parallel()
		in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
		in.AddText("hi")
		req, ok := envelopeToRequest(in, "you are terse")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if len(req.Messages) != 2 {
			t.Fatalf("got %d messages, want 2 (system + user)", len(req.Messages))
		}
		if req.Messages[0].Role != model.RoleSystem || req.Messages[0].Content != "you are terse" {
			t.Errorf("first message = %+v, want RoleSystem prompt", req.Messages[0])
		}
		if req.Messages[1].Role != model.RoleUser {
			t.Errorf("second message role = %v, want RoleUser", req.Messages[1].Role)
		}
	})

	t.Run("no text yields nothing to ask", func(t *testing.T) {
		t.Parallel()
		in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
		in.AddLocation(40.0, -3.0) // a non-text part
		req, ok := envelopeToRequest(in, "")
		if ok || req != nil {
			t.Errorf("got (%v, %v), want (nil, false) for a no-text Envelope", req, ok)
		}
	})

	t.Run("empty parts yields nothing to ask", func(t *testing.T) {
		t.Parallel()
		in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
		if _, ok := envelopeToRequest(in, ""); ok {
			t.Error("ok = true for an Envelope with no parts, want false")
		}
	})

	t.Run("whitespace-only text is nothing to ask", func(t *testing.T) {
		t.Parallel()
		in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
		in.AddText("  \n\t ")
		if req, ok := envelopeToRequest(in, ""); ok || req != nil {
			t.Errorf("got (%v, %v), want (nil, false) for whitespace-only text", req, ok)
		}
	})
}

func TestDecisionToEnvelopes_emptyMeta(t *testing.T) {
	t.Parallel()
	// An inbound carrying no addressing Meta must still yield a Validate-clean
	// outbound (undeliverable is a delivery concern, not a Brain crash).
	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
	in.AddText("question")
	out := decisionToEnvelopes("reply", in)
	if len(out) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(out))
	}
	if err := out[0].Validate(); err != nil {
		t.Errorf("empty-Meta inbound produced an invalid reply: %v", err)
	}
}

func TestDecisionToEnvelopes(t *testing.T) {
	t.Parallel()

	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "user1"})
	in.AddText("question")
	in.Meta["telegram.chat_id"] = "chat-7"
	in.Meta["telegram.message_id"] = "100"

	out := decisionToEnvelopes("the reply", in)
	if len(out) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(out))
	}
	reply := out[0]

	if err := reply.Validate(); err != nil {
		t.Errorf("outbound reply is not valid: %v", err)
	}
	if reply.Direction != envelope.Outbound {
		t.Errorf("Direction = %v, want Outbound", reply.Direction)
	}
	if reply.Channel != "telegram" {
		t.Errorf("Channel = %q, want telegram (echoed)", reply.Channel)
	}
	if reply.Sender.ID == "" {
		t.Error("Sender.ID is empty; Validate would reject the reply")
	}
	if len(reply.Parts) != 1 || reply.Parts[0].Type != envelope.Text || reply.Parts[0].Content != "the reply" {
		t.Errorf("parts = %+v, want one text part %q", reply.Parts, "the reply")
	}
	// All inbound addressing Meta is echoed (v1 echo-all, ADR-0014 §5 TODO).
	if reply.Meta["telegram.chat_id"] != "chat-7" {
		t.Errorf("chat_id = %q, want chat-7 echoed", reply.Meta["telegram.chat_id"])
	}
	if reply.Meta["telegram.message_id"] != "100" {
		t.Errorf("message_id = %q, want 100 echoed", reply.Meta["telegram.message_id"])
	}

	// The outbound Meta must be a distinct map (mutating it must not touch the
	// inbound).
	reply.Meta["telegram.chat_id"] = "mutated"
	if in.Meta["telegram.chat_id"] != "chat-7" {
		t.Error("outbound Meta aliases the inbound Meta; mutation leaked back")
	}
}
