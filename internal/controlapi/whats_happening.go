// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// «¿Qué pasa hoy?» — the screen's contract, its read door and its four write
// doors.
//
// The seventh law (CLAUDE.md, 2026-09-23): no capability is done if the operator
// cannot see it. This file is that law's wire. It carries the dimensions that
// decide what happens to an irreversible action — extracted from the code, not
// from anyone's memory — and refuses to let a new one exist in `policy` without
// a row here.
//
// The counts, as `TestContract_theHeaderArithmeticIsTrue` executes them:
// SEVENTEEN rows, SEVEN of them with a button, TEN shown as text with the
// profile key that changes them. SIX write doors serve those seven rows —
// `enable-approvals` answers two of them, because a profile whose approvals are
// off and a profile whose ceiling is reached without approvals available are
// the same fix seen from two sides. Any of those four numbers written by hand
// rots; the mould recomputes all four from `screenRows` and reddens when this
// sentence drifts.
//
// The seventh button is the adoption of a ledger founded by ANOTHER profile
// (the durable mark, director's order 2026-09-24): such a ledger refuses every
// change by name until this profile takes it with an explicit act.
//
// The sixth button is the store's own (the director's decision of 2026-09-24):
// a profile with no action store refuses every change by name, and the one
// door open on it founds the store — `enable-storage` — sealing its founding
// act in the book it creates before the cutover that makes the profile use it.
//
// The text-only rows are the conservative half of the law: an operator who
// cannot press anything must still be told where to go, never left at a dead
// end.
//
// The write doors POST a config the supervisor then builds, starts and — ONLY
// THEN — persists (ADR-0027 §c). Nothing here writes the profile: on a failed
// cutover the on-disk config is never touched, so there is no rollback to own.
//
// Every one of these doors records an OPERATOR ACT in the action ledger, and so
// does `POST /api/config`, the door the builder has written the profile through
// since Stage 14 and which recorded nothing until 2026-09-24. The seam and the
// exact boundary of what «atomic» can mean between a SQLite transaction and a
// process cutover are in act.go; the short form is that the act is sealed BEFORE
// the supervisor is asked, so a change with no act is impossible, and the act is
// closed with the outcome by the process the moment the supervisor stores it —
// the status door only reads that close.

package controlapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// ScreenRow is one dimension of the screen's contract.
type ScreenRow struct {
	// Rule is the name the CODE decides by — a policy.ToolRule, an effect
	// gate rule, or a condition of the park gate. It is the join key, so a
	// rename in the code breaks the build rather than the screen.
	Rule string `json:"rule"`
	// Label is what the operator reads.
	Label string `json:"label"`
	// ProfileKey is where it is changed. NEVER empty: a row with no button
	// and no key is a dead end.
	ProfileKey string `json:"profile_key"`
	// HasButton reports whether this release can change it from the screen.
	HasButton bool `json:"has_button"`
	// Door names the write door that serves this row, and is non-empty on
	// EXACTLY the rows that have a button. It is the join the mould checks:
	// a row claiming a button whose door does not exist is the class of lie
	// three rows of this table shipped with before the internal pass caught
	// them.
	Door string `json:"door,omitempty"`
	// Why, on a row with no button, says why not — so «no button» reads as a
	// decision rather than an omission. On a row WITH a button it may still
	// carry what the button does not reach.
	Why string `json:"why,omitempty"`
	// AfterApproval marks the dimensions that bite AFTER the operator has
	// approved. They belong in the GREEN state too, not only among the
	// blockers: the cages are the only ones that let a request park, let the
	// operator say yes, and refuse afterwards.
	AfterApproval bool `json:"after_approval,omitempty"`
}

// screenRows is the contract. Adding a rule to `policy` without adding it here
// reddens the contract mould, by name and by the source walk over the constants.
var screenRows = []ScreenRow{
	// The store's row has the one button a profile with no ledger can press.
	// Every other door refuses by name until this one has been pressed: a
	// change nobody can record is not applied, and this is the change that
	// makes recording possible.
	{Rule: "store", Label: "Almacén de acciones", ProfileKey: "storage.path",
		HasButton: true, Door: "enable-storage",
		Why: "crea el libro en la carpeta de usuario de Korvun (la misma que storage: {}) y deja en él su primer acto; sin almacén ninguna otra puerta de esta pantalla ni el builder aplican nada"},
	{Rule: "agent", Label: "Cerebro agente", ProfileKey: "brains[].agent",
		Why: "dar agencia a un cerebro es una decisión de diseño del perfil, no un botón"},
	{Rule: "approvals_disabled", Label: "Aprobaciones", ProfileKey: "approvals.enabled",
		HasButton: true, Door: "enable-approvals"},
	{Rule: "approvals_ttl", Label: "Caducidad de la petición", ProfileKey: "approvals.ttl",
		Why: "elegir una ventana de caducidad pide pensar, no un clic"},
	{Rule: "effect_ceiling", Label: "Techo de efecto", ProfileKey: "brains[].agent.effect_ceiling",
		HasButton: true, Door: "set-ceiling"},
	// `approval_unavailable` is NOT the ceiling's row wearing another name. The
	// executor returns it when the ceiling IS reached and the approvals
	// workflow is unavailable, so raising the ceiling changes nothing: the fix
	// is turning approvals on. This row pointed at `effect_ceiling` with the
	// ceiling's button until the internal pass read the executor.
	{Rule: "approval_unavailable", Label: "Aprobaciones no disponibles", ProfileKey: "approvals.enabled",
		HasButton: true, Door: "enable-approvals",
		Why: "el techo ya alcanza la acción; lo que falta es el circuito de aprobación"},
	// `deny` has NO button. `lift-shadow` lifts a SHADOW — a tool being
	// simulated — and refuses a `deny` by name: turning an explicit denial into
	// `allow` behind the shadow's confirmation text would be a different, wider
	// decision than the one the operator was asked to confirm.
	{Rule: "deny", Label: "Herramienta denegada", ProfileKey: "brains[].agent.governance[].mode",
		Why: "una denegación explícita se levanta mirando el perfil, no tras el texto de confirmación de la sombra"},
	// `not_granted` has no governance entry to edit: there is nothing for
	// `lift-shadow` to find, and inventing one would be granting a tool the
	// profile never granted.
	{Rule: "not_granted", Label: "Herramienta no concedida", ProfileKey: "brains[].agent.governance[].mode",
		Why: "no hay entrada que levantar: conceder una herramienta que el perfil no concede es una decisión de diseño, no un clic"},
	{Rule: "tool_shadowed", Label: "Herramienta en sombra", ProfileKey: "brains[].agent.governance[].mode",
		HasButton: true, Door: "lift-shadow"},
	{Rule: "channel", Label: "Restricción por canal", ProfileKey: "brains[].agent.governance[].channels",
		Why: "quitar una restricción de canal amplía por dónde entra la herramienta; se hace mirando el perfil entero"},
	// The cage row keeps ONE join key because the code decides by one rule,
	// and says in place what its button does not reach: `read_file.root` is a
	// filesystem root, not a host list, and no door widens it.
	{Rule: "cage", Label: "Jaula de la herramienta", ProfileKey: "brains[].agent.<tool>.allow_hosts · .root",
		HasButton: true, Door: "allow-host", AfterApproval: true,
		Why: "«Añadir host» alcanza las allow-list de webhook_call y http_fetch; read_file.root se cambia en el perfil"},
	{Rule: "sensitive_locality", Label: "Sensibilidad frente a localidad del modelo", ProfileKey: "brains[].sensitivity × models[].locality",
		Why: "no es una clave: se deriva de dos, y cambiar cualquiera mueve mucho más que esta herramienta"},
	{Rule: "private_network_shield", Label: "Escudo de red privada", ProfileKey: "se deriva de brains[].sensitivity y del atributo network de la herramienta",
		Why: "NO tiene clave propia: se arma solo cuando el cerebro es private y la herramienta es de red", AfterApproval: true},
	{Rule: "effect_undeclared", Label: "Operación sin clase declarada", ProfileKey: "brains[].agent.tools",
		Why: "una operación sin descriptor se deniega por construcción; declararla es trabajo de código, no de perfil"},
	{Rule: "prepare_unavailable", Label: "Operación que exige preparación", ProfileKey: "brains[].agent.tools",
		Why: "ídem: la preparación la declara la herramienta, no el perfil"},
	{Rule: "require_approval", Label: "Modo estricto, intención y grant", ProfileKey: "authority.mode · authority.activation_digest",
		Why: "el modo estricto cambia el arranque entero del perfil; no se enciende desde una pantalla"},
	// The ledger's owner. A book founded or adopted by another profile refuses
	// every change until this one adopts it — an explicit act with its receipt,
	// behind a confirmation, because taking a book is a consent.
	{Rule: "ledger_foreign_profile", Label: "Libro de otro perfil", ProfileKey: "storage.path",
		HasButton: true, Door: "adopt-ledger",
		Why: "el libro lo fundó o adoptó otro perfil; hasta que este lo adopte, ninguna puerta aplica nada en él"},
}

// ScreenRows returns the contract.
func ScreenRows() []ScreenRow { return append([]ScreenRow(nil), screenRows...) }

// screenRowForRule finds the row a code rule maps to.
//
// It is UNEXPORTED, and so are `screenRowForCondition` and `writeDoorNames`
// below. Their only reader is the contract mould, which now lives in this
// package — the official pass counted them as «doors only tests reach», a class
// this repo has already had to close twice. A helper the contract needs is not a
// public surface.
func screenRowForRule(rule string) (ScreenRow, bool) {
	for _, r := range screenRows {
		if r.Rule == rule {
			return r, true
		}
	}
	return ScreenRow{}, false
}

// conditionRows joins the park gate's conditions to the screen's rows.
//
// The two vocabularies are NOT the same words, and pretending they were would
// be the quiet kind of wrong: the gate says `ceiling` where the code's rule is
// `effect_ceiling`, `governance_denies` where the rule is `deny`, and `tool` for
// a state no single rule names. A screen that joined them by string equality
// would silently drop three of the six blockers — the operator would be told
// «something blocks this brain» with nothing under it.
//
// `TestContract_everyParkConditionHasAScreenRow` walks the CONSTANTS and
// reddens if a seventh condition arrives without an entry here.
var conditionRows = map[ParkCondition]string{
	ParkNeedsStore:        "store",
	ParkNeedsAgent:        "agent",
	ParkNeedsCeiling:      "effect_ceiling",
	ParkNeedsParkableTool: "effect_undeclared",
	ParkGovernanceDenies:  "deny",
	ParkToolShadowed:      "tool_shadowed",
}

// screenRowForCondition finds the row that explains a park-gate condition.
func screenRowForCondition(c ParkCondition) (ScreenRow, bool) {
	rule, ok := conditionRows[c]
	if !ok {
		return ScreenRow{}, false
	}
	return screenRowForRule(rule)
}

// WhatsHappeningOutcome is what a button's POST answers.
//
// Applied and ProfileUnchanged are SEPARATE because one state needs both
// halves: `StatePersistFailed` means the new app IS serving and the disk could
// NOT be updated. «Applied» alone hides that a restart reverts it; «not
// applied» alone is the opposite lie.
type WhatsHappeningOutcome struct {
	Outcome string `json:"outcome"`
	// Applied reports whether the running Korvun changed.
	Applied bool `json:"applied"`
	// ProfileUnchanged reports whether the file on disk is as it was.
	ProfileUnchanged bool `json:"profile_unchanged"`
	// Handle is the reload the screen polls while the cutover runs.
	Handle string `json:"handle,omitempty"`
	// Detail is operator text, never a parsed value.
	Detail string `json:"detail,omitempty"`
	// Act is the operator act this change was recorded as. Always present on a
	// change that was ATTEMPTED; absent on one refused before the act was
	// sealed, which is the difference the operator needs to see.
	Act ConfigAct `json:"act,omitzero"`
}

// The outcomes, closed.
const (
	// OutcomeApplying · 202 and a handle: the cutover is running and the
	// profile on disk has NOT changed yet.
	OutcomeApplying = "applying"
	// OutcomeApplied · the new app serves and the profile was saved.
	OutcomeApplied = "applied"
	// OutcomeNotApplied · the cutover rolled back. The profile was never
	// written, which is the supervisor's invariant, not this piece's.
	OutcomeNotApplied = "not_applied"
	// OutcomeAppliedNotSaved · the one with two halves.
	OutcomeAppliedNotSaved = "applied_not_saved"
	// OutcomeAnotherChangeInFlight · one cutover at a time. NEVER
	// «not_applied»: it IS being applied, by someone else.
	OutcomeAnotherChangeInFlight = "another_change_in_flight"
	// OutcomeWouldSelfLock · the change would leave Korvun refusing its own
	// admin calls, so no button on this screen can work until it is fixed.
	OutcomeWouldSelfLock = "would_self_lock"
	// OutcomeNeedsConfirmation · a door that opens real execution was called
	// without its confirmation. Nothing was attempted.
	OutcomeNeedsConfirmation = "needs_confirmation"
	// OutcomeNoLedger · there is no action store to write the operator act
	// into. The change is REFUSED rather than applied unrecorded: a profile
	// change nobody can audit is the thing the act exists to prevent.
	OutcomeNoLedger = "no_ledger"
	// OutcomeActNotRecorded · the ledger refused the act. Nothing was
	// attempted — the supervisor was never asked.
	OutcomeActNotRecorded = "act_not_recorded"
	// OutcomeRefused · the edit itself is one this door does not make (a
	// `deny` handed to `lift-shadow`, a ceiling that would come DOWN). The
	// reload was never asked for.
	OutcomeRefused = "refused"
	// OutcomeLedgerExists · `enable-storage` found a file already at the path
	// this profile would use, and this process did not create it. It is NEVER
	// adopted: the default path is the desktop's own book on this machine.
	// The detail names the path; the operator adopts it or names another by
	// hand in the profile. Nothing was attempted.
	OutcomeLedgerExists = "ledger_exists"
	// OutcomeLedgerNotCreated · `enable-storage` could not create the file at
	// all. Nothing exists that did not exist before; nothing was attempted.
	OutcomeLedgerNotCreated = "ledger_not_created"
	// OutcomeLedgerForeign · the ledger was founded or adopted by another
	// profile. Nothing was attempted: the act could not be sealed in a book
	// this profile does not own. `adopt-ledger` is the way out.
	OutcomeLedgerForeign = "ledger_foreign_profile"
	// OutcomeAdopted · the ledger now names this profile as its owner, in a
	// receipt. No profile change and no cutover were involved.
	OutcomeAdopted = "adopted"
)

// ledgerForeignDetail is what every door answers on a ledger another profile
// owns. It names the way out on this very screen.
const ledgerForeignDetail = "este libro lo fundó o adoptó otro perfil, así que un acto de este perfil no puede quedar registrado en él: no se aplica nada. Pulsa «Adoptar libro» en esta pantalla para que este perfil se quede con él"

// noLedgerDetail is what every door but `enable-storage` answers on a profile
// with no action store. It names the way out, twice: the button on this very
// screen, and the hand edit.
const noLedgerDetail = "este perfil no tiene almacén de acciones, así que un cambio no podría quedar registrado: no se aplica nada sin libro donde apuntarlo. Pulsa «Activar almacén» en esta pantalla, o escribe storage.path en el perfil y reinicia"

// whatsRequest is the body every button sends.
type whatsRequest struct {
	Confirm bool   `json:"confirm"`
	Brain   string `json:"brain,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Host    string `json:"host,omitempty"`
}

// whatsDoor is one write door: an EDIT of the current profile (four of them),
// the one door that FOUNDS the ledger before editing the profile to name it,
// or the one that ADOPTS a ledger another profile owns — an act in the book,
// no profile change.
type whatsDoor struct {
	edit   func(*config.Config, whatsRequest) error
	founds bool
	adopts bool
}

// whatsDoors are the six write doors, by the name the screen rows join on.
var whatsDoors = map[string]whatsDoor{
	"enable-approvals": {edit: enableApprovals},
	"set-ceiling":      {edit: setCeiling},
	"lift-shadow":      {edit: liftShadow},
	"allow-host":       {edit: allowHost},
	"enable-storage":   {founds: true},
	"adopt-ledger":     {adopts: true},
}

// RegisterWhatsHappening mounts the screen's read door and its four write
// doors. Call it ONLY with a non-empty bearer token, like the rest of the
// mutation surface.
//
// The read door is mounted beside the write doors ON PURPOSE, rather than
// hanging off the approvals surface: `/api/approvals` needs an open action
// store, and the profile with NO store is exactly the one whose first contract
// row explains why nothing can park. A screen that could not read its own rows
// in that state would go dark where it is most needed.
// A nil rec is NOT «record nothing»: every door then refuses by name. A profile
// with no action store has no book to write the act into, and applying a change
// unrecorded is precisely what the act exists to prevent — so the doors stay
// mounted, so the screen can explain the state, and they refuse.
func RegisterWhatsHappening(m Mounter, token string, rl Reloader, rec ActRecorder) {
	m.Handle("GET /api/whats-happening", bearerAuth(token)(whatsRowsHandler(rec)))
	for path, door := range whatsDoors {
		var h http.HandlerFunc
		switch {
		case door.founds:
			h = whatsLedgerHandler(rl, rec, path)
		case door.adopts:
			h = whatsAdoptHandler(rec, path)
		default:
			h = whatsHandler(rl, rec, path, door.edit)
		}
		m.Handle("POST /api/whats-happening/"+path, bearerAuth(token)(h))
	}
}

// whatsRowsHandler serves the contract. It reads no config and holds no state:
// the rows are what the CODE can decide by, which does not vary per profile.
//
// It serves the condition join ALONGSIDE the rows so the window never keeps a
// second copy of it. The gate's `ceiling` is the code's `effect_ceiling`, its
// `governance_denies` is `deny`; a screen that re-derived that mapping in
// TypeScript would be a second truth, and the two would drift the first time a
// condition is added on one side only.
//
// It also serves the LEDGER's standing for this profile — "ok",
// "legacy_unfounded" or "ledger_foreign_profile" with the owner the last mark
// names, or, when it could not be judged, "unreadable", "environment" or
// "unavailable" with the cause — and, from a recorder that knows it, the path
// of the ledger's file, because this door answers with approvals off and
// with zero brains, which is where `/api/approvals` cannot. Read from the
// recorder on every call: the standing is judged, never cached.
func whatsRowsHandler(rec ActRecorder) http.HandlerFunc {
	conditions := make(map[string]string, len(conditionRows))
	for c, rule := range conditionRows {
		conditions[string(c)] = rule
	}
	return func(w http.ResponseWriter, r *http.Request) {
		answer := map[string]any{"rows": ScreenRows(), "conditions": conditions}
		if rec != nil {
			standing, owner := rec.LedgerStanding(r.Context())
			if standing != "" {
				ledger := map[string]string{"standing": standing, "owner": owner}
				if p, ok := rec.(LedgerPathProvider); ok {
					if path := p.LedgerPath(); path != "" {
						ledger["path"] = path
					}
				}
				answer["ledger"] = ledger
			}
		}
		writeJSON(w, answer)
	}
}

// confirmingDoors are the two that open REAL execution — lifting a shadow makes
// a simulated tool able to act, and widening a cage lets it reach a new host —
// and the one that takes a book for this profile. They refuse without
// `confirm`, and they refuse BEFORE touching the reloader or the ledger —
// «cero escrituras» proved by never getting there, not by comparing a file
// afterwards.
var confirmingDoors = map[string]bool{"lift-shadow": true, "allow-host": true, "adopt-ledger": true}

func whatsHandler(rl Reloader, rec ActRecorder, door string,
	edit func(*config.Config, whatsRequest) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req whatsRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConfigBodyBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "the request body could not be read")
			return
		}
		if confirmingDoors[door] && !req.Confirm {
			writeJSONStatus(w, http.StatusPreconditionRequired, WhatsHappeningOutcome{
				Outcome: OutcomeNeedsConfirmation, ProfileUnchanged: true,
				Detail: "esta acción abre ejecución real y necesita tu confirmación",
			})
			return
		}
		cur := rl.CurrentConfig()
		if cur == nil {
			writeError(w, http.StatusServiceUnavailable, "no current config")
			return
		}
		// A SHALLOW copy would share the brains slice and every pointer under
		// it with the config the app is running on, so `set-ceiling`,
		// `lift-shadow` and `allow-host` would mutate the LIVE profile before
		// the supervisor had decided anything — and a rolled-back cutover would
		// leave the running app changed while the disk and the handle both said
		// nothing happened. The edits get their own copy of everything they
		// touch.
		next := cloneForEdit(cur)
		if err := edit(next, req); err != nil {
			writeJSONStatus(w, http.StatusUnprocessableEntity, WhatsHappeningOutcome{
				Outcome: OutcomeRefused, ProfileUnchanged: true, Detail: err.Error(),
			})
			return
		}
		// The same two gates `POST /api/config` runs, for the same reasons, and
		// BEFORE the reloader is asked. A button is a narrower edit than a
		// posted document, not a more trusted one: an edit that starts from a
		// config the operator hand-wrote can carry a violation the button never
		// introduced, and a cutover is the wrong place to discover it.
		if err := next.Validate(); err != nil {
			writeJSONStatus(w, http.StatusUnprocessableEntity, WhatsHappeningOutcome{
				Outcome: OutcomeRefused, ProfileUnchanged: true, Detail: err.Error(),
			})
			return
		}
		// F11: applying a config that removes the admin token would lock the
		// operator out of this very screen, irrecoverably across a restart —
		// the buttons would be gone and only a hand edit of the file could
		// bring them back.
		//
		// HONEST SCOPE, because a guard counted as live coverage it does not
		// have is the kind of overclaim this file has already had to retract:
		// none of the four edits touches `Admin`, so `wouldSelfLock(next)`
		// cannot differ from `wouldSelfLock(cur)` — and if it were true of
		// `cur`, the app would have booted without a token and these doors
		// would not be mounted at all. It is therefore DEFENCE IN DEPTH against
		// a fifth door, not a branch a today's button can reach. Its mould
		// drives the handler directly, which is the only way to reach it.
		if wouldSelfLock(next) {
			writeJSONStatus(w, http.StatusConflict, WhatsHappeningOutcome{
				Outcome: OutcomeWouldSelfLock, ProfileUnchanged: true,
				Detail: "este cambio dejaría a Korvun sin su token de administración: la pantalla se quedaría sin botones y solo un cambio a mano en el fichero los devolvería",
			})
			return
		}
		// THE ACT COMES BEFORE THE CHANGE. Everything above this line could
		// refuse without anything having been attempted, so there is nothing to
		// record; from here on a change is going to be asked for, and the book
		// has to say so first.
		if rec == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeNoLedger, ProfileUnchanged: true, Detail: noLedgerDetail,
			})
			return
		}
		act, err := rec.BeginConfigAct(r.Context(), actVerb(door), canonicalActParams(door, req))
		if err != nil {
			// A recorder that exists and has no ledger — the shape a profile
			// with no storage block mounts — refuses by the SAME name a nil
			// recorder does: the operator is told the profile has no book, not
			// that the book refused.
			if errors.Is(err, ErrNoLedger) {
				writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
					Outcome: OutcomeNoLedger, ProfileUnchanged: true, Detail: noLedgerDetail,
				})
				return
			}
			// A ledger another profile owns: nothing is attempted, and the
			// operator is told to adopt it — the one door such a ledger admits.
			if errors.Is(err, ErrLedgerForeign) {
				writeJSONStatus(w, http.StatusConflict, WhatsHappeningOutcome{
					Outcome: OutcomeLedgerForeign, ProfileUnchanged: true, Detail: ledgerForeignDetail,
				})
				return
			}
			// The supervisor was never asked. «No act, no change» holds here by
			// impossibility, not by a promise.
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeActNotRecorded, ProfileUnchanged: true,
				Detail: "el cambio no se intentó porque no se pudo registrar el acto en el libro: " + err.Error(),
			})
			return
		}
		h, err := rl.RequestReload(next)
		if err != nil {
			// The act closes FAILED here — the change was never attempted by
			// the supervisor — and the CLOSED act travels, receipt included:
			// an operator shown an act without its receipt cannot check it.
			act = rec.SettleAct(r.Context(), act.ActionID, false, err.Error())
			if errors.Is(err, supervisor.ErrReloadInProgress) {
				writeJSONStatus(w, http.StatusConflict, WhatsHappeningOutcome{
					Outcome: OutcomeAnotherChangeInFlight, ProfileUnchanged: true,
					Detail: "ya se está aplicando otro cambio; vuelve a pulsar cuando termine",
					Act:    act,
				})
				return
			}
			writeError(w, http.StatusInternalServerError, "reload could not be started")
			return
		}
		rec.BindReload(act.ActionID, string(h))
		st := rl.Status(h)
		out := outcomeFor(st, string(h))
		// Closed HERE only when the outcome is already known. A cutover still in
		// flight leaves the act OPEN and bound to its handle, and the status door
		// closes it when it learns the answer — an act closed on a `pending`
		// state would be recording a wish.
		if settled, applied := actOutcome(st); settled {
			// The close is where the receipt is born, so the answer carries the
			// CLOSED act — id and receipt — rather than the sealed one.
			act = rec.SettleAct(r.Context(), act.ActionID, applied, out.Outcome)
		}
		out.Act = act
		writeJSONStatus(w, http.StatusOK, out)
	}
}

// whatsLedgerHandler is the ONE door open on a profile with no action store
// (the director's decision, 2026-09-24). Its order is the guarantee: the ledger
// is founded and the founding act sealed in it, and ONLY THEN is the supervisor
// handed a profile that names the ledger's path. Nothing is written to the
// profile by this handler: the supervisor persists after a confirmed cutover,
// as for every other door.
//
// It needs no confirmation: founding the book that records changes opens no
// execution. It refuses, by name and before touching the ledger seam, a profile
// that already has a store — there is nothing to activate — and it turns the
// seam's three refusals into their own outcomes (a file already there that this
// process did not create; a file that could not be created; a founding act that
// could not be sealed), because the operator's next move differs for each.
func whatsLedgerHandler(rl Reloader, rec ActRecorder, door string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req whatsRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConfigBodyBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "the request body could not be read")
			return
		}
		cur := rl.CurrentConfig()
		if cur == nil {
			writeError(w, http.StatusServiceUnavailable, "no current config")
			return
		}
		if cur.Storage != nil {
			writeJSONStatus(w, http.StatusUnprocessableEntity, WhatsHappeningOutcome{
				Outcome: OutcomeRefused, ProfileUnchanged: true,
				Detail: "este perfil ya tiene almacén de acciones (" + cur.Storage.Path + "): no hay nada que activar",
			})
			return
		}
		if rec == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeNoLedger, ProfileUnchanged: true, Detail: noLedgerDetail,
			})
			return
		}
		// THE LEDGER, THEN THE ACT, THEN THE CHANGE. The founding act is sealed
		// in the book this call creates; if the book cannot be created or the
		// act cannot be sealed, the supervisor is never asked.
		act, path, err := rec.CreateLedger(r.Context(), door)
		switch {
		case errors.Is(err, ErrLedgerExists):
			writeJSONStatus(w, http.StatusConflict, WhatsHappeningOutcome{
				Outcome: OutcomeLedgerExists, ProfileUnchanged: true,
				Detail: "no se activó: ya hay un libro en " + path + " que este perfil no nombra, y esta puerta solo adopta un libro que ella misma haya creado en esta sesión. Para usarlo, o para elegir otra ruta, escribe storage.path en el perfil y reinicia",
			})
			return
		case errors.Is(err, ErrLedgerNotCreated):
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeLedgerNotCreated, ProfileUnchanged: true,
				Detail: "no se activó: no se pudo crear el libro en " + path + ": " + err.Error(),
			})
			return
		case err != nil:
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeActNotRecorded, ProfileUnchanged: true,
				Detail: "el libro se creó en " + path + " pero su primer acto no se pudo registrar, así que no se activó: " + err.Error(),
			})
			return
		}
		next := cloneForEdit(cur)
		next.Storage = &config.StorageConfig{Path: path}
		// The same two gates every door runs. They cannot fail on THIS edit
		// today — a storage block invalidates nothing and touches no admin
		// token — and they run anyway, because a cutover is the wrong place to
		// discover otherwise. A refusal here closes the founding act as NOT
		// applied: the book exists, and it says the activation was attempted.
		if err := next.Validate(); err != nil {
			act = rec.SettleAct(r.Context(), act.ActionID, false, err.Error())
			writeJSONStatus(w, http.StatusUnprocessableEntity, WhatsHappeningOutcome{
				Outcome: OutcomeRefused, ProfileUnchanged: true, Detail: err.Error(), Act: act,
			})
			return
		}
		if wouldSelfLock(next) {
			act = rec.SettleAct(r.Context(), act.ActionID, false, "would self-lock")
			writeJSONStatus(w, http.StatusConflict, WhatsHappeningOutcome{
				Outcome: OutcomeWouldSelfLock, ProfileUnchanged: true, Act: act,
				Detail: "este cambio dejaría a Korvun sin su token de administración",
			})
			return
		}
		h, err := rl.RequestReload(next)
		if err != nil {
			act = rec.SettleAct(r.Context(), act.ActionID, false, err.Error())
			if errors.Is(err, supervisor.ErrReloadInProgress) {
				writeJSONStatus(w, http.StatusConflict, WhatsHappeningOutcome{
					Outcome: OutcomeAnotherChangeInFlight, ProfileUnchanged: true,
					Detail: "ya se está aplicando otro cambio; vuelve a pulsar cuando termine",
					Act:    act,
				})
				return
			}
			writeError(w, http.StatusInternalServerError, "reload could not be started")
			return
		}
		rec.BindReload(act.ActionID, string(h))
		st := rl.Status(h)
		out := outcomeFor(st, string(h))
		if settled, applied := actOutcome(st); settled {
			act = rec.SettleAct(r.Context(), act.ActionID, applied, out.Outcome)
		}
		out.Act = act
		// Where the book lives, said in the answer: the profile names it from
		// now on, and the operator should be able to find it.
		if out.Detail != "" {
			out.Detail += ". "
		}
		out.Detail += "El libro vive en " + path
		writeJSONStatus(w, http.StatusOK, out)
	}
}

// whatsAdoptHandler is the one door a ledger owned by ANOTHER profile admits:
// the adoption act, sealed and closed with the mark naming this profile in one
// store transaction, behind a confirmation. It changes no profile and asks the
// supervisor for nothing: the book's owner changes, not the running Korvun.
func whatsAdoptHandler(rec ActRecorder, door string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req whatsRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConfigBodyBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "the request body could not be read")
			return
		}
		if confirmingDoors[door] && !req.Confirm {
			writeJSONStatus(w, http.StatusPreconditionRequired, WhatsHappeningOutcome{
				Outcome: OutcomeNeedsConfirmation, ProfileUnchanged: true,
				Detail: "quedarse con un libro es un consentimiento y necesita tu confirmación",
			})
			return
		}
		if rec == nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeNoLedger, ProfileUnchanged: true, Detail: noLedgerDetail,
			})
			return
		}
		act, err := rec.AdoptLedger(r.Context())
		switch {
		case errors.Is(err, ErrNoLedger):
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeNoLedger, ProfileUnchanged: true, Detail: noLedgerDetail,
			})
			return
		case err != nil:
			writeJSONStatus(w, http.StatusServiceUnavailable, WhatsHappeningOutcome{
				Outcome: OutcomeActNotRecorded, ProfileUnchanged: true,
				Detail: "el libro no se adoptó porque el acto de adopción no se pudo registrar: " + err.Error(),
			})
			return
		}
		writeJSONStatus(w, http.StatusOK, WhatsHappeningOutcome{
			Outcome: OutcomeAdopted, Applied: true, ProfileUnchanged: true, Act: act,
			Detail: "este perfil ha adoptado el libro: queda registrado como acto con su recibo, y las demás puertas vuelven a aplicar",
		})
	}
}

// ReloadOutcome says whether a supervisor state is terminal and, if so,
// whether the running Korvun changed. It is the one mapping every settler
// uses — the doors here and the process-wide registry in internal/app — so
// «applied» cannot mean two things.
func ReloadOutcome(st supervisor.State) (settled, applied bool) { return actOutcome(st) }

// canonicalActParams is what the act SEALS: the door and the request's own
// fields, in a fixed order. It is the digest's input, so it must not vary with
// map iteration or with how the operator's client ordered its JSON.
func canonicalActParams(door string, req whatsRequest) []byte {
	b, err := json.Marshal(struct {
		Door  string `json:"door"`
		Brain string `json:"brain,omitempty"`
		Tool  string `json:"tool,omitempty"`
		Host  string `json:"host,omitempty"`
	}{Door: door, Brain: req.Brain, Tool: req.Tool, Host: req.Host})
	if err != nil {
		// A struct of four strings cannot fail to marshal. If it ever did, the
		// act must still be sealed over SOMETHING that names the door rather
		// than over nothing.
		return []byte(`{"door":"` + door + `"}`)
	}
	return b
}

// actOutcome says whether a supervisor state is terminal and, if so, whether the
// running Korvun changed.
//
// `StatePersistFailed` counts as APPLIED because it is: the new app is serving.
// That the disk was not updated is the change's own half-truth, which the
// outcome text carries; the act records that the attempt succeeded, because it
// did.
func actOutcome(st supervisor.State) (settled, applied bool) {
	switch st {
	case supervisor.StateSucceeded, supervisor.StatePersistFailed:
		return true, true
	case supervisor.StateRolledBack, supervisor.StateFailed:
		return true, false
	default:
		return false, false
	}
}

// outcomeFor translates the supervisor's state into what the screen may say.
// Every state the supervisor can reach has its own sentence; none of them is
// allowed to be reported as a plain success.
func outcomeFor(st supervisor.State, handle string) WhatsHappeningOutcome {
	switch st {
	case supervisor.StateSucceeded:
		return WhatsHappeningOutcome{Outcome: OutcomeApplied, Applied: true}
	case supervisor.StatePersistFailed:
		return WhatsHappeningOutcome{
			Outcome: OutcomeAppliedNotSaved, Applied: true, ProfileUnchanged: true,
			Detail: "el cambio está en marcha pero no se pudo guardar: al reiniciar volverá atrás",
		}
	case supervisor.StateRolledBack, supervisor.StateFailed:
		return WhatsHappeningOutcome{
			Outcome: OutcomeNotApplied, ProfileUnchanged: true,
			Detail: "no se aplicó; tu perfil sigue como estaba",
		}
	default:
		return WhatsHappeningOutcome{
			Outcome: OutcomeApplying, ProfileUnchanged: true, Handle: handle,
			Detail: "aplicando; tu perfil en disco todavía no ha cambiado",
		}
	}
}

// cloneForEdit copies the config down to every field the four edits can touch.
// Anything they cannot reach is shared on purpose: this is a copy for ONE edit,
// not a general-purpose deep clone, and pretending otherwise would be a wider
// promise than the code keeps.
func cloneForEdit(c *config.Config) *config.Config {
	out := *c
	out.Brains = append([]config.BrainConfig(nil), c.Brains...)
	for i := range out.Brains {
		a := out.Brains[i].Agent
		if a == nil {
			continue
		}
		agent := *a
		agent.Governance = append([]config.ToolGrantConfig(nil), a.Governance...)
		for j := range agent.Governance {
			agent.Governance[j].Channels = append([]string(nil), a.Governance[j].Channels...)
		}
		if a.WebhookCall != nil {
			w := *a.WebhookCall
			w.AllowHosts = append([]string(nil), a.WebhookCall.AllowHosts...)
			agent.WebhookCall = &w
		}
		if a.HTTPFetch != nil {
			h := *a.HTTPFetch
			h.AllowHosts = append([]string(nil), a.HTTPFetch.AllowHosts...)
			agent.HTTPFetch = &h
		}
		out.Brains[i].Agent = &agent
	}
	return &out
}

// ---------------------------------------------------------------------------
// The four edits. Each one changes ONE thing and leaves the rest of the profile
// exactly as it came, so a button can never carry a change the operator did not
// ask for.
// ---------------------------------------------------------------------------

// enableApprovals turns the switch on and keeps whatever TTL the operator had.
// Replacing the whole block would silently reset a window they chose, which is
// a change nobody asked this button for.
func enableApprovals(c *config.Config, _ whatsRequest) error {
	ttl := ""
	if c.Approvals != nil {
		ttl = c.Approvals.TTL
	}
	c.Approvals = &config.ApprovalsConfig{Enabled: true, TTL: ttl}
	return nil
}

// theCeiling is the one value `set-ceiling` sets: the rung where an
// irreversible write starts needing an approval.
const theCeiling = action.EffectWriteIrreversible

// setCeiling raises the named brain's ceiling to write_irreversible, and does
// nothing else.
//
// It NAMES its brain. Defaulting to the first brain of the list was the
// original shape, and on a profile with several agent brains it would have
// applied the operator's click to whichever one the config happens to list
// first — a change to a brain they never saw on screen.
//
// It also never comes DOWN. A brain already at `critical` sits ABOVE
// write_irreversible on the declared ladder, so writing the ceiling would
// LOWER it and widen what executes without an approval. The refusal is by
// rank, not by name, and an unknown ceiling refuses too: a value this build
// cannot place on the ladder is not one to overwrite silently.
func setCeiling(c *config.Config, req whatsRequest) error {
	b, err := brainFor(c, req.Brain)
	if err != nil {
		return err
	}
	switch cur := action.EffectClass(b.Agent.EffectCeiling); {
	case cur == "":
		// No ceiling at all: the effect gate is bypassed entirely. This is the
		// state the button exists for.
	case !cur.Known():
		return fmt.Errorf("el techo actual %q no es una clase de efecto que esta versión reconozca: no se sobreescribe a ciegas", cur)
	case cur.Rank() >= theCeiling.Rank():
		return fmt.Errorf("el techo de %q ya es %q, que no está por debajo de %q: bajarlo ampliaría lo que se ejecuta sin aprobación", b.Name, cur, theCeiling)
	}
	b.Agent.EffectCeiling = string(theCeiling)
	return nil
}

// liftShadow turns a SHADOWED tool into an allowed one, and refuses anything
// else by name.
//
// It refuses a `deny`. The screen's confirmation for this door reads «Sí, que
// pueda ejecutarse tras mi aprobación», which is the truth about a shadow: a
// tool already being simulated becomes able to act. An explicit `deny` is a
// different decision — the profile says no, not «pretend» — and turning it into
// `allow` behind the shadow's sentence would take a consent the operator never
// gave.
func liftShadow(c *config.Config, req whatsRequest) error {
	b, err := brainFor(c, req.Brain)
	if err != nil {
		return err
	}
	if req.Tool == "" {
		return errors.New("lift-shadow needs the tool to lift")
	}
	for i := range b.Agent.Governance {
		g := &b.Agent.Governance[i]
		if g.Tool != req.Tool {
			continue
		}
		if g.Mode != "shadow" {
			return fmt.Errorf("«%s» está en modo %q, no en sombra: este botón solo levanta una sombra, y su confirmación no cubre convertir otra cosa en «allow»", req.Tool, g.Mode)
		}
		g.Mode = "allow"
		return nil
	}
	return errors.New("that tool has no governance entry to lift")
}

func allowHost(c *config.Config, req whatsRequest) error {
	b, err := brainFor(c, req.Brain)
	if err != nil {
		return err
	}
	if req.Host == "" {
		return errors.New("allow-host needs the host to add")
	}
	switch req.Tool {
	case "webhook_call":
		if b.Agent.WebhookCall == nil {
			return errors.New("this brain has no webhook_call cage to widen")
		}
		b.Agent.WebhookCall.AllowHosts = addHost(b.Agent.WebhookCall.AllowHosts, req.Host)
	case "http_fetch":
		if b.Agent.HTTPFetch == nil {
			return errors.New("this brain has no http_fetch cage to widen")
		}
		b.Agent.HTTPFetch.AllowHosts = addHost(b.Agent.HTTPFetch.AllowHosts, req.Host)
	default:
		return errors.New("only webhook_call and http_fetch have a host allow-list")
	}
	return nil
}

// addHost appends without duplicating: pressing the button twice must not grow
// the list, and an operator who re-adds a host they already have gets the same
// profile back rather than a silently different one.
func addHost(hosts []string, host string) []string {
	for _, h := range hosts {
		if h == host {
			return hosts
		}
	}
	return append(append([]string(nil), hosts...), host)
}

// brainFor resolves the brain a door names. The name is REQUIRED: an empty name
// used to mean «the first one», which silently pointed a click at whichever
// brain the profile lists first.
func brainFor(c *config.Config, name string) (*config.BrainConfig, error) {
	if name == "" {
		return nil, errors.New("esta acción necesita el nombre del cerebro: sin él, el cambio caería en el primero de la lista, que no es el que estás mirando")
	}
	for i := range c.Brains {
		if c.Brains[i].Name != name {
			continue
		}
		if c.Brains[i].Agent == nil {
			return nil, errors.New("that brain is not an agent brain")
		}
		return &c.Brains[i], nil
	}
	return nil, errors.New("no brain by that name")
}

// writeDoorNames returns the names of the mounted write doors, sorted. It exists
// for the contract mould: a row claiming a button must name a door that really
// exists, and the only honest source for «really exists» is the map the mount
// iterates.
func writeDoorNames() []string {
	out := make([]string, 0, len(whatsDoors))
	for name := range whatsDoors {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
