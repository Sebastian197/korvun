// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/policy"
)

// This file backs the /tools chat command (ADR-0041 FR-CHAT-1): a bounded
// in-memory ring of recent tool events fed by the bus, plus the gatekeeper
// report renderer. METADATA-ONLY BY LAW (ADR-0024 §1): the ring stores
// bus.Event values — which carry no args field by construction — and the
// report renders grants and event metadata only. Nothing is persisted
// (ADR-0021 §6: observability, not memory).

// defaultToolRingSize bounds the ring; defaultToolReportEntries is how many
// recent entries one report shows.
const (
	defaultToolRingSize      = 64
	defaultToolReportEntries = 10
)

// ringEntry is one recorded tool event with the ring's own receive stamp
// (the bus Event carries no timestamp; the stamp is system metadata).
type ringEntry struct {
	at time.Time
	ev bus.Event
}

// toolEventRing is a fixed-size ring of recent tool events, safe for
// concurrent record/read (the bus handler goroutine vs. the /tools dispatch
// path).
type toolEventRing struct {
	mu   sync.Mutex
	buf  []ringEntry
	next int
	full bool
	now  func() time.Time
}

func newToolEventRing(size int, now func() time.Time) *toolEventRing {
	if size <= 0 {
		size = defaultToolRingSize
	}
	if now == nil {
		now = time.Now
	}
	return &toolEventRing{buf: make([]ringEntry, size), now: now}
}

// subscribe registers the ring on the bus for the three tool event types.
// Unsubscription rides bus.Close at shutdown (the app owns the bus).
func (r *toolEventRing) subscribe(b *bus.InMemoryBus) {
	for _, t := range []bus.EventType{bus.ToolUsed, bus.ToolDenied, bus.ToolShadowed} {
		b.Subscribe(t, r.record)
	}
}

// record appends one event, displacing the oldest when full.
func (r *toolEventRing) record(ev bus.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = ringEntry{at: r.now(), ev: ev}
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.full = true
	}
}

// lastFor returns up to k most-recent entries for the given brain, newest
// last.
func (r *toolEventRing) lastFor(brain string, k int) []ringEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.next
	if r.full {
		n = len(r.buf)
	}
	// Walk oldest→newest in ring order.
	var out []ringEntry
	start := 0
	if r.full {
		start = r.next
	}
	for i := 0; i < n; i++ {
		e := r.buf[(start+i)%len(r.buf)]
		if e.ev.Brain == brain {
			out = append(out, e)
		}
	}
	if len(out) > k {
		out = out[len(out)-k:]
	}
	return out
}

// gatekeeperReport renders the /tools system response for one brain
// (FR-CHAT-1): the effective grants (mode, channel restriction, declared
// attrs, shield) and the recent tool activity from the ring — metadata only.
func gatekeeperReport(bc config.BrainConfig, ring *toolEventRing) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Gatekeeper — brain %q (%s)\n", bc.Name, bc.Sensitivity)

	if bc.Agent == nil {
		b.WriteString("This brain has no tools (not an agent brain).\n")
		return b.String()
	}

	attrs, err := effectiveToolAttrs(bc.Agent)
	if err != nil {
		// wire() already failed loud on this; defensive here.
		attrs = map[string]policy.ToolAttrs{}
	}
	shielded := func(name string) bool {
		return attrs[name].Network && bc.Sensitivity == "private"
	}
	annotations := func(name string) string {
		var notes []string
		if attrs[name].Sensitive {
			notes = append(notes, "sensitive: local models only")
		}
		if attrs[name].Network {
			notes = append(notes, "network")
		}
		if shielded(name) {
			notes = append(notes, "shield: private addresses only")
		}
		if len(notes) == 0 {
			return ""
		}
		return " [" + strings.Join(notes, ", ") + "]"
	}

	b.WriteString("Tools:\n")
	if len(bc.Agent.Governance) == 0 {
		for _, name := range bc.Agent.Tools {
			fmt.Fprintf(&b, "- %s: allow%s\n", name, annotations(name))
		}
		b.WriteString("(ungoverned: every listed tool is allowed on every channel)\n")
	} else {
		granted := make(map[string]bool, len(bc.Agent.Governance))
		for _, g := range bc.Agent.Governance {
			granted[g.Tool] = true
			line := fmt.Sprintf("- %s: %s", g.Tool, g.Mode)
			if len(g.Channels) > 0 {
				line += " [channels: " + strings.Join(g.Channels, ", ") + "]"
			}
			b.WriteString(line + annotations(g.Tool) + "\n")
		}
		for _, name := range bc.Agent.Tools {
			if !granted[name] {
				fmt.Fprintf(&b, "- %s: deny [not granted]\n", name)
			}
		}
	}

	fmt.Fprintf(&b, "Recent tool activity (last %d):\n", defaultToolReportEntries)
	entries := ring.lastFor(bc.Name, defaultToolReportEntries)
	if len(entries) == 0 {
		b.WriteString("(no recorded activity)\n")
		return b.String()
	}
	for _, e := range entries {
		line := fmt.Sprintf("- %s %s %s %s",
			e.at.UTC().Format(time.RFC3339), e.ev.Type.String(), e.ev.Tool, e.ev.Outcome)
		if e.ev.Rule != "" {
			line += " " + e.ev.Rule
		}
		if e.ev.Type == bus.ToolUsed {
			line += " " + e.ev.Latency.Round(time.Millisecond).String()
		}
		if e.ev.Channel != "" {
			line += " (" + e.ev.Channel + ")"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// toolsReporterFor builds the router's ToolsReporter over the boot config
// snapshot and the shared ring: the report is composed per request from the
// brain's config (immutable for the process lifetime, the BrainSummaries
// precedent) and the ring's current entries.
func toolsReporterFor(cfg *config.Config, ring *toolEventRing) func(brainName string) string {
	brains := make(map[string]config.BrainConfig, len(cfg.Brains))
	for _, bc := range cfg.Brains {
		brains[bc.Name] = bc
	}
	return func(brainName string) string {
		bc, ok := brains[brainName]
		if !ok {
			return fmt.Sprintf("Gatekeeper: unknown brain %q", brainName)
		}
		return gatekeeperReport(bc, ring)
	}
}
