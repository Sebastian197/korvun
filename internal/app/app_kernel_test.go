// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Action Kernel boot wiring (lote 3b, spec FR-WIRE): the kernel's store
// opens at boot on the SHARED storage file with its OWN lifecycle (the
// sealed decision), boot-fatal on failure, zero new config. Approved-red
// contract.

package app

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

func kernelWiringConfig(dbPath string) *config.Config {
	return &config.Config{
		Channels: []config.ChannelConfig{telegramChannel()},
		Brains: []config.BrainConfig{
			{Name: "a", Sensitivity: "public", Policy: config.PolicyConfig{Kind: "priority"},
				Models: []config.ModelConfig{{Provider: "ollama", ModelID: "m", Locality: "local"}}},
		},
		Routes:  []config.RouteConfig{{Channel: "telegram", Brain: "a"}},
		Storage: &config.StorageConfig{Path: dbPath},
	}
}

func TestBuild_storage_bootstrapsTheActionKernelStoreOnTheSharedFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build with storage: %v", err)
	}
	defer shutdownApp(t, app)

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("open shared file: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, table := range []string{"actions", "action_decisions", "action_schema", "sessions"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table') AND name = ?`, table,
		).Scan(&n); err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		if n != 1 {
			t.Fatalf("the SHARED file must carry both stores' schemas; missing %q", table)
		}
	}
}

func TestBuild_stateless_keepsTheKernelRecordingOff(t *testing.T) {
	cfg := kernelWiringConfig("")
	cfg.Storage = nil
	app, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("stateless Build: %v", err)
	}
	defer shutdownApp(t, app)
	if app.actions != nil {
		t.Fatal("stateless boot (no storage block) must not open an action store")
	}
}
