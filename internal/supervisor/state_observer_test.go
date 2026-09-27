// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the state observer (D1 of the store-and-act-close plan).
//
// The app that seals an operator act is shut down BEFORE a cutover's state turns
// terminal, and the screen polls the status door once. So the process itself
// must learn every transition without a poll: this seam is how. It is a mould of
// ORDER and of SYNCHRONY — the observer runs before `setStatus` returns, so what
// it sees is already what `Status` answers.
//
// Evidence level, honest: in-process, deterministic fakes, no app and no ledger.
//
// Plan: docs/superpowers/specs/2026-09-24-v0162-el-almacen-y-el-cierre-del-acto-pretest.md, D1.

package supervisor_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// seenStates records every (handle, state) the observer is handed, and what
// Status answered at that very moment.
type seenStates struct {
	mu   sync.Mutex
	seen []string
}

func (s *seenStates) observer(sup **supervisor.Supervisor) func(supervisor.Handle, supervisor.State) {
	return func(h supervisor.Handle, st supervisor.State) {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Synchrony: the state the observer is told is the state Status
		// already answers. An observer told a state before it was stored would
		// let a settler act on a fact the door cannot yet confirm.
		if got := (*sup).Status(h); got != st {
			s.seen = append(s.seen, string(h)+":"+string(st)+"!=status:"+string(got))
			return
		}
		s.seen = append(s.seen, string(h)+":"+string(st))
	}
}

func (s *seenStates) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func containsInOrder(got []string, want ...string) bool {
	i := 0
	for _, e := range got {
		if i < len(want) && e == want[i] {
			i++
		}
	}
	return i == len(want)
}

// TestSupervisor_stateObserverSeesEveryTransition: pending, cutover-in-progress
// and succeeded, in order, each one synchronous with the stored state.
//
// PROBING MUTATION: do not call the observer from `setStatus`. The observer sees
// only `pending` (from RequestReload) and this reddens on the order.
func TestSupervisor_stateObserverSeesEveryTransition(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	seen := &seenStates{}
	var sup *supervisor.Supervisor
	sig := make(chan os.Signal, 1)
	sup = supervisor.New(&config.Config{},
		supervisor.WithBuild(sequentialBuild(log, a, b)),
		supervisor.WithSignalChan(sig),
		supervisor.WithStateObserver(seen.observer(&sup)),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h, err := sup.RequestReload(&config.Config{})
	if err != nil {
		t.Fatalf("RequestReload: %v", err)
	}
	<-b.started
	waitStatus(t, sup, h, supervisor.StateSucceeded)

	got := seen.snapshot()
	want := []string{string(h) + ":pending", string(h) + ":cutover-in-progress", string(h) + ":succeeded"}
	if !containsInOrder(got, want...) {
		t.Fatalf("the observer saw %v, want the subsequence %v", got, want)
	}
	for _, e := range got {
		if len(e) > 0 && (e[len(e)-1] == ')' || containsBang(e)) {
			t.Fatalf("the observer was told a state before Status stored it: %q", e)
		}
	}

	sig <- os.Interrupt
	<-runDone
}

func containsBang(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '!' {
			return true
		}
	}
	return false
}

// TestSupervisor_stateObserverSeesTheTerminalFailures: a preflight failure
// (`failed`, old app still serving) and a rollback (`rolled-back`, old app
// already shut down) both reach the observer. These are the two states a settler
// must close an act as NOT applied on, and one of them is emitted with no app
// alive at all.
//
// PROBING MUTATION: call the observer only on `succeeded`. Both subtests redden.
func TestSupervisor_stateObserverSeesTheTerminalFailures(t *testing.T) {
	t.Run("preflight failure", func(t *testing.T) {
		log := &eventLog{}
		a := newFakeApp("A", log)
		seen := &seenStates{}
		var sup *supervisor.Supervisor
		sig := make(chan os.Signal, 1)
		sup = supervisor.New(&config.Config{},
			supervisor.WithBuild(staticBuild(a)),
			supervisor.WithPreflight(func(*config.Config) error { return errors.New("bad config") }),
			supervisor.WithSignalChan(sig),
			supervisor.WithStateObserver(seen.observer(&sup)),
		)
		runDone := make(chan error, 1)
		go func() { runDone <- sup.Run(context.Background()) }()
		<-a.started

		h, _ := sup.RequestReload(&config.Config{})
		waitStatus(t, sup, h, supervisor.StateFailed)
		if got := seen.snapshot(); !containsInOrder(got, string(h)+":pending", string(h)+":failed") {
			t.Fatalf("the observer saw %v, want pending then failed", got)
		}
		sig <- os.Interrupt
		<-runDone
	})

	t.Run("rollback", func(t *testing.T) {
		log := &eventLog{}
		a := newFakeApp("A", log)
		b := newFakeApp("B", log)
		b.startErr = errors.New("admin re-bind failed")
		a2 := newFakeApp("A2", log)
		seen := &seenStates{}
		var sup *supervisor.Supervisor
		sig := make(chan os.Signal, 1)
		sup = supervisor.New(&config.Config{},
			supervisor.WithBuild(sequentialBuild(log, a, b, a2)),
			supervisor.WithSignalChan(sig),
			supervisor.WithStateObserver(seen.observer(&sup)),
		)
		runDone := make(chan error, 1)
		go func() { runDone <- sup.Run(context.Background()) }()
		<-a.started

		h, _ := sup.RequestReload(&config.Config{})
		<-a2.started
		waitStatus(t, sup, h, supervisor.StateRolledBack)
		if got := seen.snapshot(); !containsInOrder(got, string(h)+":pending", string(h)+":cutover-in-progress", string(h)+":rolled-back") {
			t.Fatalf("the observer saw %v, want pending, cutover-in-progress, rolled-back", got)
		}
		// And it was told BEFORE the rollback app served — the state is emitted
		// with no app alive, which is exactly why a settler cannot lean on one.
		if !log.contains("A.Shutdown") {
			t.Fatal("the old app was not shut down before the rollback state was emitted")
		}
		sig <- os.Interrupt
		<-runDone
	})
}
