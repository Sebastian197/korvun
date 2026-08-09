// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command e2e-harness serves the BUILT desktop chrome and the SP4 same-origin
// admin proxy from one loopback origin, over a REAL no-network core,
// mirroring the Wails AssetServer semantics — assets first, handler on miss —
// so Playwright drives the real pipeline without a WebView (SP6 spec: the
// per-cut screenshot medium; the native WKWebView ride is SP8's hardware
// validation). Plain Go, no Wails import: it compiles in the default suite on
// every OS.
//
// SP6b additions, all under /__test/ (test-control surface, loopback-only by
// construction — the harness binds loopback):
//
//   - POST /__test/bindings/<Method> — the REAL shell.Desktop binding surface
//     over HTTP, mirroring Wails' generated JS (args as a JSON array, Go
//     error → {"error": ...} → Promise rejection). Playwright installs a
//     window.go.shell.Desktop shim on top, so the chrome exercises the same
//     THE-LAW-bounded surface the native window binds.
//   - POST /__test/inject {"text": ...} — feeds a real inbound Envelope into
//     the scripted fake channel; the message then rides the REAL router,
//     brain, and SSE bus (nothing is mocked past the transport edge).
//   - POST /__test/model {"mode": "ok"|"down"} — flips the in-harness fake
//     model endpoint. NOTE: a down model does NOT fail the handle — ADR-0031
//     degrades model failures to a fallback reply (still reply_sent); the
//     toggle exists for CheckOllama-style probes.
//   - POST /__test/channel {"send": "ok"|"fail"} — makes the scripted
//     channel's Send fail, the honest provocation of a real message_dropped
//     frame (router funnel: a failed Send is a message that did not complete
//     its path), which is one of FR-WIN-4's incident triggers.
//   - POST /__test/core-exit — stops the core WITHOUT going through the UI's
//     binding surface: from the chrome's point of view the core vanished out
//     from under it, exactly what the reap-driven incident state must catch.
//
// The core the harness boots carries ONE scripted telegram channel (the fake
// transport above; its token env var is set in-process to a dummy — no real
// secret, no network) and the template's private ollama brain pointed at the
// in-harness fake model endpoint.
//
// Usage: e2e-harness [-addr 127.0.0.1:43117] [-dist cmd/korvun-desktop/frontend/dist] [-start=false]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/shell"
)

// harnessTokenEnv is the dummy secret var the scripted channel references —
// set in-process to satisfy the core's env pre-check, never a real secret.
const harnessTokenEnv = "KORVUN_HARNESS_TELEGRAM_TOKEN" //nolint:gosec // an env-var NAME, not a credential

func main() {
	if err := run(); err != nil {
		slog.Error("harness failed", "error", err.Error())
		os.Exit(1)
	}
}

// fakeChannel is the scripted no-network transport (the shell test suite's
// pattern): Receive hands the router a real inbound stream the /__test/inject
// endpoint feeds.
type fakeChannel struct {
	name     string
	inbound  chan *envelope.Envelope
	sendFail atomic.Bool
	mu       sync.Mutex
	stopped  bool
}

func newFakeChannel(name string) *fakeChannel {
	return &fakeChannel{name: name, inbound: make(chan *envelope.Envelope)}
}

func (f *fakeChannel) Name() string               { return f.name }
func (f *fakeChannel) Manifest() channel.Manifest { return channel.Manifest{Text: true} }
func (f *fakeChannel) Send(context.Context, *envelope.Envelope) error {
	if f.sendFail.Load() {
		return errors.New("send failed (harness scripted outage)")
	}
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

// inject feeds one inbound envelope, bounded so a stopped core (no pump
// consuming) answers honestly instead of hanging the control endpoint.
// Every send attempt happens UNDER the mutex and non-blocking: Stop also
// closes the channel under the same mutex, so a send can never race the
// close (the TOCTOU the review caught — a lost race would panic the whole
// harness mid-suite, not 409).
func (f *fakeChannel) inject(text string) error {
	e := envelope.New(f.name, envelope.Inbound, envelope.Participant{ID: "u1", Name: "Harness"})
	e.AddText(text)
	e.Meta[conversation.MetaConversationID] = "harness-conv"
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		if f.stopped {
			f.mu.Unlock()
			return errors.New("channel stopped")
		}
		select {
		case f.inbound <- e:
			f.mu.Unlock()
			return nil
		default:
		}
		f.mu.Unlock()
		if time.Now().After(deadline) {
			return errors.New("no consumer (core stopped?)")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// memSecrets is the harness keychain double — the real OS keychain must never
// be touched from a test tool.
type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return "", shell.ErrSecretNotFound
	}
	return v, nil
}

func (s *memSecrets) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[name] = value
	return nil
}

func (s *memSecrets) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
	return nil
}

// fakeModel is the toggleable in-harness model endpoint: ok → a real chat
// completion; down → 500, so the brain's handle genuinely fails.
type fakeModel struct {
	down atomic.Bool
	srv  *http.Server
	url  string
}

func newFakeModel() (*fakeModel, error) {
	fm := &fakeModel{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("fake model listen: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, _ *http.Request) {
		if fm.down.Load() {
			http.Error(w, "model down (harness)", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"message":{"role":"assistant","content":"Respuesta del asistente (harness)."},"done":true}`)
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		if fm.down.Load() {
			http.Error(w, "model down (harness)", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"models":[]}`)
	})
	fm.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fm.url = "http://" + ln.Addr().String()
	go func() { _ = fm.srv.Serve(ln) }()
	return fm, nil
}

// harnessConfig is the channel-ful variant of the SP5 template: one scripted
// telegram channel + the template's private ollama brain pointed at the fake
// model endpoint.
func harnessConfig(modelURL string) *config.Config {
	return &config.Config{
		Channels: []config.ChannelConfig{
			{Type: defaultChannel, Mode: "polling", TokenEnv: harnessTokenEnv},
			// The direct-chat channel (FR-CONS): no secret, no mode; it
			// auto-routes to the first brain (FR-CONS-2).
			{Type: "console"},
		},
		Brains: []config.BrainConfig{{
			Name:        "asistente",
			Sensitivity: "private",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2:1b", Locality: "local", BaseURL: modelURL},
			},
		}},
		Routes: []config.RouteConfig{{Channel: "telegram", Brain: "asistente"}},
		Admin:  &config.AdminConfig{TokenEnv: "KORVUN_ADMIN_TOKEN"},
		// SP4 (operator-console): durable store + sessions so the console
		// API mounts and /__test/inject produces real persisted history.
		// Storage.Path empty resolves under the harness's ISOLATED temp
		// HOME (set below), so nothing touches the developer's real data.
		Storage:       &config.StorageConfig{},
		Session:       &config.SessionConfig{},
		Observability: &config.ObservabilityConfig{Enabled: boolPtr(true)},
	}
}

func boolPtr(b bool) *bool { return &b }

// requireLoopback refuses any bind address whose host is not a loopback IP
// (or "localhost"), making the harness's loopback guarantee real.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid -addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("refusing to bind %q: the harness serves a test-control surface and a bearer-injecting proxy, loopback only", addr)
	}
	return nil
}

// run keeps every deferred cleanup on the exit path (no os.Exit skipping the
// temp-dir removal).
func run() error {
	addr := flag.String("addr", "127.0.0.1:43117", "loopback address to serve on")
	dist := flag.String("dist", filepath.Join("cmd", "korvun-desktop", "frontend", "dist"),
		"path to the built chrome bundle")
	autostart := flag.Bool("start", true, "start the core on boot")
	fresh := flag.Bool("fresh", false,
		"fresh-install mode (SP6c onboarding e2e): HOME/XDG_CONFIG_HOME point at a "+
			"temp dir and NO config is written or loaded — EnsureDefaultConfig's "+
			"created=true is real, so the onboarding runs for real")
	agentConfig := flag.String("agent-config", "",
		"path to an operator config to run INSTEAD of the scripted default (the "+
			"governed-tools round harness): it is copied to the isolated HOME's "+
			"config path verbatim, so a real Ollama and a governed agent brain can "+
			"drive the real chrome under Playwright. The scripted telegram channel "+
			"still applies to any telegram entry referencing the harness token env.")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// "Loopback-only" is ENFORCED, not a flag default (review finding): the
	// /__test/ surface drives the core and the proxy injects the admin
	// bearer — neither may ever face a network.
	if err := requireLoopback(*addr); err != nil {
		return err
	}

	if err := os.Setenv(harnessTokenEnv, "harness-dummy"); err != nil {
		return fmt.Errorf("set dummy channel token: %w", err)
	}

	fm, err := newFakeModel()
	if err != nil {
		return err
	}
	defer func() { _ = fm.srv.Close() }()

	// The scripted channel registry: the factory records what the core
	// builds so /__test/inject can reach the live instance of the CURRENT
	// cycle (a restart builds a fresh one).
	var chanMu sync.Mutex
	channels := map[string]*fakeChannel{}
	factory := app.WithChannelFactory(func(cc config.ChannelConfig) (app.Channel, error) {
		// Mirror the REAL factory's secret pre-check (app.Build:
		// os.Getenv(cc.TokenEnv) == "" -> ErrMissingSecret). Without this the
		// harness bypassed the check and the SP6c wizard e2e never exercised
		// keychain provisioning — the F1 gap. Now the wizard e2e is a genuine
		// regression test: adding a channel whose secret lives only in the
		// keychain double only connects because the reload seam re-provisions it.
		if cc.TokenEnv != "" && os.Getenv(cc.TokenEnv) == "" {
			return nil, fmt.Errorf("%w: %q", app.ErrMissingSecret, cc.TokenEnv)
		}
		if cc.Type == "console" {
			// (nil, nil): the app builds the REAL console channel — the
			// direct chat is in-process, nothing to script (FR-CONS).
			return nil, nil
		}
		fc := newFakeChannel(cc.Type)
		chanMu.Lock()
		channels[cc.Type] = fc
		chanMu.Unlock()
		return fc, nil
	})

	ctrl := shell.New(
		shell.WithLogger(logger),
		shell.WithBuildOptions(factory),
		shell.WithSecretStore(&memSecrets{}),
	)
	desk := shell.NewDesktop(ctrl, shell.WithDesktopLogger(logger))

	dir, err := os.MkdirTemp("", "korvun-harness-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// ALWAYS isolate the default-config location into the temp dir
	// (os.UserConfigDir reads HOME on darwin, XDG_CONFIG_HOME on linux). The
	// chrome calls EnsureDefaultConfig(DefaultConfigPath) on mount; without
	// isolation it would hit the developer's or runner's REAL path — creating
	// a config on a clean HOME and wrongly triggering onboarding over the
	// running-core harness.
	if err := os.Setenv("HOME", dir); err != nil {
		return fmt.Errorf("harness HOME: %w", err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config")); err != nil {
		return fmt.Errorf("harness XDG_CONFIG_HOME: %w", err)
	}

	cfgPath, err := shell.DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("resolve default config path: %w", err)
	}
	if !*fresh {
		// Non-fresh: write the harness config (scripted channel + fake model)
		// AT the default path so the chrome's EnsureDefaultConfig sees it
		// exists (created=false → no onboarding), then load it. With
		// -agent-config, the operator's file is copied verbatim instead — the
		// governed-tools round rides the real chrome over a real model.
		if *agentConfig != "" {
			raw, err := os.ReadFile(*agentConfig)
			if err != nil {
				return fmt.Errorf("read -agent-config: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
				return fmt.Errorf("mkdir config dir: %w", err)
			}
			if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
				return fmt.Errorf("write -agent-config copy: %w", err)
			}
		} else if err := writeScriptedConfig(cfgPath, fm.url); err != nil {
			return err
		}
		if err := ctrl.LoadConfig(cfgPath); err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	}
	if *autostart && !*fresh {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := ctrl.Start(ctx); err != nil {
			cancel()
			return fmt.Errorf("start core: %w", err)
		}
		cancel()
	}

	testAPI := testControl{
		desk: desk, ctrl: ctrl, model: fm, channels: channels, chanMu: &chanMu,
		listenAddr: *addr, cfgPath: cfgPath, modelURL: fm.url, fresh: *fresh,
	}
	proxy := ctrl.ProxyHandler()
	files := http.FileServer(http.Dir(*dist))
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/__test/") {
			testAPI.ServeHTTP(w, r)
			return
		}
		// Assets first, handler on miss — the AssetServer's semantics. The
		// chrome is a single-page app: "/" is index.html; anything that is
		// not a real file under dist/ falls through to the proxy. The path
		// is cleaned BEFORE the stat so ../ can never probe outside dist
		// (http.FileServer would contain the serve anyway; the routing
		// probe must be equally contained).
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p == "" {
			files.ServeHTTP(w, r)
			return
		}
		if info, err := os.Stat(filepath.Join(*dist, filepath.FromSlash(p))); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("harness serving", "addr", "http://"+*addr)
		serveErr <- srv.ListenAndServe()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-sig:
	}
	sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	_ = srv.Shutdown(sctx)
	if ctrl.Status().Running {
		// Its own budget: a slow HTTP drain must not starve the core's stop.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = ctrl.Stop(stopCtx)
	}
	return nil
}

// defaultChannel is the scripted channel harnessConfig registers.
const defaultChannel = "telegram"

// writeScriptedConfig writes the one-telegram harness config (pointed at the
// fake model) to path, creating the parent dir.
func writeScriptedConfig(path, modelURL string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	cfgBytes, err := json.MarshalIndent(harnessConfig(modelURL), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, cfgBytes, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// testControl is the /__test/ surface (see the package comment).
type testControl struct {
	desk       *shell.Desktop
	ctrl       *shell.Controller
	model      *fakeModel
	channels   map[string]*fakeChannel
	chanMu     *sync.Mutex
	listenAddr string
	cfgPath    string
	modelURL   string
	fresh      bool
}

// lookupChannel resolves a scripted channel by name ("" → the default one).
func (tc testControl) lookupChannel(name string) *fakeChannel {
	if name == "" {
		name = defaultChannel
	}
	tc.chanMu.Lock()
	defer tc.chanMu.Unlock()
	return tc.channels[name]
}

func (tc testControl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	// Same-machine browser CSRF hardening (review finding): a cross-origin
	// "simple request" from a random page in a local browser can POST to
	// loopback, but it cannot set application/json without a preflight the
	// harness never answers, and its Host would be the rebound name.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	if r.Host != tc.listenAddr {
		http.Error(w, "wrong Host for the test-control surface", http.StatusForbidden)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/__test/bindings/"):
		tc.bindings(w, r, strings.TrimPrefix(r.URL.Path, "/__test/bindings/"))
	case r.URL.Path == "/__test/inject":
		tc.injectMessage(w, r)
	case r.URL.Path == "/__test/model":
		tc.modelMode(w, r)
	case r.URL.Path == "/__test/channel":
		tc.channelMode(w, r)
	case r.URL.Path == "/__test/core-exit":
		tc.coreExit(w)
	case r.URL.Path == "/__test/reset-config":
		tc.resetConfig(w)
	case r.URL.Path == "/__test/fresh-reset":
		tc.freshReset(w)
	default:
		http.Error(w, "unknown test endpoint", http.StatusNotFound)
	}
}

func (tc testControl) injectMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	fc := tc.lookupChannel(body.Channel)
	if fc == nil {
		http.Error(w, "no such scripted channel", http.StatusConflict)
		return
	}
	if err := fc.inject(body.Text); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (tc testControl) modelMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	switch body.Mode {
	case "ok":
		tc.model.down.Store(false)
	case "down":
		tc.model.down.Store(true)
	default:
		http.Error(w, `mode must be "ok" or "down"`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (tc testControl) channelMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
		Send    string `json:"send"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	fc := tc.lookupChannel(body.Channel)
	if fc == nil {
		http.Error(w, "no such scripted channel", http.StatusConflict)
		return
	}
	switch body.Send {
	case "ok":
		fc.sendFail.Store(false)
	case "fail":
		fc.sendFail.Store(true)
	default:
		http.Error(w, `send must be "ok" or "fail"`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// coreExit stops the core WITHOUT the UI's binding surface — the chrome's
// next poll sees the flip with no user action, the reap-shaped signal the
// incident state must catch (FR-WIN-4's honest trigger).
func (tc testControl) coreExit(w http.ResponseWriter) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := tc.ctrl.Stop(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resetConfig restores the ONE-telegram scripted config (non-fresh harness):
// stop the core if running, re-write the pristine config at the default path
// (a prior test's reload may have persisted a mutated one), and reload it —
// so each serial test and each Playwright retry starts from a known state,
// not the leftover of a test that failed mid-flight (review finding).
func (tc testControl) resetConfig(w http.ResponseWriter) {
	if tc.fresh {
		http.Error(w, "reset-config is for the non-fresh harness", http.StatusBadRequest)
		return
	}
	if tc.ctrl.Status().Running {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = tc.ctrl.Stop(ctx)
		cancel()
	}
	if err := writeScriptedConfig(tc.cfgPath, tc.modelURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tc.ctrl.LoadConfig(tc.cfgPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// freshReset deletes the config at the default path (fresh harness) so the
// chrome's next EnsureDefaultConfig returns created=true again — making the
// onboarding re-establishable per Playwright attempt (review finding).
func (tc testControl) freshReset(w http.ResponseWriter) {
	if !tc.fresh {
		http.Error(w, "fresh-reset is for the fresh harness", http.StatusBadRequest)
		return
	}
	if err := os.Remove(tc.cfgPath); err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bindings dispatches one Desktop method call, Wails-like: args as a JSON
// array, reply {"result": ...} or {"error": "..."} — the Playwright shim
// turns the latter into a Promise rejection, exactly like a Go error through
// the generated Wails bindings.
func (tc testControl) bindings(w http.ResponseWriter, r *http.Request, method string) {
	var args []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, "args must be a JSON array: "+err.Error(), http.StatusBadRequest)
		return
	}
	str := func(i int) (string, error) {
		if i >= len(args) {
			return "", fmt.Errorf("missing arg %d", i)
		}
		var s string
		if err := json.Unmarshal(args[i], &s); err != nil {
			return "", fmt.Errorf("arg %d: %w", i, err)
		}
		return s, nil
	}

	var result any
	var callErr error
	switch method {
	case "Start":
		callErr = tc.desk.Start()
	case "Stop":
		callErr = tc.desk.Stop()
	case "Status":
		result, callErr = tc.desk.Status()
	case "Version":
		result = tc.desk.Version()
	case "LoadConfig":
		var p string
		if p, callErr = str(0); callErr == nil {
			callErr = tc.desk.LoadConfig(p)
		}
	case "DefaultConfigPath":
		result, callErr = tc.desk.DefaultConfigPath()
	case "EnsureDefaultConfig":
		result, callErr = tc.desk.EnsureDefaultConfig()
	case "SetSecret":
		var name, value string
		if name, callErr = str(0); callErr == nil {
			if value, callErr = str(1); callErr == nil {
				callErr = tc.desk.SetSecret(name, value)
			}
		}
	case "DeleteSecret":
		var name string
		if name, callErr = str(0); callErr == nil {
			callErr = tc.desk.DeleteSecret(name)
		}
	case "CheckOllama":
		var base string
		if base, callErr = str(0); callErr == nil {
			result = tc.desk.CheckOllama(base)
		}
	case "CheckSecretPresence":
		var name string
		if name, callErr = str(0); callErr == nil {
			result, callErr = tc.desk.CheckSecretPresence(name)
		}
	default:
		http.Error(w, "unknown binding "+method, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if callErr != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": callErr.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
}
