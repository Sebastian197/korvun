// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/router"
)

// Audit finding R-7: a reply whose Channel names an unregistered channel was
// the ONLY discard path in the router without a hook notification — a brain
// bug (OUR bug, not the operator's) vanished silently. This file pins the
// contract: such a reply surfaces ErrKindUnknownReplyChannel to the error
// hook (and, through the app's generic RouterError→event mapping, becomes an
// observable MessageDropped), wrapping ErrUnknownChannel.

func TestSendReply_UnknownChannelHooksError(t *testing.T) {
	hook := make(chan router.RouterError, 8)
	r := router.New(
		router.WithSendTimeout(time.Second),
		router.WithErrorHandler(func(re router.RouterError) {
			select {
			case hook <- re:
			default:
			}
		}),
	)
	t.Cleanup(func() { shutdown(t, r) })

	ch := newFakeChannel("ch")
	_ = r.RegisterChannel(ch)

	// The brain replies onto a channel that does not exist ("ghost") — a
	// reply-construction bug in the brain, not operator misconfiguration.
	_ = r.RegisterBrain("brain", newFakeBrain(mkOutbound("ghost", "c", "r")))
	_ = r.Route("ch", "brain")

	_ = r.DispatchInbound(context.Background(), mkInbound("ch", "c", "x"))

	select {
	case re := <-hook:
		if re.Kind != router.ErrKindUnknownReplyChannel {
			t.Errorf("Kind = %v, want ErrKindUnknownReplyChannel", re.Kind)
		}
		if !errors.Is(re.Err, router.ErrUnknownChannel) {
			t.Errorf("Err = %v, want errors.Is(_, ErrUnknownChannel)", re.Err)
		}
		if re.Channel != "ghost" {
			t.Errorf("Channel = %q, want %q (the channel the reply named)", re.Channel, "ghost")
		}
		if re.Envelope == nil {
			t.Error("Envelope = nil, want the dropped reply for the log funnel")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no hook notification within 500ms — the reply vanished silently (the R-7 bug)")
	}

	// The registered channel must have received nothing: the reply named
	// another channel, and there is no fallback delivery.
	if got := len(ch.Sent()); got != 0 {
		t.Errorf("registered channel received %d envelopes, want 0", got)
	}
}

func TestErrKindUnknownReplyChannel_String(t *testing.T) {
	if got := router.ErrKindUnknownReplyChannel.String(); got != "unknown_reply_channel" {
		t.Errorf("String() = %q, want %q", got, "unknown_reply_channel")
	}
}
