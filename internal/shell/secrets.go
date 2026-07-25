// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/Sebastian197/korvun/internal/config"
)

// ErrSecretNotFound is the SecretStore miss sentinel: the named entry does
// not exist in the keychain. A miss is NOT a provisioning failure — the boot
// then fails loudly with the core's ErrMissingSecret naming the variable.
var ErrSecretNotFound = errors.New("shell: secret not found in the keychain")

// ErrNoSecretStore is returned by SetSecret/DeleteSecret when the Controller
// was built without a store (WithSecretStore not supplied).
var ErrNoSecretStore = errors.New("shell: no secret store configured")

// SecretStore is the OS-keychain seam (ADR-0035 §4, ADR-0037): service
// `korvun`, account = env-var NAME, value = the secret. Get returns
// ErrSecretNotFound on a miss; Delete REMOVES the entry (no orphaned or
// emptied entries). The real backend lives in internal/shell/keyring; tests
// use an in-memory double.
type SecretStore interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

// WithSecretStore injects the keychain seam. Without it (nil, the default)
// secret provisioning is skipped entirely — the SP2 behavior, byte for byte.
func WithSecretStore(s SecretStore) Option {
	return func(c *Controller) { c.secrets = s }
}

// SecretEnvNames enumerates the secret env-var NAMES the config references:
// every channel token_env and every model api_key_env (non-empty),
// deduplicated and sorted for determinism. admin.token_env is deliberately
// EXCLUDED — that is the SP2 per-cycle bearer, generated in-process, never
// stored in a keychain.
func SecretEnvNames(cfg *config.Config) []string {
	seen := make(map[string]bool)
	for _, ch := range cfg.Channels {
		if ch.TokenEnv != "" {
			seen[ch.TokenEnv] = true
		}
	}
	for _, b := range cfg.Brains {
		for _, m := range b.Models {
			if m.APIKeyEnv != "" {
				seen[m.APIKeyEnv] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// SetSecret writes a secret to the keychain (the future UI's path, FR-SEC-5).
// It takes effect on the NEXT cycle — provisioning happens at Start.
func (c *Controller) SetSecret(name, value string) error {
	if c.secrets == nil {
		return ErrNoSecretStore
	}
	return c.secrets.Set(name, value)
}

// DeleteSecret removes a secret from the keychain (no orphans: the entry is
// deleted, never emptied). Effective on the NEXT cycle, like SetSecret.
func (c *Controller) DeleteSecret(name string) error {
	if c.secrets == nil {
		return ErrNoSecretStore
	}
	return c.secrets.Delete(name)
}

// SecretInKeychain reports whether the keychain holds an entry named name —
// PRESENCE only (SP6c wizard's "Comprobar entorno"). The seam's Get reads
// the value internally; it is dropped on this line and never crosses the
// boundary, is never logged, and never reaches a caller. No store configured
// reads as not-present (the honest answer, not an error); a miss is false;
// any other store failure surfaces as an error.
func (c *Controller) SecretInKeychain(name string) (bool, error) {
	if c.secrets == nil {
		return false, nil
	}
	if _, err := c.secrets.Get(name); err != nil {
		if errors.Is(err, ErrSecretNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("shell: keychain presence check for %q: %w", name, err)
	}
	return true, nil
}

// provisionSecrets (mu held, called by Start BEFORE app.Build) fills each
// config-referenced secret variable that is ABSENT from the environment from
// the keychain. The sacred precedence (ADR-0035 §4): a variable already in
// the environment wins and the store is not even consulted for it. "Present"
// means NON-EMPTY — a set-but-empty variable counts as absent, matching the
// core's own missing-secret semantics (os.Getenv == "" → ErrMissingSecret),
// so an inherited `X=` from a profile cannot defeat a keychain that could
// satisfy it. A store miss is skipped silently (the boot then fails with
// ErrMissingSecret naming the variable); any other store failure aborts,
// naming the VARIABLE — never a value. A config that reuses the admin
// bearer's variable as a channel/model secret is rejected loudly: the
// bearer's per-cycle Setenv would silently clobber it. It returns the names
// it set, for cycle-end hygiene.
func (c *Controller) provisionSecrets() ([]string, error) {
	if c.secrets == nil {
		return nil, nil
	}
	bearerEnv := ""
	if c.cfg.Admin != nil {
		bearerEnv = c.cfg.Admin.TokenEnv
	}
	var provisioned []string
	for _, name := range SecretEnvNames(c.cfg) {
		if name == bearerEnv {
			return provisioned, fmt.Errorf(
				"shell: config reuses the admin bearer variable %q as a channel/model secret — rename one of them", name)
		}
		if v, ok := os.LookupEnv(name); ok && v != "" {
			continue // non-empty env wins; the keychain is not consulted for this name
		}
		v, err := c.secrets.Get(name)
		if errors.Is(err, ErrSecretNotFound) {
			continue
		}
		if err != nil {
			return provisioned, fmt.Errorf("shell: keychain lookup for %q: %w", name, err)
		}
		if err := os.Setenv(name, v); err != nil {
			return provisioned, fmt.Errorf("shell: set env %q: %w", name, err)
		}
		provisioned = append(provisioned, name)
	}
	return provisioned, nil
}

// clearProvisioned unsets exactly the variables the shell provisioned this
// cycle — never a pre-existing env var (the shell removes only what it
// wrote, the same hygiene as the bearer).
func clearProvisioned(names []string) {
	for _, n := range names {
		_ = os.Unsetenv(n)
	}
}
