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
// Three shapes of this guard, and the first two were built around a premise
// that was false:
//
//   - by TOOL NAME, a map typed by hand. It could only redden for a tool that
//     did not exist yet, never for a branch of the one already inside, and two
//     untyped branches walked through it;
//   - by SYNTACTIC POSITION, «past the end of Do's error block». That assumed
//     every error out of Do means nothing left, which is not what Do does: a
//     host that reads the whole POST and closes without answering returns an
//     EOF from inside that block. The guard could not reach the branch. It also
//     asked whether a return NODE mentions the sentinel, which let a one-result
//     helper return and a naked return past the frontier through, and reddened
//     a correctly-typed error hoisted into a variable.
//
// The frontier is not a position in the syntax. It is a FACT about the wire,
// and the tool now observes it with httptrace. So this guard asks the only
// question worth asking: does every error leaving this function past the Do
// call ACCOUNT for that observation — by carrying the sentinel, by consulting
// the observation, or by returning a variable that was given one?
//
// A shape it cannot analyse — a naked return, a return of some helper's two
// results — fails loudly instead of passing. A guard that cannot see a shape
// must say so; that is the whole lesson of the two before it.
//
// Probing mutations (executed, red, declared in the canto): drop the sentinel
// from any branch; drop the WroteRequest hook; add an untyped branch past the
// Do call, as an inline return, via a one-result helper, or as a naked return.
func TestWebhookCall_everyBranchAfterDo(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "webhookcall.go", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := executeDecl(t, file)

	// Without the observation there is no frontier at all, only the guessing
	// the two previous shapes did.
	if !mentions(fn, "WroteRequest") {
		t.Fatal("Execute installs no WroteRequest trace hook — delivery is being inferred from the error's shape again, and that was wrong twice")
	}

	var doPos token.Pos
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
			doPos = call.Pos()
			return false
		}
		return true
	})
	if doPos == token.NoPos {
		t.Fatal("no client.Do call in webhookCallTool.Execute — the mould's anchor moved, the rule did not")
	}

	checked := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || ret.Pos() <= doPos {
			return true
		}
		if len(ret.Results) != 2 {
			t.Errorf("%s: a return past the Do call has %d results — this guard reads the error it returns, so inline it instead of hiding it behind a helper or a naked return",
				fset.Position(ret.Pos()), len(ret.Results))
			return true
		}
		if id, ok := ret.Results[1].(*ast.Ident); ok && id.Name == "nil" {
			return true // a successful answer says nothing about delivery
		}
		checked++
		if !accountsForDelivery(fn, ret) {
			t.Errorf("%s: an error return past the Do call neither carries ErrEffectDelivered nor consults the delivery observation — if the body reached the wire, this closes somebody's ledger FAILED over an effect that already left",
				fset.Position(ret.Pos()))
		}
		return true
	})
	if checked < 3 {
		t.Fatalf("found only %d post-Do error returns — the scan is broken, not the tool", checked)
	}
}

// accountsForDelivery reports whether one error return has been through the
// delivery question: the sentinel in the returned expression, the observation
// consulted in it, or a variable it returns that was assigned either.
func accountsForDelivery(fn *ast.FuncDecl, ret *ast.ReturnStmt) bool {
	if mentions(ret, "ErrEffectDelivered") || mentions(ret, "delivered") {
		return true
	}
	// The error may be built earlier and returned through a variable. Collect
	// the identifiers this return mentions and look for an assignment to any
	// of them that did the accounting.
	named := map[string]bool{}
	ast.Inspect(ret, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			named[id.Name] = true
		}
		return true
	})
	accounted := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Pos() > ret.Pos() {
			return true
		}
		for _, lhs := range as.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || !named[id.Name] {
				continue
			}
			for _, rhs := range as.Rhs {
				if mentions(rhs, "ErrEffectDelivered") || mentions(rhs, "delivered") {
					accounted = true
				}
			}
		}
		return true
	})
	return accounted
}

// executeDecl returns the declaration of (*webhookCallTool).Execute.
func executeDecl(t *testing.T, file *ast.File) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Execute" || fn.Recv == nil || fn.Body == nil {
			continue
		}
		return fn
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

// TestWebhookCall_aHostThatReadsAndHangsUpIsADeliveredPost is the eighth pass's
// P1-1, reproduced: the host reads the whole POST and closes the socket without
// answering. Do returns a bare EOF — no cage sentinel, no deadline — and both
// previous cures classified that as «nothing left».
//
// It is the plainest shape of the whole class: a receiver that takes the
// request and dies. The ledger closed FAILED over it twice.
//
// Probing mutation (executed, red, declared in the canto): remove the
// WroteRequest hook, or the `if delivered.Load()` wrap in Do's error block.
func TestWebhookCall_aHostThatReadsAndHangsUpIsADeliveredPost(t *testing.T) {
	t.Parallel()
	read := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		select {
		case read <- string(b):
		default:
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close() // not one byte of answer
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{strings.TrimPrefix(srv.URL, "http://")}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), srv.URL+` {"event":"ping"}`)
	if execErr == nil {
		t.Fatal("a host that answers nothing must not read as a successful call")
	}
	select {
	case got := <-read:
		if !strings.Contains(got, "ping") {
			t.Fatalf("the host did not receive the payload: %q", got)
		}
	default:
		t.Fatal("the POST never reached the server — this is not the post-delivery branch")
	}
	// The oracle that the OLD reasoning cannot save this case: the error wears
	// no cage sentinel and no deadline, which is exactly why both previous
	// frontiers let it through.
	if errors.Is(execErr, ErrCageViolation) || errors.Is(execErr, context.DeadlineExceeded) {
		t.Fatalf("this branch must be the bare transport error, or it is not the one under test: %v", execErr)
	}
	if !errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("the host READ the whole POST and the error does not say so: %v", execErr)
	}
}

// TestWebhookCall_aDialThatNeverConnectsDeliveredNothing is the other
// direction, and it is the one that keeps the cure honest: without it, wrapping
// every Do error in the sentinel would pass every test above while telling the
// operator «we do not know» about a call that never left the machine.
//
// Probing mutation (executed, red, declared in the canto): wrap Do's error
// unconditionally, ignoring the observation.
func TestWebhookCall_aDialThatNeverConnectsDeliveredNothing(t *testing.T) {
	t.Parallel()
	// A listener opened and closed: the port is allow-listed and answers
	// nothing, so the dial is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	wc, err := WebhookCall(WebhookCallConfig{AllowHosts: []string{addr}})
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	_, execErr := wc.Execute(context.Background(), "http://"+addr+` {"event":"ping"}`)
	if execErr == nil {
		t.Fatal("a dial to a closed port must fail")
	}
	if errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("nothing reached the wire and the error claims delivery: %v", execErr)
	}
}
