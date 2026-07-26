// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This file is the SP2 TDD contract for the webhook channel's own HTTP-server
// lifecycle (ADR-0038 §2). It is written RED-first: it references the surface the
// implementation must add — Options, NewWithOptions, Start, Stop, BoundAddr — none
// of which exist yet, so the webhook package test binary does not compile until
// lifecycle.go lands. The existing New(name, mapping) constructor stays INTACT
// (FR-COMPAT-1): this file never touches it. Explicitly NOT pinned here (SP3/SP4):
// negative auth (401), saturation/drops, conversation.id, edge validation.

const testSecret = "s3cr3t-inbound"

// newTestAdapter builds an SP2 webhook adapter bound to an ephemeral loopback port
// (the house policy — never a fixed port in tests), with the canonical mapping so a
// posted payload's keys line up. path == "" exercises the default-path contract.
func newTestAdapter(t *testing.T, path string) *Adapter {
	t.Helper()
	return NewWithOptions("test-webhook", Options{
		Bind:        "127.0.0.1:0",
		Path:        path,
		Secret:      testSecret,
		OutboundURL: "https://downstream.example/in",
		Mapping:     defaultMapping(),
	})
}

// waitGoroutinesAtMost polls until the goroutine count settles at or below want,
// returning the final count (leak tripwire helper; no external deps).
func waitGoroutinesAtMost(want int, within time.Duration) int {
	deadline := time.Now().Add(within)
	n := runtime.NumGoroutine()
	for n > want && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

// settledGoroutines samples until the count holds steady for a few consecutive
// samples (or the deadline hits) — the baseline capture for the leak tripwire.
func settledGoroutines(within time.Duration) int {
	deadline := time.Now().Add(within)
	last := runtime.NumGoroutine()
	stable := 0
	for stable < 3 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n == last {
			stable++
		} else {
			stable = 0
			last = n
		}
	}
	return last
}

// postJSON posts a payload to url with the Bearer header set. Auth is NOT enforced
// until SP3; the header already travels here so the SP3 change never has to rewrite
// these tests (FR-AUTH-1 groundwork).
func postJSON(t *testing.T, url string, payload map[string]string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testSecret)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// TestLifecycle_startHappy pins AS-2: Start brings the server up, BoundAddr reports
// the real ephemeral address, and /healthz returns 200 while running.
func TestLifecycle_startHappy(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	addr := a.BoundAddr()
	if addr == "" {
		t.Fatal("BoundAddr() is empty after a successful Start")
	}

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200", resp.StatusCode)
	}
}

// TestLifecycle_pathMount pins AS-5 (inbound leg only): the InboundHandler is mounted
// at the configured path, a valid authenticated POST returns 200, and the Envelope
// surfaces on Inbound() with the mapped sender and text. Two cases: a custom path and
// the default path (Options.Path empty → /webhook).
func TestLifecycle_pathMount(t *testing.T) {
	cases := []struct {
		name     string
		optsPath string
		wantPath string
	}{
		{name: "custom path", optsPath: "/hook", wantPath: "/hook"},
		{name: "default path", optsPath: "", wantPath: "/webhook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAdapter(t, tc.optsPath)
			if err := a.Start(context.Background()); err != nil {
				t.Fatalf("Start() error: %v", err)
			}
			defer func() { _ = a.Stop(context.Background()) }()

			url := "http://" + a.BoundAddr() + tc.wantPath
			resp := postJSON(t, url, map[string]string{
				"sender_id":   "user-1",
				"sender_name": "Alice",
				"text":        "hello from webhook",
			})
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("POST status = %d, want 200", resp.StatusCode)
			}

			select {
			case env := <-a.Inbound():
				if env.Sender.ID != "user-1" {
					t.Errorf("Sender.ID = %q, want %q", env.Sender.ID, "user-1")
				}
				if env.Sender.Name != "Alice" {
					t.Errorf("Sender.Name = %q, want %q", env.Sender.Name, "Alice")
				}
				if len(env.Parts) != 1 || env.Parts[0].Content != "hello from webhook" {
					t.Errorf("Parts = %+v, want text 'hello from webhook'", env.Parts)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the Envelope on Inbound()")
			}
		})
	}
}

// TestLifecycle_startAllOrNothing pins FR-LIFECYCLE-1: a bind failure (the port is
// already taken) makes Start return a named, wrapped error, leaves the adapter
// un-started (BoundAddr empty), a later Stop is a no-op without panic, and no
// goroutine is left running.
func TestLifecycle_startAllOrNothing(t *testing.T) {
	base := settledGoroutines(2 * time.Second)

	// Occupy an ephemeral port, then hand its address to the adapter so its bind
	// collides.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pre-bind listener: %v", err)
	}
	defer func() { _ = ln.Close() }()
	taken := ln.Addr().String()

	a := NewWithOptions("test-webhook", Options{
		Bind:        taken,
		Path:        "/hook",
		Secret:      testSecret,
		OutboundURL: "https://downstream.example/in",
		Mapping:     defaultMapping(),
	})

	if err := a.Start(context.Background()); err == nil {
		_ = a.Stop(context.Background())
		t.Fatal("Start() on an occupied port should fail")
	} else if !strings.Contains(err.Error(), "webhook") {
		t.Errorf("Start() error %q does not name the channel", err.Error())
	}
	if addr := a.BoundAddr(); addr != "" {
		t.Errorf("BoundAddr() = %q after a failed Start, want empty (un-started)", addr)
	}
	// A Stop after a failed Start must be a safe no-op.
	if err := a.Stop(context.Background()); err != nil {
		t.Errorf("Stop() after failed Start returned %v, want nil", err)
	}
	if n := waitGoroutinesAtMost(base, 2*time.Second); n > base {
		t.Errorf("goroutines after failed Start = %d, want <= baseline %d", n, base)
	}
}

// TestLifecycle_stopIdempotentAndBounded pins FR-LIFECYCLE-1's Stop contract: a
// double Stop neither errors nor panics; a Stop with an already-cancelled ctx returns
// without hanging; and Inbound() is closed exactly once (a range over it terminates,
// and the second Stop does not re-close it — a double close would panic).
func TestLifecycle_stopIdempotentAndBounded(t *testing.T) {
	a := newTestAdapter(t, "/hook")
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop() error: %v", err)
	}

	// Second Stop with an already-cancelled ctx: idempotent, bounded, no panic.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- a.Stop(cancelled) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("second Stop() (cancelled ctx) returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second Stop() hung on an already-cancelled ctx")
	}

	// Inbound() is closed exactly once: the range terminates (closed), and the two
	// Stops above did not double-close (no panic reached here).
	for range a.Inbound() {
		t.Fatal("unexpected Envelope on a drained, closed Inbound()")
	}
}

// TestLifecycle_tenCycleNoLeak is the house ten-cycle tripwire: 10 Start/Stop cycles
// in a row must leave the goroutine count stable at the end.
func TestLifecycle_tenCycleNoLeak(t *testing.T) {
	base := settledGoroutines(2 * time.Second)
	for i := 0; i < 10; i++ {
		a := newTestAdapter(t, "/hook")
		if err := a.Start(context.Background()); err != nil {
			t.Fatalf("cycle %d: Start() error: %v", i, err)
		}
		if err := a.Stop(context.Background()); err != nil {
			t.Fatalf("cycle %d: Stop() error: %v", i, err)
		}
	}
	if n := waitGoroutinesAtMost(base, 2*time.Second); n > base {
		t.Errorf("goroutines after 10 cycles = %d, want <= baseline %d", n, base)
	}
}
