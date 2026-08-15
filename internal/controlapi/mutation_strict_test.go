// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package controlapi

import (
	"net/http"
	"strings"
	"testing"
)

// Estreno E-2 (red-team RT-1): the mutation surface must decode with the SAME
// strict schema config.Load enforces (audit A-1). Before this, a typo'd key
// POSTed to /api/config was silently dropped, the reload succeeded, and the
// persisted file lost the typo — "governence" yielded an UNGOVERNED agent
// brain while the operator believed grants applied.

func TestMutation_unknownKey_400_namesTheKey(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)

	body := strings.Replace(validCfgBody, "{", `{"governence": [],`, 1)
	rec := do(mux, "POST", "/api/config", "Bearer secret", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown key: got %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "governence") {
		t.Errorf("error body %q does not NAME the unknown key", rec.Body.String())
	}
	if rl.callCount() != 0 {
		t.Error("a config with an unknown key was handed to the supervisor")
	}
}

func TestMutation_trailingDocument_400(t *testing.T) {
	t.Setenv(adminEnv, "adminval")
	rl := &fakeReloader{handle: "r1"}
	mux := mutationMux("secret", rl)

	rec := do(mux, "POST", "/api/config", "Bearer secret", validCfgBody+"\n{\"more\":true}")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("trailing document: got %d, want 400", rec.Code)
	}
	if rl.callCount() != 0 {
		t.Error("a config with trailing data was handed to the supervisor")
	}
}
