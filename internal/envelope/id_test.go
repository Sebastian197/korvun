// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"sync"
	"testing"
)

func TestNewID_uniqueness(t *testing.T) {
	const n = 1000
	ids := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewID()
		if id == "" {
			t.Fatal("NewID() returned empty string")
		}
		if _, exists := ids[id]; exists {
			t.Fatalf("NewID() collision at iteration %d: %q", i, id)
		}
		ids[id] = struct{}{}
	}
}

func TestNewID_concurrent_uniqueness(t *testing.T) {
	const goroutines = 10
	const perGoroutine = 100

	var mu sync.Mutex
	ids := make(map[string]struct{}, goroutines*perGoroutine)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			local := make([]string, 0, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				local = append(local, NewID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, exists := ids[id]; exists {
					t.Errorf("concurrent NewID() collision: %q", id)
					return
				}
				ids[id] = struct{}{}
			}
		}()
	}
	wg.Wait()
}

func TestNewID_sortable_by_time(t *testing.T) {
	id1 := NewID()
	id2 := NewID()
	if id1 >= id2 {
		t.Errorf("NewID() should be time-sortable: %q should be < %q", id1, id2)
	}
}
