// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package supervisor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// ---- deterministic fakes ----------------------------------------------------

// eventLog is a mutex-guarded ordered record of lifecycle events, so the cutover
// ordering can be asserted without sleeps and read safely under -race.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(e string) {
	l.mu.Lock()
	l.events = append(l.events, e)
	l.mu.Unlock()
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

func (l *eventLog) contains(e string) bool {
	for _, x := range l.snapshot() {
		if x == e {
			return true
		}
	}
	return false
}

func (l *eventLog) count(e string) int {
	n := 0
	for _, x := range l.snapshot() {
		if x == e {
			n++
		}
	}
	return n
}

// assertOrder fails unless want appears as an ordered subsequence of the log.
func (l *eventLog) assertOrder(t *testing.T, want ...string) {
	t.Helper()
	got := l.snapshot()
	i := 0
	for _, e := range got {
		if i < len(want) && e == want[i] {
			i++
		}
	}
	if i != len(want) {
		t.Fatalf("event order %v does not contain subsequence %v", got, want)
	}
}

// syncBuf is a mutex-guarded writer so a test can read a supervisor log written
// from the Run goroutine without racing.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeApp is a deterministic App split like *app.App (ADR-0027): Start records
// "<name>.Start" and returns startErr (the cutover-abort failure — a bad admin
// re-bind); Serve records "<name>.Serve", closes started, then blocks until ctx is
// cancelled (or returns serveErr, a rare serving crash). Shutdown records and counts.
type fakeApp struct {
	name      string
	log       *eventLog
	started   chan struct{}
	startErr  error
	serveErr  error
	shutdowns atomic.Int32
}

func newFakeApp(name string, log *eventLog) *fakeApp {
	return &fakeApp{name: name, log: log, started: make(chan struct{})}
}

func (f *fakeApp) Start(context.Context) error {
	f.log.add(f.name + ".Start")
	return f.startErr
}

func (f *fakeApp) Serve(ctx context.Context) error {
	f.log.add(f.name + ".Serve")
	close(f.started)
	if f.serveErr != nil {
		return f.serveErr
	}
	<-ctx.Done()
	return nil
}

func (f *fakeApp) Shutdown(context.Context) error {
	f.shutdowns.Add(1)
	f.log.add(f.name + ".Shutdown")
	return nil
}

var _ supervisor.App = (*fakeApp)(nil)

// staticBuild always returns a (single app, no reload).
func staticBuild(a supervisor.App) supervisor.BuildFunc {
	return func(*config.Config) (supervisor.App, error) { return a, nil }
}

// sequentialBuild returns the apps in order, recording "build:<name>" per call.
func sequentialBuild(log *eventLog, apps ...*fakeApp) supervisor.BuildFunc {
	var mu sync.Mutex
	i := 0
	return func(*config.Config) (supervisor.App, error) {
		mu.Lock()
		defer mu.Unlock()
		a := apps[i]
		i++
		log.add("build:" + a.name)
		return a, nil
	}
}

// ---- B0: initial run + clean shutdown on an external signal ------------------

func TestSupervisor_runsInitialApp_shutsDownOnSignal(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	sig := make(chan os.Signal, 1)

	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(staticBuild(a)),
		supervisor.WithSignalChan(sig),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()

	<-a.started // initial app is serving (no sleep)

	sig <- os.Interrupt // external shutdown on the supervisor's OWN channel (F6)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on a clean signal shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not return after the shutdown signal")
	}
	if !log.contains("A.Shutdown") {
		t.Errorf("initial app was not Shutdown on signal; log=%v", log.snapshot())
	}
}

// ---- B1 (LOAD-BEARING): -race quiesce -> rebuild -> swap ---------------------

// TestSupervisor_reloadCutover_race is the load-bearing test: a full reload runs
// Shutdown(old) -> build(new) -> Start(new) -> swap -> Serve(new) with no data race.
// A goroutine reads Current()/Status() concurrently with the swap; the swap stays
// under the mutex (moving it out would trip -race). Run with -race -count=20.
func TestSupervisor_reloadCutover_race(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	sig := make(chan os.Signal, 1)

	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(sequentialBuild(log, a, b)),
		supervisor.WithSignalChan(sig),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()

	<-a.started // initial app A serving

	h, err := sup.RequestReload(&config.Config{})
	if err != nil {
		t.Fatalf("RequestReload: %v", err)
	}

	// Concurrent reader during the cutover — makes -race meaningful.
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
				_ = sup.Current()
				_ = sup.Status(h)
				runtime.Gosched()
			}
		}
	}()

	<-b.started // cutover complete: B is serving (no sleep)
	close(stop)
	<-readerDone

	log.assertOrder(t, "A.Shutdown", "build:B", "B.Start", "B.Serve")
	if sup.Current() != supervisor.App(b) {
		t.Errorf("current app after cutover is not B; log=%v", log.snapshot())
	}
	if got := sup.Status(h); got != supervisor.StateSucceeded {
		t.Errorf("reload status = %q, want %q", got, supervisor.StateSucceeded)
	}

	sig <- os.Interrupt // clean shutdown
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not return after shutdown")
	}
}

// ---- B2: single-flight (concurrent reload rejected) -------------------------

func TestSupervisor_singleFlight_rejectsConcurrentReload(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	buildEntered := make(chan struct{})
	buildGate := make(chan struct{})
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return a, nil // initial
		}
		close(buildEntered) // the reload build is now in progress (inProgress held)
		<-buildGate
		log.add("build:B")
		return b, nil
	}
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h1, err := sup.RequestReload(&config.Config{})
	if err != nil {
		t.Fatalf("first reload rejected: %v", err)
	}
	<-buildEntered

	_, err2 := sup.RequestReload(&config.Config{})
	if !errors.Is(err2, supervisor.ErrReloadInProgress) {
		t.Fatalf("second concurrent reload err = %v, want ErrReloadInProgress", err2)
	}

	close(buildGate)
	<-b.started
	if got := sup.Status(h1); got != supervisor.StateSucceeded {
		t.Errorf("first reload status = %q, want succeeded", got)
	}
	if n := log.count("build:B"); n != 1 {
		t.Errorf("cutover count = %d, want exactly 1 (the rejected reload built nothing)", n)
	}

	sig <- os.Interrupt
	<-runDone
}

// ---- B3: rollback (real contract, no self-heal, no persist on failure) -------

// (a) build(B) ok but B.Start fails -> B.Shutdown -> rollback rebuild+Start(A).
// persist is NEVER called on a failed cutover, so the -config is untouched (F5).
func TestSupervisor_rollback_onNewAppStartFailure(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.startErr = errors.New("B.Start: admin re-bind failed")
	a2 := newFakeApp("A2", log)
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			log.add("build:A")
			return a, nil
		case 2:
			log.add("build:B")
			return bFail, nil
		default:
			log.add("build:A2")
			return a2, nil
		}
	}
	var pmu sync.Mutex
	var persisted []*config.Config
	persist := func(c *config.Config) error {
		pmu.Lock()
		persisted = append(persisted, c)
		pmu.Unlock()
		return nil
	}
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithPersist(persist), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h, _ := sup.RequestReload(&config.Config{})
	<-a2.started // rollback complete: A2 serving (deterministic, no sleep)

	log.assertOrder(t, "A.Shutdown", "build:B", "B.Start", "B.Shutdown", "build:A2", "A2.Start", "A2.Serve")
	if got := sup.Status(h); got != supervisor.StateRolledBack {
		t.Errorf("status = %q, want rolled-back", got)
	}
	if sup.Current() != supervisor.App(a2) {
		t.Error("current app is not the rolled-back A2")
	}
	pmu.Lock()
	n := len(persisted)
	pmu.Unlock()
	if n != 0 {
		t.Errorf("persist called %d times on a Start-fail rollback, want 0 (F5: -config never touched on failure)", n)
	}

	sig <- os.Interrupt
	<-runDone
}

// (b) REQUIRED sub-case + the P1 GUARD: new.Start fails AND the rollback Start also
// fails -> fatal exit, and persist was NEVER called so the -config on disk stays
// good (systemd boots the last known-good config; no crash-loop, ADR §c).
func TestSupervisor_rollback_failsFatal(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.startErr = errors.New("B.Start failed")
	a2Fail := newFakeApp("A2", log)
	a2Fail.startErr = errors.New("A2.Start rollback failed")
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return a, nil
		case 2:
			return bFail, nil
		default:
			return a2Fail, nil
		}
	}
	var persistCount int32
	persist := func(*config.Config) error { atomic.AddInt32(&persistCount, 1); return nil }
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithPersist(persist), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	select {
	case err := <-runDone:
		if !errors.Is(err, a2Fail.startErr) {
			t.Fatalf("Run err = %v, want the fatal rollback-start failure (systemd backstop)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit fatally after the rollback also failed")
	}
	if n := atomic.LoadInt32(&persistCount); n != 0 {
		t.Errorf("persist called %d times on the fatal path, want 0 (P1 guard: -config must stay good)", n)
	}
}

// initial app fails to Start -> boot-fatal (no rollback target).
func TestSupervisor_initialAppStartFailure_fatal(t *testing.T) {
	log := &eventLog{}
	crash := newFakeApp("A", log)
	crash.startErr = errors.New("A.Start: boot bind failed")
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(staticBuild(crash)), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()

	select {
	case err := <-runDone:
		if !errors.Is(err, crash.startErr) {
			t.Fatalf("Run err = %v, want the fatal initial-start failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit after the initial app failed to Start")
	}
}

// a serving app crashing (Serve returns an error) with no reload in flight is not a
// cutover failure -> fatal (systemd restarts a Start-able config, no rollback loop).
func TestSupervisor_runningAppServeCrash_fatal(t *testing.T) {
	log := &eventLog{}
	crash := newFakeApp("A", log)
	crash.serveErr = errors.New("A.Serve: crashed")
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(staticBuild(crash)), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()

	select {
	case err := <-runDone:
		if !errors.Is(err, crash.serveErr) {
			t.Fatalf("Run err = %v, want the fatal serve crash", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit after the serving app crashed")
	}
}

// ---- B4: the discarded (failed) app is Shutdown exactly once (F2 leak guard) --

func TestSupervisor_discardedApp_shutdownExactlyOnce(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.startErr = errors.New("B.Start failed")
	a2 := newFakeApp("A2", log)
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return a, nil
		case 2:
			return bFail, nil
		default:
			return a2, nil
		}
	}
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	<-a2.started

	if n := bFail.shutdowns.Load(); n != 1 {
		t.Errorf("failed new app Shutdown %d times, want exactly 1 (F2: no worker/store leak)", n)
	}

	sig <- os.Interrupt
	<-runDone
}

// ---- B5: cutover-cancel vs external signal are distinguished (N2/F6) ---------

func TestSupervisor_distinguishesCutoverFromSignal(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(sequentialBuild(log, a, b)), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	// (a) a reload is a CUTOVER, not a shutdown: the supervisor keeps running.
	_, _ = sup.RequestReload(&config.Config{})
	<-b.started
	select {
	case <-runDone:
		t.Fatal("supervisor exited on a reload; a cutover must not be treated as a shutdown")
	default: // still running — correct
	}

	// (b) an external signal on the supervisor's OWN channel is a shutdown: it exits.
	sig <- os.Interrupt
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run err on signal = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit on the external signal")
	}
	if !log.contains("B.Shutdown") {
		t.Error("the current app was not shut down on the external signal")
	}
}

// ---- B6: persist ONLY after a confirmed Start (F5) --------------------------

func TestSupervisor_persistOnceAfterConfirmedStart(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	var persistCount int32
	persist := func(*config.Config) error { atomic.AddInt32(&persistCount, 1); return nil }
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(sequentialBuild(log, a, b)),
		supervisor.WithPersist(persist),
		supervisor.WithSignalChan(sig),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	<-b.started

	if n := atomic.LoadInt32(&persistCount); n != 1 {
		t.Errorf("persist called %d times on a confirmed cutover, want exactly 1", n)
	}

	sig <- os.Interrupt
	<-runDone
}

func TestSupervisor_noPersistOnBuildFailure(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	a2 := newFakeApp("A2", log)
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return a, nil
		case 2:
			return nil, errors.New("cutover build failed")
		default:
			return a2, nil
		}
	}
	var persistCount int32
	persist := func(*config.Config) error { atomic.AddInt32(&persistCount, 1); return nil }
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithPersist(persist), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	<-a2.started

	if n := atomic.LoadInt32(&persistCount); n != 0 {
		t.Errorf("persist called %d times on a build-fail cutover, want 0 (F5)", n)
	}

	sig <- os.Interrupt
	<-runDone
}

// The ADR §c case: the new app builds fine but its Start fails (admin re-bind).
// persist must NOT run, so the -config is never overwritten with a config that
// cannot come up.
func TestSupervisor_noPersistOnStartFailure(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.startErr = errors.New("B.Start: admin re-bind failed")
	a2 := newFakeApp("A2", log)
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return a, nil
		case 2:
			return bFail, nil
		default:
			return a2, nil
		}
	}
	var persistCount int32
	persist := func(*config.Config) error { atomic.AddInt32(&persistCount, 1); return nil }
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithPersist(persist), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	<-a2.started

	if n := atomic.LoadInt32(&persistCount); n != 0 {
		t.Errorf("persist called %d times on a Start-fail cutover, want 0 (ADR §c)", n)
	}

	sig <- os.Interrupt
	<-runDone
}

// ---- WithLogger: a persist failure after a confirmed cutover is logged, not fatal.

func TestSupervisor_persistError_loggedNotFatal(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	var lb syncBuf
	logger := slog.New(slog.NewTextHandler(&lb, nil))
	persist := func(*config.Config) error { return errors.New("disk full") }
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(sequentialBuild(log, a, b)),
		supervisor.WithPersist(persist),
		supervisor.WithLogger(logger),
		supervisor.WithSignalChan(sig),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	<-b.started // B is serving -> persist already ran and failed -> was logged

	if !strings.Contains(lb.String(), "persist") {
		t.Errorf("persist failure was not logged; log=%q", lb.String())
	}
	select {
	case <-runDone:
		t.Fatal("supervisor exited on a persist error; a persist failure must not be fatal (the new app is serving)")
	default: // still serving — correct
	}

	sig <- os.Interrupt
	<-runDone
}

// ---- B7: status handle transitions and survives the cutover (F4) ------------

func TestSupervisor_statusHandle_transitionsAndSurvivesCutover(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	buildEntered := make(chan struct{})
	buildGate := make(chan struct{})
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return a, nil
		}
		close(buildEntered)
		<-buildGate
		return b, nil
	}
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h, _ := sup.RequestReload(&config.Config{})
	<-buildEntered
	if got := sup.Status(h); got != supervisor.StateCutoverInProgress {
		t.Errorf("mid-cutover status = %q, want cutover-in-progress", got)
	}

	close(buildGate)
	<-b.started
	if got := sup.Status(h); got != supervisor.StateSucceeded {
		t.Errorf("post-cutover status = %q, want succeeded (state survives the cutover)", got)
	}
	if got := sup.Status(supervisor.Handle("does-not-exist")); got != "" {
		t.Errorf("unknown handle status = %q, want empty", got)
	}

	sig <- os.Interrupt
	<-runDone
}

// ---- B8: atomic config persist helper ---------------------------------------

func TestWriteConfigAtomic_writesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "korvun.json")
	cfg := &config.Config{
		Channels: []config.ChannelConfig{{Type: "telegram", Mode: "polling", TokenEnv: "KORVUN_TOKEN"}},
	}
	if err := supervisor.WriteConfigAtomic(path, cfg); err != nil {
		t.Fatalf("WriteConfigAtomic: %v", err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // reads a file just written in t.TempDir
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	var got config.Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("written config is not valid JSON: %v", err)
	}
	if len(got.Channels) != 1 || got.Channels[0].TokenEnv != "KORVUN_TOKEN" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".korvun-config-") {
			t.Errorf("leftover temp file %q — write was not clean", e.Name())
		}
	}
}

func TestWriteConfigAtomic_failureLeavesNoConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-subdir", "korvun.json")
	err := supervisor.WriteConfigAtomic(path, &config.Config{})
	if err == nil {
		t.Fatal("WriteConfigAtomic into a nonexistent dir returned nil, want an error")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("a failed atomic write left a -config at %q", path)
	}
}
