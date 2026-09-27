// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · «¿Qué pasa hoy?» — THE MOUNT.
//
// This mould exists because the internal adversarial pass found the screen's
// four doors had NO production caller. `RegisterWhatsHappening` compiled, was
// covered by its own handler tests, and was mounted by nothing — while the
// package comment beside it described which rows «have a button in v0.16.2».
// Every green in the train was green over a surface no running Korvun served.
//
// Handler tests cannot see that. They mount the routes themselves, which is
// exactly the assumption under audit. So this one builds a REAL app, lets it
// bind a REAL admin server on loopback, and asks over REAL HTTP.
//
// Evidence level, honest: in-process app, real HTTP over a real listener on
// loopback. Not a packaged binary and not a real supervisor — the reloader is a
// stub, so nothing here proves a cutover.

package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// servingApp boots an app with an admin server on an ephemeral loopback port
// and returns its base URL.
func servingApp(t *testing.T, admin *config.AdminConfig) string {
	t.Helper()
	cfg := cfgWith(ollamaBrain())
	cfg.Admin = admin
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	a, err := Build(cfg,
		withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(stubReloader{}),
		withTestProfile(),
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
	if a.adminServer.Addr() == "" {
		t.Fatal("admin server never bound")
	}
	t.Cleanup(func() {
		cancel()
		sctx, sc := context.WithTimeout(context.Background(), time.Second)
		defer sc()
		_ = a.Shutdown(sctx)
	})
	return "http://" + a.adminServer.Addr()
}

func askAdmin(t *testing.T, method, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// doorsServedBy asks the read door which write doors its rows claim, and returns
// them sorted and deduplicated. A row claiming a door the server does not mount
// is the contract mould's business; here the point is to exercise exactly the
// doors this running app says it has.
func doorsServedBy(t *testing.T, url string) []string {
	t.Helper()
	resp := askAdmin(t, http.MethodGet, url+"/api/whats-happening", "tok")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/whats-happening = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Rows []controlapi.ScreenRow `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range body.Rows {
		if r.Door == "" || seen[r.Door] {
			continue
		}
		seen[r.Door] = true
		out = append(out, r.Door)
	}
	if len(out) == 0 {
		t.Fatal("the running app serves no row that claims a door: this mould would exercise nothing")
	}
	sort.Strings(out)
	return out
}

// whatsDoorPaths are the door paths the NOT-MOUNTED mould probes. It cannot ask
// the read door for them — that door is not mounted either, which is the whole
// point — so they are written here, and `TestMount_theScreenIsServedByARunningKorvun`
// keeps them honest by driving the served list instead.
var whatsDoorPaths = []string{"adopt-ledger", "allow-host", "enable-approvals", "enable-storage", "lift-shadow", "set-ceiling"}

// TestMount_theScreenIsServedByARunningKorvun walks every door the screen needs
// over a real listener: the read door answers its rows, and each write door is
// mounted and gated.
//
// PROBING MUTATION (executed, M16): delete the RegisterWhatsHappening call from
// Build. Every subtest reddens with 404 — which is precisely what the tree
// looked like when the pass caught it, and what nothing in the train noticed.
func TestMount_theScreenIsServedByARunningKorvun(t *testing.T) {
	t.Setenv("KORVUN_WHATS_MOUNT_TEST", "tok")
	url := servingApp(t, &config.AdminConfig{TokenEnv: "KORVUN_WHATS_MOUNT_TEST"})

	// The door names come from what the RUNNING app SERVES, not from a package
	// helper. It is better evidence — the server states its own contract — and it
	// keeps `writeDoorNames` unexported, which is where the official pass said it
	// belongs.
	doors := doorsServedBy(t, url)

	t.Run("the read door serves the contract", func(t *testing.T) {
		resp := askAdmin(t, http.MethodGet, url+"/api/whats-happening", "tok")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/whats-happening = %d, want 200", resp.StatusCode)
		}
		var body struct {
			Rows       []controlapi.ScreenRow `json:"rows"`
			Conditions map[string]string      `json:"conditions"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Rows) != len(controlapi.ScreenRows()) {
			t.Fatalf("the running app served %d rows, the contract has %d",
				len(body.Rows), len(controlapi.ScreenRows()))
		}
		// The join travels WITH the rows, so the window never holds a second
		// copy of it. Every condition the gate can report must arrive with the
		// rule that explains it, and that rule must be a row the window has.
		byRule := map[string]bool{}
		for _, r := range body.Rows {
			byRule[r.Rule] = true
		}
		for _, c := range []controlapi.ParkCondition{
			controlapi.ParkNeedsStore, controlapi.ParkNeedsAgent,
			controlapi.ParkNeedsCeiling, controlapi.ParkNeedsParkableTool,
			controlapi.ParkGovernanceDenies, controlapi.ParkToolShadowed,
		} {
			rule, ok := body.Conditions[string(c)]
			if !ok {
				t.Errorf("the door served no rule for the park condition %q", c)
				continue
			}
			if !byRule[rule] {
				t.Errorf("the park condition %q joins to the rule %q, which is not among the rows served", c, rule)
			}
		}
	})

	t.Run("the read door is behind the bearer", func(t *testing.T) {
		resp := askAdmin(t, http.MethodGet, url+"/api/whats-happening", "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated GET = %d, want 401", resp.StatusCode)
		}
	})

	// Every door the contract's rows point at must be mounted. Iterating the
	// declared names rather than typing four paths means a fifth door added
	// tomorrow is checked here without anyone remembering to.
	for _, door := range doors {
		t.Run("the "+door+" door is mounted and gated", func(t *testing.T) {
			resp := askAdmin(t, http.MethodPost, url+"/api/whats-happening/"+door, "")
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("unauthenticated POST /api/whats-happening/%s = %d, want 401 (mounted and gated); a 404 means it is not mounted at all",
					door, resp.StatusCode)
			}
		})
	}
}

// TestMount_noAdminTokenMeansNoScreenDoors: the screen's doors are part of the
// mutation surface and inherit ADR-0028's default. With no resolvable token they
// are NOT mounted — the operator gets a read-only Korvun, not a set of buttons
// nothing authenticates.
//
// PROBING MUTATION (executed, M17): mount RegisterWhatsHappening outside the
// token branch. Every door answers 401 instead of 404 and this reddens.
func TestMount_noAdminTokenMeansNoScreenDoors(t *testing.T) {
	url := servingApp(t, nil)
	for _, path := range append([]string{""}, whatsDoorPaths...) {
		full := url + "/api/whats-happening"
		method := http.MethodGet
		if path != "" {
			full += "/" + path
			method = http.MethodPost
		}
		resp := askAdmin(t, method, full, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s with no admin token = %d, want 404 (not mounted)", method, full, resp.StatusCode)
		}
	}
}

// profileReloader is a stub whose CurrentConfig is the served profile, so the
// doors get past «no current config» and reach the ledger check. It COUNTS the
// reloads it is asked for: «the reloader was never asked» is the half of the
// refusal that only a counter can see (the official pass's instrument P3).
type profileReloader struct {
	cfg   *config.Config
	mu    sync.Mutex
	calls int
}

func (r *profileReloader) RequestReload(*config.Config) (supervisor.Handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return "h", nil
}
func (r *profileReloader) Status(supervisor.Handle) supervisor.State {
	return supervisor.StateSucceeded
}
func (r *profileReloader) CurrentConfig() *config.Config { return r.cfg }
func (r *profileReloader) asked() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// TestMount_aProfileWithNoStoreRefusesEveryDoorButOne is F1 on a REAL app with
// no storage block: the four buttons and the builder's door answer `no_ledger`
// through a recorder that exists (the ledgerless one), and `enable-storage` is
// the one door that works — founding the ledger in the sandboxed default path
// and answering the founding act with its receipt.
//
// PROBING MUTATION: mount the doors with a nil recorder on a store-less app (the
// shape before this piece). The four refusals stay green; enable-storage answers
// no_ledger too and the last subtest reddens.
func TestMount_aProfileWithNoStoreRefusesEveryDoorButOne(t *testing.T) {
	sandboxUserDirApp(t)
	t.Setenv("KORVUN_WHATS_NOSTORE_TEST", "tok")
	// An AGENT brain with a shadowed, caged webhook tool: every edit door must
	// get PAST its own edit, or the refusal measured here would be the edit's
	// («that brain is not an agent brain»), not the ledger's.
	brain := ollamaBrain()
	brain.Agent = &config.AgentConfig{
		Tools:       []string{"webhook_call"},
		Governance:  []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}},
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:8765"}},
	}
	cfg := cfgWith(brain)
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_WHATS_NOSTORE_TEST"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	if cfg.Storage != nil {
		t.Fatal("the mould's premise is a profile with NO storage")
	}
	reloader := &profileReloader{cfg: cfg}
	a, err := Build(cfg,
		withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(reloader),
		withTestProfile(),
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

	postJSON := func(t *testing.T, path, body string) (int, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	for _, door := range []struct{ path, body string }{
		{"/api/whats-happening/enable-approvals", `{"confirm":true}`},
		{"/api/whats-happening/set-ceiling", `{"confirm":true,"brain":"default"}`},
		{"/api/whats-happening/lift-shadow", `{"confirm":true,"brain":"default","tool":"webhook_call"}`},
		{"/api/whats-happening/allow-host", `{"confirm":true,"brain":"default","tool":"webhook_call","host":"h"}`},
	} {
		t.Run(door.path+" refuses with no_ledger", func(t *testing.T) {
			code, out := postJSON(t, door.path, door.body)
			if code != http.StatusServiceUnavailable || out["outcome"] != controlapi.OutcomeNoLedger {
				t.Fatalf("%s = %d %v, want 503 no_ledger", door.path, code, out)
			}
		})
	}
	t.Run("the builder's door refuses with no_ledger", func(t *testing.T) {
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		code, out := postJSON(t, "/api/config", string(raw))
		if code != http.StatusServiceUnavailable || out["error_code"] != "no_ledger" {
			t.Fatalf("POST /api/config = %d %v, want 503 no_ledger", code, out)
		}
	})
	t.Run("none of the refusals asked the supervisor", func(t *testing.T) {
		if n := reloader.asked(); n != 0 {
			t.Fatalf("the reloader was asked %d times by doors that refused with no_ledger", n)
		}
	})
	t.Run("enable-storage is the one door open", func(t *testing.T) {
		code, out := postJSON(t, "/api/whats-happening/enable-storage", `{}`)
		if code != http.StatusOK || out["outcome"] != controlapi.OutcomeApplied {
			t.Fatalf("enable-storage = %d %v, want 200 applied", code, out)
		}
		act, _ := out["act"].(map[string]any)
		if act == nil || act["action_id"] == "" || act["receipt_id"] == nil || act["receipt_id"] == "" {
			t.Fatalf("the founding act is not answered with its receipt: %v", out)
		}
		path := storagePath(cfg)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("no ledger founded at %q: %v", path, err)
		}
		row, receipts := readOnlyRow(t, path, act["action_id"].(string))
		if row.State != action.StateSucceeded || len(receipts) != 1 {
			t.Fatalf("the founding act is %q with %d receipts, want SUCCEEDED with one", row.State, len(receipts))
		}
		if n := reloader.asked(); n != 1 {
			t.Fatalf("the reloader was asked %d times, want exactly once, by the one door open", n)
		}
	})
}
