// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
)

func TestSend_dispatchByKind(t *testing.T) {
	type tcase struct {
		name      string
		build     func() *envelope.Envelope
		wantKind  OutboundKind
		assertHit func(t *testing.T, c *capturingBotClient)
	}
	cases := []tcase{
		{
			name: "text message routes to SendMessage",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.AddText("hola")
				return e
			},
			wantKind: OutboundKindMessage,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastMessage == nil {
					t.Fatal("SendMessage was not called")
				}
				if c.lastMessage.Text != "hola" {
					t.Errorf("Text = %q, want %q", c.lastMessage.Text, "hola")
				}
				if c.lastMessage.ChatID.(int64) != 12345 {
					t.Errorf("ChatID = %v, want 12345", c.lastMessage.ChatID)
				}
			},
		},
		{
			name: "photo media routes to SendPhoto",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.AddMedia(envelope.Image, "file-id-1", "")
				return e
			},
			wantKind: OutboundKindPhoto,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastPhoto == nil {
					t.Fatal("SendPhoto was not called")
				}
			},
		},
		{
			name: "document media routes to SendDocument",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.AddMedia(envelope.File, "file-doc", "application/pdf")
				return e
			},
			wantKind: OutboundKindDocument,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastDocument == nil {
					t.Fatal("SendDocument was not called")
				}
			},
		},
		{
			name: "audio media routes to SendAudio",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.AddMedia(envelope.Audio, "file-audio", "audio/mpeg")
				return e
			},
			wantKind: OutboundKindAudio,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastAudio == nil {
					t.Fatal("SendAudio was not called")
				}
			},
		},
		{
			name: "audio media with voice meta routes to SendVoice",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.Meta[MetaAudioKind] = AudioKindVoice
				e.AddMedia(envelope.Audio, "file-voice", "audio/ogg")
				return e
			},
			wantKind: OutboundKindVoice,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastVoice == nil {
					t.Fatal("SendVoice was not called")
				}
			},
		},
		{
			name: "video media routes to SendVideo",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.AddMedia(envelope.Video, "file-vid", "video/mp4")
				return e
			},
			wantKind: OutboundKindVideo,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastVideo == nil {
					t.Fatal("SendVideo was not called")
				}
			},
		},
		{
			name: "location routes to SendLocation",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.AddLocation(40.4168, -3.7038)
				return e
			},
			wantKind: OutboundKindLocation,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastLocation == nil {
					t.Fatal("SendLocation was not called")
				}
			},
		},
		{
			name: "callback ack routes to AnswerCallbackQuery",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.Meta[MetaCallbackQueryID] = "cb-42"
				e.SetCallbackAck("got it")
				return e
			},
			wantKind: OutboundKindAnswerCallback,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastAnswerCallback == nil {
					t.Fatal("AnswerCallbackQuery was not called")
				}
				if c.lastAnswerCallback.CallbackQueryID != "cb-42" {
					t.Errorf("CallbackQueryID = %q, want %q", c.lastAnswerCallback.CallbackQueryID, "cb-42")
				}
			},
		},
		{
			name: "edit text routes to EditMessageText",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.Meta[MetaMessageID] = "99"
				e.SetEditText("nuevo texto")
				return e
			},
			wantKind: OutboundKindEditText,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastEditText == nil {
					t.Fatal("EditMessageText was not called")
				}
			},
		},
		{
			name: "edit caption routes to EditMessageCaption",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.Meta[MetaMessageID] = "99"
				e.SetEditCaption("nuevo caption")
				return e
			},
			wantKind: OutboundKindEditCaption,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastEditCaption == nil {
					t.Fatal("EditMessageCaption was not called")
				}
			},
		},
		{
			name: "delete routes to DeleteMessage",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.Meta[MetaMessageID] = "99"
				e.SetDelete()
				return e
			},
			wantKind: OutboundKindDelete,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastDelete == nil {
					t.Fatal("DeleteMessage was not called")
				}
			},
		},
		{
			name: "set reaction routes to SetMessageReaction",
			build: func() *envelope.Envelope {
				e := newOutboundEnvelope()
				e.Meta[MetaMessageID] = "99"
				e.SetReactions("👍")
				return e
			},
			wantKind: OutboundKindSetReaction,
			assertHit: func(t *testing.T, c *capturingBotClient) {
				if c.lastSetReaction == nil {
					t.Fatal("SetMessageReaction was not called")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &capturingBotClient{}
			a, err := New(
				WithToken("test-token"),
				WithMode(ModePolling),
				withInjectedBotForTests(fake),
			)
			if err != nil {
				t.Fatalf("New() err = %v", err)
			}
			if err := a.Send(context.Background(), tc.build()); err != nil {
				t.Fatalf("Send() err = %v", err)
			}
			tc.assertHit(t, fake)
		})
	}
}

func TestSend_rejectsNilEnvelope(t *testing.T) {
	a := newTestAdapter(t)
	if err := a.Send(context.Background(), nil); !errors.Is(err, ErrNilEnvelope) {
		t.Fatalf("Send(nil) err = %v, want ErrNilEnvelope", err)
	}
}

func TestSend_rejectsWrongChannel(t *testing.T) {
	a := newTestAdapter(t)
	env := envelope.New("not-telegram", envelope.Outbound, envelope.Participant{ID: "u"})
	env.AddText("hi")
	if err := a.Send(context.Background(), env); !errors.Is(err, ErrWrongChannel) {
		t.Fatalf("Send wrong channel err = %v, want ErrWrongChannel", err)
	}
}

func TestSend_rejectsInboundDirection(t *testing.T) {
	a := newTestAdapter(t)
	env := envelope.New(ChannelName, envelope.Inbound, envelope.Participant{ID: "u"})
	env.AddText("hi")
	if err := a.Send(context.Background(), env); !errors.Is(err, ErrNotOutbound) {
		t.Fatalf("Send inbound err = %v, want ErrNotOutbound", err)
	}
}

func TestSend_wrapsTransportError(t *testing.T) {
	transportErr := errors.New("network fail")
	fake := &capturingBotClient{
		errOn:     OutboundKindMessage,
		forcedErr: transportErr,
	}
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		withInjectedBotForTests(fake),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	env := newOutboundEnvelope()
	env.AddText("hola")
	err = a.Send(context.Background(), env)
	if err == nil {
		t.Fatal("Send() err = nil, want wrapped transport error")
	}
	if !errors.Is(err, transportErr) {
		t.Errorf("Send() err = %v, does not wrap transport err", err)
	}
}

// newOutboundEnvelope builds a minimal outbound Envelope for the
// Telegram channel, addressed at a fixed chat ID. Test cases add
// the parts / operation / meta they need on top.
func newOutboundEnvelope() *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.Meta[MetaChatID] = "12345"
	return e
}
