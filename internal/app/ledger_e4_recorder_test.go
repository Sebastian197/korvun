// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// Train E, batch 4 (GE7) — the standing's projection, from the store's error
// to the screen's wire: TE48, TE55, TE56 and the Go half of TE49 (plan v3,
// §6, §7 and §13.2). unreadable is the answer of the two verdict sentinels
// alone; environment of ErrLedgerEnvironment; every other failure is
// unavailable. The cause travels exact; so does the path of the ledger file
// the app actually opened.

// e4Failing is a real ledger whose Standing fails with err.
type e4Failing struct {
	actLedger
	err error
}

func (l e4Failing) Standing(context.Context) (actionsqlite.LedgerStanding, string, error) {
	return "", "", l.err
}

// e4NativeLike is a coded error with the driver's message shape.
type e4NativeLike struct {
	code int
	msg  string
}

func (e *e4NativeLike) Error() string { return e.msg }
func (e *e4NativeLike) Code() int     { return e.code }

// e4Class is one failure of a Standing read and the standing it owes.
type e4Class struct {
	name     string
	err      error
	standing string
}

// e4Classes are the failures of TE48 and TE55: the verdict sentinels, the
// environment (also over an extended code), BUSY and LOCKED, the literal old
// «database is locked», a closed pool, a context's end, and every §5
// uncategorised code, bare.
func e4Classes(t *testing.T) []e4Class {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	closed := db.Ping()
	out := []e4Class{
		{"unreadable", fmt.Errorf("%w: table intents missing at schema v16", actionsqlite.ErrLedgerUnreadable), "unreadable"},
		{"mark malformed", fmt.Errorf("%w: the mark is not canonical", actionsqlite.ErrLedgerMarkMalformed), "unreadable"},
		{"environment", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerEnvironment, &e4NativeLike{code: 13, msg: "database or disk is full (13) (SQLITE_FULL)"}), "environment"},
		{"environment 522", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerEnvironment, &e4NativeLike{code: 522, msg: "disk I/O error (522)"}), "environment"},
		{"busy", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerBusy, &e4NativeLike{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"}), "unavailable"},
		{"locked", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerBusy, &e4NativeLike{code: 6, msg: "database table is locked (6)"}), "unavailable"},
		{"database is locked, uncoded", errors.New("database is locked"), "unavailable"},
		{"closed pool", closed, "unavailable"},
		{"canceled", context.Canceled, "unavailable"},
		{"deadline", context.DeadlineExceeded, "unavailable"},
	}
	for _, code := range []int{2, 4, 7, 9, 12, 15, 16, 17, 18, 19, 21, 25, 27, 28, 0, 101} {
		out = append(out, e4Class{fmt.Sprintf("code %d", code), &e4NativeLike{code: code, msg: fmt.Sprintf("uncategorised failure (%d)", code)}, "unavailable"})
	}
	return out
}

// e4Recorder is the app's real recorder over a real prepared store whose
// Standing fails with err, serving a profile.
func e4Recorder(t *testing.T, err error) (*configActRecorder, string) {
	t.Helper()
	cfg := cfgWith(ollamaBrain())
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	_, resolver, issuers, ierr := phase1IdentityRuntime(cfg)
	if ierr != nil {
		t.Fatalf("identity runtime: %v", ierr)
	}
	rec := newConfigActRecorderOver(e4Failing{actLedger: store, err: err}, resolver, issuers["console"], func(error) {}, NewConfigActRegistry(func(error) {}), path)
	rec.profile = ProfileIdentity(filepath.Join(t.TempDir(), "korvun.json"))
	return rec, path
}

// TE48 (the recorder) · the recorder's projection of a failed Standing: the
// two verdict sentinels are unreadable, ErrLedgerEnvironment is environment,
// every other failure is unavailable — never by the error's text — and the
// cause is the error itself, exact.
//
// PROBING MUTATIONS (MU48): the catch-all unreadable restored → reddens; a
// classification by the «locked» text → reddens; the verdict check removed →
// reddens; the cause swallowed → reddens.
//
// Evidence level: unit, the real recorder over a real store whose one
// Standing read fails as told.
func TestE4_TE48_theRecorderProjectsEachClass(t *testing.T) {
	for _, c := range e4Classes(t) {
		t.Run(c.name, func(t *testing.T) {
			rec, _ := e4Recorder(t, c.err)
			standing, cause := rec.LedgerStanding(context.Background())
			if standing != c.standing || cause != c.err.Error() {
				t.Fatalf("LedgerStanding = %q / %q, want %q with the cause %q", standing, cause, c.standing, c.err.Error())
			}
		})
	}
}

// TE48 (the real app) · the same projection through a running app's GET
// /api/whats-happening, 200, for a verdict, the environment, BUSY and the
// literal uncoded «database is locked».
//
// Evidence level: real app on loopback with a controlled read failure (the
// recorder's store wrapped through withActLedger).
func TestE4_TE48_theRunningAppServesEachClass(t *testing.T) {
	for _, c := range e4Classes(t)[:7] {
		if c.name == "mark malformed" || c.name == "environment 522" || c.name == "locked" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			cfg := e1Cfg(t)
			_, founder := e1FoundedFor(t, cfg)
			err := c.err
			r := e4Start(t, cfg, founder, withActLedger(func(l actLedger) actLedger { return e4Failing{actLedger: l, err: err} }))
			code, l, raw := r.ledger(t)
			if code != http.StatusOK || l["standing"] != c.standing || l["owner"] != c.err.Error() {
				t.Fatalf("GET = %d %s, want 200 %s with the cause %q", code, raw, c.standing, c.err.Error())
			}
		})
	}
}

// e4ServeRecorder mounts the real what's-happening doors over rec and returns
// the ledger object the read door serves, raw.
func e4ServeRecorder(t *testing.T, rec controlapi.ActRecorder) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	controlapi.RegisterWhatsHappening(mux, "tok", &profileReloader{cfg: cfgWith(ollamaBrain())}, rec)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/whats-happening", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Ledger map[string]any `json:"ledger"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out.Ledger
}

// TE55 · every failure of TE48's list, consumed by the real recorder and
// serialized by the real read door: the ledger object is exactly the owed
// standing, the exact cause and the ledger's path — nothing more.
//
// PROBING MUTATIONS (MU55): catch-all unreadable → reddens; the malformed
// sentinel left out of the verdicts → reddens.
//
// Evidence level: in process, the real recorder and the real HTTP handler.
func TestE4_TE55_theReadDoorSerializesEachClassExactly(t *testing.T) {
	for _, c := range e4Classes(t) {
		t.Run(c.name, func(t *testing.T) {
			rec, path := e4Recorder(t, c.err)
			code, l := e4ServeRecorder(t, rec)
			abs, err := filepath.Abs(path)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"standing": c.standing, "owner": c.err.Error(), "path": filepath.Clean(abs)}
			got, _ := json.Marshal(l)
			wanted, _ := json.Marshal(want)
			if code != http.StatusOK || !bytes.Equal(got, wanted) {
				t.Fatalf("GET = %d %s, want %s", code, got, wanted)
			}
		})
	}
}

// e4NoPathRecorder is a recorder that does not provide the path: an older
// double, as the controlapi fakes are.
type e4NoPathRecorder struct{ controlapi.ActRecorder }

func (e4NoPathRecorder) LedgerStanding(context.Context) (string, string) {
	return "unreadable", "a cause"
}

// TE56 · the path the screen shows is the path of the file the app actually
// opened: with storage:{} under an isolated user config dir, with an explicit
// absolute path and with an explicit relative one, the read door's ledger.path
// is exactly the running app's store's own path — never the raw configured
// string, never a guessed default. A recorder that does not provide a path
// gets none in the answer, and no placeholder.
//
// PROBING MUTATIONS (MU56): the path not serialized → reddens; the raw
// configured path served → reddens.
//
// Evidence level: real app on loopback over real temporary files; the absent
// provider through the real read door in process.
func TestE4_TE56_thePathIsTheFileTheAppOpened(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(t *testing.T, cfg *config.Config)
	}{
		{"storage:{} under an isolated user config dir", func(t *testing.T, cfg *config.Config) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			cfg.Storage = &config.StorageConfig{}
		}},
		{"an explicit absolute path", func(t *testing.T, cfg *config.Config) {
			cfg.Storage = &config.StorageConfig{Path: filepath.Join(t.TempDir(), "abs", "korvun.db")}
		}},
		{"an explicit relative path", func(t *testing.T, cfg *config.Config) {
			t.Chdir(t.TempDir())
			cfg.Storage = &config.StorageConfig{Path: filepath.Join("rel", "korvun.db")}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := e1Cfg(t)
			c.set(t, cfg)
			r := e4Start(t, cfg, filepath.Join(t.TempDir(), "p", "korvun.json"))
			store, ok := r.app.actions.(*actionsqlite.Store)
			if !ok {
				t.Fatalf("the running app holds no action store (%T)", r.app.actions)
			}
			code, l, raw := r.ledger(t)
			if code != http.StatusOK || l["path"] != store.Path() || !filepath.IsAbs(store.Path()) {
				t.Fatalf("GET = %d %s, want ledger.path %q, the file the app opened", code, raw, store.Path())
			}
		})
	}
	t.Run("a recorder that provides no path", func(t *testing.T) {
		code, l := e4ServeRecorder(t, e4NoPathRecorder{})
		if _, has := l["path"]; code != http.StatusOK || has || l["standing"] != "unreadable" {
			t.Fatalf("GET = %d %v, want 200 with no path at all", code, l)
		}
	})
}

// e4GoldenDir is where the screen's moulds read the real GET's bodies.
var e4GoldenDir = filepath.Join("..", "..", "cmd", "korvun-desktop", "frontend", "src", "views", "fixtures")

// e4Golden normalizes a GET body — the ledger's directory replaced by
// /perfil — indents it with sorted keys, and compares it with its reference
// file; KORVUN_E4_UPDATE_GOLDEN=1 writes the file instead.
func e4Golden(t *testing.T, name, body, dir string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(body, dir, "/perfil")), &v); err != nil {
		t.Fatalf("the GET body: %v", err)
	}
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	file := filepath.Join(e4GoldenDir, "whats-happening-"+name+".json")
	if os.Getenv("KORVUN_E4_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(file, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file) //nolint:gosec // G304: the repository's own fixture
	if err != nil {
		t.Fatalf("the reference body: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the real GET body for %s differs from %s\n--- real\n%s\n--- reference\n%s", name, file, got, want)
	}
}

// TE49 (the Go half) · the three bodies the screen's moulds render are the
// real app's GET /api/whats-happening, byte for byte after the ledger's
// directory is replaced by /perfil: unreadable over a renamed version column,
// environment and unavailable over a controlled read failure.
//
// Evidence level: real app on loopback; the screen's half renders these files
// in jsdom (WhatsHappening.e4.test.tsx).
func TestE4_TE49_theRealGETIsWhatTheScreenRenders(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
	}{
		{"unreadable", nil},
		{"environment", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerEnvironment, &e4NativeLike{code: 13, msg: "database or disk is full (13) (SQLITE_FULL)"})},
		{"unavailable", fmt.Errorf("%w: %w", actionsqlite.ErrLedgerBusy, &e4NativeLike{code: 5, msg: "database is locked (5) (SQLITE_BUSY)"})},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := e1Cfg(t)
			path, founder := e1FoundedFor(t, cfg)
			var opts []Option
			if c.err == nil {
				e1Exec(t, e1Raw(t, path), `ALTER TABLE action_schema RENAME COLUMN version TO v`)
			} else {
				err := c.err
				opts = append(opts, withActLedger(func(l actLedger) actLedger { return e4Failing{actLedger: l, err: err} }))
			}
			r := e4Start(t, cfg, founder, opts...)
			code, _, raw := r.call(t, http.MethodGet, "/api/whats-happening", "")
			if code != http.StatusOK {
				t.Fatalf("GET = %d %s", code, raw)
			}
			e4Golden(t, c.name, raw, filepath.Dir(path))
		})
	}
}
