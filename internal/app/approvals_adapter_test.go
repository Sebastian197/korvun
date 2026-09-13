// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.15.0 — the ADAPTER: the seam controlapi.Approvals implemented against a
// REAL SQLite store and a REAL profile. RED: NewApprovalsAdapter does not
// exist yet.
//
// What these moulds are for. The 87 screen tests feed the UI fabricated
// answers: they would pass identically if the endpoint never produced
// `expired`, `invalidated` or `evidence_corrupt`. These are the ones that
// prove the server DOES produce them — FR-TEST-4.
//
// Evidence level, honest: a REAL store on disk and a REAL config, in process.
// Not a fake. Not a compiled binary either: nothing here proves the wire.
//
// Anchors: §11 FR-API-1..25, §12 AS-88..AS-121, §13-bis, and
// 2026-09-12-approvals-adapter-pre-test-review.md §0-sexies.

package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// ---------------------------------------------------------------------------
// FR-API-6 · the gate block. brains_can_park counts the brains that meet the
// FIVE conditions the gate really demands, derived from the SAME resolution
// that wires the identities — never by re-reading the profile's JSON.
//
// It is a declared UPPER BOUND, not a promise: the fifth condition is
// per-channel, so there is no channel-independent boolean.
// ---------------------------------------------------------------------------

// TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark walks the five
// conditions one at a time. Each case meets four and fails the fifth-under-test,
// so the count must be 0; the last case meets all five and must be 1.
//
// Probing mutation, one per row: drop that condition from the predicate ⇒ the
// count becomes 1 and that row reddens. The `all five` row is the control: it
// must stay green under every mutation, which is what proves the others are not
// passing by accident.
func TestApprovalsGate_countsOnlyTheBrainsThatCanActuallyPark(t *testing.T) {
	cases := []struct {
		name string
		mut  func(b *config.BrainConfig, cfg *config.Config)
		want int
	}{
		{"1 · no action store: the storage block is gone", func(_ *config.BrainConfig, cfg *config.Config) {
			cfg.Storage = nil
		}, 0},
		{"2 · not an agent brain: no agent block", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent = nil
		}, 0},
		{"3 · the ceiling does not reach write_irreversible", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.EffectCeiling = "write_reversible"
		}, 0},
		// read_file is DECLARED read_external: the cage lists a real tool with
		// a real descriptor whose class simply is not parkable. A tool with no
		// descriptor at all would make this row pass for the wrong reason.
		{"4 · no tool of a parkable class in the cage", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.Tools = []string{"read_file"}
			b.Agent.WebhookCall = nil
		}, 0},
		{"5 · governance denies the only parkable tool", func(b *config.BrainConfig, _ *config.Config) {
			b.Agent.Governance = []config.ToolGrantConfig{{Tool: "webhook_call", Mode: "deny"}}
		}, 0},
		{"all five: the control", func(_ *config.BrainConfig, _ *config.Config) {}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, store, done := parkingProfile(t)
			defer done()
			tc.mut(&cfg.Brains[0], cfg)
			a := NewApprovalsAdapter(cfg, store)

			got, err := a.ListPending(context.Background())
			if err != nil {
				t.Fatalf("ListPending: %v", err)
			}
			if got.Gate.BrainsCanPark != tc.want {
				t.Fatalf("brains_can_park = %d, want %d", got.Gate.BrainsCanPark, tc.want)
			}
			if got.Gate.BrainsTotal != 1 {
				t.Fatalf("brains_total = %d, want 1 — the total counts brains, not capabilities", got.Gate.BrainsTotal)
			}
		})
	}
}

// TestApprovalsGate_offMeansOffAndNeverAnEmptyList pins E3: with the switch off
// the list does NOT answer 200 with zero rows, because «nothing parked» and
// «nothing can park» would become the same answer — the fail-open V1 exists to
// stop.
//
// Probing mutation: answer an empty 200 with the gate off ⇒ this reddens.
func TestApprovalsGate_offMeansOffAndNeverAnEmptyList(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	cfg.Approvals = &config.ApprovalsConfig{Enabled: false}
	a := NewApprovalsAdapter(cfg, store)

	_, err := a.ListPending(context.Background())
	if !errors.Is(err, controlapi.ErrApprovalsDisabled) {
		t.Fatalf("err = %v, want ErrApprovalsDisabled", err)
	}
}

// ---------------------------------------------------------------------------
// FR-TEST-4 · the names, produced by a REAL store.
// ---------------------------------------------------------------------------

// TestAdapter_expiredComesFromASweptRow: a row the sweep closed is `expired`,
// and the precedence is by STATE — never by whatever a belt would say.
//
// Probing mutation: map EXPIRED to already_decided ⇒ this reddens.
func TestAdapter_expiredComesFromASweptRow(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_exp")
	if _, _, err := store.SweepExpiredApprovals(context.Background(), time.Now().UTC().Add(2*time.Hour)); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	_, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if !errors.Is(err, controlapi.ErrApprovalExpired) {
		t.Fatalf("err = %v, want ErrApprovalExpired", err)
	}
}

// TestAdapter_invalidatedCarriesTheCurrentLawInItsOwnField: E7 prints BOTH
// digests, and the current one travels in a FIELD of the error body, never
// inside the message — extracting a structured value from a sentence is the
// «by text» E9 forbids.
//
// Probing mutation: put the current law inside the message ⇒ this reddens.
func TestAdapter_invalidatedCarriesTheCurrentLawInItsOwnField(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_law")
	moveTheLaw(t, cfg)

	_, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if !errors.Is(err, controlapi.ErrApprovalInvalidated) {
		t.Fatalf("err = %v, want ErrApprovalInvalidated", err)
	}
	var moved interface{ CurrentLawDigest() string }
	if !errors.As(err, &moved) {
		t.Fatalf("the error must carry the CURRENT law digest in a field of its own")
	}
	if moved.CurrentLawDigest() == "" {
		t.Fatalf("current law digest is empty")
	}
}

// TestAdapter_aMutatedStoryIsEvidenceCorruptOnTheRead is AS-96's class on the
// READ door: what the belt refuses at a GET is `evidence_corrupt`, permanent.
//
// Probing mutation: let it fall to the transient residual ⇒ this reddens.
func TestAdapter_aMutatedStoryIsEvidenceCorruptOnTheRead(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_story")
	corruptStory(t, store, "act_story")

	_, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if !errors.Is(err, controlapi.ErrApprovalEvidenceCorrupt) {
		t.Fatalf("err = %v, want ErrApprovalEvidenceCorrupt", err)
	}
	if errors.Is(err, controlapi.ErrApprovalsUnavailable) {
		t.Fatalf("permanent corruption must never be served as transient")
	}
}

// TestAdapter_theSameCorruptionAfterACommittedDecideIsDecidedEvidenceCorrupt is
// the other half, and the reason the two names exist: the SAME belt, the same
// refusal, but with a decision already sealed the literal has to say so.
//
// This is CONFLICTO 1, adjudicated: §11's eje 1 wins over §13-bis's stale row.
//
// Probing mutation: emit evidence_corrupt here ⇒ this reddens.
func TestAdapter_theSameCorruptionAfterACommittedDecideIsDecidedEvidenceCorrupt(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_dec_story")
	ad := NewApprovalsAdapter(cfg, store)
	if _, err := ad.Approve(context.Background(), a.ApprovalID, a.ActionDigest); err == nil {
		t.Logf("approve returned no error; the attack lands on the NEXT touch")
	}
	corruptStory(t, store, "act_dec_story")

	_, err := ad.Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if !errors.Is(err, controlapi.ErrApprovalDecidedEvidenceBad) {
		t.Fatalf("err = %v, want ErrApprovalDecidedEvidenceBad", err)
	}
}

// TestAdapter_aStaleDigestIsRefusedWithoutConsumingTheApproval is FR-API-14 and
// its oracle by impossibility: the refusal must leave the approval decidable.
//
// Probing mutation: compare the digest AFTER deciding ⇒ the approval is
// consumed and the re-read reddens this.
func TestAdapter_aStaleDigestIsRefusedWithoutConsumingTheApproval(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_stale")

	_, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, "sha256:"+staleHex)
	if !errors.Is(err, controlapi.ErrApprovalDigestMismatch) {
		t.Fatalf("err = %v, want ErrApprovalDigestMismatch", err)
	}
	after, _, err := store.GetApproval(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Status != action.ApprovalPending {
		t.Fatalf("status = %q, want PENDING — a stale digest consumes nothing", after.Status)
	}
}

// TestAdapter_aRowBornWithoutArgumentsIsEmptyInA200 pins FR-API-8: `empty` is a
// parameters_state inside a 200, never an error name — the screen still shows
// the document and simply does not offer the yes.
//
// Probing mutation: refuse with a name ⇒ this reddens.
func TestAdapter_aRowBornWithoutArgumentsIsEmptyInA200(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOneWithParams(t, cfg, store, "act_empty", "")

	d, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("a row born without arguments must still be READABLE: %v", err)
	}
	if d.ParametersState != "empty" {
		t.Fatalf("parameters_state = %q, want %q", d.ParametersState, "empty")
	}
}

// TestAdapter_mutatedParamsRefuseBeforeClassifying is FR-API-19 and AS-83: the
// re-derivation runs BEFORE parameters_state, so the mismatch precedes `empty`
// and `too_large`.
//
// Probing mutation: classify first ⇒ the mutated row would come back as
// present and this reddens.
func TestAdapter_mutatedParamsRefuseBeforeClassifying(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_params")
	mutateParams(t, store, a.ApprovalID, `{"a":2}`)

	_, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if !errors.Is(err, controlapi.ErrApprovalParamsDigestMismatch) {
		t.Fatalf("err = %v, want ErrApprovalParamsDigestMismatch", err)
	}
}

// TestAdapter_aRowDecidedBetweenTheListAndTheDetailIsAlreadyDecided is
// FR-API-18's precedence, which is by STATE and beats any belt's name.
//
// Probing mutation: serve the document of a non-PENDING row ⇒ this reddens.
func TestAdapter_aRowDecidedBetweenTheListAndTheDetailIsAlreadyDecided(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_raced")
	decideFromElsewhere(t, store, a.ApprovalID)

	_, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if !errors.Is(err, controlapi.ErrApprovalAlreadyDecided) {
		t.Fatalf("err = %v, want ErrApprovalAlreadyDecided", err)
	}
}

// TestAdapter_anUnknownStatusFailsClosed: the column has no CHECK, so a value
// outside the game is possible — and it answers already_decided, the
// fail-closed the domain already writes for every unknown status.
//
// Probing mutation: serve the document ⇒ this reddens.
func TestAdapter_anUnknownStatusFailsClosed(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_unknown")
	setApprovalStatus(t, store, a.ApprovalID, "WAT")

	_, err := NewApprovalsAdapter(cfg, store).Detail(context.Background(), a.ApprovalID)
	if !errors.Is(err, controlapi.ErrApprovalAlreadyDecided) {
		t.Fatalf("err = %v, want ErrApprovalAlreadyDecided for a status outside the game", err)
	}
}

// TestAdapter_brainGoneHasItsOwnSentinel is the director's adjudication 5: a
// name the screen paints and the server never emits is dead interface. Today
// the only source is a plain fmt.Errorf, so it could only be named by
// strings.Contains — which FR-API-15 calls a finding, not an implementation.
//
// Probing mutation: reword the message in internal/app ⇒ a text-based mapping
// stops matching and this reddens; a typed sentinel does not care.
func TestAdapter_brainGoneHasItsOwnSentinel(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_gone")
	cfg.Brains = nil

	_, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if !errors.Is(err, controlapi.ErrApprovalBrainGone) {
		t.Fatalf("err = %v, want ErrApprovalBrainGone", err)
	}
	if !errors.Is(err, ErrBrainNotInProfile) {
		t.Fatalf("the app-side sentinel must travel too: %v", err)
	}
}

// TestExecuteApprovedAction_aPendingRowIsNotDecided walks the ONE execution
// path — the same function `korvun approvals execute` runs — and pins its first
// cut: a PENDING row here means someone rewrote the state underneath, which is
// `not_decided` and never «already closed». The two would contradict each
// other, and the operator would be told the request was closed by a store that
// says it is still waiting.
//
// It calls the path directly because that IS the surface under test. The
// adapter used to carry its own Execute; it was dead code with a mould on it,
// and two implementations of an irreversible effect is the class this house
// forbids.
//
// Probing mutation: send the PENDING cut to ErrApprovalAlreadyClosed ⇒ this
// reddens.
func TestExecuteApprovedAction_aPendingRowIsNotDecided(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_pending")
	_, pin, err := ResolveApprovalLaw(cfg, cfg.Brains[len(cfg.Brains)-1].Name)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	_, err = ExecuteApprovedAction(context.Background(), store, nil, a.ApprovalID, pin)
	if !errors.Is(err, ErrApprovalNotDecided) {
		t.Fatalf("err = %v, want ErrApprovalNotDecided", err)
	}
	if errors.Is(err, ErrApprovalAlreadyClosed) {
		t.Fatalf("a row the store says is PENDING must never be called closed")
	}
}

// TestAdapter_theListSurfacesTheRowsItHadToSkip is V12 reaching the wire: the
// director's ruling says the corrupt row is skipped, COUNTED and NAMED, so the
// gate block has to carry that count — otherwise the operator loses a parked
// request in silence.
//
// Probing mutation: drop the count from the DTO ⇒ this reddens.
func TestAdapter_theListSurfacesTheRowsItHadToSkip(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	parkOne(t, cfg, store, "act_ok_1")
	bad := parkOne(t, cfg, store, "act_bad")
	parkOne(t, cfg, store, "act_ok_2")
	corruptRequestedAt(t, store, bad.ApprovalID)

	got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
	if err != nil {
		t.Fatalf("one corrupt row must never fail the list: %v", err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(got.Rows))
	}
	if got.Gate.RowsSkipped != 1 {
		t.Fatalf("rows_skipped = %d, want 1 — «se salta, se cuenta y se nombra»", got.Gate.RowsSkipped)
	}
}

// TestAdapter_theActionThatWasNoLongerPendingGetsItsName is V13 reaching the
// wire. The whole transaction rolls back — nothing decided, exec.Run never
// called — so the name must SAY that, not «we do not know whether the effect
// happened». The director, literal: mentir por exceso de cautela sigue siendo
// mentir.
//
// Probing mutation: map it to unknown_outcome, or let it fall to the unnamed
// 500 ⇒ this reddens.
func TestAdapter_theActionThatWasNoLongerPendingGetsItsName(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_v13_wire")
	setActionStateFromElsewhere(t, store, "act_v13_wire", action.StateSucceeded)

	_, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if !errors.Is(err, controlapi.ErrApprovalAlreadyClosed) {
		t.Fatalf("err = %v, want ErrApprovalAlreadyClosed — nothing was decided and nothing ran", err)
	}
	if errors.Is(err, controlapi.ErrApprovalUnknownOutcome) {
		t.Fatalf("a transaction that rolled back whole must never say «we do not know»")
	}
	after, _, err := store.GetApproval(context.Background(), a.ApprovalID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Status != action.ApprovalPending {
		t.Fatalf("status = %q, want PENDING", after.Status)
	}
}

// TestAdapter_resolvesTheLawExactlyOnce is AS-121, added to §12 by the
// director's adjudication 3. The probe IS the oracle: with the profile intact
// BuildApprovalExecutor resolves the SAME cage, so the visible outcome is
// identical and only a count can tell the two apart.
//
// Probing mutation: call BuildApprovalExecutor instead of …FromCage ⇒ the probe
// counts 2 and this reddens.
func TestAdapter_resolvesTheLawExactlyOnce(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_once")

	var resolutions int
	// The seam is armed on ResolveApprovalLaw itself, which is where every
	// resolution goes through — including BuildApprovalExecutor's, which is the
	// branch this mould forbids and which the first version of the probe could
	// not see at all.
	defer setLawResolutionProbe(func() { resolutions++ })()
	ad := NewApprovalsAdapter(cfg, store)
	// The outcome of the run is not what this mould watches, and it cannot be:
	// with an intact profile both resolvers hand back the SAME cage, so the
	// visible result is identical either way. That is precisely why the probe
	// is the oracle and the result is not.
	_, _ = ad.Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if resolutions != 1 {
		t.Fatalf("law resolutions = %d, want exactly 1 — not «one or none»", resolutions)
	}
}

// TestAdapter_readsTheInBandRuleAsARefusal is the director's adjudication 7.
// DecideApprovalUnderLaw signals IN BAND through four returns: it hands back a
// rule name with a NIL error. An adapter that reads only the error would
// publish a receipt for a decision that never happened.
//
// Probing mutation: treat err == nil as success ⇒ the outcome carries an empty
// receipt and this reddens.
func TestAdapter_readsTheInBandRuleAsARefusal(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_inband")
	decideFromElsewhere(t, store, a.ApprovalID)

	out, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if !errors.Is(err, controlapi.ErrApprovalAlreadyDecided) {
		t.Fatalf("err = %v, want ErrApprovalAlreadyDecided — the rule came back with a nil error", err)
	}
	if out.ReceiptID != "" {
		t.Fatalf("receipt = %q — a refusal publishes no receipt", out.ReceiptID)
	}
}

// TestAdapter_carriesTheReceiptItHadToReReadIt is cure 25: neither
// DecideApprovalUnderLaw nor FinishWithResult returns the receipt identifiers
// that P4 and P5 print, so the adapter re-reads them.
//
// Probing mutation: leave the receipt empty ⇒ this reddens.
func TestAdapter_carriesTheReceiptItHadToReReadIt(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_receipt")

	out, err := NewApprovalsAdapter(cfg, store).Reject(context.Background(), a.ApprovalID, "no")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if out.ReceiptID == "" {
		t.Fatalf("a sealed rejection must carry its receipt — P5 prints it")
	}
	if out.Outcome != "rejected" {
		t.Fatalf("outcome = %q, want %q", out.Outcome, "rejected")
	}
}

// TestAdapter_aFailedReceiptReReadNeverSaysTheExecutionDidNotStart is the
// director's adjudication 6, decided with the code in front. On the REJECT path
// there is no execution at all, so params_unreadable — whose literal opens
// «Esta ejecución no arrancó» — would be a fabricated sentence. The decision is
// sealed and what is missing is only its identifier, so the honest existing
// name is the one that says the record could not be closed.
//
// Probing mutation: map it to params_unreadable ⇒ this reddens.
func TestAdapter_aFailedReceiptReReadNeverSaysTheExecutionDidNotStart(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_receipt_bad")
	breakTheReceiptReRead(t, store, a.ApprovalID)

	_, err := NewApprovalsAdapter(cfg, store).Reject(context.Background(), a.ApprovalID, "no")
	if errors.Is(err, controlapi.ErrApprovalParamsUnreadable) {
		t.Fatalf("a rejection has no execution: its literal must never say one did not start")
	}
	if !errors.Is(err, controlapi.ErrApprovalCloseFailed) {
		t.Fatalf("err = %v, want ErrApprovalCloseFailed", err)
	}
}

// ---------------------------------------------------------------------------
// The profile, the store, and the attackers.
//
// Every attacker writes through a SECOND `*sql.DB` on the same file — a real
// second connection, which is what another process would be. The adapter's own
// store never sees these writes coming.
// ---------------------------------------------------------------------------

// staleHex is a well-formed digest that belongs to nothing: 64 lowercase hex.
const staleHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// parkingProfile builds the profile of a brain that meets the FIVE conditions
// of FR-API-6, opens the real store on it, and returns both plus the closer.
func parkingProfile(t *testing.T) (*config.Config, *actionsqlite.Store, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "korvun.db")
	cfg := kernelWiringConfig(dbPath)
	cfg.Approvals = &config.ApprovalsConfig{Enabled: true}
	cfg.Brains[0].Agent = &config.AgentConfig{
		Tools:         []string{"webhook_call"},
		MaxIterations: 2,
		EffectCeiling: "critical",
		WebhookCall:   &config.WebhookCallToolConfig{AllowHosts: []string{"hooks.acme.io"}},
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return cfg, store, func() { _ = store.Close() }
}

func parkOne(t *testing.T, cfg *config.Config, store *actionsqlite.Store, id string) action.Approval {
	t.Helper()
	return parkOneWithParams(t, cfg, store, id, `{"a":1}`)
}

// parkOneWithParams parks under the law the PROFILE resolves right now, which
// is what a real request would be born under. Anything else would make every
// touch read `invalidated` and the moulds would pass for the wrong reason.
func parkOneWithParams(t *testing.T, cfg *config.Config, store *actionsqlite.Store, id, rawParams string) action.Approval {
	t.Helper()
	_, pin, err := ResolveApprovalLaw(cfg, cfg.Brains[0].Name)
	if err != nil {
		t.Fatalf("resolve the profile's law: %v", err)
	}
	env := action.NewEnvelope(
		id, "env-1",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "webhook_call", Version: 1},
		rawParams,
		time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC),
	)
	env.IntentID = action.RootIntentID
	env.Principal = action.PrincipalRef{PrincipalID: "principal_brain_a"}
	env.Effect = action.Effect{Class: string(action.EffectWriteIrreversible)}
	b, berr := action.NewBoundApprovalRequest(env, rawParams, action.ApprovalContext{
		IntentPurpose: "avisar al webhook de pedidos",
		GrantID:       "grant_1", GrantDepth: 1, CostLine: "1 of 5",
		ToolCage: "webhook_call",
		Descriptor: action.EffectDescriptor{
			Class: action.EffectWriteIrreversible, DataEgress: true,
		},
		HasDescriptor: true,
		LawVersion:    pin.Version, LawDigest: pin.Digest,
		Rule: "require_approval",
		Now:  time.Now().UTC(), TTL: time.Hour,
	})
	if berr != nil {
		t.Fatalf("factory: %v", berr)
	}
	if err := store.CreateApprovalRequest(context.Background(), b); err != nil {
		t.Fatalf("park: %v", err)
	}
	return b.Approval()
}

// moveTheLaw edits the profile so the resolved pin no longer matches the one
// the rows were parked under. It edits the PROFILE, never the stored pin: a
// stored pin that moves is corruption, and that is a different name.
func moveTheLaw(t *testing.T, cfg *config.Config) {
	t.Helper()
	cfg.Brains[0].Agent.Tools = []string{"webhook_call", "time_now"}
}

func attackerDB(t *testing.T, store *actionsqlite.Store) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(store.Path()))
	if err != nil {
		t.Fatalf("attacker connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func attackerExec(t *testing.T, store *actionsqlite.Store, stmt string, args ...any) {
	t.Helper()
	if _, err := attackerDB(t, store).Exec(stmt, args...); err != nil {
		t.Fatalf("attacker exec: %v", err)
	}
}

func corruptStory(t *testing.T, store *actionsqlite.Store, actionID string) {
	t.Helper()
	attackerExec(t, store, `UPDATE actions SET effect_class = ? WHERE action_id = ?`,
		string(action.EffectPure), actionID)
}

func mutateParams(t *testing.T, store *actionsqlite.Store, approvalID, params string) {
	t.Helper()
	attackerExec(t, store, `UPDATE approvals SET canonical_params = ? WHERE approval_id = ?`,
		params, approvalID)
}

func corruptRequestedAt(t *testing.T, store *actionsqlite.Store, approvalID string) {
	t.Helper()
	attackerExec(t, store, `UPDATE approvals SET requested_at = 'ayer' WHERE approval_id = ?`, approvalID)
}

func setApprovalStatus(t *testing.T, store *actionsqlite.Store, approvalID, status string) {
	t.Helper()
	attackerExec(t, store, `UPDATE approvals SET status = ? WHERE approval_id = ?`, status, approvalID)
}

func setActionStateFromElsewhere(t *testing.T, store *actionsqlite.Store, actionID string, to action.State) {
	t.Helper()
	attackerExec(t, store, `UPDATE actions SET state = ? WHERE action_id = ?`, string(to), actionID)
}

// decideFromElsewhere closes the approval the way the CLI would: sealed, with
// its params purged in the same transaction.
func decideFromElsewhere(t *testing.T, store *actionsqlite.Store, approvalID string) {
	t.Helper()
	attackerExec(t, store,
		`UPDATE approvals SET status = 'REJECTED', decision = 'rejected',
		        decision_at = ?, canonical_params = '' WHERE approval_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), approvalID)
}

// breakTheReceiptReRead leaves the decision sealed and its identifier
// unreadable: the row keeps a decision_at nobody can parse, which is exactly
// the window cure 25 opens by re-reading.
func breakTheReceiptReRead(t *testing.T, store *actionsqlite.Store, approvalID string) {
	t.Helper()
	// SQLite triggers take no bound variables, so the id is inlined. It is a
	// test-owned literal, not user input.
	stmt := `CREATE TRIGGER break_receipt AFTER UPDATE OF status ON approvals
	  BEGIN UPDATE approvals SET decision_at = 'ayer'
	         WHERE approval_id = '` + approvalID + `'; END` // #nosec G202
	attackerExec(t, store, stmt)
}

// ---------------------------------------------------------------------------
// An id that does not exist is a CLASS, and every endpoint answers it.
//
// The defect that shipped was found on ONE of them, and curing that one would
// have left the others carrying the same lie. So: every endpoint, with the
// exact name it must produce.
// ---------------------------------------------------------------------------

// TestAdapter_anAbsentIdIsNamedTheSameByEveryEndpoint. A row that is not there
// is PERMANENT, so no endpoint may answer it as transient, and none may claim a
// decision exists over it.
//
// The two forbidden answers are named on purpose. `unavailable` would tell the
// operator to retry a request that will never succeed; `decided_evidence_corrupt`
// would print «the decision is recorded and sealed with its receipt» over a row
// that never existed — a receipt invented out of an absence.
//
// Probing mutation: have any store door return a bare ErrNotFound instead of
// ErrApprovalNotFound ⇒ that endpoint falls through the switch's default and
// its row reddens.
func TestAdapter_anAbsentIdIsNamedTheSameByEveryEndpoint(t *testing.T) {
	calls := map[string]func(*ApprovalsAdapter) error{
		"Detail": func(ad *ApprovalsAdapter) error {
			_, err := ad.Detail(context.Background(), "apr_nope")
			return err
		},
		"Approve": func(ad *ApprovalsAdapter) error {
			_, err := ad.Approve(context.Background(), "apr_nope", "sha256:"+staleHex)
			return err
		},
		"Reject": func(ad *ApprovalsAdapter) error {
			_, err := ad.Reject(context.Background(), "apr_nope", "no")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			cfg, store, done := parkingProfile(t)
			defer done()
			err := call(NewApprovalsAdapter(cfg, store))
			if !errors.Is(err, controlapi.ErrApprovalNotFound) {
				t.Fatalf("err = %v, want ErrApprovalNotFound", err)
			}
			if errors.Is(err, controlapi.ErrApprovalsUnavailable) {
				t.Fatalf("a row that is not there is permanent: it must never say «retry»")
			}
			if errors.Is(err, controlapi.ErrApprovalDecidedEvidenceBad) {
				t.Fatalf("no decision exists over an absent row, and its literal claims a sealed receipt")
			}
		})
	}
}

// TestAdapter_anEmptyStoreIsAnEmptyPageAndNotARefusal is the list's half of the
// same class: with nothing parked the answer is a PAGE with zero rows and its
// gate, never an error. «Nothing parked» and «something went wrong» are
// different facts and the screen paints different things for them.
//
// Probing mutation: refuse when there are no rows ⇒ this reddens.
func TestAdapter_anEmptyStoreIsAnEmptyPageAndNotARefusal(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	got, err := NewApprovalsAdapter(cfg, store).ListPending(context.Background())
	if err != nil {
		t.Fatalf("an empty store is not a failure: %v", err)
	}
	if len(got.Rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(got.Rows))
	}
	if !got.Gate.ApprovalsEnabled {
		t.Fatalf("the gate must still say the switch is on")
	}
}

// TestAdapter_aToolThatSaysNoReachesTheWindowAsFailed is the adapter's half of
// the same fact, and the one that was publishing a bare 500.
//
// What must come back is a RESULT — outcome `failed` with its receipt — and
// not an error: the run was attempted, it did not succeed, the ledger closed
// it, and that is a decided outcome the window has a literal for.
//
// HOW the run fails here, stated because the first version of this comment got
// it wrong and invented a mechanism: the parked params are `{"a":1}`, and
// webhook_call needs «URL, a space, then the JSON body», so the tool refuses
// its ARGUMENTS and no host is ever contacted. That is enough for what this
// mould watches — the outcome travels as a result and not as an error — and it
// is NOT enough to claim the effect left the window. The literal that does
// claim that is a separate finding, filed and not cured here.
//
// Probing mutation: publish it as `executed`, or send it back through the error
// channel ⇒ this reddens.
func TestAdapter_aToolThatSaysNoReachesTheWindowAsFailed(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_toolno")

	out, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if err != nil {
		t.Fatalf("a tool that says no must not surface as a failure of the call: %v", err)
	}
	if out.Outcome != "failed" {
		t.Fatalf("outcome = %q, want %q", out.Outcome, "failed")
	}
	if out.ReceiptID == "" {
		t.Fatal("the closed execution carries its receipt, failed or not")
	}
	if out.Result == "" {
		t.Fatal("the failure travels without its detail")
	}
}

// ---------------------------------------------------------------------------
// La escalera de los cuatro nombres de «¿arrancó?», uno por uno.
//
// Sin estos moldes la escalera pasaba idéntica si la rama no existiera — el
// mismo defecto que ya retiró D3 y que cazó A4. Cada nombre se fuerza con el
// estado que lo produce y se exige por su nombre.
// ---------------------------------------------------------------------------

// TestAdapter_theLadderOfDidItStart forces each rung THROUGH THE PRODUCTION
// DOOR — `Approve`, the method the HTTP route calls — and demands its name.
//
// The first version of this mould called the adapter's private nameClaim with
// an error the test handed it. It was green, and it stayed green when the whole
// routing from nameExecution to nameClaim was deleted: a mould that enters by a
// door production does not use cannot see who else walks in, and that is
// exactly how the corrupt-ledger case hid behind «the row still holds its
// parameters».
//
// The states are set up with TRIGGERS scoped to the claim's own UPDATE, so the
// decide commits normally and the claim is the thing that refuses — which is
// the real sequence, not a simulated one.
//
// Probing mutations: collapse any two rungs into one name ⇒ that row reddens;
// delete nameExecution's routing into the ladder ⇒ all of them redden, which
// the previous version did not do.
func TestAdapter_theLadderOfDidItStart(t *testing.T) {
	cases := []struct {
		name  string
		setUp func(t *testing.T, cfg *config.Config, store *actionsqlite.Store) action.Approval
		want  error
	}{
		{
			// The claim's purge is skipped and the bytes stay where they are.
			// Nothing may say they were taken.
			name: "held · the claim touched no row and the params are still there",
			setUp: func(t *testing.T, cfg *config.Config, store *actionsqlite.Store) action.Approval {
				a := parkOne(t, cfg, store, "act_held")
				attackerExec(t, store, `CREATE TRIGGER skip_purge BEFORE UPDATE OF canonical_params ON approvals
				  WHEN NEW.canonical_params = '' BEGIN SELECT RAISE(IGNORE); END`)
				return a
			},
			want: controlapi.ErrApprovalNotStartedParamsHeld,
		},
		{
			// Born WITHOUT arguments: the empty body IS what was approved, so
			// the digest re-derives over it and nothing was ever taken.
			name: "gone · the row was born without arguments and the digest re-derives",
			setUp: func(t *testing.T, cfg *config.Config, store *actionsqlite.Store) action.Approval {
				return parkOneWithParams(t, cfg, store, "act_gone", "")
			},
			want: controlapi.ErrApprovalNotStartedParamsGone,
		},
		{
			// Emptied in the window BETWEEN the decide's commit and the claim,
			// which is the race this name exists for. The trigger fires on the
			// decide's own status write, so the interleaving is exact and not
			// a lottery.
			name: "unaccounted · emptied between the decide and the claim",
			setUp: func(t *testing.T, cfg *config.Config, store *actionsqlite.Store) action.Approval {
				a := parkOne(t, cfg, store, "act_unacc")
				attackerExec(t, store, `CREATE TRIGGER steal_params AFTER UPDATE OF status ON approvals
				  WHEN NEW.status = 'APPROVED'
				  BEGIN UPDATE approvals SET canonical_params = '' WHERE approval_id = NEW.approval_id; END`)
				return a
			},
			want: controlapi.ErrApprovalParamsUnaccounted,
		},
		{
			// Corruption landing on a row whose decision is already sealed.
			// WHERE it is caught, said precisely because the first draft of
			// this comment got it wrong: the CLAIM's own read of the approval
			// row catches it, not the re-read — the claim reads that row before
			// it ever looks at the params, so a rotted timestamp never reaches
			// the ladder. The re-read's own corrupt branch is defence in depth
			// and is NOT reachable through this door; it is declared, not
			// claimed as covered.
			name: "decided_evidence_corrupt · corruption over a sealed decision",
			setUp: func(t *testing.T, cfg *config.Config, store *actionsqlite.Store) action.Approval {
				a := parkOne(t, cfg, store, "act_recorrupt")
				attackerExec(t, store, `CREATE TRIGGER rot_row AFTER UPDATE OF status ON approvals
				  WHEN NEW.status = 'APPROVED'
				  BEGIN UPDATE approvals SET canonical_params = '', requested_at = 'ayer'
				         WHERE approval_id = NEW.approval_id; END`)
				return a
			},
			want: controlapi.ErrApprovalDecidedEvidenceBad,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, store, done := parkingProfile(t)
			defer done()
			a := tc.setUp(t, cfg, store)
			_, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestAdapter_aMovedLawIsNamedByEVERYDoorTheOperatorTouches is rule 2 written
// as a mould: curing ONE door and calling the class cured is the defect this
// train has now committed twice.
//
// A moved law is PERMANENT and has its own cure — the literal tells the
// operator that rejecting still works. Published as `unavailable` it becomes
// «transient, retry», forever, and the one thing that would actually help is
// never said.
//
// Probing mutation: drop the sentinel from any of these doors ⇒ its row
// reddens.
func TestAdapter_aMovedLawIsNamedByEVERYDoorTheOperatorTouches(t *testing.T) {
	doors := map[string]func(*ApprovalsAdapter, string, string) error{
		"Detail": func(ad *ApprovalsAdapter, id, _ string) error {
			_, err := ad.Detail(context.Background(), id)
			return err
		},
		"Approve": func(ad *ApprovalsAdapter, id, digest string) error {
			_, err := ad.Approve(context.Background(), id, digest)
			return err
		},
	}
	for name, call := range doors {
		t.Run(name, func(t *testing.T) {
			cfg, store, done := parkingProfile(t)
			defer done()
			a := parkOne(t, cfg, store, "act_movedlaw")
			moveTheLaw(t, cfg)
			err := call(NewApprovalsAdapter(cfg, store), a.ApprovalID, a.ActionDigest)
			if !errors.Is(err, controlapi.ErrApprovalInvalidated) {
				t.Fatalf("err = %v, want ErrApprovalInvalidated", err)
			}
			if errors.Is(err, controlapi.ErrApprovalsUnavailable) {
				t.Fatal("a moved law is permanent: telling the operator to retry hides the only cure there is")
			}
			var moved interface{ CurrentLawDigest() string }
			if !errors.As(err, &moved) || moved.CurrentLawDigest() == "" {
				t.Fatal("the current law must travel in a field of its own, by every door")
			}
			// Oracle by impossibility: refusing under a moved law consumes nothing.
			after, serr := store.ApprovalStatusOf(context.Background(), a.ApprovalID)
			if serr != nil {
				t.Fatalf("re-read: %v", serr)
			}
			if after != action.ApprovalPending {
				t.Fatalf("status = %q, want PENDING", after)
			}
		})
	}
}

// TestAdapter_aCorruptActionRecordIsNotABenignNonStart closes the hole the
// third pass found: the parked action's own row failing to read, AFTER the
// decide has committed, was published as «the row still holds its parameters,
// so no automatic pass has closed it» — a benign sentence over a permanently
// broken ledger, chosen by re-reading the APPROVALS row to name a failure in
// the ACTIONS row.
//
// Nothing on the read path notices this corruption: GetApproval never selects
// actions.requested_at and the story belt selects four other columns. So the
// decide commits and the failure lands on the execution path.
//
// Probing mutation: drop ErrApprovalRecordUnreadable from store.Get's wrap ⇒
// the refusal falls into the ladder and this reddens.
func TestAdapter_aCorruptActionRecordIsNotABenignNonStart(t *testing.T) {
	cfg, store, done := parkingProfile(t)
	defer done()
	a := parkOne(t, cfg, store, "act_ledgerrot")
	attackerExec(t, store, `UPDATE actions SET requested_at='ayer' WHERE action_id=?`, "act_ledgerrot")

	_, err := NewApprovalsAdapter(cfg, store).Approve(context.Background(), a.ApprovalID, a.ActionDigest)
	if !errors.Is(err, controlapi.ErrApprovalDecidedEvidenceBad) {
		t.Fatalf("err = %v, want ErrApprovalDecidedEvidenceBad", err)
	}
	if errors.Is(err, controlapi.ErrApprovalNotStartedParamsHeld) {
		t.Fatal("a permanently unreadable ledger must never be reported as a quiet non-start")
	}
}
