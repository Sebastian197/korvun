// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The measured toll (Action Kernel lote 3, spec FR-COMPAT-3): the hot path
// of one tool call, benchmarked BEFORE and AFTER the kernel adapter so the
// added cost (envelope + digest + record + finish) is a recorded number,
// bounded at <=5ms p95 by the spec. Run with:
//
//	go test ./internal/brain/ -run '^$' -bench BenchmarkRunToolHotPath -benchtime 2s
package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/tool"
)

// benchEnvelope is one inbound envelope reused across iterations.
func benchEnvelope() *envelope.Envelope {
	return &envelope.Envelope{ID: "bench-env", Channel: "console"}
}

func BenchmarkRunToolHotPath(b *testing.B) {
	a := NewAgentBrain(&scriptedModel{}, tool.Registry{"echo": tool.Echo()})
	env := benchEnvelope()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out := a.runTool(ctx, env, nil, "echo", `{"say":"hola"}`); out == "" {
			b.Fatal("echo must produce an observation")
		}
	}
}
