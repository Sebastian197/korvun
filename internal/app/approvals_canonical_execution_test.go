// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/tool"

	_ "modernc.org/sqlite"
)

type approvalStoreBeforeClaim struct {
	executor.ApprovalStore
	before func()
}

func (s approvalStoreBeforeClaim) Claim(
	ctx context.Context,
	approvalID string,
	seen *action.Approval,
) ([]byte, action.Operation, error) {
	s.before()
	return s.ApprovalStore.Claim(ctx, approvalID, seen)
}

func TestResumeApproved_UsesTheSnapshotReadBeforeTheClaim(t *testing.T) {
	store, exec, fake, approvalID, dbPath := approvedFlowWithPath(t)
	digest := digestOf(t, store, approvalID)
	adapter := approvedExecutionStore{store: store, law: testLaw, approvedDigest: digest}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	moved := false
	wrapped := approvalStoreBeforeClaim{
		ApprovalStore: adapter,
		before: func() {
			if moved {
				return
			}
			moved = true
			if _, moveErr := db.Exec(
				`UPDATE approvals SET status = 'REJECTED' WHERE approval_id = ?`,
				approvalID,
			); moveErr != nil {
				t.Fatalf("move approval before claim: %v", moveErr)
			}
		},
	}

	_, err = exec.ResumeApproved(context.Background(), wrapped, approvalID)
	if !errors.Is(err, actionsqlite.ErrApprovalMovedUnderTheClaim) {
		t.Fatalf("err = %v, want ErrApprovalMovedUnderTheClaim", err)
	}
	if fake.runs.Load() != 0 {
		t.Fatalf("tool ran %d times over a moved snapshot", fake.runs.Load())
	}
	if _, state, readErr := store.ReReadParams(context.Background(), approvalID); readErr != nil || state != actionsqlite.ParamsPresent {
		t.Fatalf("params after refused claim: state=%q err=%v", state, readErr)
	}
}

func TestExecuteApprovedAction_EmptyIDKeepsTheStoreSentinel(t *testing.T) {
	store, exec, _, _ := approvedFlow(t)
	_, err := ExecuteApprovedAction(context.Background(), store, exec, "", testLaw, "")
	if !errors.Is(err, actionsqlite.ErrApprovalNotFound) {
		t.Fatalf("err = %v, want ErrApprovalNotFound", err)
	}
}

func TestExecuteApprovedAction_KeepsTheApprovedCloseClock(t *testing.T) {
	store, _, _, approvalID := approvedFlow(t)
	fixedLatencyClock := func() time.Time {
		return time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	}
	fake := &countingTool{}
	exec := executor.New(tool.Registry{"webhook_call": fake}, 0, fixedLatencyClock)
	started := time.Now().UTC()
	if _, err := ExecuteApprovedAction(
		context.Background(), store, exec, approvalID, testLaw, digestOf(t, store, approvalID),
	); err != nil {
		t.Fatalf("execute approved action: %v", err)
	}
	record, err := store.Get(context.Background(), "act_exec1")
	if err != nil {
		t.Fatalf("read closed action: %v", err)
	}
	if record.FinishedAt == nil || record.FinishedAt.Before(started) || record.FinishedAt.Equal(fixedLatencyClock()) {
		t.Fatalf("finished_at = %v, want approved wall clock after %v", record.FinishedAt, started)
	}
}
