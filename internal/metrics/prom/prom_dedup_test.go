// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package prom

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Audit finding R-1: the router's dedup window drops duplicate deliveries as
// counted events; korvun_deduped_total{channel} is the operator's saturation
// mirror for that drop class.

func TestIncDeduped_countsByChannel(t *testing.T) {
	m := New()
	m.IncDeduped("telegram")
	m.IncDeduped("telegram")
	m.IncDeduped("webhook")

	expected := `
# HELP korvun_deduped_total Inbound events dropped as duplicates by the router's dedup window, by channel.
# TYPE korvun_deduped_total counter
korvun_deduped_total{channel="telegram"} 2
korvun_deduped_total{channel="webhook"} 1
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_deduped_total"); err != nil {
		t.Fatal(err)
	}
}
