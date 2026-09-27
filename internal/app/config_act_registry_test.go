// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// v0.16.2 · the process-wide registry of operator acts, and the recorder a
// profile with NO ledger mounts (D2–D7 of the store-and-act-close plan).
//
// Captured before this file (evidence/v0.16.2/probe-real-cutover.txt): after a
// real cutover the act sealed by the old app ended OUTCOME_UNKNOWN — the app that
// sealed it was shut down before the state turned terminal, and the new app's
// recovery took it for an orphan. These moulds pin the three things that fix it:
// the registry outlives the app; the close goes through whichever ledger handle is
// ALIVE, or through a transient open when none is; and «once» means the second
// settler WAITS for the receipt rather than answering without one.
//
// Evidence level, honest: in-process, REAL SQLite store on a real file, real
// identity and sealer. The wrapped ledgers are real stores with ONE method
// intercepted, and they say which. No supervisor here: the real cutover is the
// shell's mould.

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/identity"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// sandboxUserDirApp redirects the per-platform user-config root to a fresh temp
// dir, so a ledger founded at the DEFAULT path lands inside the sandbox and never
// in the developer's real profile. The same shape as the shell's sandboxUserDir;
// it lives here because Go gives a test package no way to borrow another's helper.
func sandboxUserDirApp(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin", "linux":
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", "")
	case "windows":
		t.Setenv("AppData", tmp)
	}
	resolved, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir after the redirect: %v", err)
	}
	if resolved != tmp && !strings.HasPrefix(resolved, tmp+string(os.PathSeparator)) {
		t.Fatalf("resolved user dir %q escaped the sandbox %q", resolved, tmp)
	}
	return tmp
}

// notesOf collects the recorder's housekeeping notes.
type notesOf struct {
	mu    sync.Mutex
	notes []error
}

func (n *notesOf) add(err error) { n.mu.Lock(); n.notes = append(n.notes, err); n.mu.Unlock() }
func (n *notesOf) list() []error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]error(nil), n.notes...)
}

// preparedStore opens a real store at path with the boot's own preparation —
// root intent, signing key, identity signers, identity registry, sealer — so an
// authenticated act can be sealed and closed with a signed receipt.
func preparedStore(t *testing.T, cfg *config.Config, path string) *actionsqlite.Store {
	t.Helper()
	store, err := actionsqlite.OpenFor(path, testProfileIdentity)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry, _, _, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	if err := ensureRootIntent(context.Background(), store); err != nil {
		t.Fatalf("root intent: %v", err)
	}
	key, err := ensureSigningKey(context.Background(), store, filepath.Dir(path))
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	wireIdentitySigners(store, key)
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(key, r) })
	if err := store.RegisterIdentity(context.Background(), registry, time.Now().UTC()); err != nil {
		t.Fatalf("register identity: %v", err)
	}
	return store
}

// recorderWith wires a recorder over a real prepared store and the given
// registry, and returns it with the store, its path and the notes.
func recorderWith(t *testing.T, reg *ConfigActRegistry) (*configActRecorder, *actionsqlite.Store, string, *notesOf) {
	t.Helper()
	cfg := cfgWith(ollamaBrain())
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	_, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	notes := &notesOf{}
	rec := newConfigActRecorder(store, resolver, issuers["console"], notes.add, reg, path)
	if rec == nil {
		t.Fatal("the recorder was not wired over a real store")
	}
	rec.profile = testProfileIdentity
	return rec, store, path, notes
}

// readOnlyRow reads an act's row and its receipts through a fresh read-only open,
// the way an operator's CLI would — never through the handle under test.
func readOnlyRow(t *testing.T, path, actionID string) (actionsqlite.Record, []action.Receipt) {
	t.Helper()
	ro, err := actionsqlite.OpenReadOnlyFor(path, testProfileIdentity)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer func() { _ = ro.Close() }()
	row, err := ro.Get(context.Background(), actionID)
	if err != nil {
		t.Fatalf("read %s: %v", actionID, err)
	}
	receipts, err := ro.ReceiptsByAction(context.Background(), actionID)
	if err != nil {
		t.Fatalf("receipts of %s: %v", actionID, err)
	}
	return row, receipts
}

// TestRegistry_keepNamesTheActsBegunAndNotClosed is D5's derivation: `keep` is
// every act SEALED in this ledger and not yet closed — from BeginConfigAct, not
// from BindReload — and nothing of another ledger.
//
// PROBING MUTATION: derive keep from the bindings instead of the seals. The act
// begun and never bound disappears from keep and this reddens.
func TestRegistry_keepNamesTheActsBegunAndNotClosed(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	rec, _, path, _ := recorderWith(t, reg)
	ctx := context.Background()

	a, err := rec.BeginConfigAct(ctx, "config.enable-approvals", []byte(`{"door":"enable-approvals"}`))
	if err != nil {
		t.Fatalf("begin a: %v", err)
	}
	b, err := rec.BeginConfigAct(ctx, "config.set-ceiling", []byte(`{"door":"set-ceiling"}`))
	if err != nil {
		t.Fatalf("begin b: %v", err)
	}
	rec.SettleAct(ctx, b.ActionID, false, "reload refused")

	keep := reg.inFlightIn(path)
	if len(keep) != 1 || keep[0] != a.ActionID {
		t.Fatalf("keep = %v, want exactly the act still open [%s]", keep, a.ActionID)
	}
	if other := reg.inFlightIn(filepath.Join(t.TempDir(), "other.db")); len(other) != 0 {
		t.Fatalf("keep for another ledger = %v, want nothing", other)
	}
}

// gatedLedger is a real store whose Finish BLOCKS until released, and reports
// when a caller has entered it. It is the barrier the concurrency mould needs:
// a second settler must be observed INSIDE the first's close, not after it.
type gatedLedger struct {
	actLedger
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *gatedLedger) Finish(ctx context.Context, id string, to action.State, at time.Time) error {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.actLedger.Finish(ctx, id, to, at)
}

// TestRegistry_aConcurrentSettlerWaitsForTheReceipt is F10, with a REAL barrier.
// The first settler is held inside Finish; the second arrives, and it must
// neither close again nor answer without a receipt — it waits, and both answer
// the same receipt. One Finish, one receipt, zero notes.
//
// PROBING MUTATION: make the second settler return as soon as it sees `settled`
// (the shape before this piece). It answers ReceiptID "" and this reddens.
func TestRegistry_aConcurrentSettlerWaitsForTheReceipt(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	_, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	notes := &notesOf{}
	gate := &gatedLedger{actLedger: store, entered: make(chan struct{}), release: make(chan struct{})}
	rec := newConfigActRecorderOver(gate, resolver, issuers["console"], notes.add, reg, path)
	ctx := context.Background()

	act, err := rec.BeginConfigAct(ctx, "config.lift-shadow", []byte(`{"door":"lift-shadow"}`))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	rec.BindReload(act.ActionID, "reload-9")

	first := make(chan controlapi.ConfigAct, 1)
	go func() { first <- rec.SettleAct(ctx, act.ActionID, true, "succeeded") }()
	<-gate.entered // the first settler is INSIDE Finish now

	second := make(chan controlapi.ConfigAct, 1)
	go func() { second <- rec.SettleReload(ctx, "reload-9", true, "succeeded") }()
	select {
	case got := <-second:
		t.Fatalf("the second settler answered %+v while the first was still inside Finish: it did not wait", got)
	case <-time.After(150 * time.Millisecond):
		// Still waiting, as it must.
	}

	close(gate.release)
	r1 := <-first
	r2 := <-second
	if r1.ReceiptID == "" {
		t.Fatalf("the first settler answered no receipt: %+v (notes %v)", r1, notes.list())
	}
	if r2.ReceiptID != r1.ReceiptID {
		t.Fatalf("the second settler answered receipt %q, the first %q: the waiter must get the same receipt", r2.ReceiptID, r1.ReceiptID)
	}
	if n := notes.list(); len(n) != 0 {
		t.Fatalf("notes %v: a second close was attempted", n)
	}
	row, receipts := readOnlyRow(t, path, act.ActionID)
	if row.State != action.StateSucceeded || len(receipts) != 1 {
		t.Fatalf("row %q with %d receipts, want SUCCEEDED with exactly one", row.State, len(receipts))
	}
}

// TestRegistry_settlesThroughATransientOpenWhenNoAppHoldsTheLedger is F10b: the
// supervisor emits `rolled-back` with the old app already shut down and the
// rollback app not yet built. No recorder is attached; the registry opens the
// ledger transiently, closes the act FAILED with its signed receipt, and shuts
// the handle.
//
// PROBING MUTATION: settle through the last recorder seen (the dead app's).
// Finish hits a closed store, a note lands, the act stays AUTHORIZED, and this
// reddens twice.
func TestRegistry_settlesThroughATransientOpenWhenNoAppHoldsTheLedger(t *testing.T) {
	notes := &notesOf{}
	reg := NewConfigActRegistry(notes.add)
	rec, store, path, _ := recorderWith(t, reg)
	ctx := context.Background()

	act, err := rec.BeginConfigAct(ctx, "config.post-config", []byte(`{"door":"post-config"}`))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	rec.BindReload(act.ActionID, "reload-3")
	// The app dies: its store closes and it is no longer attached.
	reg.detach(rec)
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reg.ObserveReload(supervisor.Handle("reload-3"), supervisor.StateRolledBack)

	row, receipts := readOnlyRow(t, path, act.ActionID)
	if row.State != action.StateFailed {
		t.Fatalf("the act of a rolled-back cutover is %q, want FAILED (notes %v)", row.State, notes.list())
	}
	if len(receipts) != 1 || receipts[0].Outcome != string(action.StateFailed) {
		t.Fatalf("receipts %+v, want one sealing FAILED", receipts)
	}
	if n := notes.list(); len(n) != 0 {
		t.Fatalf("the close left notes %v", n)
	}
	// A later poll of the same handle — the status door's own path, through
	// the registry with no recorder — gets the receipt, not nothing.
	if again := reg.settleHandle(ctx, "reload-3", false, "rolled-back", nil); again.ReceiptID != receipts[0].ReceiptID {
		t.Fatalf("a later poll answers %+v, want the receipt %q", again, receipts[0].ReceiptID)
	}
}

// TestRegistry_theObserverClosesThroughTheAttachedRecorder: with the serving
// app's recorder attached, the observer closes through it — and the status door
// polled afterwards answers the same receipt.
//
// PROBING MUTATION: do not close from ObserveReload. The row stays AUTHORIZED
// until a poll and this reddens on the read taken before any poll.
func TestRegistry_theObserverClosesThroughTheAttachedRecorder(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	rec, _, path, notes := recorderWith(t, reg)
	reg.attach(rec)
	ctx := context.Background()

	act, err := rec.BeginConfigAct(ctx, "config.allow-host", []byte(`{"door":"allow-host"}`))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	rec.BindReload(act.ActionID, "reload-4")

	reg.ObserveReload(supervisor.Handle("reload-4"), supervisor.StateSucceeded)

	row, receipts := readOnlyRow(t, path, act.ActionID)
	if row.State != action.StateSucceeded || len(receipts) != 1 {
		t.Fatalf("before any poll: row %q with %d receipts, want SUCCEEDED with one", row.State, len(receipts))
	}
	polled := rec.SettleReload(ctx, "reload-4", true, "succeeded")
	if polled.ReceiptID != receipts[0].ReceiptID {
		t.Fatalf("the poll answered %+v, want the receipt %q", polled, receipts[0].ReceiptID)
	}
	if n := notes.list(); len(n) != 0 {
		t.Fatalf("notes %v", n)
	}
}

// TestLedgerless_beginRefusesWithErrNoLedger: the recorder a store-less app
// mounts exists so its status door can close a founding act, and it refuses
// every ordinary seal BY NAME.
//
// PROBING MUTATION: return a plain error. The doors answer act_not_recorded and
// the controlapi mould reddens; this one reddens on errors.Is.
func TestLedgerless_beginRefusesWithErrNoLedger(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity
	_, err := rec.BeginConfigAct(context.Background(), "config.enable-approvals", []byte(`{}`))
	if !errors.Is(err, controlapi.ErrNoLedger) {
		t.Fatalf("BeginConfigAct = %v, want ErrNoLedger", err)
	}
}

// TestCreateLedger_createsPreparesAndSealsTheFoundingAct is D7's happy path on a
// REAL disk: the file appears at the default path, the key beside it, and the
// founding act is a readable AUTHORIZED row attributed to the Control API's
// operator — in keep, because it is open and this process owns it.
//
// PROBING MUTATION: seal nothing and return an id. The row is not in the book
// and this reddens on the read.
func TestCreateLedger_createsPreparesAndSealsTheFoundingAct(t *testing.T) {
	sandbox := sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity

	act, path, err := rec.CreateLedger(context.Background(), "enable-storage")
	if err != nil {
		t.Fatalf("CreateLedger: %v", err)
	}
	if !strings.HasPrefix(path, sandbox+string(os.PathSeparator)) {
		t.Fatalf("the ledger was founded at %q, outside the sandbox %q", path, sandbox)
	}
	if path != storagePath(cfg) {
		t.Fatalf("the ledger was founded at %q, the boot would resolve %q: two truths", path, storagePath(cfg))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no ledger file at %q: %v", path, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "keys")); err != nil {
		t.Fatalf("no signing key beside the ledger: %v", err)
	}
	row, receipts := readOnlyRow(t, path, act.ActionID)
	if row.State != action.StateAuthorized {
		t.Fatalf("the founding act is %q, want AUTHORIZED (sealed, not yet closed)", row.State)
	}
	if row.Envelope.Operation.Name != "config.enable-storage" {
		t.Fatalf("the founding act reads %q, want config.enable-storage", row.Envelope.Operation.Name)
	}
	// The ledger keeps the DIGEST of the parameters, not the parameters. The
	// act seals `{"door","path"}`: the same envelope built over those exact
	// bytes must carry the same digest, or the book sealed something else.
	wantParams := `{"door":"enable-storage","path":` + jsonString(path) + `}`
	want := action.NewEnvelope("probe", controlAPIWorkloadBrain,
		action.Source{Kind: "operator", Protocol: "http", Channel: "console"},
		action.Operation{Namespace: "config", Name: "config.enable-storage", Version: 1},
		wantParams, time.Now().UTC())
	if row.Envelope.ParametersDigest != want.ParametersDigest {
		t.Fatalf("the founding act's parameters digest %q is not the digest of %s", row.Envelope.ParametersDigest, wantParams)
	}
	if row.Identity == nil || row.Identity.PrincipalID != controlAPIOperatorPrincipal {
		t.Fatalf("the founding act is attributed to %+v, want the Control API operator", row.Identity)
	}
	if len(receipts) != 0 {
		t.Fatalf("the sealed act already carries %d receipts; the receipt seals the outcome", len(receipts))
	}
	if keep := reg.inFlightIn(path); len(keep) != 1 || keep[0] != act.ActionID {
		t.Fatalf("keep for the new ledger = %v, want [%s]: the boot's recovery would take the founding act", keep, act.ActionID)
	}
}

// TestCreateLedger_aLedgerAlreadyThereIsNeverAdopted is F4b: the default path is
// the DESKTOP's own book on this machine. A file already there is refused by
// name and left byte for byte as it was.
//
// PROBING MUTATION: open whatever is there. The founding act lands in the
// foreign file and this reddens on the bytes.
func TestCreateLedger_aLedgerAlreadyThereIsNeverAdopted(t *testing.T) {
	sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	path := storagePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	foreign := []byte("someone else's book")
	if err := os.WriteFile(path, foreign, 0o600); err != nil {
		t.Fatalf("write foreign file: %v", err)
	}
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity

	_, _, err := rec.CreateLedger(context.Background(), "enable-storage")
	if !errors.Is(err, controlapi.ErrLedgerExists) {
		t.Fatalf("CreateLedger over a foreign file = %v, want ErrLedgerExists", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("the refusal %q does not name the path", err)
	}
	got, _ := os.ReadFile(path) // #nosec G304 -- sandboxed test path
	if string(got) != string(foreign) {
		t.Fatalf("the foreign file changed: %q", got)
	}
	if keep := reg.inFlightIn(path); len(keep) != 0 {
		t.Fatalf("keep = %v after a refusal", keep)
	}
}

// TestCreateLedger_readoptsOnlyWhatThisProcessCreated is F3's repair leg: a
// ledger this process founded in a bootstrap that rolled back may be founded
// again from the same process — and only from it. A fresh process finds a file
// it did not create and refuses.
//
// PROBING MUTATION: forget what this process created. The retry reddens with
// ErrLedgerExists.
func TestCreateLedger_readoptsOnlyWhatThisProcessCreated(t *testing.T) {
	sandboxUserDirApp(t)
	notes := &notesOf{}
	reg := NewConfigActRegistry(notes.add)
	cfg := cfgWith(ollamaBrain())
	rec := newLedgerlessRecorder(cfg, reg, notes.add)
	rec.profile = testProfileIdentity
	ctx := context.Background()

	first, path, err := rec.CreateLedger(ctx, "enable-storage")
	if err != nil {
		t.Fatalf("first CreateLedger: %v", err)
	}
	rec.SettleAct(ctx, first.ActionID, false, "rolled-back") // the cutover failed

	second, again, err := rec.CreateLedger(ctx, "enable-storage")
	if err != nil {
		t.Fatalf("the retry over a file this process created was refused: %v", err)
	}
	if again != path || second.ActionID == first.ActionID {
		t.Fatalf("retry founded %q with act %s; want the same path %q and a new act", again, second.ActionID, path)
	}
	row, receipts := readOnlyRow(t, path, first.ActionID)
	if row.State != action.StateFailed || len(receipts) != 1 {
		t.Fatalf("the first founding act is %q with %d receipts, want FAILED with its receipt kept", row.State, len(receipts))
	}

	// Another process: an empty registry over the same file.
	stranger := newLedgerlessRecorder(cfg, NewConfigActRegistry(notes.add), notes.add)
	stranger.profile = testProfileIdentity
	if _, _, err := stranger.CreateLedger(ctx, "enable-storage"); !errors.Is(err, controlapi.ErrLedgerExists) {
		t.Fatalf("a fresh process adopted a file it did not create: %v", err)
	}
	if n := notes.list(); len(n) != 0 {
		t.Fatalf("notes %v", n)
	}
}

// TestCreateLedger_anUnwritableDirCreatesNoLedger is F4: the profile directory
// refuses writes. Nothing is founded, nothing is sealed, keep is empty.
//
// PROBING MUTATION: swallow the open error and return an act. This reddens on
// errors.Is.
func TestCreateLedger_anUnwritableDirCreatesNoLedger(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not refuse writes here (windows, or root)")
	}
	sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	dir := filepath.Dir(storagePath(cfg))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // an unwritable dir IS the attack; test-owned temp path
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restore the test-owned temp dir so TempDir can remove it
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity

	_, _, err := rec.CreateLedger(context.Background(), "enable-storage")
	if !errors.Is(err, controlapi.ErrLedgerNotCreated) {
		t.Fatalf("CreateLedger in an unwritable dir = %v, want ErrLedgerNotCreated", err)
	}
	if _, statErr := os.Stat(storagePath(cfg)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a ledger file exists after a refused founding: %v", statErr)
	}
	if keep := reg.inFlightIn(storagePath(cfg)); len(keep) != 0 {
		t.Fatalf("keep = %v", keep)
	}
}

// sealRefusingLedger is a real, prepared store whose seal ALONE fails.
type sealRefusingLedger struct{ actLedger }

func (sealRefusingLedger) RecordAttemptAuthenticated(context.Context, action.Envelope, actionsqlite.Decision, action.State, identity.Evidence) error {
	return errors.New("disk full")
}

// TestCreateLedger_aSealThatFailsAsksForNoChange is F5: the file was created and
// prepared, and the founding act could not be sealed. The error is neither
// «exists» nor «not created» — it names the seal — and the file stays.
//
// PROBING MUTATION: map every failure after the open to ErrLedgerNotCreated. This
// reddens on the class.
func TestCreateLedger_aSealThatFailsAsksForNoChange(t *testing.T) {
	sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity
	rec.wrap = func(store *actionsqlite.Store) actLedger { return sealRefusingLedger{actLedger: store} }

	_, _, err := rec.CreateLedger(context.Background(), "enable-storage")
	if err == nil {
		t.Fatal("a seal that failed founded a ledger with no act")
	}
	if errors.Is(err, controlapi.ErrLedgerNotCreated) || errors.Is(err, controlapi.ErrLedgerExists) {
		t.Fatalf("a seal failure was reported as %v: the file exists and was created by us", err)
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("the error %q does not carry the seal's own cause", err)
	}
	if _, statErr := os.Stat(storagePath(cfg)); statErr != nil {
		t.Fatalf("the file this process created was removed: %v", statErr)
	}
	if keep := reg.inFlightIn(storagePath(cfg)); len(keep) != 0 {
		t.Fatalf("keep = %v after a seal that failed", keep)
	}
}

// TestBuild_recoverySparesTheActsThisProcessOwns is D5 at the boot: an act this
// process sealed and still owns survives the new app's recovery; an AUTHORIZED
// row nobody owns is recovered as before (the control). And after the app is
// shut down, the observer closes the owned act through a transient open — not
// through the dead app.
//
// PROBING MUTATIONS: (1) call RecoverPreviousLife with no keep — the owned act
// closes OUTCOME_UNKNOWN and this reddens; (2) do not detach the recorder in
// Shutdown — the close after Shutdown hits a closed store and this reddens on
// the note.
func TestBuild_recoverySparesTheActsThisProcessOwns(t *testing.T) {
	notes := &notesOf{}
	reg := NewConfigActRegistry(notes.add)
	cfg := cfgWith(ollamaBrain())
	path := filepath.Join(t.TempDir(), "korvun.db")
	cfg.Storage = &config.StorageConfig{Path: path}
	// The mutation surface must MOUNT, or Build never attaches a recorder and
	// «detach in Shutdown» would be a branch this mould cannot see. The first
	// version had no admin block, and the probing mutation that removes the
	// detach stayed green: the close went transient in both worlds. Found by
	// the mutation, not by reading (mutations.txt, M66).
	t.Setenv("KORVUN_KEEP_TEST_ADMIN", "tok")
	cfg.Admin = &config.AdminConfig{TokenEnv: "KORVUN_KEEP_TEST_ADMIN"}
	cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}
	store := preparedStore(t, cfg, path)
	_, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	old := newConfigActRecorder(store, resolver, issuers["console"], notes.add, reg, path)
	ctx := context.Background()
	owned, err := old.BeginConfigAct(ctx, "config.post-config", []byte(`{"door":"post-config"}`))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	old.BindReload(owned.ActionID, "reload-1")
	// An orphan nobody owns, left AUTHORIZED by «another life».
	orphan := action.NewEnvelope(action.NewID(), "default",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "console"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1}, `{}`, time.Now().UTC())
	if err := store.RecordAttempt(ctx, orphan, actionsqlite.Decision{Outcome: "allow", Rule: "r"}, action.StateAuthorized); err != nil {
		t.Fatalf("record orphan: %v", err)
	}
	_ = store.Close() // the old app is gone

	a, err := Build(cfg, withChannelFactory(okFactory(newFakeChannel("telegram"))),
		WithReloader(stubReloader{}), WithConfigActRegistry(reg), withTestProfile())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	row, _ := readOnlyRow(t, path, owned.ActionID)
	if row.State != action.StateAuthorized {
		t.Fatalf("the act this process owns was recovered by the new app's boot: %q", row.State)
	}
	orphanRow, _ := readOnlyRow(t, path, orphan.ActionID)
	if orphanRow.State != action.StateOutcomeUnknown {
		t.Fatalf("the orphan nobody owns stayed %q: keep widened into an exemption", orphanRow.State)
	}

	sctx, sc := context.WithTimeout(ctx, 2*time.Second)
	defer sc()
	if err := a.Shutdown(sctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	reg.ObserveReload(supervisor.Handle("reload-1"), supervisor.StateRolledBack)
	closed, receipts := readOnlyRow(t, path, owned.ActionID)
	if closed.State != action.StateFailed || len(receipts) != 1 {
		t.Fatalf("after the app died the act is %q with %d receipts (notes %v), want FAILED with its receipt through a transient open", closed.State, len(receipts), notes.list())
	}
	if n := notes.list(); len(n) != 0 {
		t.Fatalf("the close after Shutdown left notes %v: it went through the dead app", n)
	}
}

// TestRecorderWithLedger_createLedgerRefuses: the seam holds its own line. A
// recorder that already has a ledger never founds another, even if a door
// forgot to refuse first.
//
// PROBING MUTATION: return an act and no error. This reddens.
func TestRecorderWithLedger_createLedgerRefuses(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	rec, _, path, _ := recorderWith(t, reg)
	act, got, err := rec.CreateLedger(context.Background(), "enable-storage")
	if err == nil || act.ActionID != "" {
		t.Fatalf("a recorder with a ledger founded another: act %+v err %v", act, err)
	}
	if got != path {
		t.Fatalf("the refusal names %q, want the ledger it already has, %q", got, path)
	}
}

// TestRegistry_anActAlreadyClosedByAnotherHandAnswersItsReceipt: a close that
// finds the act already terminal — closed by a recovery, or by the other
// settler through another handle — is not a failure to close. The registry
// answers the receipt the book holds and leaves no note.
//
// PROBING MUTATION: on a Finish error, note and answer nothing without
// re-reading the row. This reddens on the receipt and on the note.
func TestRegistry_anActAlreadyClosedByAnotherHandAnswersItsReceipt(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	rec, store, _, notes := recorderWith(t, reg)
	ctx := context.Background()
	act, err := rec.BeginConfigAct(ctx, "config.enable-approvals", []byte(`{"door":"enable-approvals"}`))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Another hand closes it first.
	if err := store.Finish(ctx, act.ActionID, action.StateSucceeded, time.Now().UTC()); err != nil {
		t.Fatalf("the other hand's close: %v", err)
	}
	got := rec.SettleAct(ctx, act.ActionID, true, "succeeded")
	receipts, _ := store.ReceiptsByAction(ctx, act.ActionID)
	if len(receipts) != 1 || got.ReceiptID != receipts[0].ReceiptID {
		t.Fatalf("settle answered %+v, the book holds %+v", got, receipts)
	}
	if n := notes.list(); len(n) != 0 {
		t.Fatalf("a close that found the act already terminal left notes %v", n)
	}
}

// TestRegistry_keepMatchesTheLedgerFileNotItsSpelling is the internal pass's
// P3: the act's home was compared by the STRING of its path, and an operator
// re-spelling `storage.path` (`korvun.db` → `./korvun.db`, a relative path made
// absolute) would make the new app's recovery miss the act in flight and close
// it OUTCOME_UNKNOWN. A ledger is its file.
//
// PROBING MUTATION: compare the raw strings. The second spelling finds nothing
// and this reddens.
func TestRegistry_keepMatchesTheLedgerFileNotItsSpelling(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	dir := t.TempDir()
	one := filepath.Join(dir, "korvun.db")
	// Two OTHER spellings of the same file. Built by hand, not by filepath.Join,
	// because Join cleans its result — the first version of this mould joined
	// `sub/..` and compared a string to itself, and the probing mutation that
	// compares raw strings stayed green (mutations.txt, M84: the finding was the
	// instrument's).
	sep := string(os.PathSeparator)
	dotted := dir + sep + "sub" + sep + ".." + sep + "korvun.db"
	// `sub` must exist for the OS to resolve `sub/..` in a stat; the registry
	// keys lexically, the file check does not.
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	relative, err := filepath.Rel(cwd, one)
	if err != nil {
		t.Skipf("no relative spelling from %q to %q: %v", cwd, one, err)
	}
	for _, other := range []string{dotted, relative} {
		if other == one {
			t.Fatalf("the mould's premise failed: %q is not another spelling of %q", other, one)
		}
	}
	reg.remember("act_spelt", one, testProfileIdentity)
	for _, other := range []string{dotted, relative} {
		if keep := reg.inFlightIn(other); len(keep) != 1 || keep[0] != "act_spelt" {
			t.Fatalf("keep for %q = %v, want the act sealed under %q: the same file spelt differently", other, keep, one)
		}
	}
	other := dotted
	if keep := reg.inFlightIn(filepath.Join(dir, "another.db")); len(keep) != 0 {
		t.Fatalf("keep for another file = %v", keep)
	}
	// And the creation memory follows the same rule — over a real file, since
	// what is remembered is the file's identity, not its spelling.
	if err := os.WriteFile(one, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	reg.rememberCreated(one)
	if !reg.createdHere(other) {
		t.Fatalf("a file this process created is not recognised under another spelling of its path")
	}
}

// TestRegistry_aDetachedRecorderIsNeverUsedToClose is the internal pass's P3
// on vitality: a request handler of the app being shut down can still bring
// its recorder to settle, after Shutdown detached it and closed the store. The
// registry must not close through it — the close goes transient, and lands.
//
// PROBING MUTATION: trust `via` by path alone. Finish hits the closed store, a
// note lands, the act stays open, and this reddens twice.
func TestRegistry_aDetachedRecorderIsNeverUsedToClose(t *testing.T) {
	notes := &notesOf{}
	reg := NewConfigActRegistry(notes.add)
	rec, store, path, recNotes := recorderWith(t, reg)
	reg.attach(rec)
	ctx := context.Background()
	act, err := rec.BeginConfigAct(ctx, "config.post-config", []byte(`{"door":"post-config"}`))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Shutdown: detach, then the store closes.
	reg.detach(rec)
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// A stale handler still holding the dead recorder settles through it.
	got := rec.SettleAct(ctx, act.ActionID, false, "rolled-back")
	row, receipts := readOnlyRow(t, path, act.ActionID)
	if row.State != action.StateFailed || len(receipts) != 1 || got.ReceiptID != receipts[0].ReceiptID {
		t.Fatalf("through a dead recorder: row %q, receipts %d, answered %+v (notes %v / %v)", row.State, len(receipts), got, notes.list(), recNotes.list())
	}
	if n := append(notes.list(), recNotes.list()...); len(n) != 0 {
		t.Fatalf("the close went through the dead recorder: notes %v", n)
	}
}

// TestRegistry_forgetsTheOldestClosedActs: the memory of closed acts is
// bounded (closedRetention); the oldest go first, and what a later poll can
// still ask for is the newest. The ledger keeps every receipt regardless.
//
// PROBING MUTATION: never forget. This reddens on the count.
func TestRegistry_forgetsTheOldestClosedActs(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	ctx := context.Background()
	// Acts closed through the registry's own bookkeeping, with no ledger: an
	// unknown home cannot be closed transiently, so the mould seeds the maps
	// the way a landed close does, through the one method that records one.
	for i := 0; i < closedRetention+3; i++ {
		id := fmt.Sprintf("act_%04d", i)
		reg.mu.Lock()
		reg.receipts[id] = "rcpt_" + id
		reg.closedOrder = append(reg.closedOrder, id)
		reg.closedHandles["h_"+id] = id
		reg.forgetOldestClosedLocked()
		reg.mu.Unlock()
	}
	reg.mu.Lock()
	nReceipts, nHandles, nOrder := len(reg.receipts), len(reg.closedHandles), len(reg.closedOrder)
	reg.mu.Unlock()
	if nReceipts != closedRetention || nHandles != closedRetention || nOrder != closedRetention {
		t.Fatalf("closed memory holds %d receipts, %d handles, %d in order; want %d each", nReceipts, nHandles, nOrder, closedRetention)
	}
	if got := reg.settleHandle(ctx, "h_act_0000", true, "succeeded", nil); got.ActionID != "" {
		t.Fatalf("the oldest closed act is still answered: %+v", got)
	}
	newest := fmt.Sprintf("act_%04d", closedRetention+2)
	if got := reg.settleHandle(ctx, "h_"+newest, true, "succeeded", nil); got.ReceiptID != "rcpt_"+newest {
		t.Fatalf("the newest closed act is not answered: %+v", got)
	}
}

// TestCreateLedger_refusesARelativeDefaultPath is the official pass's P3: when
// the user's config dir cannot be resolved, storagePath falls back to a path
// RELATIVE to the process's working directory. A ledger founded there would be
// a different file after a restart from elsewhere. Refused by name, nothing
// created.
//
// PROBING MUTATION: drop the IsAbs guard. The ledger is founded under the
// working directory and this reddens on errors.Is.
func TestCreateLedger_refusesARelativeDefaultPath(t *testing.T) {
	// No HOME / XDG root / AppData: os.UserConfigDir errors on every platform
	// and storagePath falls back to its relative form. The premise is under
	// this mould's own control, so failing it is a FAULT, never a skip: a
	// guarantee mould that can go quiet is no guard.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	cfg := cfgWith(ollamaBrain())
	if p := storagePath(cfg); filepath.IsAbs(p) {
		t.Fatalf("the mould's premise failed: storagePath resolved %q, absolute, with no user dir to resolve from", p)
	}
	// Whatever the relative path's directory, it is this test's to remove at
	// the end: the pre-check below proves it did not exist before.
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(storagePath(cfg))) })
	// The relative path resolves under the package directory when the test
	// runs. A file already there is a leftover of an earlier run WITHOUT the
	// guard (the probing mutation founds it) and would mask this mould's
	// «nothing created»: refuse to measure over it.
	if _, statErr := os.Stat(storagePath(cfg)); statErr == nil {
		t.Fatalf("a stale ledger sits at the relative path %q from an earlier run; remove it before measuring", storagePath(cfg))
	}
	reg := NewConfigActRegistry(func(error) {})
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity
	_, path, err := rec.CreateLedger(context.Background(), "enable-storage")
	if !errors.Is(err, controlapi.ErrLedgerNotCreated) {
		t.Fatalf("CreateLedger over a relative default = %v (path %q), want ErrLedgerNotCreated", err, path)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a ledger was founded at the relative path %q: %v", path, statErr)
	}
}

// TestCreateLedger_aReplacedFileIsNeverReadopted is the official pass's second
// round, P3: what this process founded is a FILE, not a path. A ledger founded
// here, then removed and replaced at the same path by another actor, is
// another book — refused by name, never re-adopted by its spelling.
//
// PROBING MUTATION: remember the creation by path alone. The replacement is
// re-adopted and this reddens on errors.Is.
func TestCreateLedger_aReplacedFileIsNeverReadopted(t *testing.T) {
	sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity
	ctx := context.Background()
	first, path, err := rec.CreateLedger(ctx, "enable-storage")
	if err != nil {
		t.Fatalf("first CreateLedger: %v", err)
	}
	rec.SettleAct(ctx, first.ActionID, false, "rolled-back")
	// Another actor replaces the file at the same path.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.WriteFile(path, []byte("another book at the same path"), 0o600); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, _, err := rec.CreateLedger(ctx, "enable-storage"); !errors.Is(err, controlapi.ErrLedgerExists) {
		t.Fatalf("a replaced file was re-adopted by its path: %v", err)
	}
	got, _ := os.ReadFile(path) // #nosec G304 -- sandboxed test path
	if string(got) != "another book at the same path" {
		t.Fatalf("the replacement was touched: %q", got)
	}
}

// TestCreateLedger_anOpenThatFailsLeavesNothing is the official pass's second
// round, P3: the file is claimed exclusively and the open of it fails (a full
// disk). «Nothing exists that did not before» must stay true — the zero-byte
// husk is removed by the same call, and it is not remembered as founded.
//
// PROBING MUTATION: leave the husk. The retry meets its own empty file and
// the stat below reddens.
func TestCreateLedger_anOpenThatFailsLeavesNothing(t *testing.T) {
	sandboxUserDirApp(t)
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	rec := newLedgerlessRecorder(cfg, reg, func(error) {})
	rec.profile = testProfileIdentity
	rec.openFresh = func(string, string) (*actionsqlite.Store, error) { return nil, errors.New("disk full") }
	_, path, err := rec.CreateLedger(context.Background(), "enable-storage")
	if !errors.Is(err, controlapi.ErrLedgerNotCreated) {
		t.Fatalf("CreateLedger with an open that fails = %v, want ErrLedgerNotCreated", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the husk of a failed open remains at %q: %v", path, statErr)
	}
	if reg.createdHere(path) {
		t.Fatal("a file that does not exist is remembered as founded here")
	}
	// The retry, with the disk back, founds normally.
	rec.openFresh = actionsqlite.OpenFor
	if _, _, err := rec.CreateLedger(context.Background(), "enable-storage"); err != nil {
		t.Fatalf("the retry after a failed open: %v", err)
	}
}

// unreadableLedger is a real ledger whose Standing cannot be read.
type unreadableLedger struct{ actLedger }

func (unreadableLedger) Standing(context.Context) (actionsqlite.LedgerStanding, string, error) {
	return "", "", errors.New("database is locked")
}

// TestConfigActRecorder_anUnknownStandingIsUnavailable is the internal pass's
// P3 on the marker, narrowed by train E (plan v3, §7, C1-1): a read of the
// mark that fails with no verdict of the store — the old uncoded «database is
// locked» — reaches the screen as `unavailable` with its cause: never as
// «nothing to say», which the screen cannot tell from an app started without
// a profile, and never as `unreadable`, whose remedy is to replace the book.
//
// PROBING MUTATION: answer "", "" on error, or unreadable. This reddens on
// the standing.
func TestConfigActRecorder_anUnknownStandingIsUnavailable(t *testing.T) {
	reg := NewConfigActRegistry(func(error) {})
	cfg := cfgWith(ollamaBrain())
	path := filepath.Join(t.TempDir(), "korvun.db")
	store := preparedStore(t, cfg, path)
	_, resolver, issuers, err := phase1IdentityRuntime(cfg)
	if err != nil {
		t.Fatalf("identity runtime: %v", err)
	}
	rec := newConfigActRecorderOver(unreadableLedger{actLedger: store}, resolver, issuers["console"], func(error) {}, reg, path)
	rec.profile = ProfileIdentity(filepath.Join(t.TempDir(), "korvun.json"))
	standing, cause := rec.LedgerStanding(context.Background())
	if standing != "unavailable" || !strings.Contains(cause, "database is locked") {
		t.Fatalf("an unknown failure is answered as %q / %q, want unavailable with its cause", standing, cause)
	}
}
