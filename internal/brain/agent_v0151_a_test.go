// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The brain path's half of v0.15.1 block A's one table: A2 (only 2xx is
// success) and P2-4 with its cancellation sister (an end before a connection was
// obtained is a decided FAILED). The approved path's half lives in
// internal/app; both must name the SAME state for the same wire fact.
//
// Evidence level, honest: in-process, the REAL webhook_call tool through the
// REAL runTool seam, against real loopback sockets. Not a compiled binary.
package brain

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/tool"
)

// webhookHarnessTimeout is webhookHarness with the tool's own timeout set.
func webhookHarnessTimeout(t *testing.T, host string, timeout time.Duration) (*AgentBrain, *resultFakeRecorder) {
	t.Helper()
	wc, err := tool.WebhookCall(tool.WebhookCallConfig{AllowHosts: []string{host}, Timeout: timeout})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	journal := &[]string{}
	rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"webhook_call": wc},
		WithAgentLogger(quietLogger()), WithActionRecorder(rec))
	return a, rec
}

// TestRunTool_onlyA2xxIsSuccess is A2's mould on the brain path: 301 without
// Location is the reproduction; 200 is the control that must stay SUCCEEDED.
//
// Planned probing mutation (after green): restore `resp.StatusCode >= 400` as
// the tool's only non-success test ⇒ the 301 row reddens.
func TestRunTool_onlyA2xxIsSuccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code int
		want action.State
	}{
		{"301 without Location", http.StatusMovedPermanently, action.StateOutcomeUnknown},
		{"200", http.StatusOK, action.StateSucceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			read := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				select {
				case read <- string(b):
				default:
				}
				w.WriteHeader(tc.code)
			}))
			srv.Config.ErrorLog = log.New(io.Discard, "", 0)
			defer srv.Close()

			a, rec := webhookHarness(t, strings.TrimPrefix(srv.URL, "http://"))
			a.runTool(context.Background(), kernelEnv(), nil, laneText, "webhook_call", srv.URL+` {"event":"ping"}`)
			select {
			case got := <-read:
				if !strings.Contains(got, "ping") {
					t.Fatalf("the host did not receive the payload: %q", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the host never read the POST — the row does not attack what it names")
			}
			if len(rec.finishes) != 1 || rec.finishes[0] != tc.want {
				t.Fatalf("HTTP %d: finishes = %v, want exactly [%v]", tc.code, rec.finishes, tc.want)
			}
		})
	}
}

// stallingTLS accepts TCP and never answers the TLS ClientHello.
func stallingTLS(t *testing.T) (string, <-chan struct{}) {
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

// TestRunTool_anEndBeforeAConnectionClosesFailed is P2-4's mould on the brain
// path, with the cancellation sister HANDOFF names for this path: a TLS
// handshake that never completes, then either the tool's short deadline or the
// caller's cancellation. No connection was obtained, so no request byte
// could leave: FAILED.
//
// Planned probing mutation (after green): classify every ended context as
// unknown again in CloseStateAfterRun ⇒ both rows redden.
func TestRunTool_anEndBeforeAConnectionClosesFailed(t *testing.T) {
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
			addr, accepted := stallingTLS(t)
			timeout := 300 * time.Millisecond
			if tc.cancel {
				timeout = 30 * time.Second
			}
			a, rec := webhookHarnessTimeout(t, addr, timeout)
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
			obs := a.runTool(ctx, kernelEnv(), nil, laneText, "webhook_call", "https://"+addr+` {"event":"ping"}`)
			if len(rec.finishes) != 1 || rec.finishes[0] != action.StateFailed {
				t.Fatalf("finishes = %v, want exactly [FAILED] — no connection was obtained", rec.finishes)
			}
			// The positive class assertion (Delta 7), by the only channel this
			// path hands out: the observation string carries the error's text.
			if !strings.Contains(obs, tool.ErrNotSent.Error()) {
				t.Fatalf("the observation %q does not carry ErrNotSent's text %q", obs, tool.ErrNotSent.Error())
			}
		})
	}
}

// resettingListenerBrain reads the request line and headers, then resets the
// connection (SO_LINGER 0) while the client is still writing the body.
func resettingListenerBrain(t *testing.T) (string, <-chan string) {
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

// TestRunTool_aResetDuringTheBodyIsUnknown is F4 on the brain path: the
// receiver read the request line and headers, then reset while a 32 MiB body
// was being written. It must close OUTCOME_UNKNOWN, never FAILED.
//
// Planned probing mutation (after green): restrict the connected-and-failed
// arm of CloseStateAfterRun to ended contexts ⇒ this reddens.
func TestRunTool_aResetDuringTheBodyIsUnknown(t *testing.T) {
	t.Parallel()
	addr, headers := resettingListenerBrain(t)
	a, rec := webhookHarnessTimeout(t, addr, 10*time.Second)
	big := `{"pad":"` + strings.Repeat("A", 32<<20) + `"}`
	a.runTool(context.Background(), kernelEnv(), nil, laneText, "webhook_call", "http://"+addr+"/trigger "+big)
	select {
	case line := <-headers:
		if !strings.HasPrefix(line, "POST /trigger") {
			t.Fatalf("the receiver read %q, not the POST's request line", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the receiver never read the headers — the row does not attack what it names")
	}
	if len(rec.finishes) != 1 || rec.finishes[0] != action.StateOutcomeUnknown {
		t.Fatalf("finishes = %v, want exactly [OUTCOME_UNKNOWN]", rec.finishes)
	}
}
