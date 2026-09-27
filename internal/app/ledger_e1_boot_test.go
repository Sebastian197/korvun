// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// Train E, batch 1 — the app's half of TE13, TE14, TE15, TE18, TE31, TE63,
// TE64 and TE67, and TE27's recorder projection (plan v3, §6): the REAL boot
// over a ledger damaged on disk. A structural defect is a live, non-strict
// app that names it on /api/whats-happening; a future schema is a dead boot
// named ErrSchemaFromTheFuture.
//
// Evidence level: real app on loopback over a real file (Build, Run, HTTP);
// the attacker is a second real connection. TE27's half is a recorder unit
// over a fake ledger (unit evidence).

func e1Cfg(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("KORVUN_E1_TEST_ADMIN", "tok")
	cfg := cfgWith(ollamaBrain())
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_E1_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	return cfg
}

func e1Raw(t *testing.T, path string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return raw
}

func e1Exec(t *testing.T, raw *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := raw.Exec(q, args...); err != nil {
		t.Fatalf("raw %q: %v", q, err)
	}
}

// e1Boot builds and runs the app for profile over cfg, and returns its
// /api/whats-happening ledger object; Build must succeed.
func e1Boot(t *testing.T, cfg *config.Config, profile string) map[string]any {
	t.Helper()
	a, err := Build(cfg,
		withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(profile),
	)
	if err != nil {
		t.Fatalf("Build over the damaged ledger = %v, want a live boot that names it", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for a.adminServer.Addr() == "" && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	t.Cleanup(func() {
		cancel()
		sctx, sc := context.WithTimeout(context.Background(), time.Second)
		defer sc()
		_ = a.Shutdown(sctx)
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+a.adminServer.Addr()+"/api/whats-happening", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/whats-happening: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/whats-happening = %d (%v)", resp.StatusCode, err)
	}
	l, _ := out["ledger"].(map[string]any)
	return l
}

// e1WantUnreadable: the screen's ledger is unreadable, its cause naming want.
func e1WantUnreadable(t *testing.T, l map[string]any, want ...string) {
	t.Helper()
	if l["standing"] != "unreadable" {
		t.Fatalf("GET ledger = %v, want standing unreadable", l)
	}
	for _, w := range want {
		if !strings.Contains(fmt.Sprint(l["owner"]), w) {
			t.Fatalf("GET ledger cause %q does not name %q", l["owner"], w)
		}
	}
}

// e1FoundedFor founds a ledger for the profile at founder.
func e1FoundedFor(t *testing.T, cfg *config.Config) (path, founder string) {
	t.Helper()
	founder = filepath.Join(t.TempDir(), "founder", "korvun.json")
	path = foundedLedger(t, cfg, ProfileIdentity(founder))
	cfg.Storage = &config.StorageConfig{Path: path}
	return path, founder
}

// e1LegacyWithReceipt is a registered, inked ledger never founded, with one
// UNMARKED receipt, closed.
func e1LegacyWithReceipt(t *testing.T, cfg *config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	env := action.NewEnvelope(action.NewID(), controlAPIWorkloadBrain,
		action.Source{Kind: "operator", Protocol: "http", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "probe", Version: 1}, `{}`, time.Now().UTC())
	if err := store.RecordAttempt(context.Background(), env, actionsqlite.Decision{Outcome: "allow", Rule: "operator"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.FinishWithResult(context.Background(), env.ActionID, action.StateSucceeded, time.Now().UTC(), "sha256:"+strings.Repeat("cd", 32)); err != nil {
		t.Fatalf("close unmarked: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.Storage = &config.StorageConfig{Path: path}
	return path
}

func e1PermissiveIdentity(t *testing.T, raw *sql.DB) {
	t.Helper()
	e1Exec(t, raw, `PRAGMA foreign_keys = OFF`)
	e1Exec(t, raw, `CREATE TABLE ledger_identity_p (id INTEGER, owner_digest TEXT, founded_by_action TEXT, adopted_by_action TEXT, written_at TEXT)`)
	e1Exec(t, raw, `INSERT INTO ledger_identity_p SELECT id, owner_digest, founded_by_action, adopted_by_action, written_at FROM ledger_identity`)
	e1Exec(t, raw, `DROP TABLE ledger_identity`)
	e1Exec(t, raw, `ALTER TABLE ledger_identity_p RENAME TO ledger_identity`)
}

// TE13 (the app's half) · the version column renamed: a live non-strict boot
// that names the defect. PROBING MUTATION (MU13): the shape judge propagates
// the code-1 failure as operational → the boot dies → reddens.
func TestE1_TE13_theBootOverARenamedVersionLives(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	e1Exec(t, e1Raw(t, path), `ALTER TABLE action_schema RENAME COLUMN version TO v`)
	e1WantUnreadable(t, e1Boot(t, cfg, founder), "version")
}

// TE14 (the app's half) · the owner column renamed: a live boot naming
// owner_digest. PROBING MUTATION (MU14): readOwner's structural branch
// removed → the boot dies → reddens.
func TestE1_TE14_theBootOverARenamedOwnerLives(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	e1Exec(t, e1Raw(t, path), `ALTER TABLE ledger_identity RENAME COLUMN owner_digest TO o`)
	e1WantUnreadable(t, e1Boot(t, cfg, founder), "owner_digest")
}

// TE15 (the app's half) · N9-1 steps 1–4: a legacy ledger with an unmarked
// receipt, result_digest renamed to rd: a live, blocked app naming it.
// PROBING MUTATION (MU15): judgeWithoutRow's failure without its class → the
// boot dies → reddens.
func TestE1_TE15_theBootOverARenamedMarkColumnLives(t *testing.T) {
	cfg := e1Cfg(t)
	path := e1LegacyWithReceipt(t, cfg)
	e1Exec(t, e1Raw(t, path), `ALTER TABLE receipts RENAME COLUMN result_digest TO rd`)
	e1WantUnreadable(t, e1Boot(t, cfg, filepath.Join(t.TempDir(), "p", "korvun.json")), "result_digest")
}

// TE18 (the app's half) · a future schema kills the non-strict boot with
// ErrSchemaFromTheFuture — never a live app offering D2 over a ledger from a
// newer binary. PROBING MUTATION (MU18): the owner read before the future
// check → another error → reddens.
func TestE1_TE18_theBootOverAFutureSchemaDiesNamed(t *testing.T) {
	for name, build := range map[string]func(*testing.T, *config.Config) (string, string){
		"a v1 file at version 99": func(t *testing.T, cfg *config.Config) (string, string) {
			path := filepath.Join(t.TempDir(), "korvun.db")
			raw := e1Raw(t, path)
			e1Exec(t, raw, `CREATE TABLE action_schema (version INTEGER NOT NULL)`)
			e1Exec(t, raw, `INSERT INTO action_schema (version) VALUES (99)`)
			e1Exec(t, raw, `CREATE TABLE actions (action_id TEXT NOT NULL PRIMARY KEY, state TEXT NOT NULL) WITHOUT ROWID`)
			e1Exec(t, raw, `CREATE TABLE action_decisions (action_id TEXT NOT NULL PRIMARY KEY, outcome TEXT NOT NULL) WITHOUT ROWID`)
			cfg.Storage = &config.StorageConfig{Path: path}
			return path, filepath.Join(t.TempDir(), "p", "korvun.json")
		},
		"a v16 file at version 17 with its owner column renamed": func(t *testing.T, cfg *config.Config) (string, string) {
			path, founder := e1FoundedFor(t, cfg)
			raw := e1Raw(t, path)
			e1Exec(t, raw, `ALTER TABLE ledger_identity RENAME COLUMN owner_digest TO o`)
			e1Exec(t, raw, `UPDATE action_schema SET version = 17`)
			return path, founder
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := e1Cfg(t)
			_, profile := build(t, cfg)
			a, err := Build(cfg,
				withChannelFactory(okFactory(newFakeChannel("telegram"))),
				WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(profile),
			)
			if a != nil {
				_ = a.Shutdown(context.Background())
				t.Fatal("Build over a future schema handed out an app")
			}
			if !errors.Is(err, actionsqlite.ErrSchemaFromTheFuture) {
				t.Fatalf("Build over a future schema = %v, want ErrSchemaFromTheFuture", err)
			}
		})
	}
}

// TE31 (the conversation boundary) · a text file where the profile's storage
// belongs dies at the CONVERSATION store's open, first: that fatal is not the
// ledger's and carries no ledger class. A pin of the boundary.
func TestE1_TE31_aTextFileKillsTheBootAtTheConversationStore(t *testing.T) {
	cfg := e1Cfg(t)
	path := filepath.Join(t.TempDir(), "korvun.db")
	if err := os.WriteFile(path, []byte(strings.Repeat("this file is not a SQLite database; it is text.\n", 128)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Storage = &config.StorageConfig{Path: path}
	a, err := Build(cfg,
		withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(filepath.Join(t.TempDir(), "p", "korvun.json")),
	)
	if a != nil {
		_ = a.Shutdown(context.Background())
		t.Fatal("Build over a text file handed out an app")
	}
	if err == nil || !strings.Contains(err.Error(), "conversation") || errors.Is(err, actionsqlite.ErrLedgerUnreadable) {
		t.Fatalf("Build over a text file = %v, want the conversation store's fatal, no ledger class", err)
	}
}

// TE63 (the app's half) · two identity rows: GET unreadable, the same
// answer as the hook's. PROBING MUTATION (MU63): the DB judge filters
// WHERE id = 1 → GET ok → reddens.
func TestE1_TE63_twoIdentityRowsAreUnreadableOnTheScreen(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	raw := e1Raw(t, path)
	e1PermissiveIdentity(t, raw)
	e1Exec(t, raw, `INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at) VALUES (2, ?, 'act_b', NULL, '2026-09-26T00:00:00Z')`,
		ProfileIdentity(filepath.Join(t.TempDir(), "b", "korvun.json")))
	e1WantUnreadable(t, e1Boot(t, cfg, founder))
}

// TE64 (the app's half) · a legacy ledger whose only identity row has id 2:
// GET unreadable, never legacy_unfounded. PROBING MUTATION (MU64): the
// wrong-id row read as no row → legacy → reddens.
func TestE1_TE64_aWrongIdRowIsUnreadableOnTheScreen(t *testing.T) {
	cfg := e1Cfg(t)
	path := e1LegacyWithReceipt(t, cfg)
	raw := e1Raw(t, path)
	e1PermissiveIdentity(t, raw)
	e1Exec(t, raw, `INSERT INTO ledger_identity (id, owner_digest, founded_by_action, adopted_by_action, written_at) VALUES (2, ?, 'act_b', NULL, '2026-09-26T00:00:00Z')`,
		ProfileIdentity(filepath.Join(t.TempDir(), "b", "korvun.json")))
	e1WantUnreadable(t, e1Boot(t, cfg, filepath.Join(t.TempDir(), "p", "korvun.json")))
}

// TE67 (the app's half) · the receipts' pages damaged on disk (the store's
// half calibrates the native CORRUPT at the mark read): the non-strict boot
// lives, blocked, and names the damage. PROBING MUTATION (MU67): the mark
// read's failure kept operational → the boot dies → reddens.
func TestE1_TE67_theBootOverDamagedReceiptPagesLives(t *testing.T) {
	cfg := e1Cfg(t)
	path := e1LegacyWithReceipt(t, cfg)
	raw := e1Raw(t, path)
	var pageSize int64
	if err := raw.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		t.Fatal(err)
	}
	var roots []int64
	rows, err := raw.Query(`SELECT rootpage FROM sqlite_master WHERE tbl_name = 'receipts' AND rootpage > 1`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r int64
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, r)
	}
	_ = rows.Close()
	var busy, logFrames, done int
	if err := raw.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &done); err != nil || busy != 0 || logFrames != done {
		t.Fatalf("checkpoint: %v busy=%d log=%d done=%d", err, busy, logFrames, done)
	}
	_ = raw.Close()
	data, err := os.ReadFile(path) //nolint:gosec // G304: the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roots {
		data[(r-1)*pageSize] = 0xff
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	e1WantUnreadable(t, e1Boot(t, cfg, filepath.Join(t.TempDir(), "p", "korvun.json")), "malformed")
}

// e1Coded is a coded storage error for the recorder unit.
type e1Coded struct{ code int }

func (e *e1Coded) Error() string { return fmt.Sprintf("injected coded failure %d", e.code) }
func (e *e1Coded) Code() int     { return e.code }

// environmentLedger is a ledger whose Standing fails with the environment
// class over an extended code.
type environmentLedger struct{ actLedger }

func (environmentLedger) Standing(context.Context) (actionsqlite.LedgerStanding, string, error) {
	return actionsqlite.LedgerStandingUnreadable, "", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerEnvironment, &e1Coded{code: 522})
}

// TE27 (the recorder's half) · a Standing named ErrLedgerEnvironment reaches
// the screen as `environment`, its cause kept — never `unreadable`, whose
// remedy is to replace the book.
//
// PROBING MUTATION: the recorder answers every failure unreadable → reddens.
//
// Evidence level: recorder unit over a fake ledger.
func TestE1_TE27_theRecorderNamesTheEnvironment(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	_, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	rec := newConfigActRecorderOver(environmentLedger{actLedger: store}, resolver, issuers["console"], func(error) {}, reg, path)
	rec.profile = ProfileIdentity(filepath.Join(t.TempDir(), "korvun.json"))
	standing, cause := rec.LedgerStanding(context.Background())
	if standing != "environment" || !strings.Contains(cause, "522") {
		t.Fatalf("a Standing named ErrLedgerEnvironment reaches the screen as %q / %q, want environment with its cause", standing, cause)
	}
}
