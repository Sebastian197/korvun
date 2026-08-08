// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// THE LAW's sentinels (SP6 design spec FR-WIN-2): every binding call is
// bounded — never an unbounded wait from the UI into the Controller's mutex.
var (
	// ErrBindingTimeout names a call that outlived its deadline. The
	// underlying operation CONTINUES in the background (a mutex acquisition
	// is uncancellable); its terminal result is logged and the status
	// polling reconciles the truth.
	ErrBindingTimeout = errors.New("shell: binding call timed out")
	// ErrLifecycleBusy is returned while a Start/Stop is already in flight
	// (including an abandoned, timed-out one that has not yet landed) — the
	// serialization that keeps timed-out goroutines from piling on the mutex.
	ErrLifecycleBusy = errors.New("shell: a lifecycle operation is already in flight")
)

// bindingDeadlines carries the LAW's per-class deadlines. The keychain class
// is deliberately generous: a Linux Secret Service unlock prompt is a
// legitimate user-interaction path, not a fault.
type bindingDeadlines struct {
	lifecycle time.Duration // Start / Stop
	keychain  time.Duration // SetSecret / DeleteSecret
	check     time.Duration // CheckOllama
	read      time.Duration // Status / LoadConfig / EnsureDefaultConfig
}

var defaultBindingDeadlines = bindingDeadlines{
	lifecycle: 30 * time.Second,
	keychain:  60 * time.Second,
	check:     3 * time.Second,
	read:      5 * time.Second,
}

// Desktop is the window's binding surface (ADR-0035 §3a, SP6 spec FR-WIN-2):
// plain framework-free Go over the Controller — cmd/korvun-desktop passes it
// to Wails' Bind and the generated JS calls arrive here. Every method obeys
// THE LAW via goroutine+select; the Wails glue adds nothing.
type Desktop struct {
	ctrl      *Controller
	logger    *slog.Logger
	version   string
	deadlines bindingDeadlines
	client    *http.Client

	lifecycleBusy atomic.Bool
}

// DesktopOption configures NewDesktop.
type DesktopOption func(*Desktop)

// WithDesktopLogger sets the structured logger. A nil logger is ignored.
func WithDesktopLogger(l *slog.Logger) DesktopOption {
	return func(d *Desktop) {
		if l != nil {
			d.logger = l
		}
	}
}

// WithDesktopVersion stamps the version string Version reports (the chrome's
// logo/version row); unset it reads "dev".
func WithDesktopVersion(v string) DesktopOption {
	return func(d *Desktop) {
		if v != "" {
			d.version = v
		}
	}
}

// withBindingDeadlines overrides the LAW's deadlines — the test seam that
// makes the timeout paths run in test time.
func withBindingDeadlines(bd bindingDeadlines) DesktopOption {
	return func(d *Desktop) { d.deadlines = bd }
}

// NewDesktop builds the binding surface over ctrl.
func NewDesktop(ctrl *Controller, opts ...DesktopOption) *Desktop {
	d := &Desktop{
		ctrl:      ctrl,
		logger:    slog.Default(),
		version:   "dev",
		deadlines: defaultBindingDeadlines,
		client:    &http.Client{},
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// bounded runs fn under the LAW: result or a named timeout, never a hang. A
// timed-out fn keeps running; its terminal result is logged when it lands.
func bounded[T any](d *Desktop, op string, timeout time.Duration, fn func() (T, error)) (T, error) {
	type outcome struct {
		v   T
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		v, err := fn()
		done <- outcome{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-time.After(timeout):
		go func() {
			r := <-done
			if r.err != nil {
				d.logger.Warn("shell: abandoned binding call finished", "op", op, "error", r.err.Error())
			} else {
				d.logger.Info("shell: abandoned binding call finished cleanly", "op", op)
			}
		}()
		var zero T
		return zero, fmt.Errorf("shell: %s: %w (deadline %s)", op, ErrBindingTimeout, timeout)
	}
}

// lifecycle serializes Start/Stop under the busy gate: one in flight, ever —
// an abandoned call holds the gate until it truly lands, so timed-out
// goroutines cannot pile on the Controller's mutex.
func (d *Desktop) lifecycle(op string, fn func() error) error {
	if !d.lifecycleBusy.CompareAndSwap(false, true) {
		return fmt.Errorf("shell: %s: %w", op, ErrLifecycleBusy)
	}
	_, err := bounded(d, op, d.deadlines.lifecycle, func() (struct{}, error) {
		defer d.lifecycleBusy.Store(false)
		return struct{}{}, fn()
	})
	return err
}

// Start boots the core (bounded lifecycle call; the ctx handed to the
// Controller carries the same deadline, bounding the boot wait itself).
func (d *Desktop) Start() error {
	return d.lifecycle("start", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), d.deadlines.lifecycle)
		defer cancel()
		return d.ctrl.Start(ctx)
	})
}

// Stop tears the core down (bounded lifecycle call).
func (d *Desktop) Stop() error {
	return d.lifecycle("stop", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), d.deadlines.lifecycle)
		defer cancel()
		return d.ctrl.Stop(ctx)
	})
}

// Status reports the shell's view of the core (bounded read).
func (d *Desktop) Status() (Status, error) {
	return bounded(d, "status", d.deadlines.read, func() (Status, error) {
		return d.ctrl.Status(), nil
	})
}

// LoadConfig loads and validates the config file at path (bounded read;
// stopped-state operation, exactly the Controller's contract).
func (d *Desktop) LoadConfig(path string) error {
	_, err := bounded(d, "load config", d.deadlines.read, func() (struct{}, error) {
		return struct{}{}, d.ctrl.LoadConfig(path)
	})
	return err
}

// DefaultConfigPath resolves the desktop's default config location (pure,
// no Controller involved).
func (d *Desktop) DefaultConfigPath() (string, error) {
	return DefaultConfigPath()
}

// EnsureDefaultConfig provisions the first-run template at the default path
// (the arg-less composition the spec fixes: resolve, then ensure). created
// = true is the chrome's onboarding trigger (ADR-0035 §5). It then heals
// EXISTING configs for the chat piece (EnsureChatBlocks): a pre-chat config
// gains the storage and session blocks on mount, so the Chat tab never
// boots dead after an app upgrade.
func (d *Desktop) EnsureDefaultConfig() (bool, error) {
	return bounded(d, "ensure default config", d.deadlines.read, func() (bool, error) {
		path, err := DefaultConfigPath()
		if err != nil {
			return false, err
		}
		created, err := EnsureDefaultConfig(path)
		if err != nil {
			return false, err
		}
		if _, err := EnsureChatBlocks(path); err != nil {
			return created, err
		}
		return created, nil
	})
}

// SetSecret writes a secret to the OS keychain (wizard step 3; keychain
// deadline class — an OS unlock prompt is a legitimate slow path).
func (d *Desktop) SetSecret(name, value string) error {
	_, err := bounded(d, "set secret", d.deadlines.keychain, func() (struct{}, error) {
		return struct{}{}, d.ctrl.SetSecret(name, value)
	})
	return err
}

// DeleteSecret removes a secret from the OS keychain (keychain class).
func (d *Desktop) DeleteSecret(name string) error {
	_, err := bounded(d, "delete secret", d.deadlines.keychain, func() (struct{}, error) {
		return struct{}{}, d.ctrl.DeleteSecret(name)
	})
	return err
}

// SecretPresence is CheckSecretPresence's outcome: PRESENCE booleans only —
// no field can carry a value, by construction (the ADR-0024 frame's
// discipline applied to secrets).
type SecretPresence struct {
	InEnv      bool `json:"inEnv"`
	InKeychain bool `json:"inKeychain"`
}

// secretNamePattern bounds CheckSecretPresence to plausible channel/model
// secret variable NAMES (upper snake case, ending in _TOKEN/_KEY/_SECRET) so
// the binding cannot be turned into a general env-var presence oracle over
// AWS_*, GITHUB_TOKEN, etc. under a hypothetical XSS (review finding). It is
// a shape gate, not an allowlist of specific vars — the wizard suggests names
// the user can edit, so an exact list would be wrong.
var secretNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_(TOKEN|KEY|SECRET)$`)

// CheckSecretPresence resolves whether the named variable is present in the
// ENVIRONMENT and/or the OS keychain — the wizard's "Comprobar entorno · sin
// leer su valor" (SP6c). Keychain deadline class: a Secret Service unlock
// prompt is a legitimate slow path. The name is shape-gated so this cannot
// enumerate arbitrary environment variables.
func (d *Desktop) CheckSecretPresence(name string) (SecretPresence, error) {
	if !secretNamePattern.MatchString(name) {
		return SecretPresence{}, fmt.Errorf("shell: %q is not a channel/model secret variable name", name)
	}
	return bounded(d, "check secret presence", d.deadlines.keychain, func() (SecretPresence, error) {
		p := SecretPresence{InEnv: os.Getenv(name) != ""}
		inKeychain, err := d.ctrl.SecretInKeychain(name)
		if err != nil {
			return SecretPresence{}, err
		}
		p.InKeychain = inKeychain
		return p, nil
	})
}

// OllamaCheck is CheckOllama's outcome: reachability is a RESULT the
// onboarding paints, never a Go error.
type OllamaCheck struct {
	Reachable bool   `json:"reachable"`
	Detail    string `json:"detail"`
}

// CheckOllama probes {baseURL}/api/tags with the check deadline (onboarding
// step 1). Failures come back as an honest unreachable outcome with detail.
func (d *Desktop) CheckOllama(baseURL string) OllamaCheck {
	ctx, cancel := context.WithTimeout(context.Background(), d.deadlines.check)
	defer cancel()
	url := strings.TrimSuffix(baseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return OllamaCheck{Detail: "invalid base URL"}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return OllamaCheck{Detail: fmt.Sprintf("not reachable: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain a bounded slice so the keep-alive connection is reusable.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		return OllamaCheck{Detail: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}
	return OllamaCheck{Reachable: true, Detail: "ollama is responding"}
}

// Version reports the build-stamped version (the logo/version row).
func (d *Desktop) Version() string { return d.version }
