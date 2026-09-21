// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"path/filepath"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

func TestPrepareStrictAuthority_RefusesUnverifiedActivation(t *testing.T) {
	store, err := actionsqlite.Open(filepath.Join(t.TempDir(), "authority.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := &config.Config{Authority: &config.AuthorityConfig{
		Mode: "strict", ActivationDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}}
	if err := PrepareStrictAuthority(context.Background(), cfg, store); err == nil {
		t.Fatal("strict preparation accepted a missing activation root")
	}
}
