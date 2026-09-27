// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the operator act against a REAL action store.
//
// The Control API's own moulds drive the doors against a ledger double. These
// drive the LEDGER, over a real SQLite store on disk, because three of the
// ruling's demands cannot be seen from the other side: that the act lands as a
// row an operator can read, that it carries a receipt, and that it closes exactly
// ONCE however many times the screen polls.
//
// Evidence level, honest: in-process, REAL store on a real file in a temp dir,
// real identity resolution over the app's own phase-1 registry. Not a binary in a
// separate OS process, and `korvun receipt verify` over a SEALED receipt is the
// CLI's own mould — this one proves the receipt EXISTS and is readable, which is
// less.

package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/identity"
)

// realRecorder wires the recorder over a real store, and returns it with the
// store so a mould can read back what landed.
func realRecorder(t *testing.T) (*configActRecorder, *actionsqlite.Store, *[]error) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := cfgWith(ollamaBrain())
	store, err := actionsqlite.OpenFor(dbPath, testProfileIdentity)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	// The same two steps the boot does before any authenticated record: sign the
	// evidence with the profile's key, and register the identity registry in the
	// store so the evidence can be VALIDATED against rows rather than trusted.
	// Skipping them makes every seal fail with `identity: evidence unreadable`,
	// which is the store holding its own invariant and is how this mould found
	// out it had to do the boot's work.
	key, err := ensureSigningKey(context.Background(), store, t.TempDir())
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	wireIdentitySigners(store, key)
	// The receipt SEALER, which the boot installs at internal/app/app.go's
	// `actions.SetReceiptSealer`. Without it `FinishWithResult` mints no receipt
	// at all — `if s.sealer != nil` guards the whole block — so a mould that
	// skipped this step would have proved the act closes and quietly proved
	// nothing about the receipt the ruling asks for.
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		return action.SignReceipt(key, r)
	})
	if err := store.RegisterIdentity(context.Background(), registry, time.Now().UTC()); err != nil {
		t.Fatalf("register identity: %v", err)
	}
	var notes []error
	rec := newConfigActRecorder(store, resolver, issuers["console"],
		func(e error) { notes = append(notes, e) }, NewConfigActRegistry(nil), dbPath)
	if rec == nil {
		t.Fatal("the recorder was not wired over a real store")
	}
	rec.profile = testProfileIdentity
	return rec, store, &notes
}

// TestConfigActRecorder_theActLandsAsAReadableRowWithAReceipt is A1 and A4 from
// the ledger's side: the act is a row in the book, authorized, attributable to
// the Control API's operator workload, and it carries a receipt.
//
// PROBING MUTATION (executed): pass `action.StateSucceeded` to
// RecordAttemptAuthenticated instead of StateAuthorized. The store refuses it by
// name (`ErrNotADecisionState`) and this reddens on the seal — which is the
// ledger holding its own invariant, not this mould's kindness.
func TestConfigActRecorder_theActLandsAsAReadableRowWithAReceipt(t *testing.T) {
	rec, store, notes := realRecorder(t)
	ctx := context.Background()

	act, err := rec.BeginConfigAct(ctx, "config.enable-approvals", []byte(`{"door":"enable-approvals"}`))
	if err != nil {
		t.Fatalf("BeginConfigAct: %v", err)
	}
	if act.ActionID == "" {
		t.Fatal("the act has no id")
	}
	// NO receipt yet, and that is the ledger's shape: a receipt seals a terminal
	// state, and `RecordAttemptAuthenticated` mints one for every state EXCEPT
	// `StateAuthorized`. This mould found that out by failing, and the surface was
	// redesigned around it rather than the claim being softened.
	if act.ReceiptID != "" {
		t.Fatalf("the sealed act already carries the receipt %q; a receipt seals an OUTCOME and there is none yet", act.ReceiptID)
	}

	// The row, read back the way an operator's CLI would.
	got, err := store.Get(ctx, act.ActionID)
	if err != nil {
		t.Fatalf("read the act back: %v", err)
	}
	if got.State != action.StateAuthorized {
		t.Fatalf("the act landed in state %q, want %q: an act that is not authorized is not an act",
			got.State, action.StateAuthorized)
	}
	if got.Envelope.Operation.Namespace != "config" {
		t.Fatalf("the act's namespace is %q, want config: a profile change must be distinguishable from an authority act in the book",
			got.Envelope.Operation.Namespace)
	}
	if got.Identity == nil {
		t.Fatal("the act landed with no identity: the book cannot say who changed the profile")
	}
	if got.Identity.PrincipalID != controlAPIOperatorPrincipal {
		t.Fatalf("the act's principal is %q, want %q: the book must say WHO changed the profile",
			got.Identity.PrincipalID, controlAPIOperatorPrincipal)
	}

	// And the receipt arrives with the OUTCOME, which is what it seals.
	closed := rec.SettleAct(ctx, act.ActionID, true, "succeeded")
	if closed.ReceiptID == "" {
		t.Fatalf("the closed act carries no receipt; notes: %v", *notes)
	}
	r, err := store.GetReceipt(ctx, closed.ReceiptID)
	if err != nil {
		t.Fatalf("read the receipt back: %v", err)
	}
	if r.ActionID != act.ActionID {
		t.Fatalf("the receipt names action %q, want %q", r.ActionID, act.ActionID)
	}
	if r.Outcome != string(action.StateSucceeded) {
		t.Fatalf("the receipt seals the outcome %q, want %q", r.Outcome, action.StateSucceeded)
	}
}

// TestConfigActRecorder_closesTheActExactlyOnce is where «once» is proved, over a
// real ledger rather than between two doubles. The screen POLLS the status door,
// so the recorder is asked to settle on every poll that sees a terminal state; a
// second Finish over a closed act is a real error from a real store, and it must
// never happen.
//
// The oracle is by IMPOSSIBILITY on both sides: the row's final state, and the
// note channel — a second close that failed would have left a note, so an empty
// note list is the proof that no second close was attempted.
//
// PROBING MUTATION (executed): remove the `settled` guard from SettleAct. The
// second call reaches Finish, the store refuses it, a note appears, and this
// reddens on the note.
func TestConfigActRecorder_closesTheActExactlyOnce(t *testing.T) {
	rec, store, notes := realRecorder(t)
	ctx := context.Background()

	act, err := rec.BeginConfigAct(ctx, "config.set-ceiling", []byte(`{"door":"set-ceiling"}`))
	if err != nil {
		t.Fatalf("BeginConfigAct: %v", err)
	}
	rec.BindReload(act.ActionID, "reload-1")

	// Four settles, as four polls of a finished cutover would ask for.
	for i := 0; i < 4; i++ {
		rec.SettleReload(ctx, "reload-1", true, "succeeded")
	}

	got, err := store.Get(ctx, act.ActionID)
	if err != nil {
		t.Fatalf("read the act back: %v", err)
	}
	if got.State != action.StateSucceeded {
		t.Fatalf("the act closed in %q, want %q", got.State, action.StateSucceeded)
	}
	if len(*notes) != 0 {
		t.Fatalf("the recorder noted %v: a second close was attempted over a closed act", *notes)
	}
	// And every poll gets the SAME receipt, not nothing: a screen that polled
	// twice must not see the receipt disappear.
	first := rec.SettleReload(ctx, "reload-1", true, "succeeded")
	if first.ReceiptID == "" {
		t.Fatal("a repeated poll lost the receipt")
	}
	if again := rec.SettleReload(ctx, "reload-1", true, "succeeded"); again.ReceiptID != first.ReceiptID {
		t.Fatalf("two polls answered different receipts: %q then %q", first.ReceiptID, again.ReceiptID)
	}
}

// TestConfigActRecorder_aFailedCutoverClosesTheActAsFailed is the ruling's row.
// The act says what HAPPENED.
//
// PROBING MUTATION (executed): always pass StateSucceeded to Finish. The row then
// says the change worked and this reddens.
func TestConfigActRecorder_aFailedCutoverClosesTheActAsFailed(t *testing.T) {
	rec, store, _ := realRecorder(t)
	ctx := context.Background()

	act, err := rec.BeginConfigAct(ctx, "config.lift-shadow", []byte(`{"door":"lift-shadow"}`))
	if err != nil {
		t.Fatalf("BeginConfigAct: %v", err)
	}
	rec.SettleAct(ctx, act.ActionID, false, "rolled-back")

	got, err := store.Get(ctx, act.ActionID)
	if err != nil {
		t.Fatalf("read the act back: %v", err)
	}
	if got.State != action.StateFailed {
		t.Fatalf("the act closed in %q, want %q: the book must record the result, not the wish",
			got.State, action.StateFailed)
	}
}

// TestConfigActRecorder_aHandleNobodyBoundClosesNothing: the status door serves
// handles this surface never created — a reload from before a restart, or from
// another path. Closing an act it does not own would be inventing one.
//
// PROBING MUTATION (executed): drop the empty-actionID guard. SettleAct is then
// called with "" and this reddens on the note it leaves.
func TestConfigActRecorder_aHandleNobodyBoundClosesNothing(t *testing.T) {
	rec, _, notes := realRecorder(t)

	rec.SettleReload(context.Background(), "reload-from-another-life", true, "succeeded")

	if len(*notes) != 0 {
		t.Fatalf("the recorder noted %v for a handle it never bound", *notes)
	}
}

// TestConfigActRecorder_noStoreMeansNoRecorder is A6's other half: with no store
// the wiring returns nil, which makes every mutation door refuse by name.
//
// PROBING MUTATION (executed): return a recorder over a nil store. The doors then
// mount with a recorder that panics on first use, and this reddens.
func TestConfigActRecorder_noStoreMeansNoRecorder(t *testing.T) {
	cfg := cfgWith(ollamaBrain())
	_, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	if rec := newConfigActRecorder(nil, resolver, issuers["console"], func(error) {}, nil, ""); rec != nil {
		t.Fatal("a nil store produced a recorder: the doors would apply changes with no book to record them in")
	}
	if rec := newConfigActRecorder(nil, nil, nil, func(error) {}, nil, ""); rec != nil {
		t.Fatal("nil everything produced a recorder")
	}
}

// TestIdentity_aBrainNamedLikeTheControlAPIWorkloadFailsTheBoot is the claim the
// comment on `controlAPIWorkloadBrain` makes, held by execution.
//
// The first version of that comment said `config.Validate` rejects such a name.
// Executed, the validator ACCEPTS it. What really protects the operator's acts is
// stronger and is what this pins: two workloads under one brain make
// `identity.NewResolver` refuse, so the app fails to boot BY NAME rather than
// letting a configured brain silently take over the acts.
//
// PROBING MUTATION (executed): rename the reserved brain to a plain identifier a
// profile would never use. The collision stops being reachable and this test's
// premise disappears — so instead the mutation is to make the registry skip its
// own workload when a brain shares the name, which is the silent takeover this
// forbids; the resolver then accepts the config and this reddens.
func TestIdentity_aBrainNamedLikeTheControlAPIWorkloadFailsTheBoot(t *testing.T) {
	cfg := cfgWith(ollamaBrain())
	cfg.Brains[0].Name = controlAPIWorkloadBrain
	for i := range cfg.Routes {
		cfg.Routes[i].Brain = controlAPIWorkloadBrain
	}
	// The validator accepts it — measured, not assumed.
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config.Validate refused the name %q (%v); this mould's premise is that it does NOT, so the comment on controlAPIWorkloadBrain must be re-read",
			controlAPIWorkloadBrain, err)
	}
	if _, err := identity.NewResolver(phase1IdentityRegistry(cfg), time.Now); err == nil {
		t.Fatalf("a brain named %q booted alongside the Control API's own workload: one of them would silently own the operator's acts",
			controlAPIWorkloadBrain)
	}
}
