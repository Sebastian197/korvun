// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// Train E, batch 4 (GE2, GE3, GE5, GE7) — the doors of a running app over a
// damaged ledger: TE44, TE45, TE61 and TE62 (plan v3, §6 and §7, N9-1 step
// 6). Every door that would write judges the ledger first and refuses by
// name; a valid pending approval stays pending; nothing is reloaded, decided
// or sealed.
//
// Evidence level: real app on loopback, in-process host; the damage comes
// through a second real connection to the same file.

// e4ParkingProfile is e1Cfg with approvals on and two agent brains: the first
// parks (its webhook_call caged, a ceiling that reaches it) and is named "a",
// the brain parkOne's principal names (principal_brain_a), so the approval's
// door reaches the ledger instead of answering brain_gone; the second is what
// the ordinary doors change — no ceiling, its webhook_call in shadow, a cage
// one host wide — so each door's request is a real change.
func e4ParkingProfile(t *testing.T) *config.Config {
	t.Helper()
	cfg := e1Cfg(t)
	cfg.Brains[0].Name = "a"
	cfg.Routes[0].Brain = "a"
	cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
	cfg.Brains[0].Agent = &config.AgentConfig{
		Tools: []string{"webhook_call"}, MaxIterations: 2, EffectCeiling: "critical",
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"hooks.acme.io"}},
	}
	helper := ollamaBrain()
	helper.Name = "helper"
	helper.Agent = &config.AgentConfig{
		Tools: []string{"webhook_call"}, MaxIterations: 2,
		Governance:  []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}},
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"hooks.acme.io"}},
	}
	cfg.Brains = append(cfg.Brains, helper)
	return cfg
}

// e4Parked parks one valid request in the ledger at path, through a writer
// handle born for profile, under the law cfg resolves now; the handle closes.
func e4Parked(t *testing.T, cfg *config.Config, path, profile, id string) action.Approval {
	t.Helper()
	store, err := actionsqlite.OpenFor(path, ProfileIdentity(profile))
	if err != nil {
		t.Fatalf("open the ledger to park: %v", err)
	}
	defer func() { _ = store.Close() }()
	return parkOne(t, cfg, store, id)
}

// e4Ordinary are the four ordinary doors, each with a request that changes
// e4ParkingProfile.
var e4Ordinary = []struct{ door, body string }{
	{"enable-approvals", `{"confirm":true}`},
	{"set-ceiling", `{"confirm":true,"brain":"helper"}`},
	{"lift-shadow", `{"confirm":true,"brain":"helper","tool":"webhook_call"}`},
	{"allow-host", `{"confirm":true,"brain":"helper","tool":"webhook_call","host":"hooks.nuevo.io"}`},
}

// e4ApprovalState is what the ledger says of one approval and of the receipts,
// raw: the approval's status and the receipt count.
func e4ApprovalState(t *testing.T, path, id string) string {
	t.Helper()
	return e4Stored(t, path, `SELECT status FROM approvals WHERE approval_id = ?`, id) + " · receipts " +
		e4Stored(t, path, `SELECT COUNT(*) FROM receipts`)
}

// TE44 · a non-strict profile with approvals on and a valid pending approval,
// its ledger damaged before the boot — the version column renamed (TE13), a
// required UNIQUE index dropped (TE09), each its own app: GET 200 names the
// defect unreadable; each of the four ordinary doors, asked for a real change,
// answers 503 act_not_recorded with the profile unchanged, naming
// ledger_unreadable; the approval's door answers 503 ledger_unreadable; the
// approval stays pending; no reload is asked; no write is attempted.
//
// PROBING MUTATIONS (MU44): the judge skips a missing UNIQUE index → the doors
// proceed → reddens; the approvals adapter names a structural cause
// approvals_unavailable → reddens.
func TestE4_TE44_theDoorsRefuseADamagedLedgerByName(t *testing.T) {
	for _, damage := range []struct{ name, sql, names string }{
		{"TE13, a renamed version column", `ALTER TABLE action_schema RENAME COLUMN version TO v`, "version"},
		{"TE09, a dropped UNIQUE index", `DROP INDEX budget_debits_sequence`, "budget_debits_sequence"},
	} {
		t.Run(damage.name, func(t *testing.T) {
			cfg := e4ParkingProfile(t)
			path, founder := e1FoundedFor(t, cfg)
			apr := e4Parked(t, cfg, path, founder, "act_te44")
			before := e4ApprovalState(t, path, apr.ApprovalID)
			if !strings.HasPrefix(before, "PENDING ") {
				t.Fatalf("the parked approval is %q, want pending", before)
			}
			e1Exec(t, e1Raw(t, path), damage.sql)
			traps := e4ArmTraps(t, path, "actions", "action_decisions", "approvals", "approval_tombstones", "receipts")
			r := e4Start(t, cfg, founder)
			if code, l, rawBody := r.ledger(t); code != http.StatusOK || l["standing"] != "unreadable" || !strings.Contains(l["owner"].(string), damage.names) {
				t.Fatalf("GET = %d %s, want 200 unreadable naming %s", code, rawBody, damage.names)
			}
			for _, d := range e4Ordinary {
				code, out, rawBody := r.call(t, http.MethodPost, "/api/whats-happening/"+d.door, d.body)
				if code != http.StatusServiceUnavailable || out["outcome"] != "act_not_recorded" || out["profile_unchanged"] != true || !strings.Contains(rawBody, "ledger_unreadable") {
					t.Fatalf("%s = %d %s, want 503 act_not_recorded naming ledger_unreadable", d.door, code, rawBody)
				}
			}
			code, out, rawBody := r.call(t, http.MethodPost, "/api/approvals/"+apr.ApprovalID+"/approve", `{"digest":"`+apr.ActionDigest+`"}`)
			if code != http.StatusServiceUnavailable || out["error"] != "ledger_unreadable" {
				t.Fatalf("approve = %d %s, want 503 ledger_unreadable", code, rawBody)
			}
			if n := r.reloader.asked(); n != 0 {
				t.Fatalf("the reloader was asked %d time(s)", n)
			}
			if n := traps.hits(); n != 0 {
				t.Fatalf("%d write(s) were attempted", n)
			}
			if after := e4ApprovalState(t, path, apr.ApprovalID); after != before {
				t.Fatalf("the approval and receipts moved: %q → %q", before, after)
			}
		})
	}
}

// TE45 · a RUNNING healthy app with a valid pending approval. A second real
// connection renames the version column and commits; the approval is refused
// 503 ledger_unreadable. The column is restored and committed; the approval,
// asked again through the same app, is still refused — its connection's guard
// is sticky. Nothing is decided. A new app over the restored file finds a
// healthy ledger and the approval still pending.
//
// PROBING MUTATIONS (MU45): the judgement only at connection birth → the
// first request decides → reddens; the guard's sticky WHERE removed → the
// second request decides → reddens.
func TestE4_TE45_aLiveDamageIsRefusedAndTheGuardSticks(t *testing.T) {
	cfg := e4ParkingProfile(t)
	path, founder := e1FoundedFor(t, cfg)
	apr := e4Parked(t, cfg, path, founder, "act_te45")
	before := e4ApprovalState(t, path, apr.ApprovalID)
	r := e4Start(t, cfg, founder)
	if code, l, rawBody := r.ledger(t); code != http.StatusOK || l["standing"] != "ok" {
		t.Fatalf("GET before the damage = %d %s, want ok", code, rawBody)
	}
	attacker := e1Raw(t, path)
	approve := func(label string) {
		t.Helper()
		code, out, rawBody := r.call(t, http.MethodPost, "/api/approvals/"+apr.ApprovalID+"/approve", `{"digest":"`+apr.ActionDigest+`"}`)
		if code != http.StatusServiceUnavailable || out["error"] != "ledger_unreadable" {
			t.Fatalf("%s: approve = %d %s, want 503 ledger_unreadable", label, code, rawBody)
		}
	}
	e1Exec(t, attacker, `ALTER TABLE action_schema RENAME COLUMN version TO v`)
	approve("after the damage")
	e1Exec(t, attacker, `ALTER TABLE action_schema RENAME COLUMN v TO version`)
	approve("after the restore, same app")
	if after := e4ApprovalState(t, path, apr.ApprovalID); after != before {
		t.Fatalf("the approval and receipts moved: %q → %q", before, after)
	}
	r.stop()
	fresh := e4Start(t, cfg, founder)
	if code, l, rawBody := fresh.ledger(t); code != http.StatusOK || l["standing"] != "ok" {
		t.Fatalf("GET from a new app over the restored file = %d %s, want ok", code, rawBody)
	}
	if after := e4ApprovalState(t, path, apr.ApprovalID); after != before {
		t.Fatalf("the approval and receipts moved: %q → %q", before, after)
	}
}

// e4LegacyParked is N9-1's live legacy ledger: registered and inked, never
// founded — no identity row, no mark — with one unmarked receipt and one valid
// pending approval, checked before the app starts.
func e4LegacyParked(t *testing.T, cfg *config.Config, profile, id string) (string, action.Approval) {
	t.Helper()
	path := e1LegacyWithReceipt(t, cfg)
	apr := e4Parked(t, cfg, path, profile, id)
	if rows := e4Stored(t, path, `SELECT COUNT(*) FROM ledger_identity`); rows != "0" {
		t.Fatalf("the legacy ledger has %s identity row(s)", rows)
	}
	if marks := e4Stored(t, path, `SELECT COUNT(*) FROM receipts WHERE result_digest LIKE ?`, actionsqlite.ProfileMarkPrefix+"%"); marks != "0" {
		t.Fatalf("the legacy ledger has %s marked receipt(s)", marks)
	}
	return path, apr
}

// TE61 and TE62 · N9-1 step 6, literally, on ONE running legacy app with
// approvals on: after the app is ready, a second real connection renames
// receipts.result_digest to rd and commits. The approval is refused 503
// ledger_unreadable (the first request's own judgement: the guard was
// healthy, and only the mark query reads that column). Then enable-approvals,
// confirmed, although approvals are already on: 503 act_not_recorded, the
// profile unchanged, the detail naming ledger_unreadable AND result_digest —
// the second request's own mark read, which a sticky guard alone could not
// name. The approval stays pending; nothing is reloaded; the column stays
// renamed. Then the column is restored and the approval asked again through
// the same app: still 503 ledger_unreadable, still PENDING — the guard the
// refusals set is sticky, and only a verdict sets it (step adjudicated on
// 2026-09-27).
//
// PROBING MUTATIONS (MU61, MU62): the mark query's failure left unclassified →
// a different name → reddens; swallowed as «no mark» → the approval is
// decided → reddens; only the mark query's verdict branch skipped, the code
// left for the boundary to name (MU61-verdictbranchonly) → the guard is never
// set → the approval asked after the restore is decided → reddens;
// enable-approvals answering success when approvals are already on, without
// BeginConfigAct → reddens.
func TestE4_TE61_TE62_theLiveLegacyAppRefusesOnceTheMarkColumnMoves(t *testing.T) {
	cfg := e4ParkingProfile(t)
	profile := t.TempDir() + "/p/korvun.json"
	path, apr := e4LegacyParked(t, cfg, profile, "act_te61")
	before := e4ApprovalState(t, path, apr.ApprovalID)
	r := e4Start(t, cfg, profile)
	if code, l, rawBody := r.ledger(t); code != http.StatusOK || l["standing"] != "legacy_unfounded" {
		t.Fatalf("GET before the damage = %d %s, want legacy_unfounded", code, rawBody)
	}
	e1Exec(t, e1Raw(t, path), `ALTER TABLE receipts RENAME COLUMN result_digest TO rd`)
	t.Run("TE61: approve", func(t *testing.T) {
		code, out, rawBody := r.call(t, http.MethodPost, "/api/approvals/"+apr.ApprovalID+"/approve", `{"digest":"`+apr.ActionDigest+`"}`)
		if code != http.StatusServiceUnavailable || out["error"] != "ledger_unreadable" {
			t.Fatalf("approve = %d %s, want 503 ledger_unreadable", code, rawBody)
		}
	})
	t.Run("TE62: enable-approvals, already on", func(t *testing.T) {
		code, out, rawBody := r.call(t, http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`)
		detail, _ := out["detail"].(string)
		if code != http.StatusServiceUnavailable || out["outcome"] != "act_not_recorded" || out["profile_unchanged"] != true ||
			!strings.Contains(detail, "ledger_unreadable") || !strings.Contains(detail, "result_digest") {
			t.Fatalf("enable-approvals = %d %s, want 503 act_not_recorded naming ledger_unreadable and result_digest", code, rawBody)
		}
	})
	if n := r.reloader.asked(); n != 0 {
		t.Fatalf("the reloader was asked %d time(s)", n)
	}
	if status := e4Stored(t, path, `SELECT status FROM approvals WHERE approval_id = ?`, apr.ApprovalID); !strings.HasPrefix(before, status+" ") {
		t.Fatalf("the approval is %q, it was %q", status, before)
	}
	if cols := e4Stored(t, path, `SELECT group_concat(name) FROM pragma_table_info('receipts') WHERE name IN ('rd', 'result_digest')`); cols != "rd" {
		t.Fatalf("the receipts columns are %q, want the rename kept", cols)
	}
	t.Run("TE61: the column restored, the same app", func(t *testing.T) {
		e1Exec(t, e1Raw(t, path), `ALTER TABLE receipts RENAME COLUMN rd TO result_digest`)
		code, out, rawBody := r.call(t, http.MethodPost, "/api/approvals/"+apr.ApprovalID+"/approve", `{"digest":"`+apr.ActionDigest+`"}`)
		if code != http.StatusServiceUnavailable || out["error"] != "ledger_unreadable" {
			t.Fatalf("approve after the restore = %d %s, want 503 ledger_unreadable", code, rawBody)
		}
		if status := e4Stored(t, path, `SELECT status FROM approvals WHERE approval_id = ?`, apr.ApprovalID); status != "PENDING" {
			t.Fatalf("the approval after the restore is %q, want PENDING", status)
		}
	})
}

// TE62, the adjacent case · the same damage on a live legacy app with
// approvals OFF: enable-approvals is a real change here, and it is refused the
// same way — 503 act_not_recorded, the profile unchanged, naming
// ledger_unreadable and result_digest.
func TestE4_TE62_withApprovalsOffTheDoorIsRefusedTheSameWay(t *testing.T) {
	cfg := e4ParkingProfile(t)
	cfg.Approvals = &config.ApprovalsConfig{Enabled: false}
	profile := t.TempDir() + "/p/korvun.json"
	path := e1LegacyWithReceipt(t, cfg)
	r := e4Start(t, cfg, profile)
	e1Exec(t, e1Raw(t, path), `ALTER TABLE receipts RENAME COLUMN result_digest TO rd`)
	code, out, rawBody := r.call(t, http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`)
	detail, _ := out["detail"].(string)
	if code != http.StatusServiceUnavailable || out["outcome"] != "act_not_recorded" || out["profile_unchanged"] != true ||
		!strings.Contains(detail, "ledger_unreadable") || !strings.Contains(detail, "result_digest") {
		t.Fatalf("enable-approvals = %d %s, want 503 act_not_recorded naming ledger_unreadable and result_digest", code, rawBody)
	}
	if n := r.reloader.asked(); n != 0 {
		t.Fatalf("the reloader was asked %d time(s)", n)
	}
}
