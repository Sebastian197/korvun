// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
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

// fakeApp is a deterministic App. Run records "<name>.Run", closes started, then
// either returns runErr (a failed Run) or blocks until its context is cancelled
// (mirroring app.Run's <-ctx.Done()). Shutdown records "<name>.Shutdown" and counts.
type fakeApp struct {
	name      string
	log       *eventLog
	started   chan struct{}
	runErr    error
	shutdowns atomic.Int32
}

func newFakeApp(name string, log *eventLog) *fakeApp {
	return &fakeApp{name: name, log: log, started: make(chan struct{})}
}

func (f *fakeApp) Run(ctx context.Context) error {
	f.log.add(f.name + ".Run")
	close(f.started)
	if f.runErr != nil {
		return f.runErr
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

// staticBuild always returns a (B0: one app, no reload).
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

	<-a.started // initial app is running (no sleep)

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

// TestSupervisor_reloadCutover_race is the load-bearing test of the stage: a full
// reload must run Shutdown(old) -> build(new) -> Run(new) -> swap with no data
// race. A goroutine reads Current()/Status() concurrently with the swap so -race
// exercises the reference hand-off. Run with -race -count=20.
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

	<-a.started // initial app A running

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

	<-b.started // cutover complete: B is running (no sleep)
	close(stop)
	<-readerDone

	log.assertOrder(t, "A.Shutdown", "build:B", "B.Run")
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

// ---- B3: rollback (real contract, not over-promised) ------------------------

// (a) build(B) ok but B.Run fails -> B.Shutdown -> rollback rebuild+Run(A). The
// persist recorder also proves the crash-loop guard: the on-disk config self-heals
// to the good (rollback) config, never left at the config whose app failed to run
// (ADR-0027 §c).
func TestSupervisor_rollback_onNewAppRunFailure(t *testing.T) {
	log := &eventLog{}
	cfgA := &config.Config{} // initial / rollback target (good)
	cfgB := &config.Config{} // reloaded config whose app fails to run
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.runErr = errors.New("B.Run: admin re-bind failed")
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
	sup := supervisor.New(cfgA, supervisor.WithBuild(build), supervisor.WithPersist(persist), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h, _ := sup.RequestReload(cfgB)
	<-a2.started // rollback complete: A2 serving (deterministic, no sleep)

	log.assertOrder(t, "A.Shutdown", "build:B", "B.Run", "B.Shutdown", "build:A2", "A2.Run")
	if got := sup.Status(h); got != supervisor.StateRolledBack {
		t.Errorf("status = %q, want rolled-back", got)
	}
	if sup.Current() != supervisor.App(a2) {
		t.Error("current app is not the rolled-back A2")
	}
	// Crash-loop guard: whatever was persisted, the LAST write is the good config,
	// so a restart boots the config that actually runs — never cfgB.
	pmu.Lock()
	last := persisted[len(persisted)-1]
	pmu.Unlock()
	if last != cfgA {
		t.Error("after a run-fail rollback the on-disk config is not the good one (crash-loop risk)")
	}

	sig <- os.Interrupt
	<-runDone
}

// The initial (or an already-rolled-back) app crashing is NOT a cutover failure:
// there is nothing better to roll back to, so the supervisor exits fatally and lets
// systemd restart (no infinite rollback loop).
func TestSupervisor_initialAppFailure_fatal(t *testing.T) {
	log := &eventLog{}
	crash := newFakeApp("A", log)
	crash.runErr = errors.New("A.Run: boot serve failed")
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(staticBuild(crash)), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()

	select {
	case err := <-runDone:
		if !errors.Is(err, crash.runErr) {
			t.Fatalf("Run err = %v, want the fatal app-run failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit after the initial app failed to run")
	}
}

// (b) REQUIRED sub-case: rollback ALSO fails -> fatal exit (systemd backstop; the
// supervisor does NOT pretend to recover).
func TestSupervisor_rollback_failsFatal(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.runErr = errors.New("B.Run failed")
	rollbackErr := errors.New("rollback build failed")
	var calls int32
	build := func(*config.Config) (supervisor.App, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return a, nil
		case 2:
			return bFail, nil
		default:
			return nil, rollbackErr
		}
	}
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{}, supervisor.WithBuild(build), supervisor.WithSignalChan(sig))
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	_, _ = sup.RequestReload(&config.Config{})
	select {
	case err := <-runDone:
		if !errors.Is(err, rollbackErr) {
			t.Fatalf("Run err = %v, want the fatal rollback failure (systemd backstop)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not exit fatally after the rollback also failed")
	}
}

// ---- B4: the discarded (failed) app is Shutdown exactly once (F2 leak guard) --

func TestSupervisor_discardedApp_shutdownExactlyOnce(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	bFail := newFakeApp("B", log)
	bFail.runErr = errors.New("B.Run failed")
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

// ---- B6: persist ONLY after a successful cutover (F5) ------------------------

func TestSupervisor_persistOnceAfterSuccessfulCutover(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	var pmu sync.Mutex
	var persisted []*config.Config
	persist := func(c *config.Config) error {
		pmu.Lock()
		persisted = append(persisted, c)
		pmu.Unlock()
		return nil
	}
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

	pmu.Lock()
	n := len(persisted)
	pmu.Unlock()
	if n != 1 {
		t.Errorf("persist called %d times on a successful cutover, want exactly 1", n)
	}

	sig <- os.Interrupt
	<-runDone
}

func TestSupervisor_noPersistOnFailedCutover(t *testing.T) {
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
	<-a2.started // rollback (rebuild A2) complete

	if n := atomic.LoadInt32(&persistCount); n != 0 {
		t.Errorf("persist called %d times on a failed cutover, want 0 (F5: -config not overwritten)", n)
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
	// The parent dir does not exist, so the temp file cannot be created: the write
	// fails before any rename, leaving no -config half-written.
	path := filepath.Join(dir, "missing-subdir", "korvun.json")
	err := supervisor.WriteConfigAtomic(path, &config.Config{})
	if err == nil {
		t.Fatal("WriteConfigAtomic into a nonexistent dir returned nil, want an error")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("a failed atomic write left a -config at %q", path)
	}
}
