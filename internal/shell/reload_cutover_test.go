// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/config"
)

// B7 (bug-bash 2026-08-23): a reload the log proves APPLIED (cutover 09:40:44,
// admin 56707→56799) was painted by the builder as "reload failed — the running
// config is unchanged". The builder can only paint that banner from a genuine
// {"state":"failed"|"rolled-back"} body, so the Go-side contract its poll
// depends on is pinned here: polling /api/reload/<handle> THROUGH the shell
// proxy, tightly, across a real ephemeral-port cutover must
//
//   - never observe a terminal failure state on a happy cutover — the only
//     acceptable non-200s are the proxy's transient 503s while the admin
//     server is between ports;
//   - never observe a 404 for a live handle — the handle lives in the
//     supervisor and survives the cutover (ADR-0027 §F4), port change or not;
//   - reach "succeeded", and keep answering "succeeded" on the NEW cycle's
//     admin server after the cutover.
//
// REWRITTEN 2026-09-24 (director's decision on the profile with no ledger): the
// profile carries a ledger, because a profile without one now refuses the
// builder's door with no_ledger; and every one of the five rounds leaves its
// operator act SUCCEEDED in the ledger — five real cutovers, five closed acts.
//
// PROBING MUTATION (to be executed): call RecoverPreviousLife with no keep in
// Build. Each round's act reads OUTCOME_UNKNOWN and this reddens on the tally.
func TestProxy_reloadCutover_pollNeverSeesPhantomFailure(t *testing.T) {
	ollama := fakeOllama(t)
	c, _, ledger := startedControllerWithLedger(t, ollama.URL)
	srv := proxyServer(t, c)
	client := srv.Client()
	var acts []string

	for round := 1; round <= 5; round++ {
		addrBefore := c.Status().AdminAddr

		// The builder's editing baseline: GET the running config via the proxy.
		resp, body := get(t, client, srv.URL+"/api/config", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("round %d: GET /api/config = %d (body %q)", round, resp.StatusCode, body)
		}
		var cfg config.Config
		if err := json.Unmarshal([]byte(body), &cfg); err != nil {
			t.Fatalf("round %d: decode config: %v", round, err)
		}
		cfg.Brains[0].Models[0].ModelID = fmt.Sprintf("llama3.2-round%d", round)

		handle, actionID := postConfigForHandle(t, client, srv.URL, &cfg, round)
		acts = append(acts, actionID)

		// Tight poll through the proxy, across the cutover. 2ms keeps polls
		// landing inside the admin-rebind window without busy-spinning.
		deadline := time.Now().Add(15 * time.Second)
		history := make([]string, 0, 64)
		terminal := ""
		for time.Now().Before(deadline) && terminal == "" {
			r, b := get(t, client, srv.URL+"/api/reload/"+handle, nil)
			switch r.StatusCode {
			case http.StatusOK:
				var st struct {
					State string `json:"state"`
				}
				if err := json.Unmarshal([]byte(b), &st); err != nil {
					t.Fatalf("round %d: 200 with undecodable body %q: %v", round, b, err)
				}
				history = append(history, st.State)
				switch st.State {
				case "failed", "rolled-back":
					t.Fatalf("round %d: phantom terminal %q on a happy cutover (history %v)",
						round, st.State, history)
				case "persist-failed":
					t.Fatalf("round %d: persist-failed writing the temp config (history %v)",
						round, history)
				case "succeeded":
					terminal = st.State
				}
			case http.StatusNotFound:
				// The handle must survive the cutover (F4): the supervisor —
				// and its status map — is shared by every admin cycle.
				t.Fatalf("round %d: 404 for live handle %q mid-cutover (history %v)",
					round, handle, history)
			default:
				// Transient proxy 503 while the admin server is between
				// ports — the exact window the builder retries through.
				history = append(history, fmt.Sprintf("http-%d", r.StatusCode))
			}
			time.Sleep(2 * time.Millisecond)
		}
		if terminal != "succeeded" {
			t.Fatalf("round %d: no succeeded within budget (history %v)", round, history)
		}

		// The terminal state stays readable on the NEW cycle's admin server.
		r2, b2 := get(t, client, srv.URL+"/api/reload/"+handle, nil)
		if r2.StatusCode != http.StatusOK {
			t.Fatalf("round %d: post-cutover re-read = %d (body %q)", round, r2.StatusCode, b2)
		}
		var st2 struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal([]byte(b2), &st2); err != nil || st2.State != "succeeded" {
			t.Fatalf("round %d: post-cutover re-read state = %q (err %v), want succeeded", round, st2.State, err)
		}

		if addrAfter := c.Status().AdminAddr; addrAfter == addrBefore {
			// The kernel may hand the freed ephemeral port back; the invariant
			// under test is handle survival, not port inequality.
			t.Logf("round %d: kernel reused admin addr %s", round, addrAfter)
		}
	}

	// Five cutovers, five acts, each closed with the outcome the supervisor
	// reached — read back through a read-only open, never the running app.
	for i, id := range acts {
		row := waitLedgerTerminal(t, ledger, id, 10*time.Second)
		if row.State != action.StateSucceeded {
			t.Fatalf("round %d: act %s ended %q, want SUCCEEDED", i+1, id, row.State)
		}
	}
}

// postConfigForHandle POSTs the config through the proxy and returns the 202
// reload handle and the operator act's id, retrying briefly on 409 (the
// single-flight window right after the previous round's terminal state,
// before finishReload lands).
func postConfigForHandle(t *testing.T, client *http.Client, base string, cfg *config.Config, round int) (string, string) {
	t.Helper()
	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("round %d: marshal config: %v", round, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := client.Post(base+"/api/config", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("round %d: POST /api/config: %v", round, err)
		}
		var out struct {
			Handle   string `json:"handle"`
			ActionID string `json:"action_id"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusAccepted:
			if decodeErr != nil || out.Handle == "" {
				t.Fatalf("round %d: 202 without a handle (err %v)", round, decodeErr)
			}
			if out.ActionID == "" {
				t.Fatalf("round %d: 202 without the operator act's id", round)
			}
			return out.Handle, out.ActionID
		case resp.StatusCode == http.StatusConflict && time.Now().Before(deadline):
			time.Sleep(20 * time.Millisecond)
		default:
			t.Fatalf("round %d: POST /api/config = %d, want 202", round, resp.StatusCode)
		}
	}
}
