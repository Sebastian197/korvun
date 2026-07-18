// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"

	"github.com/coder/websocket"
)

// connectGateway opens the Discord Gateway WebSocket and (in later sub-phases) runs
// the receive loop over it. Sub-phase 1 is a stub: it returns
// ErrReceiveNotImplemented and exists so the coder/websocket dependency (ADR-0034)
// is a REAL import — referenced in this signature's *websocket.Conn return — not a
// phantom go.mod entry that `go mod tidy` would strip.
//
// SP3 fills in the handshake here: dial via websocket.Dial, resolve the bot token
// from a.cfg.tokenEnv, send Identify with it + intents 37377, heartbeat + ACK, and
// pump dispatches to the inbound channel; SP4 adds resume/reconnect.
func (a *Adapter) connectGateway(_ context.Context) (*websocket.Conn, error) {
	return nil, ErrReceiveNotImplemented
}
