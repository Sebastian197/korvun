// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · train E, the pre-PR gate, finding B: the Go half of the screen's
// terminal-state mould.
//
// The packaged pass (TE50) caught the screen saying «Hecho» and, right after
// it, the applying answer's «aplicando; tu perfil en disco todavía no ha
// cambiado» over a change that had been applied and saved. The jsdom moulds had
// never seen it: their fake applying answers carried no `detail`, and the real
// door always sends one. A fake that is not the wire proves only itself.
//
// So the bodies the screen's mould (WhatsHappening.terminal.test.tsx) stands up
// are taken HERE from the real handlers, with the supervisor held in `pending`,
// and compared with the fixtures that mould reads: the ordinary door, and the
// founding door, whose applying detail also names the ledger's path. Each body
// is decoded and re-encoded with two-space indentation (keys in alphabetical
// order), as the TE49 fixtures are, and must equal its fixture byte for byte.
//
// Evidence level: in-process over a real httptest server with a fake reloader
// and a fake ledger. The handlers, outcomeFor and the JSON encoding are the
// production ones; the cutover and the ledger are not.
//
// PROBING MUTATION: change the applying detail in outcomeFor. Both bodies then
// differ from their fixtures and this reddens.
//
// After a deliberate change of the wire, rewrite the fixtures with
// KORVUN_B_UPDATE_GOLDEN=1 and review the diff: they are the screen's input.

package controlapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// bGoldenDir is where the screen's moulds read their fixtures.
var bGoldenDir = filepath.Join("..", "..", "cmd", "korvun-desktop", "frontend", "src", "views", "fixtures")

// bGolden compares a real body with its fixture, or rewrites the fixture when
// KORVUN_B_UPDATE_GOLDEN=1.
func bGolden(t *testing.T, name string, body []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, body)
	}
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	file := filepath.Join(bGoldenDir, name+".json")
	if os.Getenv("KORVUN_B_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(file, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file) //nolint:gosec // G304: the repository's own fixture
	if err != nil {
		t.Fatalf("the reference body: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the real applying answer differs from %s\n--- real\n%s\n--- reference\n%s", file, got, want)
	}
}

// TestB_theApplyingAnswersAreWhatTheScreenStandsUp · the two applying answers
// the screen's terminal-state mould polls from are the real doors' answers.
func TestB_theApplyingAnswersAreWhatTheScreenStandsUp(t *testing.T) {
	for _, c := range []struct {
		fixture string
		door    string
		body    string
		profile func() *config.Config
		path    string
	}{
		{"whats-happening-applying", "enable-approvals", `{"confirm":true}`, actProfile, ""},
		{"whats-happening-applying-founding", "enable-storage", `{}`, storelessProfile, "/perfil/korvun.db"},
	} {
		t.Run(c.door, func(t *testing.T) {
			withAdminToken(t)
			acts := newExternalFakeActs()
			acts.createdPath = c.path
			srv, _ := actServer(t, c.profile(), supervisor.StatePending, acts)

			resp := post(t, srv, "/api/whats-happening/"+c.door, whatsToken, c.body)
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", resp.StatusCode, raw)
			}
			bGolden(t, c.fixture, raw)
		})
	}
}
