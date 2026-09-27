// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The judge's instrumentation (train E, NUEVO, plan §7): every read the judge
// makes is named by its SITE, the caller whose judgement it serves by its
// ORIGIN and the moment of the read by its STAGE, so a mould can put ONE
// failure on ONE read of ONE file and prove who consumed it. The same holds
// for the operations around the judgement — the hook's own statements, the
// guard's installation, the prune, the seals and the ping — through a
// second, stage-named seam.
//
// Production never arms anything here: every seam is nil unless a mould
// stores one, and every seam is selected by the absolute path of the file it
// may touch, so a mould cannot hit a file another mould opened. The seam
// replaces the RESULT of a read that really ran; it never skips the read and
// never hands a caller a judgement of its own.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// judgeOrigin names the caller whose judgement a read serves.
type judgeOrigin string

// The origins, one per caller of the judge (plan §7).
const (
	originHookAtBirth       judgeOrigin = "hook-at-birth"
	originPoolOpenShape     judgeOrigin = "pool-open-shape"
	originReadOwnerAtOpen   judgeOrigin = "readOwner-at-open"
	originOperatorProbe     judgeOrigin = "operator-probe"
	originReadOnlyOpen      judgeOrigin = "read-only-open"
	originRefresh           judgeOrigin = "installGuard/refresh"
	originOpenForStanding   judgeOrigin = "OpenFor-final-Standing"
	originPublicStanding    judgeOrigin = "public-Standing"
	originBeginWrite        judgeOrigin = "beginWrite"
	originBeginAdoption     judgeOrigin = "beginAdoption"
	originRefuseMaintenance judgeOrigin = "refuseMaintenance"
	originSchemaVersion     judgeOrigin = "SchemaVersion-accessor"
	// originSeedLockedRejudge is the seed's judgement inside its own
	// immediate transaction (plan §7, «Locked seed»).
	originSeedLockedRejudge judgeOrigin = "seed-locked-rejudge"
	// originPoolOpenConfirm and originReadOnlyConfirm are the second
	// judgements of plan §13.1: a bad verdict of the writer's or the
	// reader's open is judged once more before the open acts on it.
	originPoolOpenConfirm judgeOrigin = "pool-open-confirm"
	originReadOnlyConfirm judgeOrigin = "read-only-confirm"
	// originMigrationReread is a migration step's reading of the stored
	// version inside its own transaction (plan §4, «Migration»).
	originMigrationReread judgeOrigin = "migration-reread"
)

// judgeQuerySite names one read of the judge.
type judgeQuerySite string

// The sites (plan §7). siteNone is a read that is not the judge's (the
// catalog the guard's triggers are created from): no judge fault reaches it.
const (
	siteNone        judgeQuerySite = ""
	siteCatalog     judgeQuerySite = "Qcatalog"
	siteVersion     judgeQuerySite = "Qversion"
	siteOwnerDB     judgeQuerySite = "QownerDB"
	siteCountDriver judgeQuerySite = "QcountDriver"
	siteOwnerDriver judgeQuerySite = "QownerDriver"
	siteMarkDB      judgeQuerySite = "QmarkDB"
	siteMarkDriver  judgeQuerySite = "QmarkDriver"
	// siteResidue is every read that recognises an empty bootstrap prefix
	// (its objects, its rows); siteIndex every read of the UNIQUE indexes a
	// current ledger requires (plan §4, §7).
	siteResidue judgeQuerySite = "Qresidue"
	siteIndex   judgeQuerySite = "Qindex"
	// siteVersionTx is a migration step's read of the stored version inside
	// its own transaction, before any of its writes.
	siteVersionTx judgeQuerySite = "QversionTx"
)

// readStage names the moment of a read whose result a fault replaces: the
// query itself, the iteration, or the conversion of one cell.
type readStage string

// The stages of a read.
const (
	stageQuery readStage = "query"
	stageNext  readStage = "next"
	stageScan  readStage = "scan"
)

// judgeScope is what a context carries for the instrumentation: the origin of
// the judgement and the file it reads.
type judgeScope struct {
	origin judgeOrigin
	path   string
}

type judgeScopeKey struct{}

// withJudgeOrigin names origin and path on ctx — unless ctx already names an
// origin: a nested reader keeps its caller's (OpenFor's Standing is OpenFor's,
// the prune's write is the prune's).
func withJudgeOrigin(ctx context.Context, origin judgeOrigin, path string) context.Context {
	if _, ok := ctx.Value(judgeScopeKey{}).(judgeScope); ok {
		return ctx
	}
	return context.WithValue(ctx, judgeScopeKey{}, judgeScope{origin: origin, path: path})
}

func judgeScopeOf(ctx context.Context) judgeScope {
	scope, _ := ctx.Value(judgeScopeKey{}).(judgeScope)
	return scope
}

// sameFile reports whether two absolute paths are one spelling once cleaned
// (filepath.Clean) — no symlink, case or short-name folding. A seam is
// found when its mould armed it with the path spelt as the opener spells
// it, which is the spelling dsnPath gives back.
func sameFile(a, b string) bool {
	return a != "" && filepath.Clean(a) == filepath.Clean(b)
}

// dsnPath is the path of the file a connection's DSN names, for the hook,
// which sees only the DSN: the absolute path the opener built that DSN from
// (buildFileDSN, buildWriterDSN), spelt as the opener spelt it, so the seams
// selected by that path arm. The platform step is dsnPathFor.
func dsnPath(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return dsnPathFor(runtime.GOOS, u.Path)
}

// dsnPathFor turns the URL path of a file DSN built from an ABSOLUTE path —
// the only kind the openers build from — back into that path, for goos. The
// builders put a "/" in front of a path that has none, so on windows a drive
// path arrives as "/C:/…": that slash is dropped, and every "/" becomes the
// Windows separator (a UNC share, "//server/…", keeps its two). Any other URL
// path keeps its slash: a relative spelling such as "C:x" or "1:/x" does not
// come back as it was built, and the openers never build from one. On every
// other goos the URL path is already the file's path. Pure — goos is an
// argument — so a mould runs every platform's rows on any host.
func dsnPathFor(goos, urlPath string) string {
	if goos != "windows" {
		return urlPath
	}
	if len(urlPath) >= 3 && urlPath[0] == '/' && urlPath[2] == ':' && isDriveLetter(urlPath[1]) &&
		(len(urlPath) == 3 || urlPath[3] == '/') {
		urlPath = urlPath[1:]
	}
	return strings.ReplaceAll(urlPath, "/", `\`)
}

// isDriveLetter reports whether c can name a Windows drive.
func isDriveLetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// judgeReadFault is one armed failure: the read it replaces — file, origin,
// site, stage — and what it saw. At most one read ever receives it.
type judgeReadFault struct {
	path   string
	origin judgeOrigin
	site   judgeQuerySite
	stage  readStage
	fault  error

	mu sync.Mutex
	// matches counts the reads that reached this file, origin, site and stage.
	matches int
	// consumed is true once one of them received the fault.
	consumed bool
	// otherOrigins lists the origins of reads of the same file, site and stage
	// that did NOT receive the fault (it is never handed to another origin).
	otherOrigins []judgeOrigin
	// decisions are the site decisions the failure carrying the fault met.
	decisions []readDecision
}

// readDecision is one decision of a judge site on a failed read: whether the
// failure was taken as a verdict on the file, and the class it carried.
type readDecision struct {
	site    judgeQuerySite
	verdict bool
	class   ledgerClass
}

// judgeReadFaultSeam is the armed judge fault, or nil; only the moulds set it.
var judgeReadFaultSeam atomic.Pointer[judgeReadFault]

// judgeReadObservation is one read failure a judge site met, recorded for the
// moulds that must SEE a native failure (a damaged page) rather than inject
// one.
type judgeReadObservation struct {
	origin judgeOrigin
	site   judgeQuerySite
	stage  readStage
	err    error
}

// judgeReadObserver records every judge read failure on one file; only the
// moulds set it.
type judgeReadObserver struct {
	path string
	mu   sync.Mutex
	seen []judgeReadObservation
}

var judgeReadObserverSeam atomic.Pointer[judgeReadObserver]

// judgeRead is the seam on the RESULT of one judge read: it returns err, or
// the armed fault when this read is the one the fault names and none has
// received it yet.
func judgeRead(ctx context.Context, site judgeQuerySite, stage readStage, err error) error {
	scope := judgeScopeOf(ctx)
	// io.EOF is the driver's normal end of rows, not a failure.
	if err != nil && site != siteNone && !errors.Is(err, io.EOF) {
		if o := judgeReadObserverSeam.Load(); o != nil && sameFile(scope.path, o.path) {
			o.mu.Lock()
			o.seen = append(o.seen, judgeReadObservation{origin: scope.origin, site: site, stage: stage, err: err})
			o.mu.Unlock()
		}
	}
	f := judgeReadFaultSeam.Load()
	if f == nil || site == siteNone || site != f.site || stage != f.stage || !sameFile(scope.path, f.path) {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if scope.origin != f.origin {
		f.otherOrigins = append(f.otherOrigins, scope.origin)
		return err
	}
	f.matches++
	if f.consumed {
		return err
	}
	f.consumed = true
	return f.fault
}

// noteReadDecision records, for the armed fault, the decision a judge site
// took on a failure that carries it.
func noteReadDecision(site judgeQuerySite, err error, verdict bool) {
	f := judgeReadFaultSeam.Load()
	if f == nil || err == nil || f.fault == nil || !errors.Is(err, f.fault) {
		return
	}
	f.mu.Lock()
	f.decisions = append(f.decisions, readDecision{site: site, verdict: verdict, class: classOf(err)})
	f.mu.Unlock()
}

// openStageFault is one armed failure on an operation around the judgement,
// named by its stage (`hook:temp-create`, `install:trigger#3`, `prune:commit`,
// `readonly:seal` …): the operation runs, and its result is replaced once.
type openStageFault struct {
	path  string
	stage string
	fault error

	mu       sync.Mutex
	matches  int
	consumed bool
}

var openStageFaultSeam atomic.Pointer[openStageFault]

// stageFault is the seam on the result of one named operation on path.
func stageFault(path, stage string, err error) error {
	f := openStageFaultSeam.Load()
	if f == nil || stage != f.stage || !sameFile(path, f.path) {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.matches++
	if f.consumed {
		return err
	}
	f.consumed = true
	return f.fault
}

// openCapOverride gives the handles a writer opener births on one file a
// retention cap of its own, so a mould can reach the prune's DELETE through
// the profile's opener; only the moulds set it.
type openCapOverride struct {
	path    string
	capRows int
}

var openCapSeam atomic.Pointer[openCapOverride]

func capRowsFor(abs string) int {
	if o := openCapSeam.Load(); o != nil && sameFile(abs, o.path) {
		return o.capRows
	}
	return defaultCapRows
}

// seedObserver watches the seed of ONE file (plan §7, S1/S2): the named
// points of the seed — where a mould acknowledges, holds a barrier or ends a
// child process — and the production-dispatch counters of the statements the
// seed sends. Only the moulds set it.
type seedObserver struct {
	path string
	// at runs at each named point with the connection the seed sends its
	// statements on (nil where no statement follows).
	at func(point string, on seedConn)

	mu       sync.Mutex
	points   []string
	ddl      int
	inserts  int
	failures []seedFailure
}

// seedFailure is one seed statement that failed: its place in the seed (1–5
// the bootstrap DDL, 6 the version row) and the error.
type seedFailure struct {
	statement int
	err       error
}

// seedConn is what the seed sends its statements through.
type seedConn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var seedObserverSeam atomic.Pointer[seedObserver]

func seedObserverFor(abs string) *seedObserver {
	o := seedObserverSeam.Load()
	if o == nil || !sameFile(abs, o.path) {
		return nil
	}
	return o
}

// seedPoint names one point of the seed of abs.
func seedPoint(abs, point string, on seedConn) {
	o := seedObserverFor(abs)
	if o == nil {
		return
	}
	o.mu.Lock()
	o.points = append(o.points, point)
	o.mu.Unlock()
	if o.at != nil {
		o.at(point, on)
	}
}

// seedSent counts one statement the seed of abs dispatches: a bootstrap DDL,
// or the version row's INSERT.
func seedSent(abs string, insert bool) {
	o := seedObserverFor(abs)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if insert {
		o.inserts++
	} else {
		o.ddl++
	}
}

// seedStatementFailed records a seed statement of abs that failed.
func seedStatementFailed(abs string, statement int, err error) {
	o := seedObserverFor(abs)
	if o == nil {
		return
	}
	o.mu.Lock()
	o.failures = append(o.failures, seedFailure{statement: statement, err: err})
	o.mu.Unlock()
}

// hookBirthObserver records the standing the connection hook installed at
// every connection birth on one file; only the moulds set it.
type hookBirthObserver struct {
	path      string
	mu        sync.Mutex
	standings []string
}

var hookBirthObserverSeam atomic.Pointer[hookBirthObserver]

// noteHookBirth records the standing a connection of path was born with.
func noteHookBirth(path, standing string) {
	o := hookBirthObserverSeam.Load()
	if o == nil || !sameFile(path, o.path) {
		return
	}
	o.mu.Lock()
	o.standings = append(o.standings, standing)
	o.mu.Unlock()
}

// migrationObserver watches the migration of ONE file (plan §7, S4/S5): the
// named points of each step — where a mould acknowledges, holds a barrier or
// ends a child process — and the counters of the work the steps do. Only the
// moulds set it.
type migrationObserver struct {
	path string
	// at runs at each named point.
	at func(point string)

	mu      sync.Mutex
	points  []string
	ddl     int // step scripts executed
	bumps   int // version bumps executed
	skipped int // steps skipped as stale by their own reread
}

var migrationObserverSeam atomic.Pointer[migrationObserver]

func migrationObserverFor(path string) *migrationObserver {
	o := migrationObserverSeam.Load()
	if o == nil || !sameFile(path, o.path) {
		return nil
	}
	return o
}

// migrationPoint names one point of a migration of path.
func migrationPoint(path, point string) {
	o := migrationObserverFor(path)
	if o == nil {
		return
	}
	o.mu.Lock()
	o.points = append(o.points, point)
	o.mu.Unlock()
	if o.at != nil {
		o.at(point)
	}
}

// migrationWork counts one piece of a migration step's work on path: "ddl",
// "bump" or "skip".
func migrationWork(path, what string) {
	o := migrationObserverFor(path)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	switch what {
	case "ddl":
		o.ddl++
	case "bump":
		o.bumps++
	case "skip":
		o.skipped++
	}
}
