// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The two last execution moulds of v0.15.0 on the approved path — P1-1 and the
// reverse direction of P1-3 of the external review.
//
// Evidence level, honest: in-process, a real boot and a real SQLite file; the
// triggers are installed from a second real connection. Not a compiled binary.
package app

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/tool"
)

// TestExecuteApprovedAction_anAuthorityMovedInsideTheClaimNeverRuns is the
// review's reproduction, verbatim in its first row: approve without executing,
// install a trigger AFTER UPDATE OF canonical_params that moves the approval to
// REJECTED when the parameters are emptied, call ExecuteApprovedAction. The
// prechecks see APPROVED; the state moves inside the claim's own purge.
//
// The second row moves the ACTION row instead. It exists because the diff's
// adversary showed that, without it, ignoring the action state in the claim's
// re-read reddened only a mould whose attack lands before the purge — where
// the conditioned WHERE refuses first — while the effect still ran.
//
// Probing mutations (executed, red, declared in the canto): delete the
// authority re-read the claim runs after its purge ⇒ both rows redden; ignore
// the approval status in it ⇒ the first row reddens; ignore the action state in
// it ⇒ the second row reddens.
func TestExecuteApprovedAction_anAuthorityMovedInsideTheClaimNeverRuns(t *testing.T) {
	cases := []struct {
		name    string
		trigger string
	}{
		{
			name: "the approval moves to REJECTED inside the purge",
			trigger: `CREATE TRIGGER reject_on_purge AFTER UPDATE OF canonical_params ON approvals
				WHEN NEW.canonical_params = ''
				BEGIN UPDATE approvals SET status = 'REJECTED' WHERE approval_id = NEW.approval_id; END`,
		},
		{
			name: "the action moves to REJECTED inside the purge",
			trigger: `CREATE TRIGGER reject_action_on_purge AFTER UPDATE OF canonical_params ON approvals
				WHEN NEW.canonical_params = ''
				BEGIN UPDATE actions SET state = 'REJECTED' WHERE action_id = NEW.action_id; END`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, exec, fake, approvalID, dbPath := approvedFlowWithPath(t)
			ctx := context.Background()
			digest := digestOf(t, store, approvalID)

			db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
			if err != nil {
				t.Fatalf("open the second connection: %v", err)
			}
			defer func() { _ = db.Close() }()
			if _, err := db.Exec(tc.trigger); err != nil {
				t.Fatalf("install the trigger: %v", err)
			}

			_, err = ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
			if !errors.Is(err, actionsqlite.ErrApprovalNoLongerApproved) {
				t.Fatalf("err = %v, want ErrApprovalNoLongerApproved", err)
			}
			if n := fake.runs.Load(); n != 0 {
				t.Fatalf("exec.Run started %d time(s) over a request that was not APPROVED inside the claim", n)
			}
			// The refusal rolled the claim back whole: the parameters are held
			// and the parked action is still where the decide left it.
			if _, state, rerr := store.ReReadParams(ctx, approvalID); rerr != nil || state != actionsqlite.ParamsPresent {
				t.Fatalf("params after the refusal: state=%q err=%v, want present", state, rerr)
			}
			rec, err := store.Get(ctx, actionOf(t, store, approvalID))
			if err != nil {
				t.Fatalf("read the action back: %v", err)
			}
			if rec.State != action.StateApproved {
				t.Fatalf("action state = %q, want APPROVED — nothing ran, nothing closed", rec.State)
			}
		})
	}
}

// TestExecuteApprovedAction_aDialThatNeverConnectsClosesFailed is the reverse
// direction on the approved path, with the REAL webhook_call against a port
// nothing listens on: a call that never left the machine is a decided FAILED,
// never «we do not know». Its brain twin is
// TestRunTool_aDialThatNeverConnectsStillClosesFailed.
//
// Probing mutation (executed, red, declared in the canto): classify every tool
// error as unknown on the approved path ⇒ this reddens.
func TestExecuteApprovedAction_aDialThatNeverConnectsClosesFailed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	store, _, _, approvalID, _ := approvedFlowWithParams(t, "http://"+addr+` {"event":"ping"}`)
	wc, err := tool.WebhookCall(tool.WebhookCallConfig{AllowHosts: []string{addr}})
	if err != nil {
		t.Fatalf("wire the tool: %v", err)
	}
	exec := executor.New(tool.Registry{"webhook_call": wc}, 0, time.Now)

	run, err := ExecuteApprovedAction(context.Background(), store, exec, approvalID, testLaw, digestOf(t, store, approvalID))
	if err != nil {
		t.Fatalf("a refused dial is a decided outcome, not an error of this call: %v", err)
	}
	if run.Unknown {
		t.Fatalf("Unknown = true over a call that never connected: %s", run.FailureDetail)
	}
	if !run.Failed {
		t.Fatal("Failed = false over a call that never connected")
	}
	rec, err := store.Get(context.Background(), actionOf(t, store, approvalID))
	if err != nil {
		t.Fatalf("read the action back: %v", err)
	}
	if rec.State != action.StateFailed {
		t.Fatalf("state = %q, want FAILED — nothing reached the wire", rec.State)
	}
}
