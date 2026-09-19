// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The four approval endpoints of the v0.15.0 train "approvals in the app"
// (docs/superpowers/specs/2026-09-08-approvals-screen-ux.md, v36).
//
// The screen renders BY NAME, never by the English text of the body (E9), so
// every refusal carries a machine-readable `error` from the closed registry in
// this file plus a `message` an operator reads. The two travel together and
// the name is the contract; the text is contract too, because it is what an
// operator reads before an irreversible yes.
//
// This file owns the SURFACE. What the store can and cannot distinguish lives
// behind the Approvals seam; every sentinel below names one distinction the
// adapter promises to make, and the adapter's own moulds against a real SQLite
// prove it. Nothing here re-derives a digest or reads a row.
package controlapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ApprovalsPageLimit bounds one page of the pending list. The list is a page,
// never "everything": an unbounded list is a denial of service against the
// window that renders it, and FR-UI-12's tail-collision detection is declared
// to reach only the page it is computed over.
const ApprovalsPageLimit = 200

// maxRejectBodyBytes bounds the reject comment. The operator's note is a
// sentence, not a payload; anything past this is refused before it reaches the
// store (FR-API-13).
const maxRejectBodyBytes = 4 << 10 // 4 KiB

// maxApproveBodyBytes bounds the approve body, which carries exactly one key.
const maxApproveBodyBytes = 1 << 10 // 1 KiB

// OutcomeName is the machine-readable half of every approval answer. It is a
// defined type so the registry below can be written from the constants rather
// than from loose strings — and the godoc says what the type does NOT do: Go
// assigns an untyped constant to a defined string type, so this is an aid to
// the reader, never a barrier against a literal. FR-TEST-6 owns the barrier.
type OutcomeName string

// The closed registry. Every name the surface can emit is here, and
// §12-ter of the spec carries the literal each one renders as. A name that
// reaches the screen without a literal is the growth defect FR-TEST-6 exists
// to catch.
const (
	// Read and decide, shared.
	OutcomeAlreadyDecided OutcomeName = "already_decided"
	OutcomeNotFound       OutcomeName = "not_found"
	OutcomeUnavailable    OutcomeName = "unavailable"
	OutcomeForbidden      OutcomeName = "forbidden"
	OutcomeDisabled       OutcomeName = "disabled"
	OutcomeExpired        OutcomeName = "expired"

	// A peer that did not reach the core over loopback (v0.15.1 block B,
	// P2-7, director 2026-09-19: always refused, no opt-in).
	OutcomeLoopbackOnly OutcomeName = "loopback_only"

	// The two belts that refuse before anything is decided.
	OutcomeDigestMismatch       OutcomeName = "digest_mismatch"
	OutcomeParamsDigestMismatch OutcomeName = "params_digest_mismatch"

	// Permanent refusals on the read path.
	OutcomeInvalidated     OutcomeName = "invalidated"
	OutcomeEvidenceCorrupt OutcomeName = "evidence_corrupt"

	// The law's brain left the profile: reject still works, approve cannot.
	OutcomeBrainGone OutcomeName = "brain_gone"

	// The execution outcomes, after a decision that is already committed.
	OutcomeNotStartedParamsHeld OutcomeName = "not_started_params_held"
	OutcomeNotStartedParamsGone OutcomeName = "not_started_params_gone"
	OutcomeParamsUnaccounted    OutcomeName = "params_unaccounted"
	OutcomeParamsUnreadable     OutcomeName = "params_unreadable"
	OutcomeDecidedEvidenceBad   OutcomeName = "decided_evidence_corrupt"
	OutcomeAlreadyClosed        OutcomeName = "already_closed"
	OutcomeNotDecided           OutcomeName = "not_decided"
	OutcomeUnknownOutcome       OutcomeName = "unknown_outcome"
	OutcomeCloseFailed          OutcomeName = "close_failed"
	OutcomeReceiptUnreadable    OutcomeName = "receipt_unreadable"
)

// ApprovalOutcomeNames is every name this surface can emit, written from the
// constants. FR-TEST-6 crosses it against the anchors of §12-ter in BOTH
// directions: a name without a literal and an orphan anchor both redden.
var ApprovalOutcomeNames = []OutcomeName{
	OutcomeAlreadyDecided, OutcomeNotFound, OutcomeUnavailable, OutcomeForbidden,
	OutcomeDisabled, OutcomeExpired, OutcomeLoopbackOnly,
	OutcomeDigestMismatch, OutcomeParamsDigestMismatch,
	OutcomeInvalidated, OutcomeEvidenceCorrupt, OutcomeBrainGone,
	OutcomeNotStartedParamsHeld, OutcomeNotStartedParamsGone, OutcomeParamsUnaccounted,
	OutcomeParamsUnreadable, OutcomeDecidedEvidenceBad, OutcomeAlreadyClosed,
	OutcomeNotDecided, OutcomeUnknownOutcome, OutcomeCloseFailed, OutcomeReceiptUnreadable,
}

// The sentinels the adapter returns. Each one is a distinction the store can
// actually make; where the store cannot distinguish, there is deliberately no
// sentinel — FR-API-21 and FR-API-25 say which ambiguities stay ambiguous and
// why the literals never name an actor the evidence cannot prove.
var (
	ErrApprovalsDisabled = errors.New("approvals: disabled in this profile")
	// ErrApprovalLoopbackOnly: the request reached the core from a peer that
	// is not loopback. Refused before the seam, always.
	ErrApprovalLoopbackOnly   = errors.New("approvals: only a loopback peer may use this surface")
	ErrApprovalsUnavailable   = errors.New("approvals: store unavailable")
	ErrApprovalNotFound       = errors.New("approvals: no such request")
	ErrApprovalAlreadyDecided = errors.New("approvals: already decided")
	ErrApprovalExpired        = errors.New("approvals: expired")
	ErrApprovalDigestMismatch = errors.New("approvals: digest mismatch")
	ErrApprovalForbidden      = errors.New("approvals: the window could not authenticate")

	ErrApprovalParamsDigestMismatch = errors.New("approvals: stored parameters do not re-derive the digest")
	ErrApprovalInvalidated          = errors.New("approvals: the pinned law no longer holds")
	ErrApprovalEvidenceCorrupt      = errors.New("approvals: stored evidence does not verify")
	ErrApprovalBrainGone            = errors.New("approvals: the requesting brain is no longer in the profile")

	ErrApprovalNotStartedParamsHeld = errors.New("approvals: this execution did not start; the row still holds its parameters")
	ErrApprovalNotStartedParamsGone = errors.New("approvals: this execution did not start; the row was born without parameters")
	ErrApprovalParamsUnaccounted    = errors.New("approvals: the parameters are no longer where they were")
	ErrApprovalParamsUnreadable     = errors.New("approvals: the store could not be re-read")
	ErrApprovalDecidedEvidenceBad   = errors.New("approvals: the sealed decision's evidence no longer verifies")
	ErrApprovalAlreadyClosed        = errors.New("approvals: this request is not awaiting execution")
	ErrApprovalNotDecided           = errors.New("approvals: this request is still awaiting a decision")
	ErrApprovalUnknownOutcome       = errors.New("approvals: the tool ran and its outcome is unknown")
	ErrApprovalCloseFailed          = errors.New("approvals: the ledger could not be closed")
	// ErrApprovalReceiptUnreadable is a decision that IS sealed and whose
	// receipt identifier could not be read back. It exists because
	// close_failed says «whether the effect happened is unknown», and on the
	// REJECT path there is no effect to be unknown about: no execution is ever
	// attempted. Reusing that name swapped one fabricated sentence for another.
	ErrApprovalReceiptUnreadable = errors.New("approvals: the sealed decision's receipt could not be read back")
)

// lawMovedError carries the law digest in force alongside the invalidated
// sentinel. FR-API-17 needs BOTH digests in E7 — the one the request was parked
// under and the one that rules now — and the current one travels in a field of
// its own. Digging it out of the message would be the "by text" E9 forbids, and
// the message has a byte-for-byte contract that a value spliced into it would
// break on the first rewording.
type lawMovedError struct{ current string }

func (e lawMovedError) Error() string { return ErrApprovalInvalidated.Error() }

// Unwrap makes errors.Is(err, ErrApprovalInvalidated) true, so the name and the
// text come from the one binding in the table rather than a second copy.
func (e lawMovedError) Unwrap() error { return ErrApprovalInvalidated }

// CurrentLawDigest exposes the law in force to a caller outside this package.
// Without it the value travelled in a field nobody but writeApprovalError
// could read, which is a field in name only — and FR-API-17's whole point is
// that the current law must NOT be dug out of a sentence.
func (e lawMovedError) CurrentLawDigest() string { return e.current }

// LawMoved wraps the invalidated sentinel with the law digest now in force.
func LawMoved(currentLawDigest string) error { return lawMovedError{current: currentLawDigest} }

// outcome pairs a sentinel with the status, name and text the surface answers.
// The text is contract: R15 rewrote two operator-facing notes mid-train and the
// suite stayed green because nothing watched the words.
type outcome struct {
	status int
	name   OutcomeName
	text   string
}

// outcomes maps every sentinel to its answer. A sentinel absent from this map
// is an internal error and answers 500 without a name — never a name invented
// at the boundary.
var outcomes = map[error]outcome{
	// NOT «the first decision stands»: the store's consume rule answers this
	// name for ANY status that is not PENDING, and two of those — EXPIRED,
	// written by the clock, and CANCELLED, a withdrawal before any decision —
	// have no first decision to stand. Asserting an actor that may not exist is
	// the same defect the screen's decided_evidence_corrupt literal had.
	// Nor «nothing ran twice» (v0.15.1 block B): this refusal says nothing
	// about executions, and a restore of the parameters after the claim commits
	// can make one request run twice (block A's canto, §5, open for v0.15.2).
	ErrApprovalAlreadyDecided: {http.StatusConflict, OutcomeAlreadyDecided,
		"this request is no longer open to a decision — the ledger says what closed it"},
	ErrApprovalExpired: {http.StatusConflict, OutcomeExpired,
		"this request expired before the decision touched it — it never executes"},
	ErrApprovalDigestMismatch: {http.StatusConflict, OutcomeDigestMismatch,
		"the stored request no longer re-derives the digest you were shown — nothing was executed; read it again"},
	ErrApprovalNotFound: {http.StatusNotFound, OutcomeNotFound,
		"no approval request with that id"},
	ErrApprovalsUnavailable: {http.StatusServiceUnavailable, OutcomeUnavailable,
		"the store could not be read at this instant — this is transient and says nothing about the evidence"},
	ErrApprovalsDisabled: {http.StatusConflict, OutcomeDisabled,
		"approvals are switched off in this profile — there is no pending list, which is not the same as an empty one"},
	ErrApprovalForbidden: {http.StatusUnauthorized, OutcomeForbidden,
		"the window could not authenticate against the core — no decision has left it"},
	ErrApprovalLoopbackOnly: {http.StatusForbidden, OutcomeLoopbackOnly,
		"this decision can only be made from the machine that runs Korvun — the request came from another origin and was refused without touching anything"},

	ErrApprovalParamsDigestMismatch: {http.StatusConflict, OutcomeParamsDigestMismatch,
		"the stored parameters do not re-derive this request's digest — this is permanent and nothing was executed"},
	ErrApprovalInvalidated: {http.StatusConflict, OutcomeInvalidated,
		"the law this request was parked under no longer holds — this is not transient; rejecting it still works"},
	ErrApprovalEvidenceCorrupt: {http.StatusConflict, OutcomeEvidenceCorrupt,
		"the stored evidence for this request does not verify — this is permanent and no decision is offered"},
	ErrApprovalBrainGone: {http.StatusConflict, OutcomeBrainGone,
		"the brain that asked for this action is no longer in the profile — approving needs its law, rejecting does not"},

	ErrApprovalNotStartedParamsHeld: {http.StatusConflict, OutcomeNotStartedParamsHeld,
		"the decision is recorded and THIS execution did not start; the row still holds its parameters, so no automatic pass has closed it — about other executors this answer says nothing"},
	ErrApprovalNotStartedParamsGone: {http.StatusConflict, OutcomeNotStartedParamsGone,
		"the decision is recorded and THIS execution did not start; the row was born without parameters, so no execution could have started"},
	ErrApprovalParamsUnaccounted: {http.StatusConflict, OutcomeParamsUnaccounted,
		"this execution did not start, and whether the request ran cannot be asserted: on re-reading, its parameters were no longer where they were, or the row was gone"},
	ErrApprovalParamsUnreadable: {http.StatusConflict, OutcomeParamsUnreadable,
		"this execution did not start and the store could not be read — this says it does not know, not that anything was taken"},
	ErrApprovalDecidedEvidenceBad: {http.StatusConflict, OutcomeDecidedEvidenceBad,
		"the decision is recorded and sealed with its receipt, and THIS execution did not start; what no longer verifies is the stored evidence, and that is permanent"},
	ErrApprovalAlreadyClosed: {http.StatusConflict, OutcomeAlreadyClosed,
		"this request is not awaiting execution — THIS execution did nothing"},
	ErrApprovalNotDecided: {http.StatusConflict, OutcomeNotDecided,
		"the store says this request is still awaiting a decision, so THIS execution did nothing"},
	ErrApprovalUnknownOutcome: {http.StatusConflict, OutcomeUnknownOutcome,
		"the decision left this window and whether the effect happened is unknown"},
	ErrApprovalCloseFailed: {http.StatusConflict, OutcomeCloseFailed,
		"the decision left this window, whether the effect happened is unknown, and THIS execution could not close the ledger"},
	ErrApprovalReceiptUnreadable: {http.StatusConflict, OutcomeReceiptUnreadable,
		"the decision is sealed and recorded; only its receipt identifier could not be read back, and no execution was attempted"},
}

// ApprovalRow is one line of the pending list. The digest lives HERE, once, and
// ApprovalDetail inherits it by embedding: a digest that appeared twice could
// disagree with itself.
type ApprovalRow struct {
	ID          string `json:"id"`
	ActionID    string `json:"action_id"`
	Operation   string `json:"operation"`
	EffectClass string `json:"effect_class"`
	ExpiresAt   string `json:"expires_at"`
	Digest      string `json:"digest"`
	Origin      string `json:"origin"`
}

// ApprovalDetail is the document. It carries no `law_version` (a constant, so
// showing it informs nobody), no `status` (under FR-API-18's precedence it can
// only ever be PENDING here), and no grant/cost fields (always the same three
// constants in a parked request).
type ApprovalDetail struct {
	ApprovalRow
	Purpose         string `json:"purpose"`
	PrincipalID     string `json:"principal_id"`
	Reversibility   string `json:"reversibility"`
	ToolCage        string `json:"tool_cage"`
	RequiredRule    string `json:"required_rule"`
	LawDigest       string `json:"law_digest"`
	Parameters      string `json:"parameters"`
	ParametersState string `json:"parameters_state"`
	// BrainGone rides the 200 rather than an error body: an error body carries
	// no document, and E9 sends every error name to a terminal state, which
	// would leave the request unclosable. Reject does not consult the law, so
	// the screen degrades to reject-only and still finishes the request.
	BrainGone bool `json:"brain_gone,omitempty"`
}

// ApprovalGate is what the list says about the profile itself. Without it "no
// brain can park" and "nothing is pending" are the same empty answer, which is
// the hole in the guarantee E4 exists to name.
type ApprovalGate struct {
	ApprovalsEnabled bool `json:"approvals_enabled"`
	BrainsTotal      int  `json:"brains_total"`
	BrainsCanPark    int  `json:"brains_can_park"`
	// RowsSkipped counts the parked requests this page could NOT serve
	// whole — a row whose stored evidence will not scan. It travels so the
	// screen can say how many were left out instead of letting the operator
	// lose them in silence: a list that quietly returns fewer rows than the
	// store holds is the failure this field exists to make impossible.
	RowsSkipped int `json:"rows_skipped"`
}

// ApprovalList is the list response: the gate and the page, never a bare array.
type ApprovalList struct {
	Gate ApprovalGate  `json:"gate"`
	Rows []ApprovalRow `json:"rows"`
}

// ApprovalOutcome is what a committed decision reports back. The receipt id is
// re-read by the adapter: neither DecideApprovalUnderLaw nor FinishWithResult
// returns it.
type ApprovalOutcome struct {
	Outcome   string `json:"outcome"`
	Digest    string `json:"digest,omitempty"`
	Result    string `json:"result,omitempty"`
	ReceiptID string `json:"receipt_id"`
}

// Approvals is the narrow seam this surface needs. Everything hard — the
// atomic claim, the busy class, the belts, the single law resolution — lives
// behind it and is proved by the adapter's own moulds against a real store.
type Approvals interface {
	ListPending(ctx context.Context) (ApprovalList, error)
	Detail(ctx context.Context, id string) (ApprovalDetail, error)
	Approve(ctx context.Context, id, digest string) (ApprovalOutcome, error)
	Reject(ctx context.Context, id, comment string) (ApprovalOutcome, error)
}

// RegisterApprovals mounts the four routes behind the bearer.
func RegisterApprovals(m Mounter, token string, a Approvals) {
	auth := approvalsAuth(token)
	m.Handle("GET /api/approvals", loopbackOnlyApprovals(auth(listApprovalsHandler(a))))
	m.Handle("GET /api/approvals/{id}", loopbackOnlyApprovals(auth(approvalDetailHandler(a))))
	m.Handle("POST /api/approvals/{id}/approve", loopbackOnlyApprovals(auth(approveHandler(a))))
	m.Handle("POST /api/approvals/{id}/reject", loopbackOnlyApprovals(auth(rejectHandler(a))))
}

// approvalsAuth is the same gate as bearerAuth with this surface's 401 body.
//
// FILED, 2026-09-12, to be closed after the v0.15.0 tag: THIS IS THE SECOND
// COPY of a security check, and two copies are two places to hide. The other
// lives in mutation.go as bearerAuth and guards the console and mutation
// surfaces. A change to one that is not made to the other is a divergence
// nothing in the tree detects today. The agreed cure is one helper
// parameterised by the error it writes, leaving its current callers untouched;
// until then, anyone editing either copy edits both.
// The shared helper answers {"error":"unauthorized"}, a name the approvals
// registry does not contain; the screen renders BY NAME, so that body would
// land in "a response this screen does not recognise" instead of the forbidden
// literal with its [Ir a Inicio]. Widening the shared helper would have changed
// the console and mutation contracts, which this train does not touch, so the
// check is repeated here rather than parameterised — and it is repeated whole:
// the SHA-256 of both sides so the comparison is over fixed-length values (a
// raw variable-length compare leaks length through timing), the constant-time
// compare, and the refusal of an empty configured token, without which
// sha256("") matches on both sides and an empty presented token walks in.
func approvalsAuth(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				writeApprovalError(w, ErrApprovalForbidden)
				return
			}
			got := sha256.Sum256([]byte(bearerToken(r.Header.Get("Authorization"))))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				writeApprovalError(w, ErrApprovalForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeApprovalError answers with the name and text bound to err. An error with
// no binding is a surface defect, not an outcome: it answers 500 and emits no
// name, because inventing one here is exactly the growth FR-TEST-6 forbids.
func writeApprovalError(w http.ResponseWriter, err error) {
	for sentinel, o := range outcomes {
		if !errors.Is(err, sentinel) {
			continue
		}
		body := map[string]string{"error": string(o.name), "message": o.text}
		var moved lawMovedError
		if errors.As(err, &moved) && moved.current != "" {
			body["current_law_digest"] = moved.current
		}
		writeJSONStatus(w, o.status, body)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func listApprovalsHandler(a Approvals) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list, err := a.ListPending(r.Context())
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		// The page is bounded HERE as well as in the adapter: the bound is a
		// property of the surface, so a future second adapter cannot widen it.
		if len(list.Rows) > ApprovalsPageLimit {
			list.Rows = list.Rows[:ApprovalsPageLimit]
		}
		if list.Rows == nil {
			list.Rows = []ApprovalRow{}
		}
		writeJSON(w, list)
	})
}

func approvalDetailHandler(a Approvals) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validApprovalIDOrNotFound(w, r.PathValue("id")) {
			return
		}
		detail, err := a.Detail(r.Context(), r.PathValue("id"))
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, detail)
	})
}

func approveHandler(a Approvals) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validApprovalIDOrNotFound(w, r.PathValue("id")) {
			return
		}
		var body struct {
			Digest string `json:"digest"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxApproveBodyBytes))
		if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.Digest) == "" {
			// G2 in one line: without the digest the operator saw, there is
			// nothing to compare, so nothing may reach the store.
			writeError(w, http.StatusBadRequest, "body must be JSON with a non-empty digest field")
			return
		}
		out, err := a.Approve(r.Context(), r.PathValue("id"), body.Digest)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, out)
	})
}

func rejectHandler(a Approvals) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validApprovalIDOrNotFound(w, r.PathValue("id")) {
			return
		}
		var body struct {
			Comment string `json:"comment"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRejectBodyBytes))
		if err := dec.Decode(&body); err != nil {
			// One name per attack: an oversized comment is 413, never 400.
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "the reject comment is bounded")
				return
			}
			writeError(w, http.StatusBadRequest, "body must be JSON with an optional comment field")
			return
		}
		out, err := a.Reject(r.Context(), r.PathValue("id"), body.Comment)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, out)
	})
}
