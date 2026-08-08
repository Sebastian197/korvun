// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// TestDefaultConfigPath pins FR-FIRST-1: <os.UserConfigDir>/korvun/korvun.json,
// the exact pattern the core uses for the default korvun.db.
func TestDefaultConfigPath(t *testing.T) {
	got, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	want := filepath.Join(base, "korvun", "korvun.json")
	if got != want {
		t.Fatalf("DefaultConfigPath = %q, want %q", got, want)
	}
}

// AS-1: no config → the template is written atomically, the report is
// "created", the parent dir is 0o700, and the bytes equal the embed.
func TestEnsureDefaultConfig_createsWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun", "korvun.json")
	created, err := EnsureDefaultConfig(path)
	if err != nil {
		t.Fatalf("EnsureDefaultConfig: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true on a fresh path")
	}
	got, err := os.ReadFile(path) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatalf("read created config: %v", err)
	}
	if !bytes.Equal(got, firstRunTemplate) {
		t.Fatalf("created config differs from the embedded template:\n%s", got)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if runtime.GOOS != "windows" {
		// On Windows a directory's Mode().Perm() is always 0777 — real
		// permissions ride ACLs, out of this test's scope. The rest of the
		// test (creation, report, byte fidelity) still runs there.
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("config dir perm = %o, want 0700", perm)
		}
	}
}

// AS-2: an existing config is NEVER touched — bytes and mtime intact.
func TestEnsureDefaultConfig_neverTouchesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.json")
	userBytes := []byte(`{"user":"precious hand-edited bytes, not even valid config"}`)
	if err := os.WriteFile(path, userBytes, 0o600); err != nil {
		t.Fatalf("seed existing config: %v", err)
	}
	// Seed the mtime one hour into the past so the untouched assertion is
	// immune to coarse filesystem timestamp granularity.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("seed mtime: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	created, err := EnsureDefaultConfig(path)
	if err != nil {
		t.Fatalf("EnsureDefaultConfig on an existing file: %v", err)
	}
	if created {
		t.Fatal("created = true, want false when the file exists")
	}
	got, err := os.ReadFile(path) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(got, userBytes) {
		t.Fatalf("existing config was modified: %q", got)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("existing config mtime changed — the file was touched")
	}
}

// AS-3 + FR-FIRST-4 (fidelity round-trip): the written template passes the
// core's own Load+Validate byte-for-byte as embedded, and carries the
// ADR-0035 §5 amendments. If the core schema ever moves, this breaks first.
func TestFirstRunTemplate_fidelityRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "korvun.json")
	if _, err := EnsureDefaultConfig(path); err != nil {
		t.Fatalf("EnsureDefaultConfig: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("the first-run template does not pass config.Load: %v", err)
	}
	if cfg.Admin == nil || cfg.Admin.TokenEnv != "KORVUN_ADMIN_TOKEN" {
		t.Fatalf("template admin block = %+v, want token_env KORVUN_ADMIN_TOKEN", cfg.Admin)
	}
	if enabled, _ := cfg.ObservabilitySettings(); !enabled {
		t.Fatal("template observability is not enabled")
	}
	if len(cfg.Channels) != 0 || len(cfg.Routes) != 0 {
		t.Fatalf("template has %d channels / %d routes, want 0 / 0 (NC-1 option B)", len(cfg.Channels), len(cfg.Routes))
	}
	if len(cfg.Brains) != 1 || len(cfg.Brains[0].Models) != 1 ||
		cfg.Brains[0].Models[0].Provider != "ollama" || cfg.Brains[0].Models[0].ModelID != "llama3.2:1b" {
		t.Fatalf("template brain = %+v, want one ollama/llama3.2:1b model", cfg.Brains)
	}
	if cfg.Brains[0].Sensitivity != "private" || cfg.Brains[0].Policy.Kind != "priority" {
		t.Fatalf("template brain sensitivity/policy = %q/%q, want private/priority (FR-FIRST-3)",
			cfg.Brains[0].Sensitivity, cfg.Brains[0].Policy.Kind)
	}
	// Structural secret hygiene: only *_env NAMES may appear; no key shaped
	// like a credential carrier, and the model's api_key_env stays empty.
	for _, key := range []string{`"token":`, `"api_key":`, `"secret":`, `"password":`} {
		if bytes.Contains(firstRunTemplate, []byte(key)) {
			t.Fatalf("template carries a credential-shaped key %s", key)
		}
	}
	if cfg.Brains[0].Models[0].APIKeyEnv != "" {
		t.Fatal("template names an api_key_env for a local model")
	}
	onDisk, err := os.ReadFile(path) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatalf("read written template: %v", err)
	}
	if !bytes.Equal(onDisk, firstRunTemplate) {
		t.Fatal("on-disk bytes differ from the embedded template — the embed is not in WriteConfigAtomic's canonical form")
	}
}

// A stat failure that is NOT "file missing" (here: the parent is a file, so
// the path is unreachable) surfaces as a named error, never as "created".
func TestEnsureDefaultConfig_statFailure(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed parent file: %v", err)
	}
	path := filepath.Join(parentFile, "korvun.json")
	created, err := EnsureDefaultConfig(path)
	if err == nil || created {
		t.Fatalf("EnsureDefaultConfig under a file = (%v, %v), want (false, error)", created, err)
	}
}

// DefaultConfigPath propagates an unresolvable user config dir. What
// os.UserConfigDir consults differs per OS, so the test empties the right
// variables on EACH platform (darwin: HOME; linux: XDG_CONFIG_HOME and HOME;
// windows: AppData) — the meaning is identical on all three: with no
// environment to resolve from, the error must surface, never a bogus path.
func TestDefaultConfigPath_noHome(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", "")
	case "windows":
		t.Setenv("AppData", "")
	default: // linux and the rest of the unix family
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
	}
	if _, err := DefaultConfigPath(); err == nil {
		t.Fatal("DefaultConfigPath with no user-config environment: want error, got nil")
	}
}

// AS-5: an unwritable destination fails loud, names the path, and leaves no
// partial file behind (atomicity).
func TestEnsureDefaultConfig_unwritableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		// A directory's read-only bit does not block creating files inside
		// it on Windows; forcing the denial would need an ACL, which is not
		// unit-test material.
		t.Skip("windows: chmod on a directory cannot force the write failure")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not bite")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil { //nolint:gosec // deliberately read-only DIR to force the write failure
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) }) //nolint:gosec // restore a test-owned temp DIR for cleanup

	path := filepath.Join(parent, "korvun", "korvun.json")
	created, err := EnsureDefaultConfig(path)
	if err == nil {
		t.Fatal("EnsureDefaultConfig on an unwritable parent: want error, got nil")
	}
	if created {
		t.Fatal("created = true on a failed write")
	}
	if !strings.Contains(err.Error(), path) && !strings.Contains(err.Error(), filepath.Dir(path)) {
		t.Fatalf("error does not name the path: %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("a partial config file was left behind")
	}

	// Second leg: the parent EXISTS read-only, so MkdirAll succeeds and the
	// failure happens inside WriteConfigAtomic — the temp-file discipline
	// must still leave nothing behind (the real atomicity assertion).
	direct := filepath.Join(parent, "korvun.json")
	created, err = EnsureDefaultConfig(direct)
	if err == nil || created {
		t.Fatalf("EnsureDefaultConfig into a read-only dir = (%v, %v), want (false, error)", created, err)
	}
	entries, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatalf("read parent dir: %v", readErr)
	}
	for _, e := range entries {
		t.Fatalf("stray file left in the read-only dir: %q", e.Name())
	}
}

// postConfigReload POSTs a full config through the proxy (no client auth —
// the SP4 injection is the door), expects 202 + a handle, and polls the
// reload state THROUGH the proxy until it succeeds (transient 503s during
// the cutover window are part of the contract and are retried).
func postConfigReload(t *testing.T, client *http.Client, baseURL string, body []byte) {
	t.Helper()
	resp, err := client.Post(baseURL+"/api/config", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/config: %v", err)
	}
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read POST response: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/config = %d (body %q), want 202", resp.StatusCode, raw)
	}
	var accepted struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil || accepted.Handle == "" {
		t.Fatalf("POST response %q carries no handle (err %v)", raw, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st, err := client.Get(baseURL + "/api/reload/" + accepted.Handle)
		if err != nil {
			t.Fatalf("GET reload state: %v", err)
		}
		raw, err := io.ReadAll(st.Body)
		_ = st.Body.Close()
		if err != nil {
			t.Fatalf("read reload state: %v", err)
		}
		if st.StatusCode == http.StatusOK {
			var state struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatalf("reload state %q: %v", raw, err)
			}
			switch state.State {
			case string(supervisor.StateSucceeded):
				return
			case string(supervisor.StateFailed):
				t.Fatalf("reload failed (handle %s)", accepted.Handle)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("reload %s did not succeed within the deadline", accepted.Handle)
}

// The NC-1 flow end to end (the decision's whole point): the channel-less
// first-run template boots, the builder's mutation API adds the FIRST
// channel via a reload THROUGH the SP4 proxy, the channel list turns
// honest, and a second reload back to the template returns to the valid
// zero-channel state — all in one shell cycle, clean Stop at the end.
func TestFirstRun_builderAddsFirstChannel(t *testing.T) {
	t.Setenv("KORVUN_ADMIN_TOKEN", "")
	path := filepath.Join(t.TempDir(), "korvun.json")
	if _, err := EnsureDefaultConfig(path); err != nil {
		t.Fatalf("EnsureDefaultConfig: %v", err)
	}

	c := New(WithLogger(slog.New(slog.DiscardHandler)), WithBuildOptions(fakeFactory()))
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
			defer sc()
			_ = c.Stop(sctx)
		}
	})
	srv := httptest.NewServer(c.ProxyHandler())
	defer srv.Close()

	channelsBody := func() string {
		resp, body := get(t, srv.Client(), srv.URL+"/api/channels", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/channels = %d (body %q), want 200", resp.StatusCode, body)
		}
		return body
	}
	if body := channelsBody(); strings.TrimSpace(body) != "[]" {
		t.Fatalf("first-run /api/channels = %q, want []", body)
	}

	// The user adds the first channel in the builder → POST the full config.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload template: %v", err)
	}
	cfg.Channels = append(cfg.Channels, config.ChannelConfig{
		Type: "discord", Mode: "gateway", TokenEnv: "FIRSTRUN_TEST_DISCORD_TOKEN",
	})
	cfg.Routes = append(cfg.Routes, config.RouteConfig{Channel: "discord", Brain: "default"})
	withChannel, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal channel-bearing config: %v", err)
	}
	postConfigReload(t, srv.Client(), srv.URL, withChannel)
	if body := channelsBody(); !strings.Contains(body, `"discord"`) {
		t.Fatalf("post-reload /api/channels = %q, want the discord channel", body)
	}

	// And back to zero — the template itself is a valid reload target.
	postConfigReload(t, srv.Client(), srv.URL, firstRunTemplate)
	if body := channelsBody(); strings.TrimSpace(body) != "[]" {
		t.Fatalf("back-to-zero /api/channels = %q, want []", body)
	}

	sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	defer sc()
	if err := c.Stop(sctx); err != nil {
		t.Fatalf("Stop after the reload cycle: %v", err)
	}
}

// AS-4 + FR-FIRST-5 (the template BOOTS, by construction): write the
// template, then LoadConfig + Start on the REAL Controller — channel-less,
// so no channel fake and no secret are needed — and assert core up, builder
// mounted (GET /api/config 200 through the SP4 proxy, which also proves the
// bearer injection), clean Stop. The brain's ollama adapter is constructed
// but never dialed (warmup is off in the template; no message flows).
func TestFirstRun_templateBoots(t *testing.T) {
	t.Setenv("KORVUN_ADMIN_TOKEN", "")
	// The template now ships the storage block with an empty path, which
	// resolves to <os.UserConfigDir>/korvun/korvun.db at boot — sandbox the
	// user dir so the test never opens a real user's database.
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin", "linux":
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", "") // linux: fall through to HOME/.config
	case "windows":
		t.Setenv("AppData", tmp)
	}
	path := filepath.Join(t.TempDir(), "korvun.json")
	created, err := EnsureDefaultConfig(path)
	if err != nil || !created {
		t.Fatalf("EnsureDefaultConfig = (%v, %v), want (true, nil)", created, err)
	}

	c := New(WithLogger(slog.New(slog.DiscardHandler)))
	if err := c.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig on the first-run template: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start on the first-run template: %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
			defer sc()
			_ = c.Stop(sctx)
		}
	})

	st := c.Status()
	if !st.Running || st.AdminAddr == "" {
		t.Fatalf("Status after Start = %+v, want running with a bound admin addr", st)
	}

	srv := httptest.NewServer(c.ProxyHandler())
	defer srv.Close()
	for _, path := range []string{"/healthz", "/api/config", "/builder/"} {
		resp, body := get(t, srv.Client(), srv.URL+path, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("first-run GET %s = %d, want 200 (body %q)", path, resp.StatusCode, body)
		}
	}

	sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	defer sc()
	if err := c.Stop(sctx); err != nil {
		t.Fatalf("Stop after the first-run boot: %v", err)
	}
	if c.Status().Running {
		t.Fatal("Status still running after Stop")
	}
}
