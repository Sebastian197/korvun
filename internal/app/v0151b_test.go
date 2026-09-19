// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.1 block B — the server moulds. RED: written before any cure.
//
// Each reproduction is Codex's, from docs/HANDOFF.md «Tren de la v0.15.1»:
// P2-6, P2-7, P2-8, P2-9, plus the sisters the pre-test paper names. Every
// test states its own evidence level in its godoc.
//
// Evidence levels, per test and honest: a REAL store on disk opened by the
// adapter, with every attacker writing through a SECOND *sql.DB on the same
// file (a real second connection). P2-7 alone goes over a real TCP socket on a
// non-loopback interface of the SAME host: a real peer address the server did
// not choose, but not a second machine.

package app

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// ---------------------------------------------------------------------------
// P2-8 · current_law_digest belongs to the approval's own brain.
// ---------------------------------------------------------------------------

// threeBrainProfile is parkingProfile with three agent brains whose laws all
// DIFFER — Codex's setup. The owner of the request is placed in the MIDDLE, so
// neither a first-brain nor a last-brain resolver can answer its law by
// accident.
func threeBrainProfile(t *testing.T) (*config.Config, *actionsqlite.Store, func()) {
	t.Helper()
	cfg, store, done := parkingProfile(t)
	base := cfg.Brains[0]
	mk := func(name string, tools ...string) config.BrainConfig {
		b := base
		b.Name = name
		agent := *base.Agent
		agent.Tools = tools
		b.Agent = &agent
		return b
	}
	cfg.Brains = []config.BrainConfig{
		mk("a", "webhook_call"),
		mk("b", "webhook_call", "time_now"),
		mk("c", "webhook_call", "calc"),
	}
	cfg.Routes = []config.RouteConfig{{Channel: "telegram", Brain: "a"}}
	return cfg, store, done
}

// parkForBrain parks one request for the named brain under the law that brain
// resolves right now, signed by principal_brain_<name>.
func parkForBrain(t *testing.T, cfg *config.Config, store *actionsqlite.Store, brain, actionID string) action.Approval {
	t.Helper()
	_, pin, err := ResolveApprovalLaw(cfg, brain)
	if err != nil {
		t.Fatalf("resolve %s's law: %v", brain, err)
	}
	rawParams := `{"a":1}`
	env := action.NewEnvelope(actionID, "env-"+brain,
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		rawParams, time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC))
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_" + brain}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	bound, err := action.NewBoundApprovalRequest(env, rawParams, action.ApprovalContext{
		IntentPurpose: "avisar al webhook de pedidos",
		GrantID:       "grant_1", GrantDepth: 1, CostLine: "1 of 5",
		ToolCage:      "webhook_call",
		Descriptor:    action.EffectDescriptor{Class: action.EffectWriteIrreversible, DataEgress: true},
		HasDescriptor: true,
		LawVersion:    pin.Version, LawDigest: pin.Digest,
		Rule: "require_approval", Now: time.Now().UTC(), TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := store.CreateApprovalRequest(context.Background(), bound); err != nil {
		t.Fatalf("park: %v", err)
	}
	return bound.Approval()
}

// TestV0151B_P2_8_theCurrentLawIsTheApprovalsOwnBrains is Codex's reproduction:
// three brains with different laws, a request parked for the MIDDLE one (b),
// only b's law moved. Every door that names `invalidated` must carry b's
// CURRENT digest.
//
// The third door, nameClaim → nameTouch → currentLawDigest, has its own
// deterministic mould: TestV0151B_P2_8_theClaimDoorNeverPublishesAnotherBrainsLaw.
//
// Evidence: real store file and real profile, in-process.
// Probing mutation (planned): resolve from cfg.Brains[0] ⇒ both rows red; from
// cfg.Brains[len-1] ⇒ both rows red.
func TestV0151B_P2_8_theCurrentLawIsTheApprovalsOwnBrains(t *testing.T) {
	doors := map[string]func(ad *ApprovalsAdapter, a action.Approval) error{
		"detail": func(ad *ApprovalsAdapter, a action.Approval) error {
			_, err := ad.Detail(context.Background(), a.ApprovalID)
			return err
		},
		"approve": func(ad *ApprovalsAdapter, a action.Approval) error {
			_, err := ad.Approve(context.Background(), a.ApprovalID, a.ActionDigest)
			return err
		},
	}
	for name, door := range doors {
		t.Run(name, func(t *testing.T) {
			cfg, store, done := threeBrainProfile(t)
			defer done()
			a := parkForBrain(t, cfg, store, "b", "act_p28_"+name)
			// Move ONLY b's law, to one no other brain has.
			cfg.Brains[1].Agent.Tools = []string{"webhook_call", "time_now", "calc"}
			pins := map[string]string{}
			for _, n := range []string{"a", "b", "c"} {
				_, pin, err := ResolveApprovalLaw(cfg, n)
				if err != nil {
					t.Fatalf("resolve %s: %v", n, err)
				}
				pins[n] = pin.Digest
			}
			if pins["a"] == pins["b"] || pins["c"] == pins["b"] || pins["a"] == pins["c"] {
				t.Fatalf("instrument: the three laws must all differ: %v", pins)
			}

			err := door(NewApprovalsAdapter(cfg, store), a)
			if !errors.Is(err, controlapi.ErrApprovalInvalidated) {
				t.Fatalf("err = %v, want ErrApprovalInvalidated", err)
			}
			var moved interface{ CurrentLawDigest() string }
			if !errors.As(err, &moved) {
				t.Fatalf("the refusal must carry the current law in its own field")
			}
			if got := moved.CurrentLawDigest(); got != pins["b"] {
				t.Fatalf("current_law_digest = %q, want b's %q (a %q, c %q)", got, pins["b"], pins["a"], pins["c"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// P2-9 · a driver failure inside the transition is not `already_closed`.
// ---------------------------------------------------------------------------

// forceTransitionFailure installs Codex's trigger, scoped to ONE action so it
// cannot fire on any other write the decide makes (the operator's own act).
func forceTransitionFailure(t *testing.T, store *actionsqlite.Store, actionID string) (drop func()) {
	t.Helper()
	// SQLite triggers take no bound variables; the id is a test-owned literal.
	attackerExec(t, store, `CREATE TRIGGER forced_driver_failure BEFORE UPDATE OF state ON actions
	  WHEN OLD.action_id = '`+actionID+`'
	  BEGIN SELECT RAISE(ABORT, 'forced driver failure'); END`) // #nosec G202
	return func() { attackerExec(t, store, `DROP TRIGGER forced_driver_failure`) }
}

func actionStateOf(t *testing.T, store *actionsqlite.Store, actionID string) string {
	t.Helper()
	var s string
	if err := attackerDB(t, store).QueryRow(`SELECT state FROM actions WHERE action_id = ?`, actionID).Scan(&s); err != nil {
		t.Fatalf("read state: %v", err)
	}
	return s
}

// TestV0151B_P2_9_aDriverFailureInsideTheTransitionIsNotAlreadyClosed is Codex's
// reproduction at the wire's seam. The transaction rolls back whole — nothing
// was decided — and a driver failure is the transient `unavailable`, never the
// permanent `already_closed` it is published as today. After the trigger is
// dropped the SAME request must still be approvable.
//
// Probing mutation (planned): wrap the exec error of transitionTx with
// ErrApprovalActionNotPending again ⇒ this reddens.
// Evidence: real store file, attacker writes through a second real
// connection, in-process.
func TestV0151B_P2_9_aDriverFailureInsideTheTransitionIsNotAlreadyClosed(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_p29")
	drop := forceTransitionFailure(t, store, "act_p29")

	_, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if errors.Is(err, controlapi.ErrApprovalAlreadyClosed) {
		t.Fatalf("err = %v: a driver failure published as already_closed", err)
	}
	if !errors.Is(err, controlapi.ErrApprovalsUnavailable) {
		t.Fatalf("err = %v, want ErrApprovalsUnavailable — nothing was decided, retry", err)
	}
	after, _, gerr := store.GetApproval(context.Background(), a.ApprovalID)
	if gerr != nil {
		t.Fatalf("re-read: %v", gerr)
	}
	if after.Status != action.ApprovalPending {
		t.Fatalf("approval status = %q, want PENDING", after.Status)
	}
	if s := actionStateOf(t, store, "act_p29"); s != string(action.StatePendingApproval) {
		t.Fatalf("action state = %q, want PENDING_APPROVAL", s)
	}

	drop()
	_, pin, err := ResolveApprovalLaw(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	env, ident, err := approvalsOperator(a.ApprovalID)
	if err != nil {
		t.Fatalf("operator: %v", err)
	}
	// The decide alone — no execution, so no network.
	rule, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID,
		action.DecisionApproved, time.Now().UTC(), env, ident, "", pin)
	if err != nil || rule != "" {
		t.Fatalf("after the trigger is gone the request must still be approvable: rule=%q err=%v", rule, err)
	}
}

// TestV0151B_P2_9_storeLevelTheDriverErrorIsNotTypedNotPending pins the same
// fact one door down, where the lie is born: the store wraps the driver's
// failure in ErrApprovalActionNotPending. The driver's own message must survive
// and the not-pending sentinel must not appear.
// Evidence: real store file, attacker writes through a second real
// connection, in-process.
func TestV0151B_P2_9_storeLevelTheDriverErrorIsNotTypedNotPending(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_p29s")
	_ = forceTransitionFailure(t, store, "act_p29s")
	_, pin, err := ResolveApprovalLaw(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	env, ident, err := approvalsOperator(a.ApprovalID)
	if err != nil {
		t.Fatalf("operator: %v", err)
	}
	_, err = store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID,
		action.DecisionApproved, time.Now().UTC(), env, ident, "", pin)
	if err == nil {
		t.Fatal("the forced failure must refuse the decide")
	}
	if errors.Is(err, actionsqlite.ErrApprovalActionNotPending) {
		t.Fatalf("err = %v: a driver failure typed as «the action was no longer pending»", err)
	}
	if !strings.Contains(err.Error(), "forced driver failure") {
		t.Fatalf("err = %v: the driver's own failure must survive in the chain", err)
	}
}

// TestV0151B_P2_9_sister_rejectOverAMovedActionIsAlreadyClosed is the other
// face of the same function. On the REJECT path transitionTx's «row not in
// expected state» reaches the adapter UNTYPED and is published as the
// transient `unavailable` — «retry» over an action that is already terminal
// and will refuse forever. The honest name is `already_closed`, the one Approve
// already gives the same state.
//
// Probing mutation (planned): leave the zero-rows branch untyped ⇒ this reddens.
// Evidence: real store file, attacker writes through a second real
// connection, in-process.
func TestV0151B_P2_9_sister_rejectOverAMovedActionIsAlreadyClosed(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_p29r")
	setActionStateFromElsewhere(t, store, "act_p29r", action.StateSucceeded)

	_, err := NewApprovalsAdapter(cfg, store).Reject(context.Background(), a.ApprovalID, "no")
	if !errors.Is(err, controlapi.ErrApprovalAlreadyClosed) {
		t.Fatalf("err = %v, want ErrApprovalAlreadyClosed", err)
	}
	after, _, gerr := store.GetApproval(context.Background(), a.ApprovalID)
	if gerr != nil {
		t.Fatalf("re-read: %v", gerr)
	}
	if after.Status != action.ApprovalPending {
		t.Fatalf("approval status = %q, want PENDING — the rejection rolled back whole", after.Status)
	}
}

// ---------------------------------------------------------------------------
// P2-6 · the same corrupt bytes carry the same type on every door.
// ---------------------------------------------------------------------------

// TestV0151B_P2_6_aCorruptColumnIsEvidenceCorruptOnTheDecide is Codex's
// reproduction verbatim: requested_at='ayer' on a parked approval, then
// DecideApprovalUnderLaw. GetApproval (the control row) already names it
// ErrApprovalEvidenceCorrupt; the decide must name it the same, for both
// verbs.
//
// Probing mutation (planned): return scanApproval's error bare from approvalTx
// ⇒ the decide rows redden while the control stays green.
// Evidence: real store file, attacker writes through a second real
// connection, in-process.
func TestV0151B_P2_6_aCorruptColumnIsEvidenceCorruptOnTheDecide(t *testing.T) {
	for _, verb := range []string{action.DecisionApproved, action.DecisionRejected} {
		t.Run(verb, func(t *testing.T) {
			cfg, store, done := parkingProfile(t)
			defer done()
			a := parkOne(t, cfg, store, "act_p26_"+verb)
			corruptRequestedAt(t, store, a.ApprovalID)

			if _, _, err := store.GetApproval(context.Background(), a.ApprovalID); !errors.Is(err, actionsqlite.ErrApprovalEvidenceCorrupt) {
				t.Fatalf("control: GetApproval err = %v, want ErrApprovalEvidenceCorrupt", err)
			}
			_, pin, err := ResolveApprovalLaw(cfg, "a")
			if err != nil {
				t.Fatalf("law: %v", err)
			}
			env, ident, err := approvalsOperator(a.ApprovalID)
			if err != nil {
				t.Fatalf("operator: %v", err)
			}
			_, err = store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID,
				verb, time.Now().UTC(), env, ident, "", pin)
			if !errors.Is(err, actionsqlite.ErrApprovalEvidenceCorrupt) {
				t.Fatalf("decide err = %v, want ErrApprovalEvidenceCorrupt — the same bytes, the same name", err)
			}
		})
	}
}

// TestV0151B_P2_6_sister_theListNamesACorruptRow: ListApprovals is the third
// reader of the same row shape and returns scanApproval's refusal bare, so a
// corrupt column reaches the CLI's `approvals list` untyped.
//
// Probing mutation (planned): leave ListApprovals' scan error bare ⇒ reddens.
// Evidence: real store file, attacker writes through a second real
// connection, in-process.
func TestV0151B_P2_6_sister_theListNamesACorruptRow(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_p26_list")
	corruptRequestedAt(t, store, a.ApprovalID)
	_, err := store.ListApprovals(context.Background(), action.ApprovalPending)
	if !errors.Is(err, actionsqlite.ErrApprovalEvidenceCorrupt) {
		t.Fatalf("list err = %v, want ErrApprovalEvidenceCorrupt", err)
	}
}

// ---------------------------------------------------------------------------
// P2-9 regression guard (reject verb) and P2-6 at the adapter (cancelled
// context).
// ---------------------------------------------------------------------------

// TestV0151B_P2_9_rejectUnderADriverFailureStaysUnavailable is the REJECT verb
// with the same trigger. It is GREEN today — the reject branch answers
// `unavailable` — and it exists as a regression guard against a cure that
// types the reject branch's exec failure as a moved action (the adversary's
// finding 2). Its probing mutation is EXECUTED in the red phase, not planned:
// see the pre-test paper §5. ADDED in the green step (diff pass #1, P3-e): once
// the trigger is gone the same request is still rejectable, through the same
// adapter, and the parked action closes REJECTED — the reject half of «stays
// decidable once the failure is gone» (ErrApprovalWriteFailed's godoc).
//
// Evidence: real store file, trigger through a second real connection,
// in-process.
// Probing mutation EXECUTED: roll the failed reject's approval to a closed
// status ⇒ the re-decide row red.
func TestV0151B_P2_9_rejectUnderADriverFailureStaysUnavailable(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_p29rej")
	drop := forceTransitionFailure(t, store, "act_p29rej")

	ad := NewApprovalsAdapter(cfg, store)
	_, err := ad.Reject(context.Background(), a.ApprovalID, "no")
	if errors.Is(err, controlapi.ErrApprovalAlreadyClosed) {
		t.Fatalf("err = %v: a driver failure published as already_closed", err)
	}
	if !errors.Is(err, controlapi.ErrApprovalsUnavailable) {
		t.Fatalf("err = %v, want ErrApprovalsUnavailable", err)
	}
	if s := actionStateOf(t, store, "act_p29rej"); s != string(action.StatePendingApproval) {
		t.Fatalf("action state = %q, want PENDING_APPROVAL", s)
	}

	drop()
	if _, err := ad.Reject(context.Background(), a.ApprovalID, "no"); err != nil {
		t.Fatalf("after the trigger is gone the request must still be rejectable: %v", err)
	}
	after, _, gerr := store.GetApproval(context.Background(), a.ApprovalID)
	if gerr != nil || after.Status != action.ApprovalRejected {
		t.Fatalf("approval after the re-decide = %q (%v), want REJECTED", after.Status, gerr)
	}
	if s := actionStateOf(t, store, "act_p29rej"); s != string(action.StateRejected) {
		t.Fatalf("action state after the re-decide = %q, want REJECTED", s)
	}
}

// TestV0151B_P2_6_aCancelledRejectIsUnavailableNotCorrupt: Reject's first read
// is GetApproval, which today wraps a cancelled context's deferred error as
// evidence corruption — so a cancelled request tells the operator the evidence
// is permanently damaged.
//
// Evidence: real store file, in-process; the context is cancelled before the
// call.
// Probing mutation (planned): wrap GetApproval's scan error as corrupt
// unconditionally ⇒ red.
func TestV0151B_P2_6_aCancelledRejectIsUnavailableNotCorrupt(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_p26_cancel")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewApprovalsAdapter(cfg, store).Reject(ctx, a.ApprovalID, "no")
	if errors.Is(err, controlapi.ErrApprovalEvidenceCorrupt) {
		t.Fatalf("err = %v: a cancelled request published as evidence_corrupt", err)
	}
	if !errors.Is(err, controlapi.ErrApprovalsUnavailable) {
		t.Fatalf("err = %v, want ErrApprovalsUnavailable", err)
	}
}

// ---------------------------------------------------------------------------
// P2-7 · the approvals routes answer loopback peers only (director,
// 2026-09-19: always refuse a non-loopback peer, no opt-in).
// ---------------------------------------------------------------------------

// nonLoopbackIPv4 returns an address of this host that is NOT loopback, or ""
// when the host has none (then the remote moulds skip, and say why).
func nonLoopbackIPv4(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("interfaces: %v", err)
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, ad := range addrs {
			ipn, ok := ad.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			return ipn.IP.String()
		}
	}
	return ""
}

// bootAdmin builds a profile whose admin server binds bindAddr, with the bearer
// set, approvals on and one request parked before boot. It returns the bound
// port, the parked approval and the db path.
func bootAdmin(t *testing.T, bindAddr string) (port string, a action.Approval, dbPath string) {
	t.Helper()
	dbPath = filepath.Join(t.TempDir(), "korvun.db")
	cfg := kernelWiringConfig(dbPath)
	cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
	cfg.Brains[0].Agent = &config.AgentConfig{
		Tools: []string{"webhook_call"}, MaxIterations: 2, EffectCeiling: "critical",
		WebhookCall: &config.WebhookCallToolConfig{AllowHosts: []string{"hooks.acme.io"}},
	}
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_TEST_ADMIN_P27"}
	cfg.Observability = &config.ObservabilityConfig{Addr: bindAddr}
	t.Setenv("KORVUN_TEST_ADMIN_P27", "s3cr3t")

	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a = parkOne(t, cfg, store, "act_p27")
	_ = store.Close()

	ap, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))), WithReloader(stubReloader{}))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = ap.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for ap.adminServer.Addr() == "" && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if ap.adminServer.Addr() == "" {
		t.Fatal("admin server never bound")
	}
	t.Cleanup(func() {
		cancel()
		sctx, sc := context.WithTimeout(context.Background(), time.Second)
		defer sc()
		_ = ap.Shutdown(sctx)
	})
	_, port, err = net.SplitHostPort(ap.adminServer.Addr())
	if err != nil {
		t.Fatalf("addr: %v", err)
	}
	return port, a, dbPath
}

func bearerDo(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer s3cr3t")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func countDecisionActs(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM actions WHERE op_namespace = 'approval'`).Scan(&n); err != nil {
		t.Fatalf("count acts: %v", err)
	}
	return n
}

// TestV0151B_P2_7_noRouteServesOrDecidesForARemotePeer is Codex's reproduction
// (`observability.addr=0.0.0.0`, remote peer, correct bearer) under the
// director's ruling: every approvals route answers 403 `loopback_only`.
//
// NO aborting trigger here: the first oracle of this mould aborted every
// decision-act INSERT, and that hid a handler that decides and THEN refuses —
// the seam's error swallowed, the 403 still written (the adversary's
// finding). Here nothing stops a reached decide from committing, and the
// approve carries the request's VALID digest, so a route that reached the seam
// leaves a trace this mould reads: the approval no longer PENDING, a decision
// act recorded. Whether a route CALLED the seam at all is proved one level
// down, by TestV0151B_P2_7_theSeamIsNeverCalledForARemotePeer (controlapi).
// A partial cure that let the approve through would run webhook_call over
// parameters with no URL, which fails before any request leaves.
//
// Evidence: a real TCP socket from a non-loopback interface of the SAME host
// (a peer address the server did not choose, not a second machine); real
// store file; in-process core.
// Probing mutation (planned): refuse after calling the seam ⇒ the PENDING and
// zero-act assertions red.
func TestV0151B_P2_7_noRouteServesOrDecidesForARemotePeer(t *testing.T) {
	ip := nonLoopbackIPv4(t)
	if ip == "" {
		t.Skip("this host has no non-loopback IPv4 interface; the P2-7 remote mould cannot place a peer")
	}
	port, a, dbPath := bootAdmin(t, "0.0.0.0:0")
	before := countDecisionActs(t, dbPath)
	base := "http://" + net.JoinHostPort(ip, port)
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/approvals", ""},
		{http.MethodGet, "/api/approvals/" + a.ApprovalID, ""},
		{http.MethodPost, "/api/approvals/" + a.ApprovalID + "/approve", `{"digest":"` + a.ActionDigest + `"}`},
		{http.MethodPost, "/api/approvals/" + a.ApprovalID + "/reject", `{"comment":"remote"}`},
	}
	for _, r := range routes {
		status, body := bearerDo(t, r.method, base+r.path, r.body)
		if status != http.StatusForbidden || !strings.Contains(body, `"error":"loopback_only"`) {
			t.Errorf("%s %s from %s: %d %s, want 403 loopback_only", r.method, r.path, ip, status, body)
		}
		if strings.Contains(body, a.ApprovalID) || strings.Contains(body, a.ActionDigest) {
			t.Errorf("%s %s from %s: the refusal carries stored data", r.method, r.path, ip)
		}
	}
	if after := countDecisionActs(t, dbPath); after != before {
		t.Errorf("decision acts %d -> %d: a remote peer's decision was recorded", before, after)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	after, _, err := store.GetApproval(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Status != action.ApprovalPending {
		t.Errorf("approval status = %q, want PENDING", after.Status)
	}
}

// TestV0151B_P2_7_loopbackPeersStillReachTheSurface are the positive controls
// the refusal needs: on a dual-stack bind, 127.0.0.1 and ::1 are served. A cure
// that refused everybody would pass the remote mould and redden these.
//
// Evidence: real TCP sockets over both loopback families, in-process core.
func TestV0151B_P2_7_loopbackPeersStillReachTheSurface(t *testing.T) {
	port, _, _ := bootAdmin(t, "[::]:0")
	for _, host := range []string{"127.0.0.1", "::1"} {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), time.Second)
		if err != nil {
			t.Skipf("loopback %s unreachable on this host (%v); the control cannot run", host, err)
		}
		_ = c.Close()
		status, body := bearerDo(t, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/api/approvals", "")
		if status != http.StatusOK {
			t.Errorf("GET /api/approvals from %s: %d %s, want 200", host, status, body)
		}
	}
}

// TestV0151B_P2_6_sister_aCorruptApprovalRowNeverBlocksTheBoot: Build runs
// RecoverPreviousLife after wiring the sealer and treats its refusal as
// boot-fatal. A cure that REFUSED to seal over a corrupt approval row (instead
// of the director's explicit mark) would leave the core unable to start.
//
// GREEN today, by design: today the recovery seals "" and boots. It guards the
// boot against a refusing cure; its probing mutation is EXECUTED in the red
// phase (pre-test paper §5).
//
// Evidence: real store file, corruption through a second real connection, the
// real Build.
func TestV0151B_P2_6_sister_aCorruptApprovalRowNeverBlocksTheBoot(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	a := parkOne(t, cfg, store, "act_boot_corrupt")
	_, pin, err := ResolveApprovalLaw(cfg, "a")
	if err != nil {
		t.Fatalf("law: %v", err)
	}
	env, ident, err := approvalsOperator(a.ApprovalID)
	if err != nil {
		t.Fatalf("operator: %v", err)
	}
	if rule, err := store.DecideApprovalUnderLaw(context.Background(), a.ApprovalID,
		action.DecisionApproved, time.Now().UTC(), env, ident, "", pin); err != nil || rule != "" {
		t.Fatalf("approve: %q %v", rule, err)
	}
	if _, _, err := store.ClaimApprovalParamsUnderDigest(context.Background(), a.ApprovalID, &pin, a.ActionDigest, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	attackerExec(t, store, `UPDATE approvals SET policy_version = 'x' WHERE approval_id = ?`, a.ApprovalID)
	done()

	ap, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build over a store with one corrupt approval row: %v — the core must boot", err)
	}
	shutdownApp(t, ap)
}

// TestV0151B_P2_8_theClaimDoorNeverPublishesAnotherBrainsLaw: the third door
// (nameClaim → nameTouch → currentLawDigest), placed DETERMINISTICALLY (the
// adversary's construction, replacing a race mould that skipped two runs in
// three under -race).
//
// A request is parked for the MIDDLE brain b. A trigger on the moment the
// decide commits the approval (status → APPROVED) rewrites, coherently, the
// law the row carries: policy_digest, the sealed preview (canonical_preview and
// preview_digest over p2 = p with PolicyDigest "sha256:" + 64×"e") and the
// history row's policy_digest. Every belt that reads the row afterwards sees a
// self-consistent story, so nothing names corruption; only the claim's law
// check, judging b's CURRENT pin against the row, refuses — ErrApprovalInvalidated
// through nameClaim. Its current_law_digest must be b's.
//
// DEPENDENCY, declared: this relies on the claim reaching its law check.
// Block A reorders the claim (the `seen` comparison before the law); whether
// the placement survives that order is an open question routed to the
// coordinator (pre-test paper §12, Q-B8).
//
// Evidence: real store file, trigger installed through a second real
// connection, in-process.
// Probing mutation (planned): resolve currentLawDigest from cfg.Brains[0] ⇒ red.
func TestV0151B_P2_8_theClaimDoorNeverPublishesAnotherBrainsLaw(t *testing.T) {
	cfg, store, done := threeBrainProfile(t)
	defer done()
	a := parkForBrain(t, cfg, store, "b", "act_p28_claim")
	_, p, err := store.GetApproval(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("read preview: %v", err)
	}
	_, bPin, err := ResolveApprovalLaw(cfg, "b")
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}
	p2 := p
	p2.PolicyDigest = "sha256:" + strings.Repeat("e", 64)
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
	// SQLite triggers take no bound variables; every value is a test-owned
	// literal quoted here.
	attackerExec(t, store, `CREATE TRIGGER move_the_row_law AFTER UPDATE OF status ON approvals
	  WHEN NEW.approval_id = `+quote(a.ApprovalID)+` AND NEW.status = 'APPROVED'
	  BEGIN
	    UPDATE approvals SET policy_digest = `+quote(p2.PolicyDigest)+`,
	           preview_digest = `+quote(p2.Digest())+`,
	           canonical_preview = `+quote(string(action.CanonicalPreview(p2)))+`
	     WHERE approval_id = `+quote(a.ApprovalID)+`;
	    UPDATE action_decisions SET policy_digest = `+quote(p2.PolicyDigest)+`
	     WHERE action_id = `+quote(a.ActionID)+`;
	  END`) // #nosec G202

	_, err = NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if !errors.Is(err, controlapi.ErrApprovalInvalidated) {
		t.Fatalf("err = %v, want ErrApprovalInvalidated from the claim", err)
	}
	var moved interface{ CurrentLawDigest() string }
	if !errors.As(err, &moved) {
		t.Fatalf("the refusal must carry the current law in its own field")
	}
	if got := moved.CurrentLawDigest(); got != bPin.Digest {
		t.Fatalf("current_law_digest = %q, want b's %q", got, bPin.Digest)
	}
}
