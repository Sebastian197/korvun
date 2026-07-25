// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// proxyServer serves c.ProxyHandler() over a real listener — the stand-in for
// the Wails AssetServer origin (recorders cannot exercise streaming).
func proxyServer(t *testing.T, c *Controller) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(c.ProxyHandler())
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, client *http.Client, url string, header http.Header) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, vs := range header {
		req.Header[k] = vs
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of %s: %v", url, err)
	}
	return resp, string(b)
}

// assertNoTokenLeak asserts the cycle bearer appears in NO part of a response
// the handler serves to the page — neither the body nor any header value
// (the token must never reach the DOM, ADR-0035 §4).
func assertNoTokenLeak(t *testing.T, resp *http.Response, body, token, path string) {
	t.Helper()
	if strings.Contains(body, token) {
		t.Fatalf("GET %s body leaks the cycle bearer", path)
	}
	for name, values := range resp.Header {
		for _, v := range values {
			if strings.Contains(v, token) {
				t.Fatalf("GET %s response header %s leaks the cycle bearer", path, name)
			}
		}
	}
}

func assertJSONError(t *testing.T, resp *http.Response, body string, wantStatus int, wantBody string) {
	t.Helper()
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", resp.StatusCode, wantStatus, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if body != wantBody {
		t.Fatalf("body = %q, want %q", body, wantBody)
	}
}

// AS-1: non-admin paths never reach the core — small honest 404.
func TestProxy_nonAdminPath_404(t *testing.T) {
	srv := proxyServer(t, testController())
	for _, path := range []string{"/", "/favicon.ico", "/uix", "/apix/config", "/builderoo", "/index.html"} {
		resp, body := get(t, srv.Client(), srv.URL+path, nil)
		assertJSONError(t, resp, body, http.StatusNotFound, `{"error":"not found"}`)
	}
}

// AS-2: with the core stopped every admin route answers the stable 503 —
// never a hang, panic, or refused connection.
func TestProxy_coreStopped_honest503(t *testing.T) {
	srv := proxyServer(t, testController())
	for _, path := range []string{"/healthz", "/metrics", "/api/brains", "/api/config", "/builder/", "/ui/", "/api", "/builder", "/ui"} {
		resp, body := get(t, srv.Client(), srv.URL+path, nil)
		assertJSONError(t, resp, body, http.StatusServiceUnavailable, `{"error":"core stopped"}`)
	}
}

// FR-PXY-5 edge: observability explicitly disabled → no admin server exists
// while running → same honest 503, not a dial to nowhere.
func TestProxy_observabilityDisabled_503(t *testing.T) {
	t.Setenv(adminTokenEnv, "")
	ollama := fakeOllama(t)
	disabled := false
	cfg := minimalCfg(ollama.URL)
	cfg.Observability.Enabled = &disabled
	c := testController(fakeFactory())
	if err := c.LoadConfig(writeCfg(t, cfg)); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer scancel()
		_ = c.Stop(sctx)
	})
	srv := proxyServer(t, c)
	resp, body := get(t, srv.Client(), srv.URL+"/healthz", nil)
	assertJSONError(t, resp, body, http.StatusServiceUnavailable, `{"error":"core stopped"}`)
}

// AS-3 + AS-5: a running core is reached same-origin through the proxy; the
// bearer is injected server-side (and overwrites anything the page sends);
// the token value never appears in any served body.
func TestProxy_running_forwardsAndInjectsBearer(t *testing.T) {
	ollama := fakeOllama(t)
	c, _ := startedController(t, ollama.URL)
	srv := proxyServer(t, c)
	token := os.Getenv(adminTokenEnv)
	if token == "" {
		t.Fatal("precondition: cycle bearer not in env")
	}

	checks := []struct {
		path       string
		wantInBody string
	}{
		{"/healthz", "ok"},
		{"/metrics", "korvun_"},
		{"/api/brains", `"name"`},
		{"/builder/", "Korvun · builder"}, // ties the response to the single embedded dist
		{"/ui/", "Korvun — live view"},
	}
	for _, tc := range checks {
		resp, body := get(t, srv.Client(), srv.URL+tc.path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 (body %q)", tc.path, resp.StatusCode, body)
		}
		if !strings.Contains(body, tc.wantInBody) {
			t.Fatalf("GET %s body %q does not contain %q", tc.path, body, tc.wantInBody)
		}
		assertNoTokenLeak(t, resp, body, token, tc.path)
	}

	// Non-admin paths keep answering the honest 404 while the core runs.
	resp404, body404 := get(t, srv.Client(), srv.URL+"/favicon.ico", nil)
	assertJSONError(t, resp404, body404, http.StatusNotFound, `{"error":"not found"}`)

	// Negative control: the same route hit DIRECTLY on the core without auth
	// is denied — so the proxy's 200 below can only come from injection.
	direct, _ := get(t, srv.Client(), "http://"+c.Status().AdminAddr+"/api/config", nil)
	if direct.StatusCode != http.StatusUnauthorized {
		t.Fatalf("direct unauthenticated GET /api/config = %d, want 401", direct.StatusCode)
	}

	resp, body := get(t, srv.Client(), srv.URL+"/api/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxied GET /api/config without client auth = %d, want 200 (body %q)", resp.StatusCode, body)
	}
	assertNoTokenLeak(t, resp, body, token, "/api/config")

	// A bogus page-supplied Authorization must be overwritten, not forwarded.
	resp, body = get(t, srv.Client(), srv.URL+"/api/config",
		http.Header{"Authorization": []string{"Bearer page-forged-value"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxied GET /api/config with bogus client auth = %d, want 200 (body %q)", resp.StatusCode, body)
	}
}

// AS-6: Start → forwarded; Stop → honest 503 and the old port is closed;
// Start again → the proxy follows ONLY the new cycle's addr and bearer.
func TestProxy_fullCycle_followsCurrentCycle(t *testing.T) {
	ollama := fakeOllama(t)
	c, _ := startedController(t, ollama.URL)
	srv := proxyServer(t, c)

	addrA := c.Status().AdminAddr
	tokenA := os.Getenv(adminTokenEnv)
	if resp, body := get(t, srv.Client(), srv.URL+"/healthz", nil); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("cycle 1 /healthz = %d %q, want 200 ok", resp.StatusCode, body)
	}

	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if err := c.Stop(sctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	resp, body := get(t, srv.Client(), srv.URL+"/healthz", nil)
	assertJSONError(t, resp, body, http.StatusServiceUnavailable, `{"error":"core stopped"}`)
	if conn, err := net.DialTimeout("tcp", addrA, time.Second); err == nil {
		_ = conn.Close()
		t.Fatalf("old admin addr %s still accepts connections after Stop", addrA)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("restart: %v", err)
	}
	addrB := c.Status().AdminAddr
	if addrB == addrA {
		// The kernel may legitimately hand the freed ephemeral port back to
		// the new bind; cycle once more instead of flaking on OS behavior.
		if err := c.Stop(ctx); err != nil {
			t.Fatalf("collision re-stop: %v", err)
		}
		if err := c.Start(ctx); err != nil {
			t.Fatalf("collision re-start: %v", err)
		}
		addrB = c.Status().AdminAddr
	}
	tokenB := os.Getenv(adminTokenEnv)
	if addrB == addrA {
		t.Fatalf("cycles keep reusing admin addr %s", addrA)
	}
	if tokenB == tokenA {
		t.Fatal("cycle 2 reused the cycle-1 bearer")
	}
	if resp, body := get(t, srv.Client(), srv.URL+"/healthz", nil); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("cycle 2 /healthz = %d %q, want 200 ok", resp.StatusCode, body)
	}
	// Negative control on the NEW core: unauthenticated direct access is
	// denied, so the proxied 200 below can only come from the proxy
	// injecting the CURRENT cycle's bearer at the NEW addr.
	if direct, _ := get(t, srv.Client(), "http://"+addrB+"/api/config", nil); direct.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cycle 2 direct unauthenticated GET /api/config = %d, want 401", direct.StatusCode)
	}
	if resp, body := get(t, srv.Client(), srv.URL+"/api/config", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("cycle 2 proxied /api/config = %d, want 200 (body %q)", resp.StatusCode, body)
	}
}

// AS-4: the streaming tripwire — the SP1 spike's verdict as a deterministic
// test. The upstream flushes one SSE event and then BLOCKS until the client
// acknowledges having READ it through the proxy. A buffering proxy never
// delivers the event while the upstream holds the stream open → timeout.
func TestProxy_streaming_incrementalNotBuffered(t *testing.T) {
	acks := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream ResponseWriter is not a Flusher")
			return
		}
		for i := 1; i <= 3; i++ {
			if _, err := io.WriteString(w, "data: msg\n\n"); err != nil {
				return
			}
			fl.Flush()
			// No upstream-side timer: only the CLIENT select below can fail the
			// test (deadlock-deterministic). A buffering proxy leaves the client
			// starved, its 5 s timer fires, the request context cancels, and
			// this select unblocks through r.Context().Done().
			select {
			case <-acks:
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer upstream.Close()

	srv := httptest.NewServer(proxyForTarget(upstream.Listener.Addr().String(), ""))
	defer srv.Close()

	streamCtx, streamCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer streamCancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("build stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}

	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if s := sc.Text(); strings.HasPrefix(s, "data:") {
				lines <- s
			}
		}
		close(lines)
	}()
	for i := 1; i <= 3; i++ {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("stream closed before event %d arrived", i)
			}
			if line != "data: msg" {
				t.Fatalf("event %d = %q, want %q", i, line, "data: msg")
			}
			acks <- struct{}{} // the upstream may proceed: delivery was incremental
		case <-time.After(5 * time.Second):
			t.Fatalf("event %d not delivered while the upstream held the stream open — the proxy buffers", i)
		}
	}
}

// AS-7: a target that died between the stopped-check and the dial surfaces
// the stable unreachable 503, never a raw transport error.
func TestProxy_deadUpstream_503Unreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := l.Addr().String()
	_ = l.Close()

	srv := httptest.NewServer(proxyForTarget(dead, ""))
	defer srv.Close()
	resp, body := get(t, srv.Client(), srv.URL+"/api/brains", nil)
	assertJSONError(t, resp, body, http.StatusServiceUnavailable, `{"error":"core unreachable"}`)
}

// AS-8: non-GET forwarding — the builder's save flow is a POST with a body.
// An invalid JSON document must reach the core's decoder and come back as
// the core's own 400: a 401 would mean the injection failed, a 404/503 that
// routing failed, and anything else that the body was not forwarded.
func TestProxy_post_forwardsBodyAndInjectsBearer(t *testing.T) {
	ollama := fakeOllama(t)
	c, _ := startedController(t, ollama.URL)
	srv := proxyServer(t, c)
	token := os.Getenv(adminTokenEnv)

	resp, err := srv.Client().Post(srv.URL+"/api/config", "application/json",
		strings.NewReader(`{"channels": not-json`))
	if err != nil {
		t.Fatalf("POST /api/config: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST response: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("proxied POST /api/config with a bad body = %d, want 400 (body %q)", resp.StatusCode, b)
	}
	assertNoTokenLeak(t, resp, string(b), token, "POST /api/config")
}

// FR-PXY-4 tail: with NO bearer this cycle, a client-supplied Authorization
// is STRIPPED, never forwarded — the page's headers are not credentials.
func TestProxy_noBearer_stripsClientAuthorization(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "auth=["+r.Header.Get("Authorization")+"]")
	}))
	defer upstream.Close()

	srv := httptest.NewServer(proxyForTarget(upstream.Listener.Addr().String(), ""))
	defer srv.Close()
	resp, body := get(t, srv.Client(), srv.URL+"/api/echo",
		http.Header{"Authorization": []string{"Bearer page-forged-value"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("echo upstream = %d, want 200", resp.StatusCode)
	}
	if body != "auth=[]" {
		t.Fatalf("upstream saw %q — client Authorization was forwarded", body)
	}
}

// AS-9 (race tripwire): concurrent proxied requests across a Stop must only
// ever observe the honest contract — 200 from the live core or one of the
// two stable 503 bodies — with no race, panic, or torn state (-race carries
// the real assertion).
func TestProxy_concurrentRequestsAcrossStop(t *testing.T) {
	ollama := fakeOllama(t)
	c, _ := startedController(t, ollama.URL)
	srv := proxyServer(t, c)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp, err := srv.Client().Get(srv.URL + "/healthz")
				if err != nil {
					t.Errorf("GET /healthz under load: %v", err)
					return
				}
				b, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					t.Errorf("read /healthz body under load: %v", err)
					return
				}
				body := string(b)
				switch resp.StatusCode {
				case http.StatusOK:
				case http.StatusServiceUnavailable:
					if body != `{"error":"core stopped"}` && body != `{"error":"core unreachable"}` {
						t.Errorf("503 with an off-contract body %q", body)
						return
					}
				default:
					t.Errorf("proxied /healthz = %d (body %q), want 200 or 503", resp.StatusCode, body)
					return
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if err := c.Stop(sctx); err != nil {
		t.Fatalf("Stop under load: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
