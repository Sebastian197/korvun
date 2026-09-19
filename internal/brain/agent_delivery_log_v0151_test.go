// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The `delivery` field of the cage-denial log record — v0.15.1 block A.
//
// It replaced a boolean `delivered` that read false for a connection obtained
// with no answer, which is a claim nobody can make. The field is logged only
// when a cage or shield refusal denies the tool (runTool's cageRule branch),
// so the rows drive the three refusals that can reach it through the REAL
// webhook_call; the fourth value is reached through deliveryOf itself.
//
// Evidence level, honest: in-process, the real tool through the real runTool
// seam, a real loopback server for the redirect row; the shield row dials a
// TEST-NET address (192.0.2.1) that the shield refuses before any connect.
package brain

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/tool"
)

// recordingHandler keeps the attributes of every record it sees.
type recordingHandler struct {
	mu    sync.Mutex
	attrs []map[string]string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	m := map[string]string{"msg": r.Message}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	h.attrs = append(h.attrs, m)
	h.mu.Unlock()
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// deliveryLogged returns the `delivery` attribute of the cage-denial record.
func (h *recordingHandler) deliveryLogged(t *testing.T) string {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.attrs {
		if m["msg"] == "agent: tool denied by its cage" {
			v, ok := m["delivery"]
			if !ok {
				t.Fatalf("the cage-denial record has no delivery field: %v", m)
			}
			return v
		}
	}
	t.Fatalf("no cage-denial record was logged — the row does not attack what it names (records: %v)", h.attrs)
	return ""
}

// TestRunTool_theCageDenialLogsWhatTheWireEstablished drives each refusal
// that reaches the cage-denial log and asserts the exact `delivery` value.
//
// Planned probing mutations: make deliveryOf answer "response_read" for every
// error ⇒ the not_sent and not_observed rows redden; map ErrDeliveryUnknown
// to "not_sent" ⇒ the unit row of TestDeliveryOf_namesEveryClass reddens.
func TestRunTool_theCageDenialLogsWhatTheWireEstablished(t *testing.T) {
	t.Parallel()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Location", "http://elsewhere.invalid/x")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	redirect.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(redirect.Close)
	redirectHost := strings.TrimPrefix(redirect.URL, "http://")

	cases := []struct {
		name    string
		cfg     tool.WebhookCallConfig
		args    string
		wantLog string
	}{
		{"a refused redirect: the receiver answered",
			tool.WebhookCallConfig{AllowHosts: []string{redirectHost}},
			redirect.URL + ` {"event":"ping"}`, "response_read"},
		{"the shield refuses the dial: nothing was sent",
			tool.WebhookCallConfig{AllowHosts: []string{"192.0.2.1:9"}, PrivateOnly: true},
			`http://192.0.2.1:9 {"event":"ping"}`, "not_sent"},
		{"a host off the allow-list: the wire was never observed",
			tool.WebhookCallConfig{AllowHosts: []string{"allowed.invalid"}},
			`http://other.invalid {"event":"ping"}`, "not_observed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wc, err := tool.WebhookCall(tc.cfg)
			if err != nil {
				t.Fatalf("wire the tool: %v", err)
			}
			h := &recordingHandler{}
			rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: &[]string{}}}
			a := NewAgentBrain(&scriptedModel{}, tool.Registry{"webhook_call": wc},
				WithAgentLogger(slog.New(h)), WithActionRecorder(rec))
			a.runTool(context.Background(), kernelEnv(), nil, laneText, "webhook_call", tc.args)
			if got := h.deliveryLogged(t); got != tc.wantLog {
				t.Fatalf("delivery = %q, want %q", got, tc.wantLog)
			}
		})
	}
}

// TestDeliveryOf_namesEveryClass pins the mapping for all four values. The
// "unknown" value is reached only here: no cage or shield refusal fires after
// a connection was obtained and before an answer, so the cage-denial record
// cannot carry it through the real tool today (declared).
func TestDeliveryOf_namesEveryClass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("x: %w", tool.ErrEffectDelivered), "response_read"},
		{fmt.Errorf("x: %w", tool.ErrDeliveryUnknown), "unknown"},
		{fmt.Errorf("x: %w", tool.ErrNotSent), "not_sent"},
		{fmt.Errorf("x: %w", tool.ErrCageViolation), "not_observed"},
	}
	for _, tc := range cases {
		if got := deliveryOf(tc.err); got != tc.want {
			t.Fatalf("deliveryOf(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
