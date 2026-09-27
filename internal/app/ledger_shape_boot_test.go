// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// ledgerCatalog is every object of the file but SQLite's own, with its DDL.
func ledgerCatalog(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT type || ':' || name, COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	return out
}

// The REAL boot over a ledger of a bad shape — every table of the schema
// dropped in turn, the internal pass of train D having found three (the
// activation roots, the approvals, the ink registry) that killed the boot
// by driver text: the app boots, names the standing and the missing table
// to the screen, refuses a change door by name, and repairs nothing.
//
// Evidence level: real app on loopback, real file.
//
// PROBING MUTATION: the ink is registered in the ledger whatever the
// standing → the boot over a ledger without signing_keys dies → reddens.
func TestBuild_bootsOverABadShapeAndNamesIt(t *testing.T) {
	t.Setenv("KORVUN_SHAPE_TEST_ADMIN", "tok")
	for _, table := range actionsqlite.SchemaTablesForTest() {
		t.Run(table, func(t *testing.T) {
			brain := ollamaBrain()
			cfg := cfgWith(brain)
			cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_SHAPE_TEST_ADMIN"}
			cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
			founder := filepath.Join(t.TempDir(), "founder", "korvun.json")
			path := foundedLedger(t, cfg, ProfileIdentity(founder))
			cfg.Storage = &config.StorageConfig{Path: path}
			raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = raw.Close() }()
			if _, err := raw.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				t.Fatal(err)
			}
			if _, err := raw.Exec(`DROP TABLE ` + table); err != nil {
				t.Fatalf("drop %s: %v", table, err)
			}
			before := ledgerCatalog(t, raw)
			a, err := Build(cfg,
				withChannelFactory(okFactory(newFakeChannel("telegram"))),
				WithReloader(&profileReloader{cfg: cfg}), WithProfilePath(founder),
			)
			if err != nil {
				t.Fatalf("Build over a ledger without %s: %v, want a boot that names it", table, err)
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
			call := func(method, p, body string) (int, map[string]any) {
				req, err := http.NewRequestWithContext(context.Background(), method, url+p, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer tok")
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("%s %s: %v", method, p, err)
				}
				defer func() { _ = resp.Body.Close() }()
				var out map[string]any
				_ = json.NewDecoder(resp.Body).Decode(&out)
				return resp.StatusCode, out
			}
			code, out := call(http.MethodGet, "/api/whats-happening", "")
			l, _ := out["ledger"].(map[string]any)
			if code != http.StatusOK || l["standing"] != "unreadable" || !strings.Contains(fmt.Sprint(l["owner"]), table) {
				t.Fatalf("GET /api/whats-happening over a ledger without %s = %d ledger %v, want unreadable naming the table", table, code, l)
			}
			code, out = call(http.MethodPost, "/api/whats-happening/enable-approvals", `{"confirm":true}`)
			if code == http.StatusOK || code == http.StatusAccepted || !strings.Contains(fmt.Sprint(out), "unreadable") {
				t.Fatalf("a change door over a ledger without %s = %d %v, want a refusal naming unreadable", table, code, out)
			}
			after := ledgerCatalog(t, raw)
			for k := range after {
				if _, ok := before[k]; !ok && !strings.HasPrefix(k, "table:sessions") && !strings.HasPrefix(k, "table:turns") && !strings.HasPrefix(k, "table:notes") && !strings.HasPrefix(k, "index:") {
					t.Fatalf("the boot over a ledger without %s created %s", table, k)
				}
			}
			if !reflect.DeepEqual(before, after) {
				for k, v := range before {
					if after[k] != v {
						t.Fatalf("the boot over a ledger without %s changed %s", table, k)
					}
				}
			}
		})
	}
}
