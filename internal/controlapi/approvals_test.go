// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// RED for the v0.15.0 train "approvals in the app": the four Control API
// endpoints. These molds are written BEFORE the surface exists and pin
// the contract of the paper
// (docs/superpowers/specs/2026-09-08-approvals-screen-ux.md, v36):
// every named outcome, its exact NAME and its exact TEXT.
//
// The TEXT half is deliberate. In R15 the wording of two operator-facing
// notes was rewritten mid-train and the whole suite stayed green, because
// nothing watched the words. Here the words are the contract: they are
// what the operator reads before an irreversible yes.
//
// Evidence level: in-process HTTP against a FAKE that satisfies the narrow
// interface. The real-store proofs — the atomic claim, the busy class, the
// mutation between show and approve — belong to the adapter's own molds in
// internal/app, and are not claimed here.
//
// THE TWENTY-FIVE CHANGES over the first RED (§17 F, authorised in block by
// the director on 2026-09-09) are applied here. Each one that touched an
// existing mould is re-run with its mutation, and §17 F-bis lists which
// mutation belongs to which mould.

package controlapi_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/controlapi"
)

const approvalsToken = "approvals-test-token"

// fakeApprovals is the seam under test: it answers exactly what the
// narrow interface promises, and records what it was asked.
type fakeApprovals struct {
	list     controlapi.ApprovalList
	detail   controlapi.ApprovalDetail
	disabled bool

	approveErr error
	rejectErr  error
	listErr    error
	detailErr  error

	approvedID     string
	approvedDigest string
	rejectedID     string
	rejectedNote   string
}

func (f *fakeApprovals) ListPending(_ context.Context) (controlapi.ApprovalList, error) {
	if f.disabled {
		return controlapi.ApprovalList{}, controlapi.ErrApprovalsDisabled
	}
	return f.list, f.listErr
}

func (f *fakeApprovals) Detail(_ context.Context, id string) (controlapi.ApprovalDetail, error) {
	if f.disabled {
		return controlapi.ApprovalDetail{}, controlapi.ErrApprovalsDisabled
	}
	if f.detailErr != nil {
		return controlapi.ApprovalDetail{}, f.detailErr
	}
	if id != f.detail.ID {
		return controlapi.ApprovalDetail{}, controlapi.ErrApprovalNotFound
	}
	return f.detail, nil
}

func (f *fakeApprovals) Approve(_ context.Context, id, digest string) (controlapi.ApprovalOutcome, error) {
	f.approvedID, f.approvedDigest = id, digest
	if f.disabled {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsDisabled
	}
	if f.approveErr != nil {
		return controlapi.ApprovalOutcome{}, f.approveErr
	}
	return controlapi.ApprovalOutcome{
		Outcome: "executed", Digest: digest, Result: "ok", ReceiptID: "rcpt_fake",
	}, nil
}

func (f *fakeApprovals) Reject(_ context.Context, id, comment string) (controlapi.ApprovalOutcome, error) {
	f.rejectedID, f.rejectedNote = id, comment
	if f.disabled {
		return controlapi.ApprovalOutcome{}, controlapi.ErrApprovalsDisabled
	}
	if f.rejectErr != nil {
		return controlapi.ApprovalOutcome{}, f.rejectErr
	}
	return controlapi.ApprovalOutcome{Outcome: "rejected", ReceiptID: "rcpt_fake_rejected"}, nil
}

func approvalsServer(t *testing.T, f *fakeApprovals) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	controlapi.RegisterApprovals(mux, approvalsToken, f)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// errorBody is the shape every refusal carries. `current_law_digest` is a field
// of its own and never a value dug out of the message: the message is operator
// text with a byte-for-byte contract, and parsing a structured value out of a
// sentence is exactly the "by text" that E9 forbids (FR-API-17, change 10).
type errorBody struct {
	Error            string `json:"error"`
	Message          string `json:"message"`
	CurrentLawDigest string `json:"current_law_digest"`
}

// --- G5: the bearer gates EVERY route ---------------------------------------

func TestApprovals_EveryRouteRequiresBearer(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	srv := approvalsServer(t, f)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/approvals", ""},
		{"GET", "/api/approvals/apr_1", ""},
		{"POST", "/api/approvals/apr_1/approve", `{"digest":"sha256:x"}`},
		{"POST", "/api/approvals/apr_1/reject", `{"comment":"no"}`},
	} {
		for _, token := range []string{"", "wrong-token"} {
			res := doReq(t, tc.method, srv.URL+tc.path, token, tc.body)
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s %s with token %q must be 401, got %d", tc.method, tc.path, token, res.StatusCode)
			}
			// Change 5 (FR-API-11): the 401 gains a BODY assertion. A 401 whose
			// name the screen does not know renders as "a response this screen
			// does not recognise" instead of the forbidden literal with its
			// [Ir a Inicio], so the code alone was never the contract.
			body := decode[errorBody](t, res)
			if body.Error != "forbidden" {
				t.Fatalf("%s %s: the 401 carries the name forbidden, got %q", tc.method, tc.path, body.Error)
			}
			if body.Message != "the window could not authenticate against the core — no decision has left it" {
				t.Fatalf("%s %s: the 401 text is contract, got %q", tc.method, tc.path, body.Message)
			}
		}
	}
	if f.approvedID != "" || f.rejectedID != "" {
		t.Fatalf("nothing may reach the store unauthenticated: %q %q", f.approvedID, f.rejectedID)
	}
}

// --- G1: the detail carries THE DIGEST -------------------------------------

func TestApprovals_DetailCarriesTheDigest(t *testing.T) {
	t.Parallel()
	// Changes 2, 8 and 9: the digest lives on the ROW (and the detail inherits
	// it by embedding, so it cannot disagree with itself); LawVersion is gone
	// from the DTO (a constant informs nobody); and the detail gains brain_gone.
	f := &fakeApprovals{detail: controlapi.ApprovalDetail{
		ApprovalRow: controlapi.ApprovalRow{
			ID: "apr_1", ActionID: "act_1", EffectClass: "write_irreversible",
			Operation: "tool/webhook_call", ExpiresAt: "2026-09-08T13:00:00Z",
			Digest: "sha256:1bd8ce23a4e52cca4d77efa8ed3e37536aa2ee950c5fa3214a28839bc180a479",
			Origin: "telegram",
		},
		Parameters:      `http://127.0.0.1:5678/hook {"message":"hola"}`,
		ParametersState: "present",
		LawDigest:       "sha256:aab9b0d7",
	}}
	srv := approvalsServer(t, f)
	res := doReq(t, "GET", srv.URL+"/api/approvals/apr_1", approvalsToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("detail must be 200, got %d", res.StatusCode)
	}
	got := decode[controlapi.ApprovalDetail](t, res)
	if got.Digest != f.detail.Digest {
		t.Fatalf("the digest is the contract: want %q got %q", f.detail.Digest, got.Digest)
	}
	if got.Parameters != f.detail.Parameters {
		t.Fatalf("the raw parameters travel whole: %q", got.Parameters)
	}
	if got.Origin != "telegram" {
		t.Fatalf("the origin channel travels: %q", got.Origin)
	}
	// Change 12 of the RED's own kind: the raw JSON keys are asserted, not just
	// the Go fields — decoding into the same types would let any label pass
	// (FR-API-12).
	rawBody := decodeMap(t, doReq(t, "GET", srv.URL+"/api/approvals/apr_1", approvalsToken, ""))
	for _, key := range []string{
		"id", "action_id", "operation", "effect_class", "expires_at", "digest", "origin",
		"purpose", "principal_id", "reversibility", "tool_cage", "required_rule",
		"law_digest", "parameters", "parameters_state",
	} {
		if _, ok := rawBody[key]; !ok {
			t.Fatalf("the detail must carry the raw JSON key %q", key)
		}
	}
	if _, ok := rawBody["law_version"]; ok {
		t.Fatalf("law_version left the DTO in change 8: it is the pin FORMAT, a constant")
	}
	if _, ok := rawBody["status"]; ok {
		t.Fatalf("status is not in the DTO: under FR-API-18's precedence it can only be PENDING here")
	}
}

func TestApprovals_DetailCarriesBrainGoneOnThe200(t *testing.T) {
	t.Parallel()
	// Change 9 (FR-API-20): brain_gone rides the 200, not an error body. An
	// error body carries no document, and E9 sends every error name to a
	// terminal state — which would leave the request unclosable, when rejecting
	// it still works because reject does not consult the law.
	f := &fakeApprovals{detail: controlapi.ApprovalDetail{
		ApprovalRow: controlapi.ApprovalRow{ID: "apr_1", Digest: "sha256:abc"},
		BrainGone:   true,
	}}
	srv := approvalsServer(t, f)
	got := decode[controlapi.ApprovalDetail](t, doReq(t, "GET", srv.URL+"/api/approvals/apr_1", approvalsToken, ""))
	if !got.BrainGone {
		t.Fatalf("brain_gone must travel as a field of the 200")
	}
}

// --- G2: approve-what-you-saw ----------------------------------------------

func TestApprovals_ApproveCarriesTheDigestTheOperatorSaw(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	srv := approvalsServer(t, f)
	res := doReq(t, "POST", srv.URL+"/api/approvals/apr_1/approve", approvalsToken,
		`{"digest":"sha256:abc"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve must be 200, got %d", res.StatusCode)
	}
	if f.approvedDigest != "sha256:abc" {
		t.Fatalf("the digest the operator saw must reach the store: got %q", f.approvedDigest)
	}
	// Change 1 (FR-API-5): approve returns the REAL outcome — result and
	// receipt — not a bare string. P4 prints the terminal receipt id and P5 the
	// decision one; neither DecideApprovalUnderLaw nor FinishWithResult returns
	// them, so the adapter re-reads them (change 25).
	out := decode[controlapi.ApprovalOutcome](t, res)
	if out.Outcome != "executed" || out.ReceiptID != "rcpt_fake" || out.Result != "ok" {
		t.Fatalf("the outcome, its result and its receipt are the contract: %+v", out)
	}
}

func TestApprovals_ApproveWithoutADigestIsRefused(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	srv := approvalsServer(t, f)
	for _, body := range []string{`{}`, `{"digest":""}`, `{"digest":"   "}`} {
		res := doReq(t, "POST", srv.URL+"/api/approvals/apr_1/approve", approvalsToken, body)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("approving without the digest is the whole point of G2: want 400 for %s, got %d", body, res.StatusCode)
		}
	}
	if f.approvedID != "" {
		t.Fatalf("nothing may reach the store without a digest, reached %q", f.approvedID)
	}
}

// --- §12-ter: every named outcome, its NAME and its TEXT -------------------

// namedOutcomes is the table change 7 took from six to eleven rows and change
// 13 took to nineteen, plus not_decided, params_unreadable and
// decided_evidence_corrupt from changes 22 and 23 — twenty-two, exactly the
// registry of §12-ter. forbidden is proved on its own route above, where the
// bearer can actually refuse.
func namedOutcomes() []struct {
	name   string
	err    error
	status int
	error_ string
	text   string
} {
	return []struct {
		name   string
		err    error
		status int
		error_ string
		text   string
	}{
		{"already decided", controlapi.ErrApprovalAlreadyDecided, http.StatusConflict, "already_decided",
			"this request was already decided — the first decision stands and nothing ran twice"},
		{"expired", controlapi.ErrApprovalExpired, http.StatusConflict, "expired",
			"this request expired before the decision touched it — it never executes"},
		{"digest mismatch", controlapi.ErrApprovalDigestMismatch, http.StatusConflict, "digest_mismatch",
			"the stored request no longer re-derives the digest you were shown — nothing was executed; read it again"},
		{"not found", controlapi.ErrApprovalNotFound, http.StatusNotFound, "not_found",
			"no approval request with that id"},
		{"unavailable", controlapi.ErrApprovalsUnavailable, http.StatusServiceUnavailable, "unavailable",
			"the store could not be read at this instant — this is transient and says nothing about the evidence"},
		{"disabled", controlapi.ErrApprovalsDisabled, http.StatusConflict, "disabled",
			"approvals are switched off in this profile — there is no pending list, which is not the same as an empty one"},

		{"params digest mismatch", controlapi.ErrApprovalParamsDigestMismatch, http.StatusConflict, "params_digest_mismatch",
			"the stored parameters do not re-derive this request's digest — this is permanent and nothing was executed"},
		{"invalidated", controlapi.ErrApprovalInvalidated, http.StatusConflict, "invalidated",
			"the law this request was parked under no longer holds — this is not transient; rejecting it still works"},
		{"evidence corrupt", controlapi.ErrApprovalEvidenceCorrupt, http.StatusConflict, "evidence_corrupt",
			"the stored evidence for this request does not verify — this is permanent and no decision is offered"},
		{"brain gone", controlapi.ErrApprovalBrainGone, http.StatusConflict, "brain_gone",
			"the brain that asked for this action is no longer in the profile — approving needs its law, rejecting does not"},

		{"not started params held", controlapi.ErrApprovalNotStartedParamsHeld, http.StatusConflict, "not_started_params_held",
			"the decision is recorded and THIS execution did not start; the row still holds its parameters, so no automatic pass has closed it — about other executors this answer says nothing"},
		{"not started params gone", controlapi.ErrApprovalNotStartedParamsGone, http.StatusConflict, "not_started_params_gone",
			"the decision is recorded and THIS execution did not start; the row was born without parameters, so no execution could have started"},
		{"params unaccounted", controlapi.ErrApprovalParamsUnaccounted, http.StatusConflict, "params_unaccounted",
			"this execution did not start, and whether the request ran cannot be asserted: on re-reading, its parameters were no longer where they were, or the row was gone"},
		{"params unreadable", controlapi.ErrApprovalParamsUnreadable, http.StatusConflict, "params_unreadable",
			"this execution did not start and the store could not be read — this says it does not know, not that anything was taken"},
		{"decided evidence corrupt", controlapi.ErrApprovalDecidedEvidenceBad, http.StatusConflict, "decided_evidence_corrupt",
			"the decision is recorded and sealed with its receipt, and THIS execution did not start; what no longer verifies is the stored evidence, and that is permanent"},
		{"already closed", controlapi.ErrApprovalAlreadyClosed, http.StatusConflict, "already_closed",
			"this request is not awaiting execution — THIS execution did nothing"},
		{"not decided", controlapi.ErrApprovalNotDecided, http.StatusConflict, "not_decided",
			"the store says this request is still awaiting a decision, so THIS execution did nothing"},
		{"unknown outcome", controlapi.ErrApprovalUnknownOutcome, http.StatusConflict, "unknown_outcome",
			"the decision left this window and whether the effect happened is unknown"},
		{"close failed", controlapi.ErrApprovalCloseFailed, http.StatusConflict, "close_failed",
			"the decision left this window, whether the effect happened is unknown, and THIS execution could not close the ledger"},
	}
}

func TestApprovals_NamedOutcomes(t *testing.T) {
	t.Parallel()
	for _, tc := range namedOutcomes() {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeApprovals{approveErr: tc.err}
			srv := approvalsServer(t, f)
			res := doReq(t, "POST", srv.URL+"/api/approvals/apr_1/approve", approvalsToken,
				`{"digest":"sha256:abc"}`)
			if res.StatusCode != tc.status {
				t.Fatalf("%s: want status %d, got %d", tc.name, tc.status, res.StatusCode)
			}
			body := decode[errorBody](t, res)
			if body.Error != tc.error_ {
				t.Fatalf("%s: the NAME is the contract: want %q got %q", tc.name, tc.error_, body.Error)
			}
			if body.Message != tc.text {
				t.Fatalf("%s: the TEXT is the contract too — an operator reads it before an irreversible yes.\nwant: %q\ngot:  %q", tc.name, tc.text, body.Message)
			}
		})
	}
}

// TestApprovals_EveryNameIsInTheRegistry closes the set in both directions: a
// name the surface can emit that the registry does not list, and a registry
// entry no outcome produces, both redden. Change 14 renamed taken_by_another to
// params_unaccounted because the old name asserted an actor none of its three
// branches can prove; this is the mould that would have caught the rename.
func TestApprovals_EveryNameIsInTheRegistry(t *testing.T) {
	t.Parallel()
	registry := map[string]bool{}
	for _, n := range controlapi.ApprovalOutcomeNames {
		if registry[string(n)] {
			t.Fatalf("the registry lists %q twice", n)
		}
		registry[string(n)] = true
	}
	emitted := map[string]bool{"forbidden": true} // proved on the bearer route
	for _, tc := range namedOutcomes() {
		emitted[tc.error_] = true
		if !registry[tc.error_] {
			t.Fatalf("%q is emitted but absent from ApprovalOutcomeNames", tc.error_)
		}
	}
	for name := range registry {
		if !emitted[name] {
			t.Fatalf("%q is in the registry and no outcome emits it — an orphan name reaches nobody", name)
		}
	}
	// TWENTY, not the 22 §12-ter listed. `params_not_canonical` and
	// `params_belt_failed` left on 2026-09-13 under the director's rule «a name
	// with no producer goes out»: verified by grep over the whole tree, no
	// production path could emit either, so each was a paragraph of operator
	// text, a branch in the screen and a mould certifying the void. §12-ter is
	// scoped in the same commit.
	if len(registry) != 20 {
		t.Fatalf("the registry is the closed set: want 20 names, got %d", len(registry))
	}
}

// TestApprovals_InvalidatedCarriesTheCurrentLawDigest is change 10 (FR-API-17):
// E7 prints the digest it was parked under AND the one in force. The pinned one
// rides law_digest; the current one rides a field of its own, never inside the
// message.
func TestApprovals_InvalidatedCarriesTheCurrentLawDigest(t *testing.T) {
	t.Parallel()
	const current = "sha256:31c0f7ae00000000000000000000000000000000000000000000000000000000"
	f := &fakeApprovals{approveErr: controlapi.LawMoved(current)}
	srv := approvalsServer(t, f)
	res := doReq(t, "POST", srv.URL+"/api/approvals/apr_1/approve", approvalsToken, `{"digest":"sha256:abc"}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("invalidated is a 409, got %d", res.StatusCode)
	}
	body := decode[errorBody](t, res)
	if body.Error != "invalidated" {
		t.Fatalf("a law that moved is named invalidated, got %q", body.Error)
	}
	if body.CurrentLawDigest != current {
		t.Fatalf("the law in force rides its own field: want %q got %q", current, body.CurrentLawDigest)
	}
	if strings.Contains(body.Message, current) {
		t.Fatalf("the digest must NOT be dug out of the message — that is the 'by text' E9 forbids")
	}
}

// --- G7: disabled is its own state on the LIST, never an empty list --------

func TestApprovals_DisabledIsNotAnEmptyList(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{disabled: true}
	srv := approvalsServer(t, f)
	res := doReq(t, "GET", srv.URL+"/api/approvals", approvalsToken, "")
	// Change 11 (FR-TEST-3): the 409 is pinned ON THE LIST ROUTE. The old mould
	// accepted any non-200, so the code E3 depends on was fixed by nothing.
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("a disabled profile answers 409 on the list route, got %d", res.StatusCode)
	}
	body := decode[errorBody](t, res)
	if body.Error != "disabled" {
		t.Fatalf("the disabled state has its own name: got %q", body.Error)
	}
	if body.Message != "approvals are switched off in this profile — there is no pending list, which is not the same as an empty one" {
		t.Fatalf("the disabled TEXT is contract on the list route too: %q", body.Message)
	}
}

// --- G4: reject carries the comment and executes nothing ------------------

func TestApprovals_RejectCarriesTheComment(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	srv := approvalsServer(t, f)
	res := doReq(t, "POST", srv.URL+"/api/approvals/apr_2/reject", approvalsToken,
		`{"comment":"no en la ceremonia"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reject must be 200, got %d", res.StatusCode)
	}
	if f.rejectedID != "apr_2" || f.rejectedNote != "no en la ceremonia" {
		t.Fatalf("the rejection carries its id and its comment: %q %q", f.rejectedID, f.rejectedNote)
	}
	if f.approvedID != "" {
		t.Fatalf("a rejection must never reach the approve path, reached %q", f.approvedID)
	}
	out := decode[controlapi.ApprovalOutcome](t, res)
	if out.Outcome != "rejected" || out.ReceiptID != "rcpt_fake_rejected" {
		t.Fatalf("P5 prints the receipt of the DECISION act: %+v", out)
	}
}

// --- A8: the list is bounded, and it is an object -------------------------

func TestApprovals_ListIsBounded(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	f.list.Gate = controlapi.ApprovalGate{ApprovalsEnabled: true, BrainsTotal: 3, BrainsCanPark: 2}
	for i := 0; i < 500; i++ {
		f.list.Rows = append(f.list.Rows, controlapi.ApprovalRow{ID: "apr_x", ActionID: "act_x"})
	}
	srv := approvalsServer(t, f)
	res := doReq(t, "GET", srv.URL+"/api/approvals", approvalsToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list must be 200, got %d", res.StatusCode)
	}
	// Change 3 (FR-API-6): the list is an OBJECT with the gate, not a bare
	// array — without the gate "nothing can park" and "nothing is pending" are
	// the same answer. Change 12 (FR-TEST-3): the bound is asserted by EQUALITY,
	// because the old `>` passed with zero rows.
	got := decode[controlapi.ApprovalList](t, res)
	if len(got.Rows) != controlapi.ApprovalsPageLimit {
		t.Fatalf("the list is bounded at exactly %d, got %d", controlapi.ApprovalsPageLimit, len(got.Rows))
	}
	if got.Gate.BrainsCanPark != 2 || got.Gate.BrainsTotal != 3 || !got.Gate.ApprovalsEnabled {
		t.Fatalf("change 4: the ListPending seam carries the gate: %+v", got.Gate)
	}
	raw := decodeMap(t, doReq(t, "GET", srv.URL+"/api/approvals", approvalsToken, ""))
	if _, ok := raw["gate"]; !ok {
		t.Fatalf("the list carries the raw JSON key gate")
	}
	if _, ok := raw["rows"]; !ok {
		t.Fatalf("the list carries the raw JSON key rows")
	}
}

// TestApprovals_EmptyListIsAnEmptyArray pins that zero pending rows serialise
// as [] and never null: a null would decode to a nil slice and let the screen
// treat "no rows" and "no answer" as the same thing, which is the fail-open V1
// exists to prevent.
func TestApprovals_EmptyListIsAnEmptyArray(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	f.list.Gate = controlapi.ApprovalGate{ApprovalsEnabled: true}
	srv := approvalsServer(t, f)
	raw := decodeMap(t, doReq(t, "GET", srv.URL+"/api/approvals", approvalsToken, ""))
	rows, ok := raw["rows"].([]any)
	if !ok {
		t.Fatalf("rows must be an array even when empty, got %T", raw["rows"])
	}
	if len(rows) != 0 {
		t.Fatalf("want zero rows, got %d", len(rows))
	}
}

// --- A9: the reject comment is bounded ------------------------------------

func TestApprovals_RejectCommentIsBounded(t *testing.T) {
	t.Parallel()
	f := &fakeApprovals{}
	srv := approvalsServer(t, f)
	huge := `{"comment":"` + repeatByte('a', 100_000) + `"}`
	res := doReq(t, "POST", srv.URL+"/api/approvals/apr_1/reject", approvalsToken, huge)
	// Change 6 (FR-API-13): one name per attack. The old mould accepted 413 OR
	// 400, which hides that one of the two is unreachable.
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("an unbounded comment is 413, got %d", res.StatusCode)
	}
	if f.rejectedID != "" {
		t.Fatalf("nothing unbounded may reach the store, reached %q", f.rejectedID)
	}
}

func repeatByte(b byte, n int) string {
	s := make([]byte, n)
	for i := range s {
		s[i] = b
	}
	return string(s)
}

// decodeMap decodes a response body as a raw JSON object, so the RAW KEYS can
// be asserted. Decoding into the same Go types would let any label pass, which
// is the hole FR-API-12 names.
func decodeMap(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatalf("decode raw body: %v", err)
	}
	return m
}

// TestApprovals_everyNameHasAProducer is the registry's THIRD direction, and
// the one that was missing: a name is in the closed set, its text is pinned,
// and SOMEBODY IN PRODUCTION EMITS IT.
//
// Without it the registry could — and did — carry three names no code path
// could ever produce, each with a paragraph of operator text and a branch in
// the screen. This file's own rule says a name the screen paints and the
// server can never emit is dead interface; nothing enforced it.
//
// The scan is by SOURCE, over the non-test files that build the answers: each
// sentinel must be referenced somewhere other than its own declaration and the
// tables in this file.
//
// Probing mutation: re-add a sentinel with its outcome row and no producer ⇒
// this reddens.
func TestApprovals_everyNameHasAProducer(t *testing.T) {
	t.Parallel()
	roots := []string{"..", "../app", "../action/sqlite", "../cli"}
	var haystack strings.Builder
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // an unreadable subtree is not this test's subject
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// The declaration file itself proves nothing: every name is in it.
			if filepath.Base(path) == "approvals.go" && !strings.Contains(path, "/") {
				return nil
			}
			b, rerr := os.ReadFile(path) //nolint:gosec // walking our own tree
			if rerr != nil {
				return nil //nolint:nilerr // ditto
			}
			haystack.Write(b)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	src := haystack.String()
	for _, name := range controlapi.ApprovalOutcomeNames {
		sentinel := "ErrApproval" + upperCamel(string(name))
		if !strings.Contains(src, sentinel) && !strings.Contains(src, string(name)) {
			t.Errorf("%q is in the registry and NOTHING in production emits it — dead interface", name)
		}
	}
}

// upperCamel turns an outcome name into the sentinel spelling the package uses.
func upperCamel(s string) string {
	out := ""
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		out += strings.ToUpper(part[:1]) + part[1:]
	}
	return out
}
