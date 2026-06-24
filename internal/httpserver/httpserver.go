// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package httpserver is a small, general HTTP server with a Start/Shutdown
// lifecycle the app drives (ADR-0020 §4). It is deliberately NOT tied to
// observability: it owns a generic mux and a built-in /healthz liveness route,
// and exposes Handle so callers mount their own routes (the metrics handler
// today, the Stage 13 control API tomorrow, on the same server). It imports no
// Prometheus types — the metrics handler is passed in as a plain http.Handler.
//
// Lifecycle ownership (ADR-0008 / ADR-0020 §4): App.Run starts it FIRST (so
// /healthz is up before channels connect) and App.Shutdown stops it LAST (so
// /metrics stays observable across the whole drain). Start binds the listener
// synchronously, so a bind failure is a loud boot error, then serves in a
// background goroutine.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// readHeaderTimeout bounds how long a client may take to send request headers,
// closing the Slowloris hole a zero-value http.Server leaves open (gosec G112).
const readHeaderTimeout = 10 * time.Second

// Server wraps an http.Server over a mux with a built-in /healthz route.
// Construct with New; the zero value is not usable.
type Server struct {
	srv    *http.Server
	mux    *http.ServeMux
	addr   string
	logger *slog.Logger

	// mu guards boundAddr, which Start writes and Addr reads, possibly from
	// different goroutines (App.Run starts the server in its own goroutine while
	// a caller may read Addr concurrently).
	mu        sync.RWMutex
	boundAddr string
}

// New builds a Server that will bind addr when Started. The /healthz liveness
// route is registered immediately; callers add more routes with Handle before
// Start.
func New(addr string, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:    mux,
		addr:   addr,
		logger: logger,
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: readHeaderTimeout,
		},
	}
	mux.HandleFunc("/healthz", s.healthz)
	return s
}

// Handle mounts h at pattern. Call before Start; the mux is not safe to mutate
// once serving has begun.
func (s *Server) Handle(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

// Addr returns the actual bound address (host:port), valid after a successful
// Start. With a ":0" port this is how a caller (or a test) learns the chosen
// port. Returns "" before Start. Safe for concurrent use.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.boundAddr
}

// healthz is the liveness handler: 200 while the process is serving. It is
// deliberately decoupled from provider/brain health — a downed optional provider
// is not fatal (ADR-0014 §3), so liveness must not flip red over it.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}

// Start binds the listener synchronously (so a bind failure is returned, not
// lost in a goroutine) and then serves in the background. The ctx is accepted
// for symmetry with the other lifecycle components and future cancellation
// needs; serving stops via Shutdown.
func (s *Server) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("httpserver: listen %q: %w", s.addr, err)
	}
	bound := ln.Addr().String()
	s.mu.Lock()
	s.boundAddr = bound
	s.mu.Unlock()
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("httpserver: serve stopped unexpectedly", "addr", bound, "error", err)
		}
	}()
	return nil
}

// Shutdown gracefully stops the server, draining in-flight requests within ctx.
// Safe to call even if Start failed (Shutdown on an unstarted http.Server is a
// no-op returning nil).
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
