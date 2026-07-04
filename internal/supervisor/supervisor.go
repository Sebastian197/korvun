// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package supervisor owns the Korvun app lifecycle ABOVE a single *app.App
// (ADR-0027). It runs the current app, and on a reload request performs the
// cutover (Shutdown the old app -> build + Run the new one -> swap the reference)
// without touching the frozen router. It is deliberately decoupled from the
// concrete app via func-seams (BuildFunc / PreflightFunc / PersistFunc) and an
// injectable signal channel, so the dangerous cutover concurrency is unit-testable
// with deterministic fakes and `-race`.
package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// App is the lifecycle seam the supervisor drives. *app.App satisfies it (Run
// blocks until its context is cancelled; Shutdown tears the app down in ADR-0008
// order).
type App interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// BuildFunc builds a runnable App from a validated config. The real seam wraps
// app.Build (which opens the store and wires channels/brains); tests inject a fake.
type BuildFunc func(*config.Config) (App, error)

// PreflightFunc validates a config effect-free, before any cutover (the real seam
// is app.Preflight, ADR-0027 step 5). Reserved for the reload handler (Unit C).
type PreflightFunc func(*config.Config) error

// PersistFunc writes the config to the -config file, and is called ONLY after a
// successful cutover (ADR-0027 §F5). The real seam is WriteConfigAtomic.
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
	reasonAppFailed                   // app.Run returned an error on its own
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
	persist    PersistFunc
	signalCh   <-chan os.Signal

	reloadCh chan reloadReq

	// mu guards the mutable observable state (current, status, reloadInProgress),
	// which the Run loop writes at the cutover and callers (Current/Status/
	// RequestReload) read concurrently.
	mu               sync.Mutex
	current          App
	status           map[Handle]State
	reloadInProgress bool
	seq              uint64

	shutdownTimeout time.Duration
}

// Option configures a Supervisor.
type Option func(*Supervisor)

// WithBuild sets the App factory (required for Run; the real seam wraps app.Build).
func WithBuild(f BuildFunc) Option { return func(s *Supervisor) { s.build = f } }

// WithPersist sets the config-persist seam, called only after a successful cutover
// (ADR-0027 §F5). The real seam is WriteConfigAtomic; a nil persist is a no-op.
func WithPersist(f PersistFunc) Option { return func(s *Supervisor) { s.persist = f } }

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
		reloadCh:        make(chan reloadReq, 1),
		status:          make(map[Handle]State),
		shutdownTimeout: defaultShutdownTimeout,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run builds and runs the initial app, then owns its lifecycle until an external
// shutdown signal (or the parent context) stops it, performing a cutover on each
// reload request. It blocks until shutdown and returns nil on a clean stop, or a
// boot/cutover/rollback error (the caller exits non-zero; systemd restarts).
func (s *Supervisor) Run(ctx context.Context) error {
	curCfg := s.initialCfg
	prevCfg := s.initialCfg
	var curHandle Handle // "" for the initial app and rolled-back apps

	app, err := s.build(curCfg)
	if err != nil {
		return fmt.Errorf("supervisor: initial build: %w", err)
	}
	s.setCurrent(app)

	for {
		reason, req, runErr := s.serve(ctx, app)
		_ = s.shutdownApp(app)

		switch reason {
		case reasonShutdown:
			return nil

		case reasonAppFailed:
			if curHandle == "" {
				// The initial or an already-rolled-back app crashed: not a cutover
				// failure, and nothing better to roll back to. Exit fatally and let
				// systemd restart the last known-good -config (no rollback loop).
				return fmt.Errorf("supervisor: running app failed: %w", runErr)
			}
			// A freshly-reloaded app failed to serve: roll back to the previous
			// config and re-persist it, so the -config never stays at a config whose
			// app failed to run (ADR-0027 §c crash-loop guard).
			s.setStatus(curHandle, StateRolledBack)
			s.finishReload()
			rApp, rerr := s.build(prevCfg)
			if rerr != nil {
				return fmt.Errorf("supervisor: rollback failed after app run error (%v): %w", runErr, rerr)
			}
			s.setCurrent(rApp)
			s.persistConfig(prevCfg)
			curCfg, curHandle = prevCfg, ""
			app = rApp

		case reasonReload:
			s.setStatus(req.handle, StateCutoverInProgress)
			nApp, berr := s.build(req.cfg)
			if berr != nil {
				// Build failed after the old app was shut down: roll back to the
				// config that was just serving (curCfg). No persist — a failed cutover
				// must not overwrite the -config (F5).
				s.setStatus(req.handle, StateRolledBack)
				s.finishReload()
				rApp, rerr := s.build(curCfg)
				if rerr != nil {
					return fmt.Errorf("supervisor: rollback failed after cutover build error (%v): %w", berr, rerr)
				}
				s.setCurrent(rApp)
				curHandle = ""
				app = rApp
				continue
			}
			// Successful swap: swap the reference, mark succeeded, persist (F5).
			prevCfg, curCfg, curHandle = curCfg, req.cfg, req.handle
			s.setCurrent(nApp)
			s.setStatus(req.handle, StateSucceeded)
			s.persistConfig(req.cfg)
			s.finishReload()
			app = nApp
		}
	}
}

// serve runs one app under a child context the supervisor alone cancels, and blocks
// until the app fails, a reload request, an external signal, or the parent context
// ends. The child context is the "cutover cancel" half of F6/N2: cancelling it is how
// the supervisor (and only the supervisor) stops the app for a cutover, distinct from
// the external signal that arrives on its own channel; an app that returns on its own
// (without our cancel) is reasonAppFailed.
func (s *Supervisor) serve(ctx context.Context, app App) (stopReason, reloadReq, error) {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- app.Run(childCtx) }()

	select {
	case err := <-runErr:
		return reasonAppFailed, reloadReq{}, err
	case <-ctx.Done():
		cancel()
		<-runErr
		return reasonShutdown, reloadReq{}, nil
	case <-s.signalCh:
		cancel()
		<-runErr
		return reasonShutdown, reloadReq{}, nil
	case req := <-s.reloadCh:
		cancel()
		<-runErr
		return reasonReload, req, nil
	}
}

// RequestReload hands a validated config to the Run loop and returns an opaque
// handle to poll via Status. It is single-flight (ADR-0027 §flow step 4): if a
// reload is already running it returns ErrReloadInProgress and builds nothing.
func (s *Supervisor) RequestReload(cfg *config.Config) (Handle, error) {
	s.mu.Lock()
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

func (s *Supervisor) persistConfig(cfg *config.Config) {
	if s.persist != nil {
		_ = s.persist(cfg)
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
// is the real PersistFunc the supervisor calls after a successful cutover.
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
