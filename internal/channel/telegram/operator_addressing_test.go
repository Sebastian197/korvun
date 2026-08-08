// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The smoke defect of 2026-08-08, reproduced red-first (operator-console
// spec, AS-5 correction): the operator console addresses replies by the
// conversation KEY alone — its envelopes carry Meta["conversation.id"] and
// nothing channel-specific. For telegram, conversation.id IS the chat id
// verbatim (the adapter's own inbound construction copies MetaChatID into
// it), so an outbound with only the conversation identity MUST be
// addressable. Before the fix, parseChatID read only telegram.chat_id and
// the operator's reply died with ErrMissingChatID — persisted in history,
// never delivered to the phone.
package telegram

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

func operatorReply(meta map[string]string) *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "operator"}).
		AddText("aquí Chano, te atiendo")
	for k, v := range meta {
		e.Meta[k] = v
	}
	return e
}

func TestOutbound_AddressableByConversationIDAlone(t *testing.T) {
	// The console's exact envelope shape: conversation identity only.
	params, err := OutboundToSendMessage(operatorReply(map[string]string{
		conversation.MetaConversationID: "8604622746",
	}))
	if err != nil {
		t.Fatalf("operator reply with conversation.id only: %v (the smoke defect)", err)
	}
	if params.ChatID != int64(8604622746) {
		t.Fatalf("ChatID = %v, want 8604622746", params.ChatID)
	}
}

func TestOutbound_ExplicitChatIDStillWins(t *testing.T) {
	// Brain replies echo the full inbound Meta: telegram.chat_id present
	// stays authoritative even if a (hypothetically different)
	// conversation.id rides along.
	params, err := OutboundToSendMessage(operatorReply(map[string]string{
		MetaChatID:                      "111",
		conversation.MetaConversationID: "222",
	}))
	if err != nil {
		t.Fatalf("explicit chat id: %v", err)
	}
	if params.ChatID != int64(111) {
		t.Fatalf("ChatID = %v, want the explicit telegram.chat_id 111", params.ChatID)
	}
}

func TestOutbound_NoAddressingAtAllStaysAnHonestError(t *testing.T) {
	_, err := OutboundToSendMessage(operatorReply(nil))
	if !errors.Is(err, ErrMissingChatID) {
		t.Fatalf("no addressing: err = %v, want ErrMissingChatID", err)
	}
	// And a non-numeric conversation.id is invalid, not silently dropped.
	_, err = OutboundToSendMessage(operatorReply(map[string]string{
		conversation.MetaConversationID: "not-a-chat",
	}))
	if !errors.Is(err, ErrInvalidChatID) {
		t.Fatalf("bad conversation.id: err = %v, want ErrInvalidChatID", err)
	}
}
