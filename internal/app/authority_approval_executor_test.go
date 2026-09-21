// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	"github.com/Sebastian197/korvun/internal/config"
)

// legacyOnlyApprovalStore answers an APPROVED request and offers ONLY the
// legacy claim: it has no authority door. It counts the claims it is asked for.
type legacyOnlyApprovalStore struct{ claims int }

func (s *legacyOnlyApprovalStore) ReadApproval(context.Context, string) (action.Approval, error) {
	return action.Approval{ApprovalID: "apr_probe", ActionID: "act_probe", Status: action.ApprovalApproved}, nil
}
func (s *legacyOnlyApprovalStore) ReadActionState(context.Context, string) (action.State, error) {
	return action.StateApproved, nil
}
func (s *legacyOnlyApprovalStore) Claim(context.Context, string, *action.Approval) ([]byte, action.Operation, error) {
	s.claims++
	return nil, action.Operation{}, errors.New("probe: the legacy claim was asked")
}
func (s *legacyOnlyApprovalStore) Close(context.Context, string, action.State, time.Time, string) error {
	return nil
}
func (s *legacyOnlyApprovalStore) ReadReceipt(context.Context, string) (string, error) {
	return "", nil
}

// TestBuildApprovalExecutor_HonoursTheStrictConfigItIsGiven attacks the builder
// that TAKES the configuration and ignored what it said: under a strict config
// it built a non-strict executor, whose resume goes to the legacy claim (the
// adversary's pass over piece 3 phase 3, F9). A strict executor handed a store
// with no authority door must refuse by name and never ask for the legacy claim.
//
// Evidence level: in-process; the real builder and the real coordinator over a
// store double that offers only the legacy claim.
// Probing mutation executed: build non-strict whatever the config says — red
// with «legacy claims asked = 1, want 0» and the probe's own claim error in
// place of the named refusal.
func TestBuildApprovalExecutor_HonoursTheStrictConfigItIsGiven(t *testing.T) {
	cfg := kernelWiringConfig(filepath.Join(t.TempDir(), "korvun.db"))
	cfg.Brains[0].Agent = &config.AgentConfig{Tools: []string{"calc"}}
	cfg.Authority = &config.AuthorityConfig{Mode: "strict", ActivationDigest: "sha256:probe"}
	if !cfg.StrictAuthority() {
		t.Fatal("the fixture config is not strict: this mould would prove nothing")
	}
	exec, err := BuildApprovalExecutor(cfg, action.ActionPreview{
		PrincipalID: "principal_brain_a", Operation: "tool/calc"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	store := &legacyOnlyApprovalStore{}
	_, err = exec.ResumeApproved(context.Background(), store, "apr_probe")
	var resume *executor.ResumeError
	if !errors.As(err, &resume) || resume.Stage != executor.ResumeClaim ||
		!errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
		t.Errorf("resume under a strict config over a legacy-only store: error = %v, want stage %q naming %v",
			err, executor.ResumeClaim, action.ErrAuthorityEvidenceCorrupt)
	}
	if store.claims != 0 {
		t.Errorf("legacy claims asked = %d, want 0", store.claims)
	}
}
