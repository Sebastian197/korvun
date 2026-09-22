// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The cure of the ficha «La forma del recibo vive solo en TypeScript».
//
// THE SEAM. The approvals screen refuses to paint an execution whose
// `receipt_id` is not `rcpt_` + 32 lowercase hex. Nothing on the Go side said
// so: `NewReceiptID` had no test at all, and the only check anywhere was a
// prefix in `internal/cli/receipt.go` — weaker than the screen's, so the two
// descriptions of the same bytes were already inconsistent. The adversary of
// block D shortened the minted id to 40 hex and left `internal/action`,
// `internal/action/executor`, `internal/action/sqlite`, `internal/app`,
// `internal/controlapi` and `internal/cli` ALL GREEN while the screen answered
// «esta pantalla no sabe leer su respuesta» to a real, sealed execution.
//
// No live defect existed — 16 random bytes are exactly 32 hex characters — and
// that is the point: the seam held by arithmetic nobody had written down.

// TestReceiptID_theMintedShapeIsTheOneTheScreenDemands is the half that would
// have caught the adversary's mutation.
func TestReceiptID_theMintedShapeIsTheOneTheScreenDemands(t *testing.T) {
	t.Parallel()
	for i := 0; i < 64; i++ {
		id := NewReceiptID()
		if !ValidReceiptID(id) {
			t.Fatalf("NewReceiptID minted %q, which the screen refuses", id)
		}
	}
}

// TestReceiptID_ValidReceiptIDRefusesWhatTheScreenRefuses walks the shapes the
// seam has to separate. Each row is a refusal the screen already performs, so a
// Go producer that emits one of these ships a receipt no operator can read.
func TestReceiptID_ValidReceiptIDRefusesWhatTheScreenRefuses(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name, id string
		want     bool
	}{
		{"the minted shape", "rcpt_0123456789abcdef0123456789abcdef", true},
		{"forty hex, the adversary's mutation", "rcpt_0123456789abcdef0123456789abcdef01234567", false},
		{"thirty hex", "rcpt_0123456789abcdef0123456789abcd", false},
		{"uppercase hex", "rcpt_0123456789ABCDEF0123456789ABCDEF", false},
		{"the prefix alone, which internal/cli would accept", "rcpt_", false},
		{"a fixture id, the shape controlapi's own tests emit", "rcpt_fake", false},
		{"no prefix", "0123456789abcdef0123456789abcdef", false},
		{"the approval prefix", "apr3_0123456789abcdef0123456789abcdef", false},
		{"trailing newline", "rcpt_0123456789abcdef0123456789abcdef\n", false},
		{"leading space", " rcpt_0123456789abcdef0123456789abcdef", false},
		{"empty", "", false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if got := ValidReceiptID(row.id); got != row.want {
				t.Fatalf("ValidReceiptID(%q) = %v, want %v", row.id, got, row.want)
			}
		})
	}
}

// TestReceiptID_theTwoLanguagesHoldOneShape is the seam itself. It reads the
// screen's own source and requires its literal to be the Go one, character for
// character. Either side moving alone reddens here instead of shipping a
// receipt the other cannot read.
//
// Evidence level, honest: it compares two SOURCE literals, not two running
// implementations. A Go regexp and a JavaScript regex with identical text can
// still differ in engine behaviour — what this pins is that nobody edits one
// description of these bytes without the other.
func TestReceiptID_theTwoLanguagesHoldOneShape(t *testing.T) {
	t.Parallel()
	screen := filepath.Join("..", "..", "cmd", "korvun-desktop", "frontend", "src", "views", "Approvals.tsx")
	body, err := os.ReadFile(screen) // #nosec G304 -- a path built from constants inside the repository
	if err != nil {
		t.Fatalf("the screen this seam is anchored to is unreadable: %v", err)
	}
	// `const RECEIPT_ID_RE = /<shape>/` — the anchor is the NAME, so moving the
	// declaration inside the file does not break this, and renaming it does.
	decl := regexp.MustCompile(`RECEIPT_ID_RE\s*=\s*/([^/\n]+)/`)
	all := decl.FindAllSubmatch(body, -1)
	if len(all) > 1 {
		t.Fatalf("%s declares RECEIPT_ID_RE %d times; this mould would pin "+
			"whichever came first", screen, len(all))
	}
	if len(all) == 0 {
		t.Fatalf("RECEIPT_ID_RE is gone from %s; the screen's half of this seam "+
			"was renamed or deleted without moving the Go half", screen)
	}
	if got := string(all[0][1]); got != receiptIDShape {
		t.Fatalf("the screen requires %q and Go mints against %q — the two "+
			"descriptions of one receipt have drifted", got, receiptIDShape)
	}
	// And the minter really is the thing being described, not a third shape
	// that happens to satisfy both literals.
	if !strings.HasPrefix(NewReceiptID(), "rcpt_") {
		t.Fatalf("the minter no longer produces the prefix both halves name")
	}
}
