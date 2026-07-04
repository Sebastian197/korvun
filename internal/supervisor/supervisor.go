// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package supervisor owns the Korvun app lifecycle ABOVE a single *app.App
// (ADR-0027). It runs the current app, and on a reload request performs the
// cutover (Shutdown the old app -> build + Start the new one -> persist -> swap ->
// Serve) without touching the frozen router. It is decoupled from the concrete app
// via func-seams (BuildFunc / PreflightFunc / PersistFunc) and an injectable signal
// channel, so the dangerous cutover concurrency is unit-testable with deterministic
// fakes and `-race`.
//
// Persistence is by-construction safe (ADR-0027 §c / §Persistence): the new config
// is persisted ONLY after the new app's Start returns nil — i.e. after the fallible
// bind/channel-start steps have all succeeded. Any earlier failure (build or Start)
// rolls back and NEVER persists, so the on-disk -config is never overwritten with a
// config that cannot come up, and there is no crash-loop into a bad config.
package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
)

// defaultShutdownTimeout bounds each app.Shutdown the supervisor drives, preserving
// the 15s graceful-shutdown budget cmd/korvun guaranteed before the supervisor owned
// the lifecycle (ADR-0008).
const defaultShutdownTimeout = 15 * time.Second

// ErrReloadInProgress is returned by RequestReload when a reload is already running
// (single-flight, ADR-0027 §flow step 4 -> the handler maps it to 409
// reload_in_progress).
var ErrReloadInProgress = errors.New("supervisor: a reload is already in progress")

// ErrShuttingDown is returned by RequestReload once the supervisor has begun
// shutting down, so a late reload is refused rather than left as an orphaned
// pending handle no one can complete (the handler maps it to a 503).
var ErrShuttingDown = errors.New("supervisor: shutting down")

// App is the lifecycle seam the supervisor drives. *app.App satisfies it. Start
// brings the app up without blocking (the fallible bind/channel-start steps); Serve
// blocks until its context is cancelled; Shutdown tears the app down in ADR-0008
// order. A successful Start is the cutover-confirmation the supervisor persists after.
type App interface {
	Start(ctx context.Context) error
	Serve(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// BuildFunc builds a (not-yet-started) App from a validated config. The real seam
// wraps app.Build; tests inject a fake.
type BuildFunc func(*config.Config) (App, error)

// PreflightFunc validates a config effect-free, before any cutover (the real seam
// is app.Preflight, ADR-0027 step 5). Reserved for the reload handler (Unit C).
type PreflightFunc func(*config.Config) error

// PersistFunc writes the config to the -config file, and is called ONLY after a
// confirmed cutover (the new app's Start returned nil, ADR-0027 §F5). The real seam
// is WriteConfigAtomic.
type PersistFunc func(*config.Config) error

// State is the observable status of a reload, owned by the supervisor so it
// survives the cutover that tears down the admin server (ADR-0027 §F4).
type State string

// The reload lifecycle states (ADR-0027 §seam+status).
const (
	StatePending           State = "pending"
	StateCutoverInProgress State = "cutover-in-progress"
	StateSucceeded         State = "succeeded"
	StateRolledBack        State = "rolled-back"
	StateFailed            State = "failed"
)

// Handle is the opaque identifier a reload request returns; the caller polls
// Status(handle) to observe the outcome across the cutover blip.
type Handle string

// stopReason is why the current app stopped serving.
type stopReason int

const (
	reasonShutdown  stopReason = iota // external signal / parent context cancelled
	reasonReload                      // a reload request wants a cutover
	reasonAppFailed                   // app.Serve returned an error on its own
)

// reloadReq carries a validated config and its status handle from RequestReload
// to the Run loop.
type reloadReq struct {
	cfg    *config.Config
	handle Handle
}

// Supervisor owns the app lifecycle above *app.App. Construct with New.
type Supervisor struct {
	initialCfg *config.Config
	build      BuildFunc
	preflight  PreflightFunc
	persist    PersistFunc
	signalCh   <-chan os.Signal
	logger     *slog.Logger

	reloadCh chan reloadReq

	// mu guards the mutable observable state (current, status, reloadInProgress),
	// which the Run loop writes at the cutover and callers (Current/Status/
	// RequestReload) read concurrently.
	mu               sync.Mutex
	current          App
	status           map[Handle]State
	reloadInProgress bool
	shuttingDown     bool
	seq              uint64

	shutdownTimeout time.Duration
}

// Option configures a Supervisor.
type Option func(*Supervisor)

// WithBuild sets the App factory (required for Run; the real seam wraps app.Build).
func WithBuild(f BuildFunc) Option { return func(s *Supervisor) { s.build = f } }

// WithPreflight sets the effect-free pre-cutover validation seam (ADR-0027 §5). The
// supervisor runs it while the old app is still serving; a failure fails the reload
// cheaply without touching the running app. The real seam is app.Preflight.
func WithPreflight(f PreflightFunc) Option { return func(s *Supervisor) { s.preflight = f } }

// WithPersist sets the config-persist seam, called only after a confirmed cutover
// (ADR-0027 §F5). The real seam is WriteConfigAtomic; a nil persist is a no-op.
func WithPersist(f PersistFunc) Option { return func(s *Supervisor) { s.persist = f } }

// WithLogger sets the structured logger (default slog.Default()). A nil logger is
// ignored. The supervisor logs persist failures through it (they are not fatal).
func WithLogger(l *slog.Logger) Option {
	return func(s *Supervisor) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithSignalChan injects the channel the supervisor listens on for an external
// shutdown signal. Keeping it a seam (rather than calling signal.Notify itself)
// makes signal handling deterministic under test, and is the "own channel, not the
// App's context" half of the F6/N2 distinguishability contract.
func WithSignalChan(ch <-chan os.Signal) Option { return func(s *Supervisor) { s.signalCh = ch } }

// New builds a Supervisor that will run initial when Run is called. Seams are set
// via Options.
func New(initial *config.Config, opts ...Option) *Supervisor {
	s := &Supervisor{
		initialCfg:      initial,
		logger:          slog.Default(),
		reloadCh:        make(chan reloadReq, 1),
		status:          make(map[Handle]State),
		shutdownTimeout: defaultShutdownTimeout,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run builds and Starts the initial app, then owns its lifecycle until an external
// shutdown signal (or the parent context) stops it, performing a cutover on each
// reload request. It blocks until shutdown and returns nil on a clean stop, or a
// boot/cutover/rollback error (the caller exits non-zero; systemd restarts).
func (s *Supervisor) Run(ctx context.Context) error {
	curCfg := s.initialCfg // the last config whose app Started successfully

	app, err := s.buildAndStart(ctx, curCfg)
	if err != nil {
		return fmt.Errorf("supervisor: initial start: %w", err)
	}
	s.setCurrent(app)

	for {
		reason, req, serveErr := s.serve(ctx, app)
		_ = s.shutdownApp(app)

		switch reason {
		case reasonShutdown:
			s.mu.Lock()
			s.shuttingDown = true
			s.mu.Unlock()
			return nil

		case reasonAppFailed:
			// The serving app returned an error on its own. Its config had already
			// Started successfully, so a plain restart of it is safe: exit fatally
			// and let systemd restart (no rollback loop, ADR §c).
			return fmt.Errorf("supervisor: running app failed: %w", serveErr)

		case reasonReload:
			s.setStatus(req.handle, StateCutoverInProgress)
			nApp, nerr := s.buildAndStart(ctx, req.cfg)
			if nerr != nil {
				// The new config failed to build or Start (the ADR §c "admin re-bind"
				// failure). Roll back to the config that was serving. persist is NEVER
				// called on a failed cutover, so the -config on disk is untouched and
				// still boots (F5).
				s.setStatus(req.handle, StateRolledBack)
				s.finishReload()
				rApp, rerr := s.buildAndStart(ctx, curCfg)
				if rerr != nil {
					return fmt.Errorf("supervisor: rollback failed after cutover error (%v): %w", nerr, rerr)
				}
				s.setCurrent(rApp)
				app = rApp
				continue
			}
			// Start confirmed the new app is serving: swap, mark succeeded, and only
			// NOW persist (F5). curCfg advances to the new, proven config.
			s.setCurrent(nApp)
			s.setStatus(req.handle, StateSucceeded)
			s.persistConfig(req.cfg)
			curCfg = req.cfg
			s.finishReload()
			app = nApp
		}
	}
}

// buildAndStart builds an app and Starts it. On a Start failure it Shuts the app
// down (so a half-built app leaks no worker/store) and returns the error.
func (s *Supervisor) buildAndStart(ctx context.Context, cfg *config.Config) (App, error) {
	app, err := s.build(cfg)
	if err != nil {
		return nil, err
	}
	if err := app.Start(ctx); err != nil {
		_ = s.shutdownApp(app)
		return nil, err
	}
	return app, nil
}

// serve runs an already-Started app's Serve under a child context the supervisor
// alone cancels, and blocks until the app fails, a reload request, an external
// signal, or the parent context ends. The child context is the "cutover cancel" half
// of F6/N2 (cancelling it stops the app for a cutover); the external signal arrives
// on its own channel; a Serve that returns on its own is reasonAppFailed.
func (s *Supervisor) serve(ctx context.Context, app App) (stopReason, reloadReq, error) {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- app.Serve(childCtx) }()

	for {
		select {
		case err := <-serveErr:
			return reasonAppFailed, reloadReq{}, err
		case <-ctx.Done():
			cancel()
			<-serveErr
			return reasonShutdown, reloadReq{}, nil
		case <-s.signalCh:
			cancel()
			<-serveErr
			return reasonShutdown, reloadReq{}, nil
		case req := <-s.reloadCh:
			// The old app is STILL serving here. Run Preflight (effect-free) BEFORE
			// tearing it down (ADR-0027 §5 / F7): a bad config fails cheaply while the
			// old app keeps serving. Only a passing Preflight returns reasonReload; a
			// failing one keeps the same app serving (its Serve goroutine is intact).
			if s.preflight != nil {
				if err := s.preflight(req.cfg); err != nil {
					s.setStatus(req.handle, StateFailed)
					s.finishReload()
					continue
				}
			}
			cancel()
			<-serveErr
			return reasonReload, req, nil
		}
	}
}

// RequestReload hands a validated config to the Run loop and returns an opaque
// handle to poll via Status. It is single-flight (ADR-0027 §flow step 4): if a
// reload is already running it returns ErrReloadInProgress and builds nothing.
func (s *Supervisor) RequestReload(cfg *config.Config) (Handle, error) {
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return "", ErrShuttingDown
	}
	if s.reloadInProgress {
		s.mu.Unlock()
		return "", ErrReloadInProgress
	}
	s.reloadInProgress = true
	s.seq++
	h := Handle(fmt.Sprintf("reload-%d", s.seq))
	s.status[h] = StatePending
	s.mu.Unlock()

	s.reloadCh <- reloadReq{cfg: cfg, handle: h}
	return h, nil
}

// Status returns the last known state of a reload handle (empty State if unknown).
func (s *Supervisor) Status(h Handle) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status[h]
}

// Current returns the app the supervisor is currently running.
func (s *Supervisor) Current() App {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Supervisor) setCurrent(a App) {
	s.mu.Lock()
	s.current = a
	s.mu.Unlock()
}

func (s *Supervisor) setStatus(h Handle, st State) {
	s.mu.Lock()
	s.status[h] = st
	s.mu.Unlock()
}

func (s *Supervisor) finishReload() {
	s.mu.Lock()
	s.reloadInProgress = false
	s.mu.Unlock()
}

// persistConfig writes the config after a confirmed cutover. A persist failure is
// logged, not fatal: the new app is already serving (its Start succeeded); only the
// on-disk record lags, so the reload is live but would not survive a restart.
func (s *Supervisor) persistConfig(cfg *config.Config) {
	if s.persist == nil {
		return
	}
	if err := s.persist(cfg); err != nil {
		s.logger.Error("supervisor: persisting config after a confirmed cutover failed; the reload is live but will not survive a restart", "error", err.Error())
	}
}

// shutdownApp tears an app down within the supervisor's shutdown budget.
func (s *Supervisor) shutdownApp(app App) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	return app.Shutdown(ctx)
}

// WriteConfigAtomic writes cfg to path atomically: it marshals to JSON, writes a
// temp file in the SAME directory, then renames it over path. A failure before the
// rename leaves the existing -config untouched (ADR-0027 §Config persistence). This
// is the real PersistFunc the supervisor calls after a confirmed cutover.
func WriteConfigAtomic(path string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("supervisor: marshal config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".korvun-config-*.tmp")
	if err != nil {
		return fmt.Errorf("supervisor: create temp config: %w", err)
	}
	tmpName := tmp.Name()
	// Remove the temp on any failure; a no-op once the rename has consumed it.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("supervisor: write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("supervisor: close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("supervisor: rename config into place: %w", err)
	}
	return nil
}
