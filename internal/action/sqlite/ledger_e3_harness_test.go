// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 3 (GE6): the harness of the moulds TE33–TE39 and TE53 (plan
// v3, §4 «Migration», §§6–7).
//
// The old ledgers are real: the v1 bootstrap, its version row and every
// production step up to the version wanted, each applied by migrateStep; or a
// founded current ledger taken back to v15 (its identity table dropped, the
// «downgraded fixture» of the plan); or buildV10File's hand-built v10. A
// migration is watched through migrationObserverSeam: the points each step
// names (after the outer version read; after its begin, DDL, copy, tail and
// bump; right before and right after its commit; and, for a step it skips as
// stale, the skip) and the work it does (step scripts attempted, version
// bumps attempted, stale steps skipped). A crash,
// a barrier or a held write lock is a CHILD process — this test binary
// re-executed — speaking batch 2's protocol on its stdout and stdin; nothing
// synchronises by sleeping. The evidence a report quotes is logged with the
// prefix E3EVIDENCE.

package sqlite

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// e3Evidence logs one line of evidence for the report.
func e3Evidence(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("E3EVIDENCE "+format, args...)
}

// e3OldFile is a real ledger at schema v`to`, closed: the v1 bootstrap, its
// version row and every production step below `to`, each applied by
// migrateStep.
func e3OldFile(t *testing.T, to int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), fmt.Sprintf("v%d.db", to))
	db, err := sql.Open("sqlite", buildFileDSN(filepath.ToSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(createStmt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO action_schema (version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	for v := 1; v < to; v++ {
		if err := migrateStep(db, migrations[v], migrationCopies[v], v); err != nil {
			t.Fatalf("lift the fixture to v%d: %v", v+1, err)
		}
	}
	return path
}

// e3V15Founded is a ledger founded by profileA — its founding receipt carries
// the mark — taken back to schema v15: the identity table dropped, the
// version row set to 15. The 15→16 step seeds the identity row from the mark.
func e3V15Founded(t *testing.T) string {
	t.Helper()
	store, _ := foundedFor(t, profileA)
	raw := rawConn(t, store)
	rawExec(t, raw, `DROP TABLE ledger_identity`)
	rawExec(t, raw, `UPDATE action_schema SET version = 15`)
	template := filepath.Join(t.TempDir(), "v15-founded.db")
	coldCopy(t, store.path, template)
	return template
}

// e3Copy is a fresh copy of a template, in its own directory.
func e3Copy(t *testing.T, template string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	coldCopy(t, template, path)
	return path
}

// e3Versions reads every stored version of path, raw.
func e3Versions(t *testing.T, path string) []string {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	rows, err := raw.Query(`SELECT CAST(version AS TEXT) FROM action_schema`)
	if err != nil {
		t.Fatalf("read the versions of %s: %v", path, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// e3WantVersion fails unless path stores exactly one version row, want.
func e3WantVersion(t *testing.T, label, path string, want int) {
	t.Helper()
	if got := e3Versions(t, path); len(got) != 1 || got[0] != fmt.Sprint(want) {
		t.Fatalf("%s: action_schema holds %v, want the one row %d", label, got, want)
	}
}

// e3WatchMigration arms the migration observer on path; at may be nil.
func e3WatchMigration(t *testing.T, path string, at func(point string)) *migrationObserver {
	t.Helper()
	o := &migrationObserver{path: e2Abs(t, path), at: at}
	if !migrationObserverSeam.CompareAndSwap(nil, o) {
		t.Fatal("another migration observer is armed: the moulds that watch a migration are sequential")
	}
	t.Cleanup(func() { migrationObserverSeam.CompareAndSwap(o, nil) })
	return o
}

// e3Work is what a migration observer saw.
type e3Work struct {
	ddl, bumps, skipped int
	points              []string
}

func (o *migrationObserver) work() e3Work {
	o.mu.Lock()
	defer o.mu.Unlock()
	return e3Work{ddl: o.ddl, bumps: o.bumps, skipped: o.skipped, points: append([]string(nil), o.points...)}
}

// e3Gate holds the FIRST migration of its file that reaches point until
// opened; every later one passes (an in-process barrier).
type e3Gate struct {
	point   string
	reached chan struct{}
	release chan struct{}
	mu      sync.Mutex
	used    bool
	once    sync.Once
}

func newE3Gate(point string) *e3Gate {
	return &e3Gate{point: point, reached: make(chan struct{}), release: make(chan struct{})}
}

func (g *e3Gate) at(point string) {
	if point != g.point {
		return
	}
	g.mu.Lock()
	first := !g.used
	g.used = true
	g.mu.Unlock()
	if !first {
		return
	}
	close(g.reached)
	<-g.release
}

// open lets the held migration go; later calls do nothing.
func (g *e3Gate) open() { g.once.Do(func() { close(g.release) }) }

// e3Open is the outcome of one OpenFor.
type e3Open struct {
	h   *Store
	err error
}

// e3OpenAsync starts OpenFor(path, profileA) on its own goroutine. Its
// cleanup opens the gate — a mould that failed while the open was held must
// not leave it held — waits for the outcome and closes the handle.
func e3OpenAsync(t *testing.T, path string, g *e3Gate) <-chan e3Open {
	t.Helper()
	out := make(chan e3Open, 1)
	done := make(chan e3Open, 1)
	go func() {
		h, err := OpenFor(path, profileA)
		out <- e3Open{h: h, err: err}
		done <- e3Open{h: h, err: err}
	}()
	t.Cleanup(func() {
		g.open()
		select {
		case r := <-done:
			if r.h != nil {
				_ = r.h.Close()
			}
		case <-time.After(60 * time.Second):
		}
	})
	return out
}

// e3Without is st without the rows of table (a table whose rows carry the
// time they were written).
func e3Without(st e2State, table string) e2State {
	out := e2State{objects: st.objects, rows: map[string]string{}, counts: map[string]int{}}
	for k, v := range st.rows {
		if k != table {
			out.rows[k], out.counts[k] = v, st.counts[k]
		}
	}
	return out
}

// e3SameObjects fails when the catalogs of two states differ.
func e3SameObjects(t *testing.T, label string, want, got e2State) {
	t.Helper()
	if d := e2Diff(e2State{objects: want.objects}, e2State{objects: got.objects}); d != "" {
		t.Fatalf("%s: the catalog differs: %s", label, d)
	}
}

// e3Describe names, for the report, what st holds against a reference: the
// objects that appeared or disappeared and the tables whose rows changed.
func e3Describe(ref, st e2State) string {
	var appeared, gone, changed []string
	for k := range st.objects {
		if _, ok := ref.objects[k]; !ok {
			appeared = append(appeared, k)
		}
	}
	for k := range ref.objects {
		if _, ok := st.objects[k]; !ok {
			gone = append(gone, k)
		}
	}
	for k, v := range st.rows {
		if r, ok := ref.rows[k]; ok && r != v {
			changed = append(changed, fmt.Sprintf("%s(%d→%d rows)", k, ref.counts[k], st.counts[k]))
		}
	}
	sort.Strings(appeared)
	sort.Strings(gone)
	sort.Strings(changed)
	if len(appeared)+len(gone)+len(changed) == 0 {
		return "identical to the fixture"
	}
	return fmt.Sprintf("appeared %v · disappeared %v · rows changed %v", appeared, gone, changed)
}

// e3Code is the SQLite result code err carries, or 0.
func e3Code(err error) int {
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	return 0
}

// ---- the child process ----

const e3ChildEnv = "KORVUN_E3_CHILD"

// e3ChildSpec is what a child is told: the file and the profile, the
// migration point it dies at (exit 9, no defers) and the points it holds
// until released; or, with Hold, no open at all — it takes the file's write
// lock (BEGIN IMMEDIATE on a raw connection), acknowledges it and holds it
// until released.
type e3ChildSpec struct {
	Path     string   `json:"path"`
	Profile  string   `json:"profile,omitempty"`
	Crash    string   `json:"crash,omitempty"`
	Barriers []string `json:"barriers,omitempty"`
	Hold     bool     `json:"hold,omitempty"`
}

// e3Report is what a child that did not die says last.
type e3Report struct {
	Opened  bool   `json:"opened"`
	Class   string `json:"class"`
	Err     string `json:"err,omitempty"`
	Version int    `json:"version"`
	DDL     int    `json:"ddl"`
	Bumps   int    `json:"bumps"`
	Skipped int    `json:"skipped"`
}

// TestE3_migrateChild is not a mould: it is the child process of TE33, TE34
// and TE35.
func TestE3_migrateChild(t *testing.T) {
	raw := os.Getenv(e3ChildEnv)
	if raw == "" {
		t.Skip("batch 3's child; run only by its moulds")
	}
	var spec e3ChildSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("child spec: %v", err)
	}
	ctx := context.Background()
	say := func(format string, args ...any) { _, _ = fmt.Fprintf(os.Stdout, format+"\n", args...) }
	in := bufio.NewScanner(os.Stdin)
	if spec.Hold {
		db, err := sql.Open("sqlite", "file:"+spec.Path+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
			t.Fatalf("take the write lock: %v", err)
		}
		say("E2ACK lock:held")
		released := false
		for in.Scan() {
			if in.Text() == "E2GO lock:held" {
				released = true
				break
			}
		}
		if !released {
			os.Exit(3) // the parent closed the pipe without releasing
		}
		if _, err := conn.ExecContext(ctx, `ROLLBACK`); err != nil {
			t.Fatalf("release the write lock: %v", err)
		}
		say(`E2DONE {"opened":false,"class":"nil"}`)
		return
	}
	o := &migrationObserver{path: e2Abs(t, spec.Path), at: func(point string) {
		say("E2ACK %s", point)
		if point == spec.Crash {
			os.Exit(9)
		}
		for _, b := range spec.Barriers {
			if b != point {
				continue
			}
			for in.Scan() {
				if in.Text() == "E2GO "+point {
					return
				}
			}
			os.Exit(3) // the parent closed the pipe without releasing
		}
	}}
	migrationObserverSeam.Store(o)
	h, err := OpenFor(spec.Path, spec.Profile)
	rep := e3Report{Opened: err == nil, Class: e2Class(err)}
	if err != nil {
		rep.Err = err.Error()
	} else {
		rep.Version, _ = h.SchemaVersion(ctx)
		_ = h.Close()
	}
	w := o.work()
	rep.DDL, rep.Bumps, rep.Skipped = w.ddl, w.bumps, w.skipped
	out, _ := json.Marshal(rep)
	say("E2DONE %s", out)
}

// e3StartChild starts TestE3_migrateChild with spec; the child speaks batch
// 2's protocol, read through e2Child.
func e3StartChild(t *testing.T, spec e3ChildSpec) *e2Child {
	t.Helper()
	blob, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- the batch-3 moulds deliberately re-execute this test binary.
	cmd := exec.Command(os.Args[0], "-test.run=^TestE3_migrateChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), e3ChildEnv+"="+string(blob))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	c := &e2Child{cmd: cmd, stdin: stdin, lines: make(chan e2Line, 256), stderr: &bytes.Buffer{}, exited: make(chan struct{})}
	cmd.Stderr = c.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the child: %v", err)
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			c.mu.Lock()
			c.log = append(c.log, line)
			c.mu.Unlock()
			if strings.HasPrefix(line, "E2") {
				c.lines <- e2Line{text: line, at: time.Now()}
			}
		}
		close(c.lines)
		err := cmd.Wait()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			c.code = 0
		case errors.As(err, &exitErr):
			c.code = exitErr.ExitCode()
		default:
			c.code = -1
		}
		close(c.exited)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-c.exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-c.exited
		}
	})
	return c
}

// e3ReportOf decodes a batch-3 child's E2DONE line.
func e3ReportOf(t *testing.T, l e2Line) e3Report {
	t.Helper()
	var r e3Report
	if err := json.Unmarshal([]byte(strings.TrimPrefix(l.text, "E2DONE ")), &r); err != nil {
		t.Fatalf("the child's report %q: %v", l.text, err)
	}
	return r
}

// e3Lines logs, for the report, every protocol line of a child with its time
// since t0.
func e3Lines(t *testing.T, label, who string, t0 time.Time, lines ...e2Line) {
	t.Helper()
	for _, l := range lines {
		e3Evidence(t, "%s %s +%6dms %s", label, who, l.at.Sub(t0).Milliseconds(), l.text)
	}
}
