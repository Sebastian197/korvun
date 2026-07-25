// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"context"
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
	"testing"
	"time"
)

// testDesktop builds a Desktop over c with short deadlines so the LAW's
// timeout paths run in test time, not wall-clock minutes.
func testDesktop(c *Controller, opts ...DesktopOption) *Desktop {
	base := []DesktopOption{
		WithDesktopLogger(slog.New(slog.DiscardHandler)),
		withBindingDeadlines(bindingDeadlines{
			lifecycle: 150 * time.Millisecond,
			keychain:  150 * time.Millisecond,
			check:     150 * time.Millisecond,
			read:      150 * time.Millisecond,
		}),
	}
	return NewDesktop(c, append(base, opts...)...)
}

// AS-7 (THE LAW): with the Controller's mutex deliberately wedged, every
// binding returns a named timeout error within its deadline — never an
// unbounded hang from the UI into the mutex.
func TestDesktop_law_wedgedControllerTimesOut(t *testing.T) {
	c := testController()
	c.mu.Lock() // the wedge: nothing can acquire the Controller
	t.Cleanup(c.mu.Unlock)
	d := testDesktop(c)

	start := time.Now()
	if _, err := d.Status(); !errors.Is(err, ErrBindingTimeout) {
		t.Fatalf("Status on a wedged Controller: want ErrBindingTimeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Status took %s — the deadline did not bite", elapsed)
	}
	if err := d.LoadConfig("nope.json"); !errors.Is(err, ErrBindingTimeout) {
		t.Fatalf("LoadConfig on a wedged Controller: want ErrBindingTimeout, got %v", err)
	}
}

// The lifecycle pair is SERIALIZED: while one Start/Stop is in flight (even
// an abandoned, timed-out one), a second call returns ErrLifecycleBusy
// immediately — timed-out goroutines never pile on the mutex. Once the
// abandoned call lands, the gate reopens (post-timeout reconciliation).
func TestDesktop_law_lifecycleSerializedAndReconciles(t *testing.T) {
	c := testController()
	c.mu.Lock()
	d := testDesktop(c)

	if err := d.Start(); !errors.Is(err, ErrBindingTimeout) {
		c.mu.Unlock()
		t.Fatalf("Start on a wedged Controller: want ErrBindingTimeout, got %v", err)
	}
	if err := d.Start(); !errors.Is(err, ErrLifecycleBusy) {
		c.mu.Unlock()
		t.Fatalf("second Start while one is in flight: want ErrLifecycleBusy, got %v", err)
	}

	// Release the wedge: the abandoned Start completes against the real
	// Controller (ErrNoConfig — nothing loaded) and the gate reopens.
	c.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := d.Start()
		if errors.Is(err, ErrNoConfig) {
			break // the gate reopened and the real Controller answered
		}
		if !errors.Is(err, ErrLifecycleBusy) {
			t.Fatalf("Start after releasing the wedge: got %v, want eventually ErrNoConfig", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("the lifecycle gate never reopened after the abandoned call landed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The happy path: the full lifecycle through the bindings against a REAL
// core (no network — the SP2 fake-channel pattern), with default deadlines.
func TestDesktop_lifecycle_realCore(t *testing.T) {
	t.Setenv(adminTokenEnv, "")
	ollama := fakeOllama(t)
	c := testController(fakeFactory())
	d := NewDesktop(c, WithDesktopLogger(slog.New(slog.DiscardHandler)))
	path := writeCfg(t, minimalCfg(ollama.URL))

	if err := d.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if c.Status().Running {
			sctx, sc := context.WithTimeout(context.Background(), 10*time.Second)
			defer sc()
			_ = c.Stop(sctx)
		}
	})
	st, err := d.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Running || st.AdminAddr == "" {
		t.Fatalf("Status after Start = %+v, want running with an admin addr", st)
	}
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if st, err := d.Status(); err != nil || st.Running {
		t.Fatalf("Status after Stop = (%+v, %v), want stopped", st, err)
	}
}

// CheckOllama: reachability is an OUTCOME (never a Go error) — reachable on
// a 200, honest detail on refused/timeout/bad URL.
func TestDesktop_checkOllama(t *testing.T) {
	d := testDesktop(testController())

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"models":[]}`)
	}))
	defer up.Close()
	if chk := d.CheckOllama(up.URL); !chk.Reachable {
		t.Fatalf("CheckOllama against a live server: %+v, want reachable", chk)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := "http://" + l.Addr().String()
	_ = l.Close()
	if chk := d.CheckOllama(dead); chk.Reachable || chk.Detail == "" {
		t.Fatalf("CheckOllama against a dead port: %+v, want unreachable with detail", chk)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	begin := time.Now()
	if chk := d.CheckOllama(slow.URL); chk.Reachable {
		t.Fatalf("CheckOllama against a hung server: %+v, want unreachable", chk)
	}
	if elapsed := time.Since(begin); elapsed > 2*time.Second {
		t.Fatalf("CheckOllama took %s — the 3s-class deadline did not bite", elapsed)
	}

	if chk := d.CheckOllama("::not a url::"); chk.Reachable {
		t.Fatalf("CheckOllama with a broken URL: %+v, want unreachable", chk)
	}
}

// EnsureDefaultConfig composes DefaultConfigPath (arg-less binding): created
// on first call, untouched on the second. The user-config env is pointed at
// a temp dir per platform so the test never writes the real one.
func TestDesktop_ensureDefaultConfig(t *testing.T) {
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin", "linux":
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", "") // linux: fall through to HOME/.config
	case "windows":
		t.Setenv("AppData", tmp)
	}
	d := NewDesktop(testController(), WithDesktopLogger(slog.New(slog.DiscardHandler)))

	path, err := d.DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if !strings.HasPrefix(path, tmp) {
		t.Fatalf("DefaultConfigPath = %q, escaped the temp sandbox %q", path, tmp)
	}
	created, err := d.EnsureDefaultConfig()
	if err != nil || !created {
		t.Fatalf("first EnsureDefaultConfig = (%v, %v), want (true, nil)", created, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "korvun.json")); err != nil {
		t.Fatalf("template not written at the default path: %v", err)
	}
	created, err = d.EnsureDefaultConfig()
	if err != nil || created {
		t.Fatalf("second EnsureDefaultConfig = (%v, %v), want (false, nil)", created, err)
	}
}

// The keychain pair passes through the SecretStore seam, bounded by the
// keychain deadline class; without a store the named sentinel surfaces.
func TestDesktop_secrets(t *testing.T) {
	store := newFakeStore()
	d := NewDesktop(New(WithLogger(slog.New(slog.DiscardHandler)), WithSecretStore(store)),
		WithDesktopLogger(slog.New(slog.DiscardHandler)))
	if err := d.SetSecret("X_TOKEN", "v"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if got := store.data["X_TOKEN"]; got != "v" {
		t.Fatalf("store value = %q, want %q", got, "v")
	}
	if err := d.DeleteSecret("X_TOKEN"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if _, ok := store.data["X_TOKEN"]; ok {
		t.Fatal("DeleteSecret left the entry behind")
	}

	bare := NewDesktop(testController())
	if err := bare.SetSecret("X", "v"); !errors.Is(err, ErrNoSecretStore) {
		t.Fatalf("SetSecret without a store: want ErrNoSecretStore, got %v", err)
	}
}

// Version reports what the build stamped (the chrome's logo/version row).
func TestDesktop_version(t *testing.T) {
	if v := NewDesktop(testController()).Version(); v != "dev" {
		t.Fatalf("default Version = %q, want dev", v)
	}
	if v := NewDesktop(testController(), WithDesktopVersion("v9.9.9")).Version(); v != "v9.9.9" {
		t.Fatalf("Version = %q, want v9.9.9", v)
	}
}

// blockingStore wedges Get forever — the LAW probe for the presence check.
type blockingStore struct{ fakeStore }

func (b *blockingStore) Get(string) (string, error) {
	select {} // never returns
}

// CheckSecretPresence (SP6c, wizard step 3): PRESENCE only — env and
// keychain booleans, never a value in the result, and LAW-bounded.
func TestDesktop_checkSecretPresence(t *testing.T) {
	const name = "SHELL_TEST_PRESENCE_TOKEN" //nolint:gosec // an env-var NAME, not a credential

	store := newFakeStore()
	c := testController()
	WithSecretStore(store)(c)
	d := testDesktop(c)

	p, err := d.CheckSecretPresence(name)
	if err != nil {
		t.Fatalf("CheckSecretPresence (absent everywhere): %v", err)
	}
	if p.InEnv || p.InKeychain {
		t.Fatalf("absent everywhere: got %+v, want both false", p)
	}

	t.Setenv(name, "some-value")
	if p, _ = d.CheckSecretPresence(name); !p.InEnv {
		t.Fatalf("env var set: got %+v, want InEnv true", p)
	}

	store.data[name] = "keychain-value"
	p, err = d.CheckSecretPresence(name)
	if err != nil || !p.InKeychain {
		t.Fatalf("keychain entry present: got %+v err=%v, want InKeychain true", p, err)
	}

	// No store configured: not-in-keychain is the honest answer, not an error.
	bare := testDesktop(testController())
	if p, err = bare.CheckSecretPresence(name); err != nil || p.InKeychain {
		t.Fatalf("no store: got %+v err=%v, want InKeychain false, nil error", p, err)
	}
}

// THE LAW covers the new binding: a wedged keychain Get times out with the
// named error inside the keychain deadline class — never an unbounded hang.
func TestDesktop_law_checkSecretPresenceBounded(t *testing.T) {
	c := testController()
	WithSecretStore(&blockingStore{})(c)
	d := testDesktop(c)

	start := time.Now()
	if _, err := d.CheckSecretPresence("ANY_NAME"); !errors.Is(err, ErrBindingTimeout) {
		t.Fatalf("wedged store: want ErrBindingTimeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("presence check took %s — the keychain deadline did not bite", elapsed)
	}
}
