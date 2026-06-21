// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package app turns a validated config.Config into a wired, ready-to-run
// Korvun system (ADR-0017 §0). It is the testable boot layer that sits between
// internal/config (parse + validate) and cmd/korvun (the thin main): the
// catalog math, the secret resolution, the privacy selector, and the
// channel/router/brain wiring all live here, where tests can reach them —
// because func main cannot be unit-tested.
//
// The golden rule (ADR-0017 §5) is enforced at the boundary: configuration and
// boot errors are FATAL and name the offending field/var (Build returns an
// error); a provider being unreachable at runtime is NOT fatal — Ollama never
// connects at construction, so a downed Ollama still boots and the first
// message falls to the Brain fallback (ADR-0014 §3).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/channel/telegram"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/model/groq"
	"github.com/Sebastian197/korvun/internal/model/ollama"
	"github.com/Sebastian197/korvun/internal/model/sequential"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/router"
)

// DefaultPerModelTimeout bounds each provider call. It is applied to every
// coordinator and adapter the app builds.
const DefaultPerModelTimeout = 30 * time.Second

// Channel is a messaging channel with the ADR-0008 Start/Stop lifecycle the
// app drives, on top of the router-facing channel.Channel contract. The
// Telegram adapter satisfies it.
type Channel interface {
	channel.Channel
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// App is the wired Korvun system: a router with its brains and channels
// registered and routed, ready for Run.
type App struct {
	router   *router.Router
	channels []Channel
	logger   *slog.Logger
}

// builder holds resolved construction settings, including the channel factory
// seam tests override to exercise boot-error paths without a network.
type builder struct {
	logger          *slog.Logger
	perModelTimeout time.Duration
	newChannel      func(b *builder, cc config.ChannelConfig) (Channel, error)
}

// Option configures Build.
type Option func(*builder)

// WithLogger sets the structured logger used across the wired system. A nil
// logger is ignored (the default stays slog.Default()).
func WithLogger(l *slog.Logger) Option {
	return func(b *builder) {
		if l != nil {
			b.logger = l
		}
	}
}

// withChannelFactory overrides how channels are constructed. Internal-only: it
// lets tests inject a fake channel (and simulate an invalid-token boot failure)
// without a real Telegram round-trip, mirroring the telegram adapter's own
// test-injection discipline.
func withChannelFactory(f func(b *builder, cc config.ChannelConfig) (Channel, error)) Option {
	return func(b *builder) { b.newChannel = f }
}

// Build wires cfg into a ready App. Every failure is fatal and named
// (ADR-0017 §5). On any error after the router is created, the partially-built
// router is shut down so no worker goroutine is leaked.
func Build(cfg *config.Config, opts ...Option) (*App, error) {
	b := &builder{
		logger:          slog.Default(),
		perModelTimeout: DefaultPerModelTimeout,
		newChannel:      defaultChannelFactory,
	}
	for _, o := range opts {
		o(b)
	}

	r := router.New(router.WithErrorHandler(func(re router.RouterError) {
		b.logger.Error("router error",
			"kind", re.Kind.String(), "channel", re.Channel, "brain", re.Brain, "error", re.Err)
	}))

	channels, err := b.wire(r, cfg)
	if err != nil {
		// Clean up any brain/channel workers the partial wiring started, so a
		// failed Build leaks nothing.
		_ = r.Shutdown(context.Background())
		return nil, err
	}
	return &App{router: r, channels: channels, logger: b.logger}, nil
}

// wire registers brains, builds and registers channels, and binds routes.
func (b *builder) wire(r *router.Router, cfg *config.Config) ([]Channel, error) {
	for _, bc := range cfg.Brains {
		orch, err := b.buildBrain(bc)
		if err != nil {
			return nil, err
		}
		if err := r.RegisterBrain(bc.Name, orch); err != nil {
			return nil, fmt.Errorf("app: register brain %q: %w", bc.Name, err)
		}
	}

	channels := make([]Channel, 0, len(cfg.Channels))
	for _, cc := range cfg.Channels {
		ch, err := b.newChannel(b, cc)
		if err != nil {
			return nil, fmt.Errorf("app: build channel %q: %w", cc.Type, err)
		}
		if err := r.RegisterChannel(ch); err != nil {
			return nil, fmt.Errorf("app: register channel %q: %w", ch.Name(), err)
		}
		channels = append(channels, ch)
	}

	for _, rc := range cfg.Routes {
		if err := r.Route(rc.Channel, rc.Brain); err != nil {
			return nil, fmt.Errorf("app: route %q->%q: %w", rc.Channel, rc.Brain, err)
		}
	}
	return channels, nil
}

// buildBrain assembles one Orchestrator: catalog → privacy selector → policy →
// coordinator. The selector runs once here (ADR-0015), so a Private brain wired
// with only cloud models fails loudly at boot (ErrNoEligibleModels).
func (b *builder) buildBrain(bc config.BrainConfig) (*brain.Orchestrator, error) {
	catalog, err := b.buildCatalog(bc)
	if err != nil {
		return nil, err
	}
	sens, err := parseSensitivity(bc.Sensitivity)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	selected, err := policy.SelectModels(catalog, sens)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	pol, err := buildPolicy(bc.Policy)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	coord := buildCoordinator(bc.Dispatch, b.perModelTimeout)
	return brain.NewOrchestrator(coord, selected, pol, brain.WithLogger(b.logger)), nil
}

// buildCatalog constructs one CatalogEntry per model, tagging each with its
// DECLARED locality (ADR-0015 §3) and its per-provider model id (ADR-0014 §2).
func (b *builder) buildCatalog(bc config.BrainConfig) ([]policy.CatalogEntry, error) {
	entries := make([]policy.CatalogEntry, 0, len(bc.Models))
	for _, m := range bc.Models {
		adapter, err := b.buildModel(m)
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		loc, err := parseLocality(m.Locality)
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		b.logger.Info("model wired",
			"brain", bc.Name, "provider", m.Provider, "model_id", m.ModelID, "locality", m.Locality)
		entries = append(entries, policy.CatalogEntry{
			Model:    brain.WithModelID(adapter, m.ModelID),
			Locality: loc,
		})
	}
	return entries, nil
}

// buildModel constructs one provider adapter, resolving its secret from the
// environment by the configured env-var NAME (never from the file). Ollama
// never connects here (a downed Ollama is not a boot error); Groq requires its
// API key present at boot.
func (b *builder) buildModel(m config.ModelConfig) (model.Model, error) {
	switch m.Provider {
	case "ollama":
		opts := []ollama.Option{ollama.WithRequestTimeout(b.perModelTimeout)}
		if m.BaseURL != "" {
			opts = append(opts, ollama.WithBaseURL(m.BaseURL))
		}
		return ollama.New(opts...), nil
	case "groq":
		key := os.Getenv(m.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("%w: %q (groq API key for model %q)", ErrMissingSecret, m.APIKeyEnv, m.ModelID)
		}
		opts := []groq.Option{groq.WithAPIKey(key), groq.WithRequestTimeout(b.perModelTimeout)}
		if m.BaseURL != "" {
			opts = append(opts, groq.WithBaseURL(m.BaseURL))
		}
		g, err := groq.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("app: groq model %q: %w", m.ModelID, err)
		}
		return g, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, m.Provider)
	}
}

// defaultChannelFactory builds a real channel adapter. For Telegram it resolves
// the bot token from the env-var named by token_env, then constructs the
// adapter — telegram.New calls bot.New, which performs a getMe round-trip
// (verified against the go-telegram/bot docs), so an invalid token fails LOUDLY
// here at boot, closing the "silently deaf binary" gap (ADR-0017 §4).
func defaultChannelFactory(b *builder, cc config.ChannelConfig) (Channel, error) {
	switch cc.Type {
	case telegram.ChannelName:
		token := os.Getenv(cc.TokenEnv)
		if token == "" {
			return nil, fmt.Errorf("%w: %q (telegram bot token)", ErrMissingSecret, cc.TokenEnv)
		}
		ad, err := telegram.New(
			telegram.WithToken(token),
			telegram.WithMode(telegram.ModePolling),
			telegram.WithLogger(b.logger),
		)
		if err != nil {
			return nil, err
		}
		return ad, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannelType, cc.Type)
	}
}

// Run starts every channel (ADR-0008) and blocks until ctx is cancelled. If a
// channel fails to start, channels already started are stopped before the error
// is returned, so Run never leaves a half-started system behind.
func (a *App) Run(ctx context.Context) error {
	started := make([]Channel, 0, len(a.channels))
	for _, ch := range a.channels {
		if err := ch.Start(ctx); err != nil {
			a.stopChannels(context.Background(), started)
			return fmt.Errorf("app: start channel %q: %w", ch.Name(), err)
		}
		started = append(started, ch)
		a.logger.Info("channel started", "channel", ch.Name())
	}
	a.logger.Info("korvun is serving; send your bot a message")
	<-ctx.Done()
	return nil
}

// Shutdown stops the system in ADR-0008 order: every channel is stopped first
// (closing its inbound stream so the router's pump drains and exits), then the
// router is shut down (draining its brain and outbound workers). ctx bounds the
// whole operation. Errors are joined so one failing channel does not mask the
// rest.
func (a *App) Shutdown(ctx context.Context) error {
	var errs []error
	errs = append(errs, a.stopChannels(ctx, a.channels)...)
	if err := a.router.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("app: router shutdown: %w", err))
	}
	return errors.Join(errs...)
}

// stopChannels stops the given channels, collecting any errors.
func (a *App) stopChannels(ctx context.Context, channels []Channel) []error {
	var errs []error
	for _, ch := range channels {
		if err := ch.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("app: stop channel %q: %w", ch.Name(), err))
		}
	}
	return errs
}

// ---------- pure config → type mappers -------------------------------------

func parseSensitivity(s string) (policy.Sensitivity, error) {
	switch s {
	case "public":
		return policy.Public, nil
	case "private":
		return policy.Private, nil
	default:
		return 0, fmt.Errorf("%w: %q", policy.ErrUnknownSensitivity, s)
	}
}

func parseLocality(s string) (policy.Locality, error) {
	switch s {
	case "local":
		return policy.Local, nil
	case "cloud":
		return policy.Cloud, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrUnknownLocality, s)
	}
}

func buildPolicy(pc config.PolicyConfig) (policy.Policy, error) {
	switch pc.Kind {
	case "priority":
		return policy.PriorityReducer{Order: pc.Order}, nil
	case "consensus":
		return policy.ConsensusReducer{Order: pc.Order}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownPolicy, pc.Kind)
	}
}

// buildCoordinator selects the dispatch shape (ADR-0017 §3). An empty dispatch
// defaults to fan-out, the common case.
func buildCoordinator(dispatch string, perModelTimeout time.Duration) brain.Coordinator {
	switch dispatch {
	case "sequential":
		return sequential.New(sequential.WithPerModelTimeout(perModelTimeout))
	default: // "" or "fanout"
		return fanout.New(fanout.WithPerModelTimeout(perModelTimeout))
	}
}
