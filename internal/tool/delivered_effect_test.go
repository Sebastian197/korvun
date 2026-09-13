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
	"go/ast"
	"go/parser"
	"go/token"
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

// TestWebhookCall_everyBranchAfterDo holds the post-delivery rule BY SITE.
//
// The first shape of this guard was TestBuiltins_…HasADriverForItsPostDelivery‑
// Branch: it crossed the effect catalog against a map of tool names typed by
// hand, `driven := map[string]bool{"webhook_call": true}`. That is a guard by
// NAME where the rule is about a BRANCH — class (g) of the checklist, and the
// seventh adversarial pass walked straight through it: TWO further branches of
// webhook_call's own Execute stayed untyped (the cage's redirect refusal and
// an HTTP error status), both of them closing the ledger FAILED over a POST the
// host had already received, and the name guard stayed green because no new
// tool had appeared.
//
// This reads the AST instead. The frontier is not the Do CALL — the transport's
// own failures are returned just after it and mean nothing left — it is the end
// of Do's `if err != nil` block: past that point a response exists, so the host
// has the body. Every error return past it must carry ErrEffectDelivered, and
// the error block itself must name ErrRedirectRefused, which is the one
// transport error raised over a response.
//
// Probing mutations (executed, red, declared in the canto): drop the sentinel
// from the redirect wrap, from the HTTP-status wrap, from the read-response
// wrap or from the cap wrap; add a new untyped error return past the block.
func TestWebhookCall_everyBranchAfterDo(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "webhookcall.go", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body := executeBody(t, file)

	// The Do call, and the `if err != nil` that owns every transport failure.
	var doPos token.Pos
	var errBlock *ast.IfStmt
	for i, stmt := range body.List {
		as, ok := stmt.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			continue
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Do" {
			continue
		}
		doPos = call.Pos()
		if i+1 < len(body.List) {
			errBlock, _ = body.List[i+1].(*ast.IfStmt)
		}
		break
	}
	if doPos == token.NoPos {
		t.Fatal("no client.Do call in webhookCallTool.Execute — the mould's anchor moved, the rule did not")
	}
	if errBlock == nil {
		t.Fatal("the Do call is not followed by its error block — the frontier cannot be located")
	}
	if !mentions(errBlock, "ErrRedirectRefused") {
		t.Error("Do's error block does not name ErrRedirectRefused — a redirect refusal is raised over a RESPONSE, so its POST was delivered, and nothing here tells it from a dial failure")
	}

	// Past the block, a response exists.
	checked := 0
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || ret.Pos() <= errBlock.End() || len(ret.Results) != 2 {
			return true
		}
		if id, ok := ret.Results[1].(*ast.Ident); ok && id.Name == "nil" {
			return true // a successful answer says nothing about delivery
		}
		checked++
		if !mentions(ret, "ErrEffectDelivered") {
			t.Errorf("%s: an error return past Do's error block does not carry ErrEffectDelivered — the host already has the body, and the ledger will close this FAILED",
				fset.Position(ret.Pos()))
		}
		return true
	})
	if checked < 3 {
		t.Fatalf("found only %d post-delivery error returns — the scan is broken, not the tool", checked)
	}
}

// executeBody returns the body of (*webhookCallTool).Execute.
func executeBody(t *testing.T, file *ast.File) *ast.BlockStmt {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Execute" || fn.Recv == nil || fn.Body == nil {
			continue
		}
		return fn.Body
	}
	t.Fatal("webhookcall.go declares no Execute method")
	return nil
}

// mentions reports whether an identifier appears anywhere under n.
func mentions(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		if id, ok := x.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// TestBuiltins_everyIrreversibleToolHasADriverForItsPostDeliveryBranch keeps// TestBuiltins_everyIrreversibleToolHasADriverForItsPostDeliveryBranch keeps
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
	// The tools whose post-delivery branches are held BY SITE above. A name
	// belongs here only once a mould reads that tool's own Execute; this map
	// is the index of those moulds, never the guarantee itself — the guarantee
	// is TestWebhookCall_everyBranchAfterDo, and the defect this map had on its
	// own was letting webhook_call grow two untyped branches unnoticed.
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

// TestWebhookCall_aRefusedRedirectIsStillADeliveredPost is the seventh pass's
// P1-1, first half, reproduced verbatim: an allow-listed host receives the
// payload and answers 301 to its own path. The cage refuses to follow — and
// the POST is already in.
//
// Probing mutation (executed, red, declared in the canto): drop
// ErrEffectDelivered from the ErrRedirectRefused arm of Do's error block.
func TestWebhookCall_aRefusedRedirectIsStillADeliveredPost(t *testing.T) {
	t.Parallel()
	delivered := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 64)
		n, _ := r.Body.Read(body)
		select {
		case delivered <- string(body[:n]):
		default:
		}
		// The http→https upgrade every reverse proxy in the world does.
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{strings.TrimPrefix(srv.URL, "http://")}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), srv.URL+`/hook {"event":"fire"}`)
	if execErr == nil {
		t.Fatal("the cage must refuse to follow the redirect")
	}
	select {
	case got := <-delivered:
		if !strings.Contains(got, "fire") {
			t.Fatalf("the allow-listed host did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server — this is not the post-delivery branch")
	}
	if !errors.Is(execErr, ErrCageViolation) {
		t.Fatalf("a refused redirect is still a cage refusal: %v", execErr)
	}
	if !errors.Is(execErr, ErrRedirectRefused) {
		t.Fatalf("the refusal does not name itself: %v", execErr)
	}
	if !errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("the host HAS the payload and the error does not say so: %v", execErr)
	}
}

// TestWebhookCall_anErrorStatusIsStillADeliveredPost is the other half: the
// receiver read the body, did whatever it did, and answered 500. Closing that
// FAILED tells the operator the irreversible call did not happen, which is the
// one thing nobody here knows.
//
// Probing mutation (executed, red, declared in the canto): drop
// ErrEffectDelivered from the status wrap.
func TestWebhookCall_anErrorStatusIsStillADeliveredPost(t *testing.T) {
	t.Parallel()
	delivered := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 64)
		n, _ := r.Body.Read(body)
		select {
		case delivered <- string(body[:n]):
		default:
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{strings.TrimPrefix(srv.URL, "http://")}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), srv.URL+` {"event":"fire"}`)
	if execErr == nil {
		t.Fatal("an HTTP 500 must not read as a successful call")
	}
	select {
	case got := <-delivered:
		if !strings.Contains(got, "fire") {
			t.Fatalf("the host did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server — this is not the post-delivery branch")
	}
	if !errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("the receiver READ the body before answering 500, and the error does not say so: %v", execErr)
	}
}
