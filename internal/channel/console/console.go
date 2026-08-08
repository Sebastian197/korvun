// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package console is the internal direct-chat channel (operator-console
// spec FR-CONS-1): a first-class channel.Channel with NO network — the
// operator talks to the brains from the app itself. Inbound enters through
// the control API (router.DispatchInbound), never through Receive; Send
// succeeds without I/O because the reply's persistence already happened in
// the brain, and the router's post-Send ReplySent event is the SSE signal
// the chat refreshes on. The ONE thing Send records is a router-marked
// acknowledgement (envelope.MetaAck) as a SYSTEM turn — nothing else ever
// persists it. RED STUBS below where marked.
package console

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// ChannelName is the registry/type name of the console channel.
const ChannelName = "console"

// Channel implements channel.Channel plus the app's Start/Stop lifecycle.
type Channel struct {
	store conversation.SessionStore
	clock func() time.Time

	mu      sync.Mutex
	stopped bool
	inbound chan *envelope.Envelope
}

// Option configures New.
type Option func(*Channel)

// WithClock injects the time source for persisted ack turns.
func WithClock(now func() time.Time) Option {
	return func(c *Channel) {
		if now != nil {
			c.clock = now
		}
	}
}

// New builds the console channel over the sessionful store (required: the
// direct chat IS persistence).
func New(store conversation.SessionStore, opts ...Option) *Channel {
	c := &Channel{store: store, clock: time.Now, inbound: make(chan *envelope.Envelope)}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return ChannelName }

// Manifest implements channel.Channel.
func (c *Channel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }

// Receive implements channel.Channel: the stream exists but never emits —
// console inbound enters through the control API into DispatchInbound.
func (c *Channel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return c.inbound, nil
}

// Send implements channel.Channel. A plain reply is a successful no-op —
// the brain already persisted the pair, and the router's post-Send
// ReplySent event carries the SSE signal. A router-marked acknowledgement
// (envelope.MetaAck) persists as a SYSTEM turn: it exists nowhere else,
// and the chat must show it.
func (c *Channel) Send(ctx context.Context, env *envelope.Envelope) error {
	if env == nil || env.Meta[envelope.MetaAck] == "" {
		return nil
	}
	key, err := conversation.KeyFromEnvelope(env)
	if err != nil {
		return fmt.Errorf("console: ack without conversation identity: %w", err)
	}
	text := ""
	for _, p := range env.Parts {
		if p.Type == envelope.Text && p.Content != "" {
			text = p.Content
		}
	}
	if text == "" {
		return errors.New("console: ack without text")
	}
	if c.store == nil {
		// Only a Preflight-constructed throwaway lacks a store, and
		// Preflight never calls Send — keep the guard honest anyway.
		return errors.New("console: no store wired")
	}
	if _, err := c.store.Append(ctx, key, conversation.Turn{
		Role:      conversation.RoleSystem,
		Content:   text,
		Timestamp: c.clock(),
	}); err != nil {
		return fmt.Errorf("console: persist ack %q: %w", key, err)
	}
	return nil
}

// Start implements the app lifecycle: nothing to connect.
func (c *Channel) Start(context.Context) error { return nil }

// Stop implements the app lifecycle: close the (silent) inbound stream so
// the router pump drains.
func (c *Channel) Stop(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.stopped = true
		close(c.inbound)
	}
	return nil
}
