// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator act behind every profile change made through the Control API
// (director's ruling, 2026-09-24).
//
// Until this file, NO change made from the window left a trace in the action
// ledger — not the four buttons of «¿Qué pasa hoy?», and not `POST /api/config`,
// the door the builder has written the profile through since Stage 14. Only the
// CLI's authority acts were recorded. An operator could turn approvals on, raise
// an effect ceiling or widen a network cage, and the book that exists to say who
// changed what said nothing.
//
// WHAT «ATOMIC» CAN MEAN HERE, stated before it is promised. A SQLite
// transaction and a process cutover cannot be atomic with each other: the
// `Start` that confirms the change runs on a different app object, takes part in
// no transaction of the ledger, and there is no second phase binding them. So
// the guarantee is the one the ruling itself names — «the act says what was
// ATTEMPTED and the RESULT, never what was wished» — in this order:
//
//  1. the act is SEALED. If sealing fails the supervisor is never asked, so
//     nothing can have changed. This half holds BY IMPOSSIBILITY.
//  2. the cutover is requested. `Supervisor.RequestReload` is asynchronous: it
//     queues the request and answers a handle whose state is `pending`.
//  3. the act is CLOSED with the outcome by the PROCESS, the moment the
//     supervisor stores a terminal state (its state observer), through
//     whichever handle of the ledger is alive — or a transient one when the app
//     that sealed the act is already gone, which is what a rollback looks like.
//     The status door the screen polls answers that close with the receipt, and
//     performs it when a poll arrives first — with the supervisor's outcome,
//     never the caller's. Both halves live in internal/app's
//     ConfigActRegistry, and the reason they exist is a capture
//     (evidence/v0.16.2/probe-real-cutover.txt): the app that seals an act is
//     shut down BEFORE the state turns terminal, so a close that waited for a
//     poll of that app never came, and the next app's boot recovery took the
//     open act for an orphan of a previous life.
//
// Between 2 and 3 the process can die with the act still AUTHORIZED. That is
// the one window this design does not close: the next boot's recovery then
// closes it OUTCOME_UNKNOWN with its receipt — an honest terminal, since nobody
// alive knows whether the change applied. What cannot happen, and has its
// mould, is a change applied with no act at all.
//
// A profile with NO ledger refuses every change by name (`no_ledger`) except
// the one that founds the ledger — `enable-storage` — which seals its own
// founding act in the book it creates, before the cutover that makes the
// profile use it (the director's decision of 2026-09-24).

package controlapi

import (
	"context"
	"errors"
)

// ErrNoLedger is what a recorder that EXISTS but has no ledger answers from
// BeginConfigAct: the profile has no action store. The doors turn it into the
// `no_ledger` refusal, exactly as they do for a nil recorder. Such a recorder is
// mounted on purpose — its status door can still close a founding act — so the
// two shapes must mean the same thing to the operator.
var ErrNoLedger = errors.New("controlapi: this profile has no action ledger")

// ErrLedgerExists is what CreateLedger answers when a file already sits at the
// ledger's path and this process did not create it. The default path is the
// desktop's own book on this machine; a profile that founds «its» ledger there
// would be sealing acts in another profile's book. The operator adopts it, or
// names another path, by hand in the profile.
var ErrLedgerExists = errors.New("controlapi: a ledger file already exists at the path this profile would use")

// ErrLedgerNotCreated is what CreateLedger answers when the file could not be
// created or opened at all — a directory that refuses writes, an unusable
// path. Nothing exists afterwards that did not exist before.
var ErrLedgerNotCreated = errors.New("controlapi: the ledger could not be created")

// LedgerStandingUnreadable is the standing a recorder answers when the store's
// judgement is a VERDICT on the book (the store's own ErrLedgerUnreadable or
// ErrLedgerMarkMalformed): the owner field then carries the cause. It is not
// «no ledger» and not «no profile», and its remedy is to replace the book.
const LedgerStandingUnreadable = "unreadable"

// LedgerStandingEnvironment is the standing a recorder answers when the
// storage AROUND the ledger failed (permissions, disk, input/output): the
// owner field then carries the cause. It says nothing against the ledger's
// contents.
const LedgerStandingEnvironment = "environment"

// LedgerStandingUnavailable is the standing a recorder answers when the ledger
// could not be judged NOW for any other reason — SQLite answered busy (another
// connection held the file), or the failure has no class: the owner field
// then carries the cause. It says nothing against the book either.
const LedgerStandingUnavailable = "unavailable"

// LedgerPathProvider is the optional capability of a recorder that knows the
// file its ledger lives in; the read door serves that path with the standing.
// A recorder without it serves no path, and nothing stands in for it.
type LedgerPathProvider interface {
	LedgerPath() string
}

// ErrLedgerForeign is what a recorder answers when the ledger was founded or
// adopted by ANOTHER profile (the durable mark, director's order 2026-09-24):
// no new act may be recorded in it until this profile adopts it with an
// explicit act. The doors turn it into `ledger_foreign_profile`.
var ErrLedgerForeign = errors.New("controlapi: this ledger belongs to another profile")

// ErrLedgerUnreadable is what the approvals adapter answers when the store
// judged the ledger unreadable or its profile mark malformed — a verdict on
// the book, never a busy ledger or a failing environment. The approvals doors
// name it (503 `ledger_unreadable`) rather than answering 500. The write
// doors do not receive it: their recorder hands back the store's own error,
// wrapped.
var ErrLedgerUnreadable = errors.New("controlapi: this ledger's identity cannot be read")

// ConfigAct is the operator act that seals one profile change.
type ConfigAct struct {
	// ActionID is the act's id in the ledger.
	ActionID string `json:"action_id,omitempty"`
	// ReceiptID is the receipt that seals the act's OUTCOME, verifiable with
	// `korvun receipt verify`.
	//
	// It is EMPTY until the act is closed, and that is the ledger's shape, not an
	// omission: a receipt seals a terminal state, and an act sealed over an
	// attempt has no outcome to seal yet. The store is explicit about it —
	// `RecordAttemptAuthenticated` mints a receipt for every state EXCEPT
	// `StateAuthorized`, with the comment «terminal identified outcomes birth
	// their receipt here too». So a change still in flight answers with an action
	// id and no receipt, and the receipt arrives with the outcome, through the
	// status door.
	//
	// It can also be empty on a CLOSED act, when the re-read failed: that is a
	// housekeeping failure and never a reason to refuse a change already under
	// way.
	ReceiptID string `json:"receipt_id,omitempty"`
}

// ActRecorder is the seam to the action ledger, the same store the approvals
// adapter holds. It is an interface for the same reason `Reloader` is: this
// package never imports the store, so the coupling stays one-directional and the
// handlers stay testable against a ledger that lies.
type ActRecorder interface {
	// BeginConfigAct seals an operator act for verb over params.
	//
	// The caller MUST NOT attempt the change when this returns an error. That is
	// the «no act, no change» half, and it is provable by impossibility: a
	// supervisor that was never called cannot have changed anything by any path.
	BeginConfigAct(ctx context.Context, verb string, params []byte) (ConfigAct, error)

	// BindReload remembers which act a reload handle belongs to, so the status
	// door can close the act when the outcome is finally known.
	BindReload(actionID, handle string)

	// SettleAct closes an act with the outcome, ONCE. A second call for the same
	// act does nothing — the status door is polled, and an act cannot be closed
	// twice.
	//
	// It returns the act with its RECEIPT, which only exists once the act is
	// closed, and NO error. A close that fails happens AFTER the change has
	// committed, and returning its error would make the door report a refusal
	// over a cutover that is already running — the class cured for four store
	// writers on 2026-09-22, inherited by the CLI's own act, whose close goes to
	// a note and never to the exit code.
	SettleAct(ctx context.Context, actionID string, applied bool, detail string) ConfigAct

	// SettleReload closes the act bound to a handle, once, and returns it with
	// its receipt. A handle nobody bound is a no-op answering the zero act: the
	// status door serves handles this surface never created.
	SettleReload(ctx context.Context, handle string, applied bool, detail string) ConfigAct

	// CreateLedger founds the action store at the profile's default path,
	// seals the founding act — `config.<door>` over `{"door","path"}` — in the
	// book it just created, and returns that act with the path the profile
	// must now name. It only CREATES: a file already there that this process
	// did not found is ErrLedgerExists (with the path, so the operator can
	// look); a file that cannot be created is ErrLedgerNotCreated; a file
	// created whose founding act could not be sealed is any other error, and
	// the file stays. A recorder that already has a ledger refuses.
	CreateLedger(ctx context.Context, door string) (ConfigAct, string, error)

	// AdoptLedger records the adoption act — the ONE write a foreign ledger
	// admits — and closes it with the mark naming this profile, in one store
	// transaction; it answers the CLOSED act with its receipt. A recorder with
	// no ledger answers ErrNoLedger.
	AdoptLedger(ctx context.Context) (ConfigAct, error)

	// LedgerStanding says what the ledger is to this profile — "ok",
	// "legacy_unfounded" or "ledger_foreign_profile" — and who owns it (the
	// digest the last mark names, "" for a ledger with no mark). When the
	// ledger could not be judged it answers "unreadable", "environment" or
	// "unavailable", with the cause in place of the owner. A recorder with
	// no ledger answers two empty strings.
	LedgerStanding(ctx context.Context) (standing, owner string)
}

// actVerb names the act of one door, as the ledger records it. The prefix keeps
// a profile change distinguishable from an authority act at a glance in the book.
func actVerb(door string) string { return "config." + door }
