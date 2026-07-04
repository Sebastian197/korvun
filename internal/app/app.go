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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/channel/telegram"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/conversation/sqlite"
	"github.com/Sebastian197/korvun/internal/httpserver"
	"github.com/Sebastian197/korvun/internal/liveview"
	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/Sebastian197/korvun/internal/metrics/prom"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/model/groq"
	"github.com/Sebastian197/korvun/internal/model/ollama"
	"github.com/Sebastian197/korvun/internal/model/sequential"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/router"
	"github.com/Sebastian197/korvun/internal/tool"
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
	// store is the durable conversation store's closer, owned so Shutdown can
	// close it LAST (after the router drains). nil when no storage is configured
	// (stateless). Held as io.Closer, set only from a non-nil concrete store, so
	// it is never a typed-nil interface (ADR-0019 §6).
	store io.Closer
	// adminServer is the observability HTTP server (/metrics + /healthz). nil
	// when observability is disabled. Started FIRST in Run and stopped LAST in
	// Shutdown so it stays observable across the whole drain (ADR-0020 §4).
	adminServer *httpserver.Server
	// metrics is the domain's observability backend: the Prometheus impl when
	// observability is on, metrics.Nop when off. Never nil.
	metrics metrics.Metrics
	// eventBus is the ADR-0023 in-process event bus, built ONLY when observability
	// is on (its only consumer, the SSE live-view, rides the admin server —
	// ADR-0024 §4). nil otherwise, which keeps the router's WithEventPublisher hook
	// dormant at zero cost (no producer without a consumer). Owned so Shutdown can
	// Close it LAST, once both its producers (the router) and consumers (the SSE
	// live-view) are quiescent.
	eventBus *bus.InMemoryBus
	// liveView serves the read-only SSE stream (/api/events) + embedded UI (/ui)
	// over eventBus (ADR-0024). nil when observability is off. Shutdown Closes it
	// before the admin server drains so the long-lived SSE connections release.
	liveView *liveview.LiveView
	// brainSummaries is the read-only control API's boot SNAPSHOT of the resolved
	// brains (ADR-0022 §3): assembled in wire() where the config is in hand, it
	// is the live truth for the process lifetime because brains are immutable at
	// runtime in this read-only cut. App serves it via BrainSummaries().
	brainSummaries []controlapi.BrainSummary
	// channelInfos carries each channel's static facts (type/mode/name) plus a
	// LIVE drop-count reader, so ChannelSummaries() reflects the current count at
	// request time (the count is the one non-static field).
	channelInfos []channelInfo
}

// channelInfo is App's per-channel record for the control API: the static
// wiring facts captured at wire() time, plus a live reader of the cumulative
// inbound-drop count (ok == false for a channel that has no counter).
type channelInfo struct {
	typ     string
	mode    string
	name    string
	dropped func() (uint64, bool)
}

// builder holds resolved construction settings, including the channel factory
// seam tests override to exercise boot-error paths without a network.
type builder struct {
	logger          *slog.Logger
	perModelTimeout time.Duration
	newChannel      func(b *builder, cc config.ChannelConfig) (Channel, error)
	// store is the shared conversation memory injected into every brain. A true
	// nil interface (no storage configured) leaves each Orchestrator stateless.
	store conversation.Store
	// metrics is the observability backend injected into the domain. Defaults to
	// metrics.Nop; set to the Prometheus impl when observability is enabled.
	metrics metrics.Metrics
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
		metrics:         metrics.Nop{},
	}
	for _, o := range opts {
		o(b)
	}

	// Resolve observability BEFORE the router, so the Prometheus backend exists
	// when the domain is wired (ADR-0020 §4). Absent block = on with loopback
	// defaults (the asymmetry with Storage, documented in config). When enabled,
	// the domain records through the Prometheus impl and the admin server serves
	// its /metrics; when disabled, the domain records through Nop and no server
	// is built.
	enabled, addr := cfg.ObservabilitySettings()
	var adminServer *httpserver.Server
	var pm *prom.Metrics
	if enabled {
		pm = prom.New()
		b.metrics = pm
		adminServer = httpserver.New(addr, b.logger)
		adminServer.Handle("/metrics", pm.Handler(b.logger))
	}

	// Build the event bus ONLY when the admin server exists: its only consumer —
	// the SSE live-view (ADR-0024) — rides that server, so with observability off
	// there is no subscriber and the router's WithEventPublisher hook stays dormant
	// at zero cost (the "no producer without a consumer" discipline, ADR-0023). A
	// nil eventBus below means no WithEventPublisher option and no app-level
	// failure publishing.
	var eventBus *bus.InMemoryBus
	if adminServer != nil {
		eventBus = bus.New()
	}

	// Open the durable store ONCE, before wiring the brains, and share it across
	// every brain (the Key namespaces by channel::conversation, ADR-0019 §6). A
	// configured store that fails to open is a fatal boot error (ADR-0017 §5).
	store, err := openStore(cfg)
	if err != nil {
		return nil, err
	}
	if store != nil {
		b.store = store // concrete non-nil -> real conversation.Store, never typed-nil
	}

	// The router gets the app-level error funnel (logs + counts +, when the bus is
	// live, publishes MessageDropped/HandleFailed) and — only when the bus exists —
	// the WithEventPublisher hook that wakes MessageReceived/ReplySent (ADR-0023).
	ropts := []router.Option{
		router.WithErrorHandler(func(re router.RouterError) {
			onRouterError(b.logger, b.metrics, eventBus, re)
		}),
	}
	if eventBus != nil {
		ropts = append(ropts, router.WithEventPublisher(eventBus))
	}
	r := router.New(ropts...)

	channels, brainSummaries, channelInfos, err := b.wire(r, cfg)
	if err != nil {
		// Clean up any brain/channel workers the partial wiring started, plus the
		// store we just opened, so a failed Build leaks nothing.
		_ = r.Shutdown(context.Background())
		if store != nil {
			_ = store.Close()
		}
		return nil, err
	}
	// Build the SSE live-view over the bus (ADR-0024) when both exist (i.e.
	// observability is on). It is the bus's first real subscriber.
	var liveView *liveview.LiveView
	if adminServer != nil && eventBus != nil {
		liveView = liveview.New(eventBus, liveview.WithLogger(b.logger))
	}

	// Register the pull dropped-count source for every channel that exposes one
	// (telegram), plus the bus and SSE drop counters. Done after wiring so the
	// adapters exist; only when the Prometheus backend is active (pm != nil).
	if pm != nil {
		registerDroppedSources(pm, channels, b.logger)
		if eventBus != nil {
			if err := pm.RegisterPullCounter("korvun_bus_events_dropped_total",
				"Events dropped because a bus subscriber's buffer was full.", eventBus.DroppedCount); err != nil {
				b.logger.Warn("observability: bus dropped-count source not registered", "error", err)
			}
		}
		if liveView != nil {
			if err := pm.RegisterPullCounter("korvun_sse_events_dropped_total",
				"Events dropped because an SSE client could not keep up.", liveView.DroppedCount); err != nil {
				b.logger.Warn("observability: SSE dropped-count source not registered", "error", err)
			}
		}
	}

	app := &App{
		router:         r,
		channels:       channels,
		logger:         b.logger,
		adminServer:    adminServer,
		metrics:        b.metrics,
		eventBus:       eventBus,
		liveView:       liveView,
		brainSummaries: brainSummaries,
		channelInfos:   channelInfos,
	}
	if store != nil {
		app.store = store // owned closer, set only from a non-nil concrete store
	}
	// Mount the read-only control API on the EXISTING admin server (ADR-0022 §1):
	// Handle runs here in Build, before Run starts the server. When observability
	// is disabled there is no admin server, so /api is simply not served — the
	// conscious coupling documented in ADR-0022 §5.
	if adminServer != nil {
		controlapi.Register(adminServer, app)
	}
	// Mount the live-view (SSE + UI) on the same admin server, also before Start.
	if liveView != nil {
		liveView.Register(adminServer)
	}
	return app, nil
}

// BrainSummaries implements controlapi.Reader: it returns a defensive copy of
// the boot snapshot (ADR-0022 §3) so a caller can never mutate App's state. The
// per-brain Models slice is copied too (the snapshot is shared otherwise).
func (a *App) BrainSummaries() []controlapi.BrainSummary {
	out := make([]controlapi.BrainSummary, len(a.brainSummaries))
	for i, bs := range a.brainSummaries {
		models := make([]controlapi.ModelSummary, len(bs.Models))
		copy(models, bs.Models)
		bs.Models = models
		out[i] = bs
	}
	return out
}

// ChannelSummaries implements controlapi.Reader: it reads each channel's LIVE
// drop count at call time (atomic, safe under concurrent requests — the same
// concurrency discipline the rest of the domain carries) and omits the count
// for a channel with no counter.
func (a *App) ChannelSummaries() []controlapi.ChannelSummary {
	out := make([]controlapi.ChannelSummary, 0, len(a.channelInfos))
	for _, ci := range a.channelInfos {
		cs := controlapi.ChannelSummary{Type: ci.typ, Mode: ci.mode, Name: ci.name}
		if n, ok := ci.dropped(); ok {
			dropped := n
			cs.Dropped = &dropped
		}
		out = append(out, cs)
	}
	return out
}

// droppedRegistrar registers a pull source for a channel's cumulative dropped
// count. *prom.Metrics satisfies it; kept as a narrow interface so the wiring is
// testable with a fake and so app does not hard-depend on the concrete type.
type droppedRegistrar interface {
	RegisterDroppedSource(channel string, count func() uint64) error
}

// droppedCounter is a channel that maintains a cumulative inbound-drop count
// (the telegram adapter). Other channels do not implement it and are skipped.
type droppedCounter interface {
	DroppedCount() uint64
}

// registerDroppedSources wires each channel's DroppedCount (when it has one) as
// a pull metric, so the drop count is read at scrape time rather than
// double-instrumented (ADR-0020 §3). A registration error (e.g. a duplicate
// channel name) is logged and skipped, never fatal: a metric must not take down
// boot (review F2).
func registerDroppedSources(reg droppedRegistrar, channels []Channel, logger *slog.Logger) {
	for _, ch := range channels {
		if dc, ok := ch.(droppedCounter); ok {
			if err := reg.RegisterDroppedSource(ch.Name(), dc.DroppedCount); err != nil {
				logger.Warn("observability: dropped-count source not registered",
					"channel", ch.Name(), "error", err)
			}
		}
	}
}

// onRouterError is the single sink the router's WithErrorHandler funnel feeds:
// it logs the failure (standardized fields), counts it by kind on the metrics
// backend (ADR-0020 §1, §3), and — when the bus is live — publishes the matching
// failure event (MessageDropped / HandleFailed) to it (ADR-0023: these two ride
// the existing app-level funnel, NOT an in-router hook, so the router is
// untouched for drops/failures). A nil eventBus (observability off) skips the
// publish at zero cost. Keeping all three off one funnel is the near-zero-blast-
// radius wiring the stage relies on.
func onRouterError(logger *slog.Logger, m metrics.Metrics, eventBus *bus.InMemoryBus, re router.RouterError) {
	logRouterError(logger, re)
	m.IncRouterError(re.Kind.String())
	if eventBus != nil {
		eventBus.Publish(context.Background(), routerErrorToEvent(re))
	}
}

// routerErrorToEvent maps a RouterError onto the bus Event it publishes
// (ADR-0023 §3): ErrKindHandle is a brain failure (HandleFailed); every other
// kind — inbound-dispatch saturation, outbound saturation, a failed Send — is a
// message that did not complete its path (MessageDropped). The Envelope/Channel/
// Brain/Err carry through; the SSE layer serializes only the non-secret subset
// (ADR-0024 §1), so passing Err here never leaks (it is dropped before the wire).
func routerErrorToEvent(re router.RouterError) bus.Event {
	t := bus.MessageDropped
	if re.Kind == router.ErrKindHandle {
		t = bus.HandleFailed
	}
	return bus.Event{
		Type:     t,
		Envelope: re.Envelope,
		Channel:  re.Channel,
		Brain:    re.Brain,
		Err:      re.Err,
	}
}

// logRouterError records one asynchronous router failure with the standardized
// observability funnel fields (ADR-0020 §1): kind, channel, brain, envelope_id,
// error. envelope_id is the empty string when the RouterError carries no
// envelope (some kinds do not). Extracted from the WithErrorHandler closure so
// the field vocabulary is testable in isolation.
func logRouterError(logger *slog.Logger, re router.RouterError) {
	envID := ""
	if re.Envelope != nil {
		envID = re.Envelope.ID
	}
	logger.Error("router error",
		"kind", re.Kind.String(),
		"channel", re.Channel,
		"brain", re.Brain,
		"envelope_id", envID,
		"error", re.Err)
}

// openStore opens the durable conversation store when storage is configured, or
// returns (nil, nil) for the stateless case (no storage block). An empty Path
// resolves to <os.UserConfigDir>/korvun/korvun.db. A configured-but-unopenable
// store returns a named error (the boot-fatal path, ADR-0019 §5).
func openStore(cfg *config.Config) (*sqlite.SqliteStore, error) {
	if cfg.Storage == nil {
		return nil, nil
	}
	path := cfg.Storage.Path
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("app: resolve default storage dir: %w", err)
		}
		path = filepath.Join(dir, "korvun", "korvun.db")
	}
	s, err := sqlite.Open(path)
	if err != nil {
		return nil, fmt.Errorf("app: open conversation store: %w", err)
	}
	return s, nil
}

// wire registers brains, builds and registers channels, and binds routes. It
// also assembles the read-only control API's boot snapshot (ADR-0022 §3): one
// BrainSummary per brain (resolved through the same selector rule the brain
// uses) and one channelInfo per channel (static facts + a live drop reader).
// These are additive — they neither change the wiring nor touch the router.
func (b *builder) wire(r *router.Router, cfg *config.Config) ([]Channel, []controlapi.BrainSummary, []channelInfo, error) {
	brainSummaries := make([]controlapi.BrainSummary, 0, len(cfg.Brains))
	for _, bc := range cfg.Brains {
		orch, err := b.buildBrain(bc)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := r.RegisterBrain(bc.Name, orch); err != nil {
			return nil, nil, nil, fmt.Errorf("app: register brain %q: %w", bc.Name, err)
		}
		bs, err := brainSummary(bc)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("app: brain summary %q: %w", bc.Name, err)
		}
		brainSummaries = append(brainSummaries, bs)
	}

	channels := make([]Channel, 0, len(cfg.Channels))
	channelInfos := make([]channelInfo, 0, len(cfg.Channels))
	for _, cc := range cfg.Channels {
		ch, err := b.newChannel(b, cc)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("app: build channel %q: %w", cc.Type, err)
		}
		if err := r.RegisterChannel(ch); err != nil {
			return nil, nil, nil, fmt.Errorf("app: register channel %q: %w", ch.Name(), err)
		}
		channels = append(channels, ch)
		channelInfos = append(channelInfos, newChannelInfo(cc, ch))
	}

	for _, rc := range cfg.Routes {
		if err := r.Route(rc.Channel, rc.Brain); err != nil {
			return nil, nil, nil, fmt.Errorf("app: route %q->%q: %w", rc.Channel, rc.Brain, err)
		}
	}
	return channels, brainSummaries, channelInfos, nil
}

// newChannelInfo captures one channel's static facts and binds a live reader of
// its cumulative drop count when the adapter exposes one (telegram), or a
// no-counter reader otherwise. The static facts come from the config (type,
// mode); the registered name from the adapter.
func newChannelInfo(cc config.ChannelConfig, ch Channel) channelInfo {
	ci := channelInfo{typ: cc.Type, mode: cc.Mode, name: ch.Name()}
	if dc, ok := ch.(droppedCounter); ok {
		ci.dropped = func() (uint64, bool) { return dc.DroppedCount(), true }
	} else {
		ci.dropped = func() (uint64, bool) { return 0, false }
	}
	return ci
}

// brainSummary builds the read-only control API summary for one brain. The
// surviving-model set is computed with the SAME rule as policy.SelectModels
// (ADR-0015: Public keeps all models, Private keeps Local only), sourced from
// the config so it needs no adapter construction and leaves buildBrain
// untouched. TestBrainSummary_matchesSelector cross-checks it against the real
// selector so the two can never silently diverge. The summary is secret-free:
// only provider + model id, never an env-var name (ADR-0022 §4).
func brainSummary(bc config.BrainConfig) (controlapi.BrainSummary, error) {
	sens, err := parseSensitivity(bc.Sensitivity)
	if err != nil {
		return controlapi.BrainSummary{}, err
	}
	models := make([]controlapi.ModelSummary, 0, len(bc.Models))
	for _, m := range bc.Models {
		loc, err := parseLocality(m.Locality)
		if err != nil {
			return controlapi.BrainSummary{}, err
		}
		if sens == policy.Public || loc == policy.Local {
			models = append(models, controlapi.ModelSummary{Provider: m.Provider, ModelID: m.ModelID})
		}
	}
	dispatch := bc.Dispatch
	if dispatch == "" {
		dispatch = "fanout" // buildCoordinator's default (ADR-0017 §3)
	}
	return controlapi.BrainSummary{
		Name:        bc.Name,
		Sensitivity: bc.Sensitivity,
		Policy:      bc.Policy.Kind,
		Dispatch:    dispatch,
		Models:      models,
	}, nil
}

// buildBrain assembles one brain.Brain: catalog → privacy selector → then either
// the default fan-out Orchestrator OR, when an agent block is present, a tool-use
// AgentBrain (ADR-0021). Both satisfy brain.Brain, so wire() registers either the
// same way and the router/cmd/korvun stay agnostic. The selector runs once here
// (ADR-0015), so a Private brain wired with only cloud models fails loudly at boot
// (ErrNoEligibleModels).
func (b *builder) buildBrain(bc config.BrainConfig) (brain.Brain, error) {
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
	if bc.Agent != nil {
		return b.buildAgentBrain(bc, selected)
	}
	pol, err := buildPolicy(bc.Policy)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	coord := buildCoordinator(bc.Dispatch, b.perModelTimeout)
	orchOpts := []brain.Option{brain.WithLogger(b.logger), brain.WithMetrics(b.metrics)}
	if b.store != nil {
		// Shared durable memory; recentTurns 0 => the Orchestrator default
		// (ADR-0019: config stays minimal, history depth is a Brain concern).
		orchOpts = append(orchOpts, brain.WithConversationStore(b.store, 0))
	}
	return brain.NewOrchestrator(coord, selected, pol, orchOpts...), nil
}

// buildAgentBrain assembles a single-model tool-use AgentBrain (ADR-0021). The
// agent is single-model (§1), so exactly one model must survive selection
// (ErrAgentModelCount otherwise). The tool registry is resolved from the
// configured names through tool.Builtin — the one place the safe-toolset boundary
// lives, so a dangerous name fails loudly at boot (ErrUnknownTool, §8). The shared
// durable store and metrics are injected like the Orchestrator's; only the FINAL
// user+assistant pair is persisted (§6).
func (b *builder) buildAgentBrain(bc config.BrainConfig, selected []model.Model) (brain.Brain, error) {
	if len(selected) != 1 {
		return nil, fmt.Errorf("%w: brain %q: got %d", ErrAgentModelCount, bc.Name, len(selected))
	}
	reg := make(tool.Registry, len(bc.Agent.Tools))
	for _, name := range bc.Agent.Tools {
		tl, ok := tool.Builtin(name)
		if !ok {
			return nil, fmt.Errorf("%w: %q (brain %q; available: %v)", ErrUnknownTool, name, bc.Name, tool.BuiltinNames())
		}
		reg[tl.Name()] = tl
	}
	opts := []brain.AgentOption{
		brain.WithAgentLogger(b.logger),
		brain.WithAgentMetrics(b.metrics),
		brain.WithAgentPerModelTimeout(b.perModelTimeout),
	}
	if bc.Agent.MaxIterations > 0 {
		opts = append(opts, brain.WithAgentMaxIterations(bc.Agent.MaxIterations))
	}
	if bc.Agent.SystemPrompt != "" {
		opts = append(opts, brain.WithAgentSystemPrompt(bc.Agent.SystemPrompt))
	}
	if b.store != nil {
		opts = append(opts, brain.WithAgentConversationStore(b.store, 0))
	}
	b.logger.Info("agent brain wired", "brain", bc.Name, "tools", bc.Agent.Tools, "max_iterations", bc.Agent.MaxIterations)
	return brain.NewAgentBrain(selected[0], reg, opts...), nil
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

// Run starts the app (Start) and then serves until ctx is cancelled (Serve). It is
// the composition the plain boot path uses; the supervisor (ADR-0027) instead calls
// Start and Serve separately so it can confirm a cutover succeeded (Start returned
// nil) before persisting the new config.
func (a *App) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	return a.Serve(ctx)
}

// Start brings the app up without blocking: it starts the admin server FIRST
// (ADR-0020 §4) so /healthz is live before any channel connects, then starts every
// channel (ADR-0008). If a channel fails to start, channels already started (and the
// admin server) are stopped before the error is returned, so a failed Start never
// leaves a half-started system behind. A successful Start is the supervisor's
// cutover-confirmation signal (ADR-0027): the fallible bind/channel-start steps — the
// ADR §c "admin re-bind" failure — have all completed, so the config is safe to
// persist.
func (a *App) Start(ctx context.Context) error {
	// Start the admin server FIRST (ADR-0020 §4): /healthz is up before any
	// channel connects, so an operator sees the process is alive during boot. A
	// bind failure is a loud boot error (the golden rule).
	if a.adminServer != nil {
		if err := a.adminServer.Start(ctx); err != nil {
			return fmt.Errorf("app: start admin server: %w", err)
		}
		a.logger.Info("admin server listening", "addr", a.adminServer.Addr())
	}

	started := make([]Channel, 0, len(a.channels))
	for _, ch := range a.channels {
		if err := ch.Start(ctx); err != nil {
			a.stopChannels(context.Background(), started)
			if a.adminServer != nil {
				_ = a.adminServer.Shutdown(context.Background())
			}
			return fmt.Errorf("app: start channel %q: %w", ch.Name(), err)
		}
		started = append(started, ch)
		a.logger.Info("channel started", "channel", ch.Name())
	}
	a.logger.Info("korvun is serving; send your bot a message")
	return nil
}

// Serve blocks until ctx is cancelled, then returns nil. All fallible startup
// happened in Start; Serve is the steady-state block.
func (a *App) Serve(ctx context.Context) error {
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
	routerErr := a.router.Shutdown(ctx)
	if routerErr != nil {
		errs = append(errs, fmt.Errorf("app: router shutdown: %w", routerErr))
	}
	// Close the store only once the router has FULLY drained (routerErr == nil).
	// Brain workers persist the final turn on a cancellation-detached context
	// (brain.persistTurns, so the last turn survives a graceful shutdown —
	// ADR-0019 §6), which means an AppendTurns can still be in flight after the
	// router context is cancelled. router.Shutdown returns nil only after every
	// brain worker has returned, so gating Close on that guarantees no AppendTurns
	// races into a closing DB. If router.Shutdown instead timed out on ctx, a
	// worker may still be mid-persist; leave the store open and let process exit
	// reclaim the handle (SQLite WAL is crash-consistent, so no corruption) rather
	// than race Close against the in-flight write.
	if a.store != nil {
		switch {
		case routerErr != nil:
			a.logger.Warn("conversation store left open: router did not drain within the shutdown deadline")
		default:
			if err := a.store.Close(); err != nil {
				errs = append(errs, fmt.Errorf("app: close conversation store: %w", err))
			}
		}
	}
	// Unblock the live-view BEFORE draining the admin server: SSE connections are
	// long-lived streaming requests that never finish on their own, so without
	// this signal adminServer.Shutdown would block on them until ctx expires.
	// Close returns each in-flight SSE serve loop promptly (it selects on this
	// done signal), so the admin server then drains immediately (ADR-0024 §1
	// clean-teardown).
	if a.liveView != nil {
		a.liveView.Close()
	}
	// Stop the admin server LAST among the network surfaces (ADR-0020 §4):
	// /metrics and /healthz stay observable across the whole drain above, then the
	// last network surface closes. Its error is joined like a channel's, never
	// masking the rest.
	if a.adminServer != nil {
		if err := a.adminServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("app: shutdown admin server: %w", err))
		}
	}
	// Close the bus VERY LAST. It is an observer that sits between producers and
	// consumers: its producers (the router, via WithEventPublisher + onRouterError)
	// quiesced at router.Shutdown above, and its consumers (the SSE subscribers)
	// are gone once the admin server has drained. Closing it here — after both are
	// quiet — tears down any residual subscriber goroutines with nothing left to
	// publish into it (ADR-0023 teardown). Idempotent and nil-safe.
	if a.eventBus != nil {
		a.eventBus.Close()
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
