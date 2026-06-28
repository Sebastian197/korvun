// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package liveview

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// fixedNow is a deterministic clock for frame-timestamp assertions.
func fixedNow() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }

// readFrame reads one `data: ...\n\n` SSE frame from r, returning the JSON
// payload (without the "data: " prefix). It blocks until a complete frame or an
// error; tests wrap it in a timeout.
func readFrame(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("readFrame: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
}

// connectSSE opens an /api/events stream against srv and returns the response
// plus a buffered reader positioned past the response headers. The subscription
// is active by the time the headers arrive (the handler subscribes BEFORE
// writing them), so a publish after this returns is delivered.
func connectSSE(t *testing.T, srv *httptest.Server) (*http.Response, *bufio.Reader) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("SSE Content-Type = %q, want text/event-stream", ct)
	}
	return resp, bufio.NewReader(resp.Body)
}

func newTestServer(t *testing.T, lv *LiveView) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	lv.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSSE_deliversRealPublish is the end-to-end proof: a real bus publish is
// delivered to a connected SSE client as a frame. This is the bus's first real
// subscriber validating it (ADR-0024).
func TestSSE_deliversRealPublish(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	resp, br := connectSSE(t, srv)
	defer func() { _ = resp.Body.Close() }()

	b.Publish(context.Background(), bus.Event{
		Type:    bus.MessageReceived,
		Channel: "telegram",
		Brain:   "default",
	})

	got := make(chan string, 1)
	go func() { got <- readFrame(t, br) }()
	select {
	case frame := <-got:
		for _, want := range []string{`"type":"message_received"`, `"channel":"telegram"`, `"brain":"default"`} {
			if !strings.Contains(frame, want) {
				t.Errorf("frame %q missing %q", frame, want)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE frame within 2s after a real publish")
	}
}

// TestSSE_frameIsSecretFree asserts the binding invariant (ADR-0024 §1): a frame
// carries only non-secret fields, NEVER the Envelope's message content nor any
// secret-bearing Meta value.
func TestSSE_frameIsSecretFree(t *testing.T) {
	const secretText = "TOPSECRET-MESSAGE-BODY"
	const secretMeta = "meta-value-that-must-never-reach-the-wire"

	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	resp, br := connectSSE(t, srv)
	defer func() { _ = resp.Body.Close() }()

	env := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u1"})
	env.AddText(secretText)
	env.Meta["api_key"] = secretMeta

	b.Publish(context.Background(), bus.Event{
		Type:     bus.ReplySent,
		Channel:  "telegram",
		Brain:    "default",
		Envelope: env,
	})

	got := make(chan string, 1)
	go func() { got <- readFrame(t, br) }()
	select {
	case frame := <-got:
		for _, secret := range []string{secretText, secretMeta, "content", "parts", "meta"} {
			if strings.Contains(frame, secret) {
				t.Errorf("frame leaked %q: %s", secret, frame)
			}
		}
		// It must still carry the non-secret descriptor.
		if !strings.Contains(frame, `"type":"reply_sent"`) {
			t.Errorf("frame missing type: %s", frame)
		}
		if !strings.Contains(frame, env.ID) {
			t.Errorf("frame missing non-secret envelope id: %s", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE frame within 2s")
	}
}

// blockingRW is a ResponseWriter whose Write blocks until release is closed,
// and which flags any Write that happens after closed is set. It supports
// http.Flusher. It models a stalled client (the socket buffer is full) and lets
// the F2 teardown test assert no write occurs after the handler tears down.
type blockingRW struct {
	header  http.Header
	release chan struct{}
	closed  atomic.Bool
	wrote   atomic.Int64
	afterTd atomic.Bool // a Write happened after closed was set (a bug)
}

func newBlockingRW() *blockingRW {
	return &blockingRW{header: make(http.Header), release: make(chan struct{})}
}

func (w *blockingRW) Header() http.Header { return w.header }
func (w *blockingRW) WriteHeader(int)     {}
func (w *blockingRW) Flush()              {}
func (w *blockingRW) Write(p []byte) (int, error) {
	if w.closed.Load() {
		w.afterTd.Store(true)
	}
	w.wrote.Add(1)
	<-w.release // block like a stalled socket
	return len(p), nil
}

// TestSSE_teardownNoWriteAfterClose is the F2 foot-gun guard (ADR-0023 §Bus):
// unsubscribe is NOT synchronous with handler quiescence, so a buffered event
// may fire the bus handler once more after teardown. The SSE design routes that
// handler to an in-process channel ONLY (never the ResponseWriter), so once the
// serve loop returns no further write to the (torn-down) writer can occur. This
// test cancels the request mid-write, lets the handler return, marks the writer
// closed, then publishes more — asserting no write lands after close and nothing
// panics.
func TestSSE_teardownNoWriteAfterClose(t *testing.T) {
	b := bus.New(bus.WithSubscriberBuffer(1))
	defer b.Close()
	lv := New(b, WithNow(fixedNow), WithConnBuffer(1))

	w := newBlockingRW()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		lv.eventsHandler().ServeHTTP(w, req)
		close(done)
	}()

	// Drive one event through to the writer so the serve loop is parked inside
	// Write (the stalled-client moment). Republish until the first frame lands,
	// absorbing the race between this goroutine and the handler's Subscribe.
	if !waitFor(func() bool {
		b.Publish(context.Background(), bus.Event{Type: bus.MessageReceived, Channel: "telegram"})
		return w.wrote.Load() >= 1
	}) {
		t.Fatal("handler never wrote the first frame")
	}

	// Tear down: cancel the request, release the blocked Write so the loop can
	// observe the cancellation and return.
	cancel()
	close(w.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancellation")
	}

	// The writer is now torn down. Any subsequent write is a bug.
	w.closed.Store(true)
	for i := 0; i < 50; i++ {
		b.Publish(context.Background(), bus.Event{Type: bus.MessageReceived, Channel: "telegram"})
	}
	time.Sleep(50 * time.Millisecond)
	if w.afterTd.Load() {
		t.Error("SSE handler wrote to the ResponseWriter AFTER teardown (F2 foot-gun)")
	}
}

// TestSSE_slowClientDropsCounted asserts a slow client (a full per-connection
// buffer) drops events that are counted, never blocking the bus or panicking.
func TestSSE_slowClientDropsCounted(t *testing.T) {
	b := bus.New(bus.WithSubscriberBuffer(1))
	defer b.Close()
	lv := New(b, WithNow(fixedNow), WithConnBuffer(1))

	w := newBlockingRW()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		lv.eventsHandler().ServeHTTP(w, req)
		close(done)
	}()

	// Park the serve loop inside Write, then flood: with the loop blocked and a
	// 1-deep buffer, the surplus events drop at the SSE layer and are counted.
	// Republish until the first frame lands (absorbing the Subscribe race).
	if !waitFor(func() bool {
		b.Publish(context.Background(), bus.Event{Type: bus.MessageReceived, Channel: "telegram"})
		return w.wrote.Load() >= 1
	}) {
		t.Fatal("handler never wrote the first frame")
	}
	for i := 0; i < 100; i++ {
		b.Publish(context.Background(), bus.Event{Type: bus.MessageReceived, Channel: "telegram"})
	}
	if !waitFor(func() bool { return lv.DroppedCount() > 0 }) {
		t.Fatalf("DroppedCount = 0, want > 0 for a stalled client")
	}

	cancel()
	close(w.release)
	<-done
}

// TestSSE_slowClientDoesNotTumbleServer asserts one stalled client does not stop
// the server from serving a second, healthy client.
func TestSSE_slowClientDoesNotTumbleServer(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))
	srv := newTestServer(t, lv)

	// A stalled client that never reads its body.
	slowResp, _ := connectSSE(t, srv)
	defer func() { _ = slowResp.Body.Close() }()

	// A healthy client that does read.
	fastResp, fastBR := connectSSE(t, srv)
	defer func() { _ = fastResp.Body.Close() }()

	b.Publish(context.Background(), bus.Event{Type: bus.ReplySent, Channel: "telegram", Brain: "default"})

	got := make(chan string, 1)
	go func() { got <- readFrame(t, fastBR) }()
	select {
	case frame := <-got:
		if !strings.Contains(frame, `"type":"reply_sent"`) {
			t.Errorf("healthy client got unexpected frame: %s", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("healthy client got no frame; a slow client tumbled the server")
	}
}

// TestUI_servedReadOnly asserts the embedded vanilla UI is served under /ui and
// wires the live feed via EventSource (no toolchain, no write surface).
func TestUI_servedReadOnly(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b)
	srv := newTestServer(t, lv)

	resp, err := srv.Client().Get(srv.URL + "/ui/")
	if err != nil {
		t.Fatalf("GET /ui/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/ = %d, want 200", resp.StatusCode)
	}
	buf := new(strings.Builder)
	if _, err := bufio.NewReader(resp.Body).WriteTo(buf); err != nil {
		t.Fatalf("read /ui/: %v", err)
	}
	body := buf.String()
	for _, want := range []string{"EventSource", "/api/events", "/api/brains", "/api/channels"} {
		if !strings.Contains(body, want) {
			t.Errorf("/ui/ body missing %q", want)
		}
	}
}

// TestClose_idempotentAndUnblocksStreams asserts Close tears the live-view down:
// in-flight SSE loops return and a second Close does not panic.
func TestClose_idempotentAndUnblocksStreams(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithNow(fixedNow))

	w := newBlockingRW()
	close(w.release) // writes never block in this test
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)

	done := make(chan struct{})
	go func() {
		lv.eventsHandler().ServeHTTP(w, req)
		close(done)
	}()

	// Let the handler reach its serve loop, then Close should unblock it.
	time.Sleep(20 * time.Millisecond)
	lv.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not unblock the SSE serve loop")
	}
	lv.Close() // idempotent: must not panic
}

// nonFlusherRW is a ResponseWriter that does NOT implement http.Flusher, so the
// handler must reject it (SSE requires flushing).
type nonFlusherRW struct {
	header http.Header
	status int
}

func (w *nonFlusherRW) Header() http.Header         { return w.header }
func (w *nonFlusherRW) WriteHeader(code int)        { w.status = code }
func (w *nonFlusherRW) Write(p []byte) (int, error) { return len(p), nil }

// TestEventsHandler_nonFlusherRejected asserts a writer that cannot flush yields
// a 500 (streaming unsupported), never a half-open stream.
func TestEventsHandler_nonFlusherRejected(t *testing.T) {
	b := bus.New()
	defer b.Close()
	lv := New(b, WithLogger(slog.New(slog.DiscardHandler)))

	w := &nonFlusherRW{header: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	lv.eventsHandler().ServeHTTP(w, req)

	if w.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for a non-flushable writer", w.status)
	}
}

// errRW is a flushable ResponseWriter whose Write always errors, modelling a
// dead client mid-stream.
type errRW struct{ header http.Header }

func (w *errRW) Header() http.Header       { return w.header }
func (w *errRW) WriteHeader(int)           {}
func (w *errRW) Flush()                    {}
func (w *errRW) Write([]byte) (int, error) { return 0, errADead }

var errADead = &writeErr{}

type writeErr struct{}

func (*writeErr) Error() string { return "client gone" }

// TestWriteFrame_writeErrorTearsDown asserts writeFrame returns false on a write
// error (so the serve loop tears the connection down) and true on success.
func TestWriteFrame_writeErrorTearsDown(t *testing.T) {
	lv := New(bus.New(), WithNow(fixedNow))
	ev := bus.Event{Type: bus.ReplySent, Channel: "telegram"}

	if lv.writeFrame(&errRW{header: make(http.Header)}, ev) {
		t.Error("writeFrame returned true on a write error; want false (tear down)")
	}
	if !lv.writeFrame(&okRW{header: make(http.Header)}, ev) {
		t.Error("writeFrame returned false on a successful write; want true")
	}
}

// okRW is a flushable ResponseWriter that accepts writes.
type okRW struct{ header http.Header }

func (w *okRW) Header() http.Header         { return w.header }
func (w *okRW) WriteHeader(int)             {}
func (w *okRW) Flush()                      {}
func (w *okRW) Write(p []byte) (int, error) { return len(p), nil }

// waitFor polls cond up to ~2s; returns false on timeout.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}
