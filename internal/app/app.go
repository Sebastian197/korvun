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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	builderui "github.com/Sebastian197/korvun/web/builder"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/channel/console"
	"github.com/Sebastian197/korvun/internal/channel/discord"
	"github.com/Sebastian197/korvun/internal/channel/telegram"
	"github.com/Sebastian197/korvun/internal/channel/webhook"
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
	"github.com/Sebastian197/korvun/internal/model/openaicompat"
	"github.com/Sebastian197/korvun/internal/model/retry"
	"github.com/Sebastian197/korvun/internal/model/sequential"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/router"
	"github.com/Sebastian197/korvun/internal/skill"
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
	// approvalsCfg / approvalTTL mirror the builder's Etapa-5 knob for
	// the package-internal test seam (recorderForTest).
	approvalsCfg *config.ApprovalsConfig
	approvalTTL  time.Duration
	// actions is the Action Kernel store's closer (Trust Layer Etapa 1,
	// lote 3b): opened at boot on the SAME storage file with its OWN
	// lifecycle (sealed decision 1), nil when stateless. Closed alongside
	// the conversation store.
	actions io.Closer
	// profileLock is the exclusive profile lock (R4 Phase 1, ADR-0045):
	// held for the server's whole life so a rotation cannot retire the
	// signing key under a live sealer; nil when stateless. The OS also
	// releases it on process death.
	profileLock *ProfileLock
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
	// warmupTargets are the deduplicated local models to warm at Start (ADR-0031
	// sub-phase 6). Empty when no model is marked warmup. warmupCancel/warmupDone
	// let Shutdown cancel an in-flight warmup and await its unwind.
	warmupTargets []warmupTarget
	warmupCancel  context.CancelFunc
	warmupDone    chan struct{}
	// modelHealth records each probed model's last observed liveness (N6);
	// BrainSummaries joins it into the per-model summaries at read time.
	modelHealth *modelHealthRegistry
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
	// cfg is the config being wired, kept so buildCatalog can resolve each
	// model's effective per-attempt timeout for the retry decorator (ADR-0031
	// sub-phase 4). Nil in direct buildBrain unit tests — buildCatalog then
	// falls back to the zero Config (DefaultRequestTimeout).
	cfg        *config.Config
	newChannel func(b *builder, cc config.ChannelConfig) (Channel, error)
	// store is the shared conversation memory injected into every brain. A true
	// nil interface (no storage configured) leaves each Orchestrator stateless.
	store conversation.Store
	// actions is the Action Kernel's store, shared into every AgentBrain
	// through the recorder adapter. nil when stateless (no storage block —
	// recording off, zero new config, the sealed decisions).
	actions *actionsqlite.Store
	// profileLock is the R4-F1 exclusive profile lock the boot acquires
	// before opening the action store; handed to the App on success.
	profileLock *ProfileLock
	// approvalsCfg and approvalTTL carry the Etapa-5 knob through the
	// wiring (the sacred pin: nil/off = the plain adapter, E3
	// byte-for-byte).
	approvalsCfg *config.ApprovalsConfig
	approvalTTL  time.Duration
	// metrics is the observability backend injected into the domain. Defaults to
	// metrics.Nop; set to the Prometheus impl when observability is enabled.
	metrics metrics.Metrics
	// reloader is the supervisor seam the mutation endpoint signals (ADR-0027).
	// When nil (no supervisor above the app), no mutation surface is mounted.
	reloader controlapi.Reloader
	// warmupTargets accumulates the deduplicated local models marked warmup as the
	// catalog is built (ADR-0031 sub-phase 6); warmupSeen is its dedup set keyed by
	// provider|baseURL|modelID.
	warmupTargets []warmupTarget
	warmupSeen    map[string]bool
	// toolBus is the ADR-0023 bus handed to agent brains as their tool-audit
	// sink (ADR-0041 §5). Kept as the concrete type (never a typed-nil
	// interface): nil when observability is off, in which case agent brains
	// audit through metrics only.
	toolBus *bus.InMemoryBus
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

// WithReloader injects the supervisor seam the config-mutation endpoint signals
// (ADR-0027). It is what enables the mutation surface: without it (or without a
// configured admin token) Build mounts only the read-only control API.
func WithReloader(r controlapi.Reloader) Option {
	return func(b *builder) { b.reloader = r }
}

// withChannelFactory overrides how channels are constructed. Internal-only: it
// lets tests inject a fake channel (and simulate an invalid-token boot failure)
// without a real Telegram round-trip, mirroring the telegram adapter's own
// test-injection discipline.
func withChannelFactory(f func(b *builder, cc config.ChannelConfig) (Channel, error)) Option {
	return func(b *builder) { b.newChannel = f }
}

// WithChannelFactory replaces the default channel construction (telegram /
// discord) with f — the withChannelFactory seam exported for embedders' tests:
// the desktop shell's lifecycle suite (internal/shell) boots a FULL real App
// hermetically, and the suite-wide no-network discipline (ADR-0034) forbids
// real channel dials from tests. A nil f keeps the default factory.
func WithChannelFactory(f func(cc config.ChannelConfig) (Channel, error)) Option {
	return func(b *builder) {
		if f != nil {
			b.newChannel = func(inner *builder, cc config.ChannelConfig) (Channel, error) {
				ch, err := f(cc)
				if ch == nil && err == nil {
					// (nil, nil) = "not mine, use the real factory": lets a
					// test harness fake the NETWORK channels while the
					// internal console channel stays real (it needs the
					// builder's store, which an external factory cannot
					// reach) — operator-console spec FR-CONS wiring.
					return defaultChannelFactory(inner, cc)
				}
				return ch, err
			}
		}
	}
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
	b.cfg = cfg

	// Derive the router's per-Handle ceiling from the brains' per-model timeouts
	// and dispatch shapes, or honor an explicit override that clears it (ADR-0031
	// Decision 2). Done before any resource is created so a too-low override fails
	// loud with nothing to unwind.
	ceiling, err := deriveRouterCeiling(cfg)
	if err != nil {
		return nil, err
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
	b.toolBus = eventBus // nil when observability is off (concrete, no typed-nil)

	// Open the durable store ONCE, before wiring the brains, and share it across
	// every brain (the Key namespaces by channel::conversation, ADR-0019 §6). A
	// configured store that fails to open is a fatal boot error (ADR-0017 §5).
	store, err := openStore(cfg)
	if err != nil {
		return nil, err
	}
	// lockHanded flips when the built App takes ownership of the profile
	// lock; until then any failed boot path releases it on the way out.
	lockHanded := false
	if store != nil {
		b.store = store // concrete non-nil -> real conversation.Store, never typed-nil
		// The exclusive profile lock (R4 Phase 1, ADR-0045): held for
		// the server's whole life, so a key rotation cannot retire the
		// signing key under this process's live sealer. A held lock
		// means another server (or a rotation) owns the profile —
		// boot-fatal, named.
		lock, err := AcquireProfileLock(filepath.Dir(storagePath(cfg)))
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("app: acquire profile lock: %w", err)
		}
		b.profileLock = lock
		defer func() {
			if !lockHanded {
				_ = lock.Release()
			}
		}()
		// The Action Kernel's store opens on the SAME file with its OWN
		// migrations and lifecycle (R3: the recovery pass runs BELOW,
		// after the keystore and sealer are wired — its closes are
		// terminals and every terminal is born with its receipt).
		// A storage-configured boot that cannot record actions is a fatal
		// boot error — proof is part of execution (blueprint §7.8).
		actions, err := actionsqlite.Open(storagePath(cfg))
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("app: open action store: %w", err)
		}
		b.actions = actions
		// The root intent auto-materializes here (Etapa 2, sealed
		// decision 1): deterministic, idempotent across boots, and
		// boot-fatal — a boot that cannot state the operator's standing
		// authority must not run.
		if err := ensureRootIntent(context.Background(), actions); err != nil {
			_ = actions.Close()
			_ = store.Close()
			return nil, err
		}
		// The ledger's ink (Etapa 4, FR-KEY-1): the signing key lives
		// beside the store, generated idempotently, permissions verified
		// on every boot — boot-fatal on any refusal.
		signingKey, err := ensureSigningKey(context.Background(), actions, filepath.Dir(storagePath(cfg)))
		if err != nil {
			_ = actions.Close()
			_ = store.Close()
			return nil, err
		}
		// The live ink (FR-LED): every terminal outcome is born signed
		// with the profile's active key, inside the outcome's own
		// transaction — since R3 the recovery closes included, so the
		// claim carries no silent exemption.
		actions.SetReceiptSealer(func(r action.Receipt) action.Receipt {
			return action.SignReceipt(signingKey, r)
		})
		// R3: the recovery pass runs AFTER the sealer is wired, so its
		// terminal closes land in the ledger signed like every other —
		// no terminal is born without its receipt. Boot-fatal on refusal.
		recSkipped, err := actions.RecoverPreviousLife(context.Background())
		if recSkipped > 0 {
			// R7-Y5: a postponed orphan is NEVER silent — named note.
			b.logger.Info("recovery: rows held by another connection were postponed", "skipped", recSkipped)
		}
		if err != nil {
			_ = actions.Close()
			_ = store.Close()
			return nil, fmt.Errorf("app: recover previous life: %w", err)
		}
		// R4: the boot also sweeps expired PENDING approvals (EXPIRED +
		// REJECTED action with its receipt) — server-owned expiry, so
		// an untouched request cannot outlive its window forever.
		swept, skipped, err := actions.SweepExpiredApprovals(context.Background(), time.Now().UTC())
		if skipped > 0 {
			// R4-F3: losing a clean race is a NOTE, never boot-fatal.
			b.logger.Info("expiry sweep: rows decided concurrently were skipped", "skipped", skipped, "swept", swept)
		}
		if err != nil {
			_ = actions.Close()
			_ = store.Close()
			return nil, fmt.Errorf("app: sweep expired approvals: %w", err)
		}
		// The Etapa-5 approvals knob rides the builder into the brain
		// wiring; a malformed TTL is boot-fatal (ADR-0017).
		ttl, err := approvalTTL(cfg.Approvals)
		if err != nil {
			_ = actions.Close()
			_ = store.Close()
			return nil, err
		}
		b.approvalsCfg = cfg.Approvals
		b.approvalTTL = ttl
	}

	// The router gets the app-level error funnel (logs + counts +, when the bus is
	// live, publishes MessageDropped/HandleFailed) and — only when the bus exists —
	// the WithEventPublisher hook that wakes MessageReceived/ReplySent (ADR-0023).
	ropts := []router.Option{
		router.WithErrorHandler(func(re router.RouterError) {
			onRouterError(b.logger, b.metrics, eventBus, re)
		}),
		// The DERIVED ceiling replaces router.DefaultBrainHandlerTimeout as the sole
		// per-Handle deadline (ADR-0031 Decision 2); it governs every Generate via
		// the Handle ctx, so removing the coordinator/adapter timeouts below leaves
		// no path without a deadline.
		router.WithBrainHandlerTimeout(ceiling),
		// Inbound dedup drops are a drop CLASS (audit R-1): counted like every
		// other drop. b.metrics is never nil (Nop default).
		router.WithDedupCounter(b.metrics.IncDeduped),
	}
	if eventBus != nil {
		ropts = append(ropts, router.WithEventPublisher(eventBus))
	}
	// Session dispatch (operator-console spec SP3): wired only when the
	// session block is configured — Validate guarantees storage exists with
	// it, and the sqlite store IS a SessionStore. Absent block: the router
	// behaves exactly as before (the spec's `none` default).
	if cfg.Session != nil && b.store != nil {
		if ss, ok := b.store.(conversation.SessionStore); ok {
			s := cfg.SessionSettings()
			ropts = append(ropts,
				router.WithSessionStore(ss),
				router.WithSessionPolicy(router.SessionPolicy{
					Triggers:  s.Triggers,
					Daily:     s.Daily,
					DailyHour: s.DailyHour,
					DailyMin:  s.DailyMin,
					IdleMin:   s.IdleMin,
					RecallMax: s.RecallMax,
				}))
		}
	}
	// The /notes commands (minimal-memory FR-RECALL-2) mount when any brain
	// has memory configured — the router only ever sees the app-composed
	// closures, never the memory config.
	if b.store != nil {
		if ns, ok := b.store.(conversation.NoteStore); ok {
			if list, clear, any := notesClosures(cfg, ns); any {
				ropts = append(ropts, router.WithNotesCommands(list, clear))
			}
		}
	}
	// The /tools gatekeeper command (ADR-0041 FR-CHAT-1) mounts on the
	// console channel when the bus exists to feed its activity ring —
	// without observability the command stays off rather than serving an
	// empty feed as if nothing ever ran.
	if eventBus != nil {
		ring := newToolEventRing(defaultToolRingSize, nil)
		ring.subscribe(eventBus)
		ropts = append(ropts, router.WithToolsCommand(console.ChannelName, toolsReporterFor(cfg, ring)))
	}
	// B9: the direct-brain conversation-id contract, enabled for the console
	// channel ONLY (spec FR-B9-1 privacy invariant — network channels never
	// honor the prefix).
	ropts = append(ropts, router.WithDirectBrainChannel(console.ChannelName))
	r := router.New(ropts...)

	channels, brainSummaries, channelInfos, err := b.wire(r, cfg)
	if err != nil {
		// Clean up any brain/channel workers the partial wiring started, plus the
		// store we just opened, so a failed Build leaks nothing. The event bus
		// closes too: the /tools ring subscribes BEFORE wiring, so its
		// subscriber goroutines must not outlive a failed boot.
		_ = r.Shutdown(context.Background())
		if eventBus != nil {
			eventBus.Close()
		}
		if store != nil {
			_ = store.Close()
		}
		if b.actions != nil {
			_ = b.actions.Close()
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
		registerReconnectSources(pm, channels, b.logger)
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
		warmupTargets:  b.warmupTargets,
		modelHealth:    newModelHealthRegistry(),
		approvalsCfg:   b.approvalsCfg,
		approvalTTL:    b.approvalTTL,
	}
	if store != nil {
		app.store = store // owned closer, set only from a non-nil concrete store
	}
	if b.actions != nil {
		app.actions = b.actions
	}
	if b.profileLock != nil {
		app.profileLock = b.profileLock
		lockHanded = true
	}
	// Mount the read-only control API on the EXISTING admin server (ADR-0022 §1):
	// Handle runs here in Build, before Run starts the server. When observability
	// is disabled there is no admin server, so /api is simply not served — the
	// conscious coupling documented in ADR-0022 §5.
	if adminServer != nil {
		controlapi.Register(adminServer, app)
		// Mount the WRITE surface ONLY when a supervisor seam is injected AND the
		// config names an admin token that resolves non-empty (ADR-0028 §1: no token
		// => mutation not mounted, the read-only default). Build re-resolves this on
		// every reload, so enabling/rotating the env-var name is itself a config edit.
		if b.reloader != nil && cfg.Admin != nil {
			if token := os.Getenv(cfg.Admin.TokenEnv); token != "" {
				controlapi.RegisterMutation(adminServer, b.reloader, token)
				// Mount the builder UI on the SAME token gate (ADR-0030 §4): a builder
				// whose Save would 404 is a trap, so with no token only the read-only
				// /ui is served. StripPrefix("/builder") maps GET /builder/ -> "/".
				adminServer.Handle("GET /builder/", http.StripPrefix("/builder", builderui.Handler()))
				// Mount the operator console (operator-console spec SP3) on the
				// SAME bearer: its responses carry message content, so it exists
				// only where the mutation surface exists AND a sessionful store
				// is open. The router is the operator seam (SP2).
				if ss, ok := b.store.(conversation.SessionStore); ok && b.store != nil {
					controlapi.RegisterConsole(adminServer, token, ss, app.router)
				}
			}
		}
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
// Each model's health is joined at READ time (N6): the identities are the
// immutable snapshot, the liveness is whatever the warmup has observed by now.
func (a *App) BrainSummaries() []controlapi.BrainSummary {
	out := make([]controlapi.BrainSummary, len(a.brainSummaries))
	for i, bs := range a.brainSummaries {
		models := make([]controlapi.ModelSummary, len(bs.Models))
		copy(models, bs.Models)
		for j := range models {
			st := a.modelHealth.get(models[j].Provider, models[j].ModelID)
			models[j].Health = st.health
			models[j].HealthDetail = st.detail
		}
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

// reconnectRegistrar is the metrics-backend surface for the reconnect pull counter.
// *prom.Metrics satisfies it; a narrow interface keeps the wiring testable.
type reconnectRegistrar interface {
	RegisterReconnectSource(channel string, count func() uint64) error
}

// reconnectCounter is a channel that maintains a cumulative Gateway reconnect count
// (the discord adapter). Other channels do not implement it and are skipped.
type reconnectCounter interface {
	ReconnectCount() uint64
}

// registerReconnectSources wires each channel's ReconnectCount (when it has one) as a
// pull metric (korvun_channel_reconnects_total{channel}), the same scrape-time,
// no-double-instrument pattern as registerDroppedSources. A registration error is
// logged and skipped, never fatal (review F2).
func registerReconnectSources(reg reconnectRegistrar, channels []Channel, logger *slog.Logger) {
	for _, ch := range channels {
		if rc, ok := ch.(reconnectCounter); ok {
			if err := reg.RegisterReconnectSource(ch.Name(), rc.ReconnectCount); err != nil {
				logger.Warn("observability: reconnect-count source not registered",
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
	s, err := sqlite.Open(storagePath(cfg))
	if err != nil {
		return nil, fmt.Errorf("app: open conversation store: %w", err)
	}
	return s, nil
}

// storagePath resolves the configured storage path, defaulting to
// <os.UserConfigDir>/korvun/korvun.db — ONE resolution shared by the
// conversation store and the Action Kernel store so "the same file" is
// true by construction. An unresolvable home falls back to the relative
// default the sqlite mold will surface loudly.
func storagePath(cfg *config.Config) string {
	if cfg.Storage != nil && cfg.Storage.Path != "" {
		return cfg.Storage.Path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join("korvun", "korvun.db")
	}
	return filepath.Join(dir, "korvun", "korvun.db")
}

// actionRecorder adapts the kernel's sqlite store to the brain's
// consumer-side ActionRecorder seam.
type actionRecorder struct {
	store *actionsqlite.Store
	// pin is the law this adapter stamps on every decision (FR-POL-1):
	// computed per brain at config load, immutable for the adapter's life.
	pin actionsqlite.PolicyPin
}

// RecordAttempt implements brain.ActionRecorder. The adapter stamps the
// law pin on every decision (FR-POL-1).
func (r actionRecorder) RecordAttempt(ctx context.Context, env action.Envelope, outcome, rule string, state action.State) error {
	return r.store.RecordAttempt(ctx, env, actionsqlite.Decision{
		Outcome: outcome, Rule: rule,
		PolicyVersion: r.pin.Version, PolicyDigest: r.pin.Digest,
	}, state)
}

// Finish implements brain.ActionRecorder.
func (r actionRecorder) Finish(ctx context.Context, actionID string, to action.State, finishedAt time.Time) error {
	return r.store.Finish(ctx, actionID, to, finishedAt)
}

// FinishWithResult implements the brain's optional result-recorder
// extension (Etapa 4, FR-LED): the terminal close carries the on-the-fly
// result digest onto the receipt — never the raw result (NC-3).
func (r actionRecorder) FinishWithResult(ctx context.Context, actionID string, to action.State, finishedAt time.Time, resultDigest string) error {
	return r.store.FinishWithResult(ctx, actionID, to, finishedAt, resultDigest)
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
	// The console channel auto-routes to the FIRST brain when the config
	// names no explicit route (operator-console spec FR-CONS-2's
	// default-brain default): a declared direct chat must never boot inert.
	if hasChannelType(cfg, console.ChannelName) && len(cfg.Brains) > 0 {
		routed := false
		for _, rc := range cfg.Routes {
			if rc.Channel == console.ChannelName {
				routed = true
				break
			}
		}
		if !routed {
			if err := r.Route(console.ChannelName, cfg.Brains[0].Name); err != nil {
				return nil, nil, nil, fmt.Errorf("app: console default route: %w", err)
			}
		}
	}
	return channels, brainSummaries, channelInfos, nil
}

// hasChannelType reports whether cfg declares a channel of the given type.
func hasChannelType(cfg *config.Config, t string) bool {
	for _, ch := range cfg.Channels {
		if ch.Type == t {
			return true
		}
	}
	return false
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
	// Persona (builder-canvas spec FR-PERSONA-2, NC-4): composed ONCE here, then
	// applied only when non-empty — a nil persona or an all-empty block adds
	// ZERO options, keeping today's requests byte-for-byte intact.
	personaPrompt := composePersonaPrompt(bc.Persona)
	if bc.Agent != nil {
		return b.buildAgentBrain(bc, selected, catalog, sens, personaPrompt)
	}
	pol, err := buildPolicy(bc.Policy)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	coord := buildCoordinator(bc.Dispatch, bc.Policy.Kind)
	orchOpts := []brain.Option{brain.WithLogger(b.logger), brain.WithMetrics(b.metrics)}
	if personaPrompt != "" {
		// The previously-unwired path: the persona reaches the model as the
		// request's system message via WithSystemPrompt (AS-PERSONA-2).
		orchOpts = append(orchOpts, brain.WithSystemPrompt(personaPrompt))
	}
	if b.store != nil {
		// Shared durable memory; recentTurns 0 => the Orchestrator default
		// (ADR-0019: config stays minimal, history depth is a Brain concern).
		orchOpts = append(orchOpts, brain.WithConversationStore(b.store, 0))
	}
	return brain.NewOrchestrator(coord, selected, pol, orchOpts...), nil
}

// composePersonaPrompt maps the config persona block onto the brain package's
// Persona and composes it (brain.ComposePersona). The mapping lives HERE so
// internal/brain never imports internal/config (the same seam direction as the
// rest of the factory). nil in → "" out.
func composePersonaPrompt(p *config.PersonaConfig) string {
	if p == nil {
		return ""
	}
	return brain.ComposePersona(&brain.Persona{
		DisplayName:  p.DisplayName,
		Tone:         p.Tone,
		Language:     p.Language,
		Instructions: p.Instructions,
	})
}

// buildAgentBrain assembles a single-model tool-use AgentBrain (ADR-0021). The
// agent is single-model (§1), so exactly one model must survive selection
// (ErrAgentModelCount otherwise). The tool registry is resolved from the
// configured names through tool.Builtin — the one place the safe-toolset boundary
// lives, so a dangerous name fails loudly at boot (ErrUnknownTool, §8). The shared
// durable store and metrics are injected like the Orchestrator's; only the FINAL
// user+assistant pair is persisted (§6).
func (b *builder) buildAgentBrain(bc config.BrainConfig, selected []model.Model, catalog []policy.CatalogEntry, sens policy.Sensitivity, personaPrompt string) (brain.Brain, error) {
	if len(selected) != 1 {
		return nil, fmt.Errorf("%w: brain %q: got %d", ErrAgentModelCount, bc.Name, len(selected))
	}
	// R4-F5: the effective cage resolves ONCE — attrs, bounds, sorted
	// allow-lists — and BOTH the tool constructions below and the law
	// pin serialize from this same object (one resolver, one verdict).
	cage, err := ResolveEffectiveCage(bc)
	if err != nil {
		return nil, err
	}
	attrs := cage.Attrs
	listed := make(map[string]bool, len(cage.Tools))
	for _, name := range cage.Tools {
		listed[name] = true
	}
	for _, g := range cage.Governance {
		if !listed[g.Tool] {
			return nil, fmt.Errorf("app: brain %q: governance grants tool %q which is not in agent.tools", bc.Name, g.Tool)
		}
	}
	// The sensitive×locality rule is enforced by the gate — which only
	// mounts WITH governance. Ungoverned, the rule would silently not exist
	// (estreno E-11): a Sensitive tool feeding a Cloud model is exactly the
	// egress the attribute declares against, so that combination fails loud
	// at boot instead.
	if len(cage.Governance) == 0 {
		loc, err := localityOf(catalog, selected[0])
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		if loc == policy.Cloud {
			for _, name := range cage.Tools {
				if attrs[name].Sensitive {
					return nil, fmt.Errorf("%w: brain %q, tool %q (add a governance block, or override the tool's sensitive attr consciously)",
						ErrSensitiveToolUngoverned, bc.Name, name)
				}
			}
		}
	}
	// Minimal-memory boot guards (ADR-0043 §4/§6). P2: memory_note without a
	// governance grant COVERING IT fails loud (the E-11 molde) — D1 is never
	// vacuously ungoverned.
	if listed["memory_note"] {
		covered := false
		for _, g := range cage.Governance {
			if g.Tool == "memory_note" {
				covered = true
			}
		}
		if !covered {
			return nil, fmt.Errorf("%w: brain %q, tool \"memory_note\" (P2 — add a governance grant covering it)",
				ErrMemoryToolUngoverned, bc.Name)
		}
	}
	// FR-PRIV-1: brain-global scope requires the SELECTED model to be Local
	// (the localityOf precedent — not the raw catalog): cross-conversation
	// content must never ride to a cloud provider.
	if cage.Memory != nil && cage.Memory.BrainGlobal {
		loc, err := localityOf(catalog, selected[0])
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		if loc == policy.Cloud {
			return nil, fmt.Errorf("%w: brain %q: agent.memory.scope \"brain\" with a cloud-selected model",
				ErrMemoryScopeCloud, bc.Name)
		}
	}
	reg := make(tool.Registry, len(cage.Tools))
	for _, name := range cage.Tools {
		tl, err := b.agentTool(cage, name)
		if err != nil {
			return nil, err
		}
		reg[tl.Name()] = tl
	}
	// Effect-registry preflight (Etapa 3, FR-REG-3): every operation the
	// brain can reach must classify from the declared registry — an
	// undeclared tool fails the boot loudly, never silently unclassified.
	if err := validateToolEffects(reg); err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	// No WithAgentPerModelTimeout: since ADR-0031 sub-phase 4 the retry decorator
	// (wired in buildCatalog) owns the per-attempt deadline for the agent's model
	// calls too — a single owner (SV3 final state). The agent's per-model call
	// therefore runs unbounded by the agent itself and inherits the decorated
	// model's per-attempt window (EffectiveRequestTimeout).
	opts := []brain.AgentOption{
		brain.WithAgentLogger(b.logger),
		brain.WithAgentMetrics(b.metrics),
		// ALWAYS set — scope-aware tools need the brain's own name even
		// with observability off (minimal-memory FR-TOOL-2; the audit
		// option's brainName only mounts with a bus).
		brain.WithAgentName(bc.Name),
	}
	if cage.Memory != nil {
		if ns, ok := b.store.(conversation.NoteStore); ok {
			mem := *cage.Memory
			scopeCfg := noteScopeOf(mem)
			brainName := bc.Name
			loader := func(ctx context.Context, key conversation.Key) ([]conversation.Note, error) {
				scope, ekey, err := conversation.EffectiveNoteScope(scopeCfg, key)
				if err != nil {
					// No conversation identity on a conversation-scoped
					// brain: nothing to load — not an error (the write path
					// refuses loudly; the read path simply has no notes).
					return nil, nil
				}
				return ns.ListNotes(ctx, brainName, scope, ekey)
			}
			opts = append(opts, brain.WithAgentMemory(loader, mem.BudgetRunes))
		}
	}
	if cage.MaxIterations > 0 {
		opts = append(opts, brain.WithAgentMaxIterations(cage.MaxIterations))
	}
	if cage.SystemPrompt != "" {
		opts = append(opts, brain.WithAgentSystemPrompt(cage.SystemPrompt))
	}
	if personaPrompt != "" {
		// Persona as a PREFIX before the intact ADR-0021 protocol block
		// (FR-PERSONA-2, AS-PERSONA-3); empty adds nothing.
		opts = append(opts, brain.WithAgentPersona(personaPrompt))
	}
	if b.store != nil {
		opts = append(opts, brain.WithAgentConversationStore(b.store, 0))
	}
	if b.toolBus != nil {
		// Tool-use audit events ride the same bus the lifecycle events do
		// (ADR-0041 §5); when observability is off, metrics remain the only
		// (Nop) audit surface and nothing publishes.
		opts = append(opts, brain.WithAgentToolAudit(b.toolBus, bc.Name))
	}
	if len(cage.Governance) > 0 {
		grants, err := toolGrants(cage.Governance)
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		loc, err := localityOf(catalog, selected[0])
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		opts = append(opts, brain.WithAgentGovernance(&brain.AgentGovernance{
			Grants:      grants,
			Attrs:       attrs,
			Sensitivity: sens,
			Locality:    loc,
		}))
	}
	if cage.SkillsDir != "" {
		skills, err := skill.LoadDir(cage.SkillsDir, b.logger)
		if err != nil {
			// A configured skills dir that cannot be read is a config error
			// (fail loud at boot); a malformed skill INSIDE it degrades with
			// warnings inside LoadDir (AS-5).
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		block, omitted := skill.PromptBlock(skills, cage.SkillsBodyBudget)
		if len(omitted) > 0 {
			b.logger.Warn("agent skills: bodies omitted over the budget",
				"brain", bc.Name, "omitted", omitted, "budget", cage.SkillsBodyBudget)
		}
		if block != "" {
			opts = append(opts, brain.WithAgentSkillsBlock(block))
		}
	}
	// The Action Kernel's recorder rides into every agent brain when the
	// store exists (lote 3b): every tool attempt lands with its decision.
	// The identity context rides with it (Etapa 2, lote 4): the config
	// provenance registry, the root intent, and this brain's derived
	// grant — the recorded EXPLANATION of its governed allows.
	if b.actions != nil {
		// The law pin (FR-POL-1, C1-stable): the digest of the effective
		// cage-governing content, stamped by the adapter on every
		// decision — the SAME law across reboots of the same config.
		pin, err := policyPinFromCage(cage)
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
		}
		opts = append(opts, brain.WithActionRecorder(newBrainRecorder(b.actions, pin, b.approvalsCfg, b.approvalTTL)))
		// The effect engine (Etapa 3): the brain classifies every attempt
		// from the DECLARED registry — the same single safe-toolset
		// boundary the preflight above already validated.
		opts = append(opts, brain.WithEffectClassifier(tool.BuiltinEffects))
		identityCfg := b.cfg
		if identityCfg == nil {
			identityCfg = &config.Config{} // direct unit-test builds: console-only registry
		}
		identity := brain.ActionIdentity{
			Registry: provenanceRegistry(identityCfg),
			IntentID: action.RootIntentID,
		}
		if grant, ok := derivedConfigGrant(bc); ok {
			identity.GrantID = grant.GrantID
		}
		// The missing cable (Etapa 5, adjudicated 2026-09-01): the
		// per-brain config ceiling lands in the PRODUCTION identity —
		// without it the chat path could never park an action, because
		// config-derived authority is ceilingless by sealed E3 design
		// and the gate demands approval only under BOUNDED authority.
		if cage.HasAgent && cage.EffectCeiling != "" {
			ceiling := action.EffectClass(cage.EffectCeiling)
			if !ceiling.OnLadder() {
				return nil, fmt.Errorf("app: brain %q: agent.effect_ceiling %q is not on the ladder (valid: pure, read_external, write_reversible, write_compensatable, write_irreversible, critical)", bc.Name, cage.EffectCeiling)
			}
			identity.EffectCeiling = ceiling
		}
		opts = append(opts, brain.WithActionIdentity(identity))
	}
	b.logger.Info("agent brain wired", "brain", bc.Name, "tools", cage.Tools, "max_iterations", cage.MaxIterations)
	return brain.NewAgentBrain(selected[0], reg, opts...), nil
}

// agentTool resolves one configured tool name FROM the typed effective
// cage (R4-F5: one resolver, one verdict — this function cannot re-read
// raw BrainConfig): the pure set through tool.Builtin, the caged set
// through its typed constructor WITH the resolved cage (ADR-0041 §4) —
// a caged tool listed without its cage fails loud (ErrMissingToolCage).
// The network shield is armed here from the RESOLVED attrs: a
// network-classed tool on a Private brain dials through the
// private-address check.
func (b *builder) agentTool(cage *EffectiveCage, name string) (tool.Tool, error) {
	if t, ok := tool.Builtin(name); ok {
		return t, nil
	}
	brainName := cage.BrainName
	shield := cage.Attrs[name].Network && cage.Sensitivity == policy.Private
	switch name {
	case "memory_note":
		// The governed notes writer (minimal-memory FR-TOOL-1, ADR-0043 §4):
		// app-constructed over the SINGLE derivation + the shared NoteStore,
		// so the write path can never drift from the read path (H2).
		if cage.Memory == nil {
			return nil, fmt.Errorf("%w: %q (brain %q): add the agent.memory block", ErrMissingToolCage, name, brainName)
		}
		ns, ok := b.store.(conversation.NoteStore)
		if !ok {
			return nil, fmt.Errorf("app: brain %q: memory_note requires the storage block (no note store available)", brainName)
		}
		mem := *cage.Memory
		scopeCfg := noteScopeOf(mem)
		writer := func(ctx context.Context, sc tool.Scope, note string) error {
			scope, key, err := conversation.EffectiveNoteScope(scopeCfg, conversation.Key(sc.Conversation))
			if err != nil {
				return fmt.Errorf("%w: %v", tool.ErrNoteNeedsConversation, err)
			}
			if _, err := ns.AppendNote(ctx, sc.Brain, scope, key, note, mem.MaxNotes); err != nil {
				if errors.Is(err, conversation.ErrNotesFull) {
					return fmt.Errorf("%w: %v", tool.ErrNoteBoxFull, err)
				}
				return err
			}
			return nil
		}
		return tool.NewMemoryNote(writer, mem.MaxNoteRunes), nil
	case "read_file":
		if cage.ReadFile == nil {
			return nil, fmt.Errorf("%w: %q (brain %q): add the agent.read_file block", ErrMissingToolCage, name, brainName)
		}
		t, err := tool.ReadFile(tool.ReadFileConfig{Root: cage.ReadFile.Root, MaxBytes: cage.ReadFile.MaxBytes})
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", brainName, err)
		}
		return t, nil
	case "http_fetch":
		if cage.HTTPFetch == nil {
			return nil, fmt.Errorf("%w: %q (brain %q): add the agent.http_fetch block", ErrMissingToolCage, name, brainName)
		}
		t, err := tool.HTTPFetch(tool.HTTPFetchConfig{
			AllowHosts:   cage.HTTPFetch.AllowHosts,
			MaxBytes:     cage.HTTPFetch.MaxBytes,
			MaxRedirects: cage.HTTPFetch.MaxRedirects,
			PrivateOnly:  shield,
		})
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", brainName, err)
		}
		return t, nil
	case "webhook_call":
		if cage.WebhookCall == nil {
			return nil, fmt.Errorf("%w: %q (brain %q): add the agent.webhook_call block", ErrMissingToolCage, name, brainName)
		}
		t, err := tool.WebhookCall(tool.WebhookCallConfig{
			AllowHosts:  cage.WebhookCall.AllowHosts,
			MaxBytes:    cage.WebhookCall.MaxBytes,
			Timeout:     time.Duration(cage.WebhookCall.TimeoutSeconds) * time.Second,
			PrivateOnly: shield,
		})
		if err != nil {
			return nil, fmt.Errorf("app: brain %q: %w", brainName, err)
		}
		return t, nil
	default:
		return nil, fmt.Errorf("%w: %q (brain %q; pure: %v, caged: read_file, http_fetch, webhook_call)",
			ErrUnknownTool, name, brainName, tool.BuiltinNames())
	}
}

// effectiveToolAttrs merges the house defaults (tool.BuiltinAttrs) with the
// declared per-field operator overrides (R-2) for every LISTED tool. An
// override naming an unlisted tool fails loud.
func effectiveToolAttrs(a *config.AgentConfig) (map[string]policy.ToolAttrs, error) {
	attrs := make(map[string]policy.ToolAttrs, len(a.Tools))
	listed := make(map[string]bool, len(a.Tools))
	for _, name := range a.Tools {
		listed[name] = true
		if base, ok := tool.BuiltinAttrs(name); ok {
			attrs[name] = policy.ToolAttrs{Sensitive: base.Sensitive, Network: base.Network}
		}
		// An unknown name gets no attrs entry; agentTool fails loud on it.
	}
	for name, o := range a.ToolAttrs {
		if !listed[name] {
			return nil, fmt.Errorf("app: tool_attrs overrides %q which is not in agent.tools", name)
		}
		cur := attrs[name]
		if o.Sensitive != nil {
			cur.Sensitive = *o.Sensitive
		}
		if o.Network != nil {
			cur.Network = *o.Network
		}
		attrs[name] = cur
	}
	return attrs, nil
}

// toolGrants converts the config grants into the policy package's declared
// form; modes were validated structurally in config, so an unknown one here
// is a programming error surfaced loudly.
func toolGrants(gs []config.ToolGrantConfig) ([]policy.ToolGrant, error) {
	out := make([]policy.ToolGrant, 0, len(gs))
	for _, g := range gs {
		var mode policy.ToolMode
		switch g.Mode {
		case "allow":
			mode = policy.ToolAllow
		case "shadow":
			mode = policy.ToolShadow
		case "deny":
			mode = policy.ToolDeny
		default:
			return nil, fmt.Errorf("unknown grant mode %q for tool %q", g.Mode, g.Tool)
		}
		out = append(out, policy.ToolGrant{Name: g.Tool, Mode: mode, Channels: g.Channels})
	}
	return out, nil
}

// localityOf finds the catalog locality of the selected model. SelectModels
// only returns catalog entries, so a miss is a mechanism bug, not a config
// error — surfaced loudly rather than silently defaulting.
func localityOf(catalog []policy.CatalogEntry, m model.Model) (policy.Locality, error) {
	for _, e := range catalog {
		if e.Model == m {
			return e.Locality, nil
		}
	}
	return 0, fmt.Errorf("selected model not found in the catalog")
}

// buildCatalog constructs one CatalogEntry per model, tagging each with its
// DECLARED locality (ADR-0015 §3) and its per-provider model id (ADR-0014 §2).
// Each adapter is wrapped in the per-instance retry decorator (ADR-0031
// sub-phase 4) BEFORE WithModelID, so the decorated model owns the per-attempt
// deadline for every dispatch shape (including the agent, which consumes
// selected[0] from this catalog) and retries transient errors.
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
		decorated := retry.New(adapter, retry.Config{
			PerAttempt: b.effectiveConfig().EffectiveRequestTimeout(m),
			MaxRetries: effectiveMaxRetries(bc, m),
		}, retry.WithMetrics(b.metrics))
		withID := brain.WithModelID(decorated, m.ModelID)
		entries = append(entries, policy.CatalogEntry{
			Model:    withID,
			Locality: loc,
		})
		if m.Warmup {
			b.collectWarmup(m, withID)
		}
	}
	return entries, nil
}

// collectWarmup records a warmup target for a local model marked warmup,
// deduplicated by (provider, baseURL, modelID) so the same backend model used in
// several brains is warmed once (ADR-0031 sub-phase 6, FR-M4). The DECORATED
// model is stored so the warmup inherits the per-attempt window + F6 no-retry.
func (b *builder) collectWarmup(m config.ModelConfig, decorated model.Model) {
	key := m.Provider + "|" + m.BaseURL + "|" + m.ModelID
	if b.warmupSeen == nil {
		b.warmupSeen = make(map[string]bool)
	}
	if b.warmupSeen[key] {
		b.logger.Info("warmup deduplicated", "provider", m.Provider, "model", m.ModelID)
		return
	}
	b.warmupSeen[key] = true
	b.warmupTargets = append(b.warmupTargets, warmupTarget{
		model:    decorated,
		provider: m.Provider,
		modelID:  m.ModelID,
	})
}

// effectiveConfig returns the config being wired, or an empty Config for direct
// buildBrain unit tests (b.cfg nil) so EffectiveRequestTimeout falls back to
// DefaultRequestTimeout instead of dereferencing nil.
func (b *builder) effectiveConfig() *config.Config {
	if b.cfg != nil {
		return b.cfg
	}
	return &config.Config{}
}

// effectiveMaxRetries resolves the retry budget for one model under one brain
// (ADR-0031 Decisions 3/4): 0 for sequential (retry off by construction — SV2)
// or when the per-brain retry toggle is explicitly false; otherwise the
// per-model max_retries (the toggle nil means on). Zero still leaves the
// decorator applying the per-attempt deadline (SV3).
func effectiveMaxRetries(bc config.BrainConfig, m config.ModelConfig) int {
	if bc.Dispatch == "sequential" {
		return 0
	}
	if bc.Retry != nil && !*bc.Retry {
		return 0
	}
	return m.MaxRetries
}

// buildModel constructs one provider adapter, resolving its secret from the
// environment by the configured env-var NAME (never from the file). Ollama
// never connects here (a downed Ollama is not a boot error); Groq requires its
// API key present at boot.
func (b *builder) buildModel(m config.ModelConfig) (model.Model, error) {
	switch m.Provider {
	case "ollama":
		// No WithRequestTimeout: the per-attempt deadline has a single owner (the
		// router ceiling today; the retry decorator once it lands), so the adapter
		// is bounded by the ctx it receives, never by a second wired timeout
		// (ADR-0031 Decision 2 — one owner of the deadline).
		var opts []ollama.Option
		if m.BaseURL != "" {
			opts = append(opts, ollama.WithBaseURL(m.BaseURL))
		}
		return ollama.New(opts...), nil
	case "groq":
		key := os.Getenv(m.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("%w: %q (groq API key for model %q)", ErrMissingSecret, m.APIKeyEnv, m.ModelID)
		}
		// No WithRequestTimeout, for the same single-owner reason as Ollama above.
		opts := []groq.Option{groq.WithAPIKey(key)}
		if m.BaseURL != "" {
			opts = append(opts, groq.WithBaseURL(m.BaseURL))
		}
		g, err := groq.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("app: groq model %q: %w", m.ModelID, err)
		}
		return g, nil
	case "openai-compatible":
		// ADR-0044 / FR-GW-6. api_key_env is OPTIONAL (D2): absent means a
		// no-auth endpoint (no Authorization header); named-but-empty is the
		// fail-loud ErrMissingSecret naming the VARIABLE, never key material.
		// The env-var NAME also rides as the non-secret auth label so a 401
		// diagnostic can point the operator at the right variable (H7).
		// No WithRequestTimeout, for the same single-owner reason as above
		// (ADR-0031 Decision 2).
		opts := []openaicompat.Option{openaicompat.WithBaseURL(m.BaseURL)}
		if m.APIKeyEnv != "" {
			key := os.Getenv(m.APIKeyEnv)
			if key == "" {
				return nil, fmt.Errorf("%w: %q (openai-compatible API key for model %q)", ErrMissingSecret, m.APIKeyEnv, m.ModelID)
			}
			opts = append(opts, openaicompat.WithAPIKey(key), openaicompat.WithAuthLabel(m.APIKeyEnv))
		}
		oc, err := openaicompat.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("app: openai-compatible model %q: %w", m.ModelID, err)
		}
		return oc, nil
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
	case discord.ChannelName:
		// Pre-check the env at the app layer so a missing secret is a loud, named boot
		// error consistent with the telegram case (SP1 F4 reconciliation). discord.New
		// resolves the token again from the same env var at connect time and never
		// stores it; the pre-check only tests presence, never the value (ADR-0010).
		if os.Getenv(cc.TokenEnv) == "" {
			return nil, fmt.Errorf("%w: %q (discord bot token)", ErrMissingSecret, cc.TokenEnv)
		}
		ad, err := discord.New(
			discord.WithTokenEnv(cc.TokenEnv),
			discord.WithMode(discord.ModeGateway),
			discord.WithLogger(b.logger),
		)
		if err != nil {
			return nil, err
		}
		return ad, nil
	case webhook.ChannelName:
		return buildWebhookChannel(b, cc)
	case console.ChannelName:
		// The internal direct chat (FR-CONS-1): no network, no secret. In a
		// real Build the sessionful store is present (config.Validate
		// demands the storage block); in PREFLIGHT the builder carries NO
		// store by design (openStore happens inside the cutover), so a nil
		// store constructs a throwaway channel — Preflight only proves
		// constructibility (the 2026-08-08 reload-rollback repro).
		if b.store == nil {
			return console.New(nil), nil
		}
		ss, ok := b.store.(conversation.SessionStore)
		if !ok {
			return nil, fmt.Errorf("app: console channel requires the sessionful store")
		}
		return console.New(ss), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannelType, cc.Type)
	}
}

// buildWebhookChannel resolves the webhook channel's env-only secrets and threads the
// config's effective bind/path/mapping into webhook.Options (ADR-0038 §§1-5). The
// inbound secret (token_env) is mandatory — an unset value is a loud, named boot error
// with telegram/discord parity; the outbound secret (outbound_token_env) is optional
// but, once NAMED, must resolve — a named-but-unset outbound secret fails loud rather
// than booting silently un-authenticated (ADR-0038 §4). A non-loopback bind is warned
// about (§7). Secret VALUES are resolved here and passed by value to the adapter;
// only the env-var names ever appear in an error (ADR-0010).
func buildWebhookChannel(b *builder, cc config.ChannelConfig) (Channel, error) {
	secret := os.Getenv(cc.TokenEnv)
	if secret == "" {
		return nil, fmt.Errorf("%w: %q (webhook inbound secret)", ErrMissingSecret, cc.TokenEnv)
	}

	var outboundToken string
	if cc.Webhook.OutboundTokenEnv != "" {
		outboundToken = os.Getenv(cc.Webhook.OutboundTokenEnv)
		if outboundToken == "" {
			return nil, fmt.Errorf("%w: %q (webhook outbound secret)", ErrMissingSecret, cc.Webhook.OutboundTokenEnv)
		}
	}

	bind := cc.Webhook.EffectiveBind()
	if !isLoopbackBind(bind) {
		b.logger.Warn(
			"webhook: non-loopback bind — the Bearer secret crosses the network in cleartext unless a TLS-terminating reverse proxy fronts this endpoint",
			"channel", webhook.ChannelName, "bind", bind)
	}

	em := cc.Webhook.EffectiveMapping()
	return webhook.NewWithOptions(webhook.ChannelName, webhook.Options{
		Bind:          bind,
		Path:          cc.Webhook.EffectivePath(),
		Secret:        secret,
		OutboundURL:   cc.Webhook.OutboundURL,
		OutboundToken: outboundToken,
		Mapping: webhook.FieldMapping{
			SenderID:       em.SenderID,
			SenderName:     em.SenderName,
			Text:           em.Text,
			MediaURL:       em.MediaURL,
			MediaType:      em.MediaType,
			ConversationID: em.ConversationID,
		},
	}), nil
}

// isLoopbackBind reports whether a "host:port" bind address is loopback — the
// safe-by-default case (ADR-0038 §7). A parse failure or a non-loopback / unresolvable
// host returns false, so the caller warns (and net.Listen will fail on its own later
// for a truly bad address). "localhost" is treated as loopback without a DNS lookup.
func isLoopbackBind(bind string) bool {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// AdminAddr returns the admin server's effective bound address ("host:port",
// the real port even when the config asked for :0), or "" when observability
// is disabled or the server has not been started. It is the desktop shell's
// status seam (ADR-0035 §§6–7): with the ephemeral-port policy the bound
// address is state only the running App knows.
func (a *App) AdminAddr() string {
	if a.adminServer == nil {
		return ""
	}
	return a.adminServer.Addr()
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
	// Best-effort boot warmup of local models (ADR-0031 sub-phase 6). Launched
	// here in Start — NOT Run — so a supervisor-driven boot (Start/Serve called
	// separately, ADR-0027) warms up too. It runs in the background: a slow or
	// hung model never delays the service coming up, and a first message arriving
	// mid-warmup is already covered by the generous per-attempt timeout.
	a.startWarmup(ctx)
	if len(a.channels) == 0 {
		// NC-1 (SP5, option B): a channel-less boot is valid — the desktop
		// first-run shape — but the operator must learn WHY the gateway is
		// deaf, loudly, on every such start.
		a.logger.Warn("no channels configured — add one via the builder")
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
	// Cancel any in-flight boot warmup and await its unwind first (ADR-0031
	// sub-phase 6, AS-6), bounded by ctx, so no warmup goroutine outlives Shutdown.
	a.awaitWarmup(ctx)
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
	if a.actions != nil {
		if routerErr == nil {
			if err := a.actions.Close(); err != nil {
				errs = append(errs, fmt.Errorf("app: close action store: %w", err))
			}
		} else {
			a.logger.Warn("action store left open: router did not drain within the shutdown deadline")
		}
	}
	// The profile lock releases LAST among storage concerns (R4-F1): a
	// rotation may proceed only once this process is done with the key.
	if a.profileLock != nil {
		if err := a.profileLock.Release(); err != nil {
			errs = append(errs, fmt.Errorf("app: release profile lock: %w", err))
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
// defaults to fan-out, the common case. No WithPerModelTimeout is applied: the
// per-attempt deadline has a single owner (the router ceiling today; the retry
// decorator once it lands), so each Generate is bounded by the Handle ctx alone
// (ADR-0031 Decision 2 — one owner of the deadline).
//
// For fan-out, the cancel-on-first-usable-success mode (ADR-0031 SV1) is wired
// from the brain's policy kind: a "priority" brain cancels its siblings at the
// first usable success (any success is usable), while a "consensus" brain (or
// any non-priority kind) keeps wait-all — a consensus needs every vote, so no
// single success is "usable" (the Decision 4 carve-out). This maps policy shape
// to a MECHANICAL flag here; the fanout coordinator never imports internal/policy.
func buildCoordinator(dispatch, policyKind string) brain.Coordinator {
	switch dispatch {
	case "sequential":
		return sequential.New()
	default: // "" or "fanout"
		if policyKind == "priority" {
			return fanout.New(fanout.WithCancelOnFirstUsableSuccess())
		}
		return fanout.New()
	}
}
