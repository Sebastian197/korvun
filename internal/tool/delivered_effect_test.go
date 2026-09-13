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
	"io"
	"log"
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
// The branch is forced DETERMINISTICALLY: the handler declares a 64-byte body
// and writes ten, so net/http closes the connection short and io.ReadAll
// returns io.ErrUnexpectedEOF on every platform.
//
// The first shape of this test tore the connection with a TCP RST and carried a
// `t.Skip` for the case where the tear did not produce a read error. The
// director's ruling is the reason it is gone: a mould that can stop running in
// silence is not a mould, and a guarantee behind one is NOT PROVEN. The skip
// was also hiding a defect of its own — it called w.WriteHeader BEFORE setting
// Content-Length, so that header never reached the wire and the whole attack
// rested on the kernel's timing.
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
		// The POST is ACCEPTED and answered — and the answer is cut short.
		// Content-Length goes in BEFORE WriteHeader or it never reaches the
		// wire.
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("0123456789"))
	}))
	// net/http logs the short write; the test is about the client's read.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{strings.TrimPrefix(srv.URL, "http://")}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), srv.URL+` {"event":"ping"}`)
	if execErr == nil {
		t.Fatal("a body cut short must not read as a successful call")
	}
	// The oracle that this really IS the post-delivery branch, and not some
	// other failure wearing the same error: the server saw the payload.
	select {
	case got := <-delivered:
		if !strings.Contains(got, "ping") {
			t.Fatalf("the server did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server — this is not the post-delivery branch")
	}
	if !errors.Is(execErr, io.ErrUnexpectedEOF) {
		t.Fatalf("want the short-body read error, got %v — the branch under test is io.ReadAll's", execErr)
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
