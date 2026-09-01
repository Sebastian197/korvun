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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// B7 (bug-bash 2026-08-23): the 09:40:44 incident — a reload the UI painted as
// failed — left ZERO trace in the desktop file log, because the supervisor
// never logged its reload state transitions. Diagnosis discipline demands the
// primary evidence exist: every transition of a reload handle must land in the
// structured log, so the NEXT incident is diagnosable from the profile log
// alone.

// syncBuffer serializes writes so the slog handler and the test reader are
// -race clean.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(strings.TrimSpace(s.b.String()), "\n")
}

// reloadStates extracts the ordered `state` values of "reload state" records
// for one handle from a JSON slog stream.
func reloadStates(t *testing.T, buf *syncBuffer, handle supervisor.Handle) []string {
	t.Helper()
	var states []string
	for _, line := range buf.lines() {
		if line == "" {
			continue
		}
		var rec struct {
			Msg    string `json:"msg"`
			Handle string `json:"handle"`
			State  string `json:"state"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unparseable slog line %q: %v", line, err)
		}
		if rec.Msg == "reload state" && rec.Handle == string(handle) {
			states = append(states, rec.State)
		}
	}
	return states
}

func TestSupervisor_reloadStateTransitions_areLogged(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	log := &eventLog{}
	a := newFakeApp("A", log)
	b := newFakeApp("B", log)
	sig := make(chan os.Signal, 1)

	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(sequentialBuild(log, a, b)),
		supervisor.WithLogger(logger),
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

	// The happy cutover's full trail: pending → cutover-in-progress → succeeded.
	deadline := time.Now().Add(2 * time.Second)
	for sup.Status(h) != supervisor.StateSucceeded && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := sup.Status(h); got != supervisor.StateSucceeded {
		t.Fatalf("reload status = %q, want succeeded", got)
	}
	want := []string{"pending", "cutover-in-progress", "succeeded"}
	if got := reloadStates(t, buf, h); !equalStrings(got, want) {
		t.Fatalf("logged reload states = %v, want %v", got, want)
	}

	sig <- os.Interrupt
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not return after shutdown")
	}
}

func TestSupervisor_preflightRejection_logsFailedState(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	log := &eventLog{}
	a := newFakeApp("A", log)
	sig := make(chan os.Signal, 1)

	sup := supervisor.New(&config.Config{},
		supervisor.WithBuild(staticBuild(a)),
		supervisor.WithPreflight(func(*config.Config) error { return errors.New("rejected") }),
		supervisor.WithLogger(logger),
		supervisor.WithSignalChan(sig),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- sup.Run(context.Background()) }()
	<-a.started

	h, err := sup.RequestReload(&config.Config{})
	if err != nil {
		t.Fatalf("RequestReload: %v", err)
	}
	// HARDENED (the third of the timing-observability family, after the
	// Windows first-run deadlines and the SSE frame order): the old poll
	// watched Status(h) but the assert read the LOG BUFFER — and the
	// supervisor publishes the state BEFORE emitting the log line, so a
	// slow runner could exit the poll inside that window and read only
	// [pending] (seen once on windows-latest; does not reproduce locally
	// — 20 -race runs green). The poll now waits for the REAL object of
	// the assert: the log itself containing the failed state, under a
	// generous deadline. Do not "simplify" this back to polling Status.
	want := []string{"pending", "failed"}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if got := reloadStates(t, buf, h); equalStrings(got, want) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := sup.Status(h); got != supervisor.StateFailed {
		t.Fatalf("reload status = %q, want failed", got)
	}
	if got := reloadStates(t, buf, h); !equalStrings(got, want) {
		t.Fatalf("logged reload states = %v, want %v", got, want)
	}

	sig <- os.Interrupt
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not return after shutdown")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
