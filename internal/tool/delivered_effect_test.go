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
	// Class exclusivity (Delta 6): a delivered-or-unknown error never ALSO
	// carries the not-sent class, which closes FAILED.
	if errors.Is(execErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", execErr)
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
	// Class exclusivity (Delta 6): a delivered-or-unknown error never ALSO
	// carries the not-sent class, which closes FAILED.
	if errors.Is(execErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", execErr)
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
// Probing mutations of the pre-2026-09-19 shape (executed then, declared in
// that canto): drop the WroteRequest hook; add an untyped branch past the Do
// call, as an inline return, via a one-result helper, or as a naked return.
// «Drop the sentinel from any branch» was NOT true of every branch: an earlier
// assignment accounts every later return of the same variable, so the
// read-response branch could lose its sentinel with this guard green (found
// 2026-09-19; the behavioural test named in this godoc's branch table is
// what reddens).
//
// ELEVATED 2026-09-19 (v0.15.1 block A, director's decision): the observation
// that decides is now httptrace's GotConn — a clean WroteRequest fires before
// the transport's final flush and proves nothing (Go 1.26.6,
// net/http/request.go Request.write and net/http/transport.go writeLoop). The
// guard used to require a WroteRequest hook, which a decorative hook would
// satisfy; it now requires a GotConn hook whose observation is READ outside
// the hook, and it accepts as accounting the three sentinels of the one table
// (ErrEffectDelivered, ErrDeliveryUnknown, ErrNotSent) or that observation.
// WHAT THIS GUARD PINS, and nothing wider: (1) a GotConn hook that stores an
// observation; (2) that observation READ outside the hook — an identifier use
// that is not an assignment target, so a `:=` define does not count (the
// decorative hook, executed red 2026-09-19); (3) every post-Do error return
// syntactically carrying a table sentinel, the observation, or a variable
// assigned from one. It is NOT flow analysis: one assignment accounts every
// later return of that variable, so the per-branch sentinel is pinned by
// BEHAVIOUR, branch by branch:
//
//	branch (post-Do / Do error block)     behavioural test that reddens when it loses its sentinel
//	no connection obtained (ErrNotSent)   TestWebhookCall_anEndBeforeAConnectionIsADecidedFailure
//	connection, no answer (DeliveryUnknown) TestWebhookCall_aHostThatReadsAndHangsUpIsADeliveredPost,
//	                                      TestWebhookCall_aDeadPooledConnIsUnknownButNotDelivered,
//	                                      TestWebhookCall_aResetDuringTheBodyIsUnknown
//	refused redirect (ErrRedirectRefused) TestWebhookCall_aRefusedRedirectIsStillADeliveredPost
//	HTTP status outside 2xx               TestWebhookCall_anErrorStatusIsStillADeliveredPost,
//	                                      the non-2xx rows of TestWebhookCall_onlyA2xxIsSuccess
//	read response                         TestWebhookCall_anAcceptedPostThatCannotBeReadIsNotAFailure
//	byte cap                              TestWebhookCall_aCappedAnswerIsStillADeliveredPost
//
// The base's «a response arrived for a request the wire never reported
// writing» assertion has no row: no test reddens when it changes class, so
// the cure DROPS it (a response implies GotConn fired; the assertion guarded a
// WroteRequest observation the cure removes).
//
// Also syntactic, and declared to its width: a post-Do return whose ONLY
// accounting is ErrNotSent is accepted only inside an if whose condition
// mentions the observation. A return that also names a variable assigned
// earlier from a delivered/unknown sentinel is accepted by that assignment,
// whatever else it names — so ErrNotSent joined onto the read-response
// branch's `err` passes this guard (the adversary's T7g, exit 0). That case is
// caught by BEHAVIOUR: the POSITIVE assertion errors.Is(execErr,
// ErrEffectDelivered) of TestWebhookCall_anAcceptedPostThatCannotBeReadIsNotAFailure
// is the line T7g reddens; its exclusivity assertion guards the join form. Class
// EXCLUSIVITY in general (never two classes on one error) is pinned by the
// behavioural moulds: every delivered/unknown mould asserts !errors.Is(err,
// ErrNotSent), every not-sent mould asserts neither ErrDeliveryUnknown nor
// ErrEffectDelivered and, positively, ErrNotSent. What this guard does NOT
// catch — a read by name that decides nothing (`_ = c`, `_ = c.Load()`, an
// `if false`), a shadowed name, a read inside another hook, a trace that is
// built and never installed on the request — the behavioural moulds of the
// table catch. A correct cure that keeps the observation in a struct field or
// behind a pointer the hook does not `.Store` into by name false-reds this
// guard: it fails CLOSED, and the guard is adjusted with it.
//
// Executed at base 2026-09-19 (evidence folder of v0.15.1 block A): dropping
// ErrEffectDelivered from the read-response wrap reddens its test
// (mut-read-response.txt); dropping the delivered wrap of Do's error block
// reddens the refused-redirect test (mut-do-error-wrap.txt). The ErrNotSent
// and ErrDeliveryUnknown arms do not exist before the cure; their reds are
// executed after green.
//
// Planned probing mutations of this guard (after green): drop the GotConn
// hook; keep it with a `:=`-defined store nobody reads (executed red already
// against base with the hook added, mut-guard-decorative-hook.txt).
func TestWebhookCall_everyBranchAfterDo(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "webhookcall.go", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := executeDecl(t, file)

	// Without the observation there is no frontier at all, only the guessing
	// the earlier shapes did. And an observation nobody reads is decoration.
	observed := gotConnObservation(fn)
	if observed == "" {
		t.Fatal("Execute installs no GotConn trace hook that stores an observation — delivery is being inferred from the error's shape again")
	}
	if !readOutsideHook(fn, observed) {
		t.Fatalf("Execute's GotConn hook stores %q and no identifier use outside the hook reads it. This guard pins that syntactic read only; whether the read decides the sentinel is pinned by the behavioural moulds of this godoc's branch table", observed)
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
		if !accountsForDelivery(fn, ret, observed) {
			t.Errorf("%s: an error return past the Do call carries neither ErrEffectDelivered, ErrDeliveryUnknown, the GotConn observation nor a variable assigned from them (an ErrNotSent that is the return's only accounting counts only inside a condition that consults the observation) — a connection may have been obtained, and this would close somebody's ledger FAILED",
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
func accountsForDelivery(fn *ast.FuncDecl, ret *ast.ReturnStmt, observed string) bool {
	// ErrNotSent closes FAILED, so when it is a return's ONLY accounting it is
	// accepted only inside an if whose condition MENTIONS the observation. The
	// condition's polarity is not checked here (the adversary's G3 passes this
	// guard); polarity is pinned by behaviour. A return that also carries a
	// variable accounted by an earlier assignment passes regardless; that is
	// behavioural too
	// (TestWebhookCall_anAcceptedPostThatCannotBeReadIsNotAFailure).
	notSentGuarded := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		is, ok := n.(*ast.IfStmt)
		if !ok || !mentions(is.Cond, observed) {
			return true
		}
		if ret.Pos() >= is.Body.Pos() && ret.End() <= is.Body.End() {
			notSentGuarded = true
		}
		return true
	})
	accounts := func(n ast.Node) bool {
		return mentions(n, "ErrEffectDelivered") || mentions(n, "ErrDeliveryUnknown") ||
			mentions(n, observed) || (notSentGuarded && mentions(n, "ErrNotSent"))
	}
	if accounts(ret) {
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
				if accounts(rhs) {
					accounted = true
				}
			}
		}
		return true
	})
	return accounted
}

// gotConnObservation returns the name of the identifier the GotConn hook
// stores into — `x.Store(…)` or `x = …` inside the function literal keyed
// GotConn — or "" when there is no such hook.
func gotConnObservation(fn *ast.FuncDecl) string {
	name := ""
	ast.Inspect(fn, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "GotConn" {
			return true
		}
		lit, ok := kv.Value.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(lit.Body, func(m ast.Node) bool {
			switch x := m.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Store" {
					if id, ok := sel.X.(*ast.Ident); ok {
						name = id.Name
					}
				}
			case *ast.AssignStmt:
				if id, ok := x.Lhs[0].(*ast.Ident); ok {
					name = id.Name
				}
			}
			return name == ""
		})
		return false
	})
	return name
}

// readOutsideHook reports whether `name` is READ anywhere in fn outside the
// GotConn literal that stores into it. A read is an identifier use that is not
// the target of an assignment: a `var` declaration, a `:=` define and a plain
// `=` to the name are all writes, never reads — otherwise
// `x := &atomic.Bool{}` alone would count as consulting the observation.
func readOutsideHook(fn *ast.FuncDecl, name string) bool {
	targets := map[*ast.Ident]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range as.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					targets[id] = true
				}
			}
		}
		return true
	})
	var skip []ast.Node
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.KeyValueExpr:
			if key, ok := x.Key.(*ast.Ident); ok && key.Name == "GotConn" {
				skip = append(skip, x)
				return false
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				if id.Name == name {
					skip = append(skip, x)
					return false
				}
			}
		}
		return true
	})
	inside := func(n ast.Node) bool {
		for _, sk := range skip {
			if n.Pos() >= sk.Pos() && n.End() <= sk.End() {
				return true
			}
		}
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name && !inside(id) && !targets[id] {
			found = true
		}
		return !found
	})
	return found
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
// until someone writes its post-delivery attack in this file.
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
	// The tools whose post-delivery branches are held BY SITE in this file (by
	// TestWebhookCall_everyBranchAfterDo for webhook_call). A name
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
//
// Corrected 2026-09-19: at that base there was no ErrRedirectRefused arm in
// Do's error block; the delivery came from the WroteRequest observation's
// wrap, and dropping THAT wrap is what reddens this test
// (mut-do-error-wrap.txt, v0.15.1 block A evidence). The v0.15.1 cure adds
// the explicit arm — errors.Is(err, ErrRedirectRefused) ⇒ ErrEffectDelivered,
// because a refused redirect is a RESPONSE: the receiver answered with a 3xx.
// (In THIS test the host also reads the whole body before answering, which is
// why its failure message can say it has the payload.)
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
	// Class exclusivity (Delta 6): a delivered-or-unknown error never ALSO
	// carries the not-sent class, which closes FAILED.
	if errors.Is(execErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", execErr)
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
	// Class exclusivity (Delta 6): a delivered-or-unknown error never ALSO
	// carries the not-sent class, which closes FAILED.
	if errors.Is(execErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", execErr)
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
// ELEVATED 2026-09-19 (v0.15.1 block A, director's decision): from the client
// this case cannot be told apart from a pooled connection that died with zero
// bytes out — no answer arrived either way, and a clean WroteRequest fires
// before the transport's final flush. So the error may not claim delivery:
// its sentinel is ErrDeliveryUnknown, not ErrEffectDelivered. The test had no
// state assertion; it now has one — OUTCOME_UNKNOWN through
// CloseStateAfterRun, the state both paths close on. The name keeps «delivered»
// because the HOST did read the POST; the claim that the tool can prove it is
// what moved.
//
// Planned probing mutations (after green), each named by the assertion it
// reddens (the first Fatalf stops the test, so one mutation observes one
// assertion): remove the GotConn hook or the connection-obtained arm of Do's
// error block ⇒ the SENTINEL assertion reddens; drop ErrDeliveryUnknown from
// CloseStateAfterRun's OUTCOME_UNKNOWN arm (internal/tool/outcome.go) ⇒ the
// STATE assertion reddens with FAILED. Both captured after green.
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
	if !errors.Is(execErr, errDeliveryUnknown) {
		t.Fatalf("a connection was obtained and no answer came, and the error is not ErrDeliveryUnknown: %v", execErr)
	}
	if errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("the error claims a delivery the client cannot prove (no answer was read): %v", execErr)
	}
	// Class exclusivity (Delta 6): a delivered-or-unknown error never ALSO
	// carries the not-sent class, which closes FAILED.
	if errors.Is(execErr, errNotSent) {
		t.Fatalf("the error carries ErrNotSent as well as its own class: %v", execErr)
	}
	if got := CloseStateAfterRun(execErr); got != action.StateOutcomeUnknown {
		t.Fatalf("state = %v, want OUTCOME_UNKNOWN — the host read the POST", got)
	}
}

// TestWebhookCall_aDialThatNeverConnectsDeliveredNothing is the other
// direction, and it is the one that keeps the cure honest: without it, wrapping
// every Do error in the sentinel would pass every delivered-post test of this
// file while telling the
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
	// Class exclusivity (Delta 6): a not-sent error never ALSO carries a
	// delivered or delivery-unknown class.
	if errors.Is(execErr, errDeliveryUnknown) || errors.Is(execErr, ErrEffectDelivered) {
		t.Fatalf("a not-sent error also carries a delivered/unknown class: %v", execErr)
	}
	// The POSITIVE class assertion (Delta 7): it IS ErrNotSent, carrying its
	// exact text. Without it, a stand-in left unswitched — or a cure with no
	// ErrNotSent at all — passes every exclusivity assertion.
	if !errors.Is(execErr, errNotSent) || errNotSent.Error() != notSentText {
		t.Fatalf("err %v is not ErrNotSent (%q), or ErrNotSent's text is not %q", execErr, errNotSent, notSentText)
	}
}
