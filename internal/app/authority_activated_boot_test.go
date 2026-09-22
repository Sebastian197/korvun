// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/identity"
)

// activateAuthorityOnDisk turns a profile strict through the store's exported
// doors — the ones `korvun authority activate` uses — and returns the pinned
// activation digest. It closes its own store, so the file is free afterwards.
func activateAuthorityOnDisk(t *testing.T, cfg *config.Config) string {
	t.Helper()
	return activateAuthorityProfileOnDisk(t, cfg, "profile_boot", "act_activate_boot")
}

func activateAuthorityProfileOnDisk(t *testing.T, cfg *config.Config, profile, actID string) string {
	t.Helper()
	ctx := context.Background()
	store, err := actionsqlite.Open(StoragePath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	key, err := EnsureSigningKey(ctx, store, filepath.Dir(StoragePath(cfg)))
	if err != nil {
		t.Fatal(err)
	}
	wireIdentitySigners(store, key)
	store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
		return action.SignAuthorityBytes(key, domain, canonical)
	})
	if err := store.RegisterIdentity(ctx, phase1IdentityRegistry(cfg), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// The activation is an ADMINISTRATIVE act and demands a HUMAN responsible.
	// The app's own registry has none — its console role is an external system —
	// so the only door that can activate a profile is the operator CLI, which
	// registers its own human. This fixture registers the same shape rather than
	// importing that package, which would close a cycle.
	operatorRegistry := identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_local_profile", Kind: identity.PrincipalExternalSystem, DisplayName: "Local profile credential"},
			{ID: action.OperatorPrincipal().PrincipalID, Kind: identity.PrincipalWorkload, DisplayName: "Local CLI operator workload"},
			{ID: "principal_local_operator", Kind: identity.PrincipalHuman, DisplayName: "Local operator"},
		},
		Bindings: []identity.Binding{{
			ID: "binding_cli", Provider: "cli", Channel: "cli", CredentialRef: "local_profile",
			SubjectNamespace: "local_profile", VerifiedSubject: "shared_local_profile",
			PrincipalID: "principal_local_profile", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{
			Brain: "cli", PrincipalID: action.OperatorPrincipal().PrincipalID,
			ResponsiblePrincipalID: "principal_local_operator",
		}},
	}
	if err := store.RegisterIdentity(ctx, operatorRegistry, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	operatorResolver, err := identity.NewResolver(operatorRegistry, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	operatorIssuer, err := operatorResolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_cli", Method: "local_profile", CredentialClass: "local_profile", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := store.AuthorityActivationManifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reason := "turn the profile strict"
	act := func() string {
		// The activation's one-shot authenticated act, as the operator CLI
		// records it: a human responsible behind the console workload.
		id, requestID := actID, "request-"+actID
		ingress, err := operatorIssuer.Issue(requestID, "operator-subject")
		if err != nil {
			t.Fatal(err)
		}
		evidence, err := operatorResolver.Resolve(ingress, identity.ResolveRequest{
			ActionID: id, RequestID: requestID, Channel: "cli", Brain: "cli",
		})
		if err != nil {
			t.Fatal(err)
		}
		op := action.Operation{Namespace: "authority", Name: "activate", Version: 1}
		env := action.NewEnvelope(id, requestID,
			action.Source{Kind: "agent_brain", Protocol: "text", Channel: "cli"}, op,
			string(actionsqlite.CanonicalAuthorityActivation(profile, manifest, reason)), time.Now().UTC())
		env.Effect = action.Effect{Class: string(action.EffectWriteReversible)}
		env.Principal = action.PrincipalRef{PrincipalID: evidence.ActorPrincipalID,
			ResponsibleHumanID: evidence.ResponsiblePrincipalID, EvidenceID: evidence.EvidenceID}
		if err := store.RecordAttemptAuthenticated(ctx, env,
			actionsqlite.Decision{Outcome: "allow", Rule: "administrative"}, action.StateAuthorized, evidence); err != nil {
			t.Fatal(err)
		}
		return id
	}()
	digest, err := store.ActivateAuthority(ctx, profile, act, reason, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// activateSecondAuthorityOnDisk activates a SECOND profile on the same store,
// which `korvun authority activate` allows: it refuses only a repeat of a
// profile id it already activated.
func activateSecondAuthorityOnDisk(t *testing.T, cfg *config.Config) string {
	t.Helper()
	return activateAuthorityProfileOnDisk(t, cfg, "profile_boot_two", "act_activate_boot_two")
}

// TestBuild_RefusesANonStrictBootOverAnActivatedProfile attacks the one window
// the v0.16.0 release notes could not close with prose: a profile that has been
// ACTIVATED carries strict law in its own store, and a boot whose config has
// lost — or has not yet gained — the `authority` block would arm no activation
// digest. Under such a boot the approval-birth ledger is not appended
// (`appendApprovalBirthTx` returns early on an unarmed store), so every approval
// parked there is born WITHOUT its birth event, and the next strict boot reads
// the ledger as corrupt: boot-fatal, with no second activation possible and no
// repair door. The internal adversary reproduced it over the v0.16.0 text with
// two real store instances on one file; the director's decision of 2026-09-22
// was that a path which bricks a profile is not published with a warning.
//
// The guarantee: such a boot is REFUSED, by name, before anything can park. The
// profile already has strict law — it boots strict or it does not boot.
//
// Evidence level: in-process, one real SQLite file, two REAL store lives over
// it: the activation closes its store, and `app.Build` opens the file again.
// Probing mutations executed, each alone: delete the refusal from `Build` — red
// on the two refusing rows with «a non-strict boot over an activated profile was
// ACCEPTED»; and drop the digest from the message — red with «error … want it to
// print the activation digest». The control rows pass under both, which is what
// controls are for: this mould parks nothing and observes no poisoned ledger,
// and an earlier draft of this godoc claimed it did.
func TestBuild_RefusesANonStrictBootOverAnActivatedProfile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := kernelWiringConfig(dbPath)
	digest := activateAuthorityOnDisk(t, cfg)

	t.Run("without the authority block the boot refuses by name", func(t *testing.T) {
		app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if app != nil {
			shutdownApp(t, app)
		}
		if !errors.Is(err, ErrAuthorityActivatedProfileNeedsStrict) {
			t.Fatalf("a non-strict boot over an activated profile was ACCEPTED: app=%v err=%v, want %v",
				app != nil, err, ErrAuthorityActivatedProfileNeedsStrict)
		}
		// THE REMEDY MUST TRAVEL WITH THE REFUSAL. Activation is one-way and no
		// verb prints the digest twice, so a message that demanded it without
		// printing it would leave a profile that boots neither way.
		if !strings.Contains(err.Error(), "profile_boot") || !strings.Contains(err.Error(), "authority") {
			t.Errorf("error = %q, want it to name the activated profile and the config block", err)
		}
		if !strings.Contains(err.Error(), digest) {
			t.Errorf("error = %q, want it to print the activation digest %q the operator must pin", err, digest)
		}
	})

	t.Run("a non-strict block is refused too", func(t *testing.T) {
		loose := kernelWiringConfig(dbPath)
		loose.Authority = &config.AuthorityConfig{Mode: "off"}
		app, err := Build(loose, withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if app != nil {
			shutdownApp(t, app)
		}
		if !errors.Is(err, ErrAuthorityActivatedProfileNeedsStrict) {
			t.Errorf("mode %q over an activated profile: err=%v, want %v", loose.Authority.Mode, err,
				ErrAuthorityActivatedProfileNeedsStrict)
		}
	})

	// The control, and the reason the refusal is safe: the SAME file boots when
	// the config carries the strict block the profile's own law demands.
	t.Run("with the strict block it boots", func(t *testing.T) {
		strict := kernelWiringConfig(dbPath)
		strict.Authority = &config.AuthorityConfig{Mode: "strict", ActivationDigest: digest}
		app, err := Build(strict, withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if err != nil {
			t.Fatalf("the strict boot over its own activated profile: %v", err)
		}
		shutdownApp(t, app)
	})

	// TWO activation roots on one store is a state the CLI can create —
	// `authority activate` takes any profile id and refuses only a repeat of the
	// same one — so it is not corruption. The refusal names both, with both
	// digests, and keeps its own sentinel.
	t.Run("two activated profiles are named, not called corrupt", func(t *testing.T) {
		second := activateSecondAuthorityOnDisk(t, cfg)
		app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if app != nil {
			shutdownApp(t, app)
		}
		if !errors.Is(err, ErrAuthorityActivatedProfileNeedsStrict) {
			t.Fatalf("two roots: error = %v, want %v", err, ErrAuthorityActivatedProfileNeedsStrict)
		}
		if errors.Is(err, actionsqlite.ErrAuthorizationSnapshotCorrupt) {
			t.Errorf("error = %v: a state the CLI creates was called corrupt", err)
		}
		for _, want := range []string{"profile_boot", digest, "profile_boot_two", second} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to carry %q", err, want)
			}
		}
	})

	// And a profile that was NEVER activated still boots non-strict, which is
	// every profile in the field today.
	t.Run("an un-activated profile is untouched", func(t *testing.T) {
		fresh := kernelWiringConfig(filepath.Join(t.TempDir(), "fresh.db"))
		app, err := Build(fresh, withChannelFactory(okFactory(newFakeChannel("telegram"))))
		if err != nil {
			t.Fatalf("a non-strict boot over a profile that was never activated: %v", err)
		}
		shutdownApp(t, app)
	})
}

// TestRefuseNonStrictBootOverActivation_NoStoreFailsClosed covers the one arm
// `Build` cannot reach: `Build` returns on the store's own open error, so the
// refusal never receives a nil store there. The arm exists as defence in depth,
// and its posture is the point — a door that cannot ask «is this profile
// activated?» must not answer «no». Its sibling `PrepareStrictAuthority`
// refuses the same input, and an earlier draft of this one returned nil.
//
// It enters through the package function, NOT through `Build`, because no
// caller can produce the state; that is declared rather than dressed up.
//
// Evidence level: in-process, no store at all.
// Probing mutation executed: return nil for a nil store — red, «error = <nil>,
// want a refusal».
func TestRefuseNonStrictBootOverActivation_NoStoreFailsClosed(t *testing.T) {
	err := refuseNonStrictBootOverActivation(context.Background(), nil)
	if err == nil {
		t.Fatal("error = <nil>, want a refusal: a door that cannot ask must not answer «not activated»")
	}
	if !strings.Contains(err.Error(), "action store") {
		t.Errorf("error = %q, want it to name what is missing", err)
	}
}
