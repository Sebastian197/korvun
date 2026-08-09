// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Structured params for the native lane (the 2026-08-09 demo lesson: a small
// model cannot compose "URL space JSON" into one string, but it fills
// separate structured fields reliably). A ParamTool declares simple named
// string fields and reconstructs the Tool seam's args string itself —
// Execute's contract stays untouched, the gate stays single-path.

func TestWebhookCall_isAParamTool(t *testing.T) {
	t.Parallel()
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{"127.0.0.1:8765"}})
	pt, ok := wc.(ParamTool)
	if !ok {
		t.Fatal("webhook_call must declare structured params for the native lane")
	}
	params := pt.Params()
	names := make(map[string]bool, len(params))
	for _, p := range params {
		names[p.Name] = true
		if p.Description == "" {
			t.Errorf("param %q has no description", p.Name)
		}
	}
	if !names["url"] || !names["message"] {
		t.Fatalf("params = %+v, want url and message fields", params)
	}
}

func TestWebhookCall_argsFromCall(t *testing.T) {
	t.Parallel()
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{"127.0.0.1:8765"}})
	pt := wc.(ParamTool)

	cases := []struct {
		name    string
		fields  map[string]any
		want    string // the reconstructed seam args (URL space JSON)
		wantErr string // fragment of a USEFUL error, empty = success
	}{
		{
			name:   "plain message becomes a JSON body",
			fields: map[string]any{"url": "http://127.0.0.1:8765/aviso", "message": "hola"},
			want:   `http://127.0.0.1:8765/aviso {"message":"hola"}`,
		},
		{
			name:   "message with quotes survives marshaling",
			fields: map[string]any{"url": "http://127.0.0.1:8765/aviso", "message": `di "hola"`},
			want:   `http://127.0.0.1:8765/aviso {"message":"di \"hola\""}`,
		},
		{
			name:   "explicit valid json wins over message",
			fields: map[string]any{"url": "http://127.0.0.1:8765/aviso", "json": `{"aviso":"hola"}`},
			want:   `http://127.0.0.1:8765/aviso {"aviso":"hola"}`,
		},
		{
			name:   "invalid explicit json degrades to a message body (tolerant)",
			fields: map[string]any{"url": "http://127.0.0.1:8765/aviso", "json": `{aviso: hola}`},
			want:   `http://127.0.0.1:8765/aviso {"message":"{aviso: hola}"}`,
		},
		{
			name:    "missing url is a useful error",
			fields:  map[string]any{"message": "hola"},
			wantErr: "url",
		},
		{
			name:    "missing message and json is a useful error",
			fields:  map[string]any{"url": "http://127.0.0.1:8765/aviso"},
			wantErr: "message",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := pt.ArgsFromCall(tc.fields)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want a useful error naming %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ArgsFromCall: %v", err)
			}
			if got != tc.want {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

// The reconstructed args round-trip through the REAL Execute: a message-born
// payload passes the tool's own JSON validation and reaches the endpoint.
func TestWebhookCall_paramsRoundTripThroughExecute(t *testing.T) {
	t.Parallel()
	var gotBody string
	srv, hits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = fmt.Fprint(w, "ack")
	})
	wc := mustWebhookCall(t, WebhookCallConfig{AllowHosts: []string{hostPortOf(t, srv)}})
	pt := wc.(ParamTool)

	args, err := pt.ArgsFromCall(map[string]any{"url": srv.URL + "/aviso", "message": "hola"})
	if err != nil {
		t.Fatalf("ArgsFromCall: %v", err)
	}
	out, err := wc.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute(reconstructed args): %v", err)
	}
	if out != "ack" || hits.Load() != 1 {
		t.Fatalf("out=%q hits=%d, want ack with one hit", out, hits.Load())
	}
	if gotBody != `{"message":"hola"}` {
		t.Fatalf("endpoint received %q, want the marshaled message body", gotBody)
	}
}

// Pure tools remain param-less (no ParamTool) — the uniform args schema
// stays their contract.
func TestPureTools_areNotParamTools(t *testing.T) {
	t.Parallel()
	for _, tl := range []Tool{Calc(), Echo(), Time(nil)} {
		if _, ok := tl.(ParamTool); ok {
			t.Fatalf("pure tool %q must not declare params", tl.Name())
		}
	}
}
