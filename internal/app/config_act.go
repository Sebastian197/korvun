// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator act behind a profile change made through the Control API
// (director's ruling, 2026-09-24). The seam and its reasoning live in
// internal/controlapi/act.go; this file is the half that holds a ledger, and
// the half that holds none.
//
// It is the same three-step shape `internal/cli` uses for an authority act —
// RecordAttemptAuthenticated, then the change, then Finish — with the Control
// API's own provenance: the console credential in front, the Control API's
// operator workload behind it, the administrator role responsible.
//
// What is NOT here: the memory of which act belongs to which reload, and the
// close itself. Both live in ConfigActRegistry (config_act_registry.go), which
// outlives the app — the app that seals an act is shut down before its cutover
// ends, so a close kept in the app would die with it (the capture in
// evidence/v0.16.2/probe-real-cutover.txt).

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/channel/console"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
	"github.com/Sebastian197/korvun/internal/identity"
)

// actLedger is the slice of the action store an operator act needs: seal,
// close, and read back the state and the receipt. `*actionsqlite.Store`
// satisfies it; the moulds wrap a real store to intercept ONE method — a seal
// that fails, a close that blocks — and say which.
type actLedger interface {
	RecordAttemptAuthenticated(ctx context.Context, env action.Envelope, d actionsqlite.Decision, state action.State, evidence identity.Evidence) error
	Finish(ctx context.Context, actionID string, to action.State, finishedAt time.Time) error
	// FinishFounding closes the founding act SUCCEEDED with the mark naming
	// the founder; AdoptLedger records the adoption act with the mark naming
	// the adopter, in one transaction; Standing judges the ledger for the
	// profile the handle serves (the durable mark, profile_identity.go).
	FinishFounding(ctx context.Context, actionID, digest string) error
	AdoptLedger(ctx context.Context, env action.Envelope, d actionsqlite.Decision, evidence identity.Evidence, digest string) (string, error)
	Standing(ctx context.Context) (actionsqlite.LedgerStanding, string, error)
	Get(ctx context.Context, actionID string) (actionsqlite.Record, error)
	ReceiptsByAction(ctx context.Context, actionID string) ([]action.Receipt, error)
}

// configActRecorder records profile changes in ONE ledger: the one the app it
// belongs to holds open.
type configActRecorder struct {
	ledger actLedger
	// ledgerPath is the file the ledger lives in, as configured; ledgerKey is
	// its registry key (absolute, cleaned — see ledgerKey). The key is what a
	// settler compares, so THIS handle is recognised as the act's ledger
	// however the profile spelt the path.
	ledgerPath string
	ledgerKey  string
	// dead is set by the registry's detach — Shutdown, before the store
	// closes. A dead recorder is never used to close, however right its path.
	dead atomic.Bool
	// profile is the identity of the profile this app serves (ProfileIdentity),
	// "" when the embedder named no profile: then no ledger is adopted or
	// marked from here, and the standing is not judged.
	profile  string
	resolver *identity.Resolver
	issuer   *identity.Issuer
	now      func() time.Time
	// note reports a housekeeping failure that happened AFTER the change was
	// under way. It never becomes the operator's refusal — the lesson of
	// 85013fd, where four writers returned a cadence's error over a durable
	// write and the operator's belief and the ledger disagreed for good.
	note func(error)
	reg  *ConfigActRegistry
}

// newConfigActRecorder wires the recorder over the app's store, or returns nil
// when there is no store. The store-less app mounts a ledgerlessRecorder
// instead (see Build), so nil here is a construction guard, not a mode.
func newConfigActRecorder(store *actionsqlite.Store, resolver *identity.Resolver,
	issuer *identity.Issuer, note func(error), reg *ConfigActRegistry, ledgerPath string) *configActRecorder {
	if store == nil || resolver == nil || issuer == nil {
		return nil
	}
	return newConfigActRecorderOver(store, resolver, issuer, note, reg, ledgerPath)
}

// newConfigActRecorderOver is newConfigActRecorder over any actLedger. It is
// what the moulds use to put a wrapped store under the recorder; production
// reaches it only through newConfigActRecorder and CreateLedger.
func newConfigActRecorderOver(ledger actLedger, resolver *identity.Resolver,
	issuer *identity.Issuer, note func(error), reg *ConfigActRegistry, ledgerPath string) *configActRecorder {
	if note == nil {
		note = func(error) {}
	}
	if reg == nil {
		reg = NewConfigActRegistry(note)
	}
	return &configActRecorder{
		ledger: ledger, ledgerPath: ledgerPath, ledgerKey: ledgerKey(ledgerPath),
		resolver: resolver, issuer: issuer,
		now: func() time.Time { return time.Now().UTC() }, note: note, reg: reg,
	}
}

// BeginConfigAct seals the act. Nothing is attempted if this fails. The
// registry learns the act's home the moment it is sealed — not when a reload
// is bound to it — so the boot's recovery of a new app can spare it from the
// first instant it exists.
func (r *configActRecorder) BeginConfigAct(ctx context.Context, verb string, params []byte) (controlapi.ConfigAct, error) {
	env, evidence, err := r.envelope(verb, string(params))
	if err != nil {
		return controlapi.ConfigAct{}, fmt.Errorf("identify the operator act: %w", err)
	}
	if err := r.ledger.RecordAttemptAuthenticated(ctx, env,
		actionsqlite.Decision{Outcome: "allow", Rule: "operator"},
		action.StateAuthorized, evidence); err != nil {
		// A ledger another profile owns refuses by name; the doors need the
		// seam's own name for it, so the operator is told to adopt rather
		// than that «the book refused».
		if errors.Is(err, actionsqlite.ErrLedgerForeignProfile) {
			return controlapi.ConfigAct{}, fmt.Errorf("%w: %w", controlapi.ErrLedgerForeign, err)
		}
		return controlapi.ConfigAct{}, fmt.Errorf("record the operator act: %w", err)
	}
	r.reg.remember(env.ActionID, r.ledgerPath, r.profile)
	// NO receipt here, and that is the ledger's shape rather than an omission: a
	// receipt seals a terminal state, and `RecordAttemptAuthenticated` mints one
	// for every state EXCEPT `StateAuthorized`. An act sealed over an ATTEMPT has
	// no outcome to seal. The receipt arrives at the close.
	return controlapi.ConfigAct{ActionID: env.ActionID}, nil
}

// BindReload remembers which act a handle belongs to.
func (r *configActRecorder) BindReload(actionID, handle string) { r.reg.bind(actionID, handle) }

// SettleAct closes an act once, through this ledger when it is the act's home.
func (r *configActRecorder) SettleAct(ctx context.Context, actionID string, applied bool, detail string) controlapi.ConfigAct {
	return r.reg.settle(ctx, actionID, applied, detail, r)
}

// SettleReload closes the act bound to a handle, once.
func (r *configActRecorder) SettleReload(ctx context.Context, handle string, applied bool, detail string) controlapi.ConfigAct {
	return r.reg.settleHandle(ctx, handle, applied, detail, r)
}

// CreateLedger refuses: this recorder already has a ledger. The door refuses
// the same profile earlier, by name; this is the seam holding its own line.
func (r *configActRecorder) CreateLedger(context.Context, string) (controlapi.ConfigAct, string, error) {
	return controlapi.ConfigAct{}, r.ledgerPath,
		fmt.Errorf("this profile already has an action ledger at %s: nothing to create", r.ledgerPath)
}

// AdoptLedger records the adoption act in this ledger — the one write a
// foreign ledger admits — closed with the mark naming this profile, and
// answers it with its receipt. Without a profile identity there is nothing to
// mark, and the seam says so.
func (r *configActRecorder) AdoptLedger(ctx context.Context) (controlapi.ConfigAct, error) {
	if r.profile == "" {
		return controlapi.ConfigAct{}, errors.New("this app was started without a profile path, so there is no profile to adopt the ledger for")
	}
	env, evidence, err := r.envelope(actionsqlite.AdoptionVerb, `{"door":"adopt-ledger"}`)
	if err != nil {
		return controlapi.ConfigAct{}, fmt.Errorf("identify the adoption act: %w", err)
	}
	receipt, err := r.ledger.AdoptLedger(ctx, env, actionsqlite.Decision{Outcome: "allow", Rule: "operator"}, evidence, r.profile)
	if err != nil {
		return controlapi.ConfigAct{}, fmt.Errorf("record the adoption act: %w", err)
	}
	return controlapi.ConfigAct{ActionID: env.ActionID, ReceiptID: receipt}, nil
}

// LedgerStanding judges the ledger for this profile, now. With no profile
// identity there is nothing to judge, and the answer is empty.
func (r *configActRecorder) LedgerStanding(ctx context.Context) (string, string) {
	if r.profile == "" {
		return "", ""
	}
	standing, owner, err := r.ledger.Standing(ctx)
	if err != nil {
		// A failure is its own answer, never «nothing to say», and it carries
		// its cause: the doors fail closed on the same error, and the operator
		// must see why. Which answer is the store's NAME for the failure,
		// never its text.
		r.note(fmt.Errorf("judge the ledger's standing: %w", err))
		switch {
		case errors.Is(err, actionsqlite.ErrLedgerUnreadable), errors.Is(err, actionsqlite.ErrLedgerMarkMalformed):
			// A verdict on the book: its remedy is to replace it.
			return controlapi.LedgerStandingUnreadable, err.Error()
		case errors.Is(err, actionsqlite.ErrLedgerEnvironment):
			// The storage around the ledger failed, not the ledger: never
			// the unreadable answer, whose remedy is to replace the book.
			return controlapi.LedgerStandingEnvironment, err.Error()
		default:
			// Busy, or a failure with no class: the ledger could not be
			// checked now, which says nothing against the book.
			return controlapi.LedgerStandingUnavailable, err.Error()
		}
	}
	return string(standing), owner
}

// LedgerPath is the absolute path of the file this recorder's ledger lives
// in — its registry key, resolved as the store's own open resolves it — for
// the screen to name the file its remedies are about.
func (r *configActRecorder) LedgerPath() string { return r.ledgerKey }

// finish closes one act through THIS ledger and returns its receipt. ok is
// false when the act is not closed afterwards — a failure a later settler must
// retry, as opposed to a close that landed and only failed to read its receipt.
func (r *configActRecorder) finish(ctx context.Context, actionID string, applied bool, detail string, founder string) (receipt string, ok bool) {
	return finishThrough(ctx, r.ledger, r.note, actionID, applied, detail, r.now(), founder)
}

// finishThrough is the one close every settler uses — the app's own handle or
// a transient one. A Finish that fails is re-read: an act that turns out to be
// terminal already (closed by another hand) counts as closed and its receipt
// is answered; anything else is a note and NOT a close. founder, when set, is
// the identity of the profile a FOUNDING act closes for: an applied founding
// closes through FinishFounding, which marks the receipt; a failed one closes
// plain, and marks nothing.
func finishThrough(ctx context.Context, ledger actLedger, note func(error), actionID string, applied bool, detail string, at time.Time, founder string) (string, bool) {
	state := action.StateSucceeded
	if !applied {
		state = action.StateFailed
	}
	var err error
	if founder != "" && applied {
		err = ledger.FinishFounding(ctx, actionID, founder)
	} else {
		err = ledger.Finish(ctx, actionID, state, at)
	}
	if err != nil {
		row, readErr := ledger.Get(ctx, actionID)
		if readErr == nil && row.State != action.StateAuthorized {
			// Already terminal: the close happened, by this process or by a
			// recovery. The receipt is whatever the book holds.
			return lastReceipt(ctx, ledger, note, actionID), true
		}
		// The change is already under way. Returning this to the operator would
		// print a refusal over a running cutover, which is exactly the class
		// cured on 2026-09-22 for four store writers. It is NOT swallowed: it
		// goes to the note, so an act left open is visible in the profile log.
		note(fmt.Errorf("close the operator act %s (%s): %w", actionID, detail, err))
		return "", false
	}
	return lastReceipt(ctx, ledger, note, actionID), true
}

// lastReceipt re-reads the newest receipt of an act, or "" when it cannot.
func lastReceipt(ctx context.Context, ledger actLedger, note func(error), actionID string) string {
	receipts, err := ledger.ReceiptsByAction(ctx, actionID)
	if err != nil {
		note(fmt.Errorf("re-read the receipt of the operator act %s: %w", actionID, err))
		return ""
	}
	if len(receipts) == 0 {
		return ""
	}
	return receipts[len(receipts)-1].ReceiptID
}

// envelope builds the identified envelope of one Control API operator act. The
// provenance is the ADMIN surface's, not the CLI's: an act that claimed to come
// from a terminal would be a false statement about who did it.
func (r *configActRecorder) envelope(verb, params string) (action.Envelope, identity.Evidence, error) {
	now := r.now()
	env := action.NewEnvelope(action.NewID(), controlAPIWorkloadBrain,
		action.Source{Kind: "operator", Protocol: "http", Channel: "console"},
		action.Operation{Namespace: "config", Name: verb, Version: 1},
		params, now)
	ingress, err := r.issuer.Issue(env.CorrelationID, "local_operator")
	if err != nil {
		return action.Envelope{}, identity.Evidence{}, err
	}
	evidence, err := r.resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: env.ActionID, RequestID: env.CorrelationID,
		Channel: "console", Brain: controlAPIWorkloadBrain,
	})
	if err != nil {
		return action.Envelope{}, identity.Evidence{}, err
	}
	env.Principal = action.PrincipalRef{
		PrincipalID:        evidence.ActorPrincipalID,
		ResponsibleHumanID: evidence.ResponsiblePrincipalID,
		EvidenceID:         evidence.EvidenceID,
	}
	env.IntentID = action.RootIntentID
	return env, evidence, nil
}

// ---------------------------------------------------------------------------
// The recorder of a profile with NO ledger.
// ---------------------------------------------------------------------------

// ledgerlessRecorder is what a store-less app mounts on its doors. It refuses
// every ordinary seal by name (controlapi.ErrNoLedger → `no_ledger`), it can
// still close an act through the registry — the founding act of a bootstrap
// that rolled back has no other closer — and it holds the ONE door such a
// profile has: CreateLedger.
type ledgerlessRecorder struct {
	cfg  *config.Config
	reg  *ConfigActRegistry
	note func(error)
	// wrap is the seam the moulds use to put a wrapped store under the
	// founding act's seal (a seal that fails alone). Production leaves it as
	// the identity.
	wrap func(*actionsqlite.Store) actLedger
	// openFresh is the seam the moulds use to make the open of the file just
	// claimed fail (a full disk). Production is actionsqlite.Open.
	openFresh func(path, identity string) (*actionsqlite.Store, error)
	// profile is the identity of the profile this app serves; the ledger this
	// recorder founds is marked with it when the founding act closes applied.
	// "" (no profile path) founds an unmarked — legacy — ledger.
	profile string
}

func newLedgerlessRecorder(cfg *config.Config, reg *ConfigActRegistry, note func(error)) *ledgerlessRecorder {
	if note == nil {
		note = func(error) {}
	}
	if reg == nil {
		reg = NewConfigActRegistry(note)
	}
	return &ledgerlessRecorder{
		cfg: cfg, reg: reg, note: note,
		wrap:      func(s *actionsqlite.Store) actLedger { return s },
		openFresh: actionsqlite.OpenFor,
	}
}

// BeginConfigAct refuses by name: there is no book.
func (l *ledgerlessRecorder) BeginConfigAct(context.Context, string, []byte) (controlapi.ConfigAct, error) {
	return controlapi.ConfigAct{}, fmt.Errorf("%w (no storage block in the profile)", controlapi.ErrNoLedger)
}

// BindReload remembers which act a handle belongs to.
func (l *ledgerlessRecorder) BindReload(actionID, handle string) { l.reg.bind(actionID, handle) }

// SettleAct closes an act once — through a transient open, since this app
// holds no ledger.
func (l *ledgerlessRecorder) SettleAct(ctx context.Context, actionID string, applied bool, detail string) controlapi.ConfigAct {
	return l.reg.settle(ctx, actionID, applied, detail, nil)
}

// SettleReload closes the act bound to a handle, once.
func (l *ledgerlessRecorder) SettleReload(ctx context.Context, handle string, applied bool, detail string) controlapi.ConfigAct {
	return l.reg.settleHandle(ctx, handle, applied, detail, nil)
}

// CreateLedger founds the action store at the profile's default path — the
// same resolution the boot uses for an empty storage.path — and seals the
// founding act in it. Its taxonomy, in order:
//
//   - a file already there that this process did not create: ErrLedgerExists,
//     with the path. Nothing is opened. The default path is the desktop's own
//     book on this machine, and adopting it from another profile would be
//     sealing acts in someone else's book;
//   - the very file this process DID create (same device and inode, not
//     merely the same path), in a bootstrap that rolled back: opened again,
//     so the button can be pressed again (the first founding act stays FAILED
//     in it — history is kept);
//   - the file cannot be created or opened: ErrLedgerNotCreated. Nothing
//     exists that did not before: a file this call claimed and then could
//     not open is removed by this same call;
//   - created, but a preparation step or the seal fails: a plain error naming
//     the step. The file stays, declared: deleting a book is not this door's
//     call, and a retry over it works.
//
// The preparation is the boot's own — root intent, signing key, identity
// signers and registry, receipt sealer — run idempotently, so the boot that
// follows finds what it would have created.
func (l *ledgerlessRecorder) CreateLedger(ctx context.Context, door string) (controlapi.ConfigAct, string, error) {
	if l.cfg.Storage != nil {
		return controlapi.ConfigAct{}, storagePath(l.cfg),
			fmt.Errorf("this profile already has an action ledger at %s: nothing to create", storagePath(l.cfg))
	}
	path := storagePath(l.cfg)
	// The default resolution falls back to a RELATIVE path when the user's
	// config dir cannot be resolved (storagePath). A ledger founded relative to
	// the process's working directory would be a different file after a
	// restart from elsewhere; refuse by name instead of founding it.
	if !filepath.IsAbs(path) {
		return controlapi.ConfigAct{}, path, fmt.Errorf("%w: the user's config dir could not be resolved, so the ledger's path %q is not absolute", controlapi.ErrLedgerNotCreated, path)
	}
	var store *actionsqlite.Store
	if l.reg.createdHere(path) {
		// A ledger this process founded in a bootstrap that rolled back: opened
		// again, never migrated (OpenOperator refuses any other schema).
		s, err := actionsqlite.OpenOperatorFor(path, l.profile)
		if err != nil {
			return controlapi.ConfigAct{}, path, fmt.Errorf("%w: %w", controlapi.ErrLedgerNotCreated, err)
		}
		store = s
	} else {
		// CREATE, exclusively: the file is claimed with O_EXCL, so a file that
		// appears between a check and the open — the desktop's own first boot
		// on this machine — is never adopted. A stat-then-open would have been
		// a window; this is not (the official pass's P3).
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return controlapi.ConfigAct{}, path, fmt.Errorf("%w: %w", controlapi.ErrLedgerNotCreated, err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- the profile's own default ledger path
		if errors.Is(err, os.ErrExist) {
			return controlapi.ConfigAct{}, path, fmt.Errorf("%w: %s", controlapi.ErrLedgerExists, path)
		}
		if err != nil {
			return controlapi.ConfigAct{}, path, fmt.Errorf("%w: %w", controlapi.ErrLedgerNotCreated, err)
		}
		_ = f.Close()
		l.reg.rememberCreated(path)
		// Open, not OpenOperator, over the EMPTY file just claimed: it lays the
		// schema down as the boot would, and there is nothing in the file to
		// migrate — the exclusive create proved it was ours from byte zero.
		s, err := l.openFresh(path, l.profile)
		if err != nil {
			// The empty file is THIS call's own, claimed exclusively a moment
			// ago: removing it keeps «nothing exists that did not before» true,
			// and a retry does not find its own zero-byte husk in the way.
			_ = os.Remove(path)
			l.reg.forgetCreated(path)
			return controlapi.ConfigAct{}, path, fmt.Errorf("%w: %w", controlapi.ErrLedgerNotCreated, err)
		}
		store = s
	}
	defer func() { _ = store.Close() }()

	prepared := func(step string, err error) error {
		return fmt.Errorf("the ledger was created at %s but could not be prepared to seal its founding act (%s): %w", path, step, err)
	}
	if err := ensureRootIntent(ctx, store); err != nil {
		return controlapi.ConfigAct{}, path, prepared("root intent", err)
	}
	key, err := ensureSigningKey(ctx, store, filepath.Dir(path))
	if err != nil {
		return controlapi.ConfigAct{}, path, prepared("signing key", err)
	}
	wireIdentitySigners(store, key)
	registry, resolver, issuers, err := phase1IdentityRuntime(l.cfg)
	if err != nil {
		return controlapi.ConfigAct{}, path, prepared("identity runtime", err)
	}
	if err := store.RegisterIdentity(ctx, registry, time.Now().UTC()); err != nil {
		return controlapi.ConfigAct{}, path, prepared("identity registry", err)
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt { return action.SignReceipt(key, r) })

	rec := newConfigActRecorderOver(l.wrap(store), resolver, issuers[console.ChannelName], l.note, l.reg, path)
	params := []byte(`{"door":"` + door + `","path":` + jsonString(path) + `}`)
	act, err := rec.BeginConfigAct(ctx, "config."+door, params)
	if err != nil {
		return controlapi.ConfigAct{}, path, fmt.Errorf("the ledger was created at %s but its founding act could not be sealed: %w", path, err)
	}
	// The founder: when the act closes applied, the registry closes it through
	// FinishFounding with this identity, and the ledger knows its profile.
	l.reg.rememberFounder(act.ActionID, l.profile)
	return act, path, nil
}

// AdoptLedger with no ledger: nothing to adopt, by name.
func (l *ledgerlessRecorder) AdoptLedger(context.Context) (controlapi.ConfigAct, error) {
	return controlapi.ConfigAct{}, fmt.Errorf("%w (no storage block in the profile)", controlapi.ErrNoLedger)
}

// LedgerStanding with no ledger: nothing to judge.
func (l *ledgerlessRecorder) LedgerStanding(context.Context) (string, string) { return "", "" }

// jsonString quotes a path as a JSON string, so a backslash or a quote in it
// cannot break the founding act's parameters.
func jsonString(s string) string {
	b, err := jsonMarshalString(s)
	if err != nil {
		return `""`
	}
	return b
}
