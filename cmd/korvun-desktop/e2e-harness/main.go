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

	"strconv"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/shell"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
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
// harnessConfig builds the harness profile. `approvals` adds the PARKING brain
// and turns the approvals surface on — and it is a flag, not a default,
// because a second brain is visible to every other spec on this harness: the
// approvals scenarios get their OWN instance so the existing ones keep seeing
// the profile they were written against.
func harnessConfig(modelURL string, approvals bool) *config.Config {
	cfg := &config.Config{
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
	if approvals {
		// The PARKING brain: an agent with a caged webhook_call and a ceiling
		// that reaches it, so the gate's five conditions are met and
		// brains_can_park is 1. Deliberately NOT routed — the screen is
		// exercised through /__test/park, and routing it would change what the
		// chat surface shows.
		cfg.Brains = append(cfg.Brains, config.BrainConfig{
			Name:        parkingBrain,
			Sensitivity: "private",
			Policy:      config.PolicyConfig{Kind: "priority", Order: []string{"ollama"}},
			Models: []config.ModelConfig{
				{Provider: "ollama", ModelID: "llama3.2:1b", Locality: "local", BaseURL: modelURL},
			},
			Agent: &config.AgentConfig{
				Tools:         []string{"webhook_call"},
				MaxIterations: 2,
				EffectCeiling: "critical",
				WebhookCall:   &config.WebhookCallToolConfig{AllowHosts: []string{"hooks.acme.io"}},
			},
		})
		cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
	}
	return cfg
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
	approvals := flag.Bool("approvals", false,
		"approvals mode: add the PARKING brain and turn the approvals surface on. "+
			"Its own instance, because a second brain is visible to every spec on "+
			"this harness and the existing ones were written against the profile "+
			"without it.")
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
		} else if err := writeScriptedConfig(cfgPath, fm.url, *approvals); err != nil {
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
		approvals: *approvals,
		desk:      desk, ctrl: ctrl, model: fm, channels: channels, chanMu: &chanMu,
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
func writeScriptedConfig(path, modelURL string, approvals bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	cfgBytes, err := json.MarshalIndent(harnessConfig(modelURL, approvals), "", "  ")
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
	approvals  bool
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
	case r.URL.Path == "/__test/park":
		tc.park(w, r)
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
	if err := writeScriptedConfig(tc.cfgPath, tc.modelURL, tc.approvals); err != nil {
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

// park is FR-TEST-1: it parks a REAL approval so the browser scenarios have a
// document to read.
//
// It writes through a SECOND connection to the same store file, the way the
// operator CLI does, and it parks through the REAL factory — envelope, bound
// request, sealed preview, canonical params. Nothing here fabricates a row: a
// hand-written INSERT would sail past every belt the screen exists to surface,
// and the scenarios would prove nothing.
//
// The law pin comes from the SAME resolution the adapter will use, because a
// row parked under any other pin reads `invalidated` on the first touch and
// every scenario would pass for the wrong reason.
func (tc testControl) park(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         string `json:"id"`
		Operation  string `json:"operation"`
		Params     string `json:"params"`
		Purpose    string `json:"purpose"`
		Channel    string `json:"channel"`
		TTLSeconds int    `json:"ttl_seconds"`
		// Authority parks the STRICT scenario. It carries only what an operator
		// configures — the intent's id, purpose and budget — and how many
		// ordinary starts to spend AFTER the park. Who requested, who acts and
		// through which chain are the store's to establish, not the test's to
		// dictate.
		Authority *struct {
			IntentID       string `json:"intent_id"`
			IntentPurpose  string `json:"intent_purpose"`
			SpendAfterPark int    `json:"spend_after_park"`
			Budget         struct {
				Kind  string `json:"kind"`
				Total *int64 `json:"total"`
			} `json:"budget"`
		} `json:"authority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		body.ID = "act_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if body.Operation == "" {
		body.Operation = "webhook_call"
	}
	if body.Params == "" {
		body.Params = `{"url":"https://hooks.acme.io/pedidos","body":"1"}`
		if body.Authority != nil {
			// The strict scenario goes through the production door, whose
			// analyzer speaks the REAL webhook_call grammar: the URL, one
			// space, then the JSON body.
			body.Params = `https://hooks.acme.io/pedidos {"pedido":1}`
		}
	}
	if body.Channel == "" {
		body.Channel = "telegram"
	}
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl == 0 {
		ttl = time.Hour
	}
	cfg := harnessConfig(tc.modelURL, tc.approvals)
	_, pin, err := app.ResolveApprovalLaw(cfg, parkingBrain)
	if err != nil {
		http.Error(w, "resolve the parking brain's law: "+err.Error(), http.StatusInternalServerError)
		return
	}
	store, err := actionsqlite.OpenOperator(app.StoragePath(cfg))
	if err != nil {
		http.Error(w, "open the store: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer func() { _ = store.Close() }()

	env := action.NewEnvelope(body.ID, "harness",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: body.Channel},
		action.Operation{Namespace: "tool", Name: body.Operation, Version: 1},
		body.Params, time.Now().UTC())
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_" + parkingBrain}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	bound, err := action.NewBoundApprovalRequest(env, body.Params, action.ApprovalContext{
		IntentPurpose: firstNonEmpty(body.Purpose, "avisar al webhook de pedidos"),
		GrantID:       "grant_harness", GrantDepth: 1, CostLine: "1 of 5",
		ToolCage:      body.Operation,
		Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible, DataEgress: true},
		HasDescriptor: true,
		LawVersion:    pin.Version, LawDigest: pin.Digest,
		Rule: "require_approval",
		Now:  time.Now().UTC(), TTL: ttl,
	})
	if err != nil {
		http.Error(w, "bind the approval: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if body.Authority != nil {
		parked, err := tc.parkStrict(r.Context(), cfg, store, pin, body.Operation, body.Params, body.Channel, ttl,
			body.Authority.IntentID, body.Authority.IntentPurpose, body.Authority.Budget.Kind,
			body.Authority.Budget.Total, body.Authority.SpendAfterPark)
		if err != nil {
			http.Error(w, "park authorized: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"approval_id": parked.ApprovalID,
			"action_id":   parked.ActionID,
			"digest": action.Digest(action.Operation{Namespace: "tool", Name: body.Operation, Version: 1},
				body.Params),
		})
		return
	} else if err := store.CreateApprovalRequest(r.Context(), bound); err != nil {
		http.Error(w, "park: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"approval_id": bound.Approval().ApprovalID,
		"action_id":   body.ID,
		"digest":      bound.Approval().ActionDigest,
	})
}

// parkStrict parks the strict scenario through ParkAuthorization — the door
// production parks through — over an authority built only through the store's
// exported doors: an active intent owned by one brain, a root grant that brain
// issues to itself, a delegation to the parking brain, and the execution binding
// that makes that leaf applicable. Identity evidence comes from the same
// resolver and issuers the boot derives from the config. NOTHING the screen
// shows about the authority is a string this function wrote into a snapshot:
// the store computes the requester, the actor, the chain and the remainder, and
// signs them.
//
// An earlier shape parked through a store door production never called and
// handed it a snapshot assembled from the request body, so the browser read back
// what the test had posted (the adversary's pass over piece 3 phase 3, F4). That
// door no longer exists.
//
// spendAfterPark ordinary strict starts are then committed against the same
// intent and the same grants, so the LIVE remainder differs from the parked one
// and a screen showing live data as if it were the snapshot is caught.
func (tc testControl) parkStrict(ctx context.Context, cfg *config.Config, store *actionsqlite.Store,
	pin actionsqlite.PolicyPin, operationName, params, channel string, ttl time.Duration,
	intentID, purpose, budgetKind string, total *int64, spendAfterPark int) (actionsqlite.AuthorityPendingResult, error) {
	var none actionsqlite.AuthorityPendingResult
	privateKey, err := app.EnsureSigningKey(ctx, store, filepath.Dir(app.StoragePath(cfg)))
	if err != nil {
		return none, fmt.Errorf("load profile key: %w", err)
	}
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(privateKey, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(privateKey, e)
		},
	)
	store.SetIntentV2Signer(
		func(c action.IntentContractV2) action.SignedIntentContractV2 {
			return action.SignIntentContractV2(privateKey, c)
		},
		func(e action.IntentEventV1) action.SignedIntentEventV1 {
			return action.SignIntentEventV1(privateKey, e)
		},
	)
	store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
		return action.SignAuthorityBytes(privateKey, domain, canonical)
	})
	resolver, issuers, err := app.IdentityRuntime(cfg)
	if err != nil {
		return none, fmt.Errorf("identity runtime: %w", err)
	}
	issuer := issuers[channel]
	if issuer == nil {
		return none, fmt.Errorf("no authenticated ingress for channel %q", channel)
	}
	budget := action.IntentBudgetV2{}
	switch budgetKind {
	case string(action.AuthorizationBudgetFinite):
		if total == nil {
			return none, errors.New("a finite authority budget needs its total")
		}
		budget.Total = total
	case string(action.AuthorizationBudgetUnlimited):
	default:
		return none, fmt.Errorf("unknown authority budget %q", budgetKind)
	}

	const ownerBrain = "asistente"
	owner, actor := "principal_brain_"+ownerBrain, "principal_brain_"+parkingBrain
	operation := action.Operation{Namespace: "tool", Name: operationName, Version: 1}
	operations := []action.OperationRef{{Namespace: "tool", Name: operationName, Version: 1}}
	effects := []action.EffectClass{action.EffectWriteIrreversible}
	now := time.Now().UTC()
	sequence := 0
	// evidenceFor mints one ingress and returns the resolver bound to it, the
	// way an adapter authenticates one request for one brain.
	evidenceFor := func(brain string) (string, func(string) (identity.Evidence, error), error) {
		sequence++
		requestID := fmt.Sprintf("harness-%s-%d-%d", intentID, now.UnixNano(), sequence)
		ingress, err := issuer.Issue(requestID, "harness-subject")
		if err != nil {
			return "", nil, err
		}
		return requestID, func(actionID string) (identity.Evidence, error) {
			return resolver.Resolve(ingress, identity.ResolveRequest{
				ActionID: actionID, RequestID: requestID, Channel: channel, Brain: brain,
			})
		}, nil
	}
	// actorAct records the owner brain's own authenticated act for one
	// authority verb: the one-shot act every authority mutation answers to.
	actorAct := func(verb string, canonical []byte) (string, error) {
		requestID, resolve, err := evidenceFor(ownerBrain)
		if err != nil {
			return "", err
		}
		actID := fmt.Sprintf("act_harness_%s_%d", verb, now.UnixNano())
		evidence, err := resolve(actID)
		if err != nil {
			return "", err
		}
		env := action.NewEnvelope(actID, requestID,
			action.Source{Kind: "agent_brain", Protocol: "text", Channel: channel},
			action.Operation{Namespace: "authority", Name: verb, Version: 1}, string(canonical), now)
		env.Effect = action.Effect{Class: string(action.EffectWriteReversible)}
		env.Principal = action.PrincipalRef{PrincipalID: evidence.ActorPrincipalID,
			ResponsibleHumanID: evidence.ResponsiblePrincipalID, EvidenceID: evidence.EvidenceID}
		return actID, store.RecordAttemptAuthenticated(ctx, env,
			actionsqlite.Decision{Outcome: "allow", Rule: "administrative"}, action.StateAuthorized, evidence)
	}

	// The scope an operator would write for this webhook: the receiver's
	// origin as the resource and the destination, and the payload as the data
	// that may leave. Authority is closed-world for an operation that has an
	// analyzer: what is not listed is out of scope.
	resources := []action.ResourceRef{{Kind: "url", ID: "https://hooks.acme.io"}}
	data, destinations := []string{"payload"}, []string{"hooks.acme.io"}
	contract := action.IntentContractV2{
		IntentID: intentID, SchemaVersion: 2, Version: 1, ProfileID: "profile_harness",
		OwnerPrincipalID: owner, Purpose: purpose, Operations: operations, EffectClasses: effects,
		AllowedResources: resources, DataScope: data, OutputDestinations: destinations,
		Budget: budget, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		MaxDelegationDepth: 4,
	}
	if err := store.CreateIntentV2(ctx, contract, owner, now); err != nil {
		return none, fmt.Errorf("create intent: %w", err)
	}
	if err := store.ActivateIntentV2(ctx, contract.IntentID, contract.Version, owner, now); err != nil {
		return none, fmt.Errorf("activate intent: %w", err)
	}
	root := action.AuthorityGrantV2{
		GrantID: "grant_harness_root_" + intentID, SchemaVersion: 2, Version: 1, ProfileID: contract.ProfileID,
		IntentID: contract.IntentID, IntentVersion: contract.Version, IntentDigest: contract.Digest(),
		IssuerPrincipalID: owner, SubjectPrincipalID: owner, Operations: operations,
		AllowedResources: resources, AllowedData: data, OutputDestinations: destinations,
		Channels: []string{channel}, EffectClasses: effects, EffectCeiling: action.EffectWriteIrreversible,
		Budget: budget, ValidFrom: contract.ValidFrom, ExpiresAt: contract.ExpiresAt,
		DelegationDepthRemaining: 1, Status: action.LifecycleActive,
	}
	act, err := actorAct("issue", root.CanonicalBytes())
	if err != nil {
		return none, fmt.Errorf("the owner's issue act: %w", err)
	}
	if err := store.IssueAuthority(ctx, root, act, now); err != nil {
		return none, fmt.Errorf("issue the root grant: %w", err)
	}
	leaf := root
	leaf.GrantID = "grant_harness_leaf_" + intentID
	leaf.SubjectPrincipalID = actor
	leaf.ParentGrantID, leaf.ParentGrantVersion = root.GrantID, root.Version
	leaf.DelegationDepthRemaining = 0
	if act, err = actorAct("delegate", leaf.CanonicalBytes()); err != nil {
		return none, fmt.Errorf("the owner's delegate act: %w", err)
	}
	if err := store.DelegateAuthority(ctx, leaf, act, now); err != nil {
		return none, fmt.Errorf("delegate to the parking brain: %w", err)
	}
	if err := store.PutExecutionBinding(ctx, action.ExecutionBinding{
		BindingID: "binding_harness_" + intentID, ActorPrincipalID: actor, Channel: channel,
		IntentID: contract.IntentID, IntentVersion: contract.Version, IntentDigest: contract.Digest(),
		GrantID: leaf.GrantID, GrantVersion: leaf.Version, GrantDigest: leaf.Digest(),
		Revision: 1, Status: action.BindingActive,
	}); err != nil {
		return none, fmt.Errorf("bind the leaf: %w", err)
	}

	requestID, resolve, err := evidenceFor(parkingBrain)
	if err != nil {
		return none, err
	}
	parked, err := store.ParkAuthorization(ctx, actionsqlite.AuthorityPendingRequest{
		ActorPrincipalID: actor, CorrelationID: requestID, SourceProtocol: "text", Channel: channel,
		Operation: operation, Arguments: params, EffectClass: action.EffectWriteIrreversible, At: now,
		ApprovalContext: action.ApprovalContext{
			GrantID: leaf.GrantID, GrantDepth: 2, CostLine: "maximum", ToolCage: operationName,
			Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible, DataEgress: true},
			HasDescriptor: true, LawVersion: pin.Version, LawDigest: pin.Digest,
			Rule: "require_approval", TTL: ttl,
		},
		ResolveEvidence: resolve,
	})
	if err != nil {
		return none, fmt.Errorf("park through the production door: %w", err)
	}
	for i := 0; i < spendAfterPark; i++ {
		requestID, resolve, err := evidenceFor(parkingBrain)
		if err != nil {
			return none, err
		}
		if _, err := store.StartAuthorization(ctx, actionsqlite.AuthorityStartRequest{
			ActorPrincipalID: actor, Channel: channel, Operation: operation, Arguments: params,
			EffectClass: action.EffectWriteIrreversible, At: time.Now().UTC(),
			CorrelationID: requestID, ResolveEvidence: resolve,
		}); err != nil {
			return none, fmt.Errorf("live start %d after the park: %w", i+1, err)
		}
	}
	return parked, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// parkingBrain is the agent brain the approvals scenarios park under.
const parkingBrain = "operaciones"
