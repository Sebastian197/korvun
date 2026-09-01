// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's verifier — Etapa 4, lote 4 (spec FR-VER, the E2 CLI
// mold): `korvun receipt verify` re-judges one receipt OFFLINE against
// the store file, trusting nothing — not even its own base. Every check
// fails with a NAMED reason, never a generic "invalid":
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
//
// Verification is READ-ONLY: it opens the store plainly (no sealer, no
// key generation) and records nothing.
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
	// timing) breaks the sealed reference BY NAME. Approvals are never
	// pruned, so an absent row here is itself a failure.
	if r.SchemaVersion >= 2 && r.ApprovalDigest != "" {
		switch consumed, _, err := store.GetApprovalByAction(ctx, r.ActionID); {
		case errors.Is(err, actionsqlite.ErrNotFound):
			fail("approval_mismatch", "receipt seals approval digest %s but no approval row exists for %s", r.ApprovalDigest, r.ActionID)
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
	rec, err := store.Get(ctx, r.ActionID)
	switch {
	case errors.Is(err, actionsqlite.ErrNotFound):
		notes = append(notes, "action_row_absent: row "+r.ActionID+" is gone (retention prunes action rows; the digest-sealed receipt is the surviving evidence)")
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
	store, err := actionsqlite.Open(storage)
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
