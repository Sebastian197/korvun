// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

func TestIdentity_AllIngressDoorsCarryEvidence(t *testing.T) {
	cfgPath, _ := intentTestConfig(t)
	store, err := openOperatorStoreSealed(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := recordOperatorAct(context.Background(), store,
		"identity", "local-profile", `{}`, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListByOperation(context.Background(), "identity", "local-profile")
	if err != nil || len(records) != 1 {
		t.Fatalf("operator act records=%d err=%v", len(records), err)
	}
	signed, err := store.GetIdentityEvidence(context.Background(), records[0].Envelope.ActionID)
	if err != nil {
		t.Fatal(err)
	}
	evidence := signed.Evidence
	if evidence.RequesterPrincipalID != localCLIRequester ||
		evidence.ActorPrincipalID != "principal_operator" ||
		evidence.ResponsiblePrincipalID != localCLIResponsible ||
		evidence.Method != "local_profile" || evidence.BindingID != "binding_cli" ||
		evidence.SubjectClaim != "local_operator" {
		t.Fatalf("local CLI evidence = %+v", evidence)
	}
}

func TestIdentity_LocalCLIIdentityFailuresFailClosed(t *testing.T) {
	cfgPath, _ := intentTestConfig(t)
	closedStore, err := openOperatorStoreSealed(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := closedStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := recordDeniedAct(context.Background(), closedStore,
		"identity", "closed-denial", `{}`, "identity_test_denial"); err == nil {
		t.Fatal("recordDeniedAct accepted a closed identity store")
	}
	if err := recordOperatorAct(context.Background(), closedStore,
		"identity", "closed-act", `{}`, func() error { return nil }); err == nil {
		t.Fatal("recordOperatorAct accepted a closed identity store")
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := wireOperatorIdentity(closedStore, privateKey, time.Now().UTC()); err == nil {
		t.Fatal("wireOperatorIdentity accepted a closed store")
	}

	cfgPath, _ = intentTestConfig(t)
	store, err := openOperatorStoreSealed(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordOperatorAct(context.Background(), store,
		"identity", "close-before-finish", `{}`, func() error { return store.Close() }); err == nil ||
		!strings.Contains(err.Error(), "close the receipt") {
		t.Fatalf("finish after close error = %v, want close-the-receipt failure", err)
	}
}

func TestIdentity_LocalCLIV2IngressErrorsStayClosed(t *testing.T) {
	cfgPath, _ := intentTestConfig(t)
	fixtureDir := t.TempDir()
	fixture := writeIntentV2Fixture(t, fixtureDir)

	if code, _, _ := runIntentCLI(t, "intent", "create-v2"); code != 2 {
		t.Fatalf("missing create-v2 flags exit = %d, want 2", code)
	}
	if code, _, _ := runIntentCLI(t, "intent", "create-v2", "--config", cfgPath,
		"--file", filepath.Join(fixtureDir, "missing.json")); code != 1 {
		t.Fatalf("missing create-v2 file exit = %d, want 1", code)
	}
	malformed := filepath.Join(fixtureDir, "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"intent_id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runIntentCLI(t, "intent", "create-v2", "--config", cfgPath,
		"--file", malformed); code != 1 {
		t.Fatalf("malformed create-v2 file exit = %d, want 1", code)
	}
	if code, _, _ := runIntentCLI(t, "intent", "create-v2", "--config", filepath.Join(fixtureDir, "missing-config"),
		"--file", fixture); code != 1 {
		t.Fatalf("missing create-v2 config exit = %d, want 1", code)
	}
	if code, _, stderr := runIntentCLI(t, "intent", "create-v2", "--config", cfgPath,
		"--file", fixture); code != 0 {
		t.Fatalf("initial create-v2 exit = %d, stderr=%q", code, stderr)
	}
	if code, _, _ := runIntentCLI(t, "intent", "create-v2", "--config", cfgPath,
		"--file", fixture); code != 1 {
		t.Fatalf("duplicate create-v2 exit = %d, want 1", code)
	}
	if code, _, _ := runIntentCLI(t, "intent", "activate-v2", "--config", cfgPath, "int_cli_v2"); code != 2 {
		t.Fatalf("activation without version exit = %d, want 2", code)
	}
	if code, _, _ := runIntentCLI(t, "intent", "activate-v2", "--config", cfgPath, "int_cli_v2", "zero"); code != 2 {
		t.Fatalf("activation with invalid version exit = %d, want 2", code)
	}
	if code, _, _ := runIntentCLI(t, "intent", "expire-v2", "--config", cfgPath, "missing"); code != 1 {
		t.Fatalf("missing intent expiry exit = %d, want 1", code)
	}
}

func TestIdentity_LocalCLIDenialsAndFailuresStayAttributed(t *testing.T) {
	cfgPath, _ := intentTestConfig(t)
	store, err := openOperatorStoreSealed(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if err := recordDeniedAct(context.Background(), store,
		"identity", "denied", `{}`, "identity_test_denial"); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("mutation refused")
	if err := recordOperatorAct(context.Background(), store,
		"identity", "failed", `{}`, func() error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("recordOperatorAct error = %v, want sentinel", err)
	}

	for _, tt := range []struct {
		name  string
		state action.State
	}{
		{"denied", action.StateDenied},
		{"failed", action.StateFailed},
	} {
		records, err := store.ListByOperation(context.Background(), "identity", tt.name)
		if err != nil || len(records) != 1 {
			t.Fatalf("%s records=%d err=%v", tt.name, len(records), err)
		}
		if records[0].State != tt.state {
			t.Fatalf("%s state = %s, want %s", tt.name, records[0].State, tt.state)
		}
		signed, err := store.GetIdentityEvidence(context.Background(), records[0].Envelope.ActionID)
		if err != nil {
			t.Fatal(err)
		}
		if signed.Evidence.RequesterPrincipalID != localCLIRequester ||
			signed.Evidence.ActorPrincipalID != action.OperatorPrincipal().PrincipalID ||
			signed.Evidence.ResponsiblePrincipalID != localCLIResponsible {
			t.Fatalf("%s attribution = %+v", tt.name, signed.Evidence)
		}
	}
}
