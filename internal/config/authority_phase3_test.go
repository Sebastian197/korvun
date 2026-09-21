// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

func TestAuthority_StrictModeRequiresStorage(t *testing.T) {
	cfg := validBase()
	cfg.Authority = &AuthorityConfig{Mode: "strict", ActivationDigest: "sha256:" + strings.Repeat("a", 64)}
	cfg.Storage = nil
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "authority.mode") || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("Validate error = %v, want exact authority.mode storage refusal", err)
	}
	cfg.Storage = &StorageConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("strict authority with storage: %v", err)
	}
}

func TestAuthority_StrictModeRequiresPinnedActivation(t *testing.T) {
	cfg := validBase()
	cfg.Storage = &StorageConfig{}
	for _, digest := range []string{"", "sha256:nope", "SHA256:" + strings.Repeat("a", 64)} {
		cfg.Authority = &AuthorityConfig{Mode: "strict", ActivationDigest: digest}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "authority.activation_digest") {
			t.Fatalf("digest %q error = %v, want activation_digest refusal", digest, err)
		}
	}
}
