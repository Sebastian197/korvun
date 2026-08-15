// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// Estreno E-13 (testing specialist critical): the native lane's failure
// paths — model error, panicking adapter, nil response — had zero tests.
// The panic-recovery contract in particular ('a panicking adapter must not
// kill the router worker') was reimplemented locally for the native lane
// and never exercised.

// faultyNativeModel scripts a failure mode for GenerateWithTools.
type faultyNativeModel struct {
	mode string // "error" | "panic" | "nil"
}

func (m *faultyNativeModel) Name() string { return "faulty" }
func (m *faultyNativeModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	return &model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "old-lane"}, Provider: "faulty"}, nil
}
func (m *faultyNativeModel) GenerateWithTools(context.Context, *model.Request, []model.ToolSpec) (*model.Response, error) {
	switch m.mode {
	case "panic":
		panic("adapter bug")
	case "nil":
		return nil, nil
	default:
		return nil, errors.New("provider exploded")
	}
}

func TestNativeLane_failureModesDegradeToFallback(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"error", "panic", "nil"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			rec := &recordingMetrics{}
			a := NewAgentBrain(&faultyNativeModel{mode: mode}, spyRegistry(&spyTool{}),
				WithAgentLogger(quietLogger()), WithAgentMetrics(rec),
				WithAgentFallback("estoy en apuros"))

			out, err := a.Handle(context.Background(), inboundText("console", "c", "go"))
			if err != nil {
				t.Fatalf("Handle must degrade, not error (the router worker survives): %v", err)
			}
			if len(out) == 0 || !strings.Contains(out[0].Parts[0].Content, "apuros") {
				t.Fatalf("reply = %+v, want the configured fallback", out)
			}
		})
	}
}
