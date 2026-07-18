// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

// ChannelName is the unique identifier of the Discord channel (the value of
// Name(), and the config `type`).
const ChannelName = "discord"

// Mode is the Discord transport mode. Only ModeGateway exists: the Gateway
// WebSocket is the sole way to receive messages (ADR-0033); REST send is not a
// separate mode.
type Mode string

// ModeGateway is the Gateway WebSocket receive mode.
const ModeGateway Mode = "gateway"

// config carries the resolved options after every Option has run. It holds only the
// env-var NAME, never the token VALUE (ADR-0010).
type config struct {
	tokenEnv string
	mode     Mode
}

func defaultConfig() *config {
	return &config{mode: ModeGateway}
}

// Option configures the Adapter at construction time.
type Option func(*config)

// WithTokenEnv sets the NAME of the environment variable that holds the bot token
// (never the token value — ADR-0010). Required.
func WithTokenEnv(name string) Option { return func(c *config) { c.tokenEnv = name } }

// WithMode sets the transport mode. Only ModeGateway is supported.
func WithMode(m Mode) Option { return func(c *config) { c.mode = m } }
