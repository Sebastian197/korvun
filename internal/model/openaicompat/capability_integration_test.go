// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// AS-B-5 in package openaicompat_test (no cycle: openaicompat imports
// neither retry nor brain): the production decorator chain
// retry → WithModelID preserves the ToolCallingModel capability with
// ZERO new wiring code (ADR-0042 §4; retry/retry.go:100-121,
// brain/named.go:44-56).
package openaicompat_test

import (
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/openaicompat"
	"github.com/Sebastian197/korvun/internal/model/retry"
)

// TestCapabilityPropagation_throughProductionChain pins AS-B-5: the wired
// shape Build produces (retry(adapter) → WithModelID) still satisfies
// model.ToolCallingModel, so the AgentBrain's type assertion discovers
// the native lane on compat models automatically.
func TestCapabilityPropagation_throughProductionChain(t *testing.T) {
	t.Parallel()
	adapter, err := openaicompat.New(openaicompat.WithBaseURL("http://127.0.0.1:9/v1"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	decorated := retry.New(adapter, retry.Config{PerAttempt: time.Second, MaxRetries: 0})
	withID := brain.WithModelID(decorated, "some-model")

	if _, ok := withID.(model.ToolCallingModel); !ok {
		t.Fatal("retry → WithModelID chain lost the ToolCallingModel capability (ADR-0042 §4)")
	}
	if withID.Name() != "openai-compatible" {
		t.Errorf("Name() through the chain = %q, want openai-compatible", withID.Name())
	}
}
