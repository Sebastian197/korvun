// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestLogo_carriesTheA1Mark pins the banner to the A1 governed-K identity:
// the placeholder language is gone, the art tells the routing story (the K
// tile with three inputs, one decision, one exit), and every line stays
// within 80 columns so the banner never wraps on a standard terminal.
func TestLogo_carriesTheA1Mark(t *testing.T) {
	t.Parallel()

	for _, stale := range []string{"placeholder", "TBD"} {
		if strings.Contains(logo, stale) {
			t.Errorf("logo still carries the pre-A1 placeholder marker %q", stale)
		}
	}
	if !strings.Contains(logo, "KORVUN") {
		t.Error("logo must name KORVUN")
	}
	// The governed-K story in characters: the rounded tile, three input
	// nodes on its left, and the decision emitting exactly one output node.
	for _, needle := range []string{"╭", "╰", "◆"} {
		if !strings.Contains(logo, needle) {
			t.Errorf("logo must draw the governed tile and decision; missing %q", needle)
		}
	}
	if got := strings.Count(logo, "●"); got != 4 {
		t.Errorf("the A1 story is three signals in and one reply out (4 nodes); counted %d", got)
	}
	for _, line := range strings.Split(logo, "\n") {
		if width := utf8.RuneCountInString(line); width > 80 {
			t.Errorf("banner line exceeds 80 columns (%d): %q", width, line)
		}
	}
}
