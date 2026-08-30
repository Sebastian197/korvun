// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Identity boot wiring contract — Etapa 2, lote 4, pieza 3 (spec
// FR-PRIN-3, FR-MIG-1/2, sealed decisions 1-2): the provenance registry
// is wired from config; the root intent auto-materializes at boot,
// idempotent across boots and boot-fatal when the store cannot persist
// it; derived grants come from governance ALLOW rows with channel
// restrictions carried through; the app adapter records identity and
// evidence in one store transaction. Approved-red contract.

package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

func TestProvenanceRegistry_fromConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Channels: []config.ChannelConfig{
		{Type: "telegram"}, {Type: "webhook"}, {Type: "discord"},
	}}
	reg := provenanceRegistry(cfg)
	want := map[string]action.Provenance{
		"console":  {Class: "console", Credential: action.CredentialLoopbackInProcess},
		"telegram": {Class: "telegram", Credential: action.CredentialBotTokenSession},
		"webhook":  {Class: "webhook", Credential: action.CredentialInboundBearer},
		"discord":  {Class: "discord", Credential: action.CredentialGatewaySession},
	}
	if len(reg) != len(want) {
		t.Fatalf("registry = %+v, want %d entries", reg, len(want))
	}
	for name, provenance := range want {
		if reg[name] != provenance {
			t.Fatalf("registry[%q] = %+v, want %+v", name, reg[name], provenance)
		}
	}
	// A channel-less boot still carries the console: the operator's own
	// hands are in-process provenance, not config.
	bare := provenanceRegistry(&config.Config{})
	if len(bare) != 1 || bare["console"].Credential != action.CredentialLoopbackInProcess {
		t.Fatalf("the console is always present, got %+v", bare)
	}
}

func TestDerivedConfigGrant_fromGovernance(t *testing.T) {
	t.Parallel()
	bc := config.BrainConfig{Name: "asistente", Agent: &config.AgentConfig{
		Tools: []string{"calc", "time", "read_file"},
		Governance: []config.ToolGrantConfig{
			{Tool: "calc", Mode: "allow"},
			{Tool: "time", Mode: "allow", Channels: []string{"telegram"}},
			{Tool: "read_file", Mode: "deny"},
		},
	}}
	grant, ok := derivedConfigGrant(bc)
	if !ok {
		t.Fatal("governed ALLOW tools must derive a grant")
	}
	if len(grant.Operations) != 2 {
		t.Fatalf("only ALLOW rows become authority, got %v", grant.Operations)
	}
	found := map[string]bool{}
	for _, op := range grant.Operations {
		found[op] = true
	}
	if !found["calc"] || !found["time"] || found["read_file"] {
		t.Fatalf("operations = %v", grant.Operations)
	}
	// The channel restriction is CARRIED THROUGH as a coarse resource.
	if len(grant.ResourceScope) != 1 || grant.ResourceScope[0] != "channel:telegram" {
		t.Fatalf("channel restrictions must survive derivation, got %v", grant.ResourceScope)
	}
	if grant.SubjectPrincipalID != action.BrainPrincipal("asistente").PrincipalID {
		t.Fatalf("subject = %q", grant.SubjectPrincipalID)
	}
	// Deterministic across boots (AS-7).
	again, _ := derivedConfigGrant(bc)
	if again.GrantID != grant.GrantID {
		t.Fatalf("derivation must be deterministic: %q vs %q", again.GrantID, grant.GrantID)
	}
	// Unrestricted governance derives "*" resources.
	open := config.BrainConfig{Name: "b", Agent: &config.AgentConfig{
		Governance: []config.ToolGrantConfig{{Tool: "calc", Mode: "allow"}},
	}}
	openGrant, _ := derivedConfigGrant(open)
	if len(openGrant.ResourceScope) != 1 || openGrant.ResourceScope[0] != "*" {
		t.Fatalf("unrestricted governance derives *, got %v", openGrant.ResourceScope)
	}
	// No governance (or no allows) derives NOTHING: ungoverned flows act
	// directly under the root's standing authority.
	if _, ok := derivedConfigGrant(config.BrainConfig{Name: "c", Agent: &config.AgentConfig{}}); ok {
		t.Fatal("no governance must derive no grant")
	}
	if _, ok := derivedConfigGrant(config.BrainConfig{Name: "d", Agent: &config.AgentConfig{
		Governance: []config.ToolGrantConfig{{Tool: "calc", Mode: "shadow"}},
	}}); ok {
		t.Fatal("shadow-only governance grants no authority")
	}
}

func TestBuild_materializesTheRootIntentIdempotently(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	wantDigest := action.RootIntent().Digest()
	for boot := 0; boot < 2; boot++ {
		app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if err != nil {
			t.Fatalf("boot %d: %v", boot, err)
		}
		shutdownApp(t, app)
		store, err := actionsqlite.Open(dbPath)
		if err != nil {
			t.Fatalf("boot %d reopen: %v", boot, err)
		}
		root, err := store.GetIntent(context.Background(), action.RootIntentID)
		if err != nil {
			t.Fatalf("boot %d: the root intent must exist after boot: %v", boot, err)
		}
		if root.Digest() != wantDigest {
			t.Fatalf("boot %d: root digest drifted:\ngot  %s\nwant %s", boot, root.Digest(), wantDigest)
		}
		if root.Status != action.LifecycleActive || root.Version != 1 {
			t.Fatalf("boot %d: root must stay ACTIVE v1 (idempotent no-op), got %s v%d", boot, root.Status, root.Version)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("boot %d close: %v", boot, err)
		}
	}
}

func TestBuild_rootIntentFailureIsBootFatal(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	// Prepare a v2 file whose intents table refuses inserts: the boot that
	// cannot materialize the root must FAIL, not shrug.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TRIGGER block_root BEFORE INSERT ON intents
		 BEGIN SELECT RAISE(ABORT, 'blocked by test'); END;`,
	); err != nil {
		t.Fatalf("install blocker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
	if _, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram")))); err == nil {
		t.Fatal("a boot that cannot persist the root intent must be fatal")
	}
}

func TestActionRecorder_identifiedRoundTripsThroughTheStore(t *testing.T) {
	t.Parallel()
	store, err := actionsqlite.Open(filepath.Join(t.TempDir(), "korvun.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	rec := actionRecorder{store: store}
	env := action.NewEnvelope("act_app1", "corr-1",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1},
		`{"a":1}`, time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC))
	env.Principal = action.PrincipalRef{
		PrincipalID:        action.BrainPrincipal("a").PrincipalID,
		EvidenceID:         "evd_00000000000000aa",
		ResponsibleHumanID: action.OperatorPrincipal().PrincipalID,
	}
	env.IntentID = action.RootIntentID
	env.AuthorityRefs = []string{"grant_cfg_deadbeef"}
	evidence := action.IdentityEvidence{
		EvidenceID: "evd_00000000000000aa", Provider: "console",
		Subject: "console-user", Credential: action.CredentialLoopbackInProcess,
		IssuedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC), TransportBinding: "console",
		ClaimsDigest: "sha256:aa",
	}
	if err := rec.RecordAttemptIdentified(context.Background(), env, "allow", "granted",
		action.StateAuthorized, evidence); err != nil {
		t.Fatalf("record identified: %v", err)
	}
	got, err := store.Get(context.Background(), "act_app1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Identity == nil || got.Identity.PrincipalID != env.Principal.PrincipalID ||
		got.Identity.IntentID != action.RootIntentID {
		t.Fatalf("identity must round-trip through the adapter, got %+v", got.Identity)
	}
	if len(got.Identity.AuthorityRefs) != 1 || got.Identity.AuthorityRefs[0] != "grant_cfg_deadbeef" {
		t.Fatalf("authority refs = %v", got.Identity.AuthorityRefs)
	}
	storedEvidence, err := store.GetEvidence(context.Background(), "act_app1")
	if err != nil {
		t.Fatalf("evidence must land with the attempt: %v", err)
	}
	if storedEvidence != evidence {
		t.Fatalf("evidence round-trip:\ngot  %+v\nwant %+v", storedEvidence, evidence)
	}
	if !errors.Is(func() error {
		_, err := store.GetEvidence(context.Background(), "act_missing")
		return err
	}(), actionsqlite.ErrNotFound) {
		t.Fatal("sanity: unknown evidence stays not-found")
	}
}
