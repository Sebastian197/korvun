// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
)

// TestAdminAddr_nilSafeStates covers the accessor's non-running branches: no
// admin server at all (observability disabled) and built-but-not-started
// (nothing bound yet). The running branch is exercised end-to-end by the
// desktop shell's lifecycle suite (internal/shell).
func TestAdminAddr_nilSafeStates(t *testing.T) {
	t.Parallel()

	off := false
	cfgOff := cfgWith(ollamaBrain())
	cfgOff.Observability = &config.ObservabilityConfig{Enabled: &off}
	appOff, err := Build(cfgOff, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build (observability off): %v", err)
	}
	t.Cleanup(func() { _ = appOff.Shutdown(t.Context()) })
	if got := appOff.AdminAddr(); got != "" {
		t.Fatalf("AdminAddr with observability disabled = %q, want empty", got)
	}

	appOn, err := Build(cfgWith(ollamaBrain()), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build (observability on): %v", err)
	}
	t.Cleanup(func() { _ = appOn.Shutdown(t.Context()) })
	if got := appOn.AdminAddr(); got != "" {
		t.Fatalf("AdminAddr before Start = %q, want empty (nothing bound)", got)
	}
}
