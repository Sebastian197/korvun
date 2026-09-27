// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · «¿Qué pasa hoy?» — THE REFUSALS.
//
// These moulds exist because the internal adversarial pass over this train's own
// diff found four ways a button could make a change WIDER than the one the
// operator confirmed, or a change to something they were not looking at:
//
//   - `lift-shadow` turned an explicit `deny` into `allow` behind a confirmation
//     whose words are about a SHADOW («que pueda ejecutarse tras mi
//     aprobación»);
//   - `set-ceiling` with no brain named rewrote whichever brain the profile
//     happens to list first;
//   - `set-ceiling` on a brain already at `critical` LOWERED the ceiling, which
//     widens what runs without an approval — a security regression served by a
//     button labelled as a protection;
//   - no door ran `config.Validate` or the self-lock guard that the door beside
//     them, `POST /api/config`, has run since Stage 14.
//
// Every mould here forces the dangerous branch and proves TWO things: the named
// refusal, and that the reloader was never asked. The second is the oracle by
// impossibility the doctrine demands for «cero escrituras» — a counter on the
// seam, not a comparison of a file afterwards, because a supervisor that was
// never called cannot have written anything by any path.
//
// Evidence level, honest: in-process, over a real httptest server and a fake
// reloader. Not a real supervisor, not a binary, no disk.

package controlapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// shapedActions is fakeActions over a profile the test chose.
type shapedActions struct {
	fakeActions
	profile *config.Config
}

func (s *shapedActions) CurrentConfig() *config.Config { return s.profile }

func shapedServer(t *testing.T, profile *config.Config) (*httptest.Server, *shapedActions) {
	t.Helper()
	s := &shapedActions{profile: profile}
	mux := http.NewServeMux()
	registerWhats(mux, s)
	return serve(t, mux), s
}

// refused reads the outcome and demands the refusal shape: nothing applied, the
// profile untouched, and a detail the operator can act on.
func refused(t *testing.T, resp *http.Response, wantStatus int, wantOutcome, mustSay string) {
	t.Helper()
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d", resp.StatusCode, wantStatus)
	}
	var out controlapi.WhatsHappeningOutcome
	decodeInto(t, resp, &out)
	if out.Outcome != wantOutcome {
		t.Fatalf("outcome = %q, want %q (detail: %s)", out.Outcome, wantOutcome, out.Detail)
	}
	if out.Applied {
		t.Error("a refused change reports applied")
	}
	if !out.ProfileUnchanged {
		t.Error("a refused change does not say the profile is unchanged")
	}
	if !strings.Contains(out.Detail, mustSay) {
		t.Errorf("the refusal says %q, which does not name %q", out.Detail, mustSay)
	}
}

// TestRefusal_liftShadowWillNotLiftADeny is the P2 the internal pass named
// first. The door's confirmation text promises one thing; turning a `deny` into
// `allow` is another.
//
// PROBING MUTATION (executed): delete the `g.Mode != "shadow"` branch in
// liftShadow. The door then answers 200, the reloader is asked once, and this
// mould reddens on both halves.
func TestRefusal_liftShadowWillNotLiftADeny(t *testing.T) {
	withAdminToken(t)
	profile := whatsHappeningProfile()
	profile.Brains[0].Agent.Governance = []config.ToolGrantConfig{
		{Tool: "webhook_call", Mode: "deny"},
	}
	srv, s := shapedServer(t, profile)

	resp := post(t, srv, "/api/whats-happening/lift-shadow", whatsToken,
		`{"confirm":true,"brain":"default","tool":"webhook_call"}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused, `"deny"`)
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times: a refused edit must never reach the supervisor", s.requested)
	}
	if profile.Brains[0].Agent.Governance[0].Mode != "deny" {
		t.Fatalf("the live governance became %q", profile.Brains[0].Agent.Governance[0].Mode)
	}
}

// TestRefusal_setCeilingNeedsItsBrainNamed: an unnamed brain used to mean «the
// first one». The profile here carries TWO agent brains so the danger is
// visible rather than argued: under the original code the click lands on
// `primero`, which is not the brain anyone asked about, and the mould can see
// the damage instead of only the message.
//
// PROBING MUTATION (executed, M10-bis): restore brainFor's original predicate,
// `if name == "" || c.Brains[i].Name == name`. The door then answers 200 having
// raised the ceiling of `primero`, and this reddens on the status, on the
// counter and on the untouched-profile check.
func TestRefusal_setCeilingNeedsItsBrainNamed(t *testing.T) {
	withAdminToken(t)
	profile := whatsHappeningProfile()
	first := whatsHappeningBrain(&config.AgentConfig{Tools: []string{"webhook_call"}})
	first.Name = "primero"
	profile.Brains = append([]config.BrainConfig{first}, profile.Brains...)
	srv, s := shapedServer(t, profile)

	resp := post(t, srv, "/api/whats-happening/set-ceiling", whatsToken, `{"confirm":true}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused, "nombre del cerebro")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times", s.requested)
	}
	if got := profile.Brains[0].Agent.EffectCeiling; got != "" {
		t.Fatalf("the ceiling of %q became %q: the click landed on a brain nobody named",
			profile.Brains[0].Name, got)
	}
}

// TestRefusal_setCeilingNeverComesDown is the sharpest of the four: `critical`
// ranks ABOVE `write_irreversible`, so writing the ceiling LOWERS it and widens
// what executes without an approval. A button sold as a protection would have
// removed one.
//
// PROBING MUTATION (executed): delete the rank comparison. The door answers 200,
// the ceiling becomes write_irreversible, and this reddens naming both values.
func TestRefusal_setCeilingNeverComesDown(t *testing.T) {
	withAdminToken(t)
	profile := whatsHappeningProfile()
	profile.Brains[0].Agent.EffectCeiling = "critical"
	srv, s := shapedServer(t, profile)

	resp := post(t, srv, "/api/whats-happening/set-ceiling", whatsToken,
		`{"confirm":true,"brain":"default"}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused, "critical")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times: lowering a ceiling must never reach the supervisor", s.requested)
	}
	if got := profile.Brains[0].Agent.EffectCeiling; got != "critical" {
		t.Fatalf("the live ceiling became %q, want critical", got)
	}
}

// TestRefusal_setCeilingWillNotOverwriteACeilingItCannotPlace: a ceiling this
// build cannot rank is not one to overwrite silently. Fail closed — the unknown
// value may sit above write_irreversible, and guessing is how a downgrade ships.
//
// It asserts the UNKNOWN refusal's own words, not merely that something was
// refused. The first version of this mould checked only that the detail carried
// the offending value, and its probing mutation left it GREEN: with the
// `!cur.Known()` case disabled, an unrankable ceiling falls through to the rank
// comparison, where `unknownEffectRank` sits above `critical` and refuses it
// anyway. The mould passed for a reason that had nothing to do with the branch
// it claimed to watch — the finding is the mould, exactly as the doctrine says.
// Which sentence the operator reads is not cosmetic: one tells them their
// ceiling is already high enough, the other tells them this build cannot read
// their ceiling at all, and only the second is true here.
//
// PROBING MUTATION (executed, M12-bis): drop the `!cur.Known()` case. The
// refusal then arrives in the rank branch's words and this reddens on the
// sentence.
func TestRefusal_setCeilingWillNotOverwriteACeilingItCannotPlace(t *testing.T) {
	withAdminToken(t)
	profile := whatsHappeningProfile()
	profile.Brains[0].Agent.EffectCeiling = "write_apocalyptic"
	srv, s := shapedServer(t, profile)

	resp := post(t, srv, "/api/whats-happening/set-ceiling", whatsToken,
		`{"confirm":true,"brain":"default"}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused,
		"no es una clase de efecto que esta versión reconozca")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times", s.requested)
	}
}

// TestRefusal_aChangeThatWouldLockTheOperatorOut: F11, the guard the door beside
// this one has had since Stage 14 and these doors shipped without. A config
// whose admin token does not resolve leaves Korvun refusing its own admin calls
// — the screen loses every button, and only a hand edit of the file brings them
// back.
//
// The env var is deliberately NOT set here, which is the whole scenario:
// `config.Validate` passes (it only demands the NAME be present), so this
// reaches the self-lock guard rather than being caught earlier for another
// reason.
//
// WHAT THIS MOULD DOES NOT PROVE, said because the official pass was right to
// ask: none of the FOUR current edits touches `Admin`, so no button in
// production can reach this branch — if the token were already unresolvable the
// doors would not be mounted. This drives the handler directly, which is the
// only way in. The guard is defence in depth against a fifth door, and counting
// it as coverage of a live risk would be the wider-than-the-wire claim this
// train has already had to retract once.
//
// PROBING MUTATION (executed): delete the wouldSelfLock branch. The door answers
// 200, the reloader is asked, and this reddens on both.
func TestRefusal_aChangeThatWouldLockTheOperatorOut(t *testing.T) {
	profile := whatsHappeningProfile()
	profile.Admin = &config.AdminConfig{TokenEnv: "KORVUN_TEST_TOKEN_THAT_IS_NOT_SET"}
	srv, s := shapedServer(t, profile)

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	refused(t, resp, http.StatusConflict, controlapi.OutcomeWouldSelfLock, "token de administración")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times: a self-locking config must never reach the supervisor", s.requested)
	}
}

// TestRefusal_anInvalidProfileIsCaughtBeforeTheCutover: the button edits a
// config the OPERATOR wrote, which can carry a violation the button never
// introduced. A cutover is the wrong place to find out.
//
// PROBING MUTATION (executed): delete the `next.Validate()` branch. The invalid
// config then reaches RequestReload and this reddens on the counter.
func TestRefusal_anInvalidProfileIsCaughtBeforeTheCutover(t *testing.T) {
	withAdminToken(t)
	profile := whatsHappeningProfile()
	profile.Brains[0].Sensitivity = ""
	srv, s := shapedServer(t, profile)

	resp := post(t, srv, "/api/whats-happening/enable-approvals", whatsToken, `{"confirm":true}`)

	refused(t, resp, http.StatusUnprocessableEntity, controlapi.OutcomeRefused, "sensitivity")
	if s.requested != 0 {
		t.Fatalf("the reloader was asked %d times: an invalid config must never reach the supervisor", s.requested)
	}
}

// TestRead_theScreenCanReadItsRowsWithoutAnActionStore is the read door's reason
// to exist. `/api/approvals` needs an open action store, and the profile with NO
// store is exactly the one whose first contract row explains why nothing can
// park. A screen that went dark there would go dark where it is most needed.
//
// PROBING MUTATION (executed): unmount the GET route. The screen 404s and this
// reddens.
func TestRead_theScreenCanReadItsRowsWithoutAnActionStore(t *testing.T) {
	withAdminToken(t)
	srv, _ := shapedServer(t, whatsHappeningProfile())

	resp, err := http.Get(srv.URL + "/api/whats-happening")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// The rows are behind the same bearer as everything else on this surface.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated read got %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/whats-happening", nil)
	req.Header.Set("Authorization", "Bearer "+whatsToken)
	authed, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with bearer: %v", err)
	}
	defer func() { _ = authed.Body.Close() }()
	if authed.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", authed.StatusCode)
	}
	var body struct {
		Rows []controlapi.ScreenRow `json:"rows"`
	}
	decodeInto(t, authed, &body)
	if len(body.Rows) != len(controlapi.ScreenRows()) {
		t.Fatalf("the door served %d rows and the contract has %d", len(body.Rows), len(controlapi.ScreenRows()))
	}
	// The state this mould is about must be one the screen can name, and the
	// check reads the SERVED rows rather than a package helper: the operator sees
	// what the door sent, not what the package knows.
	hasStoreRow := false
	for _, r := range body.Rows {
		if r.Rule == "store" {
			hasStoreRow = true
		}
	}
	if !hasStoreRow {
		t.Fatal("the door served no row for the action store, so the profile without one has nothing to read")
	}
}
