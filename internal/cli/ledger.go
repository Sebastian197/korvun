// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's verifier, chain half — Etapa 4, lote 4 (spec FR-VER
// §19.2): `korvun ledger check` walks one partition's WHOLE chain and
// names the FIRST broken link with its reason. Gap and tamper detection
// INSIDE the profile the config names, as a first-class operation:
//
//	chain_seq_gap       — a sequence position is missing (a deleted
//	                      receipt is denounced by its hole)
//	chain_seq_duplicate — two receipts claim one chain position
//	plus every per-receipt check of `receipt verify` (hash, signature,
//	key window, link, custody), applied link by link.
//
// Read-only, like verify: the plain opener, whose connection refuses
// every statement it issues. Not a claim about the file's bytes —
// `OpenReadOnly`'s godoc carries what the open itself writes (R14).
// Honest limit,
// documented (SECURITY.md; R13 D11 and R14): this command proves that
// ONE PARTITION of the profile — the one named by --partition, "main"
// by default — is consistent with itself. It says nothing about the
// receipts of any other partition. Moving a receipt out of "main" is
// therefore the DELETION case and behaves like it: from the middle of
// the chain it is denounced as chain_seq_gap, because the expectation
// in pass 1 is index-derived; from the TAIL it leaves no hole at all. It does not prove that the profile
// is the one your history wrote (a redirected config judges another
// store), that the chain is COMPLETE (any truncation from the TAIL —
// the last receipt, any suffix, or the whole partition, which reports
// "0 receipts, chain intact" — leaves no hole to detect), or that the
// keys are the AUTHORITATIVE ones (the registry lives inside the file
// being judged). Those three were EXECUTED in the R14 canto, each in
// one shape: the last receipt of a two-receipt chain, a redirected
// config, a self-registered key. An intermediate suffix follows from
// the same index-derived expectation and was NOT executed;
// SECURITY.md's list carries a FOURTH this train did NOT execute —
// a row whose LOOKUP column changed storage class is indistinguishable
// from absence for the reader that seeks it.
// The ledger is tamper-evident, not tamper-proof (§23 honesty sentence);
// the tail anchor is filed to v0.15.1.
package cli

import (
	"context"
	"flag"
	"fmt"
)

// ledgerCmd dispatches the `ledger` noun's verbs.
func (c *cli) ledgerCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun ledger: expected a subcommand: check\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "check":
		return c.ledgerCheck(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun ledger: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// ledgerCheck implements `korvun ledger check`.
func (c *cli) ledgerCheck(args []string) int {
	fs := flag.NewFlagSet("ledger check", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	partition := fs.String("partition", "main", "chain partition to check")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun ledger check: --config is required\n")
		return 2
	}
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun ledger check: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	receipts, err := store.ListReceipts(ctx, *partition)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun ledger check: %v\n", err)
		return 1
	}
	// Pass 1 — the chain's STRUCTURE: sequence continuity over the whole
	// partition. A hole or a clone is a structural break and is denounced
	// before any per-link cryptography (a forged clone would otherwise
	// fail on its own hash first and hide the duplication).
	for i, r := range receipts {
		want := int64(i)
		switch {
		case r.ChainSeq > want:
			_, _ = fmt.Fprintf(c.stdout, "ledger %s: FAIL chain_seq_gap: position %d is missing (next receipt %s sits at seq %d)\n",
				*partition, want, r.ReceiptID, r.ChainSeq)
			return 1
		case r.ChainSeq < want:
			_, _ = fmt.Fprintf(c.stdout, "ledger %s: FAIL chain_seq_duplicate: receipt %s claims seq %d, already occupied\n",
				*partition, r.ReceiptID, r.ChainSeq)
			return 1
		}
	}
	// Pass 2 — every link: hash, signature, key window, predecessor
	// linkage, custody. The FIRST broken link stops the verdict.
	var noted int
	for _, r := range receipts {
		failures, notes := verifyReceiptChecks(ctx, store, r)
		if len(failures) > 0 {
			_, _ = fmt.Fprintf(c.stdout, "ledger %s: FAIL at receipt %s (seq %d): %s\n",
				*partition, r.ReceiptID, r.ChainSeq, failures[0])
			return 1
		}
		noted += len(notes)
	}
	if noted > 0 {
		_, _ = fmt.Fprintf(c.stdout, "ledger %s: NOTE %d receipt(s) with their action row pruned by retention — digest-sealed evidence stands\n", *partition, noted)
	}
	_, _ = fmt.Fprintf(c.stdout, "ledger %s: %d receipts, chain intact\n", *partition, len(receipts))
	return 0
}
