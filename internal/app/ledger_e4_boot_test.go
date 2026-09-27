// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// Train E, batch 4 (GE8, GE9) — the boot over a damaged ledger: TE12, TE40,
// TE41, TE58 and TE59 (plan v3, §6 and §7). The non-strict boot lives, names
// the standing and writes nothing; the warnings of the steps it skips carry the
// standing's own cause; the strict boot keeps the outcomes it inherits.
//
// Evidence level: real app on loopback, in-process host, real files.

// e4Tables is every table of the current schema.
func e4Tables() []string { return actionsqlite.SchemaTablesForTest() }

// e4StandingErr is the error a writer handle of path, born for profile, gets
// from Standing: the cause the boot's warnings owe.
func e4StandingErr(t *testing.T, path, profile string) string {
	t.Helper()
	store, err := actionsqlite.OpenFor(path, ProfileIdentity(profile))
	if err != nil {
		t.Fatalf("open the fixture: %v", err)
	}
	defer func() { _ = store.Close() }()
	_, _, serr := store.Standing(context.Background())
	if !errors.Is(serr, actionsqlite.ErrLedgerUnreadable) {
		t.Fatalf("the fixture's Standing = %v, want a verdict", serr)
	}
	return serr.Error()
}

// TE12 · for each of the 34 tables, a founded non-strict ledger without it:
// the boot lives; GET 200 names the table unreadable; a change door answers
// 503 act_not_recorded naming ledger_unreadable; no reload is asked; not one
// INSERT, UPDATE or DELETE is attempted on any other table; and the catalog —
// type, name, target table and DDL of every object — is exactly what it was.
//
// PROBING MUTATIONS (MU12): the ink registered in the ledger, or the expiry
// sweep run, whatever the standing → a trapped write or a fatal boot →
// reddens.
func TestE4_TE12_everyMissingTableBootsNamedAndTouchesNothing(t *testing.T) {
	for _, table := range e4Tables() {
		t.Run(table, func(t *testing.T) {
			cfg := e1Cfg(t)
			path, founder := e1FoundedFor(t, cfg)
			e4PreinitConversations(t, path)
			raw := e1Raw(t, path)
			e1Exec(t, raw, `PRAGMA foreign_keys = OFF`)
			e1Exec(t, raw, `DROP TABLE `+table)
			traps := e4ArmTraps(t, path, e4Tables()...)
			before := e4Catalog(t, path)
			r := e4Start(t, cfg, founder)
			code, l, rawBody := r.ledger(t)
			if code != http.StatusOK || l["standing"] != "unreadable" || !strings.Contains(l["owner"].(string), table) {
				t.Fatalf("GET /api/whats-happening = %d %s, want 200 unreadable naming %s", code, rawBody, table)
			}
			code, out, rawBody := r.call(t, http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`)
			if code != http.StatusServiceUnavailable || out["outcome"] != "act_not_recorded" || out["profile_unchanged"] != true || !strings.Contains(rawBody, "ledger_unreadable") {
				t.Fatalf("a change door = %d %s, want 503 act_not_recorded naming ledger_unreadable", code, rawBody)
			}
			if n := r.reloader.asked(); n != 0 {
				t.Fatalf("the reloader was asked %d time(s)", n)
			}
			if n := traps.hits(); n != 0 {
				t.Fatalf("the boot attempted %d write(s) on a ledger without %s", n, table)
			}
			if d := e4CatalogDiff(before, e4Catalog(t, path)); len(d) != 0 {
				t.Fatalf("the boot changed the catalog: %v", d)
			}
		})
	}
}

// TE40 · the steps the boot skips on an unreadable ledger — the root intent
// and identity registration, the strict-authority activation check, the
// recovery pass and the expiry sweep — each say so in a warning carrying the
// standing's own cause, exactly, never nil; the boot lives; none of the skipped
// writes is attempted.
//
// PROBING MUTATIONS (MU40): one warning reads the generic err a later step
// reassigned → its cause is nil → reddens; the generic err left holding the
// verdict → the sweep's check kills the boot → reddens.
func TestE4_TE40_theSkippedStepsNameTheStandingsCause(t *testing.T) {
	cfg := e1Cfg(t)
	path, founder := e1FoundedFor(t, cfg)
	raw := e1Raw(t, path)
	e1Exec(t, raw, `PRAGMA foreign_keys = OFF`)
	e1Exec(t, raw, `DROP TABLE intents`)
	want := e4StandingErr(t, path, founder)
	traps := e4ArmTraps(t, path, e4Tables()...)
	sink := newE4Sink()
	e4Start(t, cfg, founder, WithLogger(slog.New(sink)))
	for _, msg := range []string{
		"boot: no root intent and no identity registration on a ledger this profile does not own or cannot read",
		"boot: the strict-authority activation check is skipped on an unreadable ledger",
		"recovery: skipped on a ledger this profile does not own or cannot read",
		"expiry sweep: skipped on an unreadable ledger",
	} {
		records := sink.find(msg)
		if len(records) != 1 {
			t.Fatalf("the warning %q was logged %d time(s), want once", msg, len(records))
		}
		if got := records[0].attrs["err"]; got != want {
			t.Fatalf("the warning %q carries err %q, want the standing's cause %q", msg, got, want)
		}
	}
	if n := traps.hits(); n != 0 {
		t.Fatalf("the boot attempted %d skipped write(s)", n)
	}
}

// TE41 · the boot over a bad shape adds nothing to the file. Over a ledger
// whose conversation tables are already there, the catalog after the boot, a
// GET and a refused POST is exactly the one before; over a ledger that never
// held them, the delta is exactly the conversation store's three tables with
// their own DDL — nothing else, whatever its name. The refused door answers
// 503 act_not_recorded.
//
// PROBING MUTATIONS (MU41): an index created on an action table at boot →
// reddens; the door's status changed from 503 to 409 → reddens.
func TestE4_TE41_theBootAddsNothingToABadShape(t *testing.T) {
	conv := e4ConversationCatalog(t)
	for _, preinit := range []bool{true, false} {
		name := "a preinitialised conversation file"
		if !preinit {
			name = "the first conversation open"
		}
		t.Run(name, func(t *testing.T) {
			cfg := e1Cfg(t)
			path, founder := e1FoundedFor(t, cfg)
			if preinit {
				e4PreinitConversations(t, path)
			}
			raw := e1Raw(t, path)
			e1Exec(t, raw, `PRAGMA foreign_keys = OFF`)
			e1Exec(t, raw, `DROP TABLE intents`)
			before := e4Catalog(t, path)
			r := e4Start(t, cfg, founder)
			if code, l, rawBody := r.ledger(t); code != http.StatusOK || l["standing"] != "unreadable" {
				t.Fatalf("GET = %d %s, want 200 unreadable", code, rawBody)
			}
			code, out, rawBody := r.call(t, http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`)
			if code != http.StatusServiceUnavailable || out["outcome"] != "act_not_recorded" {
				t.Fatalf("a change door = %d %s, want exactly 503 act_not_recorded", code, rawBody)
			}
			want := map[string]string{}
			for k, v := range before {
				want[k] = v
			}
			if !preinit {
				for k, v := range conv {
					want[k] = v
				}
			}
			if d := e4CatalogDiff(want, e4Catalog(t, path)); len(d) != 0 {
				t.Fatalf("the catalog after the boot differs from the allowance: %v", d)
			}
		})
	}
}

// e4StrictProfile is kernelWiringConfig with its one brain named alpha — an
// agent brain when agent is true — activated strict on disk, the admin door
// on, and the version column of its ledger renamed.
func e4StrictProfile(t *testing.T, agent bool) *config.Config {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := kernelWiringConfig(dbPath)
	cfg.Brains[0].Name = "alpha"
	cfg.Routes[0].Brain = "alpha"
	if agent {
		cfg.Brains[0].Agent = &config.AgentConfig{Tools: []string{"calc"}}
	}
	digest := activateAuthorityOnDisk(t, cfg)
	cfg.Authority = &config.AuthorityConfig{Mode: "strict", ActivationDigest: digest}
	t.Setenv("KORVUN_E1_TEST_ADMIN", "tok")
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_E1_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	e1Exec(t, e1Raw(t, dbPath), `ALTER TABLE action_schema RENAME COLUMN version TO v`)
	return cfg
}

// e4AuthorityRows counts the rows of the strict authority's tables, raw.
func e4AuthorityRows(t *testing.T, path string) string {
	t.Helper()
	return e4Stored(t, path, `SELECT (SELECT COUNT(*) FROM config_authority_heads) || '/' || (SELECT COUNT(*) FROM config_authority_snapshots)`)
}

// TE58 · a strict activated profile with an agent brain over a ledger whose
// version column is renamed: Build fails where the strict clauses are
// established — «app: persist strict authority for brain "alpha":» over
// ErrLedgerUnreadable — with no app, and no clause written.
//
// PROBING MUTATION (MU58): the strict setup bypassed on an unreadable ledger
// → Build succeeds → reddens.
func TestE4_TE58_theStrictBootWithAnAgentBrainFailsNamed(t *testing.T) {
	cfg := e4StrictProfile(t, true)
	before := e4AuthorityRows(t, cfg.Storage.Path)
	a, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))), withTestProfile())
	if a != nil {
		shutdownApp(t, a)
	}
	if a != nil || !errors.Is(err, actionsqlite.ErrLedgerUnreadable) || !strings.HasPrefix(err.Error(), `app: persist strict authority for brain "alpha":`) {
		t.Fatalf("Build = app %v, %v; want no app and «app: persist strict authority for brain \"alpha\":» over ErrLedgerUnreadable", a != nil, err)
	}
	if after := e4AuthorityRows(t, cfg.Storage.Path); after != before {
		t.Fatalf("the refused strict boot changed the authority rows: %s → %s", before, after)
	}
}

// TE59 · a strict activated profile with no agent brain over the same damage:
// Build succeeds in the inherited strict branch; GET names the ledger
// unreadable; a change door is refused 503 act_not_recorded; no clause is
// written and no reload is asked.
//
// PROBING MUTATION (MU59): the strict boot fails on any unreadable ledger →
// reddens.
func TestE4_TE59_theStrictBootWithNoAgentBrainLivesBlocked(t *testing.T) {
	cfg := e4StrictProfile(t, false)
	before := e4AuthorityRows(t, cfg.Storage.Path)
	r := e4Start(t, cfg, testProfilePath)
	if code, l, rawBody := r.ledger(t); code != http.StatusOK || l["standing"] != "unreadable" {
		t.Fatalf("GET = %d %s, want 200 unreadable", code, rawBody)
	}
	if code, out, rawBody := r.call(t, http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`); code != http.StatusServiceUnavailable || out["outcome"] != "act_not_recorded" {
		t.Fatalf("a change door = %d %s, want 503 act_not_recorded", code, rawBody)
	}
	if n := r.reloader.asked(); n != 0 {
		t.Fatalf("the reloader was asked %d time(s)", n)
	}
	if after := e4AuthorityRows(t, cfg.Storage.Path); after != before {
		t.Fatalf("the strict boot changed the authority rows: %s → %s", before, after)
	}
}
