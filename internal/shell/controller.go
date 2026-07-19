// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// Sentinel errors of the Controller's state machine (design spec AS-7). They
// wrap nothing: each names a caller mistake, not a system failure.
var (
	// ErrNoConfig is returned by Start when no config has been loaded yet.
	ErrNoConfig = errors.New("shell: no config loaded")
	// ErrAlreadyRunning is returned by Start while the core is running.
	ErrAlreadyRunning = errors.New("shell: core already running")
	// ErrNotRunning is returned by Stop while the core is stopped.
	ErrNotRunning = errors.New("shell: core not running")
	// ErrRunning is returned by LoadConfig while the core is running:
	// switching config files is a stopped-state operation (live mutation is
	// the builder/reload path's job, not the shell's).
	ErrRunning = errors.New("shell: cannot load a config while the core is running")
)

// ephemeralAdminAddr is the in-memory admin bind the shell forces (ADR-0035
// §6): loopback, kernel-assigned port, so a desktop core never collides with
// a running headless korvun or a user-pinned port. The file on disk never
// learns this value.
const ephemeralAdminAddr = "127.0.0.1:0"

// Status is the shell's view of the core (design spec FR-7).
type Status struct {
	// Running reports whether the core is up (a successful Start not yet
	// followed by Stop).
	Running bool
	// ConfigPath is the loaded config file's path ("" before LoadConfig).
	ConfigPath string
	// AdminAddr is the admin server's EFFECTIVE bound address while running
	// ("" when stopped) — the real ephemeral port, state only the running
	// core knows.
	AdminAddr string
}

// Option configures New.
type Option func(*Controller)

// WithLogger sets the structured logger. A nil logger is ignored.
func WithLogger(l *slog.Logger) Option {
	return func(c *Controller) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithBuildOptions appends extra app.Build/app.Preflight options to the
// shell's build seam — the embedding/test seam that lets the lifecycle suite
// boot a full real App with fake channels (no network, ADR-0034 discipline).
func WithBuildOptions(opts ...app.Option) Option {
	return func(c *Controller) { c.buildOpts = append(c.buildOpts, opts...) }
}

// Controller is the desktop shell's lifecycle logic (ADR-0035 §§1, 3a, 4, 6)
// as plain framework-free Go: it loads a config, runs the in-process core
// under the reload supervisor (the builder's mount precondition), enforces
// the ephemeral-port policy, provisions the per-cycle admin bearer, and
// reports status. It never imports Wails (doc.go contract); the thin Wails
// adapter in cmd/korvun-desktop wraps it.
type Controller struct {
	logger    *slog.Logger
	buildOpts []app.Option

	mu       sync.Mutex
	cfg      *config.Config
	path     string
	running  bool
	tokenEnv string             // env var the shell set this cycle ("" if none)
	cancel   context.CancelFunc // stops the supervisor's Run
	done     chan struct{}      // closed when Run returns; runErr is set before
	runErr   error              // Run's result; read only after <-done

	cur atomic.Pointer[app.App] // current built core, for AdminAddr
}

// New builds a stopped Controller.
func New(opts ...Option) *Controller {
	c := &Controller{logger: slog.Default()}
	for _, o := range opts {
		o(c)
	}
	return c
}

// LoadConfig loads and validates the config file at path (config.Load: every
// failure is fatal and names what is wrong). It is a stopped-state operation:
// while the core runs it returns ErrRunning.
func (c *Controller) LoadConfig(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reapLocked()
	if c.running {
		return ErrRunning
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("shell: load config %q: %w", path, err)
	}
	c.cfg = cfg
	c.path = path
	return nil
}

// Start boots the core under the reload supervisor and returns once the core
// CONFIRMED its Start (admin bound, channels started) or failed boot (the
// supervisor error is returned and the controller stays stopped). ctx bounds
// only the wait; the running core is stopped by Stop, never by ctx. Start
// holds the controller lock for the whole boot wait, so concurrent Status/
// LoadConfig calls block until the boot resolves (boots are sub-second; a
// stuck boot is bounded by ctx).
func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reapLocked()
	if c.running {
		return ErrAlreadyRunning
	}
	if c.cfg == nil {
		return ErrNoConfig
	}

	// Per-cycle admin bearer (ADR-0035 §4): generated BEFORE the initial
	// build so the mutation surface's mount-time env read sees it. Always
	// overwritten — per-cycle generation is the ADR's mandate for the bearer.
	tokenEnv := ""
	if c.cfg.Admin != nil && c.cfg.Admin.TokenEnv != "" {
		tokenEnv = c.cfg.Admin.TokenEnv
		token, err := newAdminToken()
		if err != nil {
			return fmt.Errorf("shell: generate admin bearer: %w", err)
		}
		if err := os.Setenv(tokenEnv, token); err != nil {
			return fmt.Errorf("shell: set admin bearer env %q: %w", tokenEnv, err)
		}
	}

	// The serve.go wiring, shell-flavored: the build seam applies the
	// ephemeral-port override to a COPY of whatever config it receives
	// (initial boot AND every builder-driven reload), so the persist seam
	// always writes the user's pristine config back to disk.
	var sup *supervisor.Supervisor
	started := make(chan struct{})
	var once sync.Once
	build := func(cfg *config.Config) (supervisor.App, error) {
		a, err := app.Build(withEphemeralAdmin(cfg), c.appOptions(sup)...)
		if err != nil {
			return nil, err
		}
		// During a failed reload cutover this transiently holds the new app
		// until the supervisor's rollback re-runs this closure with the old
		// config — Status may briefly read ""/stale mid-cutover; self-healing.
		c.cur.Store(a)
		return &notifyingApp{App: a, onStarted: func() { once.Do(func() { close(started) }) }}, nil
	}
	preflight := func(cfg *config.Config) error {
		return app.Preflight(withEphemeralAdmin(cfg), c.appOptions(sup)...)
	}
	path := c.path
	persist := func(cfg *config.Config) error {
		return supervisor.WriteConfigAtomic(path, cfg)
	}
	// The shell owns shutdown through context cancellation; the signal
	// channel is never signalled (a desktop app must not hijack process
	// signals).
	sigCh := make(chan os.Signal)

	sup = supervisor.New(c.cfg,
		supervisor.WithBuild(build),
		supervisor.WithPreflight(preflight),
		supervisor.WithPersist(persist),
		supervisor.WithLogger(c.logger),
		supervisor.WithSignalChan(sigCh),
	)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		err := sup.Run(runCtx)
		c.cur.Store(nil)
		c.runErr = err // happens-before any reader via close(done)
		close(done)
	}()

	select {
	case <-started:
		c.running = true
		c.tokenEnv = tokenEnv
		c.cancel = cancel
		c.done = done
		return nil
	case <-done:
		cancel()
		c.clearBearer(tokenEnv)
		return fmt.Errorf("shell: core boot failed: %w", c.runErr)
	case <-ctx.Done():
		cancel()
		<-done
		c.clearBearer(tokenEnv)
		return fmt.Errorf("shell: start wait cancelled: %w", ctx.Err())
	}
}

// Stop cancels the supervisor and waits for a clean teardown, bounded by ctx.
// On a clean stop it returns nil, unsets the per-cycle bearer, and the
// controller is startable again. If ctx expires first the controller keeps
// its running state (the teardown is still in flight) and returns ctx's
// error — the caller retries with a longer deadline. Stop holds the
// controller lock through the teardown wait (see Start's locking note).
func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reapLocked()
	if !c.running {
		return ErrNotRunning
	}
	c.cancel()
	select {
	case <-c.done:
	case <-ctx.Done():
		return fmt.Errorf("shell: stop wait cancelled: %w", ctx.Err())
	}
	err := c.runErr
	c.running = false
	c.cancel = nil
	c.done = nil
	c.clearBearer(c.tokenEnv)
	c.tokenEnv = ""
	if err != nil {
		return fmt.Errorf("shell: core stopped with error: %w", err)
	}
	return nil
}

// Status reports the shell's view of the core.
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reapLocked()
	st := Status{Running: c.running, ConfigPath: c.path}
	if c.running {
		if a := c.cur.Load(); a != nil {
			st.AdminAddr = a.AdminAddr()
		}
	}
	return st
}

// reapLocked (mu held) observes a run goroutine that exited on its OWN — the
// supervisor returning without Stop (e.g. a failed rollback after a bad
// cutover). Without this, a dead core would read as Running=true forever and
// Start would refuse with ErrAlreadyRunning. The exit error is logged (the
// UI's Status shows stopped); the bearer is cleared like any other cycle end.
func (c *Controller) reapLocked() {
	if !c.running {
		return
	}
	select {
	case <-c.done:
	default:
		return
	}
	if c.runErr != nil {
		c.logger.Error("shell: core exited on its own", "error", c.runErr.Error())
	} else {
		c.logger.Warn("shell: core exited on its own with no error")
	}
	c.running = false
	c.cancel = nil
	c.done = nil
	c.clearBearer(c.tokenEnv)
	c.tokenEnv = ""
}

// appOptions assembles the options every app.Build/app.Preflight call in the
// build seam uses: the shell's logger, the reload supervisor (the builder's
// mount precondition, ADR-0035 §1), and any embedder extras.
func (c *Controller) appOptions(sup *supervisor.Supervisor) []app.Option {
	opts := []app.Option{app.WithLogger(c.logger), app.WithReloader(sup)}
	return append(opts, c.buildOpts...)
}

// clearBearer unsets the env var the shell set this cycle. The shell wrote
// it, the shell removes it — nothing else in the process owns that variable.
func (c *Controller) clearBearer(tokenEnv string) {
	if tokenEnv != "" {
		_ = os.Unsetenv(tokenEnv)
	}
}

// newAdminToken returns 32 crypto/rand bytes hex-encoded — the per-cycle
// admin bearer (ADR-0035 §4: generated in-process, never typed, never
// persisted).
func newAdminToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// withEphemeralAdmin returns a shallow copy of cfg whose Observability block
// is replaced by a copy binding the admin server to ephemeralAdminAddr
// (ADR-0035 §6). The input is never mutated — the supervisor's persist seam
// receives the original pointer, so the file on disk never learns the
// override. Enabled semantics are preserved: an explicitly disabled admin
// server stays disabled (the override changes WHERE it binds, never WHETHER).
// Invariant this copy leans on: the shallow copy SHARES Channels/Brains/
// Routes/Admin with the pristine original, which is safe because Build and
// Preflight never mutate their input config — if that ever changes, the
// pristine-persist property breaks here first.
func withEphemeralAdmin(cfg *config.Config) *config.Config {
	out := *cfg
	var obs config.ObservabilityConfig
	if cfg.Observability != nil {
		obs = *cfg.Observability
	}
	obs.Addr = ephemeralAdminAddr
	out.Observability = &obs
	return &out
}

// notifyingApp wraps a supervisor.App to signal the first successful Start —
// how Controller.Start learns the boot completed without polling.
type notifyingApp struct {
	supervisor.App
	onStarted func()
}

func (n *notifyingApp) Start(ctx context.Context) error {
	if err := n.App.Start(ctx); err != nil {
		return err
	}
	n.onStarted()
	return nil
}
