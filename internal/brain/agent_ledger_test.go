// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The live ink on the hot path — Etapa 4, lote 3, pieza 2 (spec FR-LED,
// NC-3): on SUCCESS the brain computes the result digest ON THE FLY over
// the observation (action.HashCanonical) and closes through the OPTIONAL
// FinishWithResult extension of the ActionRecorder seam; on FAILURE the
// digest is honestly empty; a recorder without the extension keeps the
// plain Finish path verbatim. The raw observation NEVER travels to the
// recorder — only its digest does. Approved-red contract.

package brain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/tool"
)

// resultFakeRecorder implements the OPTIONAL FinishWithResult extension
// on top of the plain fakeRecorder, capturing the digests it receives.
type resultFakeRecorder struct {
	fakeRecorder
	digests []string
}

func (f *resultFakeRecorder) FinishWithResult(ctx context.Context, actionID string, to action.State, at time.Time, resultDigest string) error {
	*f.journal = append(*f.journal, "finish_with_result:"+string(to))
	f.finishes = append(f.finishes, to)
	f.digests = append(f.digests, resultDigest)
	return nil
}

func ledgerHarness(t *testing.T, toolErr error) (*AgentBrain, *resultFakeRecorder, *[]string) {
	t.Helper()
	journal := &[]string{}
	rec := &resultFakeRecorder{fakeRecorder: fakeRecorder{journal: journal}}
	a := NewAgentBrain(
		&scriptedModel{},
		tool.Registry{"journal": &journalTool{journal: journal, err: toolErr}},
		WithActionRecorder(rec),
	)
	return a, rec, journal
}

func TestRunTool_successClosesWithTheResultDigestComputedOnTheFly(t *testing.T) {
	t.Parallel()
	a, rec, journal := ledgerHarness(t, nil)
	env := kernelEnv()
	out := a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	if out != "done" {
		t.Fatalf("observation: %q", out)
	}
	if len(rec.digests) != 1 {
		t.Fatalf("success must close through FinishWithResult exactly once, digests=%v journal=%v", rec.digests, *journal)
	}
	want := action.HashCanonical("done")
	if rec.digests[0] != want {
		t.Fatalf("the digest is computed on the fly over the OBSERVATION: got %q want %q", rec.digests[0], want)
	}
	if rec.finishes[0] != action.StateSucceeded {
		t.Fatalf("terminal state: %v", rec.finishes[0])
	}
}

func TestRunTool_failureClosesWithAnHonestlyEmptyDigest(t *testing.T) {
	t.Parallel()
	a, rec, _ := ledgerHarness(t, errors.New("tool exploded"))
	env := kernelEnv()
	_ = a.runTool(context.Background(), env, nil, laneText, "journal", `{}`)
	if len(rec.digests) != 1 || rec.digests[0] != "" {
		t.Fatalf("FAILED closes with an EMPTY digest — there is no result to attest: %v", rec.digests)
	}
	if rec.finishes[0] != action.StateFailed {
		t.Fatalf("terminal state: %v", rec.finishes[0])
	}
}

func TestRunTool_recorderWithoutTheExtensionKeepsPlainFinish(t *testing.T) {
	t.Parallel()
	a, rec, journal := kernelHarness(t, nil, nil)
	env := kernelEnv()
	if out := a.runTool(context.Background(), env, nil, laneText, "journal", `{}`); out != "done" {
		t.Fatalf("observation: %q", out)
	}
	if len(rec.finishes) != 1 || rec.finishes[0] != action.StateSucceeded {
		t.Fatalf("a recorder without the extension closes through plain Finish: %v %v", rec.finishes, *journal)
	}
}
