// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The identity-aware adapter contract (Etapa 2, lote 4, spec FR-ENV-1 +
// FR-EVID-2 live + AS-1 end-to-end): every hot-path action fills its
// identity refs — the brain as agent_brain answering to the operator,
// the channel's evidence with its subject riding the SAME record call —
// while the receipt digest, the observations and every E1 behavior stay
// byte-identical. Unknown provenance fails CLOSED on the authorized
// path. Approved-red contract; ZERO existing tests touched.

package brain

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// identifiedFakeRecorder captures BOTH seams: the identified calls and
// any legacy fallback calls, so the tests can assert which path ran.
type identifiedFakeRecorder struct {
	fakeRecorder
	identifiedEnvs  []action.Envelope
	identifiedRules []string
	evidences       []action.IdentityEvidence
	identifiedErr   error
}

func (f *identifiedFakeRecorder) RecordAttemptIdentified(ctx context.Context, env action.Envelope, outcome, rule string, state action.State, ev action.IdentityEvidence) error {
	if f.identifiedErr != nil {
		return f.identifiedErr
	}
	*f.journal = append(*f.journal, "record-identified:"+string(state))
	f.identifiedEnvs = append(f.identifiedEnvs, env)
	f.identifiedRules = append(f.identifiedRules, rule)
	f.evidences = append(f.evidences, ev)
	return nil
}

func identityHarness(t *testing.T) (*AgentBrain, *identifiedFakeRecorder, *[]string) {
	t.Helper()
	journal := &[]string{}
	rec := &identifiedFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	registry := action.ProvenanceRegistry{
		"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
		"webhook": {Class: "webhook", Credential: action.CredentialInboundBearer},
	}
	grant := action.DeriveConfigGrant("asistente", []string{"journal"}, []string{"*"})
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithAgentName("asistente"),
		WithActionRecorder(rec),
		WithActionIdentity(ActionIdentity{
			Registry: registry,
			IntentID: action.RootIntentID,
			GrantID:  grant.GrantID,
		}),
	)
	return a, rec, journal
}

// TestIdentity_identifiedActionCarriesTheFullChain: a governed allow on
// the console records the brain principal (answering to the operator),
// the root intent, the derived grant as the explaining authority, and
// the console evidence with the sender as subject — with the parameters
// digest byte-identical to the E1 form.
func TestIdentity_identifiedActionCarriesTheFullChain(t *testing.T) {
	t.Parallel()
	a, rec, journal := identityHarness(t)
	env := &envelope.Envelope{ID: "env-i", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	decisions := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolAllow}}
	out := a.runTool(context.Background(), env, decisions, laneText, "journal", `{"a":1}`)
	if out != "done" {
		t.Fatalf("observation = %q — the outside must not move", out)
	}
	want := []string{"record-identified:AUTHORIZED", "execute", "finish:SUCCEEDED"}
	if strings.Join(*journal, ",") != strings.Join(want, ",") {
		t.Fatalf("record-before-effect holds on the identified path too: %v", *journal)
	}
	e := rec.identifiedEnvs[0]
	if e.Principal.PrincipalID != action.BrainPrincipal("asistente").PrincipalID {
		t.Fatalf("the acting principal is the brain, got %q", e.Principal.PrincipalID)
	}
	if e.Principal.ResponsibleHumanID != action.OperatorPrincipal().PrincipalID {
		t.Fatalf("§14.2: the brain answers to the operator, got %q", e.Principal.ResponsibleHumanID)
	}
	if e.IntentID != action.RootIntentID {
		t.Fatalf("today's flows record under the root intent, got %q", e.IntentID)
	}
	if len(e.AuthorityRefs) != 1 || !strings.HasPrefix(e.AuthorityRefs[0], "grant_cfg_") {
		t.Fatalf("the granted rule gains its explaining derived grant, got %v", e.AuthorityRefs)
	}
	if rec.identifiedRules[0] != "granted" {
		t.Fatalf("rule = %q", rec.identifiedRules[0])
	}
	ev := rec.evidences[0]
	if ev.Provider != "console" || ev.Credential != action.CredentialLoopbackInProcess {
		t.Fatalf("console evidence: %+v", ev)
	}
	if ev.Subject != "console-user" || ev.TransportBinding != "console" {
		t.Fatalf("the sender survives as the evidence subject, got %+v", ev)
	}
	if e.Principal.EvidenceID != ev.EvidenceID {
		t.Fatal("the envelope must reference THIS attempt's evidence")
	}
	// FR-ENV-2 live: the digest is the E1 algorithm, untouched.
	if e.ParametersDigest != action.Digest(e.Operation, `{"a":1}`) {
		t.Fatal("receipt compatibility: the digest must remain the E1 form")
	}
}

// TestIdentity_forgedSenderEndToEnd is AS-1 on the recorded row: a
// webhook sender claiming the operator's principal id records under the
// brain principal with webhook evidence — the claim survives ONLY as the
// evidence subject.
func TestIdentity_forgedSenderEndToEnd(t *testing.T) {
	t.Parallel()
	a, rec, _ := identityHarness(t)
	forged := &envelope.Envelope{ID: "env-f", Channel: "webhook",
		Sender: envelope.Participant{ID: action.OperatorPrincipal().PrincipalID}}
	_ = a.runTool(context.Background(), forged, nil, laneText, "journal", `{}`)
	e := rec.identifiedEnvs[0]
	if e.Principal.PrincipalID == action.OperatorPrincipal().PrincipalID {
		t.Fatal("AS-1: a forged Sender must NEVER mint the operator on the recorded row")
	}
	ev := rec.evidences[0]
	if ev.Credential != action.CredentialInboundBearer || ev.Provider != "webhook" {
		t.Fatalf("the evidence names THAT channel's transport, got %+v", ev)
	}
	if ev.Subject != action.OperatorPrincipal().PrincipalID {
		t.Fatalf("the forged claim survives only as the subject, got %q", ev.Subject)
	}
}

// TestIdentity_ungovernedRecordsNoAuthorityRefs: today's ungoverned
// allow acts directly under the root's standing authority — no grant to
// name.
func TestIdentity_ungovernedRecordsNoAuthorityRefs(t *testing.T) {
	t.Parallel()
	a, rec, _ := identityHarness(t)
	env := &envelope.Envelope{ID: "env-u", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	_ = a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	e := rec.identifiedEnvs[0]
	if len(e.AuthorityRefs) != 0 {
		t.Fatalf("ungoverned acts under the root directly, got refs %v", e.AuthorityRefs)
	}
	if rec.identifiedRules[0] != "ungoverned" {
		t.Fatalf("rule = %q", rec.identifiedRules[0])
	}
}

// TestIdentity_unknownProvenanceFailsClosed: a channel absent from the
// registry cannot produce the identified record, so the authorized path
// fails CLOSED — the tool never executes, the audit rule is the finite
// record_failed, and NO principal is invented anywhere.
func TestIdentity_unknownProvenanceFailsClosed(t *testing.T) {
	t.Parallel()
	a, rec, journal := identityHarness(t)
	ghost := &envelope.Envelope{ID: "env-g", Channel: "ghost-channel",
		Sender: envelope.Participant{ID: "x"}}
	out := a.runTool(context.Background(), ghost, nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("unknown provenance must deny, got %q", out)
	}
	for _, step := range *journal {
		if step == "execute" {
			t.Fatal("the tool must NEVER execute under unknown provenance")
		}
	}
	if len(rec.identifiedEnvs) != 0 || len(rec.envs) != 0 {
		t.Fatal("no record may land with an invented principal")
	}
}

// TestIdentity_deniedAndShadowedRecordIdentifiedToo: the best-effort
// terminal outcomes carry identity as well, with no authority refs (a
// denial has no explaining grant).
func TestIdentity_deniedAndShadowedRecordIdentifiedToo(t *testing.T) {
	t.Parallel()
	a, rec, _ := identityHarness(t)
	env := &envelope.Envelope{ID: "env-d", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	decisions := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolShadow}}
	_ = a.runTool(context.Background(), env, decisions, laneText, "journal", `{}`)
	if len(rec.identifiedEnvs) != 1 {
		t.Fatalf("shadowed must record identified, got %d", len(rec.identifiedEnvs))
	}
	if len(rec.identifiedEnvs[0].AuthorityRefs) != 0 {
		t.Fatalf("a non-executed outcome names no authority, got %v", rec.identifiedEnvs[0].AuthorityRefs)
	}
}

// TestIdentity_legacyRecorderKeepsWorking: identity configured over a
// recorder WITHOUT the identified seam falls back to the E1 path —
// wiring skew degrades honestly, never breaks the hot path.
func TestIdentity_legacyRecorderKeepsWorking(t *testing.T) {
	t.Parallel()
	journal := &[]string{}
	rec := &fakeRecorder{journal: journal}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithAgentName("asistente"),
		WithActionRecorder(rec),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID: action.RootIntentID,
		}),
	)
	env := &envelope.Envelope{ID: "env-l", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	out := a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	if out != "done" {
		t.Fatalf("legacy fallback must keep the hot path alive, got %q", out)
	}
	if len(rec.envs) != 1 {
		t.Fatalf("the E1 seam must have recorded, got %d", len(rec.envs))
	}
}

// TestIdentity_identifiedRecordFailureFailsClosed: the identified store
// failing on the authorized path refuses execution — record_failed, the
// E1 fail-closed law on the identified seam.
func TestIdentity_identifiedRecordFailureFailsClosed(t *testing.T) {
	t.Parallel()
	a, rec, journal := identityHarness(t)
	rec.identifiedErr = context.DeadlineExceeded
	env := &envelope.Envelope{ID: "env-e", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	out := a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("an unrecordable identified attempt must deny, got %q", out)
	}
	for _, step := range *journal {
		if step == "execute" {
			t.Fatal("proof is part of execution on the identified seam too")
		}
	}
}

// TestIdentity_evidenceIsFreshPerAttempt: two attempts mint two distinct
// evidence rows (per-attempt EvidenceID), both referenced by their own
// envelope.
func TestIdentity_evidenceIsFreshPerAttempt(t *testing.T) {
	t.Parallel()
	a, rec, _ := identityHarness(t)
	env := &envelope.Envelope{ID: "env-2", Channel: "console",
		Sender: envelope.Participant{ID: "console-user"}}
	_ = a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	_ = a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	if len(rec.evidences) != 2 {
		t.Fatalf("two attempts, two evidences, got %d", len(rec.evidences))
	}
	if rec.evidences[0].EvidenceID == rec.evidences[1].EvidenceID {
		t.Fatal("evidence is minted per attempt, never shared")
	}
	if !rec.evidences[0].IssuedAt.Equal(rec.evidences[0].IssuedAt.UTC()) {
		t.Fatal("evidence instants are UTC")
	}
}
