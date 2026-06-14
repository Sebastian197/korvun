// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
)

// Mode selects the Telegram transport this adapter binds to. The
// zero value is invalid; New refuses to construct an Adapter without
// an explicit mode. See ADR-0008 §2.
type Mode int

// Recognised Modes. ModePolling is the default for callers that
// pass WithDefault* / DefaultOptions, because it works behind any
// NAT and has no infrastructure prerequisites.
const (
	ModePolling Mode = iota + 1
	ModeWebhook
)

// String returns a short lowercase name for the mode.
func (m Mode) String() string {
	switch m {
	case ModePolling:
		return "polling"
	case ModeWebhook:
		return "webhook"
	default:
		return "unknown"
	}
}

// Default knobs the adapter uses unless overridden by WithXxx options.
// Numbers match ADR-0008 §1 (capacity, enqueue timeout) and §4a
// (poll timeout, HTTP server header timeout). The webhook body cap
// is set in webhook.go.
const (
	// DefaultInboundCapacity is the size of the Envelope channel the
	// adapter exposes via Receive, and (in polling mode) the size of
	// the library's internal updates channel set via
	// bot.WithUpdatesChannelCap. The two are dimensioned together so
	// the single saturation seam in dispatchUpdate is the only drop
	// point.
	DefaultInboundCapacity = 64
	// DefaultEnqueueTimeout bounds how long dispatchUpdate or the
	// webhook handler will wait to push an Envelope onto the inbound
	// channel before dropping. Matches the router's
	// DefaultEnqueueTimeout (ADR-0003) so saturation behaviour reads
	// consistently across layers.
	DefaultEnqueueTimeout = 250 * time.Millisecond
	// DefaultWebhookPath is the URL path served by the adapter's HTTP
	// server when ModeWebhook is selected and WithWebhookPath is not
	// used. Includes the channel name so a future multi-channel mux
	// can colocate without collision.
	DefaultWebhookPath = "/telegram/webhook"
	// DefaultReadHeaderTimeout is the HTTP server header timeout used
	// in ModeWebhook unless overridden via WithReadHeaderTimeout. Set
	// to defuse Slowloris-style attacks against the webhook port.
	DefaultReadHeaderTimeout = 5 * time.Second
)

// config carries the resolved configuration after every Option has
// been applied. It is the input to validate() and to the
// constructor's wiring of the underlying *bot.Bot.
type config struct {
	token               string
	mode                Mode
	webhookURL          string
	listenAddr          string
	webhookPath         string
	secretToken         string
	allowedUpdates      []string
	inboundCapacity     int
	enqueueTimeout      time.Duration
	readHeaderTimeout   time.Duration
	dropPendingOnStart  bool
	reverseProxyTLS     bool
	tlsCertFile         string
	tlsKeyFile          string
	logger              *slog.Logger
	extraLibraryOptions []bot.Option
	injectedBotForTests botClient
}

// Option configures the Adapter at construction time. Options are
// applied left-to-right; later options override earlier ones for
// scalar fields and append for slice fields where noted.
type Option func(*config)

// WithToken sets the Telegram Bot API token. Required for both
// modes; an empty token is refused by New.
func WithToken(token string) Option {
	return func(c *config) { c.token = token }
}

// WithMode selects the transport mode. Required; the zero Mode is
// refused by New.
func WithMode(m Mode) Option {
	return func(c *config) { c.mode = m }
}

// WithWebhookURL sets the public HTTPS URL Telegram will POST to.
// Required when Mode is ModeWebhook; ignored otherwise.
func WithWebhookURL(url string) Option {
	return func(c *config) { c.webhookURL = url }
}

// WithListenAddr sets the address (host:port) the adapter's HTTP
// server binds to in ModeWebhook. Required for ModeWebhook unless
// the deployment terminates TLS at a reverse proxy that forwards on
// a different address.
func WithListenAddr(addr string) Option {
	return func(c *config) { c.listenAddr = addr }
}

// WithWebhookPath overrides the URL path the webhook handler is
// mounted at. Defaults to DefaultWebhookPath.
func WithWebhookPath(p string) Option {
	return func(c *config) { c.webhookPath = p }
}

// WithSecretToken sets the value the adapter validates against the
// X-Telegram-Bot-Api-Secret-Token header on incoming webhook
// requests. Required for ModeWebhook; refused if empty in webhook
// mode. The same value is sent to Telegram via SetWebhookParams.
func WithSecretToken(s string) Option {
	return func(c *config) { c.secretToken = s }
}

// WithAllowedUpdates restricts the kinds of updates Telegram sends.
// Empty slice means library default ("all except message_reaction
// and message_reaction_count" per Bot API). To receive reactions,
// callers should include "message" and "message_reaction"
// explicitly.
func WithAllowedUpdates(kinds []string) Option {
	return func(c *config) {
		c.allowedUpdates = append(c.allowedUpdates[:0:0], kinds...)
	}
}

// WithInboundCapacity overrides the size of the buffered Envelope
// channel. Defaults to DefaultInboundCapacity. Also drives
// WithUpdatesChannelCap on the underlying *bot.Bot in polling mode.
func WithInboundCapacity(n int) Option {
	return func(c *config) { c.inboundCapacity = n }
}

// WithEnqueueTimeout overrides the per-update enqueue timeout used
// by dispatchUpdate and the webhook handler. Defaults to
// DefaultEnqueueTimeout.
func WithEnqueueTimeout(d time.Duration) Option {
	return func(c *config) { c.enqueueTimeout = d }
}

// WithReadHeaderTimeout sets the HTTP server header timeout used in
// ModeWebhook. Defaults to DefaultReadHeaderTimeout.
func WithReadHeaderTimeout(d time.Duration) Option {
	return func(c *config) { c.readHeaderTimeout = d }
}

// WithDropPendingOnStart asks Telegram to drop any pending updates
// at SetWebhook (webhook mode) or at the initial DeleteWebhook
// safety-net call (polling mode). Off by default.
func WithDropPendingOnStart(drop bool) Option {
	return func(c *config) { c.dropPendingOnStart = drop }
}

// WithTLS configures the certificate and key files for
// ListenAndServeTLS. Required for direct TLS termination in
// ModeWebhook unless WithReverseProxyTermination is used. Ignored
// in polling mode.
func WithTLS(certFile, keyFile string) Option {
	return func(c *config) {
		c.tlsCertFile = certFile
		c.tlsKeyFile = keyFile
		c.reverseProxyTLS = false
	}
}

// WithReverseProxyTermination opts into running the webhook listener
// over plain HTTP, assuming TLS is terminated at a reverse proxy in
// front of Korvun. Mutually exclusive with WithTLS — the later
// option wins.
func WithReverseProxyTermination() Option {
	return func(c *config) {
		c.reverseProxyTLS = true
		c.tlsCertFile = ""
		c.tlsKeyFile = ""
	}
}

// WithLogger overrides the structured logger used for warnings (drop
// counters, rejected webhook requests, deferred lifecycle calls).
// Defaults to slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithLibraryOptions appends extra bot.Option values to the slice
// passed to bot.New. Use sparingly: anything covered by an explicit
// WithXxx here is preferred for clarity.
func WithLibraryOptions(opts ...bot.Option) Option {
	return func(c *config) {
		c.extraLibraryOptions = append(c.extraLibraryOptions, opts...)
	}
}

// withInjectedBotForTests replaces the *bot.Bot the adapter would
// otherwise instantiate via bot.New. Internal-only so production
// callers cannot bypass token verification by accident.
func withInjectedBotForTests(c botClient) Option {
	return func(cfg *config) { cfg.injectedBotForTests = c }
}

// defaultConfig returns the baseline config every New() starts from
// before applying caller-supplied Options.
func defaultConfig() *config {
	return &config{
		webhookPath:       DefaultWebhookPath,
		inboundCapacity:   DefaultInboundCapacity,
		enqueueTimeout:    DefaultEnqueueTimeout,
		readHeaderTimeout: DefaultReadHeaderTimeout,
		logger:            slog.Default(),
	}
}
