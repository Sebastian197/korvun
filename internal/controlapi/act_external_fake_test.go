// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The ledger stand-in for the moulds that drive the doors from OUTSIDE the
// package.
//
// There are two of these — `fakeActs` in `package controlapi` for the moulds that
// reach unexported handlers, and this one in `package controlapi_test` for those
// that drive the mounted routes. Go gives no third option: an in-package double
// is invisible across the test-package boundary, and exporting one would put a
// test helper on the public surface, which is the class the official pass called
// «doors only tests reach». Both are recorded HERE so nobody edits one and
// believes they have edited both.

package controlapi_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Sebastian197/korvun/internal/controlapi"
)

type extActs struct {
	mu       sync.Mutex
	beginErr error
	begun    []string
	bound    map[string]string
	closed   []string
	nextID   int
	// createErr, when set, makes CreateLedger fail with exactly that error, so
	// the door's taxonomy (exists / not created / seal failed) can be driven.
	createErr error
	// createdPath is the path CreateLedger answers; created is every door it
	// was asked to found a ledger for.
	createdPath string
	created     []string
	// adoptErr, when set, makes AdoptLedger fail with exactly that error;
	// adopted counts the adoptions the door asked for.
	adoptErr error
	adopted  int
	// standing and owner are what LedgerStanding answers.
	standing, owner string
}

var errTestAdoptRefused = errors.New("the store refused the adoption")

// AdoptLedger records the adoption act, CLOSED with its receipt, as the real
// recorder does in one store transaction.
func (f *extActs) AdoptLedger(context.Context) (controlapi.ConfigAct, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.adoptErr != nil {
		return controlapi.ConfigAct{}, f.adoptErr
	}
	f.nextID++
	f.adopted++
	id := fmt.Sprintf("act_%02d", f.nextID)
	f.begun = append(f.begun, "config.adopt-ledger {\"door\":\"adopt-ledger\"}")
	f.closed = append(f.closed, id+":true")
	return controlapi.ConfigAct{ActionID: id, ReceiptID: "rcpt_" + id}, nil
}

// LedgerStanding on the double answers what the mould set, or nothing.
func (f *extActs) LedgerStanding(context.Context) (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.standing, f.owner
}

func (f *extActs) adoptions() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.adopted
}

func newExternalFakeActs() *extActs { return &extActs{bound: map[string]string{}} }

func (f *extActs) BeginConfigAct(_ context.Context, verb string, params []byte) (controlapi.ConfigAct, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.beginErr != nil {
		return controlapi.ConfigAct{}, f.beginErr
	}
	f.nextID++
	id := fmt.Sprintf("act_%02d", f.nextID)
	f.begun = append(f.begun, verb+" "+string(params))
	// No receipt at the seal: the real ledger mints one only for a terminal
	// state, so a double that handed one back would let a mould pass over a
	// promise the store does not keep.
	return controlapi.ConfigAct{ActionID: id}, nil
}

func (f *extActs) BindReload(actionID, handle string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bound[handle] = actionID
}

func (f *extActs) SettleAct(_ context.Context, actionID string, applied bool, _ string) controlapi.ConfigAct {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, fmt.Sprintf("%s:%t", actionID, applied))
	// The receipt is born at the CLOSE, as the real ledger does it.
	return controlapi.ConfigAct{ActionID: actionID, ReceiptID: "rcpt_" + actionID}
}

func (f *extActs) SettleReload(ctx context.Context, handle string, applied bool, detail string) controlapi.ConfigAct {
	f.mu.Lock()
	id := f.bound[handle]
	f.mu.Unlock()
	if id == "" {
		return controlapi.ConfigAct{}
	}
	return f.SettleAct(ctx, id, applied, detail)
}

// CreateLedger founds the ledger and seals the founding act in it, as the real
// ledgerless recorder does. It records the ORDER against the reloader through
// the act it returns: the mould checks the reloader saw the created path.
func (f *extActs) CreateLedger(_ context.Context, door string) (controlapi.ConfigAct, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return controlapi.ConfigAct{}, "", f.createErr
	}
	f.nextID++
	id := fmt.Sprintf("act_%02d", f.nextID)
	f.created = append(f.created, door)
	f.begun = append(f.begun, "config."+door+" "+`{"door":"`+door+`","path":"`+f.createdPath+`"}`)
	return controlapi.ConfigAct{ActionID: id}, f.createdPath, nil
}

func (f *extActs) createdDoors() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.created...)
}

func (f *extActs) verbs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.begun...)
}

func (f *extActs) closes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.closed...)
}
