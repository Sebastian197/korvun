// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/identity"
)

// TestIdentityRuntime_MintsEvidenceTheBootsRegistryAccepts attacks the one
// promise of the exported IdentityRuntime: that a process holding it mints
// evidence through the SAME registry the boot registers. The desktop e2e harness
// parks its strict scenario through the production door with it; a runtime that
// drifted from the boot's registry would mint evidence the store refuses, or —
// worse — evidence about principals the boot never registered. So the evidence
// it mints is handed to a real store in which ONLY the boot's registry was
// registered, and must be accepted, naming the boot's own principals.
//
// Evidence level: in-process; a real SQLite store, the boot's registry
// registered through RegisterIdentity, the attempt recorded through
// RecordAttemptAuthenticated.
// Probing mutation executed: the runtime is derived from the configuration with
// its channels dropped — red with «no ingress issuer for channel "telegram"».
func TestIdentityRuntime_MintsEvidenceTheBootsRegistryAccepts(t *testing.T) {
	ctx := context.Background()
	cfg := kernelWiringConfig(filepath.Join(t.TempDir(), "korvun.db"))
	resolver, issuers, err := IdentityRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	issuer := issuers["telegram"]
	if issuer == nil {
		t.Fatalf("no ingress issuer for channel %q: issuers = %v", "telegram", issuers)
	}

	store, err := actionsqlite.Open(StoragePath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.PutSigningKey(ctx, action.SigningKeyID(pub), hex.EncodeToString(pub), now); err != nil {
		t.Fatal(err)
	}
	wireIdentitySigners(store, priv)
	if err := store.RegisterIdentity(ctx, phase1IdentityRegistry(cfg), now); err != nil {
		t.Fatalf("register the boot's registry: %v", err)
	}

	const actionID, requestID = "act_runtime_probe", "request-runtime-probe"
	ingress, err := issuer.Issue(requestID, "authenticated-subject")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: actionID, RequestID: requestID, Channel: "telegram", Brain: "a",
	})
	if err != nil {
		t.Fatalf("resolve through the exported runtime: %v", err)
	}
	if evidence.RequesterPrincipalID != "principal_channel_telegram" || evidence.ActorPrincipalID != "principal_brain_a" ||
		evidence.ResponsiblePrincipalID != consoleResponsiblePrincipal || evidence.BindingID != "binding_telegram" {
		t.Errorf("evidence names %q / %q / %q under %q, want the boot's own principals and binding",
			evidence.RequesterPrincipalID, evidence.ActorPrincipalID, evidence.ResponsiblePrincipalID, evidence.BindingID)
	}
	env := action.NewEnvelope(actionID, requestID,
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "calc", Version: 1}, "1+1", now)
	env.Effect = action.Effect{Class: string(action.EffectPure)}
	env.Principal = action.PrincipalRef{PrincipalID: evidence.ActorPrincipalID,
		ResponsibleHumanID: evidence.ResponsiblePrincipalID, EvidenceID: evidence.EvidenceID}
	if err := store.RecordAttemptAuthenticated(ctx, env,
		actionsqlite.Decision{Outcome: "allow", Rule: "probe"}, action.StateAuthorized, evidence); err != nil {
		t.Errorf("the boot's registry refused evidence minted by the exported runtime: %v", err)
	}
}
