// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// Train E, batch 4 (GE3, GE7) — the app's half of TE47 (plan v3, O9): the
// openers the app reaches indirectly keep the store's class and the native
// cause under their own wrappers. The founding door's open that fails is
// ErrLedgerNotCreated carrying the environment class and its cause; the
// registry's transient open of a ledger that is not SQLite notes
// ledger_unreadable with the native cause, and closes nothing.
//
// PROBING MUTATION (MU47): a wrapper that drops its cause (%v, or a fixed
// sentence) → reddens.
//
// Evidence level: isolated wrapper units — the real recorder and registry,
// the founding door's open seam, a real non-SQLite file.
func TestE4_TE47_theAppsOpenersKeepTheClassAndTheCause(t *testing.T) {
	t.Run("the founding door's open", func(t *testing.T) {
		sandboxUserDirApp(t)
		rec := newLedgerlessRecorder(cfgWith(ollamaBrain()), NewConfigActRegistry(func(error) {}), func(error) {})
		rec.profile = testProfileIdentity
		cause := &e4NativeLike{code: 13, msg: "database or disk is full (13) (SQLITE_FULL)"}
		rec.openFresh = func(string, string) (*actionsqlite.Store, error) {
			return nil, errors.Join(actionsqlite.ErrLedgerEnvironment, cause)
		}
		_, _, err := rec.CreateLedger(context.Background(), "enable-storage")
		if !errors.Is(err, controlapi.ErrLedgerNotCreated) || !errors.Is(err, actionsqlite.ErrLedgerEnvironment) || !errors.Is(err, cause) {
			t.Fatalf("CreateLedger over an open that failed = %v, want ErrLedgerNotCreated carrying the environment class and its cause", err)
		}
	})
	t.Run("the registry's transient open", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "korvun.db")
		if err := os.WriteFile(path, []byte(strings.Repeat("this file is not a SQLite database; it is text.\n", 128)), 0o600); err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		var notes []error
		reg := NewConfigActRegistry(func(err error) {
			mu.Lock()
			notes = append(notes, err)
			mu.Unlock()
		})
		reg.profile = testProfileIdentity
		receipt, ok := reg.closeTransient(context.Background(), path, "act_te47", true, "applied", "")
		mu.Lock()
		defer mu.Unlock()
		if ok || receipt != "" || len(notes) != 1 {
			t.Fatalf("closeTransient over a text file = %q %v with %d note(s), want nothing closed and one note", receipt, ok, len(notes))
		}
		if !errors.Is(notes[0], actionsqlite.ErrLedgerUnreadable) || !strings.Contains(notes[0].Error(), "not a database") {
			t.Fatalf("the note = %v, want ledger_unreadable with the native cause", notes[0])
		}
	})
}
