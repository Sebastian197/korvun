// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// TestPrepareStrictAuthority_RefusesUnverifiedActivation boots the strict
// preparation over a store that was NEVER activated, under a config that pins an
// activation digest: the boot must refuse, and by the name of what is wrong —
// there is no activation ledger to verify — not by «some error». With no agent
// brain in the config there is no clause sync to refuse as a second belt, so
// the activation check is the only thing standing here.
//
// Evidence level: in-process, one real SQLite store.
// Probing mutation executed: the activation check skipped — red with «error =
// <nil>, want action/sqlite: authorization snapshot corrupt».
func TestPrepareStrictAuthority_RefusesUnverifiedActivation(t *testing.T) {
	store, err := actionsqlite.Open(filepath.Join(t.TempDir(), "authority.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := &config.Config{Authority: &config.AuthorityConfig{
		Mode: "strict", ActivationDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}}
	if err := PrepareStrictAuthority(context.Background(), cfg, store); !errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("strict preparation over a missing activation root: error = %v, want %v", err, actionsqlite.ErrAuthorizationSnapshotCorrupt)
	}
}
