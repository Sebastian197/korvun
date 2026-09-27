// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	convsqlite "github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// Train E, batch 2 — the app's half of TE68 (plan v3, §13.1): a REAL boot
// over a shared file that carries the conversations and an EMPTY prefix of
// the action store's v1 bootstrap completes the ledger, and the screen's GET
// names it legacy_unfounded — never unreadable, never D2.
//
// Evidence level: real app on loopback over a real file (Build, Run, HTTP).
// TE68's one-shot residue fault is armed only by the sqlite package's half —
// its seam is internal to that package (plan §7: no cross-package test
// control) — so this half meets the prefix itself, not the fault.

// e2AppV1Bootstrap is the v1 bootstrap of the action store, statement by
// statement, as internal/action/sqlite's createStmt writes it.
var e2AppV1Bootstrap = []string{
	`CREATE TABLE IF NOT EXISTS action_schema (
    version INTEGER NOT NULL
)`,
	`CREATE TABLE IF NOT EXISTS actions (
    action_id         TEXT    NOT NULL PRIMARY KEY,
    schema_version    INTEGER NOT NULL,
    correlation_id    TEXT    NOT NULL,
    source_kind       TEXT    NOT NULL,
    source_protocol   TEXT    NOT NULL,
    source_channel    TEXT    NOT NULL,
    op_namespace      TEXT    NOT NULL,
    op_name           TEXT    NOT NULL,
    op_version        INTEGER NOT NULL,
    parameters_digest TEXT    NOT NULL,
    effect_class      TEXT    NOT NULL,
    state             TEXT    NOT NULL,
    recovery_marker   TEXT    NOT NULL DEFAULT '',
    requested_at      TEXT    NOT NULL,
    finished_at       TEXT
) WITHOUT ROWID`,
	`CREATE INDEX IF NOT EXISTS actions_by_correlation ON actions(correlation_id)`,
	`CREATE INDEX IF NOT EXISTS actions_by_requested ON actions(requested_at)`,
	`CREATE TABLE IF NOT EXISTS action_decisions (
    action_id  TEXT NOT NULL PRIMARY KEY REFERENCES actions(action_id) ON DELETE CASCADE,
    outcome    TEXT NOT NULL,
    rule       TEXT NOT NULL,
    decided_at TEXT NOT NULL
) WITHOUT ROWID`,
}

// TE68 (the app's half) · for each prefix k = 1…5 of a shared file, the real
// boot seeds the ledger and GET /api/whats-happening says legacy_unfounded;
// the version row is 16 and the conversation row is still there.
//
// PROBING MUTATION (MU02, the recogniser's): classify a prefix bad → the boot
// opens the ledger blocked → the GET says unreadable → reddens. MU68 needs
// the pool's one-shot fault, and only the sqlite package's half can arm it.
func TestE2_TE68_theBootOverAnEmptyPrefixNamesItLegacy(t *testing.T) {
	for k := 1; k <= 5; k++ {
		cfg := e1Cfg(t)
		path := filepath.Join(t.TempDir(), "korvun.db")
		conv, err := convsqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := conv.Close(); err != nil {
			t.Fatal(err)
		}
		raw := e1Raw(t, path)
		e1Exec(t, raw, `INSERT INTO sessions (key, id, created_ts) VALUES ('k', 1, 1)`)
		for _, q := range e2AppV1Bootstrap[:k] {
			e1Exec(t, raw, q)
		}
		cfg.Storage = &config.StorageConfig{Path: path}
		profile := filepath.Join(t.TempDir(), "profile", "korvun.json")
		l := e1Boot(t, cfg, profile)
		if l["standing"] != "legacy_unfounded" {
			t.Fatalf("k%d: GET ledger = %v, want standing legacy_unfounded", k, l)
		}
		var version, sessions int
		if err := raw.QueryRow(`SELECT version FROM action_schema`).Scan(&version); err != nil || version != 16 {
			t.Fatalf("k%d: the version row is %d (%v), want 16", k, version, err)
		}
		if err := raw.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 1 {
			t.Fatalf("k%d: sessions holds %d rows (%v), want the one it had", k, sessions, err)
		}
	}
}
