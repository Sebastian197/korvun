// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	identityv2 "github.com/Sebastian197/korvun/internal/identity"
)

// phase1CLIRegistry is the registry the operator CLI registered at the base of
// this phase (f5762b8), written out literally: the local operator role was an
// EXTERNAL SYSTEM there. A profile that CLI touched carries exactly these rows,
// each under its own signed lifecycle event.
func phase1CLIRegistry() identityv2.Registry {
	return identityv2.Registry{
		Principals: []identityv2.Principal{
			{ID: "principal_local_profile", Kind: identityv2.PrincipalExternalSystem,
				DisplayName: "Local profile credential"},
			{ID: action.OperatorPrincipal().PrincipalID, Kind: identityv2.PrincipalWorkload,
				DisplayName: "Local CLI operator workload"},
			{ID: "principal_local_operator_role", Kind: identityv2.PrincipalExternalSystem,
				DisplayName: "Local operator role"},
		},
		Bindings: []identityv2.Binding{{
			ID: "binding_cli", Provider: "cli", Channel: "cli",
			CredentialRef: "local_profile", SubjectNamespace: "local_profile",
			VerifiedSubject: "shared_local_profile",
			PrincipalID:     "principal_local_profile", Generation: 1,
			Status: identityv2.BindingActive,
		}},
		Workloads: []identityv2.Workload{{
			Brain: "cli", PrincipalID: action.OperatorPrincipal().PrincipalID,
			ResponsiblePrincipalID: "principal_local_operator_role",
		}},
	}
}

// plantPhase1CLIProfile leaves the profile the way the base's operator CLI left
// it: the store at the current schema, and the base's identity registry
// registered under the profile's own key.
func plantPhase1CLIProfile(t *testing.T, dbPath string) {
	t.Helper()
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open the profile: %v", err)
	}
	defer func() { _ = store.Close() }()
	priv, err := app.EnsureSigningKey(context.Background(), store, filepath.Dir(dbPath))
	if err != nil {
		t.Fatalf("profile key: %v", err)
	}
	store.SetIdentitySigners(
		func(e identityv2.Evidence) identityv2.SignedEvidence { return identityv2.SignEvidence(priv, e) },
		func(e identityv2.PrincipalEvent) identityv2.SignedPrincipalEvent {
			return identityv2.SignPrincipalEvent(priv, e)
		},
	)
	if err := store.RegisterIdentity(context.Background(), phase1CLIRegistry(), time.Now().UTC()); err != nil {
		t.Fatalf("register the base's registry: %v", err)
	}
}

// TestOperatorCLI_OpensAProfileTheBaseCLITouched attacks the sentence «existing
// non-strict profiles retain their current behavior» from the side the
// adversary's pass over this phase found it false (F1): a profile whose operator
// role was registered by the BASE's CLI as an external system must still open
// under this CLI, for every sealed verb, and the administrative door must still
// find a HUMAN to name. The base's row is history under a signed event; it is
// neither rewritten nor read as a conflict.
//
// Evidence level: in-process CLI (cli.Run over buffers) against a real SQLite
// file, read back through a second read-only connection. The base's rows are
// reproduced by registering the base's registry LITERAL, not by running the
// base's binary; the two-binary reproduction of the adversary's report was run
// by hand in separate OS processes and is captured in the phase's evidence.
// Probing mutation executed: give the human operator the base's principal id
// again — red on every row with «principal "principal_local_operator_role"
// conflicts with configured identity».
func TestOperatorCLI_OpensAProfileTheBaseCLITouched(t *testing.T) {
	cfg, dbPath := intentTestConfig(t)
	plantPhase1CLIProfile(t, dbPath)

	code, stdout, stderr := runIntentCLI(t, "intent", "create", "--config", cfg,
		"--purpose", "upgrade repro", "--operations", "calc", "--max-actions", "5",
		"--expires", "2027-01-01T00:00:00Z")
	if code != 0 || !strings.Contains(stdout, "created (DRAFT)") {
		t.Errorf("intent create on the base's profile: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// The refusal an absent approval earns is the ORDINARY one, not an identity
	// conflict: the door opened and looked for the row.
	code, _, stderr = runIntentCLI(t, "approvals", "reject", "--config", cfg,
		"apr_00000000000000000000000000000000")
	if code != 1 || strings.Contains(stderr, "conflicts with configured identity") ||
		!strings.Contains(stderr, "approval row absent") {
		t.Errorf("approvals reject on the base's profile: code=%d stderr=%q, want the ordinary «approval row absent»",
			code, stderr)
	}

	code, stdout, stderr = runIntentCLI(t, "authority", "activate", "--config", cfg,
		"--profile", "profile_local", "--reason", "go strict")
	if code != 0 || !strings.Contains(stdout, "authority activated for profile_local") {
		t.Errorf("authority activate on the base's profile: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// The base's row is untouched, and the human the administrative door names
	// is a row of its own.
	if n := authorityCLICount(t, dbPath,
		`SELECT COUNT(*) FROM principals WHERE principal_id='principal_local_operator_role' AND kind='external_system'`); n != 1 {
		t.Errorf("the base's operator-role rows still stored as external_system = %d, want 1", n)
	}
	if n := authorityCLICount(t, dbPath,
		`SELECT COUNT(*) FROM principals WHERE principal_id=? AND kind='human'`, localCLIResponsible); n != 1 {
		t.Errorf("human operator rows under %q = %d, want 1", localCLIResponsible, n)
	}
}
