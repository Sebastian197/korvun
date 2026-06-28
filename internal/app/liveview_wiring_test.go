// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// TestBuild_busAndLiveViewWiredWhenObservabilityOn asserts the bus wakes and its
// SSE consumer is built when observability is on (the default) — the producer is
// wired together with its consumer (ADR-0024).
func TestBuild_busAndLiveViewWiredWhenObservabilityOn(t *testing.T) {
	app, err := Build(cfgWith(ollamaBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	if app.eventBus == nil {
		t.Error("eventBus is nil with observability on; the router hook stays dormant")
	}
	if app.liveView == nil {
		t.Error("liveView is nil with observability on; the bus has no consumer")
	}
}

// TestBuild_busDormantWhenObservabilityOff asserts that with observability off
// there is no admin server, so no SSE consumer and no bus — the hook stays
// dormant at zero cost (ADR-0023, the no-producer-without-a-consumer discipline).
func TestBuild_busDormantWhenObservabilityOff(t *testing.T) {
	disabled := false
	cfg := cfgWith(ollamaBrain())
	cfg.Observability = &config.ObservabilityConfig{Enabled: &disabled}

	app, err := Build(cfg,
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })

	if app.eventBus != nil {
		t.Error("eventBus is non-nil with observability off; it has no consumer")
	}
	if app.liveView != nil {
		t.Error("liveView is non-nil with observability off")
	}
}

// TestRouterErrorToEvent asserts the kind→event mapping (ADR-0023 §3): a brain
// failure is HandleFailed; every other kind is MessageDropped; the fields carry.
func TestRouterErrorToEvent(t *testing.T) {
	env := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u1"})
	cases := []struct {
		kind router.ErrorKind
		want bus.EventType
	}{
		{router.ErrKindHandle, bus.HandleFailed},
		{router.ErrKindInboundDispatch, bus.MessageDropped},
		{router.ErrKindOutboundSaturated, bus.MessageDropped},
		{router.ErrKindSend, bus.MessageDropped},
	}
	for _, tc := range cases {
		ev := routerErrorToEvent(router.RouterError{
			Kind:     tc.kind,
			Brain:    "default",
			Channel:  "telegram",
			Envelope: env,
			Err:      errors.New("boom"),
		})
		if ev.Type != tc.want {
			t.Errorf("kind %v -> %v, want %v", tc.kind, ev.Type, tc.want)
		}
		if ev.Brain != "default" || ev.Channel != "telegram" || ev.Envelope != env {
			t.Errorf("kind %v: fields not carried through: %+v", tc.kind, ev)
		}
	}
}

// TestOnRouterError_publishesToBus asserts the app-level funnel publishes the
// matching failure event to a live bus (ADR-0023: drops/failures ride this
// funnel, not an in-router hook).
func TestOnRouterError_publishesToBus(t *testing.T) {
	b := bus.New()
	defer b.Close()

	got := make(chan bus.Event, 1)
	unsub := b.Subscribe(bus.HandleFailed, func(ev bus.Event) { got <- ev })
	defer unsub()

	onRouterError(slog.New(slog.DiscardHandler), &routerErrRecorder{}, b, router.RouterError{
		Kind:  router.ErrKindHandle,
		Brain: "default",
		Err:   errors.New("brain boom"),
	})

	select {
	case ev := <-got:
		if ev.Type != bus.HandleFailed || ev.Brain != "default" {
			t.Errorf("published %+v, want HandleFailed for brain default", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onRouterError did not publish to the bus")
	}
}

// TestLiveView_endToEnd_inboundProducesSSEEvent is the headline integration: a
// real inbound message routed through the running app wakes the router hook
// (publishReceived -> eventBus) and the SSE live-view delivers a message_received
// frame to a connected client — proving the bus has a real consumer end-to-end.
// It also asserts the existing surfaces (/healthz, /metrics, /api/brains, /ui)
// stay intact and the bus drop metric is exposed.
func TestLiveView_endToEnd_inboundProducesSSEEvent(t *testing.T) {
	fc := newFakeChannel("telegram")
	app, err := Build(cfgWith(ollamaBrain()),
		WithLogger(slog.New(slog.DiscardHandler)),
		withChannelFactory(okFactory(fc)))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- app.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-runDone
		sc, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = app.Shutdown(sc)
	})

	base := func() string { return "http://" + app.adminServer.Addr() }
	if !waitFor(t, func() bool {
		if app.adminServer.Addr() == "" {
			return false
		}
		code, _ := tryGet(base() + "/healthz")
		return code == 200
	}) {
		t.Fatal("admin server never became healthy")
	}

	// Existing surfaces intact, plus the new /ui and the bus drop metric.
	for _, path := range []string{"/healthz", "/metrics", "/api/brains", "/api/channels", "/ui/"} {
		if code, _ := tryGet(base() + path); code != 200 {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
	}
	if _, body := tryGet(base() + "/metrics"); !strings.Contains(body, "korvun_bus_events_dropped_total") {
		t.Error("/metrics missing korvun_bus_events_dropped_total")
	}

	// Connect an SSE client.
	req, _ := http.NewRequest(http.MethodGet, base()+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("SSE status = %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)

	// Feed a real inbound through the fake channel until a message_received frame
	// arrives (republishing absorbs the race between the SSE subscribe and the
	// router pump picking up the inbound).
	frames := make(chan string, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "data: ") {
				frames <- strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				return
			}
		}
	}()

	deadline := time.After(3 * time.Second)
	feed := time.NewTicker(50 * time.Millisecond)
	defer feed.Stop()
	for {
		select {
		case frame := <-frames:
			if !strings.Contains(frame, `"type":"message_received"`) {
				t.Errorf("first frame is not message_received: %s", frame)
			}
			if !strings.Contains(frame, `"channel":"telegram"`) {
				t.Errorf("frame missing channel: %s", frame)
			}
			return
		case <-feed.C:
			select {
			case fc.inbound <- inboundText("telegram", "c1", "hola"):
			default:
			}
		case <-deadline:
			t.Fatal("no message_received SSE frame after feeding a real inbound")
		}
	}
}
