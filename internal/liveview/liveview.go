// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package liveview is Korvun's read-only live-view: a Server-Sent Events stream
// of the message pipeline's lifecycle (GET /api/events) plus a minimal embedded
// vanilla HTML/JS UI (/ui), both mounted on the existing admin httpserver
// (ADR-0024, Stage 14 Phase 1b). It is the bus's FIRST real subscriber — the
// consumer that validates ADR-0023's event bus end-to-end.
//
// It is a leaf: it depends only on the standard library, internal/bus, and
// internal/envelope (for the non-secret descriptor it emits). It never touches
// the router or a brain, and it never mutates state — read-only keeps the
// loopback-no-auth security calculus of /metrics and /api intact (ADR-0024 §3).
//
// # Two binding invariants
//
//   - SECRET-FREE FRAMES (ADR-0024 §1). A frame serializes only NON-secret
//     fields of an Event — type, channel, brain, a server timestamp, and a
//     minimal envelope descriptor (id, direction). It NEVER serializes the
//     Envelope's message content, its Meta map, or the Event's Err (which may
//     carry provider error text). The frame type below has no field that can
//     reach a secret, so this holds by construction and is test-asserted.
//
//   - TEARDOWN-SAFE AGAINST F2 (ADR-0023). unsubscribe is not synchronous with
//     handler quiescence: a buffered event can fire the bus Handler once more
//     after unsubscribe, and an in-flight Handler runs to completion. The SSE
//     Handler here writes ONLY to an in-process per-connection channel (buf),
//     never to the ResponseWriter. The ResponseWriter is touched solely by the
//     request goroutine's serve loop; once that loop returns (client disconnect,
//     Close, or a failed write) no further write to the torn-down writer can
//     occur, even if the bus fires the Handler again. This separation is what
//     neutralizes the F2 foot-gun.
package liveview

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
)

//go:embed ui
var uiFS embed.FS

// streamedTypes is every lifecycle EventType the live-view streams. The bus
// Subscribe is per-type, so the handler subscribes once per type with the same
// per-connection sink.
var streamedTypes = []bus.EventType{
	bus.MessageReceived,
	bus.ReplySent,
	bus.MessageDropped,
	bus.HandleFailed,
}

// DefaultConnBuffer is the per-connection buffer depth. A client that falls this
// far behind starts dropping (counted via DroppedCount), never blocking the bus
// or the hot path — mirroring the bus's own best-effort contract (ADR-0024 §1).
const DefaultConnBuffer = 64

// Subscriber is the read side of the bus the live-view depends on. *bus.InMemoryBus
// satisfies it; kept narrow so liveview does not depend on the publish side and
// tests can inject a fake.
type Subscriber interface {
	Subscribe(t bus.EventType, h bus.Handler) (unsubscribe func())
}

// Mounter is the subset of *httpserver.Server (and of *http.ServeMux) the
// live-view needs to register its routes. An interface keeps liveview a leaf that
// does not import internal/httpserver.
type Mounter interface {
	Handle(pattern string, h http.Handler)
}

// LiveView serves the SSE live-event stream and the embedded UI. Construct with
// New; the zero value is not usable.
type LiveView struct {
	bus        Subscriber
	connBuffer int
	now        func() time.Time
	logger     *slog.Logger

	dropped atomic.Uint64

	// done is closed by Close to unblock in-flight SSE serve loops at shutdown,
	// so the admin server can drain its long-lived streaming connections.
	done      chan struct{}
	closeOnce sync.Once
}

// Option configures New.
type Option func(*LiveView)

// WithConnBuffer sets the per-connection buffer depth (default DefaultConnBuffer).
// A non-positive n is ignored.
func WithConnBuffer(n int) Option {
	return func(lv *LiveView) {
		if n > 0 {
			lv.connBuffer = n
		}
	}
}

// WithLogger sets the structured logger. A nil logger is ignored.
func WithLogger(l *slog.Logger) Option {
	return func(lv *LiveView) {
		if l != nil {
			lv.logger = l
		}
	}
}

// WithNow overrides the clock used to stamp frame timestamps (tests inject a
// fixed clock). A nil fn is ignored.
func WithNow(fn func() time.Time) Option {
	return func(lv *LiveView) {
		if fn != nil {
			lv.now = fn
		}
	}
}

// New constructs a LiveView over sub.
func New(sub Subscriber, opts ...Option) *LiveView {
	lv := &LiveView{
		bus:        sub,
		connBuffer: DefaultConnBuffer,
		now:        time.Now,
		logger:     slog.Default(),
		done:       make(chan struct{}),
	}
	for _, o := range opts {
		o(lv)
	}
	return lv
}

// Register mounts the live-view routes on m: GET /api/events (SSE) and the
// embedded UI under /ui/. Call before the server starts — the mux is not safe to
// mutate once serving.
func (lv *LiveView) Register(m Mounter) {
	m.Handle("GET /api/events", lv.eventsHandler())

	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		// uiFS is a compile-time embed of a present directory; a Sub failure would
		// be a build-time impossibility. Guard defensively rather than panic.
		lv.logger.Error("liveview: embedded UI unavailable", "error", err)
		return
	}
	m.Handle("/ui/", http.StripPrefix("/ui/", http.FileServerFS(sub)))
}

// DroppedCount returns the cumulative number of events dropped because a
// connection's buffer was full (a slow client). It is the live-view's saturation
// signal, exposed by the app as a pull metric (ADR-0024 §1), mirroring
// bus.DroppedCount and telegram.DroppedCount.
func (lv *LiveView) DroppedCount() uint64 { return lv.dropped.Load() }

// Close unblocks every in-flight SSE serve loop (so the admin server can drain
// its streaming connections at shutdown) and is idempotent. It does not close
// the bus — the app owns the bus lifecycle (see app.Shutdown ordering).
func (lv *LiveView) Close() {
	lv.closeOnce.Do(func() { close(lv.done) })
}

// frame is the SECRET-FREE wire shape of one event (ADR-0024 §1). It has no
// field that can carry message content, Meta, or an error string — secret-free
// by construction. Direction/EnvelopeID come from the wrapped Envelope when one
// is present; Timestamp is the server's receive time.
type frame struct {
	Type       string `json:"type"`
	Channel    string `json:"channel,omitempty"`
	Brain      string `json:"brain,omitempty"`
	Timestamp  string `json:"timestamp"`
	EnvelopeID string `json:"envelope_id,omitempty"`
	Direction  string `json:"direction,omitempty"`
}

// toFrame projects an Event onto its secret-free frame. It reads ONLY non-secret
// fields; it never touches Envelope.Parts (content), Envelope.Meta, or Event.Err.
func (lv *LiveView) toFrame(ev bus.Event) frame {
	f := frame{
		Type:      ev.Type.String(),
		Channel:   ev.Channel,
		Brain:     ev.Brain,
		Timestamp: lv.now().UTC().Format(time.RFC3339Nano),
	}
	if ev.Envelope != nil {
		f.EnvelopeID = ev.Envelope.ID
		f.Direction = ev.Envelope.Direction.String()
	}
	return f
}

// eventsHandler returns the SSE handler. Per connection it subscribes to every
// streamed type with a sink that feeds a bounded per-connection buffer, then
// serves frames until the client disconnects, Close fires, or a write fails.
func (lv *LiveView) eventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Per-connection bounded buffer. The bus Handler writes ONLY here (never to
		// w), which is what makes teardown F2-safe: a Handler firing after
		// unsubscribe does a non-blocking send to buf and returns; w is touched
		// solely by the serve loop below.
		buf := make(chan bus.Event, lv.connBuffer)
		sink := func(ev bus.Event) {
			select {
			case buf <- ev:
			default:
				lv.dropped.Add(1) // slow client: drop, count, never block the bus
			}
		}

		// Subscribe BEFORE writing headers so a publish racing the client's first
		// read is not lost: by the time the response headers reach the client the
		// subscription is already live.
		unsubs := make([]func(), 0, len(streamedTypes))
		for _, t := range streamedTypes {
			unsubs = append(unsubs, lv.bus.Subscribe(t, sink))
		}
		defer func() {
			for _, u := range unsubs {
				u()
			}
		}()

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done(): // client disconnected
				return
			case <-lv.done: // server shutting down
				return
			case ev := <-buf:
				if !lv.writeFrame(w, ev) {
					return // write failed: client gone, tear down
				}
				flusher.Flush()
			}
		}
	}
}

// writeFrame marshals the secret-free frame and writes one SSE `data:` record.
// It returns false on a marshal or write error so the serve loop tears the
// connection down. A marshal error is logged (it would be a bug in the fixed
// frame shape), never a secret.
func (lv *LiveView) writeFrame(w http.ResponseWriter, ev bus.Event) bool {
	data, err := json.Marshal(lv.toFrame(ev))
	if err != nil {
		lv.logger.Error("liveview: frame marshal failed", "type", ev.Type.String(), "error", err)
		return true // skip this frame, keep the stream alive
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	return true
}
