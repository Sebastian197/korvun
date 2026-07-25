// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command e2e-harness serves the BUILT desktop chrome and the SP4 same-origin
// admin proxy from one loopback origin, over a REAL no-network core (the SP5
// channel-less first-run template), mirroring the Wails AssetServer semantics
// — assets first, handler on miss — so Playwright drives the real pipeline
// without a WebView (SP6 spec: the per-cut screenshot medium; the native
// WKWebView ride is SP8's hardware validation). Plain Go, no Wails import:
// it compiles in the default suite on every OS.
//
// Usage: e2e-harness [-addr 127.0.0.1:43117] [-dist cmd/korvun-desktop/frontend/dist]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Sebastian197/korvun/internal/shell"
)

func main() {
	if err := run(); err != nil {
		slog.Error("harness failed", "error", err.Error())
		os.Exit(1)
	}
}

// run keeps every deferred cleanup on the exit path (no os.Exit skipping the
// temp-dir removal).
func run() error {
	addr := flag.String("addr", "127.0.0.1:43117", "loopback address to serve on")
	dist := flag.String("dist", filepath.Join("cmd", "korvun-desktop", "frontend", "dist"),
		"path to the built chrome bundle")
	autostart := flag.Bool("start", true, "start the core (template first-run) on boot")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctrl := shell.New(shell.WithLogger(logger))
	if *autostart {
		dir, err := os.MkdirTemp("", "korvun-harness-*")
		if err != nil {
			return fmt.Errorf("temp dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		cfgPath := filepath.Join(dir, "korvun.json")
		if _, err := shell.EnsureDefaultConfig(cfgPath); err != nil {
			return fmt.Errorf("ensure config: %w", err)
		}
		if err := ctrl.LoadConfig(cfgPath); err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := ctrl.Start(ctx); err != nil {
			cancel()
			return fmt.Errorf("start core: %w", err)
		}
		cancel()
	}

	proxy := ctrl.ProxyHandler()
	files := http.FileServer(http.Dir(*dist))
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assets first, handler on miss — the AssetServer's semantics. The
		// chrome is a single-page app: "/" is index.html; anything that is
		// not a real file under dist/ falls through to the proxy. The path
		// is cleaned BEFORE the stat so ../ can never probe outside dist
		// (http.FileServer would contain the serve anyway; the routing
		// probe must be equally contained).
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p == "" {
			files.ServeHTTP(w, r)
			return
		}
		if info, err := os.Stat(filepath.Join(*dist, filepath.FromSlash(p))); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("harness serving", "addr", "http://"+*addr)
		serveErr <- srv.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-sig:
	}
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	_ = srv.Shutdown(sctx)
	if ctrl.Status().Running {
		// Its own budget: a slow HTTP drain must not starve the core's stop.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = ctrl.Stop(stopCtx)
	}
	return nil
}
