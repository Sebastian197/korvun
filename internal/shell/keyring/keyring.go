// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package keyring is the real OS-keychain backend behind shell.SecretStore
// (ADR-0035 §4, ADR-0037): Keychain Services on macOS, Credential Manager on
// Windows, Secret Service D-Bus on Linux — via zalando/go-keyring, the only
// package in the repo that imports it. No file fallback exists: a platform
// without a working credential store surfaces the library's error honestly.
package keyring

import (
	"errors"
	"fmt"

	"github.com/Sebastian197/korvun/internal/shell"
	zkeyring "github.com/zalando/go-keyring"
)

// Service is the keychain service name every Korvun entry lives under
// (ADR-0035 §4 storage contract: service `korvun`, account = env-var name,
// value = the secret).
const Service = "korvun"

// Store implements shell.SecretStore over the OS keychain.
type Store struct{}

// New returns the OS-keychain store.
func New() *Store { return &Store{} }

// Get reads the secret stored under name. A missing entry maps to
// shell.ErrSecretNotFound; any other failure is wrapped naming the VARIABLE
// only — never a value.
func (s *Store) Get(name string) (string, error) {
	v, err := zkeyring.Get(Service, name)
	if errors.Is(err, zkeyring.ErrNotFound) {
		return "", fmt.Errorf("%w: %q", shell.ErrSecretNotFound, name)
	}
	if err != nil {
		return "", fmt.Errorf("keyring: get %q: %w", name, err)
	}
	return v, nil
}

// Set writes (or overwrites) the secret under name.
func (s *Store) Set(name, value string) error {
	if err := zkeyring.Set(Service, name, value); err != nil {
		return fmt.Errorf("keyring: set %q: %w", name, err)
	}
	return nil
}

// Delete REMOVES the entry under name (no orphaned or emptied entries). A
// missing entry maps to shell.ErrSecretNotFound so callers can tell "already
// gone" apart from a real failure.
func (s *Store) Delete(name string) error {
	err := zkeyring.Delete(Service, name)
	if errors.Is(err, zkeyring.ErrNotFound) {
		return fmt.Errorf("%w: %q", shell.ErrSecretNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("keyring: delete %q: %w", name, err)
	}
	return nil
}
