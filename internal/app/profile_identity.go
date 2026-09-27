// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The profile's identity for the durable mark «ledger founded by this
// profile» (director's order, 2026-09-24).
//
// WHAT A PROFILE IS, for the ledger. Not its content: every button on the
// screen edits it, and a digest of the bytes would leave the ledger foreign
// after the first change. Not an id written inside it: a copy carries the id,
// and writing one would be a profile change that the very block being built
// would refuse. A profile is its FILE — the absolute, cleaned path of the
// document Korvun loaded, symlinks resolved when they resolve (so `/tmp` and
// `/private/tmp` are one), folded to lower case on Windows (where the file
// system is case-insensitive). Stable under every edit; different when the
// profile is moved, copied elsewhere, or when another profile points at the
// same ledger. What it does not distinguish, and says: a copy that REPLACES
// the original at the same path is the same profile.

package app

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Sebastian197/korvun/internal/action"
)

// ProfileIdentity is the digest of a profile's identity — "sha256:<hex>" over
// the canonical document `{"profile":"<absolute path>"}` — or "" for no path.
//
// Not a secret, and not a privacy boundary: an absolute path is low-entropy
// (it carries the account name and the usual directories), so whoever reads
// the mark — in the founding receipt, in `ledger check`, in `receipt verify`,
// on the admin server's read door — can confirm a guessed path against it.
// The mark reaches those readers and, when a brain's act is refused on a
// foreign ledger, the refusal names both digests in the server's structured
// log. The read door is loopback by DEFAULT (the admin server's bind is the
// operator's choice, ADR-0020 §4). The path itself is never written in the
// ledger.
func ProfileIdentity(profilePath string) string {
	if profilePath == "" {
		return ""
	}
	abs, err := filepath.Abs(profilePath)
	if err != nil {
		abs = filepath.Clean(profilePath)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return action.HashCanonical(`{"profile":` + jsonString(abs) + `}`)
}

// WithProfilePath tells Build which profile FILE it is booting, so the ledger
// can be opened for it (the store's openers take the identity) and a ledger
// founded from this app is marked with it. The shell passes its config path; `korvun serve`, the
// -config flag. Without it the ledger is never judged and never marked.
func WithProfilePath(path string) Option {
	return func(b *builder) { b.profilePath = path }
}
