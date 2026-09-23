// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// The door that makes delegation reachable, attacked.
//
// THE STATE BEFORE IT. Schema 15 gave `execution_bindings` an exact
// `grant_id`/`grant_version`/`grant_digest`, `bindingAuthorityTx` has policed
// the triple all-or-nothing since, and `resolveAuthorityTx` branches on it —
// but NOTHING ever wrote one. `intent bind` left the three NULL, no other verb
// filled them, so every strict start resolved through the config clause while a
// signed grant sat ACTIVE and unused. Attenuated children, shared ancestor
// budgets and depth limits were implemented, tested, and unreachable.
//
// Evidence level, honest: in-process, one real SQLite file, the production
// door, and for the race a SECOND real connection committing between this
// transaction's read and its commit.

func bindFor(f authoritySQLiteFixture, id string) action.ExecutionBinding {
	return action.ExecutionBinding{
		BindingID: id, ActorPrincipalID: f.root.SubjectPrincipalID, Channel: "webhook",
		IntentID: f.intent.IntentID, IntentVersion: f.intent.Version,
		IntentDigest: f.intent.Digest(), Revision: 1, Status: action.BindingActive,
	}
}

// THE FAULT the door exists for: a binding that names a grant, written whole.
func TestBindWithGrant_writesTheTripleWholeOrNotAtAll(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()

	// The fixture already holds this selector, so the first call through the
	// door is itself a re-bind — which is the ordinary case in the field, since
	// an operator who bound without a grant is exactly who needs this door.
	revoked, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_one"), f.root.GrantID, f.now)
	if err != nil {
		t.Fatalf("the door refused a grant its own start path accepts: %v", err)
	}
	if revoked != "binding_authority_root" {
		t.Fatalf("the re-bind revoked %q, want the selector's holder", revoked)
	}

	var gotID, gotDigest string
	var gotVersion, revision int
	var status string
	if err := f.store.db.QueryRow(
		`SELECT grant_id,grant_version,grant_digest,revision,status
		   FROM execution_bindings WHERE binding_id='bind_one'`).
		Scan(&gotID, &gotVersion, &gotDigest, &revision, &status); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotID != f.root.GrantID || gotVersion != f.root.Version || gotDigest != f.root.Digest() {
		t.Fatalf("triple = %q/%d/%q, want the grant's own %q/%d/%q",
			gotID, gotVersion, gotDigest, f.root.GrantID, f.root.Version, f.root.Digest())
	}
	if revision != 2 || status != string(action.BindingActive) {
		t.Fatalf("revision=%d status=%q, want 2 and ACTIVE", revision, status)
	}

	// And the start path AGREES: the binding the door wrote is one it accepts.
	// Without this the door could write a shape nothing can use.
	if _, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "", f.now)); err != nil {
		t.Fatalf("a start over the binding this door wrote was refused: %v", err)
	}
}

// THE REPAIR: re-binding a selector that already has an ACTIVE row. Before this
// door it was impossible — the partial unique index refused the second insert,
// no verb revoked, and the operator was stuck with the binding they had.
func TestBindWithGrant_rebindRevokesTheOldRowAndCountsOn(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()

	if _, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_first"), f.root.GrantID, f.now); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	revoked, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_second"), f.root.GrantID, f.now)
	if err != nil {
		t.Fatalf("the re-bind was refused; the operator is stuck: %v", err)
	}
	if revoked != "bind_first" {
		t.Fatalf("re-bind revoked %q, want bind_first", revoked)
	}

	rows := map[string]struct {
		status   string
		revision int
	}{}
	cursor, err := f.store.db.Query(
		`SELECT binding_id,status,revision FROM execution_bindings ORDER BY binding_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cursor.Close() }()
	for cursor.Next() {
		var id, status string
		var revision int
		if err := cursor.Scan(&id, &status, &revision); err != nil {
			t.Fatal(err)
		}
		rows[id] = struct {
			status   string
			revision int
		}{status, revision}
	}

	// The OLD row survives. A binding is what an execution's authority resolved
	// through, so the row that authorised yesterday's action may not be
	// overwritten — only closed.
	if rows["bind_first"].status != string(action.BindingRevoked) {
		t.Fatalf("the old binding is %q, want REVOKED and still present", rows["bind_first"].status)
	}
	if rows["bind_first"].revision != 2 {
		t.Fatalf("the old binding's revision moved to %d", rows["bind_first"].revision)
	}
	if rows["bind_second"].status != string(action.BindingActive) || rows["bind_second"].revision != 3 {
		t.Fatalf("the new binding is %q at revision %d, want ACTIVE at 3",
			rows["bind_second"].status, rows["bind_second"].revision)
	}
}

// THE CONTROL: the refusals, each by its own name. A door that refused
// everything would pass the fault mould's opposite and be useless.
func TestBindWithGrant_refusesByName(t *testing.T) {
	ctx := context.Background()

	t.Run("a grant that does not exist", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		_, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_x"), "grant_nowhere", f.now)
		if !errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("err = %v, want %v", err, ErrAuthorityMissing)
		}
		assertNoBinding(t, f, "bind_x")
	})

	t.Run("a grant held by somebody else", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		b := bindFor(f, "bind_y")
		b.ActorPrincipalID = "principal_someone_else"
		_, err := f.store.BindExecutionWithGrant(ctx, b, f.root.GrantID, f.now)
		if !errors.Is(err, ErrIssuerMismatch) {
			t.Fatalf("err = %v, want %v", err, ErrIssuerMismatch)
		}
		assertNoBinding(t, f, "bind_y")
	})

	t.Run("a revoked grant", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
			CanonicalAuthorityRevoke(f.root.GrantID, "stop it"), f.now.Add(30))
		if err := f.store.RevokeAuthority(ctx, f.root.GrantID, act, "stop it", f.now.Add(31)); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		_, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_z"), f.root.GrantID, f.now)
		if !errors.Is(err, ErrAuthorityRevoked) {
			t.Fatalf("err = %v, want %v", err, ErrAuthorityRevoked)
		}
		assertNoBinding(t, f, "bind_z")
	})

	// Its OWN name, not "some error". The first shape of this subtest accepted
	// any non-nil error, and an adversarial pass showed what that hid: delete
	// the guard entirely and the flow falls through to `WHERE grant_id=''`,
	// finds nothing, and answers `ErrAuthorityMissing` — also an error, so the
	// subtest stayed green over a branch that no longer existed.
	t.Run("an empty grant id", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		_, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_w"), "", f.now)
		if !errors.Is(err, ErrGrantIDRequired) {
			t.Fatalf("err = %v, want %v", err, ErrGrantIDRequired)
		}
		if errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("an absent ARGUMENT was reported as an absent GRANT: %v", err)
		}
		assertNoBinding(t, f, "bind_w")
	})

	// A row that is not ACTIVE. The door writes the selector's current holder;
	// handed a REVOKED row it would revoke the real holder and insert a row
	// nothing resolves through, leaving the selector empty.
	t.Run("a row that is not ACTIVE", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		b := bindFor(f, "bind_v")
		b.Status = action.BindingRevoked
		_, err := f.store.BindExecutionWithGrant(ctx, b, f.root.GrantID, f.now)
		if !errors.Is(err, ErrBindingNotActive) {
			t.Fatalf("err = %v, want %v", err, ErrBindingNotActive)
		}
		assertNoBinding(t, f, "bind_v")
		// And the selector's real holder is untouched.
		var status string
		if err := f.store.db.QueryRow(
			`SELECT status FROM execution_bindings WHERE binding_id='binding_authority_root'`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != string(action.BindingActive) {
			t.Fatalf("the refused bind revoked the real holder: %q", status)
		}
	})

	// THE CHANNEL. This is the refusal the door did not have, and the one an
	// adversarial pass drove through the operator's own command: exit 0, an
	// ACTIVE row written, and every start under it dead with
	// `ErrAttenuationViolated` because the grant does not carry that channel.
	t.Run("a channel the grant does not carry", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		b := bindFor(f, "bind_channel")
		b.Channel = "console" // the fixture's root grant carries only "webhook"
		_, err := f.store.BindExecutionWithGrant(ctx, b, f.root.GrantID, f.now)
		if !errors.Is(err, action.ErrAttenuationViolated) {
			t.Fatalf("err = %v, want %v", err, action.ErrAttenuationViolated)
		}
		if !strings.Contains(err.Error(), "console") {
			t.Fatalf("the refusal does not name the channel the operator typed: %v", err)
		}
		assertNoBinding(t, f, "bind_channel")
	})

	// A HEAD THAT NAMES A VERSION NOBODY HOLDS is broken evidence, not an
	// absent grant. The head was read one statement earlier, so "missing" would
	// send an operator to issue authority they demonstrably already have.
	t.Run("a head naming a version that does not exist", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		if _, err := f.store.db.Exec(
			`UPDATE grant_heads SET active_version=99 WHERE grant_id=?`, f.root.GrantID); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_corrupt"), f.root.GrantID, f.now)
		if !errors.Is(err, action.ErrAuthorityEvidenceCorrupt) {
			t.Fatalf("err = %v, want %v", err, action.ErrAuthorityEvidenceCorrupt)
		}
		if errors.Is(err, ErrAuthorityMissing) {
			t.Fatalf("broken evidence was reported as an absent grant: %v", err)
		}
		assertNoBinding(t, f, "bind_corrupt")
	})
}

// THE STRICT ACTIVATION, mirrored from the start path. A store that is ARMED
// refuses an intent of another profile; the start calls that CORRUPT, so a door
// that wrote it would hand the operator a binding nothing can ever use.
//
// Evidence level, honest: in-process, one real SQLite file. The store is ARMED
// through its own activation path (`ActivateAuthority` + `RequireAuthorityActivation`),
// which is what makes the branch reachable at all; the foreign profile is then
// set by ASSIGNMENT, because nothing in the tree arms one store for two
// profiles. An earlier version of this comment claimed "not by assignment" over
// a line that assigns — the wire is what it is, and now it says so.
func TestBindWithGrant_anArmedStoreRefusesAnotherProfile(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()
	activation := activateAuthorityFixture(t, f)
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, activation); err != nil {
		t.Fatal(err)
	}

	// The control first: armed on its own profile, the door still works.
	if _, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_armed_ok"), f.root.GrantID, f.now); err != nil {
		t.Fatalf("an armed store refused its OWN profile: %v", err)
	}

	// And now the attack: the same store armed for a different profile.
	if err := f.store.RequireAuthorityActivation(ctx, "profile_somewhere_else", activation); err == nil {
		// If arming a foreign profile is itself refused, that is a stronger
		// guarantee than this mould asks for — but it must be refused, not
		// silently accepted, so the mould says which happened.
		t.Log("arming a foreign profile was accepted; the door must still refuse the bind")
	}
	f.store.authorityProfileID = "profile_somewhere_else"
	_, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_armed_foreign"), f.root.GrantID, f.now)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("err = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
	}
	assertNoBinding(t, f, "bind_armed_foreign")
}

// THE SECOND HALF of the armed branch, which had no mould at all.
//
// `BindExecutionWithGrant` mirrors two checks from the start when the store is
// armed: the intent's profile against the armed one, and
// `verifyAuthorityActivationTx` against the ledger. The mould above attacks the
// FIRST. An internal adversarial pass neutralised the SECOND — `if false` over
// the whole call — and the entire suite of both packages stayed green. A branch
// nobody can redden is a branch nobody guards, and the doctrine is explicit:
// without its captured red the test does not exist.
//
// Here the profile MATCHES, so the first check passes and the second is the only
// thing between the operator and a binding the start would call corrupt.
//
// Evidence level, honest: in-process, one real SQLite file, armed through the
// store's own activation path, with the activation's own evidence deleted by a
// second statement on the same connection.
func TestBindWithGrant_anArmedStoreRefusesABrokenActivation(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()
	activation := activateAuthorityFixture(t, f)
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, activation); err != nil {
		t.Fatal(err)
	}

	// The CONTROL: armed, profile matching, activation intact — the door works.
	if _, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_activation_ok"), f.root.GrantID, f.now); err != nil {
		t.Fatalf("an armed store with an intact activation refused: %v", err)
	}

	// And now the attack: the activation's birth evidence is gone, the profile
	// still matches, and the door must refuse before writing.
	if _, err := f.store.db.Exec(
		`DELETE FROM approval_birth_events WHERE profile_id=?`, f.intent.ProfileID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_activation_broken"), f.root.GrantID, f.now)
	if !errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
		t.Fatalf("err = %v, want %v", err, ErrAuthorizationSnapshotCorrupt)
	}
	assertNoBinding(t, f, "bind_activation_broken")
	// And the start agrees: it is the same verification, so a door that let this
	// through would be writing a row every start refuses.
	if _, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "", f.now.Add(9))); err == nil {
		t.Fatal("the start accepted a broken activation, so this mould proves nothing")
	}
}

// THE WILDCARD, which is the channel class caught a second time.
//
// A grant may carry `"*"` and `containsAuthorityString` honours it, so the
// channel loop accepts ANY binding channel against such a grant. But
// `bindingAuthorityTx` resolves a binding with `channel=?`, a literal match: a
// binding whose own channel is `"*"` is therefore invisible to every start —
// exit 0, an ACTIVE row, `ErrAuthorityMissing` for ever. Same signature as the
// P1 the official pass returned, through a value the wire calls legitimate.
func TestBindWithGrant_refusesTheWildcardAsABindingChannel(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()

	// A grant that really does carry the wildcard, so the channel loop would
	// accept anything: without the guard this bind succeeds.
	if _, err := f.store.db.Exec(
		`UPDATE execution_bindings SET channel='webhook' WHERE binding_id='binding_authority_root'`); err != nil {
		t.Fatal(err)
	}
	b := bindFor(f, "bind_glob")
	b.Channel = "*"
	_, err := f.store.BindExecutionWithGrant(ctx, b, f.root.GrantID, f.now)
	if !errors.Is(err, action.ErrAttenuationViolated) {
		t.Fatalf("err = %v, want %v", err, action.ErrAttenuationViolated)
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("the refusal does not say what is wrong: %v", err)
	}
	assertNoBinding(t, f, "bind_glob")
}

func assertNoBinding(t *testing.T, f authoritySQLiteFixture, id string) {
	t.Helper()
	var n int
	if err := f.store.db.QueryRow(
		`SELECT COUNT(*) FROM execution_bindings WHERE binding_id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a refused bind left %d row(s) behind", n)
	}
}

// SERIALISATION, forced and observed. The first shape of this mould expected a
// LOST RACE — a rival taking the selector between this door's read and its
// commit, answered by a named `ErrBindingRaceLost`.
//
// WHAT THIS MOULD PROVES, and what it does NOT. It parks the rival at
// `authorityBeforeWriter`, which runs BEFORE write ownership is taken — so it
// proves that a rival who lands FIRST is SEEN, revoked in its turn, and never
// overwritten. It does not, and cannot, reach the window between the selector
// read and the insert. An adversarial pass said so plainly, and it was right:
// the first shape of this file claimed "the rival always arrived first" as an
// observation when it was the mould's own construction. The window itself is
// attacked by `TestBindWithGrant_aRivalInsideTheReadInsertWindow`, below.
//
// Evidence level, honest: in-process, one real SQLite file, a SECOND real
// connection committing through it at the store's own pre-write seam.
func TestBindWithGrant_aRivalThatCommitsFirstIsSeenNotOverwritten(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()

	// The CONTROL first: with nothing racing, the bind succeeds.
	if _, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_control"), f.root.GrantID, f.now); err != nil {
		t.Fatalf("control: %v", err)
	}

	rival := make(chan error, 1)
	f.store.authorityBeforeWriter = func() {
		f.store.authorityBeforeWriter = nil // once
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f.store.path))
		if err != nil {
			rival <- err
			return
		}
		defer func() { _ = db.Close() }()
		_, err = db.Exec(`UPDATE execution_bindings SET status='REVOKED' WHERE binding_id='bind_control'`)
		if err == nil {
			_, err = db.Exec(
				`INSERT INTO execution_bindings(binding_id,actor_principal_id,channel,conversation_id,
				  intent_id,intent_version,intent_digest,revision,status)
				  SELECT 'bind_rival',actor_principal_id,channel,conversation_id,intent_id,
				         intent_version,intent_digest,9,'ACTIVE'
				    FROM execution_bindings WHERE binding_id='bind_control'`)
		}
		rival <- err
	}

	revoked, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_after"), f.root.GrantID, f.now)
	if rerr := <-rival; rerr != nil {
		t.Fatalf("the rival writer never landed, so nothing was serialised: %v", rerr)
	}
	if err != nil {
		t.Fatalf("the door refused after a rival committed ahead of it: %v", err)
	}

	// It SAW the rival and closed it, rather than colliding with it or
	// overwriting it.
	if revoked != "bind_rival" {
		t.Fatalf("the door revoked %q, want the rival that held the selector", revoked)
	}
	var rivalStatus, afterStatus string
	var afterRevision int
	if err := f.store.db.QueryRow(
		`SELECT status FROM execution_bindings WHERE binding_id='bind_rival'`).Scan(&rivalStatus); err != nil {
		t.Fatalf("the rival's row is gone: %v", err)
	}
	if rivalStatus != string(action.BindingRevoked) {
		t.Fatalf("the rival's row is %q, want REVOKED and still present", rivalStatus)
	}
	if err := f.store.db.QueryRow(
		`SELECT status,revision FROM execution_bindings WHERE binding_id='bind_after'`).
		Scan(&afterStatus, &afterRevision); err != nil {
		t.Fatal(err)
	}
	if afterStatus != string(action.BindingActive) || afterRevision != 10 {
		t.Fatalf("the new row is %q at revision %d, want ACTIVE counting on from the rival's 9",
			afterStatus, afterRevision)
	}

	// Exactly one ACTIVE row for the selector. The partial unique index would
	// have refused a second, and a door that left two would be worse than one
	// that refused.
	var active int
	if err := f.store.db.QueryRow(
		`SELECT COUNT(*) FROM execution_bindings WHERE status='ACTIVE'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("%d ACTIVE bindings after the race, want exactly 1", active)
	}
}

// sealedStart reads back the SIGNED start record — the canonical bytes the
// store sealed at authorisation time, not a value recomputed afterwards.
func sealedStart(t *testing.T, f authoritySQLiteFixture, actionID string) authorityStartRecord {
	t.Helper()
	var canonical string
	if err := f.store.db.QueryRow(
		`SELECT canonical_start FROM authorization_starts WHERE action_id=?`, actionID).
		Scan(&canonical); err != nil {
		t.Fatalf("the start left no sealed record: %v", err)
	}
	var record authorityStartRecord
	if err := json.Unmarshal([]byte(canonical), &record); err != nil {
		t.Fatalf("sealed start is not the record it claims: %v", err)
	}
	return record
}

// THE VISIBLE EFFECT, and the only oracle that can tell the two paths apart.
//
// Every mould above proves the door writes the row it promises. None of them
// proves the row CHANGES ANYTHING, and that is the whole point of the piece: a
// strict deployment whose binding carries no grant resolves through the config
// clause, and the signed grant beside it is decoration. A binding that merely
// looks right would satisfy the moulds above and leave the fault untouched.
//
// So this one forces the field's exact state — strict authority on, a clause
// covering the operation, a binding with the three grant columns NULL, which is
// precisely what `intent bind` has always written — starts, then binds through
// the door, then starts again, and compares the two SEALED start records.
//
// The oracle is by impossibility rather than by inspection. `config_generation`
// and `grant_chain` are signed into `authorization_starts` at authorisation
// time and never recomputed, so a start that resolved through the clause CANNOT
// produce a record naming the grant, and one that resolved through the grant
// CANNOT produce a non-zero generation: `resolveAuthorityTx` assigns that field
// in the clause branch alone.
//
// Evidence level, honest: in-process, one real SQLite file, the production
// start path, read back from the sealed column.
func TestBindWithGrant_theStartStopsResolvingThroughTheConfigClause(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 9)
	ctx := context.Background()

	manifest, err := f.store.AuthorityActivationManifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reason := "strict authority, the state this door was missing from"
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
		CanonicalAuthorityActivation(f.intent.ProfileID, manifest, reason), f.now.Add(5))
	activation, err := f.store.ActivateAuthority(ctx, f.intent.ProfileID, act, reason, f.now.Add(5))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RequireAuthorityActivation(ctx, f.intent.ProfileID, activation); err != nil {
		t.Fatal(err)
	}

	clause := action.ConfigAuthorityClause{
		SchemaVersion: 1, ProfileID: f.intent.ProfileID,
		BrainPrincipal: f.root.SubjectPrincipalID, ToolName: "probe",
		Channels: []string{"webhook"}, CageDigest: action.HashCanonical("cage-a"),
	}
	clause.ClauseID = "cfg_" + strings.TrimPrefix(clause.Digest(), "sha256:")
	generation, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID,
		f.root.SubjectPrincipalID, []action.ConfigAuthorityClause{clause}, f.now.Add(6))
	if err != nil || generation == 0 {
		t.Fatalf("clause generation = %d, %v", generation, err)
	}

	// What `intent bind` writes, exactly: the selector bound, the grant columns
	// NULL. The fixture ships them filled because no verb could fill them.
	if _, err := f.store.db.Exec(`UPDATE execution_bindings
		SET grant_id=NULL,grant_version=NULL,grant_digest=NULL
		WHERE binding_id='binding_authority_root'`); err != nil {
		t.Fatal(err)
	}

	before, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "through-the-clause", f.now.Add(7)))
	if err != nil {
		t.Fatalf("the clause start this piece exists to replace was refused: %v", err)
	}
	sealedBefore := sealedStart(t, f, before.ActionID)
	if sealedBefore.ConfigGeneration != generation {
		t.Fatalf("before the door, config_generation = %d, want the clause's %d",
			sealedBefore.ConfigGeneration, generation)
	}
	if !strings.Contains(sealedBefore.GrantChain, clause.ClauseID) {
		t.Fatalf("before the door, grant_chain = %q, want the clause id", sealedBefore.GrantChain)
	}
	if strings.Contains(sealedBefore.GrantChain, f.root.GrantID) {
		t.Fatalf("before the door, grant_chain already names the grant: %q", sealedBefore.GrantChain)
	}
	if len(before.AuthorityRefs) != 1 || before.AuthorityRefs[0] != clause.ClauseID {
		t.Fatalf("before the door, the refs the runtime reports = %v, want the clause alone",
			before.AuthorityRefs)
	}

	if _, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_visible"), f.root.GrantID, f.now.Add(8)); err != nil {
		t.Fatalf("the door refused: %v", err)
	}

	after, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "through-the-grant", f.now.Add(9)))
	if err != nil {
		t.Fatalf("a start over the binding the door wrote was refused: %v", err)
	}
	sealedAfter := sealedStart(t, f, after.ActionID)
	if sealedAfter.ConfigGeneration != 0 {
		t.Fatalf("after the door, config_generation = %d: the start still consulted the clause",
			sealedAfter.ConfigGeneration)
	}
	if strings.Contains(sealedAfter.GrantChain, clause.ClauseID) {
		t.Fatalf("after the door, grant_chain still names the clause: %q", sealedAfter.GrantChain)
	}
	if !strings.Contains(sealedAfter.GrantChain, `"grant_id":"`+f.root.GrantID+`"`) {
		t.Fatalf("after the door, grant_chain = %q, want the grant the operator named",
			sealedAfter.GrantChain)
	}
	if len(after.AuthorityRefs) != 1 || after.AuthorityRefs[0] != f.root.GrantID {
		t.Fatalf("after the door, the refs the runtime reports = %v, want the grant alone",
			after.AuthorityRefs)
	}
}

// THE WINDOW ITSELF: a rival driven in between the selector read and the
// insert, which is the only instant at which a lost race could exist.
//
// This is the mould the removal of `ErrBindingRaceLost` actually rests on. The
// class was retired because nobody can reach it, and an adversarial pass showed
// the evidence offered for that did not reach it either — `authorityBeforeWriter`
// parks a door before ownership, not inside the window. `authorityAfterSelectorRead`
// stands exactly there, and a SECOND real connection tries to take the selector
// through it.
//
// What serialises the rival is SQLITE'S SINGLE WRITER, not this door's ordering
// against other authority doors: the transaction is already a write
// transaction, so the rival's own write is refused SQLITE_BUSY until this one
// commits. That is the reason the code now records, because the reason it used
// to record was the wrong one.
//
// Evidence level, honest: in-process, one real SQLite file, a SECOND real
// connection opened on it, driven INSIDE the transaction's read-to-insert
// window. NOT a compiled binary in a separate OS process.
func TestBindWithGrant_aRivalInsideTheReadInsertWindow(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	ctx := context.Background()

	type attempt struct{ update, insert error }
	rival := make(chan attempt, 1)
	f.store.authorityAfterSelectorRead = func() {
		f.store.authorityAfterSelectorRead = nil // once
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(f.store.path)+"?_pragma=busy_timeout(1500)")
		if err != nil {
			rival <- attempt{update: err}
			return
		}
		defer func() { _ = db.Close() }()
		_, uerr := db.Exec(
			`UPDATE execution_bindings SET status='REVOKED' WHERE binding_id='binding_authority_root'`)
		_, ierr := db.Exec(
			`INSERT INTO execution_bindings(binding_id,actor_principal_id,channel,conversation_id,
			  intent_id,intent_version,intent_digest,revision,status)
			  SELECT 'bind_window_rival',actor_principal_id,channel,conversation_id,intent_id,
			         intent_version,intent_digest,7,'ACTIVE'
			    FROM execution_bindings WHERE binding_id='binding_authority_root'`)
		rival <- attempt{update: uerr, insert: ierr}
	}

	revoked, err := f.store.BindExecutionWithGrant(ctx, bindFor(f, "bind_window"), f.root.GrantID, f.now)
	got := <-rival

	// The rival must have been REFUSED, not merely slow. An assert that allowed
	// either outcome would pass over a door that let the write through.
	if got.update == nil {
		t.Fatalf("the rival's UPDATE landed inside the window: the door does not hold the writer")
	}
	if got.insert == nil {
		t.Fatalf("the rival's INSERT landed inside the window: the door does not hold the writer")
	}
	// BOTH statements, and by ONE name. An earlier shape asked the reason of the
	// UPDATE alone and accepted "locked OR busy"; an internal pass pointed out
	// that the INSERT's failure was then guaranteed by the scenario — with the
	// UPDATE refused, the previous row is still ACTIVE, so a second ACTIVE row
	// for the selector collides with the partial unique index whether or not the
	// writer is held. Demanding SQLITE_BUSY of both is the only form that
	// distinguishes "the door holds the writer" from "the index saved us".
	for name, err := range map[string]error{"UPDATE": got.update, "INSERT": got.insert} {
		if !strings.Contains(err.Error(), "SQLITE_BUSY") {
			t.Fatalf("the rival's %s was refused for the wrong reason: %v", name, err)
		}
	}

	if err != nil {
		t.Fatalf("the door failed while the rival was being refused: %v", err)
	}
	if revoked != "binding_authority_root" {
		t.Fatalf("the door revoked %q, want the selector's holder", revoked)
	}
	if n := authorityScalar(t, f.store,
		`SELECT COUNT(*) FROM execution_bindings WHERE binding_id='bind_window_rival'`); n != 0 {
		t.Fatalf("the rival wrote %d row(s) inside the window", n)
	}
	if n := authorityScalar(t, f.store,
		`SELECT COUNT(*) FROM execution_bindings WHERE status='ACTIVE'`); n != 1 {
		t.Fatalf("%d ACTIVE bindings after the window, want exactly 1", n)
	}
}
