// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build keyring_live

package keyring

import (
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/shell"
)

// TestStore_liveKeychain is the OPT-IN real-backend test, gated behind the
// keyring_live build tag so the default 3-OS gate never compiles it (CI
// ubuntu has no Secret Service; macOS runners have no guaranteed unlocked
// keychain). Run it in isolation:
//
//	go test -tags keyring_live -run TestStore_liveKeychain ./internal/shell/keyring
//
// The mockInstalled guard turns a sibling test's residual in-memory mock
// (go-keyring has no API to restore the real provider) into a loud failure
// instead of a vacuous pass — the SP3 review's P1.
func TestStore_liveKeychain(t *testing.T) {
	if mockInstalled {
		t.Fatal("in-memory mock installed by sibling tests — run in isolation: go test -tags keyring_live -run TestStore_liveKeychain ./internal/shell/keyring")
	}
	s := New()
	const name = "KORVUN_LIVE_TEST_VAR"
	t.Cleanup(func() { _ = s.Delete(name) })

	if err := s.Set(name, "live-v1"); err != nil {
		t.Fatalf("live Set: %v", err)
	}
	got, err := s.Get(name)
	if err != nil || got != "live-v1" {
		t.Fatalf("live Get = %q, %v; want live-v1, nil", got, err)
	}
	if err := s.Delete(name); err != nil {
		t.Fatalf("live Delete: %v", err)
	}
	if _, err := s.Get(name); !errors.Is(err, shell.ErrSecretNotFound) {
		t.Fatalf("live Get after Delete: want shell.ErrSecretNotFound, got %v", err)
	}
}
