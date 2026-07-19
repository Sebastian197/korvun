// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// fakeStore is the SecretStore test double: an in-memory map with a Get spy
// and a forceable error (AS-1, AS-4, AS-5).
type fakeStore struct {
	mu     sync.Mutex
	data   map[string]string
	gets   []string
	getErr error
}

func newFakeStore() *fakeStore { return &fakeStore{data: map[string]string{}} }

func (f *fakeStore) Get(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, name)
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.data[name]
	if !ok {
		return "", ErrSecretNotFound
	}
	return v, nil
}

func (f *fakeStore) Set(name, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[name] = value
	return nil
}

func (f *fakeStore) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.data[name]; !ok {
		return ErrSecretNotFound
	}
	delete(f.data, name)
	return nil
}

func (f *fakeStore) requested(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.gets {
		if g == name {
			return true
		}
	}
	return false
}

// ensureUnset makes an env var absent for the test, restoring it afterwards
// (t.Setenv registers the restore; the explicit Unsetenv leaves it absent).
func ensureUnset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

func TestSecretEnvNames_enumeratesDedupSortedExcludingBearer(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Channels: []config.ChannelConfig{
			{Type: "discord", Mode: "gateway", TokenEnv: "B_TOK"},
			{Type: "telegram", Mode: "polling", TokenEnv: "B_TOK"}, // dup
		},
		Brains: []config.BrainConfig{{
			Name: "b",
			Models: []config.ModelConfig{
				{Provider: "groq", ModelID: "m", Locality: "cloud", APIKeyEnv: "A_KEY"},
				{Provider: "ollama", ModelID: "m2", Locality: "local"}, // no key env
			},
		}},
		Admin: &config.AdminConfig{TokenEnv: "ADMIN_BEARER"}, // excluded
	}
	got := SecretEnvNames(cfg)
	want := []string{"A_KEY", "B_TOK"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SecretEnvNames = %v, want %v", got, want)
	}
}

// TestStart_precedence_envWins is AS-1: a variable already in the
// environment wins — the store is never even consulted for it.
func TestStart_precedence_envWins(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	t.Setenv(discordTokenEnv, "from-env")

	store := newFakeStore()
	_ = store.Set(discordTokenEnv, "from-keyring")

	c := testControllerWithStore(store, fakeFactory())
	if err := c.LoadConfig(writeCfg(t, minimalCfg(srv.URL))); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = c.Stop(ctx) }()

	if store.requested(discordTokenEnv) {
		t.Fatal("store was consulted for a variable already present in the env")
	}
	if got := os.Getenv(discordTokenEnv); got != "from-env" {
		t.Fatalf("env value = %q, want the pre-existing %q", got, "from-env")
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := os.Getenv(discordTokenEnv); got != "from-env" {
		t.Fatalf("pre-existing env var after Stop = %q, want untouched %q (AS-7)", got, "from-env")
	}
}

// TestStart_provisionsAbsentAndUnsetsOnStop is AS-2 + AS-7's provisioned
// half: an absent variable is filled from the store for the cycle and
// removed at its end.
func TestStart_provisionsAbsentAndUnsetsOnStop(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	ensureUnset(t, discordTokenEnv)

	store := newFakeStore()
	_ = store.Set(discordTokenEnv, "tok-from-keyring")

	c := testControllerWithStore(store, fakeFactory())
	if err := c.LoadConfig(writeCfg(t, minimalCfg(srv.URL))); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := os.Getenv(discordTokenEnv); got != "tok-from-keyring" {
		t.Fatalf("provisioned env = %q, want the store value", got)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, ok := os.LookupEnv(discordTokenEnv); ok {
		t.Fatal("provisioned variable still set after Stop")
	}
}

// TestStart_missingEverywhere_failsNamingVariable is AS-3, with the REAL
// channel factory: discord's factory pre-checks env presence with no
// network, so the boot fails with ErrMissingSecret naming the variable.
func TestStart_missingEverywhere_failsNamingVariable(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	ensureUnset(t, discordTokenEnv)

	c := testControllerWithStore(newFakeStore()) // empty store, REAL factory
	if err := c.LoadConfig(writeCfg(t, minimalCfg(srv.URL))); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("Start with the secret missing everywhere: want error, got nil")
	}
	if !errors.Is(err, app.ErrMissingSecret) {
		t.Fatalf("want ErrMissingSecret in the chain, got %v", err)
	}
	if !strings.Contains(err.Error(), discordTokenEnv) {
		t.Fatalf("error does not name the variable: %v", err)
	}
	if c.Status().Running {
		t.Fatal("running after failed boot")
	}
}

// TestStart_storeFailure_failsNamingVariable is AS-4: a broken store (not a
// miss) aborts Start, naming the variable and carrying the cause.
func TestStart_storeFailure_failsNamingVariable(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	ensureUnset(t, discordTokenEnv)

	store := newFakeStore()
	store.getErr = errors.New("dbus session unavailable")

	c := testControllerWithStore(store, fakeFactory())
	if err := c.LoadConfig(writeCfg(t, minimalCfg(srv.URL))); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("Start with a broken store: want error, got nil")
	}
	if !strings.Contains(err.Error(), discordTokenEnv) || !strings.Contains(err.Error(), "dbus session unavailable") {
		t.Fatalf("error must name the variable and the cause: %v", err)
	}
	if c.Status().Running {
		t.Fatal("running after failed boot")
	}
}

// TestSetDeleteSecret_delegation is FR-SEC-5 + AS-5 on the seam.
func TestSetDeleteSecret_delegation(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	c := testControllerWithStore(store)

	if err := c.SetSecret("SOME_VAR", "v1"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if got, err := store.Get("SOME_VAR"); err != nil || got != "v1" {
		t.Fatalf("store after SetSecret = %q, %v", got, err)
	}
	if err := c.DeleteSecret("SOME_VAR"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if _, err := store.Get("SOME_VAR"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("store after DeleteSecret: want ErrSecretNotFound, got %v", err)
	}

	none := testController()
	if err := none.SetSecret("X", "v"); err == nil {
		t.Fatal("SetSecret with no store: want error, got nil")
	}
	if err := none.DeleteSecret("X"); err == nil {
		t.Fatal("DeleteSecret with no store: want error, got nil")
	}
}

// TestNoSecretValuesLeakToLogsOrErrors is AS-6: a full provisioned cycle and
// a store-failure boot, with a capturing logger — the secret VALUE must
// never appear in any log line or error string.
func TestNoSecretValuesLeakToLogsOrErrors(t *testing.T) {
	const secretValue = "sup3r-s3cret-value-XYZ"
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	ensureUnset(t, discordTokenEnv)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	store := newFakeStore()
	_ = store.Set(discordTokenEnv, secretValue)

	c := New(WithLogger(logger), WithSecretStore(store), WithBuildOptions(fakeFactory()))
	if err := c.LoadConfig(writeCfg(t, minimalCfg(srv.URL))); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Failed-boot path too (store breaks on the second cycle).
	store.getErr = errors.New("backend exploded")
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("second Start: want store error")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatal("secret value leaked into the error string")
	}
	if buf.Len() == 0 {
		t.Fatal("no log lines captured — the leak assertion would be vacuous")
	}
	if strings.Contains(buf.String(), secretValue) {
		t.Fatal("secret value leaked into the logs")
	}
}

// TestStart_emptyEnvVar_treatedAsAbsent: a set-but-EMPTY variable does not
// defeat the keychain — it counts as absent (matching the core's
// ErrMissingSecret semantics), gets provisioned, and is unset at cycle end.
func TestStart_emptyEnvVar_treatedAsAbsent(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	t.Setenv(discordTokenEnv, "") // present but empty

	store := newFakeStore()
	_ = store.Set(discordTokenEnv, "tok-fills-empty")

	c := testControllerWithStore(store, fakeFactory())
	if err := c.LoadConfig(writeCfg(t, minimalCfg(srv.URL))); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := os.Getenv(discordTokenEnv); got != "tok-fills-empty" {
		t.Fatalf("empty env var not provisioned from store: %q", got)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, ok := os.LookupEnv(discordTokenEnv); ok {
		t.Fatal("provisioned variable still set after Stop")
	}
}

// TestStart_bearerVariableCollision_failsNamed: a config that reuses the
// admin bearer's variable as a channel secret is rejected before anything
// builds — the bearer's per-cycle Setenv would silently clobber the secret.
func TestStart_bearerVariableCollision_failsNamed(t *testing.T) {
	srv := fakeOllama(t)
	t.Setenv(adminTokenEnv, "")
	cfg := minimalCfg(srv.URL)
	cfg.Admin.TokenEnv = discordTokenEnv // collision

	c := testControllerWithStore(newFakeStore(), fakeFactory())
	if err := c.LoadConfig(writeCfg(t, cfg)); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.Start(ctx)
	if err == nil {
		t.Fatal("Start with a bearer/secret variable collision: want error")
	}
	if !strings.Contains(err.Error(), discordTokenEnv) || !strings.Contains(err.Error(), "reuses") {
		t.Fatalf("collision error must name the variable: %v", err)
	}
	if c.Status().Running {
		t.Fatal("running after collision rejection")
	}
}

func testControllerWithStore(s SecretStore, extra ...app.Option) *Controller {
	opts := []Option{WithLogger(slog.New(slog.DiscardHandler)), WithSecretStore(s)}
	if len(extra) > 0 {
		opts = append(opts, WithBuildOptions(extra...))
	}
	return New(opts...)
}
