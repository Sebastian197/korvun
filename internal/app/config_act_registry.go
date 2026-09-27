// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The process-wide memory of the operator acts in flight, and the close that
// survives a cutover (v0.16.2, the store-and-act-close plan).
//
// WHY IT IS NOT IN THE APP. The supervisor shuts the app that asked for a
// cutover down BEFORE it builds the next one, and a rollback is emitted with
// no app alive at all. The first shape of this piece kept the act's memory in
// the app's recorder and closed the act when the status door was polled — so
// against a REAL supervisor the poll landed on an app that had never heard of
// the act, the act stayed AUTHORIZED, and the next app's boot recovery closed
// it OUTCOME_UNKNOWN (evidence/v0.16.2/probe-real-cutover.txt). One registry
// per process (per desktop cycle), told of every reload state by the
// supervisor's observer, is what outlives the app.
//
// WHAT IT KNOWS. For every act sealed through it: the ledger it lives in, from
// the seal onwards (so a new app's recovery can spare it — `keep`); the reload
// handle bound to it; whether it is closed, and its receipt. And which ledger
// files this process itself founded. A ledger is known by its FILE, not by the
// spelling of its path: `korvun.db`, `./korvun.db` and the absolute form the
// boot resolves are one home (the internal pass's P3), so a profile re-spelt
// by the operator cannot make the new app's recovery miss an act in flight.
//
// HOW IT CLOSES. Through the recorder of the app that is serving, when that
// app's ledger is the act's home; otherwise through a TRANSIENT operator open
// of the act's ledger with the profile's own sealer — a rollback has no app
// with that ledger alive, and a profile whose storage was just removed has none
// either. It closes ONCE: a settler that arrives while another is inside the
// close WAITS for the receipt and answers the same one, rather than answering
// an act with no receipt (the internal pass's first P2).

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// ConfigActRegistry is the process-wide memory of operator acts. Construct one
// per process (per shell cycle) with NewConfigActRegistry, hand it to every
// Build with WithConfigActRegistry, and to the supervisor as its state
// observer (ObserveReload). A Build without one gets a private registry, which
// keeps a single-app process — and every mould that drives a stub reloader —
// exactly as it was.
type ConfigActRegistry struct {
	// profile is the identity of the profile this process serves (set by Build):
	// the one a transient open is born for.
	profile string
	mu      sync.Mutex
	note    func(error)
	// homes maps an act still open to the ledger file it was sealed in. An
	// entry appears at the seal and leaves at a close that landed.
	homes map[string]string
	// handles maps a reload handle to its act while the reload is open;
	// closedHandles keeps the pair afterwards so a later poll still finds it.
	handles       map[string]string
	closedHandles map[string]string
	// closing holds, per act being closed, a channel closed when the close
	// has landed (or given up). A second settler waits on it.
	closing map[string]chan struct{}
	// receipts remembers each closed act's receipt; closedOrder is the order
	// the acts closed in, so the oldest can be forgotten (closedRetention).
	receipts    map[string]string
	closedOrder []string
	// founders maps a founding act to the identity of the profile that founded
	// the ledger, so the close can mark the receipt (FinishFounding). Entries
	// leave with the close.
	founders map[string]string
	// profiles maps an act to the identity of the profile whose handle sealed
	// it: the profile a transient close opens the ledger for.
	profiles map[string]string
	// created remembers the ledger files this process founded — by FILE
	// (os.FileInfo, compared with os.SameFile), not by path: a file removed
	// and replaced at the same path by another actor is another book, and
	// re-adopting it by its spelling would seal this profile's founding act
	// in someone else's ledger (the official pass, round 2).
	created map[string]os.FileInfo
	// current is the recorder of the app that is SERVING — attached by Build
	// with its store open, detached by Shutdown before the store closes. It
	// is chosen by vitality, never by path alone: a dead app's recorder over
	// the right path would close into a closed database. The same rule holds
	// for the recorder a settler brings with it (`via`): once Shutdown has
	// detached it, it is dead and never used to close, however right its path.
	current *configActRecorder
}

// ledgerKey is how the registry names a ledger: the absolute, cleaned path of
// its file, the same resolution the store's own open performs. Two spellings
// of one file are one key; a path that cannot be made absolute is used as it
// is, which is also what the store would do.
func ledgerKey(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// closedRetention bounds the memory of CLOSED acts a registry keeps — the
// receipts and the handle→act pairs a later poll may still ask for. Every
// operator act adds one entry and nothing removed them; an operator pressing
// buttons for a year is a bounded growth the resource-bound invariant does
// not admit. The oldest closed entries go first; an act evicted here is still
// in the ledger, which is where its receipt lives.
const closedRetention = 512

// NewConfigActRegistry builds an empty registry. note hears every housekeeping
// failure of a close (never the operator's refusal); nil discards them.
func NewConfigActRegistry(note func(error)) *ConfigActRegistry {
	if note == nil {
		note = func(error) {}
	}
	return &ConfigActRegistry{
		note: note, homes: map[string]string{}, handles: map[string]string{},
		closedHandles: map[string]string{}, closing: map[string]chan struct{}{},
		receipts: map[string]string{}, created: map[string]os.FileInfo{},
		founders: map[string]string{},
		profiles: map[string]string{},
	}
}

// WithConfigActRegistry hands Build the process-wide registry, so the acts an
// earlier app sealed are spared by this app's recovery and closed through this
// app's ledger when their outcome arrives.
func WithConfigActRegistry(r *ConfigActRegistry) Option {
	return func(b *builder) {
		if r != nil {
			b.actRegistry = r
		}
	}
}

// ObserveReload is the supervisor's state observer: on a terminal state it
// closes the act bound to the handle, once, without waiting for anyone to
// poll. It runs on the supervisor's goroutine; the close is one SQLite
// transaction, bounded by the store's own busy timeout.
func (g *ConfigActRegistry) ObserveReload(h supervisor.Handle, st supervisor.State) {
	settled, applied := controlapi.ReloadOutcome(st)
	if !settled {
		return
	}
	g.settleHandle(context.Background(), string(h), applied, string(st), nil)
}

// remember records an act's home the moment it is sealed, by the ledger's key.
func (g *ConfigActRegistry) remember(actionID, ledgerPath, profile string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.homes[actionID] = ledgerKey(ledgerPath)
	g.profiles[actionID] = profile
}

// rememberFounder records which profile a founding act founds the ledger for.
func (g *ConfigActRegistry) rememberFounder(actionID, profile string) {
	if profile == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.founders[actionID] = profile
}

// bind remembers which act a reload handle belongs to.
func (g *ConfigActRegistry) bind(actionID, handle string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.handles[handle] = actionID
}

// inFlightIn names the acts sealed in ledgerPath and not yet closed — what the
// boot's recovery must spare, because this process owns them and will close
// them with the result. Sorted, so the boot's log reads the same twice.
func (g *ConfigActRegistry) inFlightIn(ledgerPath string) []string {
	key := ledgerKey(ledgerPath)
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []string
	for id, home := range g.homes {
		if home == key {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// attach names the recorder of the app now serving. detach forgets it; a
// detach for a recorder that is not the current one is a no-op, so an app
// shut down after its successor attached cannot unseat the successor.
func (g *ConfigActRegistry) attach(rec *configActRecorder) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.current = rec
}

func (g *ConfigActRegistry) detach(rec *configActRecorder) {
	if rec == nil {
		return
	}
	// Dead from here on, whoever still holds it: a request handler of the app
	// being shut down may still bring this recorder to settle, and it must
	// not close into the store that is closing under it.
	rec.dead.Store(true)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.current == rec {
		g.current = nil
	}
}

// rememberCreated / forgetCreated / createdHere: the ledger files this process
// founded, which are the only existing files CreateLedger may open again. A
// file that cannot be stat'ed at remember time is not remembered: nothing can
// be re-adopted by identity if the identity was never taken.
func (g *ConfigActRegistry) rememberCreated(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.created[ledgerKey(path)] = info
}

func (g *ConfigActRegistry) forgetCreated(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.created, ledgerKey(path))
}

// createdHere reports whether the file NOW at path is the very file this
// process founded there — same device and inode, not merely the same path.
func (g *ConfigActRegistry) createdHere(path string) bool {
	g.mu.Lock()
	info, ok := g.created[ledgerKey(path)]
	g.mu.Unlock()
	if !ok {
		return false
	}
	now, err := os.Stat(path)
	if err != nil {
		return false
	}
	return os.SameFile(info, now)
}

// settleHandle closes the act bound to a handle. A handle nobody bound is a
// no-op answering the zero act: the status door serves handles this surface
// never created, and closing an act it does not own would be inventing one.
func (g *ConfigActRegistry) settleHandle(ctx context.Context, handle string, applied bool, detail string, via *configActRecorder) controlapi.ConfigAct {
	g.mu.Lock()
	id := g.handles[handle]
	if id == "" {
		id = g.closedHandles[handle]
	}
	g.mu.Unlock()
	if id == "" {
		return controlapi.ConfigAct{}
	}
	return g.settle(ctx, id, applied, detail, via)
}

// settle closes one act ONCE and answers it with its receipt.
//
// The first settler owns the close; every settler that arrives while it is
// inside the close waits for it (bounded by its context) and answers the same
// receipt. A close that did not land releases the waiters with no receipt and
// leaves the act open for the next settler — the supervisor's observer or the
// next poll — rather than remembering a close that never happened.
//
// via is the recorder the settler holds, if any. The close goes through it
// when its ledger is the act's home; else through the serving app's recorder
// when THAT ledger is the home; else through a transient open of the home.
func (g *ConfigActRegistry) settle(ctx context.Context, actionID string, applied bool, detail string, via *configActRecorder) controlapi.ConfigAct {
	if actionID == "" {
		return controlapi.ConfigAct{}
	}
	g.mu.Lock()
	if receipt, done := g.receipts[actionID]; done {
		g.mu.Unlock()
		return controlapi.ConfigAct{ActionID: actionID, ReceiptID: receipt}
	}
	if wait, inProgress := g.closing[actionID]; inProgress {
		g.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return controlapi.ConfigAct{ActionID: actionID}
		}
		g.mu.Lock()
		receipt := g.receipts[actionID]
		g.mu.Unlock()
		return controlapi.ConfigAct{ActionID: actionID, ReceiptID: receipt}
	}
	wait := make(chan struct{})
	g.closing[actionID] = wait
	home := g.homes[actionID]
	founder := g.founders[actionID]
	if home == "" && via != nil {
		// An act this registry never saw sealed (a recorder built over a
		// private registry that was later swapped): its home is the caller's.
		home = via.ledgerKey
	}
	// The closer, by VITALITY: the settler's own recorder when it is alive and
	// holds the act's ledger; else the serving app's recorder when that one
	// does; else nobody, and the close opens the ledger transiently.
	var closer *configActRecorder
	if via != nil && !via.dead.Load() && via.ledgerKey == home {
		closer = via
	} else if g.current != nil && g.current.ledgerKey == home {
		closer = g.current
	}
	g.mu.Unlock()

	var receipt string
	var ok bool
	if closer != nil {
		receipt, ok = closer.finish(ctx, actionID, applied, detail, founder)
	} else {
		receipt, ok = g.closeTransient(ctx, home, actionID, applied, detail, founder)
	}

	g.mu.Lock()
	delete(g.closing, actionID)
	if ok {
		g.receipts[actionID] = receipt
		g.closedOrder = append(g.closedOrder, actionID)
		delete(g.homes, actionID)
		delete(g.founders, actionID)
		for h, id := range g.handles {
			if id == actionID {
				g.closedHandles[h] = id
				delete(g.handles, h)
			}
		}
		g.forgetOldestClosedLocked()
	}
	g.mu.Unlock()
	close(wait)
	return controlapi.ConfigAct{ActionID: actionID, ReceiptID: receipt}
}

// forgetOldestClosedLocked keeps the memory of closed acts within
// closedRetention, oldest first. Called with g.mu held.
func (g *ConfigActRegistry) forgetOldestClosedLocked() {
	for len(g.closedOrder) > closedRetention {
		old := g.closedOrder[0]
		g.closedOrder = g.closedOrder[1:]
		delete(g.receipts, old)
		for h, id := range g.closedHandles {
			if id == old {
				delete(g.closedHandles, h)
			}
		}
	}
}

// closeTransient closes an act through a fresh operator open of its ledger —
// the path a rollback needs, when no app holds the ledger. It installs the
// profile's own sealer, so the receipt is born signed like every other; if the
// key cannot be read, the act is NOT closed (no terminal is born without its
// receipt) and the note says so.
func (g *ConfigActRegistry) closeTransient(ctx context.Context, path, actionID string, applied bool, detail string, founder string) (string, bool) {
	if path == "" {
		g.note(fmt.Errorf("close the operator act %s (%s): its ledger is unknown", actionID, detail))
		return "", false
	}
	// The act's own profile first (the handle that sealed it), then the
	// founder's for a founding act, then the process's.
	profile := g.profiles[actionID]
	if profile == "" {
		profile = founder
	}
	if profile == "" {
		profile = g.profile
	}
	if profile == "" {
		g.note(fmt.Errorf("close the operator act %s (%s): this process has no profile to open %q for", actionID, detail, path))
		return "", false
	}
	store, err := actionsqlite.OpenOperatorFor(path, profile)
	if err != nil {
		g.note(fmt.Errorf("close the operator act %s (%s) through a transient open of %q: %w", actionID, detail, path, err))
		return "", false
	}
	defer func() { _ = store.Close() }()
	key, err := ensureSigningKey(ctx, store, filepath.Dir(path))
	if err != nil {
		g.note(fmt.Errorf("close the operator act %s (%s): the ledger's sealer could not be wired, and a terminal is never born without its receipt: %w", actionID, detail, err))
		return "", false
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(key, r) })
	return finishThrough(ctx, store, g.note, actionID, applied, detail, time.Now().UTC(), founder)
}

// jsonMarshalString quotes s as a JSON string.
func jsonMarshalString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
