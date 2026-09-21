// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// TestIdentity_DispatchBeforeReadyIsCounted attacks the gate this phase added
// to the read loop. Refusing a MESSAGE_CREATE that arrives before READY or
// RESUMED is right — the session is not authenticated yet, so no capability
// can honestly be minted for it — but the refusal dropped the message with no
// counter and no line, so a gateway misbehaving this way would look like
// silence rather than loss.
//
// The demanded outcome is both halves at once: nothing reaches the inbound
// queue, AND DroppedCount says a message was lost.
//
// Evidence level: in-process over a REAL WebSocket connection to the fake
// gateway (httptest server, the server side of coder/websocket), driving the
// production read loop.
// Probing mutation executed: delete a.dropped.Add(1) from the pre-READY branch
// — the count stays 0 and this mould reddens.
func TestIdentity_DispatchBeforeReadyIsCounted(t *testing.T) {
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, _ string) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		// The identify the client sends back is read and ignored; the point is
		// what happens to a dispatch that arrives BEFORE READY.
		_, _ = serverRead(ctx, c)
		_ = wsjson.Write(ctx, c, msgFrame(1, humanMsg))
		drainClient(ctx, c)
	})
	adapter := newGatewayAdapter(t, url)
	inbound, stop := startGateway(t, adapter)
	t.Cleanup(stop)

	waitFor(t, 2*time.Second, func() bool { return adapter.DroppedCount() >= 1 })
	if got := adapter.DroppedCount(); got == 0 {
		t.Fatal("a dispatch before the session was authenticated vanished without being counted")
	}
	select {
	case env := <-inbound:
		t.Fatalf("a pre-READY dispatch reached the queue: %+v", env)
	default:
	}
}
