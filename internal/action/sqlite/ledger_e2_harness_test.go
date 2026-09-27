// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Train E, batch 2 (GE1, GE1b, GE5): the harness of the moulds TE01–TE11,
// TE57 and TE68–TE70 (plan v3, §§4, 6–7 and §13.1).
//
// The files are real: a conversation store opened by its own package, with
// one row in each of its tables, and — where a mould needs one — the action
// store's historical bootstrap prefix laid over it statement by statement
// through a second connection in autocommit, as the seed before train E left
// it after a crash. The seed is watched through seedObserverSeam: the points
// it names, the statements it dispatches and the ones that failed. A crash or
// a barrier is a CHILD process (this test binary re-executed) that
// acknowledges each point on its stdout and waits on its stdin; nothing
// synchronises by sleeping. The conversation tables carry traps that count
// every write attempted on them, outside the transaction that attempts it.

package sqlite

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	convsqlite "github.com/Sebastian197/korvun/internal/conversation/sqlite"
	msqlite "modernc.org/sqlite"
)

// e2Statements is the v1 bootstrap as the moulds read it: createStmt cut at
// its semicolons here, not by the production splitter.
func e2Statements(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, s := range strings.Split(createStmt, ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) != 5 {
		t.Fatalf("createStmt holds %d statements, want the five of the v1 bootstrap", len(out))
	}
	return out
}

// e2BootstrapObjects are the objects the five statements create, in their
// order, as "type:name".
var e2BootstrapObjects = []string{"table:action_schema", "table:actions", "index:actions_by_correlation", "index:actions_by_requested", "table:action_decisions"}

func e2Abs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// e2Raw is a second real connection to path, closed with the test.
func e2Raw(t *testing.T, path string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return raw
}

// e2ConversationFile is a real conversation store with one row in each of
// its tables, closed: the shared file as the action store first meets it.
func e2ConversationFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "korvun.db")
	conv, err := convsqlite.Open(path)
	if err != nil {
		t.Fatalf("open the conversation store: %v", err)
	}
	if err := conv.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	for _, q := range []string{
		`INSERT INTO sessions (key, id, created_ts) VALUES ('k', 1, 1)`,
		`INSERT INTO turns (key, session, seq, role, content, ts) VALUES ('k', 1, 1, 'user', 'hola', 1)`,
		`INSERT INTO notes (brain, key, seq, content, ts) VALUES ('b', 'k', 1, 'nota', 1)`,
	} {
		rawExec(t, raw, q)
	}
	return path
}

// e2Lay runs statements over path in autocommit, one by one, through a raw
// connection it closes after.
func e2Lay(t *testing.T, path string, statements ...string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	for _, q := range statements {
		rawExec(t, raw, q)
	}
}

// e2Prefix lays the first k statements of the v1 bootstrap over path: the
// residue a crash of the historical seed left.
func e2Prefix(t *testing.T, path string, k int) {
	t.Helper()
	e2Lay(t, path, e2Statements(t)[:k]...)
}

// e2PrefixReordered lays all five in another order: action_schema last, the
// second index before the first.
func e2PrefixReordered(t *testing.T, path string) {
	t.Helper()
	s := e2Statements(t)
	e2Lay(t, path, s[1], s[3], s[2], s[4], s[0])
}

// e2State is a file as a raw reader sees it: every object of its catalog with
// its target table and DDL, and a digest of every table's rows.
type e2State struct {
	objects map[string]string // "type:name" → tbl_name NUL sql
	rows    map[string]string // table → digest of its sorted rows
	counts  map[string]int    // table → rows
}

func e2Snapshot(t *testing.T, path string) e2State {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	st := e2State{objects: map[string]string{}, rows: map[string]string{}, counts: map[string]int{}}
	rows, err := raw.Query(`SELECT type, name, tbl_name, ifnull(sql, '') FROM sqlite_master`)
	if err != nil {
		t.Fatalf("read the catalog of %s: %v", path, err)
	}
	var tables []string
	for rows.Next() {
		var kind, name, tbl, ddl string
		if err := rows.Scan(&kind, &name, &tbl, &ddl); err != nil {
			t.Fatal(err)
		}
		st.objects[kind+":"+name] = tbl + "\x00" + ddl
		if kind == "table" && !strings.HasPrefix(strings.ToUpper(ddl), "CREATE VIRTUAL TABLE") {
			tables = append(tables, name)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		r, err := raw.Query(`SELECT * FROM "` + strings.ReplaceAll(name, `"`, `""`) + `"`) //nolint:gosec // G202: a table of the mould's own file, quoted as an identifier
		if err != nil {
			t.Fatalf("read the rows of %s: %v", name, err)
		}
		cols, err := r.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for r.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, fmt.Sprintf("%#v", vals))
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		sort.Strings(lines)
		sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
		st.rows[name] = fmt.Sprintf("%x", sum)
		st.counts[name] = len(lines)
	}
	return st
}

// e2Diff names the first difference between two states, or "".
func e2Diff(a, b e2State) string {
	keys := map[string]bool{}
	for k := range a.objects {
		keys[k] = true
	}
	for k := range b.objects {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		va, ina := a.objects[k]
		vb, inb := b.objects[k]
		switch {
		case !ina:
			return fmt.Sprintf("object %s appeared", k)
		case !inb:
			return fmt.Sprintf("object %s disappeared", k)
		case va != vb:
			return fmt.Sprintf("object %s changed: %q → %q", k, va, vb)
		}
	}
	for k, va := range a.rows {
		if vb, ok := b.rows[k]; ok && va != vb {
			return fmt.Sprintf("the rows of %s changed (%d → %d)", k, a.counts[k], b.counts[k])
		}
	}
	return ""
}

// e2Same fails when after differs from before.
func e2Same(t *testing.T, label string, before, after e2State) {
	t.Helper()
	if d := e2Diff(before, after); d != "" {
		t.Fatalf("%s: the file changed: %s", label, d)
	}
}

// e2ConversationTables are the conversation store's.
var e2ConversationTables = []string{"sessions", "turns", "notes"}

// conversation is the part of a state the conversation store owns: its
// tables, every object on them (the traps included) and their rows.
func (s e2State) conversation() e2State {
	own := map[string]bool{}
	for _, c := range e2ConversationTables {
		own[c] = true
	}
	out := e2State{objects: map[string]string{}, rows: map[string]string{}, counts: map[string]int{}}
	for k, v := range s.objects {
		tbl, _, _ := strings.Cut(v, "\x00")
		if own[tbl] {
			out.objects[k] = v
		}
	}
	for _, c := range e2ConversationTables {
		out.rows[c], out.counts[c] = s.rows[c], s.counts[c]
	}
	return out
}

// e2TrapHits counts, per mould token, the conversation writes a trap stopped.
var (
	e2TrapOnce sync.Once
	e2TrapHits sync.Map // token → *atomic.Int64
)

// e2RegisterTrap registers e2_trap(token) once per process: a trap calls it
// before it aborts the write, so the count survives the rollback.
func e2RegisterTrap() {
	e2TrapOnce.Do(func() {
		msqlite.MustRegisterScalarFunction("e2_trap", 1, func(_ *msqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			token, _ := args[0].(string)
			n, _ := e2TrapHits.LoadOrStore(token, new(atomic.Int64))
			n.(*atomic.Int64).Add(1)
			return nil, nil
		})
	})
}

// e2Traps are the traps of one mould on one file.
type e2Traps struct{ token string }

// e2ArmTraps puts a BEFORE INSERT, UPDATE and DELETE trap on every
// conversation table of path: each counts the write and aborts it.
func e2ArmTraps(t *testing.T, path string) e2Traps {
	t.Helper()
	e2RegisterTrap()
	token := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	var stmts []string
	for _, table := range e2ConversationTables {
		for _, ev := range []string{"INSERT", "UPDATE", "DELETE"} {
			stmts = append(stmts, fmt.Sprintf(`CREATE TRIGGER e2_trap_%s_%s BEFORE %s ON %s BEGIN SELECT e2_trap('%s'); SELECT RAISE(ABORT, 'e2 trap: %s on %s'); END`,
				table, strings.ToLower(ev), ev, table, strings.ReplaceAll(token, "'", "''"), ev, table))
		}
	}
	e2Lay(t, path, stmts...)
	return e2Traps{token: token}
}

func (tr e2Traps) hits() int64 {
	if n, ok := e2TrapHits.Load(tr.token); ok {
		return n.(*atomic.Int64).Load()
	}
	return 0
}

// e2WatchSeed arms the seed observer on path; at may be nil.
func e2WatchSeed(t *testing.T, path string, at func(point string, on seedConn)) *seedObserver {
	t.Helper()
	o := &seedObserver{path: e2Abs(t, path), at: at}
	if !seedObserverSeam.CompareAndSwap(nil, o) {
		t.Fatal("another seed observer is armed: the moulds of this batch are sequential")
	}
	t.Cleanup(func() { seedObserverSeam.CompareAndSwap(o, nil) })
	return o
}

// e2Seen is what a seed observer saw.
type e2Seen struct {
	ddl, inserts int
	points       []string
	failures     []seedFailure
}

func (o *seedObserver) seen() e2Seen {
	o.mu.Lock()
	defer o.mu.Unlock()
	return e2Seen{ddl: o.ddl, inserts: o.inserts, points: append([]string(nil), o.points...), failures: append([]seedFailure(nil), o.failures...)}
}

// e2WatchHook arms the hook's birth observer on path.
func e2WatchHook(t *testing.T, path string) *hookBirthObserver {
	t.Helper()
	o := &hookBirthObserver{path: e2Abs(t, path)}
	if !hookBirthObserverSeam.CompareAndSwap(nil, o) {
		t.Fatal("another hook observer is armed: the moulds of this batch are sequential")
	}
	t.Cleanup(func() { hookBirthObserverSeam.CompareAndSwap(o, nil) })
	return o
}

func (o *hookBirthObserver) seen() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.standings...)
}

// e2Gate holds the FIRST seed of its file that reaches point until released;
// every later seed of the file passes (an in-process barrier).
type e2Gate struct {
	point   string
	reached chan struct{}
	release chan struct{}
	mu      sync.Mutex
	used    bool
}

func newE2Gate(point string) *e2Gate {
	return &e2Gate{point: point, reached: make(chan struct{}), release: make(chan struct{})}
}

func (g *e2Gate) at(point string, _ seedConn) {
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

// e2Class names the class of an opener's error for the reports.
func e2Class(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ErrSchemaFromTheFuture):
		return "future"
	case errors.Is(err, ErrNoActionStore):
		return "nostore"
	case errors.Is(err, ErrSchemaBehind):
		return "behind"
	case errors.Is(err, ErrLedgerUnreadable):
		return "unreadable"
	case errors.Is(err, ErrLedgerBusy):
		return "busy"
	case errors.Is(err, ErrLedgerEnvironment):
		return "environment"
	default:
		return "none"
	}
}

// e2Healthy opens path for profileA and asserts the one healthy outcome of a
// completed seed: v16, legacy_unfounded, and a guarded act recorded.
func e2Healthy(t *testing.T, label, path string) {
	e2HealthyAct(t, label, path, "e2_act")
}

// e2HealthyAct is e2Healthy recording the act named id.
func e2HealthyAct(t *testing.T, label, path, id string) {
	t.Helper()
	h, err := OpenFor(path, profileA)
	if err != nil {
		t.Fatalf("%s: OpenFor = %v, want the ledger seeded and opened", label, err)
	}
	defer func() { _ = h.Close() }()
	if v, err := h.SchemaVersion(context.Background()); err != nil || v != schemaVersionCurrent {
		t.Fatalf("%s: SchemaVersion = %d %v, want %d", label, v, err, schemaVersionCurrent)
	}
	if st, _, err := h.Standing(context.Background()); err != nil || st != LedgerStandingLegacyUnfounded {
		t.Fatalf("%s: Standing = %q %v, want legacy_unfounded", label, st, err)
	}
	if err := guardedWrite(h, id); err != nil {
		t.Fatalf("%s: a guarded write = %v, want it recorded", label, err)
	}
}

// ---- the child process ----

const e2ChildEnv = "KORVUN_E2_CHILD"

// e2ChildSpec is what a child is told: the file and the profile, the seed
// point it dies at (exit 9, no defers) and the points it holds until
// released, whether it records one guarded act once open; or, for the
// official reproduction, how many bootstrap statements it lays in autocommit
// before it dies (the historical seed).
type e2ChildSpec struct {
	Path       string   `json:"path"`
	Profile    string   `json:"profile"`
	Crash      string   `json:"crash,omitempty"`
	Barriers   []string `json:"barriers,omitempty"`
	Write      bool     `json:"write,omitempty"`
	Historical int      `json:"historical,omitempty"`
}

// e2Report is what a child that did not die says last.
type e2Report struct {
	Opened   bool   `json:"opened"`
	Class    string `json:"class"`
	Err      string `json:"err,omitempty"`
	Standing string `json:"standing,omitempty"`
	Write    string `json:"write,omitempty"`
	DDL      int    `json:"ddl"`
	Inserts  int    `json:"inserts"`
	Traps    int64  `json:"traps"`
}

// TestE2_seedChild is not a mould: it is the child process of TE01, TE04 and
// TE57, and the historical seed of TE02's official reproduction.
func TestE2_seedChild(t *testing.T) {
	raw := os.Getenv(e2ChildEnv)
	if raw == "" {
		t.Skip("batch 2's seed child; run only by its moulds")
	}
	var spec e2ChildSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("child spec: %v", err)
	}
	e2RegisterTrap()
	say := func(format string, args ...any) { _, _ = fmt.Fprintf(os.Stdout, format+"\n", args...) }
	if spec.Historical > 0 {
		db, err := sql.Open("sqlite", "file:"+spec.Path+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range e2Statements(t)[:spec.Historical] {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("historical statement: %v", err)
			}
		}
		say("E2ACK historical:%d", spec.Historical)
		os.Exit(9)
	}
	in := bufio.NewScanner(os.Stdin)
	o := &seedObserver{path: e2Abs(t, spec.Path), at: func(point string, on seedConn) {
		_, inTx := on.(*sql.Tx)
		say("E2ACK %s tx=%v", point, inTx)
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
	seedObserverSeam.Store(o)
	h, err := OpenFor(spec.Path, spec.Profile)
	rep := e2Report{Opened: err == nil, Class: e2Class(err)}
	if err != nil {
		rep.Err = err.Error()
	} else {
		st, _, serr := h.Standing(context.Background())
		rep.Standing = string(st)
		if serr != nil {
			rep.Standing += " " + serr.Error()
		}
		if spec.Write {
			if werr := guardedWrite(h, fmt.Sprintf("e2_child_act_%d", os.Getpid())); werr != nil {
				rep.Write = werr.Error()
			}
		}
		_ = h.Close()
	}
	seen := o.seen()
	rep.DDL, rep.Inserts = seen.ddl, seen.inserts
	e2TrapHits.Range(func(_, n any) bool {
		rep.Traps += n.(*atomic.Int64).Load()
		return true
	})
	out, _ := json.Marshal(rep)
	say("E2DONE %s", out)
}

// e2Child is a running child and every line it said.
type e2Child struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan e2Line
	stderr *bytes.Buffer
	mu     sync.Mutex
	log    []string
	exited chan struct{}
	code   int
}

// e2Line is one protocol line and when it arrived.
type e2Line struct {
	text string
	at   time.Time
}

func e2StartChild(t *testing.T, spec e2ChildSpec) *e2Child {
	t.Helper()
	blob, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- the batch-2 moulds deliberately re-execute this test binary.
	cmd := exec.Command(os.Args[0], "-test.run=^TestE2_seedChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), e2ChildEnv+"="+string(blob))
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

// said is everything the child said, for a failure message.
func (c *e2Child) said() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.log, "\n") + "\n--- stderr ---\n" + c.stderr.String()
}

// next is the child's next protocol line within d; ok is false when the
// child's output ended first.
func (c *e2Child) next(t *testing.T, d time.Duration) (e2Line, bool) {
	t.Helper()
	select {
	case l, ok := <-c.lines:
		return l, ok
	case <-time.After(d):
		_ = c.cmd.Process.Kill()
		t.Fatalf("the child said nothing within %v\n%s", d, c.said())
		return e2Line{}, false
	}
}

// until reads the child's lines up to the one that starts with prefix.
func (c *e2Child) until(t *testing.T, prefix string, d time.Duration) e2Line {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		l, ok := c.next(t, time.Until(deadline))
		if !ok {
			t.Fatalf("the child ended before %q\n%s", prefix, c.said())
		}
		if strings.HasPrefix(l.text, prefix) {
			return l
		}
	}
}

// release lets the child past its barrier at point.
func (c *e2Child) release(t *testing.T, point string) {
	t.Helper()
	if _, err := io.WriteString(c.stdin, "E2GO "+point+"\n"); err != nil {
		t.Fatalf("release %s: %v\n%s", point, err, c.said())
	}
}

// finish reads every remaining line and waits for the exit, within d.
func (c *e2Child) finish(t *testing.T, d time.Duration) (int, []e2Line) {
	t.Helper()
	var got []e2Line
	deadline := time.Now().Add(d)
	for {
		l, ok := c.next(t, time.Until(deadline))
		if !ok {
			break
		}
		got = append(got, l)
	}
	select {
	case <-c.exited:
	case <-time.After(time.Until(deadline)):
		_ = c.cmd.Process.Kill()
		t.Fatalf("the child did not exit within %v\n%s", d, c.said())
	}
	return c.code, got
}

// e2ReportOf decodes an E2DONE line.
func e2ReportOf(t *testing.T, l e2Line) e2Report {
	t.Helper()
	var r e2Report
	if err := json.Unmarshal([]byte(strings.TrimPrefix(l.text, "E2DONE ")), &r); err != nil {
		t.Fatalf("the child's report %q: %v", l.text, err)
	}
	return r
}

// e2Acks are the points a child acknowledged, in order, and the done line.
func e2Acks(lines []e2Line) (acks []string, done *e2Line) {
	for i := range lines {
		switch {
		case strings.HasPrefix(lines[i].text, "E2ACK "):
			acks = append(acks, strings.TrimPrefix(lines[i].text, "E2ACK "))
		case strings.HasPrefix(lines[i].text, "E2DONE "):
			done = &lines[i]
		}
	}
	return acks, done
}
