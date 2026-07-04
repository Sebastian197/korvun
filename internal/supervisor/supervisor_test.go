// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package supervisor_test

import (
	"context"
	"os"
	"runtime"
	"sync"
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

// fakeApp is a deterministic App: Run records "<name>.Run", closes started, then
// blocks until its context is cancelled (mirroring app.Run's <-ctx.Done()).
// Shutdown records "<name>.Shutdown".
type fakeApp struct {
	name    string
	log     *eventLog
	started chan struct{}
}

func newFakeApp(name string, log *eventLog) *fakeApp {
	return &fakeApp{name: name, log: log, started: make(chan struct{})}
}

func (f *fakeApp) Run(ctx context.Context) error {
	f.log.add(f.name + ".Run")
	close(f.started)
	<-ctx.Done()
	return nil
}

func (f *fakeApp) Shutdown(context.Context) error {
	f.log.add(f.name + ".Shutdown")
	return nil
}

var _ supervisor.App = (*fakeApp)(nil)

// staticBuild always returns a (B0: one app, no reload).
func staticBuild(a supervisor.App) supervisor.BuildFunc {
	return func(*config.Config) (supervisor.App, error) { return a, nil }
}

// sequentialBuild returns the apps in order, recording "build:<name>" per call
// (B1: A for the initial build, B for the reload).
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
