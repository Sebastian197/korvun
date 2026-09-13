// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The post-delivery frontier — the sixth adversarial pass's P2-3, taken
// inside the train because the sentence it left alive is a definite claim
// about an irreversible effect.
//
// internal/app closes a deferred approved execution FAILED for every error
// that is not classified unknown, and said so in its own words: «the tool ran
// and said no. That is a DECIDED outcome with its receipt.» For a webhook_call
// whose POST the remote end ACCEPTED and whose body then failed to read — or
// breached the size cap — the POST went out. The ledger recorded a refusal
// that did not happen.
//
// Evidence level, honest: in-process, against a real loopback HTTP server that
// accepts the request and then breaks the connection. No fake transport.
package tool

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// TestWebhookCall_anAcceptedPostThatCannotBeReadIsNotAFailure forces the
// dangerous branch: the handler reads the body (the effect IS delivered) and
// then hijacks the connection and closes it, so io.ReadAll fails.
//
// Probing mutation (executed, red, declared in the canto): drop
// `ErrEffectDelivered` from the read-response wrap ⇒ this reddens.
func TestWebhookCall_anAcceptedPostThatCannotBeReadIsNotAFailure(t *testing.T) {
	t.Parallel()
	delivered := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 64)
		n, _ := r.Body.Read(body)
		delivered <- string(body[:n])
		// The status line goes out, then the connection dies under the body.
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Length", "64")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the test server cannot hijack — the attack needs a torn connection")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetLinger(0) // RST, not FIN: the read fails, it does not EOF cleanly
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{strings.TrimPrefix(srv.URL, "http://")}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), srv.URL+` {"event":"ping"}`)
	if execErr == nil {
		t.Skip("the connection survived the tear on this platform; the branch needs a real failure to judge")
	}
	select {
	case got := <-delivered:
		if !strings.Contains(got, "ping") {
			t.Fatalf("the server did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server — this is not the post-delivery branch")
	}
	if !errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("the POST was ACCEPTED and the answer lost, and the error does not say so: %v", execErr)
	}
}

// TestWebhookCall_aCappedAnswerIsStillADeliveredPost is the other post-delivery
// branch: the remote end accepted the POST and answered more bytes than the
// cage allows. The cage refusal stands AND the effect happened.
//
// Probing mutation (executed, red, declared in the canto): drop
// `ErrEffectDelivered` from the cap wrap ⇒ this reddens.
func TestWebhookCall_aCappedAnswerIsStillADeliveredPost(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 64)))
	}))
	defer srv.Close()
	wc, err := WebhookCall(WebhookCallConfig{
		AllowHosts: []string{strings.TrimPrefix(srv.URL, "http://")},
		MaxBytes:   8,
	})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), srv.URL+` {"event":"ping"}`)
	if execErr == nil {
		t.Fatal("an answer over the cap must refuse")
	}
	if !errors.Is(execErr, ErrCageViolation) {
		t.Fatalf("the cap is still a cage refusal: %v", execErr)
	}
	if !errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("the POST was ACCEPTED before the cap fired, and the error does not say so: %v", execErr)
	}
}

// TestBuiltins_everyIrreversibleToolHasADriverForItsPostDeliveryBranch keeps
// the class OPEN. The catalog is read out of effects.go, not typed here: the
// defect this train kept repeating is curing one door and calling the class
// cured, so a NEW tool declared write_irreversible or critical reddens this
// until someone writes its post-delivery attack above.
func TestBuiltins_everyIrreversibleToolHasADriverForItsPostDeliveryBranch(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("effects.go")
	if err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	names := regexp.MustCompile(`case "([a-z_]+)"(?:, "([a-z_]+)")*:`).
		FindAllStringSubmatch(string(src), -1)
	if len(names) < 4 {
		t.Fatalf("read only %d catalog arms — the scan is broken, not the catalog", len(names))
	}
	// The tools with a post-delivery attack in this file.
	driven := map[string]bool{"webhook_call": true}
	seen := 0
	for _, raw := range regexp.MustCompile(`"([a-z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		d, ok := BuiltinEffects(raw[1])
		if !ok {
			continue
		}
		seen++
		if d.Class.Rank() < action.EffectWriteIrreversible.Rank() {
			continue
		}
		if !driven[raw[1]] {
			t.Errorf("%q is declared %s — it can park for approval, and no test here forces its post-delivery branch", raw[1], d.Class)
		}
	}
	if seen < 4 {
		t.Fatalf("resolved only %d builtin descriptors — the scan is broken", seen)
	}
}
