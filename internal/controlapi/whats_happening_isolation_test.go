// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the button must not touch the LIVE config.
//
// Found while writing the green, not by a test: `next := *cur` is a shallow
// copy, so the brains slice and every pointer under it are SHARED with the
// config the app is running on. Three of the four edits reach through those
// pointers, so a button would have changed the live profile before the
// supervisor decided anything — and a rolled-back cutover would leave the
// running app quietly modified while both the disk and the handle said nothing
// happened.
//
// The oracle here is by COMPARISON against the caller's own object, which is
// the only thing that can see the leak: the handler's answer looks identical
// either way.
//
// Evidence level, honest: in-process, over a real httptest server.

package controlapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// liveConfigReloader hands out the SAME object every time, the way a running
// supervisor hands out the config its app was built from.
type liveConfigReloader struct {
	live  *config.Config
	state supervisor.State
}

func (r *liveConfigReloader) RequestReload(*config.Config) (supervisor.Handle, error) {
	return supervisor.Handle("reload-live"), nil
}
func (r *liveConfigReloader) Status(supervisor.Handle) supervisor.State { return r.state }
func (r *liveConfigReloader) CurrentConfig() *config.Config             { return r.live }

// TestAction_neverMutatesTheLiveConfig walks all four buttons over one live
// config and demands it comes out identical.
//
// PROBING MUTATION: go back to `next := *cur`. Three of the four rows redden —
// ceiling, shadow and host — and the approvals row stays green, because that
// edit replaces a pointer instead of reaching through one. That asymmetry is
// itself the finding: a shallow copy is not uniformly wrong, which is exactly
// why it survives a casual reading.
func TestAction_neverMutatesTheLiveConfig(t *testing.T) {
	withAdminToken(t)
	cases := []struct {
		name string
		door string
		body string
	}{
		{"enable approvals", "enable-approvals", `{"confirm":true}`},
		{"set ceiling", "set-ceiling", `{"confirm":true,"brain":"default"}`},
		{"lift shadow", "lift-shadow", `{"confirm":true,"brain":"default","tool":"webhook_call"}`},
		{"allow host", "allow-host", `{"confirm":true,"brain":"default","tool":"webhook_call","host":"hooks.acme.io"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			live := &config.Config{
				Admin:     &config.AdminConfig{TokenEnv: adminEnvVar},
				Approvals: &config.ApprovalsConfig{Enabled: false, TTL: "30m"},
				Brains: []config.BrainConfig{whatsHappeningBrain(&config.AgentConfig{
					Tools:         []string{"webhook_call"},
					EffectCeiling: "",
					Governance:    []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "shadow"}},
					WebhookCall:   &config.WebhookCallToolConfig{AllowHosts: []string{"127.0.0.1:8765"}},
				})},
			}
			r := &liveConfigReloader{live: live, state: supervisor.StateSucceeded}
			mux := http.NewServeMux()
			registerWhats(mux, r)
			srv := serve(t, mux)

			// The WHOLE live config, before. `cloneForEdit` is a PARTIAL copy by
			// design, so it aliases Storage, Admin, Session, Models, Channels,
			// Routes, Observability and Authority with the object the app runs
			// on. A sweep of internal/app and internal/supervisor found no write
			// through any of those pointers today — but «today» is not a
			// guarantee, and the official pass was right to name it a declared
			// risk rather than a settled question. This oracle settles it by
			// COMPARISON over every field at once, so a future writer that
			// reaches through an aliased pointer reddens here instead of leaving
			// a rolled-back cutover with a quietly altered running app.
			before, err := json.Marshal(live)
			if err != nil {
				t.Fatalf("marshal the live config: %v", err)
			}

			resp := post(t, srv, "/api/whats-happening/"+tc.door, whatsToken, tc.body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			after, err := json.Marshal(live)
			if err != nil {
				t.Fatalf("marshal the live config: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("the LIVE config changed:\n before %s\n after  %s", before, after)
			}

			a := live.Brains[0].Agent
			if live.Approvals.Enabled {
				t.Error("the LIVE config now has approvals on: the button reached through the copy")
			}
			if live.Approvals.TTL != "30m" {
				t.Errorf("the live TTL is %q, want %q", live.Approvals.TTL, "30m")
			}
			if a.EffectCeiling != "" {
				t.Errorf("the LIVE ceiling is now %q: the button reached through the copy", a.EffectCeiling)
			}
			if a.Governance[0].Mode != "shadow" {
				t.Errorf("the LIVE governance is now %q: the button reached through the copy", a.Governance[0].Mode)
			}
			if len(a.WebhookCall.AllowHosts) != 1 {
				t.Errorf("the LIVE allow-list grew to %v: the button reached through the copy", a.WebhookCall.AllowHosts)
			}
		})
	}
}

// TestAction_enableApprovalsKeepsTheOperatorsTTL: the switch is the only thing
// this button may change. Replacing the whole block would reset a window the
// operator chose, which is a change nobody asked it for.
//
// PROBING MUTATION: assign a fresh ApprovalsConfig without carrying the TTL.
func TestAction_enableApprovalsKeepsTheOperatorsTTL(t *testing.T) {
	withAdminToken(t)
	live := &config.Config{
		Admin:     &config.AdminConfig{TokenEnv: adminEnvVar},
		Approvals: &config.ApprovalsConfig{Enabled: false, TTL: "12m"},
		Brains: []config.BrainConfig{whatsHappeningBrain(&config.AgentConfig{
			Tools: []string{"webhook_call"},
		})},
	}
	var sent *config.Config
	r := &capturingReloader{live: live, onRequest: func(c *config.Config) { sent = c }}
	mux := http.NewServeMux()
	registerWhats(mux, r)
	srv := serve(t, mux)

	post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	if sent == nil || sent.Approvals == nil {
		t.Fatal("no config reached the reloader")
	}
	if !sent.Approvals.Enabled {
		t.Fatal("the button did not turn approvals on")
	}
	if sent.Approvals.TTL != "12m" {
		t.Fatalf("the TTL became %q: the button reset a window the operator chose", sent.Approvals.TTL)
	}
}

type capturingReloader struct {
	live      *config.Config
	onRequest func(*config.Config)
}

func (r *capturingReloader) RequestReload(c *config.Config) (supervisor.Handle, error) {
	r.onRequest(c)
	return supervisor.Handle("reload-capture"), nil
}
func (r *capturingReloader) Status(supervisor.Handle) supervisor.State {
	return supervisor.StateSucceeded
}
func (r *capturingReloader) CurrentConfig() *config.Config { return r.live }
