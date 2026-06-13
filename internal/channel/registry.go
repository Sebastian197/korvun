// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package channel

import "sync"

// Registry manages a set of active channels, allowing registration,
// lookup, removal, and listing. It is safe for concurrent use.
type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

// NewRegistry creates an empty channel registry.
func NewRegistry() *Registry {
	return &Registry{
		channels: make(map[string]Channel),
	}
}

// Register adds or replaces a channel in the registry.
func (r *Registry) Register(ch Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[ch.Name()] = ch
}

// Unregister removes a channel from the registry by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, name)
}

// Get returns the channel with the given name and true, or nil and false
// if not found.
func (r *Registry) Get(name string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.channels[name]
	return ch, ok
}

// List returns the names of all registered channels.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}
