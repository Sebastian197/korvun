// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The seam wakes — Etapa 5, lote 3, pieza 1 (spec FR-GATE, sealed):
// when the recorder carries the OPTIONAL RequestApproval extension
// (wired by the app ONLY when approvals.enabled), the gate's honest
// approval_unavailable becomes a full request: the action parks with
// its preview and the model receives an honest pending observation —
// the conversation never blocks on a human. Without the extension the
// E3 behavior is byte-for-byte (THE SACRED PIN at this layer).
// Precedence §13.3 untouched: the ceiling still rules first; SHADOWED
// never touches this path; a failed request falls CLOSED back to the
// honest no. Approved-red contract.

package brain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// requestingRecorder implements the optional approval-request extension.
type requestingRecorder struct {
	fakeRecorder
	requests []action.Envelope
	rawArgs  []string
	reqRules []string
	fail     error
}

func (r *requestingRecorder) RequestApproval(ctx context.Context, env action.Envelope, rule string, rawParams string) (string, error) {
	if r.fail != nil {
		return "", r.fail
	}
	r.requests = append(r.requests, env)
	r.rawArgs = append(r.rawArgs, rawParams)
	r.reqRules = append(r.reqRules, rule)
	return "apr_testrequest0000", nil
}

// irreversibleTool is a journal tool declared write_irreversible.
func approvalHarness(t *testing.T, rec ActionRecorder) (*AgentBrain, *[]string) {
	t.Helper()
	journal := &[]string{}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithAgentName("apr"),
		WithActionRecorder(rec),
		WithEffectClassifier(func(name string) (action.EffectDescriptor, bool) {
			return action.EffectDescriptor{Class: action.EffectWriteIrreversible}, true
		}),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID:      action.RootIntentID,
			GrantID:       "grant_test",
			EffectCeiling: action.EffectWriteIrreversible, // bounded: approval demanded
		}),
	)
	return a, journal
}

func TestGate_theArmWakes_requestInsteadOfDenial(t *testing.T) {
	t.Parallel()
	journal := &[]string{}
	rec := &requestingRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a, toolJournal := approvalHarness(t, rec)
	_ = toolJournal
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{"note":"irreversible"}`)
	// The observation is HONEST about the pending human: it names the
	// request and never pretends execution or failure.
	if !strings.Contains(out, "apr_testrequest0000") {
		t.Fatalf("the observation must name the pending request: %q", out)
	}
	if strings.Contains(out, "not permitted") || strings.Contains(out, "failed") {
		t.Fatalf("pending is neither a denial nor a failure: %q", out)
	}
	// The request carried the full envelope and the RAW args (the store
	// needs them for the born-whole birth), under the require_approval rule.
	if len(rec.requests) != 1 || rec.reqRules[0] != "require_approval" {
		t.Fatalf("one request under require_approval: %+v %+v", rec.requests, rec.reqRules)
	}
	if rec.rawArgs[0] != `{"note":"irreversible"}` {
		t.Fatalf("the raw args must travel to the birth: %q", rec.rawArgs[0])
	}
	// NOTHING executed and NOTHING recorded as denied.
	for _, entry := range *journal {
		if entry == "execute" || strings.HasPrefix(entry, "record:") {
			t.Fatalf("no execution, no denial record — journal: %v", *journal)
		}
	}
}

func TestGate_theSacredPin_withoutTheExtensionE3ByteForByte(t *testing.T) {
	t.Parallel()
	journal := &[]string{}
	rec := &fakeRecorder{journal: journal}
	a, _ := approvalHarness(t, rec)
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("without the extension the honest no stands byte-for-byte: %q", out)
	}
	if len(rec.rules) != 1 || rec.rules[0] != "approval_unavailable" {
		t.Fatalf("the E3 rule stands: %+v", rec.rules)
	}
	if len(rec.states) != 1 || rec.states[0] != action.StateDenied {
		t.Fatalf("the E3 denial stands: %+v", rec.states)
	}
}

func TestGate_theCeilingStillRulesFirst(t *testing.T) {
	t.Parallel()
	journal := &[]string{}
	rec := &requestingRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal}},
		WithAgentName("apr"),
		WithActionRecorder(rec),
		WithEffectClassifier(func(name string) (action.EffectDescriptor, bool) {
			return action.EffectDescriptor{Class: action.EffectCritical}, true
		}),
		WithActionIdentity(ActionIdentity{
			Registry: action.ProvenanceRegistry{
				"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
			},
			IntentID:      action.RootIntentID,
			GrantID:       "grant_test",
			EffectCeiling: action.EffectReadExternal, // critical EXCEEDS the ceiling
		}),
	)
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("§13.3: the ceiling rules FIRST — denial, never a request: %q", out)
	}
	if len(rec.requests) != 0 {
		t.Fatal("a ceiling violation must NEVER become an approval request")
	}
	if len(rec.rules) != 1 || rec.rules[0] != "effect_ceiling" {
		t.Fatalf("the ceiling rule stands: %+v", rec.rules)
	}
}

func TestGate_aFailedRequestFallsClosedToTheHonestNo(t *testing.T) {
	t.Parallel()
	journal := &[]string{}
	rec := &requestingRecorder{fakeRecorder: fakeRecorder{journal: journal}, fail: errors.New("store on fire")}
	a, _ := approvalHarness(t, rec)
	out := a.runTool(context.Background(), kernelEnv(), nil, laneText, "journal", `{}`)
	if out != deniedObservation("journal") {
		t.Fatalf("a failed birth falls CLOSED to the denial: %q", out)
	}
	if len(rec.rules) != 1 || rec.rules[0] != "approval_unavailable" {
		t.Fatalf("the fallback denial carries the honest rule: %+v", rec.rules)
	}
}

func TestGate_shadowNeverTouchesTheApprovalPath(t *testing.T) {
	t.Parallel()
	journal := &[]string{}
	rec := &requestingRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a, _ := approvalHarness(t, rec)
	decisions := map[string]policy.ToolDecision{"journal": {Mode: policy.ToolShadow}}
	out := a.runTool(context.Background(), kernelEnv(), decisions, laneText, "journal", `{}`)
	if !strings.Contains(out, "REHEARSAL") {
		t.Fatalf("shadow keeps its rehearsal observation: %q", out)
	}
	if len(rec.requests) != 0 {
		t.Fatal("SHADOWED must never create approval requests")
	}
	for _, entry := range *journal {
		if entry == "execute" {
			t.Fatal("shadow NEVER executes")
		}
	}
}

// The pending observation must be reproducible for the mold: verify it
// tells the model the truth (pending, not done, no manual offer).
func TestGate_pendingObservationSpeaksTheTruth(t *testing.T) {
	t.Parallel()
	obs := pendingApprovalObservation("journal", "apr_abc123", time.Time{})
	for _, must := range []string{"journal", "apr_abc123", "approval"} {
		if !strings.Contains(obs, must) {
			t.Fatalf("observation must mention %q: %q", must, obs)
		}
	}
	if strings.Contains(strings.ToLower(obs), "error") {
		t.Fatalf("pending is not an error: %q", obs)
	}
}
