// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package supervisor owns the Korvun app lifecycle ABOVE a single *app.App
// (ADR-0027). It runs the current app, and on a reload request performs the
// cutover (Shutdown the old app -> build + Run the new one -> swap the reference)
// without touching the frozen router. It is deliberately decoupled from the
// concrete app via func-seams (BuildFunc / PreflightFunc / PersistFunc) and an
// injectable signal channel, so the dangerous cutover concurrency is unit-testable
// with deterministic fakes and `-race`.
//
// Scope note: this cut implements the lifecycle skeleton — the initial run, a
// clean shutdown on an external signal, and the reload cutover with a race-free
// reference swap. Single-flight, rollback, effect-free Preflight gating, config
// persistence, and the full status lifecycle are added by later units.
package supervisor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
)

// defaultShutdownTimeout bounds each app.Shutdown the supervisor drives, preserving
// the 15s graceful-shutdown budget cmd/korvun guaranteed before the supervisor owned
// the lifecycle (ADR-0008).
const defaultShutdownTimeout = 15 * time.Second

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
// is app.Preflight, ADR-0027 step 5). Reserved for the reload path (B2+); not
// exercised by the initial-run and cutover skeleton yet.
type PreflightFunc func(*config.Config) error

// PersistFunc writes the config to the -config file, and is called ONLY after a
// successful cutover (ADR-0027 §F5). The real seam is an atomic temp+rename.
// Reserved for B6+.
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
	reasonShutdown stopReason = iota // external signal / parent context cancelled
	reasonReload                     // a reload request wants a cutover
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
	signalCh   <-chan os.Signal

	reloadCh chan reloadReq

	// mu guards the mutable observable state (current + status), which the Run
	// loop writes at the cutover and callers (Current/Status, and the future HTTP
	// status route) read concurrently.
	mu      sync.Mutex
	current App
	status  map[Handle]State
	seq     uint64

	shutdownTimeout time.Duration
}

// Option configures a Supervisor.
type Option func(*Supervisor)

// WithBuild sets the App factory (required for Run; the real seam wraps app.Build).
func WithBuild(f BuildFunc) Option { return func(s *Supervisor) { s.build = f } }

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
// shutdown signal (or the parent context) stops it. On a reload request it performs
// the cutover. It blocks until shutdown and returns nil on a clean stop, or a
// boot/cutover error.
func (s *Supervisor) Run(ctx context.Context) error {
	app, err := s.build(s.initialCfg)
	if err != nil {
		return fmt.Errorf("supervisor: initial build: %w", err)
	}
	s.setCurrent(app)

	for {
		reason, req := s.serve(ctx, app)
		// The app that was serving is torn down before we either exit or cut over.
		_ = s.shutdownApp(app)
		if reason == reasonShutdown {
			return nil
		}

		// Cutover: build the new app AFTER the old one has been shut down, then
		// swap the reference under the lock so a concurrent reader never races it.
		s.setStatus(req.handle, StateCutoverInProgress)
		newApp, berr := s.build(req.cfg)
		if berr != nil {
			// Rollback is a later unit; for now surface the failure loudly.
			s.setStatus(req.handle, StateFailed)
			return fmt.Errorf("supervisor: cutover build: %w", berr)
		}
		s.setCurrent(newApp)
		s.setStatus(req.handle, StateSucceeded)
		app = newApp
	}
}

// serve runs one app under a child context the supervisor alone cancels, and blocks
// until a reload request, an external signal, or the parent context ends. The child
// context is the "cutover cancel" half of F6/N2: cancelling it is how the supervisor
// (and only the supervisor) stops the app for a cutover, distinct from the external
// signal that arrives on its own channel.
func (s *Supervisor) serve(ctx context.Context, app App) (stopReason, reloadReq) {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- app.Run(childCtx) }()

	select {
	case <-ctx.Done():
		cancel()
		<-runErr
		return reasonShutdown, reloadReq{}
	case <-s.signalCh:
		cancel()
		<-runErr
		return reasonShutdown, reloadReq{}
	case req := <-s.reloadCh:
		cancel()
		<-runErr
		return reasonReload, req
	}
}

// RequestReload hands a validated config to the Run loop and returns an opaque
// handle to poll via Status. (Single-flight rejection is a later unit; this cut
// enqueues the request.)
func (s *Supervisor) RequestReload(cfg *config.Config) (Handle, error) {
	h := s.newHandle()
	s.setStatus(h, StatePending)
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

// shutdownApp tears an app down within the supervisor's shutdown budget.
func (s *Supervisor) shutdownApp(app App) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	return app.Shutdown(ctx)
}

func (s *Supervisor) newHandle() Handle {
	s.mu.Lock()
	s.seq++
	n := s.seq
	s.mu.Unlock()
	return Handle(fmt.Sprintf("reload-%d", n))
}
