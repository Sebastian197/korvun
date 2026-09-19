// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.1 block A on the approved path: A1 (a purge that does not hold inside
// the claim's transaction is refused; a post-commit restore by another
// connection is not covered, filed for v0.15.2),
// A2 (only 2xx is success) and P2-4 with its cancellation sister (an end
// before a connection was obtained is a decided FAILED).
//
// Evidence level, honest: in-process, a real boot and a real SQLite file; the
// attacking trigger is installed from a second real connection, and the
// competing execution is a second call through the production door made while
// the first one's tool runs. Not a compiled binary, not a second OS process.
package app

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/action/executor"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/tool"
)

// competingTool counts its runs and, on its FIRST run only, fires `compete`:
// a second caller entering the production door while the first one's effect
// is in flight — the window in which the parked action is still APPROVED.
type competingTool struct {
	runs    atomic.Int64
	compete func() error
	second  error
	fired   atomic.Bool
}

func (c *competingTool) Name() string        { return "webhook_call" }
func (c *competingTool) Description() string { return "competing fake" }
func (c *competingTool) Execute(ctx context.Context, args string) (string, error) {
	c.runs.Add(1)
	// A flag, not a sync.Once: the competitor re-enters THIS method when its
	// own claim succeeds, and a Once would deadlock on itself right there.
	if c.fired.CompareAndSwap(false, true) {
		c.second = c.compete()
	}
	return "EXTERNAL-EFFECT-DONE", nil
}

// TestExecuteApprovedAction_aPurgeThatDoesNotHoldIsRefused is A1's mould on the
// approved path: Codex's trigger restores OLD.canonical_params after the
// claim's purge, so the purge reports one row and the parameters are still
// there when it commits. A competitor entering while the first effect is in
// flight passes the prechecks (the action is still APPROVED) and claims the
// same bytes again.
//
// The attacked row demands both claims refused by NAME, the claim rolled back
// whole (the probe table stays empty), and exec.Run never started: a claim
// that cannot prove it consumed the parameters consumes nothing. Zero
// executions under the persistent restoring trigger is the director's
// accepted outcome (2026-09-19).
//
// The second row is a NO-ATTACK control: no trigger, the first execution
// runs once and the competitor is refused by name. It must stay green before
// and after the cure; it proves the cure does not refuse the normal claim.
//
// Taxonomy: ErrApprovalEvidenceCorrupt, the existing sentinel (director,
// 2026-09-19); the adapter is untouched.
//
// Planned probing mutations (after green): delete the post-purge re-read ⇒
// runs=2; move it after the Commit ⇒ probe rows appear.
func TestExecuteApprovedAction_aPurgeThatDoesNotHoldIsRefused(t *testing.T) {
	t.Run("attacked: the trigger restores the parameters", func(t *testing.T) {
		store, _, _, approvalID, dbPath := approvedFlowWithPath(t)
		ctx := context.Background()
		digest := digestOf(t, store, approvalID)

		hand, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatalf("open the second connection: %v", err)
		}
		defer func() { _ = hand.Close() }()
		if _, err := hand.Exec(`CREATE TABLE purge_probe (approval_id TEXT NOT NULL)`); err != nil {
			t.Fatalf("create the probe table: %v", err)
		}
		if _, err := hand.Exec(`CREATE TRIGGER restore_params AFTER UPDATE OF canonical_params ON approvals
			WHEN NEW.canonical_params = '' AND OLD.canonical_params != ''
			BEGIN
			  INSERT INTO purge_probe (approval_id) VALUES (NEW.approval_id);
			  UPDATE approvals SET canonical_params = OLD.canonical_params
			   WHERE approval_id = NEW.approval_id;
			END`); err != nil {
			t.Fatalf("install the restoring trigger: %v", err)
		}

		ct := &competingTool{}
		exec := executor.New(tool.Registry{"webhook_call": ct}, 0, time.Now)
		ct.compete = func() error {
			_, err := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
			return err
		}

		_, first := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
		if !errors.Is(first, actionsqlite.ErrApprovalEvidenceCorrupt) {
			t.Fatalf("first claim: err = %v, want ErrApprovalEvidenceCorrupt (runs=%d, competitor err=%v)",
				first, ct.runs.Load(), ct.second)
		}
		_, second := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
		if !errors.Is(second, actionsqlite.ErrApprovalEvidenceCorrupt) {
			t.Fatalf("second claim: err = %v, want ErrApprovalEvidenceCorrupt", second)
		}
		if n := ct.runs.Load(); n != 0 {
			t.Fatalf("exec.Run started %d time(s) over parameters whose consumption never held", n)
		}
		// The oracle by impossibility: a probe row exists only if a claim's
		// transaction committed with its purge in it.
		var probes int
		if err := hand.QueryRow(`SELECT COUNT(*) FROM purge_probe`).Scan(&probes); err != nil {
			t.Fatalf("read the probe: %v", err)
		}
		if probes != 0 {
			t.Fatalf("purge_probe holds %d row(s): a refused claim's transaction COMMITTED its purge", probes)
		}
		if _, state, rerr := store.ReReadParams(ctx, approvalID); rerr != nil || state != actionsqlite.ParamsPresent {
			t.Fatalf("params after the refusals: state=%q err=%v, want present", state, rerr)
		}
		rec, err := store.Get(ctx, actionOf(t, store, approvalID))
		if err != nil {
			t.Fatalf("read the action back: %v", err)
		}
		if rec.State != action.StateApproved {
			t.Fatalf("action state = %q, want APPROVED — nothing ran, nothing closed", rec.State)
		}
	})

	t.Run("no-attack control: no trigger, one run, the competitor refused by name", func(t *testing.T) {
		store, _, _, approvalID, _ := approvedFlowWithPath(t)
		ctx := context.Background()
		digest := digestOf(t, store, approvalID)

		ct := &competingTool{}
		exec := executor.New(tool.Registry{"webhook_call": ct}, 0, time.Now)
		ct.compete = func() error {
			_, err := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
			return err
		}
		if _, err := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest); err != nil {
			t.Fatalf("the first execution: %v", err)
		}
		if !errors.Is(ct.second, actionsqlite.ErrApprovalParamsEmpty) {
			t.Fatalf("competitor: err = %v, want ErrApprovalParamsEmpty", ct.second)
		}
		if n := ct.runs.Load(); n != 1 {
			t.Fatalf("exec.Run ran %d time(s), want exactly 1", n)
		}
	})
}

// TestExecuteApprovedAction_onlyA2xxIsSuccess is A2's mould on the approved
// path, with the REAL webhook_call: 301 without Location is the reproduction,
// 200 the control.
//
// Planned probing mutation (after green): restore `resp.StatusCode >= 400` as
// the tool's only non-success test ⇒ the 301 row reddens.
func TestExecuteApprovedAction_onlyA2xxIsSuccess(t *testing.T) {
	cases := []struct {
		name        string
		code        int
		want        action.State
		wantUnknown bool
	}{
		{"301 without Location", http.StatusMovedPermanently, action.StateOutcomeUnknown, true},
		{"200", http.StatusOK, action.StateSucceeded, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			read := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				select {
				case read <- string(b):
				default:
				}
				w.WriteHeader(tc.code)
			}))
			srv.Config.ErrorLog = log.New(io.Discard, "", 0)
			defer srv.Close()
			host := strings.TrimPrefix(srv.URL, "http://")

			store, _, _, approvalID, _ := approvedFlowWithParams(t, srv.URL+` {"event":"ping"}`)
			wc, err := tool.WebhookCall(tool.WebhookCallConfig{AllowHosts: []string{host}})
			if err != nil {
				t.Fatalf("wire the tool: %v", err)
			}
			exec := executor.New(tool.Registry{"webhook_call": wc}, 0, time.Now)

			run, err := ExecuteApprovedAction(context.Background(), store, exec, approvalID, testLaw, digestOf(t, store, approvalID))
			if err != nil {
				t.Fatalf("a delivered request is a closed outcome, not an error of this call: %v", err)
			}
			select {
			case got := <-read:
				if !strings.Contains(got, "ping") {
					t.Fatalf("the receiver did not read the payload: %q", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the receiver never read the POST — the row does not attack what it names")
			}
			if run.Unknown != tc.wantUnknown {
				t.Fatalf("HTTP %d: Unknown = %v, want %v (detail %q)", tc.code, run.Unknown, tc.wantUnknown, run.FailureDetail)
			}
			rec, err := store.Get(context.Background(), actionOf(t, store, approvalID))
			if err != nil {
				t.Fatalf("read the action back: %v", err)
			}
			if rec.State != tc.want {
				t.Fatalf("HTTP %d: state = %q, want %q", tc.code, rec.State, tc.want)
			}
		})
	}
}

// stallingTLSApp accepts TCP and never answers the TLS ClientHello.
func stallingTLSApp(t *testing.T) (string, <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan struct{}, 8)
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})
	return ln.Addr().String(), ch
}

// TestExecuteApprovedAction_anEndBeforeAConnectionClosesFailed is P2-4's mould on
// the approved path — Codex's reproduction verbatim in the deadline row: a
// loopback listener that accepts and never completes the TLS handshake,
// WebhookCallConfig with that https://host:port, the allow-list and a short
// timeout, execute an approval. The cancellation row is the class sister.
//
// Planned probing mutation (after green): classify every ended context as
// unknown again in CloseStateAfterRun ⇒ both rows redden.
func TestExecuteApprovedAction_anEndBeforeAConnectionClosesFailed(t *testing.T) {
	cases := []struct {
		name   string
		cancel bool
	}{
		{"the tool's deadline fires during the handshake", false},
		{"the caller cancels during the handshake", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, accepted := stallingTLSApp(t)
			timeout := 300 * time.Millisecond
			if tc.cancel {
				timeout = 30 * time.Second
			}
			store, _, _, approvalID, _ := approvedFlowWithParams(t, "https://"+addr+` {"event":"ping"}`)
			wc, err := tool.WebhookCall(tool.WebhookCallConfig{AllowHosts: []string{addr}, Timeout: timeout})
			if err != nil {
				t.Fatalf("wire the tool: %v", err)
			}
			exec := executor.New(tool.Registry{"webhook_call": wc}, 0, time.Now)
			digest := digestOf(t, store, approvalID)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				go func() {
					select {
					case <-accepted:
						cancel()
					case <-time.After(10 * time.Second):
					}
				}()
			}
			run, err := ExecuteApprovedAction(ctx, store, exec, approvalID, testLaw, digest)
			if err != nil {
				t.Fatalf("a call that never wrote is a decided outcome, not an error of this call: %v", err)
			}
			if run.Unknown {
				t.Fatalf("Unknown = true over a request that never obtained a connection: %s", run.FailureDetail)
			}
			if !run.Failed {
				t.Fatal("Failed = false over a request that never obtained a connection")
			}
			// The positive class assertion (Delta 7), by the only channel this
			// path hands out: FailureDetail carries the error's text.
			if !strings.Contains(run.FailureDetail, tool.ErrNotSent.Error()) {
				t.Fatalf("FailureDetail %q does not carry ErrNotSent's text %q", run.FailureDetail, tool.ErrNotSent.Error())
			}
			rec, err := store.Get(context.Background(), actionOf(t, store, approvalID))
			if err != nil {
				t.Fatalf("read the action back: %v", err)
			}
			if rec.State != action.StateFailed {
				t.Fatalf("state = %q, want FAILED", rec.State)
			}
		})
	}
}
