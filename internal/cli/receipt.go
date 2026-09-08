// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's verifier — Etapa 4, lote 4 (spec FR-VER, the E2 CLI
// mold): `korvun receipt verify` re-judges one receipt OFFLINE against
// the store file, recomputing every derived value instead of trusting
// the stored one. What it CANNOT re-derive is the store's own registry:
// the key it verifies against is registered inside the file it judges,
// so a chain re-signed with a key the attacker registered there passes
// (R14, verified by execution; SECURITY.md's honest scope lists that
// one and the other THREE — the profile's identity, the chain's
// completeness, and a row whose lookup column changed storage class).
// Each failure the LADDER judges carries a NAMED reason rather than a
// generic "invalid" — while a receipt whose STORED BYTES do not parse
// is refused with the read error and no ladder name at all (R14 §3.5):
//
//	canonical_roundtrip_broken — the sealed form no longer survives the
//	                             fuzzed strict parser
//	hash_mismatch              — the stored hash is not the recomputed one
//	key_unknown                — the signing key id is not registered
//	signature_invalid          — the ink does not verify against the
//	                             REGISTERED public key
//	key_window_violated        — sealed outside the key's life
//	chain_link_broken          — the previous hash does not match the
//	                             predecessor (or the genesis link)
//	custody_mismatch           — the receipt and its action row disagree
//	action_evidence_incomplete — the action row and its decision row do
//	                             not survive together, which retention's
//	cascade never produces
//
// and, when the receipt seals an approval digest, the arms that judge
// the approval and tombstone evidence behind it:
//
//	approval_mismatch          — the sealed digest and the approval row
//	                             re-derive differently
//	tombstone_action_mismatch  — the tombstone points at another action
//	tombstone_corrupt          — the tombstone selected by the digest is
//	                             corrupt at a named field
//	tombstone_read_failed      — that evidence cannot be read, said by
//	                             name and never disguised as old history
//
// Verification is READ-ONLY in the sense `OpenReadOnly` defines and no
// wider: no sealer, no key generation, no schema migration, and a
// sealed connection that refuses every WRITE at the SQLite level. It
// is not a claim
// that the file on disk is untouched — that godoc carries what the
// open itself writes (R14).
package cli

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// receiptCmd dispatches the `receipt` noun's verbs.
func (c *cli) receiptCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun receipt: expected a subcommand: verify | rotate-key\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "verify":
		return c.receiptVerify(args[1:])
	case "rotate-key":
		return c.receiptRotateKey(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// receiptVerify implements `korvun receipt verify`: one positional id —
// a receipt id ("rcpt_…") or an action id (every receipt of the action).
func (c *cli) receiptVerify(args []string) int {
	fs := flag.NewFlagSet("receipt verify", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprint(c.stderr, "korvun receipt verify: usage: korvun receipt verify --config <path> <receipt-id | action-id>\n")
		return 2
	}
	id := fs.Arg(0)
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt verify: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	var receipts []action.Receipt
	if strings.HasPrefix(id, "rcpt_") {
		r, err := store.GetReceipt(ctx, id)
		if err != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun receipt verify: %s: %v\n", id, err)
			return 1
		}
		receipts = []action.Receipt{r}
	} else {
		receipts, err = store.ReceiptsByAction(ctx, id)
		if err != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun receipt verify: %s: %v\n", id, err)
			return 1
		}
		if len(receipts) == 0 {
			_, _ = fmt.Fprintf(c.stderr, "korvun receipt verify: %s: no receipts recorded\n", id)
			return 1
		}
	}
	code := 0
	for _, r := range receipts {
		failures, notes := verifyReceiptChecks(ctx, store, r)
		if len(failures) > 0 {
			code = 1
			for _, f := range failures {
				_, _ = fmt.Fprintf(c.stdout, "receipt %s (seq %d): FAIL %s\n", r.ReceiptID, r.ChainSeq, f)
			}
			continue
		}
		_, _ = fmt.Fprintf(c.stdout, "receipt %s (seq %d): OK\n", r.ReceiptID, r.ChainSeq)
		for _, n := range notes {
			_, _ = fmt.Fprintf(c.stdout, "receipt %s (seq %d): NOTE %s\n", r.ReceiptID, r.ChainSeq, n)
		}
	}
	return code
}

// verifyReceiptChecks runs the full named-check ladder over one receipt.
// It returns every failure found ("name: detail") plus non-fatal NAMED
// notes (R2, cross-check law: an absent action row under retention is a
// degraded check stated out loud — never a lying custody_mismatch,
// because the digest-sealed receipt IS the evidence that survives the
// prune, and absence-by-prune is indistinguishable from absence).
func verifyReceiptChecks(ctx context.Context, store *actionsqlite.Store, r action.Receipt) (failures, notes []string) {
	fail := func(name, format string, args ...any) {
		failures = append(failures, name+": "+fmt.Sprintf(format, args...))
	}
	// 1. Canonical roundtrip through the fuzzed strict parser: the sealed
	// form must survive its own wire format.
	if parsed, err := action.ParseCanonicalReceipt(action.CanonicalReceipt(r)); err != nil {
		fail("canonical_roundtrip_broken", "%v", err)
	} else if action.ComputeReceiptHash(parsed) != action.ComputeReceiptHash(r) {
		fail("canonical_roundtrip_broken", "the parsed form diverges from the stored one")
	}
	// 2. Hash recompute.
	if recomputed := action.ComputeReceiptHash(r); recomputed != r.ReceiptHash {
		fail("hash_mismatch", "stored %s, recomputed %s", r.ReceiptHash, recomputed)
	}
	// 3-5. The key: registered, valid ink, sealed inside its life.
	key, err := store.GetSigningKey(ctx, r.SigningKeyID)
	if err != nil {
		fail("key_unknown", "signing key %q is not registered", r.SigningKeyID)
	} else {
		pub, decodeErr := hex.DecodeString(key.PublicKey)
		switch {
		case decodeErr != nil || len(pub) != ed25519.PublicKeySize:
			fail("key_unknown", "registered public key for %q is unreadable", r.SigningKeyID)
		case action.VerifyReceiptSignature(ed25519.PublicKey(pub), r) != nil:
			fail("signature_invalid", "the signature does not verify against the registered public key %q", r.SigningKeyID)
		}
		sealedAt := r.FinishedAt
		if sealedAt.Before(key.CreatedAt) {
			fail("key_window_violated", "sealed %s but key %q was created %s",
				sealedAt.Format("2006-01-02T15:04:05Z07:00"), r.SigningKeyID, key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		if !key.RetiredAt.IsZero() && sealedAt.After(key.RetiredAt) {
			fail("key_window_violated", "sealed %s but key %q was retired %s",
				sealedAt.Format("2006-01-02T15:04:05Z07:00"), r.SigningKeyID, key.RetiredAt.Format("2006-01-02T15:04:05Z07:00"))
		}
	}
	// 6. The chain link.
	if r.ChainSeq == 0 {
		if r.PreviousReceiptHash != action.GenesisPreviousHash {
			fail("chain_link_broken", "seq 0 must link to the genesis hash, links to %s", r.PreviousReceiptHash)
		}
	} else {
		pred, err := store.ReceiptAt(ctx, r.Partition, r.ChainSeq-1)
		switch {
		case err != nil:
			fail("chain_link_broken", "predecessor %s/%d is missing", r.Partition, r.ChainSeq-1)
		case r.PreviousReceiptHash != pred.ReceiptHash:
			fail("chain_link_broken", "links to %s but predecessor %s carries %s",
				r.PreviousReceiptHash, pred.ReceiptID, pred.ReceiptHash)
		}
	}
	// 7b. Approval coherence (Etapa 5, FR-RCP): a v2 receipt that
	// references its approval must match the approval row's CONSUMED
	// decision digest — a rewritten approval row (decider, decision,
	// timing) breaks the sealed reference BY NAME. Approvals ARE
	// pruned — the retention cascade removes them WITH their terminal
	// action (ADR-0046) — so an absent approval row forks below: both
	// rows gone is retention (the tombstone carries the story); the
	// action alone remaining makes the absence a failure.
	if r.SchemaVersion >= 2 && r.ApprovalDigest != "" {
		// R15-P1A (Codex's fourteenth pass): the tombstone is judged
		// WHENEVER a row carries the sealed digest — never only after
		// retention took the approval AND the action row. The arms
		// below used to hang inside the both-rows-absent branch, so
		// forged evidence coexisting with a live approval was never
		// read and the verifier printed OK. The evidence is the same
		// evidence; its judgement no longer depends on whether the
		// story it seals was pruned. What absence MEANS still does
		// depend on that, and is decided in the approval-row fork.
		// R7-Y2: the natural lookup is the APPROVAL's own sealed
		// identity — each receipt reconstructs ITS story even when an
		// action_id was reused after the prune.
		tomb, tombAtPresent, terr := store.ApprovalTombstoneByDigest(ctx, r.ApprovalDigest)
		tombstoneProven := false
		switch {
		case terr == nil && tomb.ActionID != r.ActionID:
			// R8-Z2 binding: the preimage proves the digest but points
			// at ANOTHER action — a mutated tombstone.
			fail("tombstone_action_mismatch", "the tombstone for the sealed digest points at %s but the receipt belongs to %s — the evidence was re-pointed", tomb.ActionID, r.ActionID)
		case terr == nil && tomb.Digest() != r.ApprovalDigest:
			// Closed guard: the row was SELECTed BY this digest and the
			// reader's contract re-derives it from the preimage before
			// returning, so this is unreachable by construction (R12
			// killed the old either/or arm for exactly that reason).
			// It is kept as a NAMED failure and never as silence: an
			// unreachable arm that degrades quietly is how a verifier
			// learns to lie.
			fail("tombstone_corrupt", "the tombstone selected by the sealed digest re-derives %s: the reader's own contract was bypassed", tomb.Digest())
		case terr == nil:
			tombstoneProven = true
		case errors.Is(terr, actionsqlite.ErrNotFound):
			// R11: absence proves nothing — legacy history, deletion or
			// a coherent rewrite are indistinguishable to this
			// verifier. What it means HERE is said in the fork below,
			// where the surrounding rows are known.
		default:
			// R12-A11: a typed fault means the bytes WERE read and
			// are corrupt — saying "cannot read" would be a lie.
			var fault *actionsqlite.TombstoneFault
			if errors.As(terr, &fault) {
				fail("tombstone_corrupt", "the tombstone selected by the sealed digest is corrupt at %s: %v", fault.Field, terr)
			} else {
				fail("tombstone_read_failed", "cannot read the tombstone evidence for %s: %v — never disguised as old history", r.ActionID, terr)
			}
		}
		switch consumed, _, err := store.GetApprovalByAction(ctx, r.ActionID); {
		case errors.Is(err, actionsqlite.ErrNotFound):
			// R4-F4 (ADR-0046): retention CASCADES the approval with its
			// terminal action — both rows gone is the honest note (the
			// action_row_absent mold: the digest-sealed receipt is the
			// surviving evidence). The action still PRESENT with the
			// approval gone is sabotage: a cascade cannot do that.
			// R15: the question is whether the ACTION ROW is gone, and
			// `Get`'s ErrNotFound cannot answer it — it is a JOIN with
			// action_decisions, so it also fires when only the decision
			// row is missing. Asked directly, of the actions table.
			actionRow, _, perr := store.ActionRowsPresent(ctx, r.ActionID)
			if perr != nil {
				fail("approval_mismatch", "the approval row for %s is gone and the action row's presence cannot be established: %v", r.ActionID, perr)
				break
			}
			if !actionRow {
				// R6-X2: with both rows pruned the tombstone is the only
				// story left — a PROVEN one is reconstructed out loud,
				// its absence gets the ambiguous note, and a sabotaged
				// one already failed by name above.
				switch {
				case tombstoneProven:
					notes = append(notes, reconstructionNote(tomb, tombAtPresent))
				case errors.Is(terr, actionsqlite.ErrNotFound):
					// R11: the by-action integrity arm DIED with its false
					// positive and its false negative (direction decision;
					// SECURITY.md documents the v2-era limits until sealed
					// provenance). Absence gets the epistemological truth,
					// verbatim — never a certainty this verifier cannot have.
					notes = append(notes, "approval_row_absent: no tombstone with the sealed digest exists; legacy history, deletion, or a coherent rewrite are indistinguishable (the digest-sealed receipt is the surviving evidence)")
				}
				break
			}
			fail("approval_mismatch", "receipt seals approval digest %s but no approval row exists for %s while its action row remains — a cascade cannot do that", r.ApprovalDigest, r.ActionID)
		case err != nil:
			// F2: the failure NAMES its real cause — a stored preview
			// that no longer re-derives its pin reports the C2
			// preview_digest_mismatch, never a fictitious absent row.
			fail("approval_mismatch", "approval row for %s is unreadable: %v", r.ActionID, err)
		case consumed.Digest() != r.ApprovalDigest:
			fail("approval_mismatch", "receipt seals %s but the approval row re-derives %s", r.ApprovalDigest, consumed.Digest())
		}
	}
	// 7. Coherence with the action row — WHEN it still exists. The E1
	// retention prune legitimately removes action rows while receipts
	// stay (the sealed exemption): absence degrades to a named note.
	//
	// R15: `Store.Get` is a JOIN of `actions` with `action_decisions`,
	// so ONE ErrNotFound covered two different worlds and this check
	// read it as the first of them. Retention's cascade removes BOTH
	// rows; either one surviving alone is evidence no cascade can
	// produce, and treating it as an absence printed a note claiming a
	// prune that never happened AND returned early, silently skipping
	// the custody comparisons below. The two questions are now asked
	// separately and each world has its own name.
	actionRow, decisionRow, perr := store.ActionRowsPresent(ctx, r.ActionID)
	switch {
	case perr != nil:
		fail("custody_mismatch", "the presence of the action row for %q cannot be established: %v", r.ActionID, perr)
		return failures, notes
	case !actionRow && !decisionRow:
		// The cascade's own shape: both rows gone together.
		notes = append(notes, "action_row_absent: the action row for "+r.ActionID+" and its decision row are both gone, which is the shape retention's cascade leaves; the digest-sealed receipt is the surviving evidence")
		return failures, notes
	case !actionRow:
		fail("action_evidence_incomplete", "the action row for %q is gone but its decision row remains — retention removes both together, so this is evidence no cascade can produce", r.ActionID)
		return failures, notes
	case !decisionRow:
		fail("action_evidence_incomplete", "the action row for %q is present but its decision row is gone — retention removes both together, so this is evidence no cascade can produce", r.ActionID)
		return failures, notes
	}
	rec, err := store.Get(ctx, r.ActionID)
	switch {
	case errors.Is(err, actionsqlite.ErrNotFound):
		// Both rows were present a moment ago; a not-found here is the
		// TOCTOU window, not an absence this command can attest.
		fail("custody_mismatch", "the action row for %q vanished between the presence check and the read", r.ActionID)
		return failures, notes
	case err != nil:
		fail("custody_mismatch", "action row %q is unreadable: %v", r.ActionID, err)
		return failures, notes
	}
	if last := lastReceiptOutcome(ctx, store, r); last && string(rec.State) != r.Outcome {
		fail("custody_mismatch", "receipt attests %q but the action row says %q", r.Outcome, rec.State)
	}
	if rec.Envelope.ParametersDigest != r.ActionDigest {
		fail("custody_mismatch", "receipt action digest %s but the action row carries %s",
			r.ActionDigest, rec.Envelope.ParametersDigest)
	}
	return failures, notes
}

// reconstructionNote narrates the tombstone's proven story. An absent
// decision instant is said BY NAME — never printed as the year-0001
// zero value, and never asserted as certainty: a NULL is compatible
// with legacy history, and a coherent rewrite is indistinguishable.
func reconstructionNote(tomb action.Approval, decisionAtPresent bool) string {
	at := tomb.DecisionAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	if !decisionAtPresent {
		at = "decision_at_absent (compatible with legacy history; a coherent rewrite is indistinguishable)"
	}
	return fmt.Sprintf(
		"approval_evidence_reconstructed: decision=%s by=%s at=%s law=v%d %s preview=%s (digest re-derived from the tombstone and matches the seal)",
		tomb.Decision, tomb.DecisionPrincipalID, at,
		tomb.PolicyVersion, tomb.PolicyDigest, tomb.PreviewDigest)
}

// lastReceiptOutcome reports whether r is the LAST receipt of its
// action — only the last one must agree with the row's current state
// (an AUTHORIZED row records no receipt; a finished one records the
// terminal receipt after any earlier ones).
func lastReceiptOutcome(ctx context.Context, store *actionsqlite.Store, r action.Receipt) bool {
	receipts, err := store.ReceiptsByAction(ctx, r.ActionID)
	if err != nil || len(receipts) == 0 {
		return true
	}
	return receipts[len(receipts)-1].ReceiptID == r.ReceiptID
}

// receiptRotateKey implements `korvun receipt rotate-key`: the lote-2
// retire-and-activate rotation exposed to the operator. The act leaves
// its OWN receipt — sealed with the NEW key: the sealer is swapped
// inside the act, between the registry rotation and the terminal close.
func (c *cli) receiptRotateKey(args []string) int {
	fs := flag.NewFlagSet("receipt rotate-key", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun receipt rotate-key: --config is required\n")
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt rotate-key: %v\n", err)
		return 1
	}
	storage := app.StoragePath(cfg)
	profileDir := filepath.Dir(storage)
	// R4-F1 (ADR-0045): the rotation takes the exclusive profile lock.
	// A live server holds it for its whole life — rotating under it
	// would retire the key beneath the server's sealer, so a held lock
	// refuses with the stable rule and ZERO mutations.
	lock, err := app.AcquireProfileLock(profileDir)
	if err != nil {
		if errors.Is(err, app.ErrProfileLocked) {
			_, _ = fmt.Fprintf(c.stderr, "korvun receipt rotate-key: signing_key_in_use: a live server holds this profile — stop it before rotating (%v)\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt rotate-key: %v\n", err)
		return 1
	}
	defer func() { _ = lock.Release() }()
	// R2: the operator door — a key rotation beside a live server must
	// never run the boot's recovery/prune/migration under it.
	store, err := actionsqlite.OpenOperator(storage)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt rotate-key: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	// The CURRENT ink seals the act's AUTHORIZED record; the NEW ink
	// seals its terminal receipt after the swap below.
	oldPriv, err := app.EnsureSigningKey(ctx, store, profileDir)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt rotate-key: %v\n", err)
		return 1
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		return action.SignReceipt(oldPriv, r)
	})
	oldKeyID := action.SigningKeyID(oldPriv.Public().(ed25519.PublicKey))
	var newKeyID string
	params := contractParams(map[string]any{"old_key_id": oldKeyID})
	if err := recordOperatorAct(ctx, store, "receipt", "rotate-key", params, func() error {
		newPriv, rotateErr := app.RotateProfileSigningKey(ctx, store, profileDir)
		if newPriv != nil {
			// Even on a failed file swap the registry rotation is
			// effective — the act's own receipt must carry the ACTIVE
			// ink, never the retired one.
			newKeyID = action.SigningKeyID(newPriv.Public().(ed25519.PublicKey))
			store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
				return action.SignReceipt(newPriv, r)
			})
		}
		return rotateErr
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun receipt rotate-key: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "signing key rotated: %s retired, %s active\n", oldKeyID, newKeyID)
	return 0
}
