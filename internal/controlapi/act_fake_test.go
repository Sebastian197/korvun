// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ledger stand-in every mutation mould writes its act into, and the ledger
// that LIES, which is the one the guarantee needs.

package controlapi

import (
	"context"
	"fmt"
	"sync"
)

// fakeActs is an ActRecorder that remembers what it was asked to record. It can
// also refuse to seal, which is the attack «no act, no change» is about.
type fakeActs struct {
	mu sync.Mutex
	// beginErr, when set, makes every seal fail.
	beginErr error
	// begun is every act sealed, in order, as verb+params.
	begun []string
	// bound maps handle -> action id, as the surface asked.
	bound map[string]string
	// closed is every act closed, as "actionID:applied".
	closed []string
	// nextID counts the acts, so ids are distinct and predictable.
	nextID int
}

func newFakeActs() *fakeActs { return &fakeActs{bound: map[string]string{}} }

func (f *fakeActs) BeginConfigAct(_ context.Context, verb string, params []byte) (ConfigAct, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.beginErr != nil {
		return ConfigAct{}, f.beginErr
	}
	f.nextID++
	id := fmt.Sprintf("act_%02d", f.nextID)
	f.begun = append(f.begun, verb+" "+string(params))
	// No receipt at the seal: the real ledger mints one only for a terminal
	// state, so a double that handed one back would let a mould pass over a
	// promise the store does not keep.
	return ConfigAct{ActionID: id}, nil
}

func (f *fakeActs) BindReload(actionID, handle string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bound[handle] = actionID
}

func (f *fakeActs) SettleAct(_ context.Context, actionID string, applied bool, _ string) ConfigAct {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, fmt.Sprintf("%s:%t", actionID, applied))
	// The receipt is born at the CLOSE, as the real ledger does it.
	return ConfigAct{ActionID: actionID, ReceiptID: "rcpt_" + actionID}
}

func (f *fakeActs) SettleReload(ctx context.Context, handle string, applied bool, detail string) ConfigAct {
	f.mu.Lock()
	id := f.bound[handle]
	f.mu.Unlock()
	if id == "" {
		return ConfigAct{}
	}
	return f.SettleAct(ctx, id, applied, detail)
}

// CreateLedger on the in-package double refuses: the builder's door never
// founds a ledger, and a double that pretended to would hide a call that must
// not happen there.
func (f *fakeActs) CreateLedger(context.Context, string) (ConfigAct, string, error) {
	return ConfigAct{}, "", fmt.Errorf("fakeActs: CreateLedger is not a door of this surface")
}

// AdoptLedger on the in-package double refuses: the builder's door never
// adopts a ledger.
func (f *fakeActs) AdoptLedger(context.Context) (ConfigAct, error) {
	return ConfigAct{}, fmt.Errorf("fakeActs: AdoptLedger is not a door of this surface")
}

// LedgerStanding on the in-package double: nothing to say.
func (f *fakeActs) LedgerStanding(context.Context) (string, string) { return "", "" }

func (f *fakeActs) verbs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.begun...)
}

func (f *fakeActs) closes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.closed...)
}
