// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Red for the 2026-08-08 Windows rehearsal red (round 1, quality:
// TestNoSecretValuesLeakToLogsOrErrors): on a clean shutdown the parent
// context cancels, App.Serve unwinds with nil, and serve()'s select has
// TWO ready cases — serveErr (carrying nil) and ctx.Done(). When the
// select drew serveErr, a clean stop was classified reasonAppFailed with
// a nil error ("supervisor: running app failed: %!w(<nil>)"). The
// classification must not depend on select luck: a cancelled context is
// a shutdown, whichever channel the select happens to see first.
package supervisor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// spontaneousApp returns nil from Serve immediately, without any shutdown
// being requested — the "Serve gave up cleanly on its own" corner.
type spontaneousApp struct{}

func (spontaneousApp) Start(context.Context) error    { return nil }
func (spontaneousApp) Serve(context.Context) error    { return nil }
func (spontaneousApp) Shutdown(context.Context) error { return nil }

// The deterministic red: Serve returning nil on its own (context alive) is
// an app failure — but it must surface as a REAL error, never fmt's
// "%!w(<nil>)" artifact of wrapping a nil.
func TestSupervisor_spontaneousNilServeIsARealError(t *testing.T) {
	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(staticBuild(spontaneousApp{})))
	err := sup.Run(context.Background())
	if err == nil {
		t.Fatal("Run with a Serve that quit on its own returned nil, want an error")
	}
	if strings.Contains(err.Error(), "%!") {
		t.Fatalf("Run error wraps a nil (%q) — want a real error, never a fmt artifact", err)
	}
}

// The scheduler-dependent half, kept as a regression guard: a cancelled
// context is a shutdown whichever channel the select sees first. The old
// draw does NOT reproduce locally on demand (the Serve goroutine must win
// the select — a scheduler accident; CI Windows run 31272381273 is the
// recorded red); the ctx.Err() guard makes both outcomes equivalent.
func TestSupervisor_cleanCancelNeverClassifiedAppFailure(t *testing.T) {
	// Pre-cancelled parent: the shutdown must be classified as clean on
	// every draw.
	for i := 0; i < 30; i++ {
		log := &eventLog{}
		a := newFakeApp("A", log)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sup := supervisor.New(&config.Config{}, supervisor.WithBuild(staticBuild(a)))
		if err := sup.Run(ctx); err != nil {
			t.Fatalf("iteration %d: run under an already-cancelled context returned %v, want nil (clean shutdown)", i, err)
		}
	}

	// The ordinary stop too: cancel once the app is serving.
	for i := 0; i < 30; i++ {
		log := &eventLog{}
		a := newFakeApp("A", log)
		ctx, cancel := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		sup := supervisor.New(&config.Config{}, supervisor.WithBuild(staticBuild(a)))
		go func() { runDone <- sup.Run(ctx) }()
		<-a.started
		cancel()
		if err := <-runDone; err != nil {
			t.Fatalf("iteration %d: clean stop returned %v, want nil", i, err)
		}
	}
}
