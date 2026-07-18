// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package discord tests — Piece 4, sub-phases 3 (base lifecycle) and 4
// (resume/reconnect). These exercise the Gateway state machine and the reconnect
// SUPERVISOR against a FAKE gateway WebSocket (httptest + the server side of
// coder/websocket) that accepts MULTIPLE connections — each reconnect is a fresh
// dial. Deterministic, no real network: the backoff clock and jitter source are
// injected. They pin the SP3 lifecycle (Hello -> Identify -> Ready -> Dispatch,
// heartbeat/ACK/zombie, mapper + backpressure drops, clean ctx-cancel) AND the SP4
// supervisor: op7/zombie -> Resume on the SAME inbound channel (never closed across a
// reconnect), op9 non-resumable -> re-Identify, op9 during a resume -> fall back to
// Identify, fatal close codes -> named terminal cause (no retry), and exponential
// backoff between attempts.
package discord

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const gwTestTokenEnv = "KORVUN_DISCORD_GW_TEST_TOKEN" // #nosec G101 -- env-var NAME, not a credential
const gwTestTokenValue = "bot-token-abc"              // #nosec G101 -- test-only fake token, not a real credential

// reusable message fixtures
const (
	humanMsg  = `{"id":"900","channel_id":"555","content":"hola korvun","author":{"id":"222","username":"alice","global_name":"Alice A."}}`
	humanMsg2 = `{"id":"901","channel_id":"555","content":"segundo","author":{"id":"222","username":"alice","global_name":"Alice A."}}`
)

// --- fake gateway server (multi-connection) --------------------------------

// startFakeGateway runs a server-side Discord gateway whose per-connection behaviour
// is `script`. selfURL (the fake's own ws:// URL) is passed so a script can hand it
// back as the READY resume_gateway_url, making the client's resume dial hit the same
// fake. It returns the ws:// URL the adapter dials.
func startFakeGateway(t *testing.T, script func(ctx context.Context, c *websocket.Conn, selfURL string)) string {
	t.Helper()
	var selfURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		script(r.Context(), c, selfURL)
	}))
	t.Cleanup(srv.Close)
	selfURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	return selfURL
}

// scriptGateway dispatches connection N to handlers[N-1]; connections past the list
// just drain. Lets a test choreograph an identify connection, then a resume
// connection, then whatever follows.
func scriptGateway(t *testing.T, handlers ...func(ctx context.Context, c *websocket.Conn, selfURL string)) string {
	t.Helper()
	var n atomic.Int32
	return startFakeGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		i := int(n.Add(1)) - 1
		if i < len(handlers) {
			handlers[i](ctx, c, selfURL)
			return
		}
		drainClient(ctx, c)
	})
}

func serverRead(ctx context.Context, c *websocket.Conn) (gatewayPayload, error) {
	var f gatewayPayload
	err := wsjson.Read(ctx, c, &f)
	return f, err
}

func drainClient(ctx context.Context, c *websocket.Conn) {
	for {
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
	}
}

func helloFrame(intervalMs int) map[string]any {
	return map[string]any{"op": opHello, "d": map[string]any{"heartbeat_interval": intervalMs}}
}

func readyFrame(seq int, selfID, resumeURL string) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "READY", "d": map[string]any{
		"session_id":         "SESS-1",
		"resume_gateway_url": resumeURL,
		"user":               map[string]any{"id": selfID, "username": "korvun"},
	}}
}

func resumedFrame(seq int) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "RESUMED", "d": map[string]any{}}
}

func msgFrame(seq int, d string) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "MESSAGE_CREATE", "d": json.RawMessage(d)}
}

func typingFrame(seq int) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "TYPING_START", "d": map[string]any{}}
}

func ackFrame() map[string]any             { return map[string]any{"op": opHeartbeatACK} }
func opFrame(op int) map[string]any        { return map[string]any{"op": op} }
func invalidSession(d bool) map[string]any { return map[string]any{"op": opInvalidSession, "d": d} }

// --- helpers ---------------------------------------------------------------

type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) reasons() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, r := range h.records {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "reason" {
				out = append(out, a.Value.String())
			}
			return true
		})
	}
	return out
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// recordingClock records each requested sleep duration on an unbounded-by-pacing
// channel (the reader paces the reconnect loop) and returns immediately, honouring
// ctx. It lets a test assert the exact backoff sequence with no wall-clock wait.
type recordingClock struct{ durs chan time.Duration }

func newRecordingClock() *recordingClock { return &recordingClock{durs: make(chan time.Duration)} }
func (c *recordingClock) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case c.durs <- d:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// immediateClock returns from Sleep at once (honouring ctx), so behaviour tests never
// wait in wall-clock time. It is the default injected by newGatewayAdapter.
type immediateClock struct{}

func (immediateClock) Sleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

// blockingClock blocks in Sleep until ctx is cancelled and signals when it parks, so a
// test can deterministically cancel the supervisor mid-backoff.
type blockingClock struct{ entered chan struct{} }

func (c blockingClock) Sleep(ctx context.Context, _ time.Duration) error {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func newGatewayAdapter(t *testing.T, url string, extra ...Option) *Adapter {
	t.Helper()
	t.Setenv(gwTestTokenEnv, gwTestTokenValue)
	opts := []Option{
		WithTokenEnv(gwTestTokenEnv),
		withGatewayURLForTests(url),
		withRandForTests(func() float64 { return 0 }), // immediate heartbeat jitter
		withClockForTests(immediateClock{}),           // no wall-clock backoff waits
	}
	opts = append(opts, extra...)
	a, err := New(opts...)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	return a
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

// waitClosed drains inbound until it is closed, proving a clean shutdown (the
// supervisor only closes inbound after every gateway goroutine has returned).
func waitClosed(t *testing.T, ch <-chan *envelope.Envelope, d time.Duration) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatalf("inbound channel not closed within %s", d)
		}
	}
}

// recvEnvelope waits for one Envelope on inbound.
func recvEnvelope(t *testing.T, ch <-chan *envelope.Envelope, d time.Duration) *envelope.Envelope {
	t.Helper()
	select {
	case env, ok := <-ch:
		if !ok {
			t.Fatal("inbound closed while waiting for an envelope")
		}
		return env
	case <-time.After(d):
		t.Fatalf("no envelope within %s", d)
		return nil
	}
}

// --- SP3 lifecycle tests (adapted to the supervisor) -----------------------

func TestGateway_HappyFlow(t *testing.T) {
	const self = "BOTSELF-1"
	idfCh := make(chan identifyData, 1)
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		f, err := serverRead(ctx, c)
		if err != nil {
			return
		}
		var idd identifyData
		_ = json.Unmarshal(f.D, &idd)
		select {
		case idfCh <- idd:
		default:
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
		_ = wsjson.Write(ctx, c, msgFrame(2, humanMsg))
		_ = wsjson.Write(ctx, c, msgFrame(3, `{"id":"901","channel_id":"555","content":"beep","author":{"id":"999","username":"botz","bot":true}}`))
		_ = wsjson.Write(ctx, c, msgFrame(4, `{"id":"902","channel_id":"555","content":"my own","author":{"id":"BOTSELF-1","username":"korvun","bot":true}}`))
		drainClient(ctx, c)
	})

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	env := recvEnvelope(t, inbound, 2*time.Second)
	if env.Sender.ID != "222" || env.Meta[conversation.MetaConversationID] != "555" ||
		len(env.Parts) != 1 || env.Parts[0].Content != "hola korvun" {
		t.Errorf("unexpected envelope: sender=%q conv=%q parts=%+v", env.Sender.ID, env.Meta[conversation.MetaConversationID], env.Parts)
	}

	select {
	case idd := <-idfCh:
		if idd.Token != gwTestTokenValue {
			t.Errorf("Identify token = %q, want the env value", idd.Token)
		}
		if idd.Intents != gatewayIntents {
			t.Errorf("Identify intents = %d, want %d", idd.Intents, gatewayIntents)
		}
		if idd.Properties.Browser != "korvun" || idd.Properties.Device != "korvun" || idd.Properties.OS != runtime.GOOS {
			t.Errorf("Identify properties = %+v, want os=%q browser=device=korvun", idd.Properties, runtime.GOOS)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no Identify captured")
	}

	waitFor(t, 2*time.Second, func() bool { return a.DroppedCount() == 2 }) // other-bot + self
	if got := a.DroppedCount(); got != 2 {
		t.Errorf("DroppedCount = %d, want 2", got)
	}
	if ri := a.readyInfo(); ri == nil || ri.id != "SESS-1" || ri.resumeURL == "" {
		t.Errorf("readyInfo = %+v, want session id + resume url recorded", ri)
	}

	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

func TestGateway_Heartbeat(t *testing.T) {
	const self = "BOT-HB"
	beats := make(chan int, 64)
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		if err := wsjson.Write(ctx, c, helloFrame(100)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
		_ = wsjson.Write(ctx, c, typingFrame(5))
		for {
			f, err := serverRead(ctx, c)
			if err != nil {
				return
			}
			if f.Op == opHeartbeat {
				var seq *int
				_ = json.Unmarshal(f.D, &seq)
				v := -1
				if seq != nil {
					v = *seq
				}
				select {
				case beats <- v:
				default:
				}
				_ = wsjson.Write(ctx, c, ackFrame())
			}
		}
	})

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	deadline := time.After(2 * time.Second)
	got, sawSeq5 := 0, false
	for got < 3 {
		select {
		case s := <-beats:
			got++
			if s == 5 {
				sawSeq5 = true
			}
		case <-deadline:
			t.Fatalf("only %d heartbeats within 2s", got)
		}
	}
	if !sawSeq5 {
		t.Error("no heartbeat carried the latest tracked seq (5)")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

func TestGateway_DropReasons(t *testing.T) {
	const self = "BOT-D"
	handler := &capturingHandler{}
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
		_ = wsjson.Write(ctx, c, msgFrame(2, `{"id":"1","channel_id":"555","content":"x","author":{"id":"777","username":"b","bot":true}}`))
		_ = wsjson.Write(ctx, c, msgFrame(3, `{"id":"2","channel_id":"555","content":"y","webhook_id":"42","author":{"id":"888","username":"wh"}}`))
		_ = wsjson.Write(ctx, c, msgFrame(4, `{"id":"3","channel_id":"555","content":"z","author":{"id":"BOT-D","username":"korvun","bot":true}}`))
		drainClient(ctx, c)
	})

	a := newGatewayAdapter(t, url, WithLogger(slog.New(handler)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	for _, want := range []string{"bot", "webhook", "self"} {
		want := want
		waitFor(t, 2*time.Second, func() bool { return contains(handler.reasons(), want) })
	}
	if got := a.DroppedCount(); got != 3 {
		t.Errorf("DroppedCount = %d, want 3", got)
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

func TestGateway_Backpressure(t *testing.T) {
	const self = "BOT-BP"
	handler := &capturingHandler{}
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
		for i := 0; i < 3; i++ {
			_ = wsjson.Write(ctx, c, msgFrame(2+i, humanMsg))
		}
		drainClient(ctx, c)
	})

	a := newGatewayAdapter(t, url, WithLogger(slog.New(handler)), withInboundCapacityForTests(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		n := 0
		for _, r := range handler.reasons() {
			if r == "inbound_buffer_saturated" {
				n++
			}
		}
		return n == 2
	})
	if got := a.DroppedCount(); got != 2 {
		t.Errorf("DroppedCount = %d, want 2", got)
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// --- SP4 resume / reconnect tests ------------------------------------------

// resumeCapture records what the fake saw in a Resume (op 6) frame.
type resumeCapture struct {
	op        int
	sessionID string
	seq       int
}

// TestGateway_Op7Resumes is the SP4 headline: a Reconnect (op 7) makes the client
// dial the resume URL, send Resume (op 6) with the right session_id + seq, and the
// replayed message arrives on the SAME inbound channel (never closed across the
// reconnect), finishing with RESUMED.
func TestGateway_Op7Resumes(t *testing.T) {
	const self = "BOT-R7"
	resumeCh := make(chan resumeCapture, 1)
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // connection 1: identify
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			_ = wsjson.Write(ctx, c, msgFrame(2, humanMsg))
			_ = wsjson.Write(ctx, c, opFrame(opReconnect)) // op7 -> resume
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, _ string) { // connection 2: resume
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			f, err := serverRead(ctx, c) // Resume
			if err != nil {
				return
			}
			var rd resumeData
			_ = json.Unmarshal(f.D, &rd)
			resumeCh <- resumeCapture{op: f.Op, sessionID: rd.SessionID, seq: rd.Seq}
			_ = wsjson.Write(ctx, c, msgFrame(3, humanMsg2)) // replayed
			_ = wsjson.Write(ctx, c, resumedFrame(4))
			drainClient(ctx, c)
		},
	)

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	if env := recvEnvelope(t, inbound, 2*time.Second); env.Parts[0].Content != "hola korvun" {
		t.Errorf("first message = %q, want 'hola korvun'", env.Parts[0].Content)
	}
	// The replayed message must arrive on the SAME channel — it was never closed.
	if env := recvEnvelope(t, inbound, 2*time.Second); env.Parts[0].Content != "segundo" {
		t.Errorf("replayed message = %q, want 'segundo'", env.Parts[0].Content)
	}

	select {
	case rc := <-resumeCh:
		if rc.op != opResume {
			t.Errorf("resume op = %d, want %d", rc.op, opResume)
		}
		if rc.sessionID != "SESS-1" {
			t.Errorf("resume session_id = %q, want SESS-1", rc.sessionID)
		}
		if rc.seq != 2 {
			t.Errorf("resume seq = %d, want 2 (last seq before the reconnect)", rc.seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no Resume frame captured")
	}

	if a.ReconnectCount() < 1 {
		t.Errorf("ReconnectCount = %d, want >= 1", a.ReconnectCount())
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_ZombieResumes: a zombied connection (heartbeats stop being ACKed) makes
// the client resume rather than terminate.
func TestGateway_ZombieResumes(t *testing.T) {
	const self = "BOT-RZ"
	resumed := make(chan struct{}, 1)
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // identify; then stop ACKing
			_ = wsjson.Write(ctx, c, helloFrame(25))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			drainClient(ctx, c) // read heartbeats, never ACK -> zombie
		},
		func(ctx context.Context, c *websocket.Conn, _ string) { // resume
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if f, err := serverRead(ctx, c); err != nil || f.Op != opResume {
				return
			}
			select {
			case resumed <- struct{}{}:
			default:
			}
			_ = wsjson.Write(ctx, c, resumedFrame(2))
			drainClient(ctx, c)
		},
	)

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case <-resumed:
	case <-time.After(3 * time.Second):
		t.Fatal("zombie did not trigger a resume within 3s")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_Op9ResumableResumes: Invalid Session with d=true keeps the session —
// the client reconnects with a Resume (op 6), not a fresh Identify.
func TestGateway_Op9ResumableResumes(t *testing.T) {
	const self = "BOT-R9T"
	secondOp := make(chan int, 1)
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // identify -> op9 d=true
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			_ = wsjson.Write(ctx, c, invalidSession(true))
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, _ string) { // must be a Resume
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			f, err := serverRead(ctx, c)
			if err != nil {
				return
			}
			select {
			case secondOp <- f.Op:
			default:
			}
			_ = wsjson.Write(ctx, c, resumedFrame(2))
			drainClient(ctx, c)
		},
	)

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case op := <-secondOp:
		if op != opResume {
			t.Errorf("second connection op = %d, want Resume (%d) after op9 d=true", op, opResume)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no resume after op9 d=true")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_ServerRequestedHeartbeat: a server-initiated heartbeat request (op 1)
// elicits an immediate heartbeat, independent of the periodic cadence (jitter pushes
// the periodic beat far past the test window).
func TestGateway_ServerRequestedHeartbeat(t *testing.T) {
	const self = "BOT-RQ"
	beat := make(chan struct{}, 1)
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		_ = wsjson.Write(ctx, c, helloFrame(600000))
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
		_ = wsjson.Write(ctx, c, opFrame(opHeartbeat)) // "beat now"
		for {
			f, err := serverRead(ctx, c)
			if err != nil {
				return
			}
			if f.Op == opHeartbeat {
				select {
				case beat <- struct{}{}:
				default:
				}
			}
		}
	})
	a := newGatewayAdapter(t, url, withRandForTests(func() float64 { return 0.99 }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case <-beat:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not answer the server heartbeat request within 2s")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_Op9NonResumableReidentifies: Invalid Session with d=false makes the
// client start a FRESH session (the fake sees an Identify, not a Resume) after the
// injected backoff wait.
func TestGateway_Op9NonResumableReidentifies(t *testing.T) {
	const self = "BOT-R9"
	secondOp := make(chan int, 1)
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // identify -> op9 d=false
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			_ = wsjson.Write(ctx, c, invalidSession(false))
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // must be a fresh Identify
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			f, err := serverRead(ctx, c)
			if err != nil {
				return
			}
			select {
			case secondOp <- f.Op:
			default:
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			drainClient(ctx, c)
		},
	)

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case op := <-secondOp:
		if op != opIdentify {
			t.Errorf("second connection op = %d, want Identify (%d) after op9 d=false", op, opIdentify)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no second connection after op9 d=false")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_Op9DuringResumeFallsBackToIdentify: op9 received while resuming makes
// the client fall back to a fresh Identify on the next connection.
func TestGateway_Op9DuringResumeFallsBackToIdentify(t *testing.T) {
	const self = "BOT-RF"
	thirdOp := make(chan int, 1)
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // identify -> op7
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			_ = wsjson.Write(ctx, c, opFrame(opReconnect))
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, _ string) { // resume -> op9 d=false
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if f, err := serverRead(ctx, c); err != nil || f.Op != opResume {
				return
			}
			_ = wsjson.Write(ctx, c, invalidSession(false))
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, selfURL string) { // must be a fresh Identify
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			f, err := serverRead(ctx, c)
			if err != nil {
				return
			}
			select {
			case thirdOp <- f.Op:
			default:
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			drainClient(ctx, c)
		},
	)

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case op := <-thirdOp:
		if op != opIdentify {
			t.Errorf("third connection op = %d, want Identify (%d) after op9 during resume", op, opIdentify)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no fallback Identify after op9 during resume")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_FatalCloseCodeTerminates: a fatal close code (4004 auth failed) is NOT
// retried — the supervisor closes inbound and surfaces ErrGatewayFatalClose.
func TestGateway_FatalCloseCodeTerminates(t *testing.T) {
	var conns atomic.Int32
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn, _ string) {
		conns.Add(1)
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		_, _ = serverRead(ctx, c)                                        // Identify
		_ = c.Close(websocket.StatusCode(4004), "authentication failed") // fatal
	})

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	waitClosed(t, inbound, 3*time.Second)
	if e := a.terminalErr(); !errors.Is(e, ErrGatewayFatalClose) {
		t.Errorf("terminalErr = %v, want ErrGatewayFatalClose", e)
	}
	if n := conns.Load(); n != 1 {
		t.Errorf("fatal close was retried: %d connections, want exactly 1", n)
	}
}

// TestGateway_BackoffSequence pins the exponential backoff between reconnect attempts
// using an injected clock (records durations) and a fixed jitter fraction. Driven by
// repeated dial failures (a server that never upgrades), which take the backoff path.
func TestGateway_BackoffSequence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // never upgrades -> dial fails
	}))
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	clock := newRecordingClock()
	a := newGatewayAdapter(t, url,
		withRandForTests(func() float64 { return 0.5 }),
		withClockForTests(clock),
	)
	ctx, cancel := context.WithCancel(context.Background())
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	// backoff(n) = 0.5 * (1s << n): 500ms, 1s, 2s, 4s ...
	want := []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second, 4 * time.Second}
	for i, w := range want {
		select {
		case got := <-clock.durs:
			if got != w {
				t.Errorf("backoff[%d] = %s, want %s", i, got, w)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d backoff sleeps observed", i)
		}
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_Op9FreshWait pins that a fresh re-Identify after Invalid Session
// (op 9, d=false) waits Discord's mandated 1-5s, NOT the exponential backoff. With
// jitter 0.5 the wait is 1s + 0.5*4s = 3s.
func TestGateway_Op9FreshWait(t *testing.T) {
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn, _ string) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, invalidSession(false)) // -> reidentify with the 1-5s wait
		drainClient(ctx, c)
	})

	clock := newRecordingClock()
	a := newGatewayAdapter(t, url,
		withRandForTests(func() float64 { return 0.5 }),
		withClockForTests(clock),
	)
	ctx, cancel := context.WithCancel(context.Background())
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case got := <-clock.durs:
		if got != 3*time.Second {
			t.Errorf("op9 re-identify wait = %s, want 3s (1s + 0.5*4s), NOT the backoff", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no re-identify wait observed after op9 d=false")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_NoTokenInLogs is an ADR-0010 regression guard: a full identify + resume
// cycle must never write the bot token value to any log record.
func TestGateway_NoTokenInLogs(t *testing.T) {
	const self = "BOT-LG"
	handler := &capturingHandler{}
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) {
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			_ = wsjson.Write(ctx, c, opFrame(opReconnect))
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, _ string) {
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, resumedFrame(2))
			drainClient(ctx, c)
		},
	)
	a := newGatewayAdapter(t, url, WithLogger(slog.New(handler)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	waitFor(t, 2*time.Second, func() bool { return a.ReconnectCount() >= 1 })
	cancel()
	waitClosed(t, inbound, 2*time.Second)

	handler.mu.Lock()
	defer handler.mu.Unlock()
	for _, r := range handler.records {
		if strings.Contains(r.Message, gwTestTokenValue) {
			t.Fatalf("token value leaked into a log message: %q", r.Message)
		}
		r.Attrs(func(at slog.Attr) bool {
			if strings.Contains(at.Value.String(), gwTestTokenValue) {
				t.Fatalf("token value leaked into log attr %q = %q", at.Key, at.Value.String())
			}
			return true
		})
	}
}

// --- shutdown / lifecycle guards -------------------------------------------

func TestGateway_ShutdownIsClean(t *testing.T) {
	const self = "BOT-S"
	cases := []struct {
		name   string
		script func(ctx context.Context, c *websocket.Conn, selfURL string)
		drain  bool
	}{
		{
			name:   "before hello",
			script: func(ctx context.Context, c *websocket.Conn, _ string) { drainClient(ctx, c) },
		},
		{
			name: "after ready, idle",
			script: func(ctx context.Context, c *websocket.Conn, selfURL string) {
				_ = wsjson.Write(ctx, c, helloFrame(60000))
				if _, err := serverRead(ctx, c); err != nil {
					return
				}
				_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
				drainClient(ctx, c)
			},
		},
		{
			name:  "mid dispatch",
			drain: true,
			script: func(ctx context.Context, c *websocket.Conn, selfURL string) {
				_ = wsjson.Write(ctx, c, helloFrame(60000))
				if _, err := serverRead(ctx, c); err != nil {
					return
				}
				_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
				for {
					if err := wsjson.Write(ctx, c, msgFrame(2, humanMsg)); err != nil {
						return
					}
					time.Sleep(time.Millisecond)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := scriptGateway(t, tc.script)
			a := newGatewayAdapter(t, url)
			ctx, cancel := context.WithCancel(context.Background())
			inbound, err := a.Receive(ctx)
			if err != nil {
				t.Fatalf("Receive = %v", err)
			}
			if tc.drain {
				select {
				case <-inbound:
				case <-time.After(time.Second):
				}
			} else {
				time.Sleep(30 * time.Millisecond)
			}
			cancel()
			waitClosed(t, inbound, 2*time.Second)
		})
	}
}

// TestGateway_CancelMidBackoff: cancelling the ctx while the supervisor is parked in a
// backoff sleep is a clean stop.
func TestGateway_CancelMidBackoff(t *testing.T) {
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn, _ string) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, invalidSession(false)) // -> reidentify -> backoff sleep
		drainClient(ctx, c)
	})
	clock := blockingClock{entered: make(chan struct{}, 1)}
	a := newGatewayAdapter(t, url, withClockForTests(clock))
	ctx, cancel := context.WithCancel(context.Background())
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	select {
	case <-clock.entered: // supervisor is parked in the backoff wait
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor never reached the backoff wait")
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
	if e := a.terminalErr(); e != nil {
		t.Errorf("terminalErr on clean mid-backoff cancel = %v, want nil", e)
	}
}

// TestGateway_CancelMidResume: cancelling while a resume is in flight is a clean stop.
func TestGateway_CancelMidResume(t *testing.T) {
	const self = "BOT-CR"
	url := scriptGateway(t,
		func(ctx context.Context, c *websocket.Conn, selfURL string) {
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			if _, err := serverRead(ctx, c); err != nil {
				return
			}
			_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
			_ = wsjson.Write(ctx, c, opFrame(opReconnect))
			drainClient(ctx, c)
		},
		func(ctx context.Context, c *websocket.Conn, _ string) {
			_ = wsjson.Write(ctx, c, helloFrame(60000))
			_, _ = serverRead(ctx, c) // Resume; then hang (no RESUMED)
			drainClient(ctx, c)
		},
	)
	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	time.Sleep(60 * time.Millisecond) // let it reach the resume session
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// --- construction / guards -------------------------------------------------

func TestGateway_SecondReceiveRejected(t *testing.T) {
	const self = "BOT-2R"
	url := scriptGateway(t, func(ctx context.Context, c *websocket.Conn, selfURL string) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self, selfURL))
		drainClient(ctx, c)
	})
	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("first Receive = %v", err)
	}
	if _, err := a.Receive(ctx); !errors.Is(err, ErrAlreadyReceiving) {
		t.Errorf("second Receive = %v, want ErrAlreadyReceiving", err)
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

// TestGateway_TokenMissingAtConnectIsFatal: a token that disappears after New is a
// fatal terminal cause at connect time (ADR-0010) — no infinite retry.
func TestGateway_TokenMissingAtConnectIsFatal(t *testing.T) {
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn, _ string) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		drainClient(ctx, c)
	})
	t.Setenv(gwTestTokenEnv, gwTestTokenValue)
	a, err := New(
		WithTokenEnv(gwTestTokenEnv),
		withGatewayURLForTests(url),
		withRandForTests(func() float64 { return 0 }),
	)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	if err := os.Unsetenv(gwTestTokenEnv); err != nil {
		t.Fatalf("Unsetenv = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	waitClosed(t, inbound, 2*time.Second)
	e := a.terminalErr()
	if !errors.Is(e, ErrMissingToken) {
		t.Fatalf("terminalErr = %v, want ErrMissingToken", e)
	}
	if !strings.Contains(e.Error(), gwTestTokenEnv) {
		t.Errorf("error must name the env var %q; got %q", gwTestTokenEnv, e.Error())
	}
}

// TestGateway_DialFailureRetries: a dial failure is transient — the supervisor retries
// with backoff (counted) rather than terminating, until the ctx is cancelled.
func TestGateway_DialFailureRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // never upgrades
	}))
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	clock := newRecordingClock()
	// rnd 0.5 so the backoff is non-zero and the (blocking) recording clock is
	// actually invoked — this test proves retry happens, not the exact durations.
	a := newGatewayAdapter(t, url, withRandForTests(func() float64 { return 0.5 }), withClockForTests(clock))
	ctx, cancel := context.WithCancel(context.Background())
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	// Observe at least two backoff sleeps (proves it retried, did not terminate).
	for i := 0; i < 2; i++ {
		select {
		case <-clock.durs:
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d dial retries observed", i)
		}
	}
	if a.ReconnectCount() < 2 {
		t.Errorf("ReconnectCount = %d, want >= 2", a.ReconnectCount())
	}
	cancel()
	waitClosed(t, inbound, 2*time.Second)
}
