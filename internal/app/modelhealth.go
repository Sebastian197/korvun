// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import "sync"

// Model health states surfaced through BrainSummaries (N6, bug-bash
// 2026-08-23): the warmup outcome used to die as a WARN in the log, leaving
// the UI blind to a model that would fail on first use.
const (
	healthUnknown     = "unknown"
	healthWarming     = "warming"
	healthReady       = "ready"
	healthUnreachable = "unreachable"
)

// modelHealthState is one model backend's last observed liveness.
type modelHealthState struct {
	health string
	detail string
}

// modelHealthRegistry records the last observed liveness per model identity
// (provider + model id — the identity BrainSummaries exposes; two backends
// sharing that identity share the entry, last write wins). Warmup goroutines
// write concurrently with control-API reads, so access is mutex-guarded.
type modelHealthRegistry struct {
	mu     sync.Mutex
	states map[string]modelHealthState
}

func newModelHealthRegistry() *modelHealthRegistry {
	return &modelHealthRegistry{states: make(map[string]modelHealthState)}
}

func healthKey(provider, modelID string) string {
	return provider + "\x00" + modelID
}

func (r *modelHealthRegistry) set(provider, modelID, health, detail string) {
	r.mu.Lock()
	r.states[healthKey(provider, modelID)] = modelHealthState{health: health, detail: detail}
	r.mu.Unlock()
}

// get returns the recorded state, or an honest "unknown" for a model that was
// never probed. Nil-receiver safe: a hand-built App without a registry reads
// as all-unknown instead of panicking.
func (r *modelHealthRegistry) get(provider, modelID string) modelHealthState {
	if r == nil {
		return modelHealthState{health: healthUnknown}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.states[healthKey(provider, modelID)]; ok {
		return st
	}
	return modelHealthState{health: healthUnknown}
}
