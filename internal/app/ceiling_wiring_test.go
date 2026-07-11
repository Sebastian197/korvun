// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// This file pins the WIRING half of ADR-0031 sub-phase 1 (Decision 2): the app
// derives the router ceiling from config and installs it via
// WithBrainHandlerTimeout, replacing the fixed 5s default; an explicit override
// below the derived ceiling fails loud at boot; and, end-to-end, a model that
// always expires is cut by the derived ceiling (F5), not by the old default.
//
// RED note: it references config fields (RequestTimeout, BrainHandlerTimeout),
// the app derivation (deriveBrainCeiling, defaultCeilingMargin), the new boot
// sentinel (ErrCeilingOverrideTooLow), and the router accessor
// (BrainHandlerTimeout) — none of which exist yet, so the package fails to
// build. That is the red for the wiring; F5's behavioral red (a hang cut at ~5s
// today instead of the tight derived ceiling) surfaces once it compiles.

func boolPtr(b bool) *bool { return &b }

// shutdownApp drains a built App within a bounded window (no shutdown helper
// exists in package app; the router_test one is in another package).
func shutdownApp(t *testing.T, a *App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// fanoutCfg builds a one-channel, one-fanout-brain, one-route config over a
// single Ollama model with the given per-model request_timeout and base URL,
// observability off (no admin port to collide on), and an optional top-level
// brain_handler_timeout override.
func fanoutCfg(baseURL, requestTimeout, override string) *config.Config {
	return &config.Config{
		BrainHandlerTimeout: override,
		Observability:       &config.ObservabilityConfig{Enabled: boolPtr(false)},
		Channels:            []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{{
			Name:        "default",
			Sensitivity: "public",
			Dispatch:    "fanout",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: baseURL, RequestTimeout: requestTimeout},
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "default"}},
	}
}

// TestBuild_installsDerivedCeiling_replacingDefault pins the core wiring: after
// Build, the router runs on the ceiling DERIVED from the brain's per-model
// timeout and dispatch shape — not on router.DefaultBrainHandlerTimeout (the 5s
// guillotine that cut Chano's first message). ADR-0031 Decision 2.
func TestBuild_installsDerivedCeiling_replacingDefault(t *testing.T) {
	a, err := Build(fanoutCfg("", "200ms", ""), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { shutdownApp(t, a) })

	// max_retries defaults to 0 => backoffBudget 0, so the fan-out ceiling is
	// just perAttempt + margin.
	want := deriveBrainCeiling(brainCeilingSpec{
		shape:         "fanout",
		perAttempt:    []time.Duration{200 * time.Millisecond},
		backoffBudget: []time.Duration{0},
		margin:        defaultCeilingMargin,
	})
	if got := a.router.BrainHandlerTimeout(); got != want {
		t.Errorf("router BrainHandlerTimeout = %v, want derived %v", got, want)
	}
	if a.router.BrainHandlerTimeout() == router.DefaultBrainHandlerTimeout {
		t.Errorf("router still on the %v default — the derived ceiling was not installed", router.DefaultBrainHandlerTimeout)
	}
}

// TestBuild_explicitCeilingOverride pins ADR-0031 Decision 2's override rule: an
// explicit brain_handler_timeout is honored ONLY if it is >= the derived
// ceiling; below it, Build fails loud (never silently guillotine a slow model).
func TestBuild_explicitCeilingOverride(t *testing.T) {
	t.Run("below derived fails loud", func(t *testing.T) {
		// Derived is ~200ms + margin; 50ms is below it.
		_, err := Build(fanoutCfg("", "200ms", "50ms"), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if !errors.Is(err, ErrCeilingOverrideTooLow) {
			t.Fatalf("err = %v, want ErrCeilingOverrideTooLow", err)
		}
	})

	t.Run("at or above derived is honored", func(t *testing.T) {
		a, err := Build(fanoutCfg("", "200ms", "5s"), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		t.Cleanup(func() { shutdownApp(t, a) })
		if got := a.router.BrainHandlerTimeout(); got != 5*time.Second {
			t.Errorf("router BrainHandlerTimeout = %v, want the 5s override", got)
		}
	})
}

// recordingChannel is a fake channel that signals when the router sends a reply
// through it, so a test can measure how long Handle took end-to-end. It embeds
// *fakeChannel (inheriting Name/Manifest/Receive/Start/Stop and the inbound
// channel) and overrides only Send.
type recordingChannel struct {
	*fakeChannel
	sent chan time.Time
}

func newRecordingChannel(name string) *recordingChannel {
	return &recordingChannel{fakeChannel: newFakeChannel(name), sent: make(chan time.Time, 4)}
}

func (c *recordingChannel) Send(context.Context, *envelope.Envelope) error {
	select {
	case c.sent <- time.Now():
	default:
	}
	return nil
}

// TestHandle_boundedByDerivedCeiling is the F5 integration test: a model that
// always expires (an httptest server that never answers within the window) is
// cut by the DERIVED ceiling, not by the 5s default and not by an unbounded
// per-attempt wait. With request_timeout 200ms the derived ceiling is a few
// hundred ms, so the all-failed fallback reply is sent well under a second.
//
// RED: today the app installs the 5s default, so the reply is sent at ~5s;
// GREEN installs the tight derived ceiling and the reply arrives at ~ceiling.
func TestHandle_boundedByDerivedCeiling(t *testing.T) {
	// A server that never answers within any sane per-attempt window: it holds
	// the request until either the client's ctx is cancelled (the ceiling firing,
	// the behavior under test) or the test releases it at teardown. The explicit
	// release channel keeps srv.Close from blocking on a handler still parked on a
	// connection the client has not yet fully torn down.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	ch := newRecordingChannel("telegram")
	a, err := Build(fanoutCfg(srv.URL, "200ms", ""), withChannelFactory(okFactory(ch)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- a.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-runDone
		shutdownApp(t, a)
	})

	// Wait for the channel to start, then inject one inbound message.
	deadline := time.Now().Add(time.Second)
	for !ch.isStarted() && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if !ch.isStarted() {
		t.Fatal("Run did not start the channel")
	}

	start := time.Now()
	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u-1"})
	in.AddText("hello")
	in.Meta[router.MetaConversationID] = "c-1"
	ch.inbound <- in

	select {
	case at := <-ch.sent:
		if elapsed := at.Sub(start); elapsed >= 2*time.Second {
			t.Errorf("reply sent after %v — Handle is not bounded by the derived ceiling (still on the 5s default?)", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("no reply sent within 8s")
	}
}
