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
func TestProxy_reloadCutover_pollNeverSeesPhantomFailure(t *testing.T) {
	ollama := fakeOllama(t)
	c, _ := startedController(t, ollama.URL)
	srv := proxyServer(t, c)
	client := srv.Client()

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

		handle := postConfigForHandle(t, client, srv.URL, &cfg, round)

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
}

// postConfigForHandle POSTs the config through the proxy and returns the 202
// reload handle, retrying briefly on 409 (the single-flight window right
// after the previous round's terminal state, before finishReload lands).
func postConfigForHandle(t *testing.T, client *http.Client, base string, cfg *config.Config, round int) string {
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
			Handle string `json:"handle"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusAccepted:
			if decodeErr != nil || out.Handle == "" {
				t.Fatalf("round %d: 202 without a handle (err %v)", round, decodeErr)
			}
			return out.Handle
		case resp.StatusCode == http.StatusConflict && time.Now().Before(deadline):
			time.Sleep(20 * time.Millisecond)
		default:
			t.Fatalf("round %d: POST /api/config = %d, want 202", round, resp.StatusCode)
		}
	}
}
