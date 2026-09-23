// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

func authorityProbeClause(f authoritySQLiteFixture, channels []string, cage string) action.ConfigAuthorityClause {
	clause := action.ConfigAuthorityClause{
		SchemaVersion: 1, ProfileID: f.intent.ProfileID,
		BrainPrincipal: f.root.SubjectPrincipalID, ToolName: "probe",
		Channels: channels, CageDigest: action.HashCanonical(cage),
	}
	clause.ClauseID = "cfg_" + strings.TrimPrefix(clause.Digest(), "sha256:")
	return clause
}

// TestAuthority_StoreOwnsApplicableLeaf: WHICH authority applies is the store's
// decision and nobody else's. The start request has no field through which a
// caller could name a grant — that half is structural — so the mould attacks
// what is left: a store that picks the wrong leaf by itself.
//
// Evidence level: in-process, one real SQLite store.
// Probing mutation executed: after the chain is read, keep only its broadest
// ancestor as the applicable leaf ("first matching grant") — the narrow bound
// leaf stops governing, and the first row reddens on its refs and on a second
// start that should have been exhausted.
func TestAuthority_StoreOwnsApplicableLeaf(t *testing.T) {
	t.Run("the exactly bound leaf governs, not a broader valid ancestor", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 10)
		leaf := f.delegate(t, "grant_narrow_leaf", f.root.SubjectPrincipalID, 1, f.root)
		putAuthorityBinding(t, f, leaf, "narrow")

		started, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "narrow", f.now.Add(time.Second)))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{f.root.GrantID, leaf.GrantID}
		if !sameStringSlice(started.AuthorityRefs, want) {
			t.Errorf("authority refs = %v, want the verified chain %v", started.AuthorityRefs, want)
		}
		var stored string
		if err := f.store.db.QueryRow(`SELECT authority_refs FROM actions WHERE action_id=?`, started.ActionID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		for _, id := range want {
			if !strings.Contains(stored, id) {
				t.Errorf("persisted authority_refs %q does not name %q", stored, id)
			}
		}
		// The leaf's maximum is 1 and its ancestor still has 9: a second start
		// can only be refused if the LEAF, not the ancestor, is what governs.
		if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "narrow", f.now.Add(2*time.Second))); !errors.Is(err, ErrBudgetExhausted) {
			t.Errorf("second start under the exhausted leaf = %v, want %v", err, ErrBudgetExhausted)
		}
	})

	t.Run("no binding for this actor and channel is authority missing", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 10)
		request := authorityStartRequest(f, "", f.now.Add(time.Second))
		request.Channel = "telegram"
		if _, err := f.store.StartAuthorization(context.Background(), request); !errors.Is(err, ErrAuthorityMissing) {
			t.Errorf("error = %v, want %v", err, ErrAuthorityMissing)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 0 {
			t.Errorf("durable starts = %d, want 0", n)
		}
	})

	// ErrAuthorityAmbiguous is NOT reached by this mould, and that is said
	// rather than hidden: the only door that writes config clauses refuses a
	// second clause for one tool before ambiguity can exist, so the sentinel is
	// a second belt behind a door that already refuses.
	t.Run("two config clauses for one tool are refused at the door that would create the ambiguity", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 10)
		activateAuthorityFixture(t, f)
		_, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID, f.root.SubjectPrincipalID,
			[]action.ConfigAuthorityClause{
				authorityProbeClause(f, []string{"webhook"}, "cage-a"),
				authorityProbeClause(f, []string{"webhook"}, "cage-b"),
			}, f.now.Add(6*time.Nanosecond))
		if !errors.Is(err, action.ErrAuthorityMalformed) {
			t.Errorf("error = %v, want %v", err, action.ErrAuthorityMalformed)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM config_authority_heads`); n != 0 {
			t.Errorf("config heads = %d, want 0: the refused sync advanced a generation", n)
		}
	})
}

// authorityProtectedDoors is the closed inventory of functions that take write
// ownership. A door added, renamed or that stops calling beginAuthorityWrite
// moves this list, and the mould below notices.
var authorityProtectedDoors = []string{
	// BindExecutionWithGrant joined the inventory in v0.16.1. It belongs here
	// for the reason the list exists: it reads the grant head, walks the chain
	// and reads the selector's current holder, and it writes from what it read,
	// so every one of those reads must happen behind the same ownership its
	// INSERT commits under.
	"ActivateAuthority", "BindExecutionWithGrant", "ImportLegacyAuthority", "ParkAuthorization",
	"StartAuthorization", "SyncConfigAuthorityClauses",
	"delegateAuthority", "issueAuthority", "revokeAuthority", "startApprovedAuthorization",
}

// TestAuthority_WriteOwnershipPrecedesProtectedReads watches the ORDER of
// statements, which no behaviour test can see: a door that read first and
// locked second would behave identically until the day a second process
// committed in between.
//
// PERIMETER, said once: a source scan over this package's non-test files. It
// judges (a) that the set of functions calling beginAuthorityWrite is exactly
// the inventory above; (b) that inside each, no database call — on the pool or
// on a transaction — textually precedes the ownership call; and (c) that the
// first statement beginAuthorityWrite itself runs on its transaction is the
// lock UPDATE. It does NOT follow calls into helpers, so a read hidden inside a
// function invoked before the ownership call is outside what it sees.
//
// Evidence level: source-level (go/parser); no store.
// Probing mutations executed, each alone: a pool read placed before the
// ownership call in delegateAuthority; a SELECT placed before the lock UPDATE
// inside beginAuthorityWrite. Both redden, naming the function.
func TestAuthority_WriteOwnershipPrecedesProtectedReads(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	databaseCall := func(call *ast.CallExpr) bool {
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		switch selector.Sel.Name {
		case "QueryContext", "QueryRowContext", "ExecContext", "Query", "QueryRow", "Exec", "BeginTx", "Begin":
		default:
			return false
		}
		switch receiver := selector.X.(type) {
		case *ast.Ident:
			return receiver.Name == "tx"
		case *ast.SelectorExpr:
			return receiver.Sel.Name == "db"
		}
		return false
	}
	var doors []string
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				var ownership token.Pos
				var calls []*ast.CallExpr
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "beginAuthorityWrite" && ownership == token.NoPos {
						ownership = call.Pos()
					}
					if databaseCall(call) {
						calls = append(calls, call)
					}
					return true
				})
				if fn.Name.Name == "beginAuthorityWrite" {
					sort.Slice(calls, func(i, j int) bool { return calls[i].Pos() < calls[j].Pos() })
					first := ""
					for _, call := range calls {
						selector := call.Fun.(*ast.SelectorExpr)
						if receiver, ok := selector.X.(*ast.Ident); !ok || receiver.Name != "tx" {
							continue
						}
						for _, arg := range call.Args {
							if literal, ok := arg.(*ast.BasicLit); ok && literal.Kind == token.STRING {
								first, _ = strconv.Unquote(literal.Value)
								break
							}
						}
						break
					}
					if !strings.HasPrefix(strings.TrimSpace(first), "UPDATE authority_write_lock") {
						t.Errorf("beginAuthorityWrite's first transaction statement is %q, want the lock UPDATE", first)
					}
					continue
				}
				if ownership == token.NoPos {
					continue
				}
				doors = append(doors, fn.Name.Name)
				for _, call := range calls {
					if call.Pos() < ownership {
						t.Errorf("%s: %s makes a database call before it takes write ownership",
							fset.Position(call.Pos()), fn.Name.Name)
					}
				}
			}
		}
	}
	sort.Strings(doors)
	if !sameStringSlice(doors, authorityProtectedDoors) {
		t.Errorf("functions taking write ownership = %v, want exactly %v", doors, authorityProtectedDoors)
	}
}

// TestAuthority_ProtectedReadersUseTransactionReceiver leans on a fact of the
// production store: its pool holds ONE connection. A protected reader that
// queried the pool instead of its transaction would therefore wait for the very
// connection its own transaction holds — forever. Every door is run here under
// a deadline, so that mistake is a named failure rather than a hung suite.
//
// The legacy-import door is not in this table; the readers it shares with the
// doors that are (the actor validation, the intent reader) are the ones the
// mutations below move.
//
// Evidence level: in-process, the real one-connection store.
// Probing mutations executed, each alone — the transaction receiver swapped for
// the pool in: the actor-act reader, the grant reader, the intent-version
// reader, the config-head reader inside the resolver, and the config-clause
// reader. Each reddens this mould: by a row's deadline when the door under test
// is the first to reach the swapped reader, by the watchdog when the fixture
// reaches it first.
func TestAuthority_ProtectedReadersUseTransactionReceiver(t *testing.T) {
	// The fixture itself goes through these doors with no deadline, so a
	// swapped receiver can hang BEFORE any row's own deadline is armed. The
	// watchdog turns that hang into this mould's failure, by name, instead of
	// the suite's ten-minute timeout.
	watchdog := time.AfterFunc(90*time.Second, func() {
		panic("TestAuthority_ProtectedReadersUseTransactionReceiver: a protected reader is waiting for the one connection its own transaction holds")
	})
	defer watchdog.Stop()
	if got := authorityPoolSize(t); got != 1 {
		t.Fatalf("the production pool holds %d connections; this mould's oracle needs exactly 1", got)
	}
	deadline := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		return ctx
	}
	doors := []struct {
		name string
		run  func(*testing.T, context.Context, authoritySQLiteFixture) error
	}{
		{"issue", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			grant := f.root
			grant.GrantID = "grant_second_root"
			act := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", grant.CanonicalBytes(), f.now.Add(11*time.Nanosecond))
			return f.store.IssueAuthority(ctx, grant, act, f.now.Add(time.Second))
		}},
		{"delegate", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			child := authorityChildForDoor(f, "grant_receiver_child")
			act := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", child.CanonicalBytes(), f.now.Add(12*time.Nanosecond))
			return f.store.DelegateAuthority(ctx, child, act, f.now.Add(time.Second))
		}},
		{"revoke", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			act := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke", CanonicalAuthorityRevoke(f.root.GrantID, "stop"), f.now.Add(13*time.Nanosecond))
			return f.store.RevokeAuthority(ctx, f.root.GrantID, act, "stop", f.now.Add(time.Second))
		}},
		{"immediate start", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			_, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "", f.now.Add(time.Second)))
			return err
		}},
		{"activation, config sync and a start under the config clause", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			activateAuthorityFixture(t, f)
			if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID, f.root.SubjectPrincipalID,
				[]action.ConfigAuthorityClause{authorityProbeClause(f, []string{"webhook"}, "cage-a")}, f.now.Add(6*time.Nanosecond)); err != nil {
				return err
			}
			if _, err := f.store.db.ExecContext(ctx, `UPDATE execution_bindings SET grant_id=NULL,grant_version=NULL,grant_digest=NULL WHERE binding_id='binding_authority_root'`); err != nil {
				return err
			}
			_, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "", f.now.Add(time.Second)))
			return err
		}},
		{"pending birth and approved resume", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			parked := parkStrictAuthorityFixture(t, f)
			approval := approveStrictAuthorityFixture(t, f, parked)
			_, err := f.store.StartApprovedAuthorization(ctx, approval.ApprovalID,
				PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
			return err
		}},
		{"bind with a grant", func(t *testing.T, ctx context.Context, f authoritySQLiteFixture) error {
			_, err := f.store.BindExecutionWithGrant(ctx, action.ExecutionBinding{
				BindingID: "bind_receiver", ActorPrincipalID: f.root.SubjectPrincipalID,
				Channel: "webhook", IntentID: f.intent.IntentID, IntentVersion: f.intent.Version,
				IntentDigest: f.intent.Digest(), Revision: 1, Status: action.BindingActive,
			}, f.root.GrantID, f.now.Add(time.Second))
			return err
		}},
	}
	for _, door := range doors {
		t.Run(door.name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 5)
			if err := door.run(t, deadline(t), f); err != nil {
				t.Errorf("the door did not complete on the one-connection store: %v", err)
			}
		})
	}
}

func authorityPoolSize(t *testing.T) int {
	t.Helper()
	f := newAuthoritySQLiteFixture(t, 1)
	return f.store.db.Stats().MaxOpenConnections
}

// TestAuthority_BudgetAccountSurvivesVersionAndReload: a budget account is
// identified by a STABLE scope, never by the terms that may move. Eight starts
// are spent under config generation 1, whose clause admits every channel; the
// clause is then narrowed to `webhook` only — a new clause digest, a new
// generation — and the account must be the same one, still eight spent, with
// room for exactly two more under the intent's maximum of ten.
//
// What this mould does NOT cover, because this phase has no door for it: a
// GRANT moving from version 1 to version 2. insertGrantTx inserts the head, it
// never advances it, so a second version of one grant cannot be written.
//
// Evidence level: in-process, one real SQLite store.
// Probing mutation executed: put the clause's channel selector into the config
// account scope — generation 2 then opens a fresh account, and the mould
// reddens on two config accounts where there must be one.
func TestAuthority_BudgetAccountSurvivesVersionAndReload(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 10)
	ctx := context.Background()
	activateAuthorityFixture(t, f)
	if _, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID, f.root.SubjectPrincipalID,
		[]action.ConfigAuthorityClause{authorityProbeClause(f, []string{"*"}, "cage-a")}, f.now.Add(6*time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_id=NULL,grant_version=NULL,grant_digest=NULL WHERE binding_id='binding_authority_root'`); err != nil {
		t.Fatal(err)
	}
	start := func(i int) error {
		_, err := f.store.StartAuthorization(ctx, authorityStartRequest(f, "", f.now.Add(time.Duration(i+1)*time.Second)))
		return err
	}
	for i := 0; i < 8; i++ {
		if err := start(i); err != nil {
			t.Fatalf("start %d under generation 1: %v", i, err)
		}
	}
	generation, err := f.store.SyncConfigAuthorityClauses(ctx, f.intent.ProfileID, f.root.SubjectPrincipalID,
		[]action.ConfigAuthorityClause{authorityProbeClause(f, []string{"webhook"}, "cage-a")}, f.now.Add(20*time.Second))
	if err != nil || generation != 2 {
		t.Fatalf("narrowed reload = generation %d, %v; want generation 2", generation, err)
	}
	for i := 8; i < 10; i++ {
		if err := start(i); err != nil {
			t.Errorf("start %d under generation 2: %v — the two units left did not fit", i, err)
		}
	}
	if err := start(10); !errors.Is(err, ErrBudgetExhausted) {
		t.Errorf("eleventh start = %v, want %v", err, ErrBudgetExhausted)
	}
	if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_accounts WHERE scope_kind='config'`); n != 1 {
		t.Errorf("config accounts = %d, want 1: the reload opened a new account", n)
	}
	account := budgetAccountID(f.intent.ProfileID, "config", configAccountScope(f.root.SubjectPrincipalID, "probe"))
	if spent := authorityScalar(t, f.store, `SELECT spent FROM budget_counters WHERE account_id=? AND operation_key='*'`, account); spent != 10 {
		t.Errorf("config account total spent = %d, want 10 across both generations", spent)
	}
}

// TestAuthority_ActivationRequiresAuthenticatedOneShotAct: the trust root is an
// administrative ACT, and an act is authenticated, exact, human and single-use.
// Every row below must leave NO root behind.
//
// Evidence level: in-process, one real SQLite store. The spec asks for a child
// OS process creating and closing the act; that is FILED — what this mould
// proves is the door's own judgement, not survival across a restart.
// Probing mutation executed: skip validateAuthorityActorTx inside
// ActivateAuthority — every refusal row then activates, red on each.
func TestAuthority_ActivationRequiresAuthenticatedOneShotAct(t *testing.T) {
	const reason = "enable strict authority"
	manifestOf := func(t *testing.T, f authoritySQLiteFixture) string {
		t.Helper()
		manifest, err := f.store.AuthorityActivationManifest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	tests := []struct {
		name string
		want error
		act  func(*testing.T, authoritySQLiteFixture) string
	}{
		{"no act at all", identity.ErrIdentityEvidenceMissing,
			func(*testing.T, authoritySQLiteFixture) string { return "" }},
		{"an act id that was never recorded", ErrNotFound,
			func(*testing.T, authoritySQLiteFixture) string { return "act_never_recorded" }},
		{"an act for another operation", identity.ErrIdentityBindingMismatch,
			func(t *testing.T, f authoritySQLiteFixture) string {
				return authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
					CanonicalAuthorityActivation(f.intent.ProfileID, manifestOf(t, f), reason), f.now.Add(21*time.Nanosecond))
			}},
		{"an act over another manifest", identity.ErrIdentityBindingMismatch,
			func(t *testing.T, f authoritySQLiteFixture) string {
				return authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
					CanonicalAuthorityActivation(f.intent.ProfileID, "sha256:some-other-manifest", reason), f.now.Add(22*time.Nanosecond))
			}},
		{"an act already consumed by another mutation", identity.ErrIdentityBindingMismatch,
			func(t *testing.T, f authoritySQLiteFixture) string {
				// The act that issued the fixture's root grant: authentic,
				// human-backed, and spent.
				var spent string
				if err := f.store.db.QueryRow(`SELECT actor_action_id FROM grant_events WHERE grant_id=?`, f.root.GrantID).Scan(&spent); err != nil {
					t.Fatal(err)
				}
				return spent
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			digest, err := f.store.ActivateAuthority(context.Background(), f.intent.ProfileID, tt.act(t, f), reason, f.now.Add(time.Second))
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if digest != "" {
				t.Errorf("activation digest = %q, want none", digest)
			}
			for what, query := range map[string]string{
				"birth heads":  `SELECT COUNT(*) FROM approval_birth_heads`,
				"birth events": `SELECT COUNT(*) FROM approval_birth_events`,
			} {
				if n := authorityScalar(t, f.store, query); n != 0 {
					t.Errorf("%s = %d, want 0: a refused activation left a root behind", what, n)
				}
			}
		})
	}
	t.Run("the exact act activates once and names its human operator", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		digest := activateAuthorityFixture(t, f)
		var canonical string
		if err := f.store.db.QueryRow(`SELECT canonical_event FROM approval_birth_events WHERE sequence=0`).Scan(&canonical); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(canonical, `"Actor":"principal_responsible"`) || !strings.Contains(canonical, reason) {
			t.Errorf("signed activation root %s does not name its human operator and reason", canonical)
		}
		if digest == "" {
			t.Error("activation returned no digest to pin")
		}
	})
}

// TestAuthority_ApprovedResumeUsesStartAuthorization is the phase's central
// attack: a human approves, and BETWEEN the approval and the resume the
// authority is withdrawn or spent. The approval must not resurrect it. Each row
// demands its named sentinel and, as the oracle by impossibility, the parked
// parameters STILL THERE — a claim that refuses must not have consumed them —
// with no debit and no durable start.
//
// Evidence level: in-process, one real SQLite store; the revocation and the
// spend are committed through the store's own doors before the resume begins.
// Probing mutations executed, each alone: skip the authority re-resolution on
// resume — red, but by another belt's sentinel («budget evidence corrupt»), not
// by a start; and drop the comparison with the signed pending snapshot — the
// rewritten-action row reddens, again by sentinel: the tampered row is then
// called «authority revoked», which is false, and that guard is what names it
// truthfully.
func TestAuthority_ApprovedResumeUsesStartAuthorization(t *testing.T) {
	tests := []struct {
		name     string
		want     error
		withdraw func(*testing.T, authoritySQLiteFixture)
	}{
		{"the authority is revoked after the approval", ErrAuthorityRevoked,
			func(t *testing.T, f authoritySQLiteFixture) {
				act := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
					CanonicalAuthorityRevoke(f.root.GrantID, "stop"), f.now.Add(31*time.Nanosecond))
				if err := f.store.RevokeAuthority(context.Background(), f.root.GrantID, act, "stop", f.now.Add(1500*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
			}},
		{"the last unit is spent after the approval", ErrBudgetExhausted,
			func(t *testing.T, f authoritySQLiteFixture) {
				if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "", f.now.Add(1500*time.Millisecond))); err != nil {
					t.Fatal(err)
				}
			}},
		{"the action row is rewritten to another intent than the SIGNED pending snapshot names", ErrAuthorizationSnapshotCorrupt,
			func(t *testing.T, f authoritySQLiteFixture) {
				externalIdentityExec(t, f.store, `UPDATE actions SET intent_id='int_somebody_elses' WHERE action_id LIKE 'act3_%'`)
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 1)
			parked := parkStrictAuthorityFixture(t, f)
			approval := approveStrictAuthorityFixture(t, f, parked)
			tt.withdraw(t, f)
			spentBefore := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`)

			_, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
				PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			var params string
			if err := f.store.db.QueryRow(`SELECT canonical_params FROM approvals WHERE approval_id=?`, approval.ApprovalID).Scan(&params); err != nil {
				t.Fatal(err)
			}
			if params != `{}` {
				t.Errorf("parked parameters after the refused resume = %q, want them intact", params)
			}
			if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, parked.ActionID); n != 0 {
				t.Errorf("durable starts for the refused action = %d, want 0", n)
			}
			if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != spentBefore {
				t.Errorf("debits moved from %d to %d across a refused resume", spentBefore, n)
			}
		})
	}
}

func parkAuthorityUnderConversation(t *testing.T, f authoritySQLiteFixture, conversationKey string) AuthorityPendingResult {
	t.Helper()
	requestID := "request-pending-" + conversationKey
	ingress, err := f.issuer.Issue(requestID, "verified-subject")
	if err != nil {
		t.Fatal(err)
	}
	parked, err := f.store.ParkAuthorization(context.Background(), AuthorityPendingRequest{
		ActorPrincipalID: "principal_brain_alpha", CorrelationID: requestID, SourceProtocol: "native",
		Channel: "webhook", ConversationID: conversationKey,
		Operation: action.Operation{Namespace: "tool", Name: "probe", Version: 1}, Arguments: `{}`,
		EffectClass: action.EffectWriteReversible, At: f.now,
		ApprovalContext: action.ApprovalContext{
			ToolCage: "probe", Descriptor: action.EffectDescriptor{Class: action.EffectWriteReversible},
			HasDescriptor: true, LawVersion: 1, LawDigest: "sha256:authority-law", TTL: time.Minute,
		},
		ResolveEvidence: func(actionID string) (identity.Evidence, error) {
			return f.resolver.Resolve(ingress, identity.ResolveRequest{
				ActionID: actionID, RequestID: requestID, Channel: "webhook", Brain: "alpha",
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return parked
}

// TestAuthority_ApprovedResumeKeepsConversationScope pins a defect the delivery
// session found in the phase's central flow. The pending birth resolved
// authority under the request's conversation; the approved resume re-resolved
// it under the EMPTY conversation, which selects another binding. A request
// parked under a conversation-scoped leaf could therefore never start: the refs
// no longer matched and it was refused as «authority revoked» — when nothing
// had been revoked. Fail-closed, and a lie in the taxonomy.
//
// The conversation key now rides inside the SIGNED pending snapshot, and the
// resume resolves under it. Both directions are demanded, because one alone
// proves nothing: the scoped request STARTS under its own leaf, and a leaf
// genuinely revoked after the approval still refuses — this time truthfully.
//
// Evidence level: in-process, one real SQLite store.
// Probing mutation executed: resolve the resume under "" again — the first row
// reddens with the false «authority revoked».
func TestAuthority_ApprovedResumeKeepsConversationScope(t *testing.T) {
	const conversationKey = "webhook::scoped"
	law := PolicyPin{Version: 1, Digest: "sha256:authority-law"}

	t.Run("a request parked under a conversation-scoped leaf starts under that leaf", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		leaf := f.delegate(t, "grant_scoped_leaf", f.root.SubjectPrincipalID, 2, f.root)
		putAuthorityBinding(t, f, leaf, conversationKey)
		parked := parkAuthorityUnderConversation(t, f, conversationKey)
		approval := approveStrictAuthorityFixture(t, f, parked)

		started, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID, law,
			approval.ActionDigest, f.now.Add(2*time.Second))
		if err != nil {
			t.Fatalf("resume under the scoped leaf = %v, want a start", err)
		}
		if started.ActionID != parked.ActionID {
			t.Errorf("resumed action = %q, want the parked %q", started.ActionID, parked.ActionID)
		}
		leafAccount := budgetAccountID(leaf.ProfileID, "grant", leaf.GrantID)
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits WHERE action_id=? AND account_id=?`, parked.ActionID, leafAccount); n == 0 {
			t.Error("the resume debited nothing from the scoped leaf: it started under another authority")
		}
	})

	t.Run("the scoped leaf revoked after the approval refuses, and this time it is true", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 5)
		leaf := f.delegate(t, "grant_scoped_leaf", f.root.SubjectPrincipalID, 2, f.root)
		putAuthorityBinding(t, f, leaf, conversationKey)
		parked := parkAuthorityUnderConversation(t, f, conversationKey)
		approval := approveStrictAuthorityFixture(t, f, parked)
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "revoke",
			CanonicalAuthorityRevoke(leaf.GrantID, "stop"), f.now.Add(41*time.Nanosecond))
		if err := f.store.RevokeAuthority(context.Background(), leaf.GrantID, act, "stop", f.now.Add(1500*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID, law,
			approval.ActionDigest, f.now.Add(2*time.Second)); !errors.Is(err, ErrAuthorityRevoked) {
			t.Errorf("error = %v, want %v", err, ErrAuthorityRevoked)
		}
		if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts WHERE action_id=?`, parked.ActionID); n != 0 {
			t.Errorf("durable starts = %d, want 0", n)
		}
	})
}

// authorityBindLeaf binds the AUTHENTICATED actor of these fixtures, on one
// channel and conversation key, to an arbitrary grant triple — including one
// that names a grant which does not exist.
func authorityBindLeaf(t *testing.T, f authoritySQLiteFixture, bindingID, channel, conversationKey string,
	intent action.IntentContractV2, grantID string, grantVersion int, grantDigest string) {
	t.Helper()
	if err := f.store.PutExecutionBinding(context.Background(), action.ExecutionBinding{
		BindingID: bindingID, ActorPrincipalID: "principal_brain_alpha", Channel: channel,
		ConversationID: conversationKey, IntentID: intent.IntentID, IntentVersion: intent.Version,
		IntentDigest: intent.Digest(), GrantID: grantID, GrantVersion: grantVersion, GrantDigest: grantDigest,
		Revision: 1, Status: action.BindingActive,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAuthority_StartRefusalTaxonomy walks the refusals of the strict start that
// no other mould reached, one per row, and demands for each its NAMED class —
// the stable taxonomy is a promise to operators, and a class nobody has seen
// returned is a class nobody knows is reachable. Every row also demands that
// nothing was started and nothing was spent.
//
// It is the in-process, store-door form of the commissioned
// TestAuthority_StrictEffectRequiresPrincipalIntentAndAuthority.
//
// Evidence level: in-process, one real SQLite store, through StartAuthorization.
// Probing mutations executed, ONE PER ROW, each row's own guard neutralized
// alone. Four redden with the forbidden state observed — «error = <nil>», a
// durable start, debits spent. Two redden only by SENTINEL, because a second
// belt still refuses under another class: the ghost leaf comes back as a raw
// «sql: no rows», and the foreign channel as «ingress binding mismatch» from
// the identity evidence. TWO ROWS WERE RE-AIMED, because their first mutation
// SURVIVED and a mutation that survives is the finding: see the comments on
// the intent-approval row and on the unpinned-process row.
func TestAuthority_StartRefusalTaxonomy(t *testing.T) {
	tests := []struct {
		name  string
		want  error
		setup func(*testing.T, authoritySQLiteFixture) AuthorityStartRequest
	}{
		// Under a GRANT chain this guard is shadowed: attenuation forces every root
		// of an approval-requiring intent to require approval itself, so the
		// grant's own guard refuses first with the same sentinel — the first
		// version of this row did exactly that, and its mutation SURVIVED. The
		// intent's guard is the ONLY belt where there is no chain at all: under
		// config-clause authority. That is where this row stands.
		{"the INTENT requires approval, under config-clause authority with no grant chain", ErrAuthorityApprovalRequired,
			func(t *testing.T, f authoritySQLiteFixture) AuthorityStartRequest {
				activateAuthorityFixture(t, f)
				if _, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID, f.root.SubjectPrincipalID,
					[]action.ConfigAuthorityClause{authorityProbeClause(f, []string{"webhook"}, "cage-a")}, f.now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
				guarded := f.intent
				guarded.IntentID, guarded.Purpose = "int_needs_approval", "Every start needs a human"
				guarded.Approval.Required = true
				if err := f.store.CreateIntentV2(context.Background(), guarded, guarded.OwnerPrincipalID, f.now.Add(-time.Minute)); err != nil {
					t.Fatal(err)
				}
				if err := f.store.ActivateIntentV2(context.Background(), guarded.IntentID, guarded.Version, guarded.OwnerPrincipalID, f.now); err != nil {
					t.Fatal(err)
				}
				authorityBindLeaf(t, f, "binding_needs_approval", "webhook", "webhook::guarded", guarded, "", 0, "")
				return authorityStartRequest(f, "webhook::guarded", f.now.Add(3*time.Second))
			}},
		{"a GRANT in the chain requires approval and this is an immediate start", ErrAuthorityApprovalRequired,
			func(t *testing.T, f authoritySQLiteFixture) AuthorityStartRequest {
				child := authorityChildForDoor(f, "grant_child_needs_approval")
				child.Approval.Required = true
				act := authorityActorAct(t, f.store, f.resolver, f.issuer, "delegate", child.CanonicalBytes(), f.now.Add(52*time.Nanosecond))
				if err := f.store.DelegateAuthority(context.Background(), child, act, f.now); err != nil {
					t.Fatal(err)
				}
				authorityBindLeaf(t, f, "binding_child_needs_approval", "webhook", "webhook::guarded-child", f.intent, child.GrantID, child.Version, child.Digest())
				return authorityStartRequest(f, "webhook::guarded-child", f.now.Add(time.Second))
			}},
		{"the binding names a leaf that does not exist", ErrAuthorityMissing,
			func(t *testing.T, f authoritySQLiteFixture) AuthorityStartRequest {
				authorityBindLeaf(t, f, "binding_ghost_leaf", "webhook", "webhook::ghost", f.intent, "grant_that_was_never_issued", 1, "sha256:no-such-grant")
				return authorityStartRequest(f, "webhook::ghost", f.now.Add(time.Second))
			}},
		// The first version of this row used a store with no clauses at all, and
		// its mutation SURVIVED: with the guard gone, the clause lookup found no
		// head and returned the same sentinel. What the guard really protects is
		// a process that never PINNED the activation reading clauses another
		// process wrote — config authority honoured with its trust root
		// unverified. So the clauses exist here, and the start comes from a
		// second store on the same file that pinned nothing. (The guard's other
		// half, a non-tool namespace, is watched by no row: the intent's own
		// operation check refuses such a request first.)
		{"clauses exist, but THIS process never pinned the activation", ErrAuthorityMissing,
			func(t *testing.T, f authoritySQLiteFixture) AuthorityStartRequest {
				activateAuthorityFixture(t, f)
				if _, err := f.store.SyncConfigAuthorityClauses(context.Background(), f.intent.ProfileID, f.root.SubjectPrincipalID,
					[]action.ConfigAuthorityClause{authorityProbeClause(f, []string{"webhook"}, "cage-a")}, f.now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.db.Exec(`UPDATE execution_bindings SET grant_id=NULL,grant_version=NULL,grant_digest=NULL WHERE binding_id='binding_authority_root'`); err != nil {
					t.Fatal(err)
				}
				return authorityStartRequest(f, "", f.now.Add(3*time.Second))
			}},
		{"the request arrives on a channel the grant does not name", action.ErrAttenuationViolated,
			func(t *testing.T, f authoritySQLiteFixture) AuthorityStartRequest {
				authorityBindLeaf(t, f, "binding_other_channel", "telegram", "", f.intent, f.root.GrantID, f.root.Version, f.root.Digest())
				request := authorityStartRequest(f, "", f.now.Add(time.Second))
				request.Channel = "telegram"
				return request
			}},
		{"the bound leaf belongs to another subject", ErrIssuerMismatch,
			func(t *testing.T, f authoritySQLiteFixture) AuthorityStartRequest {
				worker := f.delegate(t, "grant_for_the_worker", "principal_worker", 2, f.root)
				authorityBindLeaf(t, f, "binding_someone_elses_leaf", "webhook", "webhook::borrowed", f.intent, worker.GrantID, worker.Version, worker.Digest())
				return authorityStartRequest(f, "webhook::borrowed", f.now.Add(time.Second))
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 5)
			request := tt.setup(t, f)
			starter := f.store
			if strings.Contains(tt.name, "never pinned the activation") {
				unpinned, err := Open(f.store.path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = unpinned.Close() })
				unpinned.authoritySigner = f.store.authoritySigner
				unpinned.SetIdentitySigners(
					func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(f.private, e) },
					func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
						return identity.SignPrincipalEvent(f.private, e)
					},
				)
				unpinned.identityNow = f.store.identityNow
				starter = unpinned
			}
			if _, err := starter.StartAuthorization(context.Background(), request); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			for what, query := range map[string]string{
				"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
				"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
				"strict actions": `SELECT COUNT(*) FROM actions WHERE action_id LIKE 'act3_%'`,
			} {
				if n := authorityScalar(t, f.store, query); n != 0 {
					t.Errorf("%s = %d, want 0", what, n)
				}
			}
		})
	}
}

// TestAuthority_IssuedDigestIsTheAuthorizedDigest: what the store persists and
// signs is EXACTLY what the authenticated act authorized. A root built with an
// absent per-operation map used to be stored under another digest, because the
// door normalized the map and the encoding told nil from empty.
//
// Evidence level: in-process, one real SQLite store, through IssueAuthority and
// then StartAuthorization under a binding made with the AUTHORIZED digest.
// Probing mutation executed: the same one as its domain sister — the encoding
// tells absent from empty again — red on the stored digest and on the start.
func TestAuthority_IssuedDigestIsTheAuthorizedDigest(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 5)
	// An intent with a TOTAL and no per-operation limits, so that a root may
	// legitimately carry no per-operation map at all.
	totalOnly := f.intent
	totalOnly.IntentID, totalOnly.Purpose = "int_total_only", "A total and no per-operation limits"
	totalOnly.Budget.PerOperation = nil
	if err := f.store.CreateIntentV2(context.Background(), totalOnly, totalOnly.OwnerPrincipalID, f.now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ActivateIntentV2(context.Background(), totalOnly.IntentID, totalOnly.Version, totalOnly.OwnerPrincipalID, f.now); err != nil {
		t.Fatal(err)
	}
	root := f.root
	root.GrantID, root.IntentID, root.IntentDigest = "grant_absent_per_operation", totalOnly.IntentID, totalOnly.Digest()
	root.Budget.PerOperation = nil
	act := authorityActorAct(t, f.store, f.resolver, f.issuer, "issue", root.CanonicalBytes(), f.now.Add(61*time.Nanosecond))
	if err := f.store.IssueAuthority(context.Background(), root, act, f.now); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := f.store.db.QueryRow(`SELECT digest FROM grant_versions WHERE grant_id=?`, root.GrantID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != root.Digest() {
		t.Errorf("stored digest = %s, want the authorized %s", stored, root.Digest())
	}
	authorityBindLeaf(t, f, "binding_absent_per_operation", "webhook", "webhook::absent-map", totalOnly, root.GrantID, root.Version, root.Digest())
	if _, err := f.store.StartAuthorization(context.Background(), authorityStartRequest(f, "webhook::absent-map", f.now.Add(time.Second))); err != nil {
		t.Errorf("a start under a binding made with the authorized digest = %v, want a start", err)
	}
}

// TestAuthority_ApprovedResumeRefusesRewrittenParameters: the bytes a human
// approved are the bytes that run. Between the approval and the resume the
// parked parameters are rewritten, from a SECOND REAL CONNECTION, into other
// well-formed bytes. The resume must refuse by name, start nothing, spend
// nothing — and hand nobody the rewritten bytes.
//
// Evidence level: MULTIPLE REAL SQLITE CONNECTIONS (the rewrite commits through
// its own connection before the resume begins); in-process.
// Probing mutation executed: drop the parameters-digest comparison — the resume
// then STARTS and returns the rewritten bytes, red.
func TestAuthority_ApprovedResumeRefusesRewrittenParameters(t *testing.T) {
	f := newAuthoritySQLiteFixture(t, 2)
	parked := parkStrictAuthorityFixture(t, f)
	approval := approveStrictAuthorityFixture(t, f, parked)
	externalIdentityExec(t, f.store, `UPDATE approvals SET canonical_params='{"rewritten":true}' WHERE approval_id=?`, approval.ApprovalID)

	started, err := f.store.StartApprovedAuthorization(context.Background(), approval.ApprovalID,
		PolicyPin{Version: 1, Digest: "sha256:authority-law"}, approval.ActionDigest, f.now.Add(2*time.Second))
	if !errors.Is(err, ErrApprovalParamsDigestMismatch) {
		t.Errorf("error = %v, want %v", err, ErrApprovalParamsDigestMismatch)
	}
	if len(started.Params) != 0 {
		t.Errorf("the refused resume handed out parameters %q", started.Params)
	}
	for what, query := range map[string]string{
		"durable starts": `SELECT COUNT(*) FROM authorization_starts`,
		"budget debits":  `SELECT COUNT(*) FROM budget_debits`,
	} {
		if n := authorityScalar(t, f.store, query); n != 0 {
			t.Errorf("%s = %d, want 0", what, n)
		}
	}
}
