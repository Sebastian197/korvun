// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"testing"
)

// TE49's normalizer (e4Normalized) on every platform's spelling. The GET body
// is JSON, so the ledger's directory sits in it ESCAPED, and on Windows its
// separators are backslashes: in the first CI run of PR #69 the raw directory
// never matched there, and TE49's three bodies kept the real path. The rows
// are built with json.Marshal, the way the app's encoder writes them.
//
// PROBING MUTATIONS: the directory searched raw, not as JSON spells it → the
// Windows row reddens; nothing replaced → TE49 reddens on the host.
//
// Evidence level: unit, in process; the Windows row runs on any host.
func TestE4Normalized_findsTheDirectoryAsTheJSONSpellsIt(t *testing.T) {
	for _, c := range []struct{ name, path, dir, sep string }{
		{"posix", "/var/folders/x/T/TestE4/002/korvun.db", "/var/folders/x/T/TestE4/002", "/"},
		{"windows", `C:\Users\RUNNER~1\AppData\Local\Temp\TestE4\002\korvun.db`, `C:\Users\RUNNER~1\AppData\Local\Temp\TestE4\002`, `\`},
	} {
		body, err := json.Marshal(map[string]any{"ledger": map[string]string{"path": c.path}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := e4Normalized(string(body), c.dir, c.sep), `{"ledger":{"path":"/perfil/korvun.db"}}`; got != want {
			t.Errorf("%s: e4Normalized(%s) = %s, want %s", c.name, body, got, want)
		}
	}
	if body := `{"ledger":{"path":"/elsewhere/korvun.db"}}`; e4Normalized(body, "/var/x", "/") != body {
		t.Error("a body without the ledger's directory was changed")
	}
}
