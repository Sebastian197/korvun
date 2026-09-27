// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	convsqlite "github.com/Sebastian197/korvun/internal/conversation/sqlite"
	msqlite "modernc.org/sqlite"
)

// Train E, batch 4 (GE7, GE8, GE9): the harness of the app's moulds — TE12,
// TE36-style boots, TE40, TE41, TE44, TE45, TE48, TE49's Go half, TE55, TE56,
// TE58, TE59, TE61 and TE62 (plan v3, §§6–7).
//
// The files are real ledgers founded by the boot's own profile and damaged by
// a second, raw connection. Writes the app must not make are caught by traps:
// BEFORE INSERT, UPDATE and DELETE triggers that count the attempt through a
// scalar function — outside the transaction that attempts it, so a rollback
// cannot erase the count — and abort it. The catalog is compared whole: type,
// name, target table and DDL of every object, both ways.

var (
	e4TrapOnce sync.Once
	e4TrapHits sync.Map // token → *atomic.Int64
)

// e4RegisterTrap registers e4_trap(token) once per process, before any
// connection a mould's app opens.
func e4RegisterTrap() {
	e4TrapOnce.Do(func() {
		msqlite.MustRegisterScalarFunction("e4_trap", 1, func(_ *msqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			token, _ := args[0].(string)
			n, _ := e4TrapHits.LoadOrStore(token, new(atomic.Int64))
			n.(*atomic.Int64).Add(1)
			return nil, nil
		})
	})
}

// e4Traps are the traps of one mould on one file.
type e4Traps struct{ token string }

func (tr e4Traps) hits() int64 {
	if n, ok := e4TrapHits.Load(tr.token); ok {
		return n.(*atomic.Int64).Load()
	}
	return 0
}

// e4ArmTraps puts a counting, aborting trap on every INSERT, UPDATE and
// DELETE of each table of path that exists.
func e4ArmTraps(t *testing.T, path string, tables ...string) e4Traps {
	t.Helper()
	e4RegisterTrap()
	token := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	raw := e1Raw(t, path)
	for _, table := range tables {
		var n int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			continue
		}
		for _, ev := range []string{"INSERT", "UPDATE", "DELETE"} {
			e1Exec(t, raw, fmt.Sprintf(`CREATE TRIGGER e4_trap_%s_%s BEFORE %s ON %s BEGIN SELECT e4_trap('%s'); SELECT RAISE(ABORT, 'e4 trap: %s on %s'); END`,
				table, strings.ToLower(ev), ev, table, strings.ReplaceAll(token, "'", "''"), ev, table))
		}
	}
	return e4Traps{token: token}
}

// e4Catalog is every object of path: "type:name" → target table NUL DDL.
func e4Catalog(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	rows, err := raw.Query(`SELECT type, name, tbl_name, ifnull(sql, '') FROM sqlite_master`)
	if err != nil {
		t.Fatalf("catalog of %s: %v", path, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var kind, name, tbl, ddl string
		if err := rows.Scan(&kind, &name, &tbl, &ddl); err != nil {
			t.Fatal(err)
		}
		out[kind+":"+name] = tbl + "\x00" + ddl
	}
	return out
}

// e4CatalogDiff names every difference between two catalogs, sorted.
func e4CatalogDiff(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if b, ok := before[k]; !ok {
			out = append(out, "appeared "+k)
		} else if b != v {
			out = append(out, "changed "+k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			out = append(out, "disappeared "+k)
		}
	}
	sort.Strings(out)
	return out
}

// e4ConversationCatalog is the catalog a first open of the conversation store
// writes on an empty file: the exact allowance of a boot over a ledger that
// never held the conversations.
func e4ConversationCatalog(t *testing.T) map[string]string {
	t.Helper()
	path := t.TempDir() + "/conversations.db"
	conv, err := convsqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := conv.Close(); err != nil {
		t.Fatal(err)
	}
	return e4Catalog(t, path)
}

// e4PreinitConversations opens and closes the conversation store over path,
// so its tables are there before the mould snapshots the catalog.
func e4PreinitConversations(t *testing.T, path string) {
	t.Helper()
	conv, err := convsqlite.Open(path)
	if err != nil {
		t.Fatalf("open the conversation store over the ledger: %v", err)
	}
	if err := conv.Close(); err != nil {
		t.Fatal(err)
	}
}

// e4Record is one log record: its message and its attributes, rendered.
type e4Record struct {
	msg   string
	attrs map[string]string
}

// e4Sink is an slog.Handler that keeps every record.
type e4Sink struct {
	mu      *sync.Mutex
	records *[]e4Record
	attrs   []slog.Attr
}

func newE4Sink() *e4Sink {
	return &e4Sink{mu: &sync.Mutex{}, records: &[]e4Record{}}
}

func (s *e4Sink) Enabled(context.Context, slog.Level) bool { return true }

func (s *e4Sink) Handle(_ context.Context, r slog.Record) error {
	rec := e4Record{msg: r.Message, attrs: map[string]string{}}
	for _, a := range s.attrs {
		rec.attrs[a.Key] = fmt.Sprint(a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = fmt.Sprint(a.Value.Any())
		return true
	})
	s.mu.Lock()
	*s.records = append(*s.records, rec)
	s.mu.Unlock()
	return nil
}

func (s *e4Sink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &e4Sink{mu: s.mu, records: s.records, attrs: append(append([]slog.Attr(nil), s.attrs...), attrs...)}
}

func (s *e4Sink) WithGroup(string) slog.Handler { return s }

// find is every record whose message is msg.
func (s *e4Sink) find(msg string) []e4Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []e4Record
	for _, r := range *s.records {
		if r.msg == msg {
			out = append(out, r)
		}
	}
	return out
}

// e4Running is an app built and running on loopback, and how to call it.
type e4Running struct {
	app      *App
	url      string
	reloader *profileReloader
	stopOnce sync.Once
	cancel   context.CancelFunc
}

// stop shuts the app down, once; the test's cleanup calls it too.
func (r *e4Running) stop() {
	r.stopOnce.Do(func() {
		r.cancel()
		sctx, sc := context.WithTimeout(context.Background(), 2*time.Second)
		defer sc()
		_ = r.app.Shutdown(sctx)
	})
}

// e4Start builds cfg for profile with extra options, runs it and waits for its
// admin listener; Build must succeed.
func e4Start(t *testing.T, cfg *config.Config, profile string, opts ...Option) *e4Running {
	t.Helper()
	reloader := &profileReloader{cfg: cfg}
	all := append([]Option{withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(reloader), WithProfilePath(profile)}, opts...)
	a, err := Build(cfg, all...)
	if err != nil {
		t.Fatalf("Build = %v, want a live boot", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for a.adminServer.Addr() == "" && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	r := &e4Running{app: a, url: "http://" + a.adminServer.Addr(), reloader: reloader, cancel: cancel}
	t.Cleanup(r.stop)
	return r
}

// call sends one authenticated request and decodes the JSON answer.
func (r *e4Running) call(t *testing.T, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, r.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var raw json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, string(raw)
}

// ledger is the GET's ledger object.
func (r *e4Running) ledger(t *testing.T) (int, map[string]any, string) {
	t.Helper()
	code, out, raw := r.call(t, http.MethodGet, "/api/whats-happening", "")
	l, _ := out["ledger"].(map[string]any)
	return code, l, raw
}

// e4Stored reads one string cell, raw.
func e4Stored(t *testing.T, path, query string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := e1Raw(t, path).QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v.String
}
