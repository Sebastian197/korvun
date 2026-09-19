// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// What a webhook_call run closes on, as ONE table — v0.15.1 block A: A2 of the
// seventeenth external pass (only 2xx is success) and P2-4 of the fifteenth (a
// run that ends before a connection was obtained is a decided FAILED).
//
// The table has three axes: whether httptrace's GotConn fired (a connection
// was handed to the request — before it, no request byte can leave), the
// status class of the answer when one arrived, and the kind of error when
// none did. Every row runs the REAL caged tool against a real loopback listener and
// judges the state through CloseStateAfterRun, the one classifier both
// execution paths call.
//
// Evidence level, honest: in-process, the real tool against real loopback
// sockets. Not a compiled binary.
package tool

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// statusServer answers every request with `code`, after reading the whole
// body, and with a Location header only when `location` is non-empty. `hits`
// counts every request the receiver saw, so a row can pin that the cage
// followed nothing.
func statusServer(t *testing.T, code int, location string) (*httptest.Server, <-chan string, *atomic.Int64) {
	t.Helper()
	read := make(chan string, 4)
	hits := &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		select {
		case read <- string(b):
		default:
		}
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(code)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(srv.Close)
	return srv, read, hits
}

// rawResponder is a plain-TCP receiver that reads one whole HTTP/1.1 POST
// (headers, then Content-Length bytes) and writes `response` verbatim. It
// exists for the status lines net/http's server will not emit as a final
// answer, 101 among them.
func rawResponder(t *testing.T, response string) (string, <-chan string, *atomic.Int64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	read := make(chan string, 4)
	hits := &atomic.Int64{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				hits.Add(1)
				b, _ := io.ReadAll(req.Body)
				select {
				case read <- string(b):
				default:
				}
				_, _ = c.Write([]byte(response))
				time.Sleep(200 * time.Millisecond)
			}(c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String(), read, hits
}

// TestWebhookCall_onlyA2xxIsSuccess is A2's mould at the tool: the receiver
// answered a status outside 2xx without a contract that says what it did with
// the POST. That is an answer that is not a usable success — OUTCOME_UNKNOWN —
// never SUCCEEDED. The rule is «< 200 or > 299 ⇒ unknown».
//
// Which 3xx net/http follows is decided by client.go's redirectBehavior: only
// 301, 302, 303, 307 and 308, and only when a Location is present. Those reach
// the cage's zero-hop CheckRedirect, which raises ErrRedirectRefused wrapping
// ErrCageViolation — not ErrEffectDelivered. At base 61ae582 they closed
// unknown only because the WroteRequest observation wrapped the Do error; the
// cure keeps them unknown through an explicit ErrRedirectRefused ⇒
// ErrEffectDelivered arm (controls).
// Every other 3xx — any 3xx without a Location, and 300, 304, 305, 306 even
// with one — is handed back to the caller as the final answer; at base the
// tool read any status under 400 as success. A raw 101 is handed back too.
//
// Every row also pins that the receiver saw exactly ONE request: the cage
// followed nothing.
//
// Planned probing mutation (after green): restore `resp.StatusCode >= 400` as
// the only non-success test ⇒ every row whose status is under 400 and outside
// 2xx reddens.
func TestWebhookCall_onlyA2xxIsSuccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		code     int
		location string
		want     action.State
	}{
		{"200 is success", http.StatusOK, "", action.StateSucceeded},
		{"204 is success", http.StatusNoContent, "", action.StateSucceeded},
		{"299 is success", 299, "", action.StateSucceeded},
		{"300 without Location", http.StatusMultipleChoices, "", action.StateOutcomeUnknown},
		{"301 without Location", http.StatusMovedPermanently, "", action.StateOutcomeUnknown},
		{"302 without Location", http.StatusFound, "", action.StateOutcomeUnknown},
		{"303 without Location", http.StatusSeeOther, "", action.StateOutcomeUnknown},
		{"304 without Location", http.StatusNotModified, "", action.StateOutcomeUnknown},
		{"307 without Location", http.StatusTemporaryRedirect, "", action.StateOutcomeUnknown},
		{"308 without Location", http.StatusPermanentRedirect, "", action.StateOutcomeUnknown},
		{"399 without Location", 399, "", action.StateOutcomeUnknown},
		{"300 with Location (not followed)", http.StatusMultipleChoices, "http://elsewhere.invalid/x", action.StateOutcomeUnknown},
		{"304 with Location (not followed)", http.StatusNotModified, "http://elsewhere.invalid/x", action.StateOutcomeUnknown},
		{"305 with Location (not followed)", http.StatusUseProxy, "http://elsewhere.invalid/x", action.StateOutcomeUnknown},
		{"306 with Location (not followed)", 306, "http://elsewhere.invalid/x", action.StateOutcomeUnknown},
		{"301 with Location (followed, cage refuses)", http.StatusMovedPermanently, "http://elsewhere.invalid/x", action.StateOutcomeUnknown},
		{"308 with Location (followed, cage refuses)", http.StatusPermanentRedirect, "http://elsewhere.invalid/x", action.StateOutcomeUnknown},
		{"500", http.StatusInternalServerError, "", action.StateOutcomeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, read, hits := statusServer(t, tc.code, tc.location)
			host := strings.TrimPrefix(srv.URL, "http://")
			wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{host}})
			if err != nil {
				t.Fatalf("wire the tool: %v", err)
			}
			_, runErr := wc.Execute(context.Background(), srv.URL+` {"event":"ping"}`)
			select {
			case got := <-read:
				if !strings.Contains(got, "ping") {
					t.Fatalf("the receiver did not read the payload: %q", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the receiver never read the POST — the row does not attack what it names")
			}
			if n := hits.Load(); n != 1 {
				t.Fatalf("the receiver saw %d requests, want exactly 1 — the cage followed a hop", n)
			}
			if tc.want == action.StateOutcomeUnknown && errors.Is(runErr, errNotSent) {
				t.Fatalf("HTTP %d: the error carries ErrNotSent over a response: %v", tc.code, runErr)
			}
			if got := CloseStateAfterRun(runErr); got != tc.want {
				t.Fatalf("HTTP %d (Location %q): state = %v (err %v), want %v",
					tc.code, tc.location, got, runErr, tc.want)
			}
		})
	}

	t.Run("a raw 101 Switching Protocols", func(t *testing.T) {
		t.Parallel()
		addr, read, hits := rawResponder(t, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: x\r\n\r\n")
		wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{addr}})
		if err != nil {
			t.Fatalf("wire the tool: %v", err)
		}
		_, runErr := wc.Execute(context.Background(), "http://"+addr+` {"event":"ping"}`)
		select {
		case got := <-read:
			if !strings.Contains(got, "ping") {
				t.Fatalf("the receiver did not read the payload: %q", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the receiver never read the POST — the row does not attack what it names")
		}
		if n := hits.Load(); n != 1 {
			t.Fatalf("the receiver saw %d requests, want exactly 1", n)
		}
		if errors.Is(runErr, errNotSent) {
			t.Fatalf("HTTP 101: the error carries ErrNotSent over a response: %v", runErr)
		}
		if got := CloseStateAfterRun(runErr); got != action.StateOutcomeUnknown {
			t.Fatalf("HTTP 101: state = %v (err %v), want OUTCOME_UNKNOWN", got, runErr)
		}
	})
}

// stallingTLSListener accepts TCP connections and never answers the TLS
// ClientHello: a handshake that cannot complete, so no request byte is ever
// written. `accepted` receives once per accepted connection.
func stallingTLSListener(t *testing.T) (addr string, accepted <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan struct{}, 8)
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})
	return ln.Addr().String(), ch
}

// TestWebhookCall_anEndBeforeAConnectionIsADecidedFailure is P2-4's mould at
// the tool, with its cancellation sister: Codex's reproduction — a loopback
// listener that accepts and never completes the TLS handshake, an https URL on
// the allow-list and a short timeout. The handshake never finished, so no
// connection was handed to the request and no request byte could leave:
// FAILED, never OUTCOME_UNKNOWN. What this proves is scoped to «before a
// connection was obtained»; a deadline after one (a pooled connection, a
// completed handshake) is OUTCOME_UNKNOWN by the table.
//
// The cure keys «nothing left» on the one signal that proves it: httptrace's
// GotConn never fired. In Go 1.26.6 GotConn fires in Transport.getConn only
// once dialConn returned a connection, and dialConn runs the TLS handshake
// (addTLS) before returning it; no request byte is written before GotConn.
//
// Planned probing mutation (after green): drop the not-connected arm from
// CloseStateAfterRun (every ended context unknown again) ⇒ both rows redden.
func TestWebhookCall_anEndBeforeAConnectionIsADecidedFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		cancel bool
	}{
		{"the tool's deadline fires during the handshake", false},
		{"the caller cancels during the handshake", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			addr, accepted := stallingTLSListener(t)
			timeout := 300 * time.Millisecond
			if tc.cancel {
				timeout = 30 * time.Second
			}
			wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{addr}, Timeout: timeout})
			if err != nil {
				t.Fatalf("wire the tool: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				go func() {
					select {
					case <-accepted:
						cancel()
					case <-time.After(10 * time.Second):
					}
				}()
			}
			_, runErr := wc.Execute(ctx, "https://"+addr+` {"event":"ping"}`)
			if runErr == nil {
				t.Fatal("a handshake that never completed answered success")
			}
			if errors.Is(runErr, errDeliveryUnknown) || errors.Is(runErr, ErrEffectDelivered) {
				t.Fatalf("a not-sent error also carries a delivered/unknown class: %v", runErr)
			}
			// The POSITIVE class assertion (Delta 7): it IS ErrNotSent, carrying its
			// exact text. Without it, a stand-in left unswitched — or a cure with no
			// ErrNotSent at all — passes every exclusivity assertion.
			if !errors.Is(runErr, errNotSent) || errNotSent.Error() != notSentText {
				t.Fatalf("err %v is not ErrNotSent (%q), or ErrNotSent's text is not %q", runErr, errNotSent, notSentText)
			}
			if got := CloseStateAfterRun(runErr); got != action.StateFailed {
				t.Fatalf("state = %v (err %v), want FAILED — no connection was obtained", got, runErr)
			}
		})
	}
}

// headersOnlyListener accepts plain-HTTP connections, reads the request line
// and headers, then never reads the body: a POST whose headers — URL
// included — reached the receiver while its body could not be written in
// full. `headers` receives the request line once per connection.
func headersOnlyListener(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan string, 8)
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
			go func(c net.Conn) {
				buf := make([]byte, 0, 4096)
				one := make([]byte, 1)
				for !strings.Contains(string(buf), "\r\n\r\n") {
					if _, err := c.Read(one); err != nil {
						return
					}
					buf = append(buf, one[0])
				}
				select {
				case ch <- strings.SplitN(string(buf), "\r\n", 2)[0]:
				default:
				}
			}(c)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})
	return ln.Addr().String(), ch
}

// TestWebhookCall_anEndAfterTheHeadersLeftStaysUnknown is a GUARD against
// over-curing P2-4: once the request line and headers reached the receiver,
// the URL alone may carry the effect, so an end that fires while the body is
// still being written is NOT «nothing left». The end is the caller's
// cancellation, fired once the receiver reported the request line. It must stay OUTCOME_UNKNOWN.
// Green today; it must stay green after the cure. The body is larger than the
// socket buffers so its write blocks for real.
//
// Planned probing mutation (after green): key FAILED on «WroteRequest never
// fired» instead of «GotConn never fired» ⇒ this reddens.
func TestWebhookCall_anEndAfterTheHeadersLeftStaysUnknown(t *testing.T) {
	t.Parallel()
	addr, headers := headersOnlyListener(t)
	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{addr}, Timeout: 5 * time.Minute, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	ctx, received := cancelWhenReceived(t, headers)
	big := `{"pad":"` + strings.Repeat("A", 32<<20) + `"}`
	_, runErr := wc.Execute(ctx, "http://"+addr+"/trigger "+big)
	if line := received(); !strings.HasPrefix(line, "POST /trigger") {
		t.Fatalf("the receiver read %q, not the POST's request line — the row does not attack what it names", line)
	}
	if errors.Is(runErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", runErr)
	}
	if got := CloseStateAfterRun(runErr); got != action.StateOutcomeUnknown {
		t.Fatalf("state = %v (err %v), want OUTCOME_UNKNOWN — the headers had left", got, runErr)
	}
}

// urlPrefixListener accepts plain-HTTP connections and reads only the first
// `n` bytes the client writes, then stops reading: a receiver that has seen
// the start of the request line and nothing else.
func urlPrefixListener(t *testing.T, n int) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan string, 4)
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
			go func(c net.Conn) {
				buf := make([]byte, n)
				if _, err := io.ReadFull(c, buf); err != nil {
					return
				}
				select {
				case ch <- string(buf):
				default:
				}
			}(c)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})
	return ln.Addr().String(), ch
}

// TestWebhookCall_aLongURLWhoseStartWasReadStaysUnknown is the adversary's
// reproduction against keying P2-4 on WroteHeaders: an 8 MiB query string,
// a receiver that reads the first bytes of «POST /trigger?x=AAA…» and stops.
// WroteHeaders can fire after the headers enter the transport's bufio, before
// the flush; the receiver has read part of the URL all the same. An end here
// must close OUTCOME_UNKNOWN, never «nothing was written». The end is the
// caller's cancellation, fired only once the receiver reported the bytes —
// the first version rode a 500 ms deadline and flaked 6/60 under -race
// -count=20 load. Green today; it must stay green after the cure.
//
// Planned probing mutation (after green): key FAILED on «WroteHeaders never
// fired» or «WroteRequest never fired» ⇒ this reddens.
func TestWebhookCall_aLongURLWhoseStartWasReadStaysUnknown(t *testing.T) {
	t.Parallel()
	addr, prefix := urlPrefixListener(t, 64)
	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{addr}, Timeout: 5 * time.Minute})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	ctx, received := cancelWhenReceived(t, prefix)
	longURL := "http://" + addr + "/trigger?x=" + strings.Repeat("A", 8<<20)
	_, runErr := wc.Execute(ctx, longURL+` {"event":"ping"}`)
	if got := received(); !strings.HasPrefix(got, "POST /trigger?x=AAA") {
		t.Fatalf("the receiver read %q, not the start of the POST's URL — the row does not attack what it names", got)
	}
	if errors.Is(runErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", runErr)
	}
	if got := CloseStateAfterRun(runErr); got != action.StateOutcomeUnknown {
		t.Fatalf("state = %v (err %v), want OUTCOME_UNKNOWN — the receiver read the URL", got, runErr)
	}
}

// resettingListener reads the request line and headers, then resets the
// connection (SO_LINGER 0) while the client is still writing the body.
func resettingListener(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan string, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 0, 4096)
				one := make([]byte, 1)
				for !strings.Contains(string(buf), "\r\n\r\n") {
					if _, err := c.Read(one); err != nil {
						_ = c.Close()
						return
					}
					buf = append(buf, one[0])
				}
				select {
				case ch <- strings.SplitN(string(buf), "\r\n", 2)[0]:
				default:
				}
				if tc, ok := c.(*net.TCPConn); ok {
					_ = tc.SetLinger(0)
				}
				_ = c.Close()
			}(c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String(), ch
}

// TestWebhookCall_aResetDuringTheBodyIsUnknown is F4 of the adversary's pass
// over this paper: the receiver read the request line and headers, then reset
// the connection while a 32 MiB body was being written. The client sees a bare
// write error («broken pipe» / «connection reset»), WroteRequest reports it,
// and today the run closes FAILED through CloseStateAfterRun's default arm —
// over a request whose URL reached the receiver. After a connection was
// obtained, any error without a 2xx answer is OUTCOME_UNKNOWN.
//
// Planned probing mutation (after green): restrict the connected-and-failed
// arm to ended contexts only ⇒ this reddens.
func TestWebhookCall_aResetDuringTheBodyIsUnknown(t *testing.T) {
	t.Parallel()
	addr, headers := resettingListener(t)
	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{addr}, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	big := `{"pad":"` + strings.Repeat("A", 32<<20) + `"}`
	_, runErr := wc.Execute(context.Background(), "http://"+addr+"/trigger "+big)
	select {
	case line := <-headers:
		if !strings.HasPrefix(line, "POST /trigger") {
			t.Fatalf("the receiver read %q, not the POST's request line", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the receiver never read the headers — the row does not attack what it names")
	}
	if runErr == nil {
		t.Fatal("a reset connection answered success")
	}
	if errors.Is(runErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", runErr)
	}
	if got := CloseStateAfterRun(runErr); got != action.StateOutcomeUnknown {
		t.Fatalf("state = %v (err %v), want OUTCOME_UNKNOWN — the headers reached the receiver", got, runErr)
	}
}

// cancelWhenReceived returns a context the caller cancels the moment the
// receiver signals it read what the row names — never on a wall clock, so a
// loaded machine cannot fire the end before the receiver has read. `read`
// reports what arrived; a receiver that stays silent for a full minute
// cancels anyway and `read` reports "" so the row fails by name.
func cancelWhenReceived(t *testing.T, signal <-chan string) (context.Context, func() string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan string, 1)
	go func() {
		select {
		case s := <-signal:
			got <- s
		case <-time.After(time.Minute):
			got <- ""
		}
		cancel()
	}()
	t.Cleanup(cancel)
	var once sync.Once
	var v string
	return ctx, func() string {
		once.Do(func() { v = <-got })
		return v
	}
}

// deadWriteConn is a pooled connection whose writes fail with EPIPE and
// write zero bytes once `dead` is set: the peer went away while it sat idle.
type deadWriteConn struct {
	net.Conn
	dead *atomic.Bool
}

func (c deadWriteConn) Write(b []byte) (int, error) {
	if c.dead.Load() {
		return 0, &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}
	}
	return c.Conn.Write(b)
}

// TestWebhookCall_aDeadPooledConnIsUnknownButNotDelivered is P2-1 of the
// adversary's re-pass over this paper. A first POST succeeds and leaves its
// connection in the pool; the connection dies while idle (every write EPIPEs,
// zero bytes out) and every new dial is refused. The second POST obtains the
// pooled connection (GotConn fires, Reused=true), writes nothing, retries,
// and fails to re-dial. The receiver saw ONE request.
//
// A connection was obtained, so the table says OUTCOME_UNKNOWN — and the
// error must NOT claim delivery. Asserted positively: it IS ErrDeliveryUnknown,
// it is NOT ErrEffectDelivered, ErrDeliveryUnknown's text is EXACTLY
// deliveryUnknownText (a neutral sentence that claims neither delivery nor
// non-delivery — a denylist of words cannot enumerate every way to claim
// it), and the operator-visible error is EXACTLY
// "webhook_call: request failed: " + that sentence + ": " + the transport's
// error — nothing added between, nothing after — and never carries
// ErrEffectDelivered's text.
//
// At base 61ae582 it closed OUTCOME_UNKNOWN already, for the wrong reason: the
// tool's WroteRequest observation fired clean before the failed flush, so the
// error wrapped ErrEffectDelivered and read «the request was delivered». The
// red was the sentinel assertion, not the state.
//
// Planned probing mutations (after green): wrap every after-GotConn error in
// ErrEffectDelivered ⇒ the sentinel assertions redden; give ErrDeliveryUnknown
// a text such as «the request reached the receiver and its answer was lost»
// ⇒ the exact-text assertion reddens.
//
// Evidence level, honest: in-process, the real tool's own transport with its
// DialContext wrapped by the test (the wrapper is the attacking hand).
func TestWebhookCall_aDeadPooledConnIsUnknownButNotDelivered(t *testing.T) {
	t.Parallel()
	hits := &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{host}})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	tr, ok := wc.(*webhookCallTool).client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("the caged client's transport is not *http.Transport — the attack has no hand")
	}
	dial := tr.DialContext
	dead := &atomic.Bool{}
	dials := &atomic.Int64{}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dead.Load() {
			dials.Add(1)
			return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
		}
		c, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return deadWriteConn{Conn: c, dead: dead}, nil
	}

	if _, err := wc.Execute(context.Background(), srv.URL+` {"event":"first"}`); err != nil {
		t.Fatalf("the first POST, which fills the pool: %v", err)
	}
	dead.Store(true)

	var reused atomic.Bool
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Reused {
				reused.Store(true)
			}
		},
	})
	_, runErr := wc.Execute(ctx, srv.URL+` {"event":"second"}`)
	if runErr == nil {
		t.Fatal("a dead pooled connection and a refused re-dial answered success")
	}
	if !reused.Load() {
		t.Fatal("the second POST never obtained the pooled connection — the row does not attack what it names")
	}
	if dials.Load() == 0 {
		t.Fatal("the transport never re-dialled — the row does not attack what it names")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("the receiver saw %d requests, want exactly 1", n)
	}
	if errors.Is(runErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", runErr)
	}
	if got := CloseStateAfterRun(runErr); got != action.StateOutcomeUnknown {
		t.Fatalf("state = %v (err %v), want OUTCOME_UNKNOWN — a connection was obtained", got, runErr)
	}
	if errors.Is(runErr, ErrEffectDelivered) {
		t.Fatalf("err %v claims delivery (ErrEffectDelivered) over a request that wrote zero bytes", runErr)
	}
	if !errors.Is(runErr, errDeliveryUnknown) {
		t.Fatalf("err %v is not ErrDeliveryUnknown — a connection was obtained and no answer came", runErr)
	}
	if got := errDeliveryUnknown.Error(); got != deliveryUnknownText {
		t.Fatalf("ErrDeliveryUnknown's text = %q, want exactly %q", got, deliveryUnknownText)
	}
	// The whole operator-visible prefix, not a substring: a wrapper that adds
	// its own prose between the tool's prefix and the neutral sentence could
	// claim delivery with every other assertion green.
	// The WHOLE text, prefix to end (Delta 6): the tool's prefix, the neutral
	// sentence, and the transport's own error verbatim — nothing between and
	// nothing after («…; the host received it» must not fit).
	want := "webhook_call: request failed: " + deliveryUnknownText + `: Post "` + srv.URL + `": dial tcp: connection refused`
	if runErr.Error() != want {
		t.Fatalf("err text = %q, want exactly %q", runErr, want)
	}
	if strings.Contains(runErr.Error(), ErrEffectDelivered.Error()) {
		t.Fatalf("err text %q carries ErrEffectDelivered's sentence over a request that wrote zero bytes", runErr)
	}
}

// TestWebhookCall_anEarlyAnswerIsNotADeliveryClaim is the diff pass's P2-2: a
// receiver that answers 413 after the headers, without reading the 32 MiB
// body. The answer is a response the tool reads, so the class is
// ErrEffectDelivered and the state OUTCOME_UNKNOWN — but nothing proves the
// receiver read the whole request, so the text may not say «delivered». It
// must be exactly «the receiver answered and the answer was not a usable
// success».
//
// Planned probing mutation: restore the sentinel text «the request was
// delivered and …» ⇒ the exact-text assertion reddens.
func TestWebhookCall_anEarlyAnswerIsNotADeliveryClaim(t *testing.T) {
	t.Parallel()
	// The receiver reads 162 bytes of the body and answers: the earliness is
	// BY CONSTRUCTION of this handler (it never reads more), not observed at
	// the socket. The check that follows guards the construction against an
	// edit that makes the handler read the whole body.
	readBytes := make(chan int, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.ReadFull(r.Body, make([]byte, 162))
		readBytes <- n
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{host}, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	big := `{"pad":"` + strings.Repeat("A", 32<<20) + `"}`
	_, runErr := wc.Execute(context.Background(), srv.URL+"/hook "+big)
	select {
	case n := <-readBytes:
		if n != 162 {
			t.Fatalf("the handler read %d bytes, want exactly 162 — its construction changed, the row no longer attacks what it names", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the receiver never answered — the row does not attack what it names")
	}
	want := "webhook_call: HTTP 413 from " + host + ": tool: the receiver answered and the answer was not a usable success"
	if runErr == nil || runErr.Error() != want {
		t.Fatalf("err = %v, want exactly %q", runErr, want)
	}
	if !errors.Is(runErr, ErrEffectDelivered) || errors.Is(runErr, errNotSent) || errors.Is(runErr, errDeliveryUnknown) {
		t.Fatalf("err %v does not carry exactly the answered class", runErr)
	}
	if got := CloseStateAfterRun(runErr); got != action.StateOutcomeUnknown {
		t.Fatalf("state = %v, want OUTCOME_UNKNOWN", got)
	}
}
