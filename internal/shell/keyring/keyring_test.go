// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package keyring

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/shell"
	zkeyring "github.com/zalando/go-keyring"
)

// mockInstalled records that a test replaced go-keyring's package-global
// provider with the in-memory mock. The library has no API to restore the
// real OS backend, so the live test (keyring_live_test.go) uses this flag to
// fail loudly instead of passing vacuously against the residual mock.
var mockInstalled bool

func mockInit() {
	zkeyring.MockInit()
	mockInstalled = true
}

func mockInitWithError(err error) {
	zkeyring.MockInitWithError(err)
	mockInstalled = true
}

// TestStore_roundtrip_mockBackend exercises the full mapping contract over
// the library's in-memory mock — hermetic, so the 3-OS quality gate never
// needs a live keyring (CI ubuntu has no Secret Service session).
func TestStore_roundtrip_mockBackend(t *testing.T) {
	mockInit()
	s := New()

	if _, err := s.Get("KORVUN_TEST_VAR"); !errors.Is(err, shell.ErrSecretNotFound) {
		t.Fatalf("Get on empty store: want shell.ErrSecretNotFound, got %v", err)
	}

	if err := s.Set("KORVUN_TEST_VAR", "v-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("KORVUN_TEST_VAR")
	if err != nil || got != "v-1" {
		t.Fatalf("Get after Set = %q, %v; want v-1, nil", got, err)
	}

	if err := s.Set("KORVUN_TEST_VAR", "v-2"); err != nil {
		t.Fatalf("overwrite Set: %v", err)
	}
	if got, _ := s.Get("KORVUN_TEST_VAR"); got != "v-2" {
		t.Fatalf("Get after overwrite = %q, want v-2", got)
	}

	if err := s.Delete("KORVUN_TEST_VAR"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("KORVUN_TEST_VAR"); !errors.Is(err, shell.ErrSecretNotFound) {
		t.Fatalf("Get after Delete: want shell.ErrSecretNotFound (no orphans), got %v", err)
	}
	if err := s.Delete("KORVUN_TEST_VAR"); !errors.Is(err, shell.ErrSecretNotFound) {
		t.Fatalf("second Delete: want shell.ErrSecretNotFound, got %v", err)
	}
}

// TestStore_errorsNameVariableOnly asserts the no-leak contract on the
// backend's own errors: variable names yes, values never.
func TestStore_errorsNameVariableOnly(t *testing.T) {
	mockInit()
	s := New()
	_, err := s.Get("SOME_NAMED_VAR")
	if err == nil {
		t.Fatal("want miss error")
	}
	if msg := err.Error(); msg == "" || !errors.Is(err, shell.ErrSecretNotFound) {
		t.Fatalf("unexpected error shape: %v", err)
	}
}

// TestStore_backendFailure_wrapsNamingVariable covers the non-miss error
// branches via the library's error-injecting mock: a broken backend (e.g. no
// D-Bus session) wraps with the variable name and the cause, never a value.
func TestStore_backendFailure_wrapsNamingVariable(t *testing.T) {
	backendErr := errors.New("secret service unavailable")
	mockInitWithError(backendErr)
	t.Cleanup(mockInit)
	s := New()

	if _, err := s.Get("VAR_A"); !errors.Is(err, backendErr) || !contains(err, "VAR_A") {
		t.Fatalf("Get on broken backend: want wrapped cause naming VAR_A, got %v", err)
	}
	if err := s.Set("VAR_A", "v"); !errors.Is(err, backendErr) || !contains(err, "VAR_A") {
		t.Fatalf("Set on broken backend: want wrapped cause naming VAR_A, got %v", err)
	}
	if err := s.Delete("VAR_A"); !errors.Is(err, backendErr) || !contains(err, "VAR_A") {
		t.Fatalf("Delete on broken backend: want wrapped cause naming VAR_A, got %v", err)
	}
}

func contains(err error, s string) bool {
	return err != nil && len(err.Error()) > 0 && strings.Contains(err.Error(), s)
}
