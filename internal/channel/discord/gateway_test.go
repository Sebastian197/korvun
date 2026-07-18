// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package discord tests — Piece 4, sub-phase 3 (Gateway base lifecycle). These
// exercise the Gateway state machine against a FAKE gateway WebSocket (httptest +
// the server side of coder/websocket) — deterministic, no real network. They pin:
// the Hello -> Identify(intents 37377) -> Ready(selfID) -> Dispatch choreography;
// heartbeat cadence carrying the last seq with ACK tracking; zombie detection when
// ACKs stop; mapper and backpressure drops incrementing DroppedCount with a logged
// reason; and clean ctx-cancel shutdown with no leaked goroutines (proven by the
// inbound channel closing, which only happens after run joins the heartbeat loop).
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
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const gwTestTokenEnv = "KORVUN_DISCORD_GW_TEST_TOKEN" // #nosec G101 -- env-var NAME, not a credential
const gwTestTokenValue = "bot-token-abc"              // #nosec G101 -- test-only fake token, not a real credential

// --- fake gateway server ---------------------------------------------------

// startFakeGateway runs a server-side Discord gateway whose per-connection
// behaviour is `script`. It returns the ws:// URL the adapter dials. The server is
// torn down at test end.
func startFakeGateway(t *testing.T, script func(ctx context.Context, c *websocket.Conn)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		script(r.Context(), c)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// serverRead reads one gateway frame the client sent.
func serverRead(ctx context.Context, c *websocket.Conn) (gatewayPayload, error) {
	var f gatewayPayload
	err := wsjson.Read(ctx, c, &f)
	return f, err
}

// drainClient reads (and discards) client frames until the client closes; it keeps
// the connection open so the client can process server-sent frames.
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

func readyFrame(seq int, selfID string) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "READY", "d": map[string]any{
		"session_id":         "SESS-1",
		"resume_gateway_url": "wss://resume.example/?v=10",
		"user":               map[string]any{"id": selfID, "username": "korvun"},
	}}
}

func msgFrame(seq int, d string) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "MESSAGE_CREATE", "d": json.RawMessage(d)}
}

func typingFrame(seq int) map[string]any {
	return map[string]any{"op": opDispatch, "s": seq, "t": "TYPING_START", "d": map[string]any{}}
}

func ackFrame() map[string]any { return map[string]any{"op": opHeartbeatACK} }

// --- helpers ---------------------------------------------------------------

// capturingHandler records the structured log records so tests can assert the
// "reason" attribute attached to a drop.
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

func newGatewayAdapter(t *testing.T, url string, extra ...Option) *Adapter {
	t.Helper()
	t.Setenv(gwTestTokenEnv, gwTestTokenValue)
	opts := []Option{
		WithTokenEnv(gwTestTokenEnv),
		withGatewayURLForTests(url),
		withJitterFracForTests(func() float64 { return 0 }),
	}
	opts = append(opts, extra...)
	a, err := New(opts...)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	return a
}

// waitFor polls cond until it holds or the deadline elapses.
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

// waitClosed drains inbound until it is closed, proving a clean shutdown (run only
// closes inbound after every gateway goroutine has returned).
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

// --- tests -----------------------------------------------------------------

func TestGateway_HappyFlow(t *testing.T) {
	const self = "BOTSELF-1"
	idfCh := make(chan identifyData, 1)
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
			return
		}
		f, err := serverRead(ctx, c) // Identify
		if err != nil {
			return
		}
		var idd identifyData
		_ = json.Unmarshal(f.D, &idd)
		select {
		case idfCh <- idd:
		default:
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
		// human => kept; other bot => dropped; self => dropped.
		_ = wsjson.Write(ctx, c, msgFrame(2, `{"id":"900","channel_id":"555","content":"hola korvun","author":{"id":"222","username":"alice","global_name":"Alice A."}}`))
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

	select {
	case env := <-inbound:
		if env.Channel != ChannelName {
			t.Errorf("Channel = %q, want %q", env.Channel, ChannelName)
		}
		if env.Sender.ID != "222" {
			t.Errorf("Sender.ID = %q, want 222", env.Sender.ID)
		}
		if got := env.Meta[conversation.MetaConversationID]; got != "555" {
			t.Errorf("conversation.id = %q, want 555", got)
		}
		if len(env.Parts) != 1 || env.Parts[0].Content != "hola korvun" {
			t.Errorf("Parts = %+v, want single text 'hola korvun'", env.Parts)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no inbound envelope within 2s")
	}

	select {
	case idd := <-idfCh:
		if idd.Token != gwTestTokenValue {
			t.Errorf("Identify token = %q, want the env value (read at connect time)", idd.Token)
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

	waitFor(t, 2*time.Second, func() bool { return a.DroppedCount() == 2 })

	if ri := a.readyInfo(); ri == nil || ri.id != "SESS-1" || ri.resumeURL == "" {
		t.Errorf("readyInfo = %+v, want session id + resume url recorded (for SP4)", ri)
	}

	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

func TestGateway_Heartbeat(t *testing.T) {
	const self = "BOT-HB"
	beats := make(chan int, 64)
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		// 100ms interval: wide enough that a slow ACK round-trip under -race/CI load
		// never falsely trips the zombie detector, while still exercising cadence
		// (3 beats ~300ms, well inside the 2s deadline).
		if err := wsjson.Write(ctx, c, helloFrame(100)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
		_ = wsjson.Write(ctx, c, typingFrame(5)) // bump the tracked seq to 5
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
	// Collecting 3 beats is itself the ACK proof: an implementation that ignored
	// op-11 ACKs would trip the zombie detector and kill the session after beat #1.

	cancel()
	waitClosed(t, inbound, 2*time.Second)
}

func TestGateway_Zombie(t *testing.T) {
	const self = "BOT-Z"
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		if err := wsjson.Write(ctx, c, helloFrame(25)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
		drainClient(ctx, c) // read heartbeats but NEVER ACK
	})

	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}

	waitClosed(t, inbound, 3*time.Second) // zombie => client closes => inbound closes
	if e := a.terminalErr(); !errors.Is(e, ErrZombieConnection) {
		t.Errorf("terminalErr = %v, want ErrZombieConnection", e)
	}
}

func TestGateway_DropReasons(t *testing.T) {
	const self = "BOT-D"
	handler := &capturingHandler{}
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
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

	// Poll the reason itself, not DroppedCount: the count is incremented BEFORE the
	// log, so waiting on the count then reading reasons() would race the last log.
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
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
		for i := 0; i < 3; i++ {
			_ = wsjson.Write(ctx, c, msgFrame(2+i, `{"id":"m","channel_id":"555","content":"hi","author":{"id":"222","username":"alice"}}`))
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

	// Never drain inbound: capacity 1 accepts one, the other two saturate and drop.
	// Poll the reason count (logged after the counter increment) to avoid the race.
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

func TestGateway_ShutdownIsClean(t *testing.T) {
	const self = "BOT-S"
	cases := []struct {
		name   string
		script func(ctx context.Context, c *websocket.Conn)
		drain  bool
	}{
		{
			name:   "before hello (blocked reading Hello)",
			script: func(ctx context.Context, c *websocket.Conn) { drainClient(ctx, c) },
		},
		{
			name: "after identify, before ready",
			script: func(ctx context.Context, c *websocket.Conn) {
				_ = wsjson.Write(ctx, c, helloFrame(60000))
				drainClient(ctx, c)
			},
		},
		{
			name: "after ready, idle",
			script: func(ctx context.Context, c *websocket.Conn) {
				_ = wsjson.Write(ctx, c, helloFrame(60000))
				if _, err := serverRead(ctx, c); err != nil {
					return
				}
				_ = wsjson.Write(ctx, c, readyFrame(1, self))
				drainClient(ctx, c)
			},
		},
		{
			name:  "mid dispatch",
			drain: true,
			script: func(ctx context.Context, c *websocket.Conn) {
				_ = wsjson.Write(ctx, c, helloFrame(60000))
				if _, err := serverRead(ctx, c); err != nil {
					return
				}
				_ = wsjson.Write(ctx, c, readyFrame(1, self))
				for {
					if err := wsjson.Write(ctx, c, msgFrame(2, `{"id":"m","channel_id":"555","content":"hi","author":{"id":"222","username":"alice"}}`)); err != nil {
						return
					}
					time.Sleep(time.Millisecond)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := startFakeGateway(t, tc.script)
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
				time.Sleep(30 * time.Millisecond) // let the client reach the stage
			}
			cancel()
			waitClosed(t, inbound, 2*time.Second)
		})
	}
}

// TestGateway_ServerControlOps pins that op 7 (Reconnect) and op 9 (Invalid Session)
// terminate the SP3 session with their named errors (SP4 turns them into
// resume/reconnect).
func TestGateway_ServerControlOps(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   int
		want error
	}{
		{"reconnect op7", opReconnect, ErrGatewayReconnect},
		{"invalid session op9", opInvalidSession, ErrGatewayInvalidSession},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := tc.op
			url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
				if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
					return
				}
				if _, err := serverRead(ctx, c); err != nil {
					return
				}
				_ = wsjson.Write(ctx, c, map[string]any{"op": op})
				drainClient(ctx, c)
			})
			a := newGatewayAdapter(t, url)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			inbound, err := a.Receive(ctx)
			if err != nil {
				t.Fatalf("Receive = %v", err)
			}
			waitClosed(t, inbound, 2*time.Second)
			if e := a.terminalErr(); !errors.Is(e, tc.want) {
				t.Errorf("terminalErr = %v, want %v", e, tc.want)
			}
		})
	}
}

// TestGateway_ServerRequestedHeartbeat pins that a server-initiated heartbeat request
// (op 1) elicits an immediate heartbeat from the client, independent of the periodic
// cadence (here a very long interval).
func TestGateway_ServerRequestedHeartbeat(t *testing.T) {
	const self = "BOT-RQ"
	beat := make(chan struct{}, 1)
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		if err := wsjson.Write(ctx, c, helloFrame(600000)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
		_ = wsjson.Write(ctx, c, map[string]any{"op": opHeartbeat}) // "beat now"
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
	// Push the periodic beat to ~594s (0.99 of the 600s interval) so any beat within
	// 2s can ONLY be the op-1 response, not the startup heartbeat.
	a := newGatewayAdapter(t, url, withJitterFracForTests(func() float64 { return 0.99 }))
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

// TestGateway_UnexpectedFirstFrame pins that a first frame other than Hello aborts
// the handshake with ErrUnexpectedFirstFrame.
func TestGateway_UnexpectedFirstFrame(t *testing.T) {
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		_ = wsjson.Write(ctx, c, map[string]any{"op": opDispatch, "t": "READY", "d": map[string]any{}})
		drainClient(ctx, c)
	})
	a := newGatewayAdapter(t, url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbound, err := a.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive = %v", err)
	}
	waitClosed(t, inbound, 2*time.Second)
	if e := a.terminalErr(); !errors.Is(e, ErrUnexpectedFirstFrame) {
		t.Errorf("terminalErr = %v, want ErrUnexpectedFirstFrame", e)
	}
}

// TestGateway_TokenReadAtConnect pins the ADR-0010 contract that the token is read
// from the env AT CONNECT TIME, not stored: New succeeds, the token is then removed,
// and Identify fails with ErrMissingToken naming the env var.
func TestGateway_TokenReadAtConnect(t *testing.T) {
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		_ = wsjson.Write(ctx, c, helloFrame(60000))
		drainClient(ctx, c)
	})
	t.Setenv(gwTestTokenEnv, gwTestTokenValue)
	a, err := New(
		WithTokenEnv(gwTestTokenEnv),
		withGatewayURLForTests(url),
		withJitterFracForTests(func() float64 { return 0 }),
	)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	if err := os.Unsetenv(gwTestTokenEnv); err != nil { // token disappears after New
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

// TestGateway_SecondReceiveRejected pins that a second Receive on a running session
// fails loudly with ErrAlreadyReceiving rather than racing (and double-closing) the
// inbound channel. One Adapter drives one session (SP4 adds resume/reconnect).
func TestGateway_SecondReceiveRejected(t *testing.T) {
	const self = "BOT-2R"
	url := startFakeGateway(t, func(ctx context.Context, c *websocket.Conn) {
		if err := wsjson.Write(ctx, c, helloFrame(60000)); err != nil {
			return
		}
		if _, err := serverRead(ctx, c); err != nil {
			return
		}
		_ = wsjson.Write(ctx, c, readyFrame(1, self))
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

// TestGateway_DialError pins that a Gateway dial failure is an honest error returned
// to Receive's caller (no goroutine, no channel).
func TestGateway_DialError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // never upgrades to WebSocket
	}))
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	a := newGatewayAdapter(t, url)
	inbound, err := a.Receive(context.Background())
	if err == nil {
		t.Fatal("Receive = nil error, want a dial error")
	}
	if inbound != nil {
		t.Errorf("Receive returned a non-nil channel alongside the error: %v", inbound)
	}
}
