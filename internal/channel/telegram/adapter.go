// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// botClient is the subset of *bot.Bot the adapter calls into during
// Send and during the webhook lifecycle. Defined as an interface so
// tests can inject a fake without an HTTPS round-trip to Telegram.
// *bot.Bot satisfies this interface in production.
type botClient interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SendPhoto(ctx context.Context, params *bot.SendPhotoParams) (*models.Message, error)
	SendDocument(ctx context.Context, params *bot.SendDocumentParams) (*models.Message, error)
	SendVoice(ctx context.Context, params *bot.SendVoiceParams) (*models.Message, error)
	SendAudio(ctx context.Context, params *bot.SendAudioParams) (*models.Message, error)
	SendVideo(ctx context.Context, params *bot.SendVideoParams) (*models.Message, error)
	SendLocation(ctx context.Context, params *bot.SendLocationParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
	EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error)
	EditMessageCaption(ctx context.Context, params *bot.EditMessageCaptionParams) (*models.Message, error)
	DeleteMessage(ctx context.Context, params *bot.DeleteMessageParams) (bool, error)
	SetMessageReaction(ctx context.Context, params *bot.SetMessageReactionParams) (bool, error)
	SetWebhook(ctx context.Context, params *bot.SetWebhookParams) (bool, error)
	DeleteWebhook(ctx context.Context, params *bot.DeleteWebhookParams) (bool, error)
}

// botRunner is the polling-loop seam: *bot.Bot exposes Start(ctx)
// as the only public way to run the getUpdates loop in v1.21.0.
// Lifted behind an interface so tests can inject a controllable
// stand-in without instantiating a real *bot.Bot.
type botRunner interface {
	Start(ctx context.Context)
}

// Adapter is the Korvun channel adapter for Telegram. It satisfies
// the channel.Channel contract from internal/channel by exposing
// Name, Manifest, Send, and Receive. Lifecycle (Start, Stop) is
// adapter-owned; see ADR-0008 §1.
//
// An Adapter owns one buffered chan *envelope.Envelope (capacity
// from WithInboundCapacity). Inbound updates are written to it from
// one of two writers, depending on Mode: dispatchUpdate (polling)
// or webhookHandler (webhook). Send dispatches via OutboundParams
// against the configured botClient.
type Adapter struct {
	cfg *config

	client  botClient
	runner  botRunner
	inbound chan *envelope.Envelope
	dropped atomic.Uint64

	mu         sync.Mutex
	state      adapterState
	loopCtx    context.Context
	loopCancel context.CancelFunc
	httpServer *http.Server
	workers    sync.WaitGroup
	stopOnce   sync.Once
}

// adapterState tracks the lifecycle for ErrAlreadyStarted /
// ErrNotStarted enforcement. Transitions are strictly linear:
// stateNew -> stateRunning -> stateStopped.
type adapterState int

const (
	stateNew adapterState = iota
	stateRunning
	stateStopped
)

// New builds an Adapter from the supplied Options. It validates the
// resolved config before constructing the underlying *bot.Bot, so a
// misconfigured caller fails fast without a network round-trip.
//
// In tests, withInjectedBotForTests bypasses bot.New, so the adapter
// can be exercised without a real Telegram token or network.
func New(opts ...Option) (*Adapter, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if err := validate(cfg); err != nil {
		return nil, err
	}

	a := &Adapter{
		cfg:     cfg,
		inbound: make(chan *envelope.Envelope, cfg.inboundCapacity),
		state:   stateNew,
	}

	if cfg.injectedBotForTests != nil {
		a.client = cfg.injectedBotForTests
		if r, ok := cfg.injectedBotForTests.(botRunner); ok {
			a.runner = r
		}
		return a, nil
	}

	libOpts := []bot.Option{
		bot.WithUpdatesChannelCap(cfg.inboundCapacity),
	}
	if cfg.secretToken != "" {
		libOpts = append(libOpts, bot.WithWebhookSecretToken(cfg.secretToken))
	}
	if len(cfg.allowedUpdates) > 0 {
		libOpts = append(libOpts, bot.WithAllowedUpdates(append([]string(nil), cfg.allowedUpdates...)))
	}
	if cfg.mode == ModePolling {
		libOpts = append(libOpts, bot.WithDefaultHandler(a.handleLibraryUpdate))
	}
	libOpts = append(libOpts, cfg.extraLibraryOptions...)

	b, err := bot.New(cfg.token, libOpts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: bot.New: %w", err)
	}
	a.client = b
	a.runner = b
	return a, nil
}

// validate enforces the per-mode config invariants listed in
// ADR-0008 §1/§3. Errors are sentinel and matchable via errors.Is.
func validate(c *config) error {
	if c.token == "" {
		return ErrMissingToken
	}
	switch c.mode {
	case ModePolling, ModeWebhook:
	default:
		return ErrInvalidMode
	}
	if c.inboundCapacity <= 0 {
		return ErrInvalidInboundCapacity
	}
	if c.enqueueTimeout <= 0 {
		return ErrInvalidEnqueueTimeout
	}
	if c.mode == ModeWebhook {
		if c.webhookURL == "" {
			return ErrMissingWebhookURL
		}
		if c.listenAddr == "" {
			return ErrMissingListenAddr
		}
		if c.secretToken == "" {
			return ErrMissingSecretToken
		}
		if !c.reverseProxyTLS && (c.tlsCertFile == "" || c.tlsKeyFile == "") {
			return ErrMissingTLSConfig
		}
	}
	return nil
}

// Name returns the canonical channel name, "telegram".
func (a *Adapter) Name() string { return ChannelName }

// Manifest reports the content kinds this adapter supports across
// the inbound and outbound converters. Mirrors the capability set
// fixed by Phases 2.3 through 2E.7.
func (a *Adapter) Manifest() channel.Manifest {
	return channel.Manifest{
		Text:    true,
		Image:   true,
		Audio:   true,
		Video:   true,
		Buttons: true,
	}
}

// Receive returns the read-only Envelope channel the router consumes
// from. The same channel is returned on every call; the supplied ctx
// is not bound here (the router's ctx is not the adapter's lifecycle
// ctx — Start/Stop own that). See ADR-0008 §1.
func (a *Adapter) Receive(_ context.Context) (<-chan *envelope.Envelope, error) {
	return a.inbound, nil
}

// Mode reports the configured transport mode. Useful for callers
// (e.g. main.go) that branch on mode for bootstrap reporting.
func (a *Adapter) Mode() Mode { return a.cfg.mode }

// DroppedCount returns the cumulative number of inbound Envelopes
// that hit the saturation drop path. Surfaced for tests and for the
// observability hook (ADR-0008 §4c / §Open follow-ups).
func (a *Adapter) DroppedCount() uint64 { return a.dropped.Load() }

// handleLibraryUpdate adapts the library's bot.HandlerFunc shape to
// the adapter's dispatchUpdate(ctx, *models.Update) shape. Used as
// WithDefaultHandler in polling mode.
func (a *Adapter) handleLibraryUpdate(ctx context.Context, _ *bot.Bot, u *models.Update) {
	a.dispatchUpdate(ctx, u)
}

// dispatchUpdate runs the per-update conversion and enqueues the
// resulting Envelope onto the buffered inbound channel with the
// bounded backpressure rule fixed by ADR-0008 §1 / §4c. Defined as
// a method (not a closure) so tests can call it directly with
// fixture updates, no *bot.Bot needed.
//
// Three outcomes are possible:
//
//   - The update converts to an Envelope and enqueues within
//     enqueueTimeout: success, no log.
//   - The update has no Korvun-visible content (ErrNoMessage or
//     ErrUnsupportedContent from InboundFromUpdate): silently skip.
//     These are normal events (a service-message update, an
//     anonymous-admin reaction, etc.); logging every one would drown
//     real signal.
//   - Conversion succeeded but the inbound buffer is saturated past
//     enqueueTimeout: drop the Envelope, log a structured warning,
//     and increment dropped. The library worker that delivered this
//     update is released by the same return; see ADR-0008 §4c for
//     why this is the right operational shape rather than blocking
//     or pushing back to Telegram.
//
// ctx cancellation short-circuits both the convert and the enqueue
// without counting as a saturation drop — a cancelled ctx means the
// process is shutting down, which is not the same condition the
// observability layer wants to alert on.
func (a *Adapter) dispatchUpdate(ctx context.Context, u *models.Update) {
	if ctx.Err() != nil {
		return
	}
	env, err := InboundFromUpdate(u)
	if err != nil {
		if errors.Is(err, ErrNoMessage) || errors.Is(err, ErrUnsupportedContent) {
			return
		}
		a.cfg.logger.WarnContext(ctx,
			"telegram: failed to convert update",
			"error", err.Error())
		return
	}
	convID := env.Meta[MetaChatID]
	if convID != "" {
		env.Meta[router.MetaConversationID] = convID
	}
	timer := time.NewTimer(a.cfg.enqueueTimeout)
	defer timer.Stop()
	select {
	case a.inbound <- env:
	case <-ctx.Done():
	case <-timer.C:
		a.dropped.Add(1)
		a.cfg.logger.WarnContext(ctx,
			"telegram: dropped inbound envelope after enqueue timeout",
			"conversation_id", convID,
			"chat_id", env.Meta[MetaChatID],
			"timeout", a.cfg.enqueueTimeout.String(),
			"reason", "inbound_buffer_saturated")
	}
}
