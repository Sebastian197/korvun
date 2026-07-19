// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
)

const (
	discordTokenEnv = "SHELL_TEST_DISCORD_TOKEN"
	adminTokenEnv   = "SHELL_TEST_ADMIN_TOKEN"
)

// fakeChannel implements app.Channel with no transport (mirrors the
// internal/app e2e discipline: the suite never dials a real network).
type fakeChannel struct {
	name    string
	inbound chan *envelope.Envelope

	mu      sync.Mutex
	stopped bool
}

func newFakeChannel(name string) *fakeChannel {
	return &fakeChannel{name: name, inbound: make(chan *envelope.Envelope)}
}

func (f *fakeChannel) Name() string               { return f.name }
func (f *fakeChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (f *fakeChannel) Send(context.Context, *envelope.Envelope) error {
	return nil
}
func (f *fakeChannel) Receive(context.Context) (<-chan *envelope.Envelope, error) {
	return f.inbound, nil
}
func (f *fakeChannel) Start(context.Context) error { return nil }
func (f *fakeChannel) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.stopped {
		f.stopped = true
		close(f.inbound)
	}
	return nil
}

// fakeFactory boots the real App with fake channels (FR-8b seam).
func fakeFactory() app.Option {
	return app.WithChannelFactory(func(cc config.ChannelConfig) (app.Channel, error) {
		return newFakeChannel(cc.Type), nil
	})
}

// failingFactory simulates a boot failure (AS-8): the channel cannot be built.
func failingFactory(err error) app.Option {
	return app.WithChannelFactory(func(config.ChannelConfig) (app.Channel, error) {
		return nil, err
	})
}

// minimalCfg is the e2e minimal config pattern (one channel, one brain, one
// route) with the admin block the builder precondition requires and the
// observability address PINNED to the conventional port — the ephemeral-port
// policy must override it in memory (AS-4).
func minimalCfg(ollamaURL string) *config.Config {
	return &config.Config{
		Channels: []config.ChannelConfig{{Type: "discord", Mode: "gateway", TokenEnv: discordTokenEnv}},
		Brains: []config.BrainConfig{{
			Name:        "b",
			Sensitivity: "public",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2", Locality: "local", BaseURL: ollamaURL},
			},
		}},
		Routes:        []config.RouteConfig{{Channel: "discord", Brain: "b"}},
		Admin:         &config.AdminConfig{TokenEnv: adminTokenEnv},
		Observability: &config.ObservabilityConfig{Addr: "127.0.0.1:2112"},
	}
}

func writeCfg(t *testing.T, cfg *config.Config) string {
	t.Helper()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func fakeOllama(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testController(extra ...app.Option) *Controller {
	opts := []Option{WithLogger(slog.New(slog.DiscardHandler))}
	if len(extra) > 0 {
		opts = append(opts, WithBuildOptions(extra...))
	}
	return New(opts...)
}

// startedController loads the minimal config and starts the core, failing the
// test on any boot error. Env hygiene rides t.Setenv's cleanup.
func startedController(t *testing.T, ollamaURL string) (*Controller, string) {
	t.Helper()
	t.Setenv(adminTokenEnv, "")
	c := testController(fakeFactory())
	path := writeCfg(t, minimalCfg(ollamaURL))
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer scancel()
			_ = c.Stop(sctx)
		}
	})
	return c, path
}

// waitGoroutinesAtMost polls until the goroutine count settles at or below
// want, returning the final count (leak tripwire helper; no external deps).
func waitGoroutinesAtMost(want int, within time.Duration) int {
	deadline := time.Now().Add(within)
	n := runtime.NumGoroutine()
	for n > want && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

// settledGoroutines samples the goroutine count until it holds steady for a
// few consecutive samples (or the deadline hits) and returns it — the
// baseline capture for the leak tripwire.
func settledGoroutines(within time.Duration) int {
	deadline := time.Now().Add(within)
	last := runtime.NumGoroutine()
	stable := 0
	for stable < 3 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n == last {
			stable++
		} else {
			stable = 0
			last = n
		}
	}
	return last
}

func TestLoadConfig_missingFile_namesFileAndStaysStopped(t *testing.T) {
	c := testController()
	path := filepath.Join(t.TempDir(), "nope.json")
	err := c.LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig on a missing file: want error, got nil")
	}
	if !strings.Contains(err.Error(), "nope.json") {
		t.Fatalf("error does not name the file: %v", err)
	}
	if c.Status().Running {
		t.Fatal("Status.Running true after a failed LoadConfig")
	}
}

func TestStart_withoutConfig_errNoConfig(t *testing.T) {
	c := testController()
	if err := c.Start(context.Background()); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("Start without config: want ErrNoConfig, got %v", err)
	}
}

func TestStop_whileStopped_errNotRunning(t *testing.T) {
	c := testController()
	if err := c.Stop(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop while stopped: want ErrNotRunning, got %v", err)
	}
}

// TestLifecycle_e2e drives the full AS-3/AS-4/AS-5/AS-7 contract on one real
// core: ephemeral admin port with the file intact, mounted builder mutation
// surface behind the per-cycle bearer, guard errors, and a clean stop that
// closes the listener and unsets the bearer.
func TestLifecycle_e2e(t *testing.T) {
	srv := fakeOllama(t)
	c, path := startedController(t, srv.URL)

	fileBefore, err := os.ReadFile(path) // #nosec G304 -- t.TempDir-controlled test path
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}

	st := c.Status()
	if !st.Running {
		t.Fatal("Status.Running false after Start")
	}
	if st.ConfigPath != path {
		t.Fatalf("Status.ConfigPath = %q, want %q", st.ConfigPath, path)
	}

	// AS-4: the effective address is loopback on an EPHEMERAL port, never the
	// pinned 2112 from the file.
	host, port, err := net.SplitHostPort(st.AdminAddr)
	if err != nil {
		t.Fatalf("AdminAddr %q is not host:port: %v", st.AdminAddr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("AdminAddr host = %q, want 127.0.0.1", host)
	}
	if port == "2112" || port == "0" {
		t.Fatalf("AdminAddr port = %q, want a real ephemeral port", port)
	}

	// Liveness on the effective address.
	resp, err := http.Get("http://" + st.AdminAddr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", resp.StatusCode)
	}

	// AS-3: the per-cycle bearer is a 32-byte crypto/rand hex token...
	token := os.Getenv(adminTokenEnv)
	if raw, derr := hex.DecodeString(token); derr != nil || len(raw) != 32 {
		t.Fatalf("bearer env %q: want 64-char hex (32 bytes), got %q", adminTokenEnv, token)
	}
	// ...and the builder's mutation surface is MOUNTED behind it: 401 bare,
	// 200 with the bearer (the WithReloader + token precondition).
	req, _ := http.NewRequest(http.MethodGet, "http://"+st.AdminAddr+"/api/config", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/config (bare): %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bare /api/config status = %d, want 401", resp.StatusCode)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/config (bearer): %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bearer /api/config status = %d, want 200", resp.StatusCode)
	}

	// AS-7 guards while running.
	if err := c.Start(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start: want ErrAlreadyRunning, got %v", err)
	}
	if err := c.LoadConfig(path); !errors.Is(err, ErrRunning) {
		t.Fatalf("LoadConfig while running: want ErrRunning, got %v", err)
	}

	// AS-4: the file on disk never learns the ephemeral override.
	fileAfter, err := os.ReadFile(path) // #nosec G304 -- t.TempDir-controlled test path
	if err != nil {
		t.Fatalf("re-read config file: %v", err)
	}
	if string(fileBefore) != string(fileAfter) {
		t.Fatal("config file bytes changed across Start — the ephemeral override leaked to disk")
	}

	// AS-5: clean stop — listener closed, bearer unset, status stopped.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped := c.Status(); stopped.Running || stopped.AdminAddr != "" {
		t.Fatalf("Status after Stop = %+v, want stopped with empty AdminAddr", stopped)
	}
	if _, ok := os.LookupEnv(adminTokenEnv); ok {
		t.Fatalf("bearer env %q still set after Stop", adminTokenEnv)
	}
	// st still holds the RUNNING-time address: the closed listener must
	// refuse connections there.
	if conn, derr := net.DialTimeout("tcp", st.AdminAddr, time.Second); derr == nil {
		_ = conn.Close()
		t.Fatal("admin listener still accepting after Stop")
	}
}

// TestCycles_noLeak is the ADR-0035 §1 TRIPWIRE: N repeated Start/Stop cycles
// in one process must leak no goroutines and close every cycle's listener.
// This test failing is the documented trigger for fallback B (subprocess).
func TestCycles_noLeak(t *testing.T) {
	const cycles = 10
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")

	c := testController(fakeFactory())
	path := writeCfg(t, minimalCfg(srv.URL))
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	baseline := settledGoroutines(2 * time.Second)
	addrs := make([]string, 0, cycles)

	for i := 0; i < cycles; i++ {
		startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := c.Start(startCtx); err != nil {
			startCancel()
			t.Fatalf("cycle %d Start: %v", i, err)
		}
		startCancel()
		st := c.Status()
		if !st.Running || st.AdminAddr == "" {
			t.Fatalf("cycle %d Status = %+v, want running with an addr", i, st)
		}
		resp, err := http.Get("http://" + st.AdminAddr + "/healthz")
		if err != nil {
			t.Fatalf("cycle %d /healthz: %v", i, err)
		}
		_ = resp.Body.Close()
		addrs = append(addrs, st.AdminAddr)
		// Stop gets its OWN deadline: a slow teardown must surface as a Stop
		// timeout, never masquerade as the leak tripwire firing.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := c.Stop(stopCtx); err != nil {
			stopCancel()
			t.Fatalf("cycle %d Stop: %v", i, err)
		}
		stopCancel()
	}

	// Every cycle's listener is provably closed.
	for i, addr := range addrs {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			_ = conn.Close()
			t.Fatalf("cycle %d listener %s still accepting after Stop", i, addr)
		}
	}

	// Goroutines settle back to the pre-cycle baseline (small tolerance for
	// runtime jitter; a real per-cycle leak of even one goroutine would show
	// as baseline+10).
	const tolerance = 2
	final := waitGoroutinesAtMost(baseline+tolerance, 5*time.Second)
	t.Logf("tripwire: %d cycles, goroutines baseline=%d final=%d (tolerance +%d)",
		cycles, baseline, final, tolerance)
	if final > baseline+tolerance {
		t.Fatalf("goroutines after %d cycles = %d, baseline %d (+%d tolerance) — LEAK: fallback B trigger",
			cycles, final, baseline, tolerance)
	}
}

// TestStart_bootFailure_surfacesAndLeaksNothing is AS-8: a failing channel
// build (the missing-secret shape) surfaces a wrapped supervisor error, the
// controller stays stopped, and nothing half-started leaks.
func TestStart_bootFailure_surfacesAndLeaksNothing(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	bootErr := errors.New("channel token missing")

	c := testController(failingFactory(bootErr))
	path := writeCfg(t, minimalCfg(srv.URL))
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	baseline := settledGoroutines(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("Start with a failing channel build: want error, got nil")
	}
	if !strings.Contains(err.Error(), bootErr.Error()) {
		t.Fatalf("boot error does not carry the cause: %v", err)
	}
	if c.Status().Running {
		t.Fatal("Status.Running true after a failed boot")
	}
	if _, ok := os.LookupEnv(adminTokenEnv); ok {
		t.Fatal("bearer env still set after a failed boot")
	}
	const tolerance = 2
	if final := waitGoroutinesAtMost(baseline+tolerance, 5*time.Second); final > baseline+tolerance {
		t.Fatalf("goroutines after failed boot = %d, baseline %d — leak", final, baseline)
	}
}

// TestWithEphemeralAdmin_copySemantics is the FR-4 unit contract: the override
// applies to a copy, preserves Enabled semantics, creates a missing block, and
// never mutates the input (the persist seam depends on that).
func TestWithEphemeralAdmin_copySemantics(t *testing.T) {
	t.Parallel()
	off := false

	cases := []struct {
		name string
		in   *config.ObservabilityConfig
	}{
		{"pinned addr", &config.ObservabilityConfig{Addr: "127.0.0.1:2112"}},
		{"nil block", nil},
		{"explicit disabled", &config.ObservabilityConfig{Enabled: &off, Addr: "127.0.0.1:9"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := &config.Config{Observability: tc.in}
			out := withEphemeralAdmin(in)

			if out == in {
				t.Fatal("withEphemeralAdmin returned the same pointer — must copy")
			}
			if out.Observability == nil || out.Observability.Addr != "127.0.0.1:0" {
				t.Fatalf("override addr = %+v, want 127.0.0.1:0", out.Observability)
			}
			if in.Observability != tc.in {
				t.Fatal("input Observability pointer changed")
			}
			if tc.in != nil && tc.in.Addr == "127.0.0.1:2112" && in.Observability.Addr != "127.0.0.1:2112" {
				t.Fatal("input block mutated")
			}
			if tc.in != nil && tc.in.Enabled != nil {
				if out.Observability.Enabled == nil || *out.Observability.Enabled != *tc.in.Enabled {
					t.Fatal("Enabled semantics not preserved on the copy")
				}
			}
		})
	}
}

// TestReload_pristinePersistAndAddrRotation drives the builder's reload path
// end-to-end — the exact place the pristine-persist property matters: the
// build seam re-applies the ephemeral override to the incoming config while
// the persist seam writes the user's config (pinned addr, never :0) to disk.
func TestReload_pristinePersistAndAddrRotation(t *testing.T) {
	srv := fakeOllama(t)
	c, path := startedController(t, srv.URL)

	before := c.Status()
	token := os.Getenv(adminTokenEnv)

	// POST a mutated but valid config (a new model id marks the reload).
	mutated := minimalCfg(srv.URL)
	mutated.Brains[0].Models[0].ModelID = "llama3.2-reloaded"
	body, err := json.Marshal(mutated)
	if err != nil {
		t.Fatalf("marshal mutated config: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+before.AdminAddr+"/api/config", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/config: %v", err)
	}
	accepted, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/config status = %d (%s), want 202", resp.StatusCode, accepted)
	}
	var handle struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(accepted, &handle); err != nil || handle.Handle == "" {
		t.Fatalf("202 body %q: want a reload handle", accepted)
	}

	// Poll the handle to "succeeded". The admin address ROTATES on cutover
	// (new app, new ephemeral bind), so poll the CURRENT Status addr and
	// tolerate the transition window's connection errors.
	deadline := time.Now().Add(10 * time.Second)
	state := ""
	for time.Now().Before(deadline) && state != "succeeded" {
		addr := c.Status().AdminAddr
		if addr != "" {
			sReq, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/api/reload/"+handle.Handle, nil)
			if sResp, sErr := http.DefaultClient.Do(sReq); sErr == nil {
				var got struct {
					State string `json:"state"`
				}
				b, _ := io.ReadAll(sResp.Body)
				_ = sResp.Body.Close()
				_ = json.Unmarshal(b, &got)
				state = got.State
				if state == "failed" || state == "rolled-back" {
					t.Fatalf("reload ended in state %q", state)
				}
			}
		}
		if state != "succeeded" {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if state != "succeeded" {
		t.Fatalf("reload did not reach succeeded within deadline (last state %q)", state)
	}

	// The file on disk holds the user's PRISTINE mutated config: the pinned
	// observability addr survived and the ephemeral override never leaked.
	onDisk, err := os.ReadFile(path) // #nosec G304 -- t.TempDir-controlled test path
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if !strings.Contains(string(onDisk), "llama3.2-reloaded") {
		t.Fatal("persisted config lacks the mutation — persist did not run")
	}
	if !strings.Contains(string(onDisk), "127.0.0.1:2112") {
		t.Fatal("persisted config lost the user's pinned observability addr")
	}
	if strings.Contains(string(onDisk), "127.0.0.1:0") {
		t.Fatal("the ephemeral override leaked into the persisted config")
	}

	// The new core serves on the CURRENT effective address (rotation is not
	// asserted as inequality — the kernel may hand back the same port), and
	// the per-cycle bearer survived the reload's re-Build.
	after := c.Status()
	if !after.Running || after.AdminAddr == "" {
		t.Fatalf("Status after reload = %+v, want running with an addr", after)
	}
	hResp, err := http.Get("http://" + after.AdminAddr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz after reload: %v", err)
	}
	_ = hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz after reload = %d, want 200", hResp.StatusCode)
	}
	if got := os.Getenv(adminTokenEnv); got != token {
		t.Fatalf("bearer changed across reload: cycle token must survive a re-Build")
	}
}

// TestStatus_reapsDeadCore is the reap contract (white-box): a run goroutine
// that exited on its own must fold into a stopped state on the next Status,
// with the bearer cleared, instead of reading Running=true forever.
func TestStatus_reapsDeadCore(t *testing.T) {
	t.Setenv(adminTokenEnv, "sentinel")
	c := testController()
	closed := make(chan struct{})
	close(closed)
	c.running = true
	c.cancel = func() {}
	c.done = closed
	c.runErr = errors.New("rollback failed")
	c.tokenEnv = adminTokenEnv

	if st := c.Status(); st.Running {
		t.Fatal("Status did not reap the dead core")
	}
	if _, ok := os.LookupEnv(adminTokenEnv); ok {
		t.Fatal("bearer env still set after reap")
	}
	if err := c.Stop(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop after reap: want ErrNotRunning, got %v", err)
	}
}
