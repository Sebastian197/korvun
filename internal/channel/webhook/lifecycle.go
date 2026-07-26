// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Default listen address and inbound path for a webhook channel, mirrored here so
// the webhook package does not import internal/config (layering). They MUST match
// config.DefaultWebhookBind and config.DefaultWebhookPath (ADR-0038 §1).
const (
	defaultBind = "127.0.0.1:8090" // == config.DefaultWebhookBind (ADR-0038 §1)
	defaultPath = "/webhook"       // == config.DefaultWebhookPath (ADR-0038 §1)
)

// readHeaderTimeout bounds the header read on the inbound server, a Slowloris guard
// mirroring the Telegram webhook adapter (ADR-0008).
const readHeaderTimeout = 5 * time.Second

// ErrAlreadyStarted is returned by Start when the adapter's server is already
// running; the running server is left untouched.
var ErrAlreadyStarted = errors.New("webhook: already started")

// Options configures a wired webhook channel (ADR-0038 §2). Bind is the listen
// address (empty → defaultBind); Path is the inbound POST path (empty → defaultPath);
// Secret is the inbound shared secret VALUE, already resolved from its env var by the
// caller (env-only, ADR-0010 — never read here from a file); OutboundURL is where
// replies are POSTed (copied into the adapter's single outboundURL field, the one
// Send uses); OutboundToken is the OPTIONAL outbound downstream Bearer secret value;
// Mapping is the field mapping. The inbound-auth ENFORCEMENT of Secret lands in SP3;
// SP2 only carries it.
type Options struct {
	Bind          string
	Path          string
	Secret        string
	OutboundURL   string
	OutboundToken string
	Mapping       FieldMapping
}

// NewWithOptions builds a wired webhook Adapter: the Stage-2 core (same inbound
// buffer and HTTP client, via New) plus the SP2 lifecycle options (ADR-0038 §2). The
// outbound URL keeps a SINGLE source of truth — opts.OutboundURL is copied into the
// existing outboundURL field through SetOutboundURL, which stays valid for tests and
// late binding (FR-COMPAT-1).
func NewWithOptions(name string, opts Options) *Adapter {
	a := New(name, opts.Mapping)
	a.SetOutboundURL(opts.OutboundURL)
	a.bind = opts.Bind
	a.path = opts.Path
	a.secret = opts.Secret
	a.outboundToken = opts.OutboundToken
	return a
}

// effectiveBind returns the configured bind or the loopback default (ADR-0038 §1).
func (a *Adapter) effectiveBind() string {
	if a.bind != "" {
		return a.bind
	}
	return defaultBind
}

// effectivePath returns the configured path or the default (ADR-0038 §1).
func (a *Adapter) effectivePath() string {
	if a.path != "" {
		return a.path
	}
	return defaultPath
}

// Start brings the adapter's own HTTP server up (ADR-0038 §2), all-or-nothing: it
// binds a listener on the effective address (a bind failure returns a named, wrapped
// error and leaves the adapter un-started), records the real bound address (which
// matters under the ephemeral-port policy), mounts the EXISTING InboundHandler at the
// effective path plus a running-gated /healthz, and serves in a background goroutine.
// A serve error other than http.ErrServerClosed is logged (ADR-0008 §4a) since Start
// has already returned by then. Start is idempotent on an already-started adapter
// (returns ErrAlreadyStarted without disturbing the running server).
func (a *Adapter) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return ErrAlreadyStarted
	}

	ln, err := net.Listen("tcp", a.effectiveBind())
	if err != nil {
		return fmt.Errorf("webhook: listen on %q: %w", a.effectiveBind(), err)
	}

	mux := http.NewServeMux()
	mux.Handle(a.effectivePath(), a.InboundHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		running := a.started
		a.mu.Unlock()
		if !running {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	a.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	a.boundAddr = ln.Addr().String()
	a.started = true

	go func() {
		serveErr := a.server.Serve(ln)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Default().WarnContext(ctx,
				"webhook: HTTP server stopped with error",
				"channel", a.name, "error", serveErr.Error())
		}
	}()
	return nil
}

// Stop tears the server down, bounded by ctx, and is idempotent (ADR-0038 §2). An
// adapter that never started (including after a failed Start) returns nil immediately.
// A started adapter is shut down gracefully; if Shutdown errors (an unclean drain, or
// ctx already cancelled/expired), a Close() backstop guarantees the server is ALWAYS
// closed, the degradation is logged, and Stop still returns nil. The inbound channel
// is closed EXACTLY once (sync.Once) so the router pump sees a single clean
// end-of-stream (ADR-0008 ordering: channel stopped before router).
func (a *Adapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		// Never started (or already stopped): a safe no-op. Nothing to close; the
		// inbound channel is only ever closed for an adapter that ran.
		return nil
	}
	a.started = false
	server := a.server
	a.mu.Unlock()

	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
		slog.Default().WarnContext(ctx,
			"webhook: graceful shutdown degraded to Close",
			"channel", a.name, "error", err.Error())
	}

	a.stopOnce.Do(func() { close(a.inbound) })
	return nil
}

// BoundAddr returns the real address the server bound to (host:port, the actual port
// even under an ephemeral :0 bind), or "" until a successful Start (ADR-0038 §2).
func (a *Adapter) BoundAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.boundAddr
}
