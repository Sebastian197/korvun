// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the durable mark «ledger founded by this profile» at the app
// (director's order, 2026-09-24): the profile's identity is the absolute path
// of its file; the boot hands it to the store; a REAL app over a ledger another
// profile founded refuses every change by name and adopts through its one open
// door; the founding close marks the receipt with the founder.
//
// Evidence level, honest: in-process; real SQLite ledgers on disk; a real app
// with a real admin server on loopback, a stub reloader. No supervisor cutover
// here (the shell's mould has it) and no packaged binary.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-libro-fundado-por-este-perfil-pretest.md, D1, D5, D7, G3, G6, G7.

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// TestProfileIdentity_isTheFileNotItsSpelling is D1: the same file, however
// spelt — relative, absolute, through `..`, through a symlink — is one
// identity; another file is another; no path is no identity.
//
// PROBING MUTATION: digest the path as given. The relative spelling reddens.
func TestProfileIdentity_isTheFileNotItsSpelling(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "korvun.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	relative, err := filepath.Rel(cwd, file)
	if err != nil {
		t.Fatalf("no relative spelling: %v", err)
	}
	dotted := dir + string(os.PathSeparator) + "sub" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "korvun.json"
	want := ProfileIdentity(file)
	if want == "" || !strings.HasPrefix(want, "sha256:") {
		t.Fatalf("ProfileIdentity(%q) = %q, want a sha256 digest", file, want)
	}
	for _, spelling := range []string{relative, dotted} {
		if got := ProfileIdentity(spelling); got != want {
			t.Fatalf("ProfileIdentity(%q) = %q, want %q: the same file spelt differently", spelling, got, want)
		}
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.json")
		if err := os.Symlink(file, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if got := ProfileIdentity(link); got != want {
			t.Fatalf("ProfileIdentity through a symlink = %q, want the target's %q", got, want)
		}
	}
	other := filepath.Join(dir, "other.json")
	if err := os.WriteFile(other, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if ProfileIdentity(other) == want {
		t.Fatal("two files share an identity")
	}
	if ProfileIdentity("") != "" {
		t.Fatal("an empty path has an identity")
	}
}

// foundedLedger founds a real ledger under owner's identity, the way a
// completed bootstrap leaves it, and returns its path.
func foundedLedger(t *testing.T, cfg *config.Config, owner string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	env := action.NewEnvelope(action.NewID(), controlAPIWorkloadBrain,
		action.Source{Kind: "operator", Protocol: "http", Channel: "console"},
		action.Operation{Namespace: "config", Name: "config.enable-storage", Version: 1}, `{"door":"enable-storage"}`, time.Now().UTC())
	if err := store.RecordAttempt(context.Background(), env, actionsqlite.Decision{Outcome: "allow", Rule: "operator"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.FinishFounding(context.Background(), env.ActionID, owner); err != nil {
		t.Fatalf("found: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

// TestMount_aForeignLedgerRefusesEveryDoorButAdopt is G7 and G3 on a REAL app:
// built with the identity of another profile over a founded ledger, every
// change door answers `ledger_foreign_profile` with nothing attempted, the
// read door says so, the adoption door takes the book with its receipt, and
// the same doors work afterwards — on the same running app, no restart.
//
// PROBING MUTATIONS: do not hand the store its identity in Build (every door
// applies and this reddens); cache the standing at open (the «afterwards»
// leg reddens).
func TestMount_aForeignLedgerRefusesEveryDoorButAdopt(t *testing.T) {
	t.Setenv("KORVUN_FOREIGN_TEST_ADMIN", "tok")
	brain := ollamaBrain()
	brain.Agent = &config.AgentConfig{
		Tools:       []string{"webhook_call"},
		Governance:  []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}},
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:8765"}},
	}
	cfg := cfgWith(brain)
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_FOREIGN_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	founder := filepath.Join(t.TempDir(), "founder", "korvun.json")
	cfg.Storage = &config.StorageConfig{Path: foundedLedger(t, cfg, ProfileIdentity(founder))}
	thisProfile := filepath.Join(t.TempDir(), "other", "korvun.json")

	reloader := &profileReloader{cfg: cfg}
	a, err := Build(cfg,
		withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(reloader), WithProfilePath(thisProfile),
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
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
	url := "http://" + a.adminServer.Addr()
	call := func(t *testing.T, method, path, body string) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), method, url+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	ledgerOf := func(out map[string]any) map[string]any {
		l, _ := out["ledger"].(map[string]any)
		return l
	}

	t.Run("the read door names the foreign ledger and its owner", func(t *testing.T) {
		code, out := call(t, http.MethodGet, "/api/whats-happening", "")
		l := ledgerOf(out)
		if code != http.StatusOK || l["standing"] != "ledger_foreign_profile" || l["owner"] != ProfileIdentity(founder) {
			t.Fatalf("GET /api/whats-happening = %d ledger %v, want foreign with the founder as owner", code, l)
		}
	})
	doors := []struct{ path, body string }{
		{"/api/whats-happening/enable-approvals", `{"confirm":true}`},
		{"/api/whats-happening/set-ceiling", `{"confirm":true,"brain":"default"}`},
		{"/api/whats-happening/lift-shadow", `{"confirm":true,"brain":"default","tool":"webhook_call"}`},
		{"/api/whats-happening/allow-host", `{"confirm":true,"brain":"default","tool":"webhook_call","host":"h"}`},
	}
	for _, d := range doors {
		t.Run(d.path+" refuses by name", func(t *testing.T) {
			code, out := call(t, http.MethodPost, d.path, d.body)
			if code != http.StatusConflict || out["outcome"] != controlapi.OutcomeLedgerForeign {
				t.Fatalf("%s = %d %v, want 409 ledger_foreign_profile", d.path, code, out)
			}
		})
	}
	t.Run("the builder's door refuses by name", func(t *testing.T) {
		raw, _ := json.Marshal(cfg)
		code, out := call(t, http.MethodPost, "/api/config", string(raw))
		if code != http.StatusConflict || out["error_code"] != "ledger_foreign_profile" {
			t.Fatalf("POST /api/config = %d %v, want 409 ledger_foreign_profile", code, out)
		}
	})
	if n := reloader.asked(); n != 0 {
		t.Fatalf("the reloader was asked %d times over a foreign ledger", n)
	}
	t.Run("the adoption door takes the book with its receipt", func(t *testing.T) {
		code, out := call(t, http.MethodPost, "/api/whats-happening/adopt-ledger", `{"confirm":true}`)
		if code != http.StatusOK || out["outcome"] != controlapi.OutcomeAdopted {
			t.Fatalf("adopt-ledger = %d %v, want 200 adopted", code, out)
		}
		act, _ := out["act"].(map[string]any)
		if act == nil || act["receipt_id"] == nil || act["receipt_id"] == "" {
			t.Fatalf("the adoption act travels without its receipt: %v", out)
		}
		row, receipts := readOnlyRow(t, cfg.Storage.Path, act["action_id"].(string))
		if row.State != action.StateSucceeded || len(receipts) != 1 || receipts[0].ResultDigest != actionsqlite.ProfileMarkPrefix+ProfileIdentity(thisProfile) {
			t.Fatalf("the adoption row is %q with receipts %+v, want SUCCEEDED marked with this profile", row.State, receipts)
		}
	})
	t.Run("afterwards, on the same running app, the doors work and the read door says ok", func(t *testing.T) {
		code, out := call(t, http.MethodGet, "/api/whats-happening", "")
		if l := ledgerOf(out); code != http.StatusOK || l["standing"] != "ok" {
			t.Fatalf("after adopting, GET /api/whats-happening ledger = %v, want ok", l)
		}
		code, out = call(t, http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`)
		if code != http.StatusOK || out["outcome"] != controlapi.OutcomeApplied {
			t.Fatalf("after adopting, enable-approvals = %d %v, want applied", code, out)
		}
	})
}

// TestMount_aLedgerWithNoMarkIsLegacy is G5 on a real app: a ledger older than
// this version is named legacy_unfounded by the read door and blocks nothing.
//
// PROBING MUTATION: treat legacy as foreign. The door refuses and this reddens.
func TestMount_aLedgerWithNoMarkIsLegacy(t *testing.T) {
	t.Setenv("KORVUN_LEGACY_TEST_ADMIN", "tok")
	cfg := cfgWith(ollamaBrain())
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_LEGACY_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	cfg.Storage = &config.StorageConfig{Path: filepath.Join(t.TempDir(), "korvun.db")}
	if s := preparedStore(t, cfg, cfg.Storage.Path); s != nil {
		_ = s.Close() // a ledger with no receipt at all: no mark
	}
	a, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(filepath.Join(t.TempDir(), "korvun.json")))
	if err != nil {
		t.Fatalf("Build: %v", err)
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
	url := "http://" + a.adminServer.Addr()
	resp := askAdmin(t, http.MethodGet, url+"/api/whats-happening", "tok")
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	l, _ := out["ledger"].(map[string]any)
	if l["standing"] != "legacy_unfounded" {
		t.Fatalf("a ledger with no mark reads %v, want legacy_unfounded", l)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url+"/api/whats-happening/enable-approvals", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Authorization", "Bearer tok")
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("press: %v", err)
	}
	defer func() { _ = r2.Body.Close() }()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("a legacy ledger blocked a change: %d", r2.StatusCode)
	}
}

// TestCreateLedger_theFoundingCloseMarksTheFounder is D5: the ledger founded
// through the registry, once its act closes applied, knows its founder — ok
// under the founder's identity, foreign under another's.
//
// PROBING MUTATION: close the founding act with a plain Finish. The ledger
// reads legacy under both and this reddens.
func TestCreateLedger_theFoundingCloseMarksTheFounder(t *testing.T) {
	sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	founder := filepath.Join(t.TempDir(), "korvun.json")
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = ProfileIdentity(founder)
	act, path, err := rec.CreateLedger(context.Background(), "enable-storage")
	if err != nil {
		t.Fatalf("CreateLedger: %v", err)
	}
	if got := rec.SettleAct(context.Background(), act.ActionID, true, "succeeded"); got.ReceiptID == "" {
		t.Fatal("the founding act closed with no receipt")
	}
	judge := func(identity string) actionsqlite.LedgerStanding {
		s, err := actionsqlite.OpenOperatorFor(path, identity)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = s.Close() }()
		st, _, err := s.Standing(context.Background())
		if err != nil {
			t.Fatalf("standing: %v", err)
		}
		return st
	}
	if st := judge(ProfileIdentity(founder)); st != actionsqlite.LedgerStandingOK {
		t.Fatalf("under the founder the ledger reads %q, want ok", st)
	}
	if st := judge(ProfileIdentity(filepath.Join(t.TempDir(), "other.json"))); st != actionsqlite.LedgerStandingForeignProfile {
		t.Fatalf("under another profile the ledger reads %q, want foreign", st)
	}
}

// TestMount_anUnreadableIdentityRefusesAdoption is R01/R02 of the redesign at
// the app: a ledger whose identity row vanished while a marked receipt exists
// is named unreadable by the read door, and the adoption door refuses it by
// name and writes nothing, in BOTH orders — the row deleted before the app
// ever looked, and deleted AFTER the app looked (the screen's own sequence:
// read, then press), where the read door, which has no cache in the redesign,
// already says unreadable and the adoption reads the row inside its own
// transaction. Evidence: a REAL app with its admin server on loopback (same
// process), the ledger founded by one profile, the row deleted through a
// second real connection, the app built for another profile.
//
// PROBING MUTATIONS: let the adoption skip the row, or read it from a value
// the handle kept. The POST answers 200 adopted; the order in question
// reddens.
func TestMount_anUnreadableIdentityRefusesAdoption(t *testing.T) {
	for _, order := range []string{"deleted before the app looked", "deleted after the app looked"} {
		t.Run(order, func(t *testing.T) {
			mountMalformedMark(t, order == "deleted after the app looked")
		})
	}
}

func mountMalformedMark(t *testing.T, afterTheLook bool) {
	t.Helper()
	t.Setenv("KORVUN_MALFORMED_TEST_ADMIN", "tok")
	cfg := cfgWith(ollamaBrain())
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_MALFORMED_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	founder := filepath.Join(t.TempDir(), "founder", "korvun.json")
	ledger := foundedLedger(t, cfg, ProfileIdentity(founder))
	cfg.Storage = &config.StorageConfig{Path: ledger}
	tamper := func(t *testing.T) {
		t.Helper()
		raw, err := sql.Open("sqlite", "file:"+ledger+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatalf("open raw: %v", err)
		}
		defer func() { _ = raw.Close() }()
		res, err := raw.Exec(`DELETE FROM ledger_identity`)
		if err != nil {
			t.Fatalf("tamper: %v", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			t.Fatalf("the tamper deleted %d identity rows, want 1", n)
		}
	}
	if !afterTheLook {
		tamper(t)
	}
	thisProfile := filepath.Join(t.TempDir(), "other", "korvun.json")
	reloader := &profileReloader{cfg: cfg}
	a, err := Build(cfg,
		withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(reloader), WithProfilePath(thisProfile),
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
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
	url := "http://" + a.adminServer.Addr()
	call := func(t *testing.T, method, path, body string) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), method, url+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	standing := func(t *testing.T) (string, string) {
		t.Helper()
		code, out := call(t, http.MethodGet, "/api/whats-happening", "")
		l, _ := out["ledger"].(map[string]any)
		if code != http.StatusOK {
			t.Fatalf("GET /api/whats-happening = %d %v", code, out)
		}
		s, _ := l["standing"].(string)
		o, _ := l["owner"].(string)
		return s, o
	}
	if afterTheLook {
		// The screen reads first: the row with «Adoptar libro», the founder
		// as owner — and the handle caches that.
		if s, o := standing(t); s != "ledger_foreign_profile" || o != ProfileIdentity(founder) {
			t.Fatalf("before the rewrite: standing %q owner %q, want foreign with the founder", s, o)
		}
		tamper(t)
		// No cache in the redesign: the read door says unreadable at once.
		if s, o := standing(t); s != controlapi.LedgerStandingUnreadable || !strings.Contains(o, "ledger_unreadable") {
			t.Fatalf("after the deletion the read door says %q %q, want unreadable naming ledger_unreadable", s, o)
		}
	} else if s, o := standing(t); s != controlapi.LedgerStandingUnreadable || !strings.Contains(o, "ledger_unreadable") {
		t.Fatalf("before the adoption: standing %q cause %q, want unreadable naming ledger_unreadable", s, o)
	}
	// … and the adoption refuses by name in either order.
	code, out := call(t, http.MethodPost, "/api/whats-happening/adopt-ledger", `{"confirm":true}`)
	detail, _ := out["detail"].(string)
	if code != http.StatusServiceUnavailable || out["outcome"] != controlapi.OutcomeActNotRecorded || !strings.Contains(detail, "ledger_unreadable") {
		t.Fatalf("POST adopt-ledger over an unreadable identity = %d %v, want 503 act_not_recorded naming ledger_unreadable", code, out)
	}
	if s, _ := standing(t); s == "ok" || s == "legacy_unfounded" {
		t.Fatalf("after the refused adoption the standing is %q: the corruption was buried", s)
	}
	if reloader.calls != 0 {
		t.Fatalf("the profile was reloaded %d times by a refused adoption", reloader.calls)
	}
	raw, err := sql.Open("sqlite", "file:"+ledger+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = raw.Close() }()
	var acts, receipts int
	if err := raw.QueryRow(`SELECT (SELECT COUNT(*) FROM actions), (SELECT COUNT(*) FROM receipts)`).Scan(&acts, &receipts); err != nil {
		t.Fatalf("count: %v", err)
	}
	if acts != 1 || receipts != 1 {
		t.Fatalf("actions %d receipts %d after the refused adoption, want the founding one only", acts, receipts)
	}
}

// TestMount_theApprovalDoorsNameAForeignLedger is R13 at the app: a REAL app
// for another profile over a founded ledger answers 409 ledger_foreign_profile
// on the approval doors, writes nothing and consumes no decision.
//
// PROBING MUTATION: let the adapter pass the store's error through unnamed →
// 500 and reddens.
func TestMount_theApprovalDoorsNameAForeignLedger(t *testing.T) {
	t.Setenv("KORVUN_APPROVALS_FOREIGN_TEST_ADMIN", "tok")
	brain := ollamaBrain()
	brain.Name = "a" // the brain parkOne parks for (principal_brain_a)
	brain.Agent = &config.AgentConfig{
		Tools:       []string{"webhook_call"},
		Governance:  []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "allow"}},
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:8765"}},
	}
	cfg := cfgWith(brain)
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_APPROVALS_FOREIGN_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
	founder := filepath.Join(t.TempDir(), "founder", "korvun.json")
	ledger := foundedLedger(t, cfg, ProfileIdentity(founder))
	cfg.Storage = &config.StorageConfig{Path: ledger}
	// A request the FOUNDER parked: the decision the other profile attempts
	// reaches the store's write, where the guard refuses it by name.
	owner, err := actionsqlite.OpenOperatorFor(ledger, ProfileIdentity(founder))
	if err != nil {
		t.Fatalf("open the founder's handle: %v", err)
	}
	parked := parkOne(t, cfg, owner, "act_parked_by_the_founder")
	_ = owner.Close()
	thisProfile := filepath.Join(t.TempDir(), "other", "korvun.json")
	a, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))), WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(thisProfile))
	if err != nil {
		t.Fatalf("Build: %v", err)
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
	for _, door := range []string{"approve", "reject"} {
		body := fmt.Sprintf(`{"digest":%q}`, parked.ActionDigest)
		if door == "reject" {
			body = `{"comment":"no"}`
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+a.adminServer.Addr()+"/api/approvals/"+parked.ApprovalID+"/"+door, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusConflict || out["error"] != "ledger_foreign_profile" {
			t.Fatalf("%s over a foreign ledger = %d %v, want 409 ledger_foreign_profile", door, resp.StatusCode, out)
		}
	}
}
