// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Red for the 2026-08-08 rehearsal flake (macOS quality:
// TestReload_pristinePersistAndAddrRotation): the supervisor reported
// StateSucceeded BEFORE persistConfig wrote the file, so a poller that
// sees "succeeded" can legitimately read a config the persist has not
// reached yet. The contract fixed here: success is never reported until
// the persist has completed, and a persist failure is reported honestly
// as StatePersistFailed — never as succeeded (the new app IS serving,
// but the reload would not survive a restart, and the state says so).
package supervisor_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// waitState polls the handle until it reaches want or the deadline expires.
func waitState(t *testing.T, sup *supervisor.Supervisor, h supervisor.Handle, want supervisor.State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last supervisor.State
	for time.Now().Before(deadline) {
		last = sup.Status(h)
		if last == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("reload state = %q after deadline, want %q", last, want)
}

func TestSupervisor_neverSucceededBeforePersistCompletes(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	entered := make(chan struct{})
	release := make(chan struct{})
	persist := func(*config.Config) error {
		close(entered)
		<-release
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

	h, err := sup.RequestReload(&config.Config{})
	if err != nil {
		t.Fatalf("RequestReload: %v", err)
	}

	// Persist is in flight (entered, not released): the handle must NOT
	// read succeeded yet — that is the exact window the macOS run lost.
	<-entered
	if st := sup.Status(h); st == supervisor.StateSucceeded {
		t.Error("reload reported succeeded while persist was still in flight")
	}
	close(release)
	waitState(t, sup, h, supervisor.StateSucceeded)

	sig <- os.Interrupt
	<-runDone
}

func TestSupervisor_persistFailureNeverReportsSucceeded(t *testing.T) {
	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	persist := func(*config.Config) error { return errors.New("disk full") }
	sig := make(chan os.Signal, 1)
	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(sequentialBuild(log, a, b)),
		supervisor.WithPersist(persist),
		supervisor.WithSignalChan(sig),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h, err := sup.RequestReload(&config.Config{})
	if err != nil {
		t.Fatalf("RequestReload: %v", err)
	}
	<-b.started

	// The cutover is live (B serves) but the on-disk record is NOT: the
	// state must say persist-failed, never succeeded.
	waitState(t, sup, h, supervisor.StatePersistFailed)

	sig <- os.Interrupt
	<-runDone
}
