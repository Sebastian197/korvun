// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// WithModelID must PROPAGATE the tool-calling capability (ADR-0042 §4: a
// capability must not vanish inside a decorator) — the production chain is
// retry(adapter) → WithModelID, so a hidden capability here would silently
// disable the native lane for every wired brain. GenerateWithTools applies
// the SAME copy-don't-mutate id binding Generate does (ADR-0014 §2).

func TestWithModelID_propagatesCapabilityIffInnerHasIt(t *testing.T) {
	t.Parallel()
	native := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply("done")}}
	bound := WithModelID(native, "llama3.2")
	if _, ok := bound.(model.ToolCallingModel); !ok {
		t.Fatal("WithModelID hid the tool-calling capability — the native lane would never run in production")
	}
	plain := &scriptedModel{name: "m", replies: []string{"x"}}
	if _, ok := WithModelID(plain, "id").(model.ToolCallingModel); ok {
		t.Fatal("WithModelID invented a capability the inner model lacks")
	}
}

func TestWithModelID_generateWithToolsBindsIDWithoutMutation(t *testing.T) {
	t.Parallel()
	native := &nativeScriptedModel{name: "n", replies: []model.Message{finalReply("done")}}
	bound := WithModelID(native, "llama3.2").(model.ToolCallingModel)

	shared := &model.Request{Model: "placeholder", Messages: []model.Message{{Role: model.RoleUser, Content: "x"}}}
	if _, err := bound.GenerateWithTools(context.Background(), shared, []model.ToolSpec{{Name: "calc", Description: "d"}}); err != nil {
		t.Fatalf("GenerateWithTools: %v", err)
	}
	if native.lastReq.Model != "llama3.2" {
		t.Fatalf("bound id not applied: %q", native.lastReq.Model)
	}
	if shared.Model != "placeholder" {
		t.Fatalf("shared request mutated: %q — the ADR-0014 §2 race reborn", shared.Model)
	}
	if len(native.lastSpecs) != 1 || native.lastSpecs[0].Name != "calc" {
		t.Fatalf("specs lost in the decorator: %+v", native.lastSpecs)
	}
}
